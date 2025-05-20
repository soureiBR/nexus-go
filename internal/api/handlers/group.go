// internal/api/handlers/group.go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"yourproject/internal/services/whatsapp"
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

	// Criar grupo
	group, err := h.sessionManager.CreateGroup(userIDStr, req.Name, req.Participants)
	if err != nil {
		logger.Error("Falha ao criar grupo", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, group)
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

	// Obter informações do grupo
	group, err := h.sessionManager.GetGroupInfo(userIDStr, req.GroupJID)
	if err != nil {
		logger.Error("Falha ao obter informações do grupo", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
}

// GetJoinedGroups obtém lista de grupos em que o usuário é membro
func (h *GroupHandler) GetJoinedGroups(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do usuário é obrigatório"})
		return
	}

	// Obter lista de grupos
	groups, err := h.sessionManager.GetJoinedGroups(userID)
	if err != nil {
		logger.Error("Falha ao obter lista de grupos", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter lista de grupos", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, groups)
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

	// Adicionar participantes
	err := h.sessionManager.AddGroupParticipants(userIDStr, req.GroupJID, req.Participants)
	if err != nil {
		logger.Error("Falha ao adicionar participantes", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao adicionar participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Participantes adicionados com sucesso"})
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

	// Remover participantes
	err := h.sessionManager.RemoveGroupParticipants(userIDStr, req.GroupJID, req.Participants)
	if err != nil {
		logger.Error("Falha ao remover participantes", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao remover participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Participantes removidos com sucesso"})
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

	// Promover participantes
	err := h.sessionManager.PromoteGroupParticipants(userIDStr, req.GroupJID, req.Participants)
	if err != nil {
		logger.Error("Falha ao promover participantes", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao promover participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Participantes promovidos com sucesso"})
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

	// Rebaixar participantes
	err := h.sessionManager.DemoteGroupParticipants(userIDStr, req.GroupJID, req.Participants)
	if err != nil {
		logger.Error("Falha ao rebaixar participantes", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao rebaixar participantes", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Participantes rebaixados com sucesso"})
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

	// Atualizar nome do grupo
	err := h.sessionManager.UpdateGroupName(userIDStr, req.GroupJID, req.NewName)
	if err != nil {
		logger.Error("Falha ao atualizar nome do grupo", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar nome do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Nome do grupo atualizado com sucesso"})
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

	// Atualizar tópico do grupo
	err := h.sessionManager.UpdateGroupTopic(userIDStr, req.GroupJID, req.NewTopic)
	if err != nil {
		logger.Error("Falha ao atualizar tópico do grupo", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar tópico do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Tópico do grupo atualizado com sucesso"})
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

	// Sair do grupo
	err := h.sessionManager.LeaveGroup(userIDStr, req.GroupJID)
	if err != nil {
		logger.Error("Falha ao sair do grupo", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao sair do grupo", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Saiu do grupo com sucesso"})
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

	// Entrar no grupo via link
	group, err := h.sessionManager.JoinGroupWithLink(userIDStr, req.Link)
	if err != nil {
		logger.Error("Falha ao entrar no grupo via link", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao entrar no grupo via link", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
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

	// Obter link de convite
	link, err := h.sessionManager.GetGroupInviteLink(userIDStr, req.GroupJID)
	if err != nil {
		logger.Error("Falha ao obter link de convite", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter link de convite", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"invite_link": link})
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

	// Revogar link atual e obter novo
	link, err := h.sessionManager.RevokeGroupInviteLink(userIDStr, req.GroupJID)
	if err != nil {
		logger.Error("Falha ao revogar link de convite", "error", err, "user_id", userIDStr, "group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao revogar link de convite", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"invite_link": link})
}
