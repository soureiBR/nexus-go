// internal/api/handlers/session.go
package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"

	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

// SessionHandler gerencia endpoints para operações com sessões
type SessionHandler struct {
	sessionManager *whatsapp.SessionManager
}

// CreateSessionRequest representa a requisição para criar uma sessão
type CreateSessionRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

// SessionResponse representa a resposta de uma operação com sessão
type SessionResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Connected  bool   `json:"connected"`
	CreatedAt  string `json:"created_at"`
	LastActive string `json:"last_active,omitempty"`
	Picture    string `json:"picture,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// NewSessionHandler cria um novo handler para sessões
func NewSessionHandler(sm *whatsapp.SessionManager) *SessionHandler {
	return &SessionHandler{
		sessionManager: sm,
	}
}

// CreateSession cria uma nova sessão de WhatsApp
func (h *SessionHandler) CreateSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Criar sessão
	client, err := h.sessionManager.CreateSession(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao criar sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar sessão", "details": err.Error()})
		return
	}

	// Verificar se cliente já está conectado
	status := "created"
	if client.Connected {
		status = "connected"
	}

	resp := SessionResponse{
		ID:        userIDStr,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}

	if !client.LastActive.IsZero() {
		resp.LastActive = client.LastActive.Format(time.RFC3339)
	}

	// Obter foto de perfil se o cliente estiver conectado
	if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
		myJID := client.WAClient.Store.ID.ToNonAD()
		pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
		if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
			resp.Picture = pictureInfo.URL
		}
	}

	c.JSON(http.StatusCreated, resp)
}

// GetSession retorna informações sobre uma sessão existente
func (h *SessionHandler) GetSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Determinar status
	status := "disconnected"
	if client.Connected {
		status = "connected"
	}

	resp := SessionResponse{
		ID:        userIDStr,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}

	if !client.LastActive.IsZero() {
		resp.LastActive = client.LastActive.Format(time.RFC3339)
	}

	// Obter foto de perfil se o cliente estiver conectado
	if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
		ownJID := client.WAClient.Store.ID
        if !ownJID.IsEmpty() {
            phoneNumber := ownJID.User // This contains your phone number
            resp.PhoneNumber = phoneNumber
        }
		myJID := client.WAClient.Store.ID.ToNonAD()
		pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
		if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
			resp.Picture = pictureInfo.URL
		}
	}

	c.JSON(http.StatusOK, resp)
}

// GetQRCode gera um QR code para autenticação
func (h *SessionHandler) GetQRCode(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Verificar se já existe uma sessão e se está autenticada
	client, exists := h.sessionManager.GetSession(userIDStr)
	if exists && client.WAClient.Store.ID != nil {
		if !client.Connected {
			// Sessão autenticada mas desconectada - resetar automaticamente
			logger.Info("Resetando sessão autenticada mas desconectada", "user_id", userIDStr)
			if err := h.sessionManager.ResetSession(c.Request.Context(), userIDStr); err != nil {
				logger.Error("Falha ao resetar sessão", "error", err, "user_id", userIDStr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao resetar sessão", "details": err.Error()})
				return
			}
		} else {
			// Sessão autenticada E conectada - não permitir gerar novo QR
			c.JSON(http.StatusBadRequest, gin.H{
				"error":     "Sessão já está autenticada e conectada",
				"connected": true,
			})
			return
		}
	}

	// Criar contexto com timeout mais longo para permitir o escaneamento
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Obter canal de QR
	qrChan, err := h.sessionManager.GetQRChannel(ctx, userIDStr)
	if err != nil {
		logger.Error("Falha ao obter canal QR", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao gerar QR code", "details": err.Error()})
		return
	}

	// Iniciar processamento de stream como SSE (Server-Sent Events)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Enviar um evento inicial para confirmar conexão
	c.SSEvent("status", gin.H{"message": "Aguardando QR code..."})
	c.Writer.Flush()

	// Função para verificar estado de conexão periodicamente
	connectionCheckTicker := time.NewTicker(5 * time.Second)
	defer connectionCheckTicker.Stop()

	// Monitorar o canal do cliente para enviar atualizações
	for {
		select {
		case evt, ok := <-qrChan:
			if !ok {
				// Canal fechado
				c.SSEvent("error", gin.H{"message": "Canal de QR code fechado"})
				c.Writer.Flush()
				return
			}

			logger.Info("Evento QR recebido", "event", evt.Event, "user_id", userIDStr)

			if evt.Event == "code" {
				// Imprimir QR no terminal para debug
				fmt.Println("\n===== QR CODE para sessão", userIDStr, "=====")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("\nEscaneie o código acima com o seu WhatsApp")

				// Gerar QR para cliente
				qrImg, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
				if err != nil {
					c.SSEvent("error", gin.H{"message": "Falha ao gerar QR code"})
					c.Writer.Flush()
					return
				}

				// Enviar QR code como evento SSE
				qrBase64 := base64.StdEncoding.EncodeToString(qrImg)
				c.SSEvent("qrcode", gin.H{
					"qrcode": qrBase64,
					"data":   evt.Code,
				})
				c.Writer.Flush()
			} else if evt.Event == "success" {
				// Login realizado com sucesso
				c.SSEvent("success", gin.H{"message": "Login realizado com sucesso"})
				c.Writer.Flush()
				return
			} else {
				// Outros eventos (timeout, etc)
				c.SSEvent("status", gin.H{"event": evt.Event})
				c.Writer.Flush()
			}

		case <-connectionCheckTicker.C:
			// Verificar se o cliente está conectado a cada 5 segundos
			updatedClient, exists := h.sessionManager.GetSession(userIDStr)
			if exists && updatedClient.Connected && updatedClient.WAClient.Store.ID != nil {
				c.SSEvent("success", gin.H{
					"message": "Cliente autenticado e conectado",
				})
				c.Writer.Flush()
				return
			}

		case <-ctx.Done():
			// Timeout geral
			c.SSEvent("error", gin.H{"message": "Timeout ao aguardar QR code"})
			c.Writer.Flush()
			return
		}
	}
}

// DeleteSession encerra uma sessão
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Excluir sessão
	if err := h.sessionManager.DeleteSession(c.Request.Context(), userIDStr); err != nil {
		logger.Error("Falha ao excluir sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao excluir sessão", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sessão encerrada com sucesso"})
}

// ConnectSession inicia a conexão com o WhatsApp
func (h *SessionHandler) ConnectSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Iniciar conexão
	err := h.sessionManager.Connect(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao conectar sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao conectar ao WhatsApp", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conectando ao WhatsApp"})
}

// DisconnectSession encerra a conexão com o WhatsApp
func (h *SessionHandler) DisconnectSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Encerrar conexão
	err := h.sessionManager.Disconnect(userIDStr)
	if err != nil {
		logger.Error("Falha ao desconectar sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao desconectar do WhatsApp", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Desconectado do WhatsApp"})
}

// LogoutSession encerra e remove a sessão do WhatsApp
func (h *SessionHandler) LogoutSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Fazer logout da sessão
	err := h.sessionManager.Logout(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao fazer logout da sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao fazer logout do WhatsApp", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logout realizado com sucesso"})
}
