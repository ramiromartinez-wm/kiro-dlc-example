# Implementation Tasks

Stack: **Go 1.23**, router `chi`, `aws-sdk-go-v2`, `go-redis/v9`, `prometheus/client_golang`, `opentelemetry-go`. Docker Compose para toda la infraestructura.

Orden: infraestructura base → datos/seed → paquetes internos → servicios → observabilidad → integración → tests.

---

## Fase 1 — Infraestructura base (Docker Compose + servicios de plataforma)

### Tarea 1.1 — Estructura del proyecto
- [x] Crear directorio raíz `perfil-crediticio/` con la estructura definida en `design.md`
- [x] Inicializar `go.work` con `go work init` (workspace multi-módulo)
- [x] Crear `.env.example` con todas las variables documentadas en `design.md`
- [x] Crear `.gitignore` (excluir `.env`, `bin/`, volúmenes Docker, `*.test`)

### Tarea 1.2 — `docker-compose.yml` — servicios de plataforma
- [x] Definir red `creditprofile-net` (bridge)
- [x] Servicio `dynamodb-local`: imagen `amazon/dynamodb-local`, puerto `8888:8000`, volumen `dynamo-data`, healthcheck (`curl -f http://localhost:8000`)
- [x] Servicio `minio`: imagen `minio/minio`, puertos `9000:9000` y `9001:9001`, volumen `minio-data`, healthcheck (`curl -f http://localhost:9000/minio/health/live`)
- [x] Servicio `redis`: imagen `redis:7-alpine`, puerto `6379:6379`, healthcheck (`redis-cli ping`)
- [x] Servicio `ollama`: imagen `ollama/ollama`, puerto `11434:11434`, volumen `ollama-models`
- [x] Todos con `restart: unless-stopped` y red `creditprofile-net`

### Tarea 1.3 — `docker-compose.yml` — observabilidad
- [x] Servicio `prometheus`: imagen `prom/prometheus`, montar `observability/prometheus.yml`, puerto `9090:9090`
- [x] Servicio `grafana`: imagen `grafana/grafana`, puerto `3000:3000`, provisionar datasources y dashboards vía volumen
- [x] Servicio `loki`: imagen `grafana/loki`, montar `observability/loki-config.yml`, puerto `3100:3100`
- [x] Servicio `promtail`: imagen `grafana/promtail`, montar socket Docker `/var/run/docker.sock` y `observability/promtail-config.yml`
- [x] Servicio `jaeger`: imagen `jaegertracing/all-in-one`, puertos `16686:16686` (UI) y `4317:4317` (OTLP gRPC)
- [x] Escribir `observability/prometheus.yml`: scrape de `api-gateway:8000`, `profile-service:8001`, `assistant-service:8002`, `dispute-service:8003`, `support-mock:8004`
- [x] Escribir `observability/loki-config.yml` básico
- [x] Escribir `observability/promtail-config.yml`: recolectar logs de contenedores por label `com.docker.compose.service`

---

## Fase 2 — Datos y seed

### Tarea 2.1 — Seed data JSON (`data/seed/`)
- [ ] `narratives/state-mapping.json`: objeto con 8 claves de estado → 4 narrative keys (R10)
- [ ] `narratives/narrative-1.json`: `{ "key": "grow", "text": "Tenés línea y podés hacerla crecer", "prompt_context": "..." }`
- [ ] `narratives/narrative-2.json`: `{ "key": "unlock", "text": "Podés destrabar tu línea", "prompt_context": "..." }`
- [ ] `narratives/narrative-3.json`: narrativa rechazo tipo A
- [ ] `narratives/narrative-4.json`: narrativa rechazo tipo B
- [ ] `actionables-context/resolution-windows.json`: mapa de tipo de accionable → días estimados de resolución (R9)
- [ ] `dispute-guides/bcra.json`: `{ "steps": [...], "links": [...], "contact": "..." }` (R8)
- [ ] `dispute-guides/bank.json` (R8)
- [ ] `reference-data/bcra-situations.json`: definición de situaciones 1-6 del BCRA (R6)
- [ ] `reference-data/glossary.json`: términos crediticios en español (R6)
- [ ] `guardrail-rules.json`: `{ "patterns": ["garantiz", "seguro que", "vas a ser aprobad"], "fallback_key": "narrative-3" }` (R12)

