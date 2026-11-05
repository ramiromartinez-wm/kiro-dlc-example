FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o init-minio main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary
COPY --from=builder /build/init-minio .

# Copy seed data
COPY ../../data/seed /app/data/seed

ENTRYPOINT ["/app/init-minio"]
