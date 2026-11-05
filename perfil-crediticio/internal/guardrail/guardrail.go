package guardrail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"
)

// MinioClient es una interfaz para el cliente MinIO
// Permite desacoplar y facilitar testing
type MinioClient interface {
	GetObjectJSON(ctx context.Context, prefix, key string, target any) error
	GetObjectText(ctx context.Context, prefix, key string) (string, error)
}

// Guardrail maneja las reglas de guardrails para filtrar contenido no deseado
type Guardrail struct {
	patterns    []*regexp.Regexp
	mu          sync.RWMutex
	minioClient MinioClient
	stopRefresh chan struct{}
}

// guardrailRule representa una regla de guardrail desde MinIO
type guardrailRule struct {
	Pattern     string `json:"pattern"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type guardrailRules struct {
	Rules []guardrailRule `json:"rules"`
}

// New crea un nuevo Guardrail cargando reglas desde MinIO
// Inicia una goroutine de refresh cada 5 minutos
func New(ctx context.Context, minioClient MinioClient) (*Guardrail, error) {
	g := &Guardrail{
		minioClient: minioClient,
		stopRefresh: make(chan struct{}),
	}

	// Cargar reglas iniciales
	if err := g.loadRules(ctx); err != nil {
		return nil, fmt.Errorf("guardrail New: failed to load initial rules: %w", err)
	}

	// Iniciar refresh periódico
	go g.refreshLoop(ctx)

	slog.InfoContext(ctx, "guardrail initialized", "patterns", len(g.patterns))
	return g, nil
}

// loadRules carga las reglas de guardrails desde MinIO
func (g *Guardrail) loadRules(ctx context.Context) error {
	var rules guardrailRules
	err := g.minioClient.GetObjectJSON(ctx, "seed", "guardrail-rules.json", &rules)
	if err != nil {
		return fmt.Errorf("guardrail loadRules: failed to load rules: %w", err)
	}

	// Compilar patrones
	patterns := make([]*regexp.Regexp, 0, len(rules.Rules))
	for _, rule := range rules.Rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			slog.WarnContext(ctx, "failed to compile pattern",
				"pattern", rule.Pattern,
				"error", err,
			)
			continue
		}
		patterns = append(patterns, re)
	}

	// Actualizar patrones de forma thread-safe
	g.mu.Lock()
	g.patterns = patterns
	g.mu.Unlock()

	slog.InfoContext(ctx, "guardrail rules loaded",
		"total", len(rules.Rules),
		"compiled", len(patterns),
	)

	return nil
}

// refreshLoop recarga las reglas cada 5 minutos
func (g *Guardrail) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := g.loadRules(ctx); err != nil {
				slog.ErrorContext(ctx, "failed to refresh guardrail rules", "error", err)
			}
		case <-g.stopRefresh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop detiene el loop de refresh
func (g *Guardrail) Stop() {
	close(g.stopRefresh)
}

// Apply aplica los guardrails al texto dado
// Retorna el texto procesado y un booleano indicando si fue bloqueado
// Si bloqueado, retorna la narrativa fallback
func (g *Guardrail) Apply(ctx context.Context, text, fallbackNarrativeKey string) (result string, blocked bool) {
	g.mu.RLock()
	patterns := g.patterns
	g.mu.RUnlock()

	// Verificar cada patrón
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			slog.WarnContext(ctx, "guardrail blocked content",
				"pattern", pattern.String(),
				"fallback", fallbackNarrativeKey,
			)

			// Cargar fallback
			fallback := g.LoadFallback(ctx, fallbackNarrativeKey)
			return fallback, true
		}
	}

	// No bloqueado, retornar texto original
	return text, false
}

// LoadFallback carga una narrativa fallback desde MinIO
func (g *Guardrail) LoadFallback(ctx context.Context, narrativeKey string) string {
	// Intentar cargar la narrativa específica
	var narrative struct {
		Content string `json:"content"`
	}

	err := g.minioClient.GetObjectJSON(ctx, "seed/narratives", narrativeKey+".json", &narrative)
	if err != nil {
		slog.ErrorContext(ctx, "failed to load fallback narrative",
			"key", narrativeKey,
			"error", err,
		)
		// Narrativa fallback genérica
		return "Lo siento, no puedo procesar tu solicitud en este momento. Por favor, contacta a soporte."
	}

	return narrative.Content
}

// GetPatternsCount retorna el número de patrones cargados
func (g *Guardrail) GetPatternsCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.patterns)
}