### Tarea 2.2 — `scripts/init-dynamo/main.go`
- [ ] Crear tabla `user_credit_profile` (PK: `user_id` String; TTL en atributo `ttl`)
- [ ] Crear tabla `credit_decisions_history` (PK: `user_id`; SK: `decision_timestamp`)
- [ ] Crear tabla `dispute_audit_log` (PK: `user_id`; SK: `timestamp`)
- [ ] Usar `aws-sdk-go-v2/dynamodb` con `EndpointResolverV2` apuntando a `DYNAMODB_ENDPOINT`
- [ ] Manejo de error `ResourceInUseException` (tabla ya existe → skip silencioso)
- [ ] Servicio efímero `dynamo-init` en docker-compose: `depends_on: dynamodb-local` (condición `service_healthy`)

### Tarea 2.3 — `scripts/init-minio/main.go`
- [ ] Crear bucket `credit-profile-assets` si no existe
- [ ] Subir todos los archivos de `data/seed/` recursivamente con `aws-sdk-go-v2/s3`
- [ ] Usar `endpoint_url = MINIO_ENDPOINT`, path-style addressing (`UsePathStyle: true`)
- [ ] Servicio efímero `minio-init` en docker-compose: `depends_on: minio` (condición `service_healthy`)

### Tarea 2.4 — `scripts/seed-dynamo/main.go`
- [ ] Insertar usuario aprobado: `user_001` (con accionables directos e indirectos, `decision_state: "approved"`)
- [ ] Insertar usuario rechazado: `user_002` (`decision_state: "rejected"`, 3 factores bloqueantes)
- [ ] Insertar usuario bloqueado: `user_003` (`decision_state: "1P_rejected"`) (R17)
- [ ] Insertar historial de decisiones para `user_001` y `user_002`
- [ ] Servicio efímero `dynamo-seed` en docker-compose: `depends_on: dynamo-init`

### Tarea 2.5 — Pull modelo Ollama
- [ ] Servicio efímero `ollama-pull` en docker-compose: comando `ollama pull ${OLLAMA_MODEL}`, `depends_on: ollama`

---

## Fase 3 — Paquetes internos (`internal/`)

### Tarea 3.1 — `internal/auth/jwt.go`
- [ ] Función `NewJWTMiddleware(secret string) func(http.Handler) http.Handler` compatible con `chi`
- [ ] Parsear y verificar token Bearer con `golang-jwt/jwt/v5`, `SigningMethodHS256`
- [ ] Extraer claim `sub` → comparar con `chi.URLParam(r, "userID")` → HTTP 403 si no coincide (R4, R6, R8)
- [ ] HTTP 401 si token ausente o inválido
- [ ] Inyectar `userID` en `context.Context` para uso en handlers

### Tarea 3.2 — `internal/dynamo/client.go`
- [ ] Struct `Client` con `*dynamodb.Client` configurado vía `aws-sdk-go-v2`
- [ ] `GetUserProfile(ctx, userID) (*UserProfile, error)`
- [ ] `PutUserProfile(ctx, profile *UserProfile) error`
- [ ] `GetDecisionsHistory(ctx, userID) ([]Decision, error)`
- [ ] `LogDispute(ctx, entry *DisputeLog) error`
- [ ] Tipos Go para `UserProfile`, `Decision`, `DisputeLog` con tags `dynamodbav`
- [ ] Wrap de errores con contexto (`fmt.Errorf("dynamo GetUserProfile: %w", err)`)

### Tarea 3.3 — `internal/minio/client.go`
- [ ] Struct `Client` con `*s3.Client` configurado con `UsePathStyle: true`
- [ ] `GetObjectJSON(ctx, prefix, key string, target any) error` — descarga y deserializa JSON
- [ ] Cache en memoria con TTL 30 s usando `sync.Map` + timestamp (evitar llamadas repetidas a MinIO)
- [ ] `PutObject(ctx, key string, data []byte) error` (usado por scripts de init)

### Tarea 3.4 — `internal/ollama/client.go`
- [ ] Struct `Client` con `baseURL` y `model` de env vars
- [ ] `Chat(ctx, messages []Message) (string, time.Duration, error)` — llama a `/api/chat` con `stream: false`
- [ ] `ChatStream(ctx, messages []Message) (<-chan string, error)` — llama a `/api/chat` con `stream: true`, devuelve channel de tokens
- [ ] Timeout configurable (default 60 s)
- [ ] En `context.DeadlineExceeded` o error de conexión → retorna `ErrOllamaUnavailable`
- [ ] Registrar duración en Histogram Prometheus `llm_request_duration_seconds` (R18)

