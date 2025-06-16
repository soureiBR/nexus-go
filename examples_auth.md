# Exemplos de Uso do Sistema de Autenticação

## 1. Gerar uma chave criptografada (Admin)

**Endpoint:** `POST /api/v1/auth/session/encrypt`

**Headers:**
```
x-key: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.2PKmc4CNZgH3nGJP380pD5LszqdyMx-IXI3LeZnhJh0
Content-Type: application/json
```

**Body:**
```json
{
  "user_id": "5511999887766"
}
```

**Resposta:**
```json
{
  "user_secret": "encrypted_base64_string_here",
  "expires_at": "2025-06-17T12:00:00Z",
  "user_id": "5511999887766"
}
```

### Exemplo com cURL:
```bash
curl -X POST http://localhost:8888/api/v1/auth/session/encrypt \
  -H "x-key: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.2PKmc4CNZgH3nGJP380pD5LszqdyMx-IXI3LeZnhJh0" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "5511999887766"}'
```

---

## 2. Usar a chave para fazer requisições (Usuário)

### Método Novo (Recomendado) - Usando x-user-secret

**Headers:**
```
x-user-secret: encrypted_base64_string_here
Content-Type: application/json
```

### Exemplo: Criar sessão usando x-user-secret
```bash
curl -X POST http://localhost:8888/api/v1/session/create \
  -H "x-user-secret: encrypted_base64_string_here" \
  -H "Content-Type: application/json"
```

### Exemplo: Obter QR Code usando x-user-secret
```bash
curl -X GET http://localhost:8888/api/v1/session/qr \
  -H "x-user-secret: encrypted_base64_string_here" \
  -H "Accept: text/event-stream"
```

### Exemplo: Enviar mensagem usando x-user-secret
```bash
curl -X POST http://localhost:8888/api/v1/message/text \
  -H "x-user-secret: encrypted_base64_string_here" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "5511888777666@s.whatsapp.net",
    "message": "Olá! Esta mensagem foi enviada usando x-user-secret"
  }'
```

---

## 3. Método Antigo (Ainda funciona) - Usando API Key + X-User-ID

**Headers:**
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.2PKmc4CNZgH3nGJP380pD5LszqdyMx-IXI3LeZnhJh0
X-User-ID: 5511999887766
Content-Type: application/json
```

### Exemplo: Criar sessão usando método antigo
```bash
curl -X POST http://localhost:8888/api/v1/session/create \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.2PKmc4CNZgH3nGJP380pD5LszqdyMx-IXI3LeZnhJh0" \
  -H "X-User-ID: 5511999887766" \
  -H "Content-Type: application/json"
```

---

## 4. Fluxo Completo de Exemplo

### Passo 1: Admin gera chave para usuário
```bash
# Admin gera chave criptografada para o usuário
RESPONSE=$(curl -s -X POST http://localhost:8888/api/v1/auth/session/encrypt \
  -H "x-key: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.2PKmc4CNZgH3nGJP380pD5LszqdyMx-IXI3LeZnhJh0" \
  -H "Content-Type: application/json" \
  -d '{"user_id": "5511999887766"}')

# Extrair o user_secret da resposta
USER_SECRET=$(echo $RESPONSE | jq -r '.user_secret')
echo "User Secret: $USER_SECRET"
```

### Passo 2: Usuário usa a chave para suas operações
```bash
# Criar sessão
curl -X POST http://localhost:8888/api/v1/session/create \
  -H "x-user-secret: $USER_SECRET"

# Obter QR Code
curl -X GET http://localhost:8888/api/v1/session/qr \
  -H "x-user-secret: $USER_SECRET" \
  -H "Accept: text/event-stream"

# Conectar sessão
curl -X POST http://localhost:8888/api/v1/session/connect \
  -H "x-user-secret: $USER_SECRET"

# Enviar mensagem
curl -X POST http://localhost:8888/api/v1/message/text \
  -H "x-user-secret: $USER_SECRET" \
  -H "Content-Type: application/json" \
  -d '{
    "to": "5511888777666@s.whatsapp.net",
    "message": "Mensagem de teste!"
  }'
```

---

## 5. Vantagens do Novo Sistema

1. **Segurança**: O userID está criptografado e não pode ser alterado pelo cliente
2. **Simplicidade**: Usuário só precisa de um header (`x-user-secret`) em vez de dois
3. **Expiração**: As chaves têm prazo de validade (24 horas por padrão)
4. **Controle Admin**: Apenas admins podem gerar chaves para usuários
5. **Compatibilidade**: O método antigo ainda funciona para retrocompatibilidade

---

## 6. Códigos de Erro

- **400 Bad Request**: Dados inválidos no request
- **401 Unauthorized**: Chave inválida ou expirada
- **403 Forbidden**: Chave admin inválida
- **500 Internal Server Error**: Erro interno do servidor

### Exemplos de Respostas de Erro:

**Chave expirada:**
```json
{
  "error": "User secret expirado"
}
```

**Formato inválido:**
```json
{
  "error": "User secret com formato inválido"
}
```

**Chave admin necessária:**
```json
{
  "error": "Admin key required",
  "code": "MISSING_ADMIN_KEY"
}
```
