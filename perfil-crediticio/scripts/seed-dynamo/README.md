# Seed DynamoDB Script

Este script inserta datos de prueba en DynamoDB para desarrollo y testing del sistema de perfil crediticio.

## Datos de Prueba

### Tabla `user_credit_profile`

El script inserta 3 usuarios con perfiles diferentes:

#### 1. **user_001** (Usuario Aprobado)
- `decision_state`: "approved"
- `line_status`: "active"
- `profile_level`: "bueno"
- `bcra_cutoff_date`: "2024-10-01"
- **Factores primarios**:
  - Historial crediticio estable y consistente
  - Sin atrasos significativos en los últimos 12 meses
  - Baja utilización de líneas de crédito existentes
  - Antigüedad crediticia mayor a 2 años
- **Actionables**: 3 items (2 directos, 1 indirecto) con períodos de resolución
- **TTL**: 24 horas desde la inserción

#### 2. **user_002** (Usuario Rechazado)
- `decision_state`: "rejected"
- `line_status`: "rejected"
- `profile_level`: "crítico"
- `bcra_cutoff_date`: "2024-09-15"
- **Factores primarios**:
  - Situación 3 reportada en BCRA
  - Morosidad mayor a 90 días en múltiples obligaciones
  - Consultas crediticias excesivas en el último mes (más de 10)
- **Actionables**: 3 items indirectos para regularización
- **TTL**: 24 horas desde la inserción

#### 3. **user_003** (Usuario Bloqueado)
- `decision_state`: "1P_rejected"
- `line_status`: "blocked"
- `profile_level`: "bloqueado"
- **Factores primarios**:
  - Bloqueado por política interna de la empresa
- **Actionables**: Sin actionables
- **TTL**: 24 horas desde la inserción

### Tabla `credit_decisions_history`

El script inserta 3 registros históricos:

1. **user_001** - Hace 30 días
   - Estado: approved/active/bueno
   - 2 factores positivos

2. **user_001** - Hace 60 días
   - Estado: approved/active/muy bueno
   - 2 factores positivos

3. **user_002** - Hace 15 días
   - Estado: rejected/rejected/crítico
   - 3 factores bloqueantes

## Uso

### Variables de Entorno

- `DYNAMODB_ENDPOINT`: Endpoint de DynamoDB (default: `http://dynamodb-local:8000`)

### Ejecución

```bash
# Configurar endpoint (opcional)
export DYNAMODB_ENDPOINT=http://localhost:8000

# Ejecutar el script
go run main.go

# O compilar y ejecutar
go build -o seed-dynamo main.go
./seed-dynamo
```

### Con Docker Compose

```bash
# Desde el directorio raíz del proyecto
docker-compose exec seed-dynamo go run /app/main.go
```

## Estructura de Datos

### Actionable

```go
type Actionable struct {
    Type            string // "directo" o "indirecto"
    Description     string // Descripción de la acción
    ResolutionDays  int    // Días estimados para resolución
}
```

### UserCreditProfile

```go
type UserCreditProfile struct {
    UserID          string       // ID del usuario
    DecisionState   string       // approved, rejected, 1P_rejected
    LineStatus      string       // active, rejected, blocked
    ProfileLevel    string       // bueno, crítico, bloqueado
    BCRACutoffDate  string       // Fecha de corte BCRA (opcional)
    PrimaryFactors  []string     // Factores principales de la decisión
    Actionables     []Actionable // Items accionables
    TTL             int64        // Timestamp Unix para expiración
}
```

### CreditDecisionHistory

```go
type CreditDecisionHistory struct {
    UserID            string   // ID del usuario
    DecisionTimestamp string   // Timestamp ISO8601
    DecisionState     string   // Estado de la decisión
    LineStatus        string   // Estado de la línea
    ProfileLevel      string   // Nivel del perfil
    PrimaryFactors    []string // Factores de la decisión
}
```

## Dependencias

- `github.com/aws/aws-sdk-go-v2`
- `github.com/aws/aws-sdk-go-v2/config`
- `github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue`
- `github.com/aws/aws-sdk-go-v2/service/dynamodb`

## Logs

El script utiliza `log/slog` con formato JSON para los logs. Ejemplos de salida:

```json
{"time":"2024-01-15T10:30:00Z","level":"INFO","msg":"Initializing DynamoDB client","endpoint":"http://dynamodb-local:8000"}
{"time":"2024-01-15T10:30:01Z","level":"INFO","msg":"Seeding user credit profiles"}
{"time":"2024-01-15T10:30:01Z","level":"INFO","msg":"Inserted user credit profile","user_id":"user_001","decision_state":"approved"}
{"time":"2024-01-15T10:30:02Z","level":"INFO","msg":"Database seeded successfully"}
```

## Notas

- Los registros tienen TTL de 24 horas para limpieza automática
- El script es idempotente - puede ejecutarse múltiples veces
- Los timestamps históricos son relativos a la fecha de ejecución
- Los datos son ficticios y diseñados para testing
