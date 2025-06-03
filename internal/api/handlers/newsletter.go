// internal/api/handlers/newsletter.go
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"yourproject/internal/services/whatsapp"
	"yourproject/internal/services/whatsapp/worker"
	"yourproject/pkg/logger"
)

// NewsletterHandler gerencia endpoints para operações de canais do WhatsApp
type NewsletterHandler struct {
	sessionManager *whatsapp.SessionManager
}

// NewNewsletterHandler cria um novo handler de newsletter
func NewNewsletterHandler(sm *whatsapp.SessionManager) *NewsletterHandler {
	return &NewsletterHandler{
		sessionManager: sm,
	}
}

// submitWorkerTask submits a task to the worker system and waits for response with proper error handling
func (h *NewsletterHandler) submitWorkerTask(userID string, taskType worker.CommandType, payload interface{}) (interface{}, error) {
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

	// Create task
	task := worker.Task{
		ID:       fmt.Sprintf("%s_%s_%d", taskType, userID, time.Now().UnixNano()),
		Type:     taskType,
		UserID:   userID,
		Priority: worker.NormalPriority,
		Payload:  payload,
		Response: responseChan,
		Created:  time.Now(),
	}

	// Submit task to worker pool
	if err := workerPool.SubmitTask(task); err != nil {
		return nil, fmt.Errorf("failed to submit task: %w", err)
	}

	// Wait for response with timeout
	select {
	case response := <-responseChan:
		if response.Error != nil {
			return nil, response.Error
		}
		return response.Data, nil
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for worker response")
	}
}

// CreateChannelRequest representa a requisição para criar um canal
type CreateChannelRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	PictureURL  string `json:"picture_url"`
}

// ChannelJIDRequest representa uma requisição que identifica um canal por JID
type ChannelJIDRequest struct {
	JID string `json:"jid" binding:"required"`
}

// ChannelInviteRequest representa uma requisição para obter canal por convite
type ChannelInviteRequest struct {
	InviteLink string `json:"invite_link" binding:"required"`
}

// UpdateNewsletterPictureRequest representa uma requisição para atualizar foto de newsletter
type UpdateNewsletterPictureRequest struct {
	JID      string `json:"jid" binding:"required"`
	ImageURL string `json:"image_url" binding:"required"`
}

// UpdateNewsletterNameRequest representa uma requisição para atualizar nome de newsletter
type UpdateNewsletterNameRequest struct {
	JID  string `json:"jid" binding:"required"`
	Name string `json:"name" binding:"required"`
}

// UpdateNewsletterDescriptionRequest representa uma requisição para atualizar descrição de newsletter
type UpdateNewsletterDescriptionRequest struct {
	JID         string `json:"jid" binding:"required"`
	Description string `json:"description" binding:"required"`
}

// ListChannelsRequest representa uma requisição para listar canais
type ListChannelsRequest struct{}

// CreateChannel cria um novo canal do WhatsApp
func (h *NewsletterHandler) CreateChannel(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req CreateChannelRequest
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
	payload := worker.CreateChannelPayload{
		Name:        req.Name,
		Description: req.Description,
		PictureURL:  req.PictureURL,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdCreateChannel, payload)
	if err != nil {
		logger.Error("Falha ao criar canal", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, result)
}

// GetChannelInfo obtém informações sobre um canal específico
func (h *NewsletterHandler) GetChannelInfo(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ChannelJIDRequest
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
	payload := worker.ChannelJIDPayload{
		JID: req.JID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetChannelInfo, payload)
	if err != nil {
		logger.Error("Falha ao obter informações do canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações do canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetChannelWithInvite obtém informações do canal usando um link de convite
func (h *NewsletterHandler) GetChannelWithInvite(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ChannelInviteRequest
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
	payload := worker.ChannelInvitePayload{
		InviteLink: req.InviteLink,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetChannelWithInvite, payload)
	if err != nil {
		logger.Error("Falha ao obter informações do canal por convite", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações do canal por convite", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ListMyChannels lista todos os canais que o usuário está inscrito
func (h *NewsletterHandler) ListMyChannels(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

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
	payload := worker.ListChannelsPayload{}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdListMyChannels, payload)
	if err != nil {
		logger.Error("Falha ao listar canais inscritos", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao listar canais inscritos", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// FollowChannel inscreve o usuário em um canal
func (h *NewsletterHandler) FollowChannel(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ChannelJIDRequest
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
	payload := worker.ChannelJIDPayload{
		JID: req.JID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdFollowChannel, payload)
	if err != nil {
		logger.Error("Falha ao seguir canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao seguir canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Canal seguido com sucesso"})
}

// UnfollowChannel cancela a inscrição do usuário em um canal
func (h *NewsletterHandler) UnfollowChannel(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ChannelJIDRequest
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
	payload := worker.ChannelJIDPayload{
		JID: req.JID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUnfollowChannel, payload)
	if err != nil {
		logger.Error("Falha ao deixar de seguir canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao deixar de seguir canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Inscrição no canal cancelada com sucesso"})
}

// MuteChannel silencia notificações de um canal
func (h *NewsletterHandler) MuteChannel(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ChannelJIDRequest
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
	payload := worker.ChannelJIDPayload{
		JID: req.JID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdMuteChannel, payload)
	if err != nil {
		logger.Error("Falha ao silenciar canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao silenciar canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Canal silenciado com sucesso"})
}

// UnmuteChannel reativa notificações de um canal
func (h *NewsletterHandler) UnmuteChannel(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ChannelJIDRequest
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
	payload := worker.ChannelJIDPayload{
		JID: req.JID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUnmuteChannel, payload)
	if err != nil {
		logger.Error("Falha ao reativar notificações do canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao reativar notificações do canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notificações do canal reativadas com sucesso"})
}

// UpdateNewsletterPicture atualiza a foto da newsletter a partir de uma URL
func (h *NewsletterHandler) UpdateNewsletterPicture(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateNewsletterPictureRequest
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
	payload := worker.UpdateNewsletterPicturePayload{
		JID:      req.JID,
		ImageURL: req.ImageURL,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateNewsletterPicture, payload)
	if err != nil {
		logger.Error("Falha ao atualizar foto da newsletter",
			"error", err,
			"user_id", userIDStr,
			"newsletter_jid", req.JID,
			"image_url", req.ImageURL)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar foto da newsletter", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Foto da newsletter atualizada com sucesso",
		"data":    result,
	})
}

// UpdateNewsletterName atualiza o nome da newsletter
func (h *NewsletterHandler) UpdateNewsletterName(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateNewsletterNameRequest
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
	payload := worker.UpdateNewsletterNamePayload{
		JID:  req.JID,
		Name: req.Name,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateNewsletterName, payload)
	if err != nil {
		logger.Error("Falha ao atualizar nome da newsletter",
			"error", err,
			"user_id", userIDStr,
			"newsletter_jid", req.JID,
			"name", req.Name)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar nome da newsletter", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Nome da newsletter atualizado com sucesso",
		"data":    result,
	})
}

// UpdateNewsletterDescription atualiza a descrição da newsletter
func (h *NewsletterHandler) UpdateNewsletterDescription(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateNewsletterDescriptionRequest
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
	payload := worker.UpdateNewsletterDescriptionPayload{
		JID:         req.JID,
		Description: req.Description,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateNewsletterDescription, payload)
	if err != nil {
		logger.Error("Falha ao atualizar descrição da newsletter",
			"error", err,
			"user_id", userIDStr,
			"newsletter_jid", req.JID,
			"description", req.Description)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar descrição da newsletter", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Descrição da newsletter atualizada com sucesso",
		"data":    result,
	})
}
