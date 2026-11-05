# DynamoDB Initialization Script

Este script inicializa las tablas necesarias en DynamoDB Local para el sistema de perfil crediticio.

## Tablas Creadas

1. **user_credit_profile**
   - PK: `user_id` (String)
   - TTL: `ttl` (Number) - habilitado automáticamente

2. **credit_decisions_history**
   - PK: `user_id` (String)
   - SK: `decision_timestamp` (String)

3. **dispute_audit_log**
   - PK: `user_id` (String)
   - SK: `timestamp` (String)

## Uso

### Ejecución directa con go run

```bash
cd scripts/init-dynamo
GOWORK=off go run main.go
```

### Con endpoint personalizado

```bash
export DYNAMODB_ENDPOINT=http://localhost:8000
GOWORK=off go run main.go
```

### Build y ejecución

```bash
cd scripts/init-dynamo
GOWORK=off go build -o init-dynamo main.go
./init-dynamo
```

## Características

- ✅ Usa `aws-sdk-go-v2/service/dynamodb`
- ✅ Endpoint configurable vía variable de entorno `DYNAMODB_ENDPOINT`
- ✅ Default endpoint: `http://dynamodb-local:8000`
- ✅ Manejo de error `ResourceInUseException` (tabla ya existe → skip silencioso)
- ✅ Logs estructurados con `log/slog` en formato JSON
- ✅ Módulo Go independiente

## Variables de Entorno

| Variable | Default | Descripción |
|----------|---------|-------------|
| `DYNAMODB_ENDPOINT` | `http://dynamodb-local:8000` | Endpoint de DynamoDB Local |

## Notas

- El script es idempotente: si las tablas ya existen, las omite silenciosamente
- TTL se configura en dos pasos: primero se crea la tabla, luego se habilita TTL con `UpdateTimeToLive`
- Todas las tablas usan `BillingMode: PayPerRequest` (sin throughput provisionado)
