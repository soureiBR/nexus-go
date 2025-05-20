// internal/api/handlers/newsletter.go
package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"

	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

// NewsletterHandler gerencia endpoints para operações de canais do WhatsApp
type NewsletterHandler struct {
	newsletterService *whatsapp.NewsletterService
}

// NewNewsletterHandler cria um novo handler de newsletter
func NewNewsletterHandler(ns *whatsapp.NewsletterService) *NewsletterHandler {
	return &NewsletterHandler{
		newsletterService: ns,
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

	// Verificar se há uma imagem enviada
	var picture []byte
	if req.PictureURL != "" {
		var err error
		picture, err = downloadPictureFromURL(req.PictureURL)
		if err != nil {
			logger.Error("Falha ao baixar imagem da URL", "error", err, "url", req.PictureURL)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Falha ao baixar imagem da URL",
				"details": err.Error(),
			})
			return
		}
	}

	// Criar canal
	metadata, err := h.newsletterService.CreateChannel(c.Request.Context(), userIDStr, req.Name, req.Description, picture)
	if err != nil {
		logger.Error("Falha ao criar canal", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, metadata)
}

func downloadPictureFromURL(url string) ([]byte, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make the request
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("erro ao acessar URL: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status da resposta inválido: %d", resp.StatusCode)
	}

	// Check content type to ensure it's an image
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, fmt.Errorf("o conteúdo não é uma imagem: %s", contentType)
	}

	// Read the image data
	return io.ReadAll(resp.Body)
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

	// Converter JID para o formato correto
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JID inválido", "details": err.Error()})
		return
	}

	// Obter informações do canal
	metadata, err := h.newsletterService.GetChannelInfo(c.Request.Context(), userIDStr, jid)
	if err != nil {
		logger.Error("Falha ao obter informações do canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações do canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metadata)
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

	// Obter informações do canal por convite
	metadata, err := h.newsletterService.GetChannelWithInvite(c.Request.Context(), userIDStr, req.InviteLink)
	if err != nil {
		logger.Error("Falha ao obter informações do canal por convite", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter informações do canal por convite", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metadata)
}

// ListMyChannels lista todos os canais que o usuário está inscrito
func (h *NewsletterHandler) ListMyChannels(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	var req ListChannelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}

	// Listar canais inscritos
	channels, err := h.newsletterService.ListMyChannels(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao listar canais inscritos", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao listar canais inscritos", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, channels)
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

	// Converter JID para o formato correto
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JID inválido", "details": err.Error()})
		return
	}

	// Seguir canal
	err = h.newsletterService.FollowChannel(c.Request.Context(), userIDStr, jid)
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

	// Converter JID para o formato correto
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JID inválido", "details": err.Error()})
		return
	}

	// Deixar de seguir canal
	err = h.newsletterService.UnfollowChannel(c.Request.Context(), userIDStr, jid)
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

	// Converter JID para o formato correto
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JID inválido", "details": err.Error()})
		return
	}

	// Silenciar canal
	err = h.newsletterService.MuteChannel(c.Request.Context(), userIDStr, jid)
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

	// Converter JID para o formato correto
	jid, err := types.ParseJID(req.JID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "JID inválido", "details": err.Error()})
		return
	}

	// Reativar notificações do canal
	err = h.newsletterService.UnmuteChannel(c.Request.Context(), userIDStr, jid)
	if err != nil {
		logger.Error("Falha ao reativar notificações do canal", "error", err, "user_id", userIDStr, "jid", req.JID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao reativar notificações do canal", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notificações do canal reativadas com sucesso"})
}
