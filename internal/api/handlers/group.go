// internal/api/handlers/group.go
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

// GroupHandler gerencia endpoints para operações de grupos
type GroupHandler struct {
	sessionManager *whatsapp.SessionManager
}

// NewGroupHandler cria um novo handler de grupos
func NewGroupHandler(sm *whatsapp.SessionManager) *GroupHandler {
	return &GroupHandler{
		sessionManager: sm,
	}
}

// CreateGroupRequest representa a requisição para criar um grupo
type CreateGroupRequest struct {
	Name         string   `json:"name" binding:"required"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

// AddParticipantsRequest representa a requisição para adicionar participantes
type AddParticipantsRequest struct {
	GroupJID     string   `json:"group_jid" binding:"required"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

// RemoveParticipantsRequest representa a requisição para remover participantes
type RemoveParticipantsRequest struct {
	GroupJID     string   `json:"group_jid" binding:"required"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

// PromoteParticipantsRequest representa a requisição para promover participantes
type PromoteParticipantsRequest struct {
	GroupJID     string   `json:"group_jid" binding:"required"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

// DemoteParticipantsRequest representa a requisição para rebaixar participantes
type DemoteParticipantsRequest struct {
	GroupJID     string   `json:"group_jid" binding:"required"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

// UpdateGroupNameRequest representa a requisição para atualizar nome do grupo
type UpdateGroupNameRequest struct {
	GroupJID string `json:"group_jid" binding:"required"`
	NewName  string `json:"new_name" binding:"required"`
}

// UpdateGroupTopicRequest representa a requisição para atualizar tópico do grupo
type UpdateGroupTopicRequest struct {
	UserID   string `json:"user_id" binding:"required"`
	GroupJID string `json:"group_jid" binding:"required"`
	NewTopic string `json:"new_topic" binding:"required"`
}

// LeaveGroupRequest representa a requisição para sair de um grupo
type LeaveGroupRequest struct {
	GroupJID string `json:"group_jid" binding:"required"`
}

// GroupInfoRequest representa a requisição para obter informações do grupo
type GroupInfoRequest struct {
	GroupJID string `json:"group_jid" binding:"required"`
}

// JoinGroupWithLinkRequest representa a requisição para entrar em um grupo via link
type JoinGroupWithLinkRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Link   string `json:"link" binding:"required"`
}

// GetInviteLinkRequest representa a requisição para obter link de convite
type GetInviteLinkRequest struct {
	GroupJID string `json:"group_jid" binding:"required"`
}

// RevokeInviteLinkRequest representa a requisição para revogar link de convite
type RevokeInviteLinkRequest struct {
	GroupJID string `json:"group_jid" binding:"required"`
}

// submitWorkerTask submits a task to the worker system and waits for response with proper error handling
func (h *GroupHandler) submitWorkerTask(userID string, taskType worker.CommandType, payload interface{}) (interface{}, error) {
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

// CreateGroup cria um novo grupo
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req CreateGroupRequest
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
	payload := worker.CreateGroupPayload{
		Name:         req.Name,
		Participants: req.Participants,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdCreateGroup, payload)
	if err != nil {
		logger.Error("Falha ao criar grupo", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    result,
		"message": "Grupo criado com sucesso",
	})
}

// GetGroupInfo obtém informações de um grupo
func (h *GroupHandler) GetGroupInfo(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req GroupInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupInfoPayload{
		GroupJID: req.GroupJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetGroupInfo, payload)
	if err != nil {
		logger.Error("Falha ao obter informações do grupo",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetJoinedGroups obtém lista de grupos em que o usuário é membro
func (h *GroupHandler) GetJoinedGroups(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Submit task to worker (no payload needed)
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetJoinedGroups, nil)
	if err != nil {
		logger.Error("Falha ao obter lista de grupos", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter lista de grupos", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// AddParticipants adiciona participantes a um grupo
func (h *GroupHandler) AddParticipants(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req AddParticipantsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupParticipantsPayload{
		GroupJID:     req.GroupJID,
		Participants: req.Participants,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdAddGroupParticipants, payload)
	if err != nil {
		logger.Error("Falha ao adicionar participantes",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao adicionar participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Participantes adicionados com sucesso",
	})
}

// RemoveParticipants remove participantes de um grupo
func (h *GroupHandler) RemoveParticipants(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req RemoveParticipantsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupParticipantsPayload{
		GroupJID:     req.GroupJID,
		Participants: req.Participants,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdRemoveGroupParticipants, payload)
	if err != nil {
		logger.Error("Falha ao remover participantes",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao remover participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Participantes removidos com sucesso",
	})
}

// PromoteParticipants promove participantes a admins
func (h *GroupHandler) PromoteParticipants(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req PromoteParticipantsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupParticipantsPayload{
		GroupJID:     req.GroupJID,
		Participants: req.Participants,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdPromoteGroupParticipants, payload)
	if err != nil {
		logger.Error("Falha ao promover participantes",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao promover participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Participantes promovidos com sucesso",
	})
}

// DemoteParticipants rebaixa admins para participantes comuns
func (h *GroupHandler) DemoteParticipants(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req DemoteParticipantsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupParticipantsPayload{
		GroupJID:     req.GroupJID,
		Participants: req.Participants,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdDemoteGroupParticipants, payload)
	if err != nil {
		logger.Error("Falha ao rebaixar participantes",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao rebaixar participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Participantes rebaixados com sucesso",
	})
}

// UpdateGroupName atualiza o nome do grupo
func (h *GroupHandler) UpdateGroupName(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateGroupNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.UpdateGroupNamePayload{
		GroupJID: req.GroupJID,
		NewName:  req.NewName,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateGroupName, payload)
	if err != nil {
		logger.Error("Falha ao atualizar nome do grupo",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar nome do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Nome do grupo atualizado com sucesso",
	})
}

// UpdateGroupTopic atualiza o tópico/descrição do grupo
func (h *GroupHandler) UpdateGroupTopic(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req UpdateGroupTopicRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.UpdateGroupTopicPayload{
		GroupJID: req.GroupJID,
		NewTopic: req.NewTopic,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdUpdateGroupTopic, payload)
	if err != nil {
		logger.Error("Falha ao atualizar tópico do grupo",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar tópico do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Tópico do grupo atualizado com sucesso",
	})
}

// LeaveGroup sai de um grupo
func (h *GroupHandler) LeaveGroup(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req LeaveGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.LeaveGroupPayload{
		GroupJID: req.GroupJID,
	}

	// Submit task to worker
	_, err := h.submitWorkerTask(userIDStr, worker.CmdLeaveGroup, payload)
	if err != nil {
		logger.Error("Falha ao sair do grupo",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao sair do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Saiu do grupo com sucesso",
	})
}

// JoinGroupWithLink entra em um grupo usando um link de convite
func (h *GroupHandler) JoinGroupWithLink(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req JoinGroupWithLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.JoinGroupWithLinkPayload{
		Link: req.Link,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdJoinGroupWithLink, payload)
	if err != nil {
		logger.Error("Falha ao entrar no grupo via link", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao entrar no grupo via link", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "Entrou no grupo com sucesso",
	})
}

// GetGroupInviteLink obtém o link de convite de um grupo
func (h *GroupHandler) GetGroupInviteLink(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req GetInviteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupInviteLinkPayload{
		GroupJID: req.GroupJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdGetGroupInviteLink, payload)
	if err != nil {
		logger.Error("Falha ao obter link de convite",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter link de convite", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"invite_link": result,
	})
}

// RevokeGroupInviteLink revoga o link atual e gera um novo
func (h *GroupHandler) RevokeGroupInviteLink(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req RevokeInviteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Create payload
	payload := worker.GroupInviteLinkPayload{
		GroupJID: req.GroupJID,
	}

	// Submit task to worker
	result, err := h.submitWorkerTask(userIDStr, worker.CmdRevokeGroupInviteLink, payload)
	if err != nil {
		logger.Error("Falha ao revogar link de convite",
			"error", err,
			"user_id", userIDStr,
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao revogar link de convite", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"invite_link": result,
		"message":     "Link de convite revogado e novo gerado com sucesso",
	})
}