### Tarea 3.5 — `internal/guardrail/guardrail.go`
- [ ] Struct `Guardrail` con lista de `*regexp.Regexp` compilados al iniciar y mutex para refresh
- [ ] `New(ctx context.Context, minioClient *minio.Client) (*Guardrail, error)` — carga `guardrail-rules.json` de MinIO
- [ ] Goroutine de refresh cada 5 minutos (re-lee reglas de MinIO)
- [ ] `Apply(ctx context.Context, text, fallbackNarrativeKey string) (result string, blocked bool)`:
  - Itera patrones; si hay match → log `slog.WarnContext` ("guardrail blocked", sin PII) → retorna (fallback, true)
  - Si no → retorna (text, false)
- [ ] `LoadFallback(ctx, narrativeKey string) string` — lee narrativa de MinIO como texto plano

---

## Fase 4 — Servicios Go

### Tarea 4.1 — `services/api-gateway/main.go`
- [ ] Inicializar `chi.Router`
- [ ] Aplicar `JWTMiddleware` globalmente (de `internal/auth`)
- [ ] Aplicar middleware `prometheus-fastapi-instrumentator` equivalente: `github.com/prometheus/client_golang` con handler `/metrics`
- [ ] Aplicar middleware OpenTelemetry: `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`
- [ ] Rutas proxy con `httputil.ReverseProxy` o `httpx` manual:
  - `GET /user/{userID}/credit-profile` → `profile-service:8001`
  - `POST /user/{userID}/ask` → `assistant-service:8002` (con streaming: flush headers + body)
  - `POST /user/{userID}/dispute` → `dispute-service:8003`
- [ ] `GET /health` sin middleware → `200 OK`
- [ ] `go.mod`: módulo `gateway`, dependencias `chi`, `golang-jwt/jwt/v5`, `prometheus/client_golang`, `otel`
- [ ] `Dockerfile` multi-stage (builder `golang:1.23-alpine` → runtime `scratch`)

### Tarea 4.2 — `services/profile-service/main.go`
- [ ] Inicializar clientes: `dynamo.Client`, `minio.Client`, `ollama.Client`, `guardrail.Guardrail`
- [ ] Registrar métricas Prometheus: `profile_views_total` (Counter), `retry_date_shown_total` (Counter)
- [ ] `GET /profile/{userID}` — flujo completo (ver design.md):
  1. `dynamo.GetUserProfile` → 404 si no existe con respuesta empty-state (R4)
  2. Check `BLOCKED_STATES` → 403 (R17)
  3. Cache check (`cached_explanation` + `ttl`) → return si válido (R1)
  4. `minio.GetObjectJSON("narratives/", "state-mapping.json")` → seleccionar narrative key (R10)
  5. `minio.GetObjectJSON("narratives/", narrativeKey+".json")` → texto narrativa
  6. Si accionables directos → calcular `suggested_retry_date` con `resolution-windows.json` (R9)
  7. Construir `[]ollama.Message` con contexto del usuario
  8. `ollama.Chat()` → en `ErrOllamaUnavailable`: cargar fallback de MinIO (R10)
  9. `guardrail.Apply()` (R12)
  10. `dynamo.PutUserProfile()` con `cached_explanation` y TTL
  11. Responder `200 OK` con struct JSON; campos faltantes → omitempty, no 500 (R5)
  12. Emitir `profile_views_total.Inc()`; si `suggested_retry_date` presente → `retry_date_shown_total.Inc()`
- [ ] `POST /profile/{userID}/pre-compute` — mismo flujo, responder `202 Accepted` sin body
- [ ] `go.mod` + `Dockerfile`

### Tarea 4.3 — `services/assistant-service/main.go`
- [ ] Registrar métricas: `assistant_sessions_total`, `support_deflection_total`, `support_escalation_total`
- [ ] `POST /ask/{userID}`:
  1. Parsear `{ "question": "...", "session_id": "..." }`
  2. Cargar historial de sesión de Redis (`redis.HGetAll("session:{sessionID}")`)
  3. Scope check: si ninguna keyword del slice `scopeKeywords` aparece en la pregunta (case-insensitive) → out-of-scope
  4. **In-scope**:
     - `dynamo.GetUserProfile` → construir contexto
     - Armar historial + nueva pregunta como `[]ollama.Message`
     - `ollama.ChatStream()` → iterar channel → escribir chunks SSE (`data: ...\n\n`) → flush con `http.Flusher`
     - `guardrail.Apply()` sobre respuesta acumulada; si bloqueado → enviar evento SSE con fallback
     - `redis.HSet("session:{sessionID}", ...)` + `redis.Expire(3600)`
     - `assistant_sessions_total.Inc()`, `support_deflection_total.Inc()`
  5. **Out-of-scope**:
     - `http.Post("http://support-mock:8004/tickets", ...)` con `user_id` + `question`
     - Responder `{ "escalated": true, "ticket_id": "...", "eta": "24h", "message": "Tu consulta fue derivada al equipo de soporte." }`
     - `support_escalation_total.Inc()`
- [ ] `go.mod` + `Dockerfile`

### Tarea 4.4 — `services/dispute-service/main.go`
- [ ] Registrar métrica `dispute_flows_initiated_total`
- [ ] `POST /dispute/{userID}`:
  1. Parsear `{ "disputed_field": "...", "dispute_type": "bcra|bank" }`
  2. `minio.GetObjectJSON("dispute-guides/", disputeType+".json")`
  3. `dynamo.LogDispute()` → tabla `dispute_audit_log` (R8)
  4. `dispute_flows_initiated_total.Inc()`
  5. Responder con guía JSON
- [ ] `go.mod` + `Dockerfile`

### Tarea 4.5 — `services/support-mock/main.go`
- [ ] `sync.Map` para almacenar tickets en memoria
- [ ] `POST /tickets`: generar UUID (`crypto/rand`), guardar en map → `{ "ticket_id": "...", "eta": "24h" }` (R7)
- [ ] `GET /tickets/{id}`: lookup en map → 404 si no existe
- [ ] `GET /health`
- [ ] `go.mod` + `Dockerfile`

### Tarea 4.6 — `services/event-consumer/main.go`
- [ ] Conectar a Redis con `go-redis/v9` usando `REDIS_URL`
- [ ] Bucle principal: `client.XRead(ctx, &redis.XReadArgs{Streams: []string{"credit-decisions", "0"}, Block: 5 * time.Second, Count: 1})`
- [ ] Por cada mensaje: extraer `user_id` del payload → `http.Post("http://profile-service:8001/profile/{userID}/pre-compute", ...)`
- [ ] Retry: hasta 2 intentos con `time.Sleep(2^attempt * time.Second)`
- [ ] Si falla tras reintentos: `client.XAdd(ctx, &redis.XAddArgs{Stream: "dlq", Values: map})` (R15)
- [ ] Actualizar puntero con `client.XAck` tras procesamiento exitoso
- [ ] Logs con `slog.InfoContext` / `slog.ErrorContext`
- [ ] `go.mod` + `Dockerfile`

### Tarea 4.7 — Agregar todos los servicios al `docker-compose.yml`
- [ ] Servicios: `api-gateway` (:8000), `profile-service` (:8001), `assistant-service` (:8002), `dispute-service` (:8003), `support-mock` (:8004), `event-consumer` (sin puerto)
- [ ] Todos: `build: ./services/<name>`, `env_file: .env`, red `creditprofile-net`
- [ ] `depends_on` con `service_healthy`: plataforma + servicios init
- [ ] Healthchecks: `GET /health` para los 5 servicios HTTP
- [ ] `event-consumer`: `depends_on: profile-service` (condición `service_healthy`) + `redis`

---

## Fase 5 — Observabilidad

### Tarea 5.1 — Métricas Prometheus en servicios Go
- [ ] Paquete interno `internal/metrics/metrics.go`: definir y registrar todos los Counters e Histogramas
- [ ] Cada servicio importa `internal/metrics` y llama `metrics.Register()`
- [ ] Handler `/metrics` con `promhttp.Handler()` en cada servicio
- [ ] Verificar scrape desde Prometheus UI (`http://localhost:9090`)

### Tarea 5.2 — OpenTelemetry + Jaeger
- [ ] Función `internal/telemetry/tracer.go`: `InitTracer(serviceName string) (func(), error)` — configura `TracerProvider` con `OTLPSpanExporter` gRPC a `OTEL_EXPORTER_OTLP_ENDPOINT`
- [ ] Cada servicio llama `InitTracer` al arrancar y hace `defer shutdown()`
- [ ] Instrumentar handlers con `otelhttp.NewHandler(router, "service-name")`
- [ ] Instrumentar `aws-sdk-go-v2` con `go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws`
- [ ] Verificar trazas en Jaeger UI (`http://localhost:16686`)

