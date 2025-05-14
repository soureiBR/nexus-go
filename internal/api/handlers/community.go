// internal/api/handlers/community.go
package handlers

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	
	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

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

// CreateCommunityRequest representa a requisição para criar uma comunidade
type CreateCommunityRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateCommunityNameRequest representa a requisição para atualizar nome da comunidade
type UpdateCommunityNameRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
	NewName      string `json:"new_name" binding:"required"`
}

// UpdateCommunityDescriptionRequest representa a requisição para atualizar descrição da comunidade
type UpdateCommunityDescriptionRequest struct {
	UserID         string `json:"user_id" binding:"required"`
	CommunityJID   string `json:"community_jid" binding:"required"`
	NewDescription string `json:"new_description" binding:"required"`
}

// CommunityInfoRequest representa a requisição para obter informações da comunidade
type CommunityInfoRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
}

// LeaveCommunityRequest representa a requisição para sair de uma comunidade
type LeaveCommunityRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
}

// CreateGroupForCommunityRequest representa a requisição para criar grupo em uma comunidade
type CreateGroupForCommunityRequest struct {
	UserID       string   `json:"user_id" binding:"required"`
	CommunityJID string   `json:"community_jid" binding:"required"`
	GroupName    string   `json:"group_name" binding:"required"`
	Participants []string `json:"participants" binding:"required,min=1"`
}

// LinkGroupRequest representa a requisição para vincular grupo a comunidade
type LinkGroupRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
	GroupJID     string `json:"group_jid" binding:"required"`
}

// UnlinkGroupRequest representa a requisição para desvincular grupo de comunidade
type UnlinkGroupRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
	GroupJID     string `json:"group_jid" binding:"required"`
}

// JoinCommunityWithLinkRequest representa a requisição para entrar em uma comunidade via link
type JoinCommunityWithLinkRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Link   string `json:"link" binding:"required"`
}

// GetInviteLinkRequest representa a requisição para obter link de convite
type GetCommunityInviteLinkRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
}

// RevokeInviteLinkRequest representa a requisição para revogar link de convite
type RevokeCommunityInviteLinkRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
}

// SendAnnouncementRequest representa a requisição para enviar anúncio para a comunidade
type SendAnnouncementRequest struct {
	UserID       string `json:"user_id" binding:"required"`
	CommunityJID string `json:"community_jid" binding:"required"`
	Message      string `json:"message" binding:"required"`
}

