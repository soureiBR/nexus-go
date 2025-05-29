// internal/api/handlers/message.go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"yourproject/internal/services/whatsapp"
	"yourproject/internal/services/whatsapp/worker"
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
	To      string              `json:"to" binding:"required"`
	Text    string              `json:"text" binding:"required"`
	Footer  string              `json:"footer"`
	Buttons []worker.ButtonData `json:"buttons" binding:"required,min=1,max=3"`
}

type ListMessageRequest struct {
	To         string           `json:"to" binding:"required"`
	Text       string           `json:"text" binding:"required"`
	Footer     string           `json:"footer"`
	ButtonText string           `json:"button_text" binding:"required"`
	Sections   []worker.Section `json:"sections" binding:"required,min=1"`
}

type CheckNumberRequest struct {
	Number string `json:"number" binding:"required"`
}

type CheckNumberResponse struct {
	Number string `json:"number"`
	Exists bool   `json:"exists"`
	Status string `json:"status"`
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

// submitWorkerTask submits a task to the worker system and waits for response with proper error handling
func (h *MessageHandler) submitWorkerTask(userID string, taskType worker.CommandType, payload interface{}) (interface{}, error) {
	// Get coordinator and worker pool
	coordinator := h.sessionManager.GetCoordinator()
	if coordinator == nil {
		return nil, fmt.Errorf("coordinator not available")
	}

	workerPool := coordinator.GetWorkerPool()
	if workerPool == nil {
		return nil, fmt.Errorf("worker pool not available")
	}

	// Ensure worker exists for user
	if _, exists := workerPool.GetWorker(userID); !exists {
		logger.Debug("Creating worker for user", "user_id", userID)
		if err := coordinator.CreateWorker(userID); err != nil {
			return nil, fmt.Errorf("failed to create worker: %w", err)
		}

		// Give worker a moment to initialize
		time.Sleep(100 * time.Millisecond)
	}

	// Create response channel with proper buffering
	responseChan := make(chan worker.CommandResponse, 1)

	// Create task with proper ID generation
	task := worker.Task{
		ID:         fmt.Sprintf("%s_%s_%d", taskType, userID, time.Now().UnixNano()),
		Type:       taskType,
		UserID:     userID,
		Priority:   worker.NormalPriority,
		Payload:    payload,
		Response:   responseChan,
		Created:    time.Now(),
		MaxRetries: 3,
	}

	// Submit task
	if err := workerPool.SubmitTask(task); err != nil {
		close(responseChan)
		return nil, fmt.Errorf("failed to submit task: %w", err)
	}

	// Wait for response with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, response.Error
		}
		return response.Data, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("task timeout after 30 seconds")
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

	// Create payload
	payload := worker.SendTextPayload{
		To:      req.To,
		Message: req.Message,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdSendText, payload)
	if err != nil {
		logger.Error("Falha ao enviar mensagem", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mensagem", "details": err.Error()})
		return
	}

	// Extract message ID from result
	msgID := ""
	if resultStr, ok := result.(string); ok {
		msgID = resultStr
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

	// Create payload
	payload := worker.SendMediaPayload{
		To:        req.To,
		MediaURL:  req.MediaURL,
		MediaType: req.MediaType,
		Caption:   req.Caption,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdSendMedia, payload)
	if err != nil {
		logger.Error("Falha ao enviar mídia", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mídia", "details": err.Error()})
		return
	}

	// Extract message ID from result
	msgID := ""
	if resultStr, ok := result.(string); ok {
		msgID = resultStr
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

	// Create payload
	payload := worker.SendButtonsPayload{
		To:      req.To,
		Text:    req.Text,
		Footer:  req.Footer,
		Buttons: req.Buttons,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdSendButtons, payload)
	if err != nil {
		logger.Error("Falha ao enviar mensagem com botões", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mensagem com botões", "details": err.Error()})
		return
	}

	// Extract message ID from result
	msgID := ""
	if resultStr, ok := result.(string); ok {
		msgID = resultStr
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

	// Create payload
	payload := worker.SendListPayload{
		To:         req.To,
		Text:       req.Text,
		Footer:     req.Footer,
		ButtonText: req.ButtonText,
		Sections:   req.Sections,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdSendList, payload)
	if err != nil {
		logger.Error("Falha ao enviar mensagem com lista", "error", err, "user_id", userIDStr, "to", req.To)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar mensagem com lista", "details": err.Error()})
		return
	}

	// Extract message ID from result
	msgID := ""
	if resultStr, ok := result.(string); ok {
		msgID = resultStr
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

// CheckNumber verifica se um número existe no WhatsApp
func (h *MessageHandler) CheckNumber(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req CheckNumberRequest
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

	// Create payload
	payload := worker.CheckNumberPayload{
		Number: req.Number,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdCheckNumber, payload)
	if err != nil {
		logger.Error("Falha ao verificar número", "error", err, "user_id", userIDStr, "number", req.Number)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao verificar número", "details": err.Error()})
		return
	}

	// Extract result
	exists = false
	if resultBool, ok := result.(bool); ok {
		exists = resultBool
	}

	c.JSON(http.StatusOK, CheckNumberResponse{
		Number: req.Number,
		Exists: exists,
		Status: "checked",
	})
}