### Tarea 5.3 — Dashboard Grafana
- [ ] Crear `observability/grafana/dashboards/credit-profile.json` con panels:
  - `rate(profile_views_total[5m])` (R19)
  - `rate(assistant_sessions_total[5m])` (R19)
  - `rate(support_deflection_total[5m]) / (rate(support_deflection_total[5m]) + rate(support_escalation_total[5m]))` → tasa de deflexión % (R13)
  - `rate(dispute_flows_initiated_total[5m])` (R19)
  - `rate(retry_date_shown_total[5m])` (R19)
  - Histograma `llm_request_duration_seconds` p50/p95/p99 (R18)
  - Error rate HTTP 5xx por servicio
- [ ] Provisionar dashboard automáticamente vía `observability/grafana/provisioning/dashboards/`

### Tarea 5.4 — Logs Loki
- [ ] Configurar `promtail-config.yml` para recolectar logs de Docker con label `job=creditprofile`
- [ ] Verificar en Grafana → Explore → Loki: `{job="creditprofile"}`

---

## Fase 6 — Makefile y README

### Tarea 6.1 — `Makefile`
- [ ] `make up`: `docker compose up -d --build`
- [ ] `make init`: ejecutar servicios init en orden (`dynamo-init` → `minio-init` → `dynamo-seed` → `ollama-pull`)
- [ ] `make down`: `docker compose down -v`
- [ ] `make logs`: `docker compose logs -f`
- [ ] `make test`: `go test ./tests/integration/... -v -timeout 120s`
- [ ] `make token USER_ID=user_001`: genera JWT de prueba con `go run scripts/gen-token/main.go user_001`
- [ ] `make tidy`: `go work sync && go mod tidy` en cada módulo

### Tarea 6.2 — `README.md`
- [ ] Requisitos: Docker Desktop ≥ 4.x, Go 1.23+, `make`, 8 GB RAM (por Ollama)
- [ ] Inicio rápido: `cp .env.example .env && make up && make init`
- [ ] Tabla de URLs: api-gateway, MinIO console, Grafana, Jaeger, Prometheus
- [ ] Ejemplos `curl` para los 4 flujos con JWT generado por `make token`
- [ ] Nota: cambiar modelo → `OLLAMA_MODEL=mistral make up && make init`

---

## Fase 7 — Tests de integración

### Tarea 7.1 — `tests/integration/flows_test.go`
- [ ] `TestMain`: verificar que todos los servicios respondan `/health`; generar JWT de prueba con `JWT_SECRET` del `.env`
- [ ] `TestGetProfile_OK` (R1, R5): `GET /user/user_001/credit-profile` → verificar `line_status`, `profile_level`, `primary_factors`, `bcra_cutoff_date`, `explanation` presentes
- [ ] `TestGetProfile_Blocked` (R17): `GET /user/user_003/credit-profile` → HTTP 403
- [ ] `TestGetProfile_CacheHit` (R1): segunda llamada a `user_001` es más rápida que la primera (campo `cached_explanation` en DynamoDB populado)
- [ ] `TestAsk_InScope` (R6): `POST /user/user_001/ask` con `"question": "¿qué significa estar en situación 2?"` → respuesta con contenido
- [ ] `TestAsk_OutOfScope` (R7): `POST /user/user_001/ask` con `"question": "¿cómo cambio mi contraseña?"` → `escalated: true` + `ticket_id`; verificar ticket en `support-mock`
- [ ] `TestDispute_OK` (R8): `POST /user/user_001/dispute` con `dispute_type: "bcra"` → respuesta con `steps`; verificar audit log en DynamoDB
- [ ] `TestEventConsumer_PreCompute` (R15): publicar en Redis stream `credit-decisions`; esperar 3 s; verificar `cached_explanation` en DynamoDB para `user_002`
- [ ] `TestGuardrail_Blocks` (R12): manipular `guardrail-rules.json` en MinIO con patrón que matchee la respuesta; verificar que la respuesta es el fallback
- [ ] `TestRetryDate_Shown` (R9): `user_001` tiene accionable directo → `suggested_retry_date` no vacío en respuesta
- [ ] `TestFallbackNarrative` (R10): insertar usuario con `decision_state: "unknown_xyz"` → responder con narrativa fallback, no 500
