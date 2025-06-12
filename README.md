# WhatsApp API

Uma API REST em Go para gerenciar múltiplas sessões do WhatsApp usando a biblioteca whatsmeow.

## Características

- API REST para gerenciar sessões do WhatsApp
- Armazenamento de sessões em arquivos (sem necessidade de banco de dados)
- Suporte para múltiplas sessões simultâneas
- Gerenciamento eficiente de recursos e concorrência
- Envio de mensagens (texto, mídia, botões, listas)
- Webhook para notificação de eventos
- Autenticação via API Key
- Docker ready

## Requisitos

- Go 1.18+
- Ou Docker e Docker Compose

## Instalação

### Usando Go

1. Clone o repositório
```bash
git clone https://github.com/yourusername/whatsapp-api.git
cd whatsapp-api
```

2. Instale as dependências
```bash
go mod download
```

3. Configure o ambiente
```bash
cp .env.example .env
# Edite o arquivo .env com suas configurações
```

4. Execute a aplicação
```bash
CGO_ENABLED=1 go run cmd/api/main.go
```

### Usando Docker

1. Clone o repositório
```bash
git clone https://github.com/yourusername/whatsapp-api.git
cd whatsapp-api
```

2. Configure o ambiente
```bash
cp .env.example .env
# Edite o arquivo .env com suas configurações
```

3. Execute com Docker Compose
```bash
docker-compose up -d
```

## Configuração

| Variável | Descrição | Padrão |
|----------|-----------|--------|
| PORT | Porta do servidor HTTP | 8080 |
| HOST | Host do servidor HTTP | 0.0.0.0 |
| ENVIRONMENT | Ambiente (development/production) | production |
| API_KEY | Chave de API para autenticação | - |
| SESSION_DIR | Diretório para armazenar sessões | ./sessions |
| WEBHOOK_URL | URL para envio de eventos | - |
| WEBHOOK_SECRET | Segredo para assinatura de webhooks | - |
| LOG_LEVEL | Nível de log (debug/info/warn/error) | info |
| LOG_FORMAT | Formato de log (json/text) | json |
| CLEANUP_INTERVAL | Intervalo para limpeza de sessões | 24h |
| MAX_INACTIVE_TIME | Tempo máximo de inatividade de sessões | 72h |
| REQUEST_TIMEOUT | Timeout para requisições HTTP | 30s |
| WEBHOOK_TIMEOUT | Timeout para envio de webhooks | 10s |
| MAX_UPLOAD_SIZE | Tamanho máximo de upload | 10MB |
| TEMP_DIR | Diretório temporário | /tmp |

## Endpoints da API

### Sessões

- **POST /api/v1/session/create** - Criar nova sessão
  ```json
  {
    "user_id": "user123"
  }
  ```

- **GET /api/v1/session/:id** - Obter informações da sessão

- **GET /api/v1/session/:id/qr** - Obter QR code para autenticação

- **POST /api/v1/session/:id/connect** - Conectar sessão existente

- **DELETE /api/v1/session/:id** - Encerrar e remover sessão

### Mensagens

- **POST /api/v1/message/text** - Enviar mensagem de texto
  ```json
  {
    "user_id": "user123",
    "to": "5511999999999",
    "message": "Olá, mundo!"
  }
  ```

- **POST /api/v1/message/media** - Enviar mídia (imagem, vídeo, etc.)
  - Form data:
    - user_id: ID da sessão
    - to: Número do destinatário
    - caption: Legenda da mídia (opcional)
    - media_type: Tipo de mídia (image, video, audio, document)
    - file: Arquivo a ser enviado

- **POST /api/v1/message/buttons** - Enviar mensagem com botões
  ```json
  {
    "user_id": "user123",
    "to": "5511999999999",
    "text": "Escolha uma opção:",
    "footer": "Rodapé da mensagem",
    "buttons": [
      {"id": "btn1", "text": "Opção 1"},
      {"id": "btn2", "text": "Opção 2"},
      {"id": "btn3", "text": "Opção 3"}
    ]
  }
  ```

- **POST /api/v1/message/list** - Enviar mensagem com lista
  ```json
  {
    "user_id": "user123",
    "to": "5511999999999",
    "text": "Escolha uma opção:",
    "footer": "Rodapé da mensagem",
    "button_text": "Ver opções",
    "sections": [
      {
        "title": "Seção 1",
        "rows": [
          {"id": "row1", "title": "Item 1", "description": "Descrição do item 1"},
          {"id": "row2", "title": "Item 2", "description": "Descrição do item 2"}
        ]
      },
      {
        "title": "Seção 2",
        "rows": [
          {"id": "row3", "title": "Item 3", "description": "Descrição do item 3"}
        ]
      }
    ]
  }
  ```

### Webhook

- **POST /api/v1/webhook/configure** - Configurar URL de webhook
  ```json
  {
    "url": "https://your-webhook-url.com/api/webhook",
    "enabled_events": ["message", "connected", "disconnected", "qr", "logged_out"],
    "secret": "your-webhook-secret"
  }
  ```

- **GET /api/v1/webhook/status** - Verificar status do webhook

- **POST /api/v1/webhook/test** - Testar webhook

## Eventos de Webhook

Os seguintes eventos podem ser enviados para o webhook configurado:

- **message** - Recebimento de mensagens
- **connected** - Conexão estabelecida
- **disconnected** - Conexão encerrada
- **qr** - Código QR gerado
- **logged_out** - Sessão encerrada pelo WhatsApp

## Autenticação

Todas as requisições à API devem incluir o cabeçalho `Authorization` com o token de API:

```
Authorization: Bearer your-api-key
```

## Licença

MIT