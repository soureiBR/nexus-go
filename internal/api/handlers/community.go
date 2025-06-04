// internal/api/handlers/community.go
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

// Request structures for community operations
type CreateCommunityRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type CommunityInfoRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
}

type UpdateCommunityNameRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
	NewName      string `json:"new_name" binding:"required"`
}

type UpdateCommunityDescriptionRequest struct {
	CommunityJID   string `json:"community_jid" binding:"required"`
	NewDescription string `json:"new_description" binding:"required"`
}

type UpdateCommunityPictureRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
	PictureURL   string `json:"picture_url" binding:"required"`
}

type LeaveCommunityRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
}

type CreateGroupForCommunityRequest struct {
	CommunityJID string   `json:"community_jid" binding:"required"`
	GroupName    string   `json:"group_name" binding:"required"`
	Participants []string `json:"participants"`
}

type LinkGroupRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
	GroupJID     string `json:"group_jid" binding:"required"`
}

type UnlinkGroupRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
	GroupJID     string `json:"group_jid" binding:"required"`
}

type JoinCommunityWithLinkRequest struct {
	Link string `json:"link" binding:"required"`
}

type GetCommunityInviteLinkRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
}

type GetCommunityLinkedGroupsRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
}

type RevokeCommunityInviteLinkRequest struct {
	CommunityJID string `json:"community_jid" binding:"required"`
}

// CommunityHandler gerencia endpoints para operações de comunidades
type CommunityHandler struct {
	sessionManager *whatsapp.SessionManager
}

// NewCommunityHandler cria um novo handler de comunidades
func NewCommunityHandler(sm *whatsapp.SessionManager) *CommunityHandler {
	return &CommunityHandler{
		sessionManager: sm,
	}
}

// submitWorkerTask submits a task to the worker system and waits for response with proper error handling
func (h *CommunityHandler) submitWorkerTask(userID string, taskType worker.CommandType, payload interface{}) (interface{}, error) {
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

// CreateCommunity cria uma nova comunidade
func (h *CommunityHandler) CreateCommunity(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req CreateCommunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Validate session exists and is connected
	session, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	if !session.IsActive() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sessão não está ativa ou conectada"})
		return
	}

	// Create payload
	payload := worker.CreateCommunityPayload{
		Name:        req.Name,
		Description: req.Description,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdCreateCommunity, payload)
	if err != nil {
		logger.Error("Falha ao criar comunidade", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    result,
		"message": "Comunidade criada com sucesso",
	})
}

