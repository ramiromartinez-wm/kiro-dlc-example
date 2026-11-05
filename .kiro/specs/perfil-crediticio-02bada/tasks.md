# Implementation Tasks

Stack local con Docker Compose. Orden de construcción: infraestructura base → datos/seed → servicios → observabilidad → integración → tests.

---

## Fase 1 — Infraestructura base (Docker Compose + servicios de plataforma)

### Tarea 1.1 — Estructura del proyecto
- [ ] Crear directorio raíz `perfil-crediticio/` con la estructura definida en `design.md`
- [ ] Crear `.env.example` con todas las variables documentadas en design.md
- [ ] Crear `.gitignore` (excluir `.env`, `__pycache__`, `*.pyc`, volúmenes de Docker)

### Tarea 1.2 — `docker-compose.yml` base
- [ ] Definir red Docker `creditprofile-net` (bridge)
- [ ] Servicio `dynamodb-local`: imagen `amazon/dynamodb-local`, puerto `8888:8000`, volumen para persistencia
- [ ] Servicio `minio`: imagen `minio/minio`, puertos `9000:9000` y `9001:9001`, volumen, healthcheck
- [ ] Servicio `redis`: imagen `redis:7-alpine`, puerto `6379:6379`, healthcheck
- [ ] Servicio `ollama`: imagen `ollama/ollama`, puerto `11434:11434`, volumen para modelos
- [ ] Agregar `depends_on` con condición `service_healthy` entre servicios dependientes

### Tarea 1.3 — Observabilidad: Prometheus + Grafana + Loki + Jaeger
- [ ] Servicio `prometheus`: imagen `prom/prometheus`, montar `observability/prometheus.yml`, puerto `9090:9090`
- [ ] Servicio `grafana`: imagen `grafana/grafana`, puerto `3000:3000`, provisionar datasources (Prometheus, Loki, Jaeger) y dashboards vía volumen
- [ ] Servicio `loki`: imagen `grafana/loki`, montar `observability/loki-config.yml`, puerto `3100:3100`
- [ ] Servicio `promtail`: imagen `grafana/promtail`, montar socket Docker y `observability/promtail-config.yml`
- [ ] Servicio `jaeger`: imagen `jaegertracing/all-in-one`, puertos `16686:16686` (UI) y `4317:4317` (OTLP gRPC)
- [ ] Escribir `observability/prometheus.yml` con scrape configs para los 5 servicios FastAPI
- [ ] Escribir `observability/loki-config.yml` básico
- [ ] Escribir `observability/promtail-config.yml` para recolectar logs de contenedores Docker

---

## Fase 2 — Datos y seed

### Tarea 2.1 — Script `scripts/init-dynamo.py`
- [ ] Crear tabla `user_credit_profile` (PK: `user_id`; TTL en atributo `ttl`)
- [ ] Crear tabla `credit_decisions_history` (PK: `user_id`; SK: `decision_timestamp`)
- [ ] Crear tabla `dispute_audit_log` (PK: `user_id`; SK: `timestamp`)
- [ ] Usar `boto3` con `endpoint_url=DYNAMODB_ENDPOINT`, credenciales dummy del `.env`
- [ ] Ejecutar como servicio efímero `dynamodb-init` en docker-compose (depende de `dynamodb-local` healthy)

### Tarea 2.2 — Seed data JSON (`data/seed/`)
- [ ] `narratives/state-mapping.json`: mapeo de 8 estados del motor → 4 narrative keys (R10)
- [ ] `narratives/narrative-1.json`: "Tenés línea y podés hacerla crecer" (R10)
- [ ] `narratives/narrative-2.json`: "Podés destrabar tu línea" (R10)
- [ ] `narratives/narrative-3.json`: narrativa de rechazo tipo A (R10)
- [ ] `narratives/narrative-4.json`: narrativa de rechazo tipo B (R10)
- [ ] `actionables-context/resolution-windows.json`: ventanas de resolución por tipo de accionable (R9)
- [ ] `dispute-guides/bcra.json`: pasos + links + contacto BCRA (R8)
- [ ] `dispute-guides/bank.json`: pasos + links bancarios (R8)
- [ ] `reference-data/bcra-situations.json`: definición de situaciones 1-6 del BCRA (R6)
- [ ] `reference-data/glossary.json`: glosario de términos crediticios (R6)
- [ ] `guardrail-rules.json`: lista de patrones regex prohibidos (R12)

