// internal/api/handlers/webhook.go
package handlers

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	
	"yourproject/internal/services/webhook"
	"yourproject/pkg/logger"
)

type WebhookHandler struct {
	webhookService *webhook.Dispatcher
}

type ConfigureWebhookRequest struct {
	URL             string   `json:"url" binding:"required,url"`
	EnabledEvents   []string `json:"enabled_events"`
	Secret          string   `json:"secret"`
}

type WebhookStatusResponse struct {
	URL             string   `json:"url"`
	EnabledEvents   []string `json:"enabled_events"`
	Connected       bool     `json:"connected"`
	LastError       string   `json:"last_error,omitempty"`
	LastSuccessful  string   `json:"last_successful,omitempty"`
}

func NewWebhookHandler(ws *webhook.Dispatcher) *WebhookHandler {
	return &WebhookHandler{
		webhookService: ws,
	}
}

// Configure configura a URL do webhook e eventos habilitados
func (h *WebhookHandler) Configure(c *gin.Context) {
	var req ConfigureWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Lista de eventos válidos
	validEvents := map[string]bool{
		"message":      true,
		"connected":    true,
		"disconnected": true,
		"qr":           true,
		"logged_out":   true,
	}
	
	// Verificar se os eventos são válidos
	enabledEvents := make([]string, 0)
	for _, evt := range req.EnabledEvents {
		if validEvents[evt] {
			enabledEvents = append(enabledEvents, evt)
		}
	}
	
	// Se nenhum evento válido for fornecido, habilitar todos
	if len(enabledEvents) == 0 {
		enabledEvents = []string{"message", "connected", "disconnected", "qr", "logged_out"}
	}
	
	// Configurar webhook
	err := h.webhookService.Configure(req.URL, enabledEvents, req.Secret)
	if err != nil {
		logger.Error("Falha ao configurar webhook", "error", err, "url", req.URL)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao configurar webhook", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Webhook configurado com sucesso",
		"url":     req.URL,
		"events":  enabledEvents,
	})
}

// Status retorna o status atual do webhook
func (h *WebhookHandler) Status(c *gin.Context) {
	status := h.webhookService.GetStatus()
	
	response := WebhookStatusResponse{
		URL:           status.URL,
		EnabledEvents: status.EnabledEvents,
		Connected:     status.Connected,
	}
	
	if status.LastError != "" {
		response.LastError = status.LastError
	}
	
	if !status.LastSuccessful.IsZero() {
		response.LastSuccessful = status.LastSuccessful.Format(http.TimeFormat)
	}
	
	c.JSON(http.StatusOK, response)
}

// Test envia um evento de teste para o webhook configurado
func (h *WebhookHandler) Test(c *gin.Context) {
	if !h.webhookService.IsConfigured() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Webhook não configurado"})
		return
	}
	
	// Evento de teste
	testEvent := map[string]interface{}{
		"type": "test",
		"data": map[string]interface{}{
			"message": "Este é um evento de teste",
			"time":    c.Request.URL.Query().Get("time"),
		},
	}
	
	// Enviar evento de teste
	err := h.webhookService.DispatchEvent("test", "test", testEvent)
	if err != nil {
		logger.Error("Falha ao enviar evento de teste", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao enviar evento de teste", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Evento de teste enviado com sucesso"})
}