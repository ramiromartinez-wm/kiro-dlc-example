# Design Document

## Overview

Perfil Crediticio corre **100% local** con Docker Compose. Todos los servicios están escritos en **Go 1.23** con el router `chi`. Reemplaza todos los servicios AWS gestionados por equivalentes open-source o emuladores oficiales.

Traceability: requirements.md R1–R19.

---

## Mapa de equivalencias AWS → local

| Servicio AWS | Equivalente local | Imagen Docker |
|---|---|---|
| API Gateway + Lambda | **Go + chi** (un binario por servicio) | `golang:1.23-alpine` (build) → `scratch` (runtime) |
| Amazon Bedrock (Claude Sonnet) | **Ollama** (`llama3.2` o `mistral`) | `ollama/ollama` |
| Bedrock AgentCore | Orquestador Go en `assistant-service` | — |
| Bedrock Guardrails | Middleware Go en paquete `guardrail` | — |
| DynamoDB | **DynamoDB Local** (oficial de Amazon) | `amazon/dynamodb-local` |
| Amazon S3 | **MinIO** | `minio/minio` |
| EventBridge + regla | **Redis Streams** | `redis:7-alpine` |
| SQS DLQ | Stream separado `dlq` en Redis | — |
| Secrets Manager | Archivo **`.env`** montado como volumen | — |
| CloudWatch Logs | **stdout JSON → Loki** (vía Promtail) | `grafana/loki`, `grafana/promtail` |
| CloudWatch Metrics | **Prometheus + Grafana** | `prom/prometheus`, `grafana/grafana` |
| AWS X-Ray | **Jaeger** (OpenTelemetry) | `jaegertracing/all-in-one` |
| Cognito / JWT auth | Middleware JWT con `golang-jwt/jwt/v5` | — |
| Zendesk | `support-mock` en Go | — |

---

## Stack tecnológico Go

| Responsabilidad | Librería |
|---|---|
| HTTP router | `github.com/go-chi/chi/v5` |
| JWT | `github.com/golang-jwt/jwt/v5` |
| DynamoDB | `github.com/aws/aws-sdk-go-v2/service/dynamodb` |
| S3 / MinIO | `github.com/aws/aws-sdk-go-v2/service/s3` |
| Redis | `github.com/redis/go-redis/v9` |
| HTTP client | `net/http` stdlib |
| Prometheus | `github.com/prometheus/client_golang/prometheus` |
| OpenTelemetry | `go.opentelemetry.io/otel` + OTLP exporter |
| Logs estructurados | `log/slog` (stdlib Go 1.21+) |
| Config / env | `github.com/caarlos0/env/v11` |
| Tests | `testing` stdlib + `github.com/stretchr/testify` |

---

## Arquitectura local

```
┌─────────────────────────────────────────────────────────────────┐
│  Cliente HTTP (curl / frontend / Postman)                       │
└────────────────────┬────────────────────────────────────────────┘
                     │  HTTP :8000
                     ▼
┌─────────────────────────────────────────────────────────────────┐
│  api-gateway  (Go/chi · puerto 8000)                            │
│  GET  /user/{id}/credit-profile  →  profile-service            │
│  POST /user/{id}/ask             →  assistant-service           │
│  POST /user/{id}/dispute         →  dispute-service             │
│  Middleware JWT (golang-jwt/jwt/v5)                             │
└──────┬─────────────────────┬─────────────────────────────────────┘
       │                     │
       ▼                     ▼
┌──────────────┐    ┌──────────────────────────┐
│profile-service│   │assistant-service          │
│(Go/chi :8001) │   │(Go/chi :8002)            │
│ - GenerateExp │   │ - scope guard             │
│ - CalcRetry  │   │ - ProcessConvQuery        │
│ - cache check│   │ - DeriveToSupport         │
└──────┬───────┘   └────────┬─────────────────┘
       │                    │
       ▼                    ▼
┌───────────────────────────────────────────────┐
│  dispute-service  (Go/chi :8003)              │
│  - HandleDisputeFlow                          │
└──────┬────────────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────────────────────────────┐
│  DynamoDB Local  (amazon/dynamodb-local) · puerto 8888         │
│  Tablas: user_credit_profile, credit_decisions_history,        │
│          dispute_audit_log                                      │
└────────────────────────────────────────────────────────────────┘

┌──────────────────────────────┐
│  MinIO  :9000/:9001          │
│  Bucket: credit-profile-assets│
│  narratives/, dispute-guides/ │
│  actionables-context/         │
│  reference-data/              │
└──────────────────────────────┘

┌──────────────────────────────┐
│  Ollama  :11434              │
│  Modelo: llama3.2 (o mistral)│
│  POST /api/chat              │
└──────────────────────────────┘

┌──────────────────────────────┐
│  Redis  :6379                │
│  Stream: credit-decisions    │
│  Stream: dlq                 │
└──────────────────────────────┘

┌──────────────────────────────┐     ┌──────────────────┐
│  event-consumer  (Go)        │◀────│  Redis Stream    │
│  - XREAD credit-decisions    │     │  credit-decisions│
│  - POST profile-service      │     └──────────────────┘
└──────────────────────────────┘

┌──────────────────────────────┐
│  support-mock  (Go :8004)    │
│  POST /tickets               │
│  GET  /tickets/{id}          │
└──────────────────────────────┘

Observabilidad
┌──────────────────────────────────────────────────────┐
│  Prometheus :9090  ←  /metrics de cada servicio Go   │
│  Grafana    :3000  ←  dashboards                     │
│  Loki       :3100  ←  logs JSON (via Promtail)       │
│  Jaeger     :16686 ←  trazas OTLP                    │
└──────────────────────────────────────────────────────┘
```

