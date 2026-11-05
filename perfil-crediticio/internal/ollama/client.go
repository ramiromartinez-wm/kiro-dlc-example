package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

var ErrOllamaUnavailable = errors.New("ollama service unavailable")

// Client es un cliente HTTP para Ollama
type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
	timeout    time.Duration
}

// Message representa un mensaje en el chat
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest representa una solicitud a /api/chat
type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// chatResponse representa una respuesta de /api/chat (no streaming)
type chatResponse struct {
	Model     string  `json:"model"`
	CreatedAt string  `json:"created_at"`
	Message   Message `json:"message"`
	Done      bool    `json:"done"`
}

// chatStreamResponse representa una línea de respuesta streaming
type chatStreamResponse struct {
	Model     string  `json:"model"`
	CreatedAt string  `json:"created_at"`
	Message   Message `json:"message"`
	Done      bool    `json:"done"`
}

// NewClient crea un nuevo cliente Ollama
func NewClient(baseURL, model string) *Client {
	return &Client{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		timeout: 60 * time.Second,
	}
}

// SetTimeout configura el timeout del cliente
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
}

// Chat realiza una llamada sin streaming a Ollama
// Retorna la respuesta completa, la duración de la llamada y cualquier error
func (c *Client) Chat(ctx context.Context, messages []Message) (string, time.Duration, error) {
	start := time.Now()

	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("ollama Chat: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", 0, fmt.Errorf("ollama Chat: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Detectar errores de conexión o timeout
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", time.Since(start), ErrOllamaUnavailable
		}
		return "", time.Since(start), fmt.Errorf("ollama Chat: request failed: %w", ErrOllamaUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Since(start), fmt.Errorf("ollama Chat: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", time.Since(start), fmt.Errorf("ollama Chat: failed to decode response: %w", err)
	}

	duration := time.Since(start)
	
	slog.InfoContext(ctx, "ollama chat completed",
		"model", c.model,
		"duration_ms", duration.Milliseconds(),
		"response_length", len(chatResp.Message.Content),
	)

	// Placeholder para métricas Prometheus (se registran en los servicios)
	// prometheus.HistogramVec.WithLabelValues("chat").Observe(duration.Seconds())

	return chatResp.Message.Content, duration, nil
}

// ChatStream realiza una llamada con streaming a Ollama
// Retorna un channel que emite tokens a medida que llegan
func (c *Client) ChatStream(ctx context.Context, messages []Message) (<-chan string, error) {
	reqBody := chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama ChatStream: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama ChatStream: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, ErrOllamaUnavailable
		}
		return nil, fmt.Errorf("ollama ChatStream: request failed: %w", ErrOllamaUnavailable)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("ollama ChatStream: unexpected status %d", resp.StatusCode)
	}

	// Crear channel para tokens
	tokenCh := make(chan string, 10)

	// Goroutine para leer el stream
	go func() {
		defer close(tokenCh)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var streamResp chatStreamResponse
			if err := json.Unmarshal(line, &streamResp); err != nil {
				slog.ErrorContext(ctx, "failed to parse stream line", "error", err)
				continue
			}

			// Enviar contenido al channel
			if streamResp.Message.Content != "" {
				select {
				case tokenCh <- streamResp.Message.Content:
				case <-ctx.Done():
					return
				}
			}

			// Finalizar si done=true
			if streamResp.Done {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			slog.ErrorContext(ctx, "stream scanner error", "error", err)
		}
	}()

	return tokenCh, nil
}
