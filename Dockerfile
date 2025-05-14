# Dockerfile multistage para aplicação Go
# Stage 1: Build
FROM golang:1.21-alpine AS builder

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

# Copiar arquivos de configuração
COPY --chown=appuser:appuser .env.example .env

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