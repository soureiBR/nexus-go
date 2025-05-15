// internal/api/handlers/message.go
package handlers

import (
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

type MessageHandler struct {
	sessionManager *whatsapp.SessionManager
}

type TextMessageRequest struct {
	UserID  string `json:"user_id" binding:"required"`
	To      string `json:"to" binding:"required"`
	Message string `json:"message" binding:"required"`
}

type MediaMessageRequest struct {
	UserID    string `form:"user_id" binding:"required"`
	To        string `form:"to" binding:"required"`
	Caption   string `form:"caption"`
	MediaType string `form:"media_type" binding:"required"`
}

type ButtonMessageRequest struct {
	UserID  string                `json:"user_id" binding:"required"`
	To      string                `json:"to" binding:"required"`
	Text    string                `json:"text" binding:"required"`
	Footer  string                `json:"footer"`
	Buttons []whatsapp.ButtonData `json:"buttons" binding:"required,min=1,max=3"`
}

type ListMessageRequest struct {
	UserID     string             `json:"user_id" binding:"required"`
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

// SaveUploadedFile salva um arquivo enviado pelo cliente
func saveUploadedFile(file *multipart.FileHeader, dst string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	// Garantir que o diretório existe
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// SendText envia uma mensagem de texto
func (h *MessageHandler) SendText(c *gin.Context) {
	var req TextMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(req.UserID)
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
	msgID, err := h.sessionManager.SendText(req.UserID, req.To, req.Message)
	if err != nil {
		logger.Error("Falha ao enviar mensagem", "error", err, "user_id", req.UserID, "to", req.To)
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
	// Vincular dados do formulário
	var req MediaMessageRequest
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Receber arquivo
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Arquivo não fornecido", "details": err.Error()})
		return
	}

	// Verificar tipo de mídia
	mediaType := req.MediaType
	if mediaType == "" {
		// Tentar deduzir do Content-Type
		mediaType = file.Header.Get("Content-Type")
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(req.UserID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Verificar se o cliente está conectado
	if !client.Connected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliente não está conectado"})
		return
	}

	// Salvar arquivo temporariamente
	tempPath := filepath.Join(os.TempDir(), file.Filename)
	if err := saveUploadedFile(file, tempPath); err != nil {
		logger.Error("Falha ao salvar arquivo", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao salvar arquivo", "details": err.Error()})
		return
	}

	// Enviar mídia
	msgID, err := h.sessionManager.SendMedia(req.UserID, req.To, tempPath, mediaType, req.Caption)

	// Remover arquivo temporário
	os.Remove(tempPath)

	if err != nil {
		logger.Error("Falha ao enviar mídia", "error", err, "user_id", req.UserID, "to", req.To)
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
	var req ButtonMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(req.UserID)
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
	msgID, err := h.sessionManager.SendButtons(req.UserID, req.To, req.Text, req.Footer, req.Buttons)
	if err != nil {
		logger.Error("Falha ao enviar mensagem com botões", "error", err, "user_id", req.UserID, "to", req.To)
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
	var req ListMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Verificar se a sessão existe
	client, exists := h.sessionManager.GetSession(req.UserID)
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
	msgID, err := h.sessionManager.SendList(req.UserID, req.To, req.Text, req.Footer, req.ButtonText, req.Sections)
	if err != nil {
		logger.Error("Falha ao enviar mensagem com lista", "error", err, "user_id", req.UserID, "to", req.To)
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
