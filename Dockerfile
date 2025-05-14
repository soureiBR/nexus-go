# Dockerfile multistage para aplicação Go
# Stage 1: Build
FROM golang:1.23.9-alpine AS builder

# Instalar dependências de build
RUN apk add --no-cache gcc musl-dev

# Configurar diretório de trabalho
WORKDIR /app

# Copiar módulos Go
COPY go.mod go.sum ./

# Baixar dependências
RUN go mod download

# Copiar código-fonte
COPY . .

# Compilar a aplicação
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags="-w -s" -o whatsapp-api ./cmd/api

# Stage 2: Runtime
FROM alpine:3.18

# Adicionar suporte a fuso horário e CA certs
RUN apk add --no-cache tzdata ca-certificates && \
    update-ca-certificates

# Criar usuário não-root
RUN adduser -D -u 1000 appuser

# Criar diretórios necessários
RUN mkdir -p /app/sessions && \
    chown -R appuser:appuser /app

# Configurar diretório de trabalho
WORKDIR /app

# Copiar binário compilado
COPY --from=builder --chown=appuser:appuser /app/whatsapp-api .

# Criar arquivo .env padrão no contêiner
RUN echo "PORT=8080\n\
HOST=0.0.0.0\n\
ENVIRONMENT=production\n\
API_KEY=your-secure-api-key\n\
LOG_LEVEL=info\n\
LOG_FORMAT=json\n\
SESSION_DIR=/app/sessions\n\
TEMP_DIR=/tmp\n\
WEBHOOK_URL=\n\
WEBHOOK_SECRET=\n\
CLEANUP_INTERVAL=24h\n\
MAX_INACTIVE_TIME=72h\n\
REQUEST_TIMEOUT=30s\n\
WEBHOOK_TIMEOUT=10s\n\
MAX_UPLOAD_SIZE=10MB" > /app/.env && \
    chown appuser:appuser /app/.env

# Definir usuário não-root
USER appuser

# Expor porta da aplicação
EXPOSE 8080

# Definir variáveis de ambiente padrão
ENV GIN_MODE=release \
    PORT=8080 \
    SESSION_DIR=/app/sessions \
    LOG_LEVEL=info

# Definir ponto de entrada
ENTRYPOINT ["/app/whatsapp-api"]

# Definir health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1