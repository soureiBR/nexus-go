// internal/api/handlers/newsletter.go
package handlers

import (
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	
	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

// NewsletterHandler gerencia endpoints para operações de newsletter
type NewsletterHandler struct {
	newsletterService *newsletter.NewsletterService
}

// NewNewsletterHandler cria um novo handler de newsletter
func NewNewsletterHandler(ns *newsletter.NewsletterService) *NewsletterHandler {
	return &NewsletterHandler{
		newsletterService: ns,
	}
}

// CreateNewsletterRequest representa a requisição para criar um newsletter
type CreateNewsletterRequest struct {
	Title       string                 `json:"title" binding:"required"`
	Content     string                 `json:"content" binding:"required"`
	MediaURL    string                 `json:"media_url,omitempty"`
	MediaType   string                 `json:"media_type,omitempty"`
	Buttons     []whatsapp.ButtonData  `json:"buttons,omitempty"`
	Recipients  []string               `json:"recipients" binding:"required,min=1"`
	CreatedBy   string                 `json:"created_by" binding:"required"`
	SessionID   string                 `json:"session_id" binding:"required"`
}

// ScheduleNewsletterRequest representa a requisição para agendar um newsletter
type ScheduleNewsletterRequest struct {
	ID             string `json:"id" binding:"required"`
	ScheduledFor   string `json:"scheduled_for" binding:"required"`
}

// SendNewsletterRequest representa a requisição para enviar um newsletter
type SendNewsletterRequest struct {
	ID string `json:"id" binding:"required"`
}

// CancelNewsletterRequest representa a requisição para cancelar um newsletter
type CancelNewsletterRequest struct {
	ID string `json:"id" binding:"required"`
}

// DeleteNewsletterRequest representa a requisição para excluir um newsletter
type DeleteNewsletterRequest struct {
	ID string `json:"id" binding:"required"`
}

// GetNewsletterRequest representa a requisição para obter um newsletter
type GetNewsletterRequest struct {
	ID string `json:"id" binding:"required"`
}

// CreateNewsletter cria um novo newsletter
func (h *NewsletterHandler) CreateNewsletter(c *gin.Context) {
	var req CreateNewsletterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Criar newsletter
	result, err := h.newsletterService.CreateNewsletter(
		req.Title,
		req.Content,
		req.MediaURL,
		req.MediaType,
		req.Buttons,
		req.Recipients,
		req.CreatedBy,
		req.SessionID,
	)
	
	if err != nil {
		logger.Error("Falha ao criar newsletter", "error", err, "created_by", req.CreatedBy)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar newsletter", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, result)
}

// GetNewsletter obtém um newsletter pelo ID
func (h *NewsletterHandler) GetNewsletter(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do newsletter é obrigatório"})
		return
	}
	
	// Obter newsletter
	result, err := h.newsletterService.GetNewsletter(id)
	if err != nil {
		logger.Error("Falha ao obter newsletter", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter newsletter", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, result)
}

// ListNewsletters lista todos os newsletters
func (h *NewsletterHandler) ListNewsletters(c *gin.Context) {
	// Listar newsletters
	results := h.newsletterService.ListNewsletters()
	c.JSON(http.StatusOK, results)
}

// ScheduleNewsletter agenda um newsletter para envio
func (h *NewsletterHandler) ScheduleNewsletter(c *gin.Context) {
	var req ScheduleNewsletterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Parsear data agendada
	scheduledTime, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Formato de data inválido. Use o formato RFC3339", "details": err.Error()})
		return
	}
	
	// Verificar se a data está no futuro
	if scheduledTime.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A data agendada deve estar no futuro"})
		return
	}
	
	// Agendar newsletter
	err = h.newsletterService.ScheduleNewsletter(req.ID, scheduledTime)
	if err != nil {
		logger.Error("Falha ao agendar newsletter", "error", err, "id", req.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao agendar newsletter", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Newsletter agendado com sucesso", "scheduled_for": scheduledTime})
}

// SendNewsletter envia um newsletter imediatamente
func (h *NewsletterHandler) SendNewsletter(c *gin.Context) {
	var req SendNewsletterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Enviar newsletter
	err := h.newsletterService.SendNewsletter(req.ID)
	if err != nil {
		logger.Error("Falha ao enviar newsletter", "error", err, "id", req.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar newsletter", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Envio de newsletter iniciado"})
}

// CancelNewsletter cancela um newsletter agendado
func (h *NewsletterHandler) CancelNewsletter(c *gin.Context) {
	var req CancelNewsletterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Cancelar newsletter
	err := h.newsletterService.CancelNewsletter(req.ID)
	if err != nil {
		logger.Error("Falha ao cancelar newsletter", "error", err, "id", req.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao cancelar newsletter", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Newsletter cancelado com sucesso"})
}

// DeleteNewsletter exclui um newsletter
func (h *NewsletterHandler) DeleteNewsletter(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do newsletter é obrigatório"})
		return
	}
	
	// Excluir newsletter
	err := h.newsletterService.DeleteNewsletter(id)
	if err != nil {
		logger.Error("Falha ao excluir newsletter", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao excluir newsletter", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Newsletter excluído com sucesso"})
}

// GetDeliveryReports obtém relatórios de entrega de um newsletter
func (h *NewsletterHandler) GetDeliveryReports(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID do newsletter é obrigatório"})
		return
	}
	
	// Obter relatórios de entrega
	reports, err := h.newsletterService.GetDeliveryReports(id)
	if err != nil {
		logger.Error("Falha ao obter relatórios de entrega", "error", err, "id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao obter relatórios de entrega", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, reports)
}