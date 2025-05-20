// internal/api/handlers/message.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

type MessageHandler struct {
	sessionManager *whatsapp.SessionManager
}

type TextMessageRequest struct {
	To      string `json:"to" binding:"required"`
	Message string `json:"message" binding:"required"`
}

type MediaMessageRequest struct {
	To        string `json:"to" binding:"required"`
	Caption   string `json:"caption"`
	MediaURL  string `json:"media_url" binding:"required"`
	MediaType string `json:"media_type" binding:"required"`
}

type ButtonMessageRequest struct {
	To      string                `json:"to" binding:"required"`
	Text    string                `json:"text" binding:"required"`
	Footer  string                `json:"footer"`
	Buttons []whatsapp.ButtonData `json:"buttons" binding:"required,min=1,max=3"`
}

type ListMessageRequest struct {
	To         string             `json:"to" binding:"required"`
	Text       string             `json:"text" binding:"required"`
	Footer     string             `json:"footer"`
	ButtonText string             `json:"button_text" binding:"required"`
	Sections   []whatsapp.Section `json:"sections" binding:"required,min=1"`
}

type MessageResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

func NewMessageHandler(sm *whatsapp.SessionManager) *MessageHandler {
	return &MessageHandler{
		sessionManager: sm,
	}
}

// SendText envia uma mensagem de texto
func (h *MessageHandler) SendText(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req TextMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Verificar se o cliente está conectado
	if !client.Connected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliente não está conectado"})
		return
	}

	// Enviar mensagem
	msgID, err := h.sessionManager.SendText(userIDStr, req.To, req.Message)
	if err != nil {
		logger.Error("Falha ao enviar mensagem", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mensagem", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{
		MessageID: msgID,
		Status:    "sent",
	})
}

// SendMedia envia uma mensagem de mídia (imagem, vídeo, documento, etc.)
func (h *MessageHandler) SendMedia(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req MediaMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Ensure media URL is provided
	if req.MediaURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "É necessário fornecer media_url"})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Verificar se o cliente está conectado
	if !client.Connected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliente não está conectado"})
		return
	}

	// Enviar mídia usando a URL
	msgID, err := h.sessionManager.SendMedia(userIDStr, req.To, req.MediaURL, req.MediaType, req.Caption)
	if err != nil {
		logger.Error("Falha ao enviar mídia", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mídia", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{
		MessageID: msgID,
		Status:    "sent",
	})
}

// SendButtons envia uma mensagem com botões
func (h *MessageHandler) SendButtons(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ButtonMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Verificar se o cliente está conectado
	if !client.Connected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliente não está conectado"})
		return
	}

	// Enviar mensagem com botões
	msgID, err := h.sessionManager.SendButtons(userIDStr, req.To, req.Text, req.Footer, req.Buttons)
	if err != nil {
		logger.Error("Falha ao enviar mensagem com botões", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mensagem com botões", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{
		MessageID: msgID,
		Status:    "sent",
	})
}

// SendList envia uma mensagem com lista
func (h *MessageHandler) SendList(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ListMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Verificar se o cliente está conectado
	if !client.Connected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliente não está conectado"})
		return
	}

	// Enviar mensagem com lista
	msgID, err := h.sessionManager.SendList(userIDStr, req.To, req.Text, req.Footer, req.ButtonText, req.Sections)
	if err != nil {
		logger.Error("Falha ao enviar mensagem com lista", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mensagem com lista", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, MessageResponse{
		MessageID: msgID,
		Status:    "sent",
	})
}

// SendTemplate envia uma mensagem de template
func (h *MessageHandler) SendTemplate(c *gin.Context) {

	// Implementação semelhante às anteriores para envio de templates
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Funcionalidade em desenvolvimento"})
}