---

## Servicios

### api-gateway (puerto 8000)

- **Framework**: Go + `chi`
- **Responsabilidad**: enrutar requests a los servicios internos, validar JWT
- **Auth**: middleware `JWTMiddleware` que verifica el Bearer token con `golang-jwt/jwt/v5` usando `JWT_SECRET`. Extrae `user_id` del claim `sub` y lo compara con el path param.
- **Rutas**:
  - `GET /user/{userID}/credit-profile` → proxy a `http://profile-service:8001/profile/{userID}`
  - `POST /user/{userID}/ask` → proxy con streaming a `http://assistant-service:8002/ask/{userID}`
  - `POST /user/{userID}/dispute` → proxy a `http://dispute-service:8003/dispute/{userID}`
- **Sin estado** — no accede a DynamoDB ni a Ollama directamente.
- **Build**: imagen multi-stage (`golang:1.23-alpine` → `scratch`). Binario estático ~8 MB.

### profile-service (puerto 8001)

Equivale a `GenerateProfileExplanation` + `CalculateRetryDate`.

**Flujo `GET /profile/{userID}`**:
1. Leer `user_credit_profile` de DynamoDB Local con `aws-sdk-go-v2`.
2. Si `decision_state` en `BLOCKED_STATES` (env var, slice de strings) → HTTP 403 (R17).
3. Si `cached_explanation` existe y `ttl` no expiró → devolver directo (R1).
4. Leer `narratives/state-mapping.json` de MinIO con `aws-sdk-go-v2/s3` (R10).
5. Leer narrativa correspondiente de MinIO.
6. Si hay accionables directos → llamar lógica `CalculateRetryDate` (R9).
7. Construir prompt; llamar `http://ollama:11434/api/chat` con `net/http` (R1, R2, R3).
8. Aplicar `guardrail.Apply(text)` del paquete interno (R12).
9. Guardar `cached_explanation` + `suggested_retry_date` en DynamoDB.
10. Responder con struct JSON: `line_status`, `profile_level`, `primary_factors`, `bcra_cutoff_date`, `explanation`, `actionables`, `suggested_retry_date`.

**Error handling**:
- Ollama no disponible → devolver narrativa de fallback desde MinIO (R10).
- Campos faltantes en DynamoDB → respuesta parcial, nunca 500 (R5).
- `bcra_cutoff_date` vacío → nota genérica hardcodeada (R16).

**Endpoint adicional**: `POST /profile/{userID}/pre-compute` (mismo flujo, sin body de respuesta, llamado por `event-consumer`).

### assistant-service (puerto 8002)

Equivale a AgentCore + `ProcessConversationalQuery` + `DeriveToSupport`.

**Flujo `POST /ask/{userID}`**:
1. Parsear `{ "question": "...", "session_id": "..." }`.
2. Cargar historial de sesión desde Redis (`HGETALL session:{sessionID}`) (R6).
3. Evaluar scope: lista de keywords de dominio (crédito, BCRA, perfil, línea, etc.). Strings simples, sin regex pesado.
4. **In-scope**: leer DynamoDB → armar prompt con historial → `ollama/api/chat` → `guardrail.Apply()` → responder.
5. **Out-of-scope**: `POST http://support-mock:8004/tickets` → devolver `{ "escalated": true, "ticket_id": "...", "eta": "24h" }` (R7).
6. Guardar turno en Redis `HSET session:{sessionID}` con `EXPIRE 3600` (R6).
7. Streaming: para in-scope, usar `http.Flusher` para enviar chunks SSE mientras Ollama devuelve tokens (R6).

### dispute-service (puerto 8003)

Equivale a `HandleDisputeFlow`.

**Flujo `POST /dispute/{userID}`**:
1. Parsear `{ "disputed_field": "...", "dispute_type": "bcra|bank" }`.
2. Leer `dispute-guides/{dispute_type}.json` de MinIO (R8).
3. Escribir registro en DynamoDB tabla `dispute_audit_log` (sin PII extra) (R8).
4. Emitir counter Prometheus `dispute_flows_initiated_total` (R19).
5. Devolver guía (pasos, links, contactos).

### support-mock (puerto 8004)

Servicio Go/chi mínimo que simula Zendesk.

- `POST /tickets` → guarda ticket en `sync.Map` en memoria → retorna `{ "ticket_id": "<uuid>", "eta": "24h" }` (R7).
- `GET /tickets/{id}` → retorna estado.
- `GET /health` → `200 OK`.

### event-consumer

Binario Go independiente. Equivale al trigger EventBridge → Lambda.

- Bucle: `XREAD COUNT 1 BLOCK 5000 STREAMS credit-decisions $` con `go-redis/v9`.
- Por cada mensaje: `POST http://profile-service:8001/profile/{userID}/pre-compute`.
- Retry: hasta 2 reintentos con backoff exponencial (`time.Sleep`).
- Si falla → `XADD dlq * ...` con payload original + error string (R15).
- Logs estructurados con `slog` (nivel INFO/ERROR).

### Ollama (puerto 11434)

- Imagen: `ollama/ollama`
- Modelo por defecto: `llama3.2` (configurable via `OLLAMA_MODEL`)
- Los servicios Go llaman a `POST http://ollama:11434/api/chat` con `net/http`.

### Paquete interno `guardrail`

Paquete Go compartido (en `internal/guardrail/`), importado por `profile-service` y `assistant-service`.

- Función `Apply(ctx context.Context, text string) (result string, blocked bool, err error)`
  - Carga `guardrail-rules.json` desde MinIO al iniciar (y refresca cada 5 min).
  - Evalúa el texto contra patrones `regexp.Regexp` compilados al inicio.
  - Si hay match → devuelve (fallbackText, true, nil) y emite log WARNING con `slog` (sin PII).
  - Si no → devuelve (text, false, nil).
- Configurable sin redeployment: cambiar `guardrail-rules.json` en MinIO, esperar refresh.

### DynamoDB Local (puerto 8888)

- Imagen oficial: `amazon/dynamodb-local`
- Acceso con `aws-sdk-go-v2` apuntando a `http://dynamodb-local:8000` con credenciales dummy.
- Tablas creadas por script Go `scripts/init-dynamo/main.go`.

### MinIO (puertos 9000 / 9001)

- Equivale a S3. Acceso con `aws-sdk-go-v2/s3` apuntando a `http://minio:9000`.
- Bucket `credit-profile-assets`. Seed data subida por `scripts/init-minio/main.go`.
- Consola web en `:9001`.

### Redis (puerto 6379)

- Streams: `credit-decisions` (ingesta), `dlq` (dead-letter).
- Historial de sesión: hashes `session:{sessionID}`, TTL 1 hora.

### Observabilidad local

| Componente | Puerto | Rol |
|---|---|---|
| Prometheus | 9090 | Scraping de `/metrics` de cada servicio Go |
| Grafana | 3000 | Dashboards KPIs (R19) |
| Loki | 3100 | Logs JSON de todos los contenedores |
| Promtail | — | Recolector de logs Docker |
| Jaeger | 16686 | UI de trazas OTLP |

Cada servicio Go registra métricas con `prometheus/client_golang` y envía spans vía `go.opentelemetry.io/otel` + OTLP exporter a Jaeger.

Métricas personalizadas (R13, R18, R19):
- `profile_views_total` — Counter
- `assistant_sessions_total` — Counter
- `support_deflection_total` — Counter
- `support_escalation_total` — Counter
- `dispute_flows_initiated_total` — Counter
- `llm_request_duration_seconds` — Histogram (equivale a latencia Bedrock)
- `retry_date_shown_total` — Counter

---

## Estructura de archivos del proyecto