### Tarea 2.3 — Script `scripts/init-minio.py`
- [ ] Crear bucket `credit-profile-assets`
- [ ] Subir todos los archivos de `data/seed/` al bucket con la estructura de prefijos correcta
- [ ] Usar `boto3` con `endpoint_url=MINIO_ENDPOINT`
- [ ] Ejecutar como servicio efímero `minio-init` en docker-compose (depende de `minio` healthy)

### Tarea 2.4 — Script `scripts/seed-dynamo.py`
- [ ] Insertar 3 usuarios de prueba en `user_credit_profile`:
  - usuario aprobado (con accionables directos e indirectos)
  - usuario rechazado standard
  - usuario `1P_rejected` (para probar bloqueo R17)
- [ ] Insertar historial de decisiones en `credit_decisions_history`
- [ ] Ejecutar como servicio efímero `dynamo-seed` (depende de `dynamodb-init`)

### Tarea 2.5 — Descarga del modelo Ollama
- [ ] Servicio efímero `ollama-pull` en docker-compose: `ollama pull ${OLLAMA_MODEL}` (depende de `ollama` healthy)

---

## Fase 3 — Código compartido (`services/shared/`)

### Tarea 3.1 — `auth.py`
- [ ] Clase `JWTMiddleware` para Starlette
- [ ] Verificar token Bearer con `python-jose` usando `JWT_SECRET`
- [ ] Extraer `user_id` del claim `sub`; comparar con path param `user_id`
- [ ] Retornar HTTP 401 si token inválido, HTTP 403 si `user_id` no coincide (R4, R6, R8)

### Tarea 3.2 — `dynamo_client.py`
- [ ] Wrapper de `boto3` configurado con `DYNAMODB_ENDPOINT` y credenciales del `.env`
- [ ] Funciones: `get_user_profile`, `put_user_profile`, `get_decisions_history`, `log_dispute`
- [ ] Manejo de `ClientError` con logs estructurados

### Tarea 3.3 — `minio_client.py`
- [ ] Wrapper de `boto3` S3 con `endpoint_url=MINIO_ENDPOINT`
- [ ] Funciones: `get_object_json(prefix, key)`, `list_objects(prefix)`
- [ ] Caché en memoria con TTL corto (30 s) para narrativas (evitar llamadas repetidas a MinIO)

### Tarea 3.4 — `ollama_client.py`
- [ ] Función `chat(prompt, stream=False)` → llama a `POST {OLLAMA_URL}/api/chat`
- [ ] Función `chat_stream(prompt)` → generador que hace streaming de la respuesta
- [ ] Timeout configurable; en TimeoutError → raise `OllamaUnavailableError`
- [ ] Log de duración de cada invocación (para métrica Prometheus) (R18)

### Tarea 3.5 — `guardrail.py`
- [ ] Función `apply(text: str) -> tuple[str, bool]`
  - Carga `guardrail-rules.json` de MinIO (cacheado en memoria)
  - Evalúa texto contra patrones regex prohibidos
  - Si hay match → retorna (fallback_text, True); si no → retorna (text, False)
- [ ] Función `load_fallback(narrative_key: str) -> str` → lee narrativa de MinIO
- [ ] Log de bloqueo a stdout (nivel WARNING, sin PII) (R12)

---

## Fase 4 — Servicios FastAPI

### Tarea 4.1 — `api-gateway/main.py`
- [ ] App FastAPI con `JWTMiddleware` aplicado globalmente
- [ ] Rutas proxy:
  - `GET /user/{user_id}/credit-profile` → `httpx` async a `profile-service:8001`
  - `POST /user/{user_id}/ask` → forward con streaming a `assistant-service:8002`
  - `POST /user/{user_id}/dispute` → `httpx` async a `dispute-service:8003`
- [ ] `GET /health` sin auth (para healthcheck de Docker)
- [ ] Instrumentación OpenTelemetry + Prometheus (`prometheus-fastapi-instrumentator`)
- [ ] `Dockerfile`: imagen `python:3.12-slim`, instalar deps, `CMD uvicorn main:app --host 0.0.0.0 --port 8000`