// CreateCommunity cria uma nova comunidade
func (h *CommunityHandler) CreateCommunity(c *gin.Context) {
	var req CreateCommunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Criar comunidade
	community, err := h.sessionManager.CreateCommunity(req.UserID, req.Name, req.Description)
	if err != nil {
		logger.Error("Falha ao criar comunidade", "error", err, "user_id", req.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, community)
}

// GetCommunityInfo obtém informações de uma comunidade
func (h *CommunityHandler) GetCommunityInfo(c *gin.Context) {
	var req CommunityInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Obter informações da comunidade
	community, err := h.sessionManager.GetCommunityInfo(req.UserID, req.CommunityJID)
	if err != nil {
		logger.Error("Falha ao obter informações da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, community)
}

// GetJoinedCommunities obtém lista de comunidades em que o usuário é membro
func (h *CommunityHandler) GetJoinedCommunities(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do usuário é obrigatório"})
		return
	}
	
	// Obter lista de comunidades
	communities, err := h.sessionManager.GetJoinedCommunities(userID)
	if err != nil {
		logger.Error("Falha ao obter lista de comunidades", "error", err, "user_id", userID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter lista de comunidades", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, communities)
}

// UpdateCommunityName atualiza o nome da comunidade
func (h *CommunityHandler) UpdateCommunityName(c *gin.Context) {
	var req UpdateCommunityNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Atualizar nome da comunidade
	err := h.sessionManager.UpdateCommunityName(req.UserID, req.CommunityJID, req.NewName)
	if err != nil {
		logger.Error("Falha ao atualizar nome da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar nome da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Nome da comunidade atualizado com sucesso"})
}

// UpdateCommunityDescription atualiza a descrição da comunidade
func (h *CommunityHandler) UpdateCommunityDescription(c *gin.Context) {
	var req UpdateCommunityDescriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Atualizar descrição da comunidade
	err := h.sessionManager.UpdateCommunityDescription(req.UserID, req.CommunityJID, req.NewDescription)
	if err != nil {
		logger.Error("Falha ao atualizar descrição da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao atualizar descrição da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Descrição da comunidade atualizada com sucesso"})
}

// LeaveCommunity sai de uma comunidade
func (h *CommunityHandler) LeaveCommunity(c *gin.Context) {
	var req LeaveCommunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Sair da comunidade
	err := h.sessionManager.LeaveCommunity(req.UserID, req.CommunityJID)
	if err != nil {
		logger.Error("Falha ao sair da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao sair da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Saiu da comunidade com sucesso"})
}

// CreateGroupForCommunity cria um novo grupo dentro de uma comunidade
func (h *CommunityHandler) CreateGroupForCommunity(c *gin.Context) {
	var req CreateGroupForCommunityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Criar grupo na comunidade
	group, err := h.sessionManager.CreateGroupForCommunity(req.UserID, req.CommunityJID, req.GroupName, req.Participants)
	if err != nil {
		logger.Error("Falha ao criar grupo na comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar grupo na comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, group)
}

// LinkGroupToCommunity vincula um grupo existente a uma comunidade
func (h *CommunityHandler) LinkGroupToCommunity(c *gin.Context) {
	var req LinkGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Vincular grupo à comunidade
	err := h.sessionManager.LinkGroupToCommunity(req.UserID, req.CommunityJID, req.GroupJID)
	if err != nil {
		logger.Error("Falha ao vincular grupo à comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID, 
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao vincular grupo à comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Grupo vinculado à comunidade com sucesso"})
}

// UnlinkGroupFromCommunity desvincula um grupo de uma comunidade
func (h *CommunityHandler) UnlinkGroupFromCommunity(c *gin.Context) {
	var req UnlinkGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Desvincular grupo da comunidade
	err := h.sessionManager.UnlinkGroupFromCommunity(req.UserID, req.CommunityJID, req.GroupJID)
	if err != nil {
		logger.Error("Falha ao desvincular grupo da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID, 
			"group_jid", req.GroupJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao desvincular grupo da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Grupo desvinculado da comunidade com sucesso"})
}

// JoinCommunityWithLink entra em uma comunidade usando um link de convite
func (h *CommunityHandler) JoinCommunityWithLink(c *gin.Context) {
	var req JoinCommunityWithLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Entrar na comunidade via link
	community, err := h.sessionManager.JoinCommunityWithLink(req.UserID, req.Link)
	if err != nil {
		logger.Error("Falha ao entrar na comunidade via link", "error", err, "user_id", req.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao entrar na comunidade via link", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, community)
}

// GetCommunityInviteLink obtém o link de convite de uma comunidade
func (h *CommunityHandler) GetCommunityInviteLink(c *gin.Context) {
	var req GetCommunityInviteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Obter link de convite
	link, err := h.sessionManager.GetCommunityInviteLink(req.UserID, req.CommunityJID)
	if err != nil {
		logger.Error("Falha ao obter link de convite da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter link de convite da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"invite_link": link})
}

// RevokeCommunityInviteLink revoga o link atual e gera um novo
func (h *CommunityHandler) RevokeCommunityInviteLink(c *gin.Context) {
	var req RevokeCommunityInviteLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Revogar link atual e obter novo
	link, err := h.sessionManager.RevokeCommunityInviteLink(req.UserID, req.CommunityJID)
	if err != nil {
		logger.Error("Falha ao revogar link de convite da comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao revogar link de convite da comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"invite_link": link})
}

// SendCommunityAnnouncement envia um anúncio para todos os grupos vinculados a uma comunidade
func (h *CommunityHandler) SendCommunityAnnouncement(c *gin.Context) {
	var req SendAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Enviar anúncio para a comunidade
	err := h.sessionManager.SendCommunityAnnouncement(req.UserID, req.CommunityJID, req.Message)
	if err != nil {
		logger.Error("Falha ao enviar anúncio para a comunidade", 
			"error", err, 
			"user_id", req.UserID, 
			"community_jid", req.CommunityJID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar anúncio para a comunidade", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Anúncio enviado com sucesso"})
}