```
perfil-crediticio/
├── docker-compose.yml
├── .env.example
├── .env                            # NO en git
├── Makefile
├── go.work                         # Go workspace (multi-módulo)
├── data/
│   └── seed/
│       ├── narratives/
│       │   ├── state-mapping.json
│       │   ├── narrative-1.json
│       │   ├── narrative-2.json
│       │   ├── narrative-3.json
│       │   └── narrative-4.json
│       ├── actionables-context/
│       │   └── resolution-windows.json
│       ├── dispute-guides/
│       │   ├── bcra.json
│       │   └── bank.json
│       ├── reference-data/
│       │   ├── bcra-situations.json
│       │   └── glossary.json
│       └── guardrail-rules.json
├── internal/                       # paquetes compartidos
│   ├── guardrail/
│   │   └── guardrail.go
│   ├── dynamo/
│   │   └── client.go
│   ├── minio/
│   │   └── client.go
│   ├── ollama/
│   │   └── client.go
│   └── auth/
│       └── jwt.go                  # middleware JWT chi
├── scripts/
│   ├── init-dynamo/
│   │   └── main.go
│   ├── init-minio/
│   │   └── main.go
│   └── seed-dynamo/
│       └── main.go
├── services/
│   ├── api-gateway/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── main.go
│   ├── profile-service/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── main.go
│   ├── assistant-service/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── main.go
│   ├── dispute-service/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── main.go
│   ├── support-mock/
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── main.go
│   └── event-consumer/
│       ├── Dockerfile
│       ├── go.mod
│       └── main.go
├── observability/
│   ├── prometheus.yml
│   ├── loki-config.yml
│   ├── promtail-config.yml
│   └── grafana/
│       └── dashboards/
│           └── credit-profile.json
└── tests/
    └── integration/
        ├── go.mod
        └── flows_test.go
```

**Nota Go workspace**: `go.work` une todos los módulos para que `internal/` sea importable por cada servicio durante desarrollo sin publicar nada.

---

## Dockerfiles (patrón multi-stage)

Todos los servicios usan el mismo patrón:

```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /service ./...

# Runtime stage — imagen mínima
FROM scratch
COPY --from=builder /service /service
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/service"]
```

Imagen final: ~10–15 MB. Sin shell, sin OS overhead.

---

## Variables de entorno (`.env`)

```
# Auth
JWT_SECRET=dev-secret-change-in-prod

# Ollama
OLLAMA_MODEL=llama3.2
OLLAMA_URL=http://ollama:11434

# DynamoDB Local
AWS_ACCESS_KEY_ID=local
AWS_SECRET_ACCESS_KEY=local
AWS_DEFAULT_REGION=us-east-1
DYNAMODB_ENDPOINT=http://dynamodb-local:8000

# MinIO (S3)
MINIO_ENDPOINT=http://minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=credit-profile-assets

# Redis
REDIS_URL=redis://redis:6379

# Guardrails
BLOCKED_STATES=1P_rejected,Renov_blocked

# Observabilidad
OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
```

---

## Data flow diagrams (local)

### Lectura de perfil
```
Cliente → api-gateway:8000 (JWT check)
  → profile-service:8001
    → DynamoDB Local (cache check)
      HIT  → respuesta inmediata
      MISS → MinIO (narrativa) → Ollama (llama3.2) → guardrail.Apply()
           → DynamoDB (guardar cache) → respuesta JSON
```

### Asistente conversacional
```
Cliente → api-gateway:8000
  → assistant-service:8002
    → Redis (historial sesión)
    → scope check
      IN SCOPE  → DynamoDB → Ollama → guardrail.Apply() → SSE streaming
      OUT SCOPE → support-mock:8004 (POST /tickets) → mensaje de derivación
```

### Ingesta asíncrona
```
XADD credit-decisions * user_id xxx decision_state approved ...
  → event-consumer (XREAD bloqueante)
    → POST profile-service:8001/profile/{userID}/pre-compute
```

### Reclamo
```
Cliente → api-gateway:8000
  → dispute-service:8003
    → MinIO (guía) → DynamoDB (audit log) → respuesta JSON
```

---

## Notas de equivalencia para requisitos

| R-ID | Ajuste local |
|---|---|
| R1 | Ollama reemplaza Bedrock. Cache en DynamoDB Local. |
| R4 | Redis Streams reemplaza EventBridge. `event-consumer` Go reemplaza trigger Lambda. |
| R6 | SSE con `http.Flusher` en Go reemplaza AgentCore streaming. |
| R7 | `support-mock` Go reemplaza Zendesk. |
| R10 | Narrativas en MinIO. Hot-reload sin redeployment. |
| R12 | Paquete `internal/guardrail` reemplaza Bedrock Guardrails. Reglas en MinIO. |
| R13 | Counter Prometheus reemplaza CloudWatch custom metric. |
| R15 | Redis Streams + `event-consumer` Go reemplazan EventBridge + SQS DLQ. |
| R17 | Env var `BLOCKED_STATES` parseado como slice en config struct. |
| R18 | Histogram Prometheus `llm_request_duration_seconds` + dashboard Grafana. |
| R19 | Dashboard Grafana con los 5 KPIs reemplaza CloudWatch dashboard. |