### Tarea 4.2 — `profile-service/main.py`
- [ ] `GET /profile/{user_id}` — flujo completo según design.md:
  1. Leer `user_credit_profile` de DynamoDB
  2. Chequear `BLOCKED_STATES` → 403 (R17)
  3. Chequear `cached_explanation` + TTL → devolver si válido (R1)
  4. Leer `state-mapping.json` de MinIO → seleccionar narrativa (R10)
  5. Llamar a `CalculateRetryDate` si hay accionables directos (R9)
  6. Construir prompt y llamar `ollama_client.chat()` (R1)
  7. Aplicar `guardrail.apply()` (R12)
  8. Guardar en DynamoDB
  9. Emitir métricas Prometheus: `profile_views_total`, `llm_request_duration_seconds`
  10. Devolver respuesta estructurada
- [ ] `POST /profile/{user_id}/pre-compute` — mismo flujo pero sin devolver body HTTP (para event-consumer)
- [ ] Manejo de campos faltantes → respuesta parcial, no 500 (R5)
- [ ] `bcra_cutoff_date` nulo → nota genérica (R16)
- [ ] Fallback Ollama → narrativa de MinIO (R10)
- [ ] `Dockerfile` + `requirements.txt`

### Tarea 4.3 — `assistant-service/main.py`
- [ ] `POST /ask/{user_id}` — flujo según design.md:
  1. Cargar historial de sesión de Redis
  2. Evaluar scope (keywords de crédito/perfil/BCRA)
  3. **In-scope**: DynamoDB → prompt con contexto → `ollama_client.chat_stream()` → guardrail → `StreamingResponse`
  4. **Out-of-scope**: `httpx.post("http://support-mock:8004/tickets", ...)` → respuesta de derivación
  5. Guardar turno en Redis (HSET con TTL 1h)
  6. Emitir métricas: `assistant_sessions_total`, `support_deflection_total` / `support_escalation_total`
- [ ] `Dockerfile` + `requirements.txt`

### Tarea 4.4 — `dispute-service/main.py`
- [ ] `POST /dispute/{user_id}`:
  1. Parsear `{ "disputed_field": "...", "dispute_type": "bcra|bank" }`
  2. Leer guía de MinIO (`dispute-guides/{dispute_type}.json`)
  3. Escribir audit log en DynamoDB (sin PII extra) (R8)
  4. Emitir métrica `dispute_flows_initiated_total`
  5. Devolver guía
- [ ] `Dockerfile` + `requirements.txt`

### Tarea 4.5 — `support-mock/main.py`
- [ ] `POST /tickets`: crear ticket en lista en memoria → retornar `{ "ticket_id": uuid, "eta": "24h" }` (R7)
- [ ] `GET /tickets/{id}`: retornar estado del ticket
- [ ] `GET /health`
- [ ] `Dockerfile` + `requirements.txt`

### Tarea 4.6 — `event-consumer/consumer.py`
- [ ] Conexión a Redis, `XREAD` bloqueante en stream `credit-decisions`
- [ ] Por cada mensaje: `POST http://profile-service:8001/profile/{user_id}/pre-compute`
- [ ] Retry: hasta 2 reintentos con backoff exponencial
- [ ] Si falla → `XADD dlq` con payload + error
- [ ] Log estructurado de cada evento procesado
- [ ] `Dockerfile` + `requirements.txt`

---

## Fase 5 — Agregar todos los servicios al docker-compose.yml

- [ ] Servicio `api-gateway`: build `./services/api-gateway`, puerto `8000:8000`, env del `.env`
- [ ] Servicio `profile-service`: build `./services/profile-service`, puerto `8001:8001`
- [ ] Servicio `assistant-service`: build `./services/assistant-service`, puerto `8002:8002`
- [ ] Servicio `dispute-service`: build `./services/dispute-service`, puerto `8003:8003`
- [ ] Servicio `support-mock`: build `./services/support-mock`, puerto `8004:8004`
- [ ] Servicio `event-consumer`: build `./services/event-consumer` (sin puerto expuesto)
- [ ] Todos los servicios: `depends_on` infraestructura (`dynamodb-local`, `minio`, `redis`, `ollama`), red `creditprofile-net`, env file `.env`
- [ ] Healthchecks en todos los servicios FastAPI (`GET /health`)

