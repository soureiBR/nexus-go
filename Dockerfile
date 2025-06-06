# Dockerfile multistage para aplicação Go
# Stage 1: Build
FROM golang:1.24.3-alpine AS builder

# Instalar dependências de build necessárias
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Configurar diretório de trabalho
WORKDIR /app

# Copiar módulos Go primeiro (para aproveitar cache do Docker)
COPY go.mod go.sum ./

# Baixar dependências
RUN go mod download && go mod verify

# Copiar código-fonte
COPY . .

# Compilar a aplicação com flags otimizadas
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags="-w -s -extldflags '-static'" \
    -o whatsapp-api ./cmd/api

# Verificar se o binário foi criado
RUN ls -la /app/whatsapp-api

# Stage 2: Runtime
FROM alpine:3.19

# Instalar dependências de runtime
RUN apk add --no-cache \
    tzdata \
    ca-certificates \
    sqlite \
    wget \
    && update-ca-certificates

# Criar usuário não-root
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# Criar diretórios necessários com permissões corretas
RUN mkdir -p /app/sessions /app/data /app/logs && \
    chown -R appuser:appuser /app

# Configurar diretório de trabalho
WORKDIR /app

# Copiar binário compilado com permissões corretas
COPY --from=builder --chown=appuser:appuser /app/whatsapp-api ./whatsapp-api

# Tornar o binário executável
RUN chmod +x /app/whatsapp-api

# Criar arquivo .env padrão (opcional, pois usaremos env vars)
RUN touch /app/.env && chown appuser:appuser /app/.env

# Definir usuário não-root
USER appuser

# Expor porta da aplicação
EXPOSE 8080

# Definir variáveis de ambiente padrão
ENV GIN_MODE=release \
    PORT=8080 \
    LOG_LEVEL=info \
    DB_PATH=/app/data/whatsapp.db

# Definir ponto de entrada
ENTRYPOINT ["./whatsapp-api"]

# Definir health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1