// GetCommunityInfo obtém informações de uma comunidade
func (h *CommunityHandler) GetCommunityInfo(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Get community_jid from query parameters
	communityJID := c.Query("community_jid")
	if communityJID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "community_jid query parameter is required"})
		return
	}

	// Create payload
	payload := worker.CommunityInfoPayload{
		CommunityJID: communityJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetCommunityInfo, payload)
	if err != nil {
		logger.Error("Falha ao obter informações da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", communityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetJoinedCommunities obtém lista de comunidades em que o usuário é membro
func (h *CommunityHandler) GetJoinedCommunities(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Submit task to worker (no payload needed)
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetJoinedCommunities, nil)
	if err != nil {
		logger.Error("Falha ao obter lista de comunidades", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter lista de comunidades", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// UpdateCommunityName atualiza o nome da comunidade
func (h *CommunityHandler) UpdateCommunityName(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateCommunityNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.UpdateCommunityNamePayload{
		CommunityJID: req.CommunityJID,
		NewName:      req.NewName,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateCommunityName, payload)
	if err != nil {
		logger.Error("Falha ao atualizar nome da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar nome da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Nome da comunidade atualizado com sucesso",
	})
}

// UpdateCommunityDescription atualiza a descrição da comunidade
func (h *CommunityHandler) UpdateCommunityDescription(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateCommunityDescriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.UpdateCommunityDescriptionPayload{
		CommunityJID:   req.CommunityJID,
		NewDescription: req.NewDescription,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateCommunityDescription, payload)
	if err != nil {
		logger.Error("Falha ao atualizar descrição da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar descrição da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Descrição da comunidade atualizada com sucesso",
	})
}



// LeaveCommunity sai de uma comunidade
func (h *CommunityHandler) LeaveCommunity(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req LeaveCommunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.LeaveCommunityPayload{
		CommunityJID: req.CommunityJID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdLeaveCommunity, payload)
	if err != nil {
		logger.Error("Falha ao sair da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao sair da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Saiu da comunidade com sucesso",
	})
}

// CreateGroupForCommunity cria um novo grupo dentro de uma comunidade
func (h *CommunityHandler) CreateGroupForCommunity(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req CreateGroupForCommunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.CreateGroupForCommunityPayload{
		CommunityJID: req.CommunityJID,
		GroupName:    req.GroupName,
		Participants: req.Participants,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdCreateGroupForCommunity, payload)
	if err != nil {
		logger.Error("Falha ao criar grupo na comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar grupo na comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    result,
		"message": "Grupo criado na comunidade com sucesso",
	})
}

// LinkGroupToCommunity vincula um grupo existente a uma comunidade
func (h *CommunityHandler) LinkGroupToCommunity(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req LinkGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.LinkGroupPayload{
		CommunityJID: req.CommunityJID,
		GroupJID:     req.GroupJID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdLinkGroupToCommunity, payload)
	if err != nil {
		logger.Error("Falha ao vincular grupo à comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao vincular grupo à comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Grupo vinculado à comunidade com sucesso",
	})
}

// UnlinkGroupFromCommunity desvincula um grupo de uma comunidade
func (h *CommunityHandler) UnlinkGroupFromCommunity(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UnlinkGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.LinkGroupPayload{
		CommunityJID: req.CommunityJID,
		GroupJID:     req.GroupJID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUnlinkGroupFromCommunity, payload)
	if err != nil {
		logger.Error("Falha ao desvincular grupo da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao desvincular grupo da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Grupo desvinculado da comunidade com sucesso",
	})
}

// JoinCommunityWithLink entra em uma comunidade usando um link de convite
func (h *CommunityHandler) JoinCommunityWithLink(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req JoinCommunityWithLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.JoinCommunityWithLinkPayload{
		Link: req.Link,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdJoinCommunityWithLink, payload)
	if err != nil {
		logger.Error("Falha ao entrar na comunidade via link", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao entrar na comunidade via link", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "Entrou na comunidade com sucesso",
	})
}

// GetCommunityInviteLink obtém o link de convite de uma comunidade
func (h *CommunityHandler) GetCommunityInviteLink(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Get community_jid from query parameters
	communityJID := c.Query("community_jid")
	if communityJID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "community_jid query parameter is required"})
		return
	}

	// Create payload
	payload := worker.GetCommunityInviteLinkPayload{
		CommunityJID: communityJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetCommunityInviteLink, payload)
	if err != nil {
		logger.Error("Falha ao obter link de convite da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", communityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter link de convite da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"invite_link": result,
	})
}

// RevokeCommunityInviteLink revoga o link atual e gera um novo
func (h *CommunityHandler) RevokeCommunityInviteLink(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req RevokeCommunityInviteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GetCommunityInviteLinkPayload{
		CommunityJID: req.CommunityJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdRevokeCommunityInviteLink, payload)
	if err != nil {
		logger.Error("Falha ao revogar link de convite da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao revogar link de convite da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"invite_link": result,
		"message":     "Link de convite revogado e novo gerado com sucesso",
	})
}

// UpdateCommunityPicture atualiza a foto da comunidade a partir de uma URL
func (h *CommunityHandler) UpdateCommunityPicture(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateCommunityPictureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Validate session exists and is connected
	session, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	if !session.IsActive() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sessão não está ativa ou conectada"})
		return
	}

	// Create payload
	payload := worker.UpdateCommunityPicturePayload{
		CommunityJID: req.CommunityJID,
		ImageURL:     req.PictureURL,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateCommunityPicture, payload)
	if err != nil {
		logger.Error("Falha ao atualizar foto da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", req.CommunityJID,
			"picture_url", req.PictureURL)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar foto da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Foto da comunidade atualizada com sucesso",
	})
}

// GetCommunityLinkedGroups obtém todos os grupos vinculados a uma comunidade
func (h *CommunityHandler) GetCommunityLinkedGroups(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Get community_jid from query parameters
	communityJID := c.Query("community_jid")
	if communityJID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "community_jid query parameter is required"})
		return
	}

	// Validate session exists and is connected
	session, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	if !session.IsActive() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sessão não está ativa ou conectada"})
		return
	}

	// Create payload
	payload := worker.GetCommunityLinkedGroupsPayload{
		CommunityJID: communityJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetCommunityLinkedGroups, payload)
	if err != nil {
		logger.Error("Falha ao obter grupos vinculados da comunidade",
			"error", err,
			"user_id", userIDStr,
			"community_jid", communityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter grupos vinculados da comunidade", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