---

## Fase 6 — Observabilidad

### Tarea 6.1 — Métricas Prometheus
- [ ] Agregar `prometheus-fastapi-instrumentator` a cada servicio FastAPI
- [ ] Definir contadores y histogramas personalizados en cada servicio (ver lista en design.md)
- [ ] Verificar que `GET /metrics` responde en cada servicio

### Tarea 6.2 — Dashboard Grafana
- [ ] Crear `observability/grafana/dashboards/credit-profile.json` con panels:
  - `profile_views_total` (Counter → rate) (R19)
  - `assistant_sessions_total` (R19)
  - `support_deflection_total` vs `support_escalation_total` → tasa de deflexión (R13)
  - `dispute_flows_initiated_total` (R19)
  - `retry_date_shown_total` (R19)
  - `llm_request_duration_seconds` p50/p95/p99 (R18)
  - Error rate por servicio
- [ ] Provisionar dashboard automáticamente en Grafana al iniciar

### Tarea 6.3 — Trazas Jaeger
- [ ] Inicializar `TracerProvider` con `OTLPSpanExporter` en cada servicio FastAPI
- [ ] Instrumentar con `FastAPIInstrumentor` y `Boto3Instrumentor`
- [ ] Verificar trazas end-to-end en Jaeger UI `http://localhost:16686`

### Tarea 6.4 — Logs Loki
- [ ] Configurar Promtail para recolectar logs de contenedores Docker por label
- [ ] Verificar exploración de logs en Grafana → Loki

---

## Fase 7 — Script de inicio rápido

### Tarea 7.1 — `Makefile`
- [ ] `make up`: `docker compose up -d --build`
- [ ] `make init`: ejecutar `dynamodb-init`, `minio-init`, `dynamo-seed`, `ollama-pull` en secuencia
- [ ] `make down`: `docker compose down`
- [ ] `make logs`: `docker compose logs -f`
- [ ] `make test`: ejecutar tests de integración
- [ ] `make token`: generar JWT de prueba para un `user_id` dado (script Python inline)

### Tarea 7.2 — `README.md`
- [ ] Requisitos previos: Docker Desktop ≥ 4.x, 8 GB RAM mínimo (por Ollama), `make`
- [ ] Instrucciones de inicio: `cp .env.example .env && make up && make init`
- [ ] Tabla de URLs: api-gateway, MinIO console, Grafana, Jaeger, Prometheus
- [ ] Ejemplos de curl para los 4 flujos (con JWT de prueba)
- [ ] Nota: cómo cambiar el modelo Ollama (`OLLAMA_MODEL=mistral make init`)

---

## Fase 8 — Tests de integración

### Tarea 8.1 — `tests/integration/test_flows.py`
- [ ] Setup: generar JWT de prueba; verificar que todos los servicios respondan `/health`
- [ ] Test R1/R5: `GET /user/{id}/credit-profile` devuelve `line_status`, `profile_level`, `primary_factors`, `bcra_cutoff_date`, `explanation`
- [ ] Test R17: usuario `1P_rejected` recibe HTTP 403
- [ ] Test R1 cache: segunda llamada al mismo usuario es más rápida (hit en DynamoDB)
- [ ] Test R6: `POST /user/{id}/ask` con pregunta in-scope devuelve respuesta con contenido
- [ ] Test R7: `POST /user/{id}/ask` con pregunta out-of-scope devuelve ticket ID y mensaje de derivación; verificar ticket en `support-mock`
- [ ] Test R8: `POST /user/{id}/dispute` devuelve guía con steps; verificar audit log en DynamoDB
- [ ] Test R15: publicar evento en Redis Stream `credit-decisions`; verificar que `cached_explanation` aparece en DynamoDB tras unos segundos
- [ ] Test R12: inyectar prompt que debería disparar guardrail; verificar que la respuesta es el fallback
- [ ] Test R9: usuario con accionable directo tiene `suggested_retry_date` en respuesta
- [ ] Test R10: estado desconocido devuelve narrativa de fallback (no 500)
- [ ] Test R14: evento sin `rejection_variable` → explicación genérica en DynamoDB
