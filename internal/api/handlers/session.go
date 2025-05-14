// internal/api/handlers/session.go
package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/store/rawstore"
	
	"yourproject/internal/models"
	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

type SessionHandler struct {
	sessionManager *whatsapp.SessionManager
}

type CreateSessionRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type SessionResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Connected bool   `json:"connected"`
	CreatedAt string `json:"created_at"`
	LastActive string `json:"last_active,omitempty"`
}

func NewSessionHandler(sm *whatsapp.SessionManager) *SessionHandler {
	return &SessionHandler{
		sessionManager: sm,
	}
}

// CreateSession cria uma nova sessão de WhatsApp
func (h *SessionHandler) CreateSession(c *gin.Context) {
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dados inválidos", "details": err.Error()})
		return
	}
	
	// Criar sessão
	client, err := h.sessionManager.CreateSession(req.UserID)
	if err != nil {
		logger.Error("Falha ao criar sessão", "error", err, "user_id", req.UserID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar sessão", "details": err.Error()})
		return
	}
	
	// Verificar se cliente já está conectado
	status := "created"
	if client.Connected {
		status = "connected"
	}
	
	resp := SessionResponse{
		ID:        req.UserID,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}
	
	if !client.LastActive.IsZero() {
		resp.LastActive = client.LastActive.Format(time.RFC3339)
	}
	
	c.JSON(http.StatusCreated, resp)
}

// GetSession retorna informações sobre uma sessão existente
func (h *SessionHandler) GetSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID da sessão é obrigatório"})
		return
	}
	
	client, exists := h.sessionManager.GetSession(id)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}
	
	// Determinar status
	status := "disconnected"
	if client.Connected {
		status = "connected"
	}
	
	resp := SessionResponse{
		ID:        id,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}
	
	if !client.LastActive.IsZero() {
		resp.LastActive = client.LastActive.Format(time.RFC3339)
	}
	
	c.JSON(http.StatusOK, resp)
}

// GetQRCode gera um QR code para autenticação
func (h *SessionHandler) GetQRCode(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID da sessão é obrigatório"})
		return
	}
	
	client, exists := h.sessionManager.GetSession(id)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}
	
	// Verificar se o cliente já está conectado
	if client.Connected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cliente já conectado"})
		return
	}
	
	// Obter código QR
	qrChan, err := h.sessionManager.GetQRChannel(id)
	if err != nil {
		logger.Error("Falha ao obter canal QR", "error", err, "user_id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao gerar QR code", "details": err.Error()})
		return
	}
	
	select {
	case qrCode := <-qrChan:
		// Gerar imagem QR
		qrImg, err := qrcode.Encode(qrCode, qrcode.Medium, 256)
		if err != nil {
			logger.Error("Falha ao gerar QR code", "error", err, "user_id", id)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao gerar imagem QR", "details": err.Error()})
			return
		}
		
		// Codificar em base64
		qrBase64 := base64.StdEncoding.EncodeToString(qrImg)
		
		c.JSON(http.StatusOK, gin.H{
			"qrcode": qrBase64,
			"data": qrCode,
		})
		
	case <-time.After(60 * time.Second):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "Timeout ao gerar QR code"})
	}
}

// DeleteSession encerra uma sessão
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID da sessão é obrigatório"})
		return
	}
	
	// Verificar se a sessão existe
	if _, exists := h.sessionManager.GetSession(id); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}
	
	// Excluir sessão
	if err := h.sessionManager.DeleteSession(id); err != nil {
		logger.Error("Falha ao excluir sessão", "error", err, "user_id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao excluir sessão", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Sessão encerrada com sucesso"})
}

// ConnectSession inicia a conexão com o WhatsApp
func (h *SessionHandler) ConnectSession(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID da sessão é obrigatório"})
		return
	}
	
	// Verificar se a sessão existe
	if _, exists := h.sessionManager.GetSession(id); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}
	
	// Iniciar conexão
	err := h.sessionManager.Connect(id)
	if err != nil {
		logger.Error("Falha ao conectar sessão", "error", err, "user_id", id)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao conectar ao WhatsApp", "details": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Conectando ao WhatsApp"})
}