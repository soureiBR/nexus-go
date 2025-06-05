// internal/api/handlers/session.go
package handlers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdp/qrterminal/v3"
	"github.com/skip2/go-qrcode"

	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

// SessionHandler gerencia endpoints para operações com sessões
type SessionHandler struct {
	sessionManager *whatsapp.SessionManager
}

// SessionResponse representa a resposta de uma operação com sessão
type SessionResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Connected  bool   `json:"connected"`
	CreatedAt  string `json:"created_at"`
	LastActive string `json:"last_active,omitempty"`
	Picture    string `json:"picture,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// NewSessionHandler cria um novo handler para sessões
func NewSessionHandler(sm *whatsapp.SessionManager) *SessionHandler {
	return &SessionHandler{
		sessionManager: sm,
	}
}

// CreateSession cria uma nova sessão de WhatsApp
func (h *SessionHandler) CreateSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Criar sessão
	client, err := h.sessionManager.CreateSession(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao criar sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao criar sessão", "details": err.Error()})
		return
	}

	// Verificar se cliente já está conectado
	status := "created"
	if client.Connected && client.WAClient.Store.ID != nil {
		status = "connected"
	} else if client.WAClient.Store.ID != nil {
		status = "authenticated_disconnected"
	}

	resp := SessionResponse{
		ID:        userIDStr,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}

	if !client.LastActive.IsZero() {
		resp.LastActive = client.LastActive.Format(time.RFC3339)
	}

	// Obter foto de perfil se o cliente estiver conectado
	if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
		myJID := client.WAClient.Store.ID.ToNonAD()
		pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
		if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
			resp.Picture = pictureInfo.URL
		}
	}

	c.JSON(http.StatusCreated, resp)
}

// GetSession retorna informações sobre uma sessão existente
func (h *SessionHandler) GetSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Sessão não encontrada"})
		return
	}

	// Determinar status
	status := "disconnected"
	if client.Connected && client.WAClient.Store.ID != nil {
		// Only consider connected if authenticated (has Store.ID)
		status = "connected"
	} else if client.WAClient.Store.ID != nil {
		// Has authentication but not connected
		status = "authenticated_disconnected"
	} else {
		// Not authenticated
		status = "not_authenticated"
	}

	resp := SessionResponse{
		ID:        userIDStr,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}

	if !client.LastActive.IsZero() {
		resp.LastActive = client.LastActive.Format(time.RFC3339)
	}

	// Obter foto de perfil se o cliente estiver conectado
	if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
		ownJID := client.WAClient.Store.ID
        if !ownJID.IsEmpty() {
            phoneNumber := ownJID.User // This contains your phone number
            resp.PhoneNumber = phoneNumber
        }
		myJID := client.WAClient.Store.ID.ToNonAD()
		pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
		if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
			resp.Picture = pictureInfo.URL
		}
	}

	c.JSON(http.StatusOK, resp)
}

// GetAllSessions retorna informações sobre todas as sessões ativas
func (h *SessionHandler) GetAllSessions(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)
	
	// Para esta implementação, podemos optar por:
	// 1. Retornar apenas a sessão do usuário autenticado (mais seguro)
	// 2. Retornar todas as sessões (apenas para admin/debug)
	
	// Opção 1: Retornar apenas a sessão do usuário atual
	client, exists := h.sessionManager.GetSession(userIDStr)
	if !exists {
		c.JSON(http.StatusOK, gin.H{
			"sessions": []interface{}{},
			"count":    0,
		})
		return
	}

	// Determinar status
	status := "disconnected"
	if client.Connected && client.WAClient.Store.ID != nil {
		status = "connected"
	} else if client.WAClient.Store.ID != nil {
		status = "authenticated_disconnected"
	} else {
		status = "not_authenticated"
	}

	sessionResp := SessionResponse{
		ID:        userIDStr,
		Status:    status,
		Connected: client.Connected,
		CreatedAt: client.CreatedAt.Format(time.RFC3339),
	}

	if !client.LastActive.IsZero() {
		sessionResp.LastActive = client.LastActive.Format(time.RFC3339)
	}

	// Obter informações adicionais se conectado
	if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
		ownJID := client.WAClient.Store.ID
		if !ownJID.IsEmpty() {
			phoneNumber := ownJID.User
			sessionResp.PhoneNumber = phoneNumber
		}
		myJID := client.WAClient.Store.ID.ToNonAD()
		pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
		if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
			sessionResp.Picture = pictureInfo.URL
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": []SessionResponse{sessionResp},
		"count":    1,
	})
}

// GetAllSessionsAdmin retorna todas as sessões (apenas para administradores)
func (h *SessionHandler) GetAllSessionsAdmin(c *gin.Context) {
	// Esta função pode ser implementada para administradores
	// que precisam ver todas as sessões do sistema
	
	allSessions := h.sessionManager.GetAllSessions()
	
	var sessions []SessionResponse
	for userID, client := range allSessions {
		status := "disconnected"
		if client.Connected && client.WAClient.Store.ID != nil {
			status = "connected"
		} else if client.WAClient.Store.ID != nil {
			status = "authenticated_disconnected"
		} else {
			status = "not_authenticated"
		}

		sessionResp := SessionResponse{
			ID:        userID,
			Status:    status,
			Connected: client.Connected,
			CreatedAt: client.CreatedAt.Format(time.RFC3339),
		}

		if !client.LastActive.IsZero() {
			sessionResp.LastActive = client.LastActive.Format(time.RFC3339)
		}

		// Obter informações adicionais se conectado
		if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
			ownJID := client.WAClient.Store.ID
			if !ownJID.IsEmpty() {
				phoneNumber := ownJID.User
				sessionResp.PhoneNumber = phoneNumber
			}
			myJID := client.WAClient.Store.ID.ToNonAD()
			pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
			if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
				sessionResp.Picture = pictureInfo.URL
			}
		}

		sessions = append(sessions, sessionResp)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"count":    len(sessions),
	})
}

// BulkSessionStatusRequest representa a estrutura para requisição de status em lote
type BulkSessionStatusRequest struct {
	UserIDs []string `json:"user_ids" binding:"required"`
}

// BulkSessionStatusResponse representa a resposta de status em lote
type BulkSessionStatusResponse struct {
	Sessions []SessionResponse `json:"sessions"`
	Count    int               `json:"count"`
	NotFound []string          `json:"not_found,omitempty"`
}

// GetBulkSessionStatus retorna o status de múltiplas sessões especificadas
func (h *SessionHandler) GetBulkSessionStatus(c *gin.Context) {
	var req BulkSessionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validar se há userIDs para processar
	if len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one user_id must be provided"})
		return
	}

	// Limitar o número de sessões que podem ser consultadas de uma vez
	const maxBulkSize = 50
	if len(req.UserIDs) > maxBulkSize {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Too many user_ids. Maximum allowed: %d", maxBulkSize),
		})
		return
	}

	var sessions []SessionResponse
	var notFound []string

	// Processar cada userID
	for _, userID := range req.UserIDs {
		client, exists := h.sessionManager.GetSession(userID)
		if !exists {
			notFound = append(notFound, userID)
			continue
		}

		// Determinar status
		status := "disconnected"
		if client.Connected && client.WAClient.Store.ID != nil {
			status = "connected"
		} else if client.WAClient.Store.ID != nil {
			status = "authenticated_disconnected"
		} else {
			status = "not_authenticated"
		}

		sessionResp := SessionResponse{
			ID:        userID,
			Status:    status,
			Connected: client.Connected,
			CreatedAt: client.CreatedAt.Format(time.RFC3339),
		}

		if !client.LastActive.IsZero() {
			sessionResp.LastActive = client.LastActive.Format(time.RFC3339)
		}

		// Obter informações adicionais se conectado
		if client.Connected && client.WAClient != nil && client.WAClient.Store.ID != nil {
			ownJID := client.WAClient.Store.ID
			if !ownJID.IsEmpty() {
				phoneNumber := ownJID.User
				sessionResp.PhoneNumber = phoneNumber
			}
			myJID := client.WAClient.Store.ID.ToNonAD()
			pictureInfo, err := client.WAClient.GetProfilePictureInfo(myJID, nil)
			if err == nil && pictureInfo != nil && pictureInfo.URL != "" {
				sessionResp.Picture = pictureInfo.URL
			}
		}

		sessions = append(sessions, sessionResp)
	}

	response := BulkSessionStatusResponse{
		Sessions: sessions,
		Count:    len(sessions),
	}

	// Incluir não encontrados apenas se houver
	if len(notFound) > 0 {
		response.NotFound = notFound
	}

	c.JSON(http.StatusOK, response)
}

// GetQRCode gera um QR code para autenticação
func (h *SessionHandler) GetQRCode(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Check if there's already an active session (authenticated and connected)
	// Block QR generation to prevent overriding existing sessions
	existingClient, exists := h.sessionManager.GetSession(userIDStr)
	if exists && existingClient.Connected && existingClient.WAClient.Store.ID != nil {
		// Active session exists - do not allow QR generation
		c.JSON(http.StatusConflict, gin.H{
			"error":      "Sessão ativa já existe",
			"message":    "Já existe uma sessão autenticada e conectada. Desconecte a sessão atual antes de gerar um novo QR code.",
			"status":     "connected",
			"connected":  true,
			"session_id": userIDStr,
		})
		return
	}

	// Clean up any non-active session state before generating new QR
	// This ensures we don't have conflicting goroutines from previous QR attempts
	if exists {
		logger.Info("Limpando estado de sessão não-ativa antes de gerar novo QR", "user_id", userIDStr)
		if err := h.sessionManager.ResetSession(c.Request.Context(), userIDStr); err != nil {
			logger.Warn("Falha ao resetar sessão existente", "error", err, "user_id", userIDStr)
			// Continue anyway - this might be the first QR request
		}
	}

	// Verificar se já existe uma sessão e se está autenticada
	client, exists := h.sessionManager.GetSession(userIDStr)
	if exists && client.WAClient.Store.ID != nil {
		if !client.Connected {
			// Sessão autenticada mas desconectada - resetar automaticamente
			logger.Info("Resetando sessão autenticada mas desconectada", "user_id", userIDStr)
			if err := h.sessionManager.ResetSession(c.Request.Context(), userIDStr); err != nil {
				logger.Error("Falha ao resetar sessão", "error", err, "user_id", userIDStr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao resetar sessão", "details": err.Error()})
				return
			}
		} else {
			// Sessão autenticada E conectada - não permitir gerar novo QR
			c.JSON(http.StatusBadRequest, gin.H{
				"error":     "Sessão já está autenticada e conectada",
				"connected": true,
			})
			return
		}
	}

	// Criar contexto com timeout mais longo para permitir o escaneamento
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Obter canal de QR
	qrChan, err := h.sessionManager.GetQRChannel(ctx, userIDStr)
	if err != nil {
		logger.Error("Falha ao obter canal QR", "error", err, "user_id", userIDStr)
		
		// Check if it's a concurrent request error
		if strings.Contains(err.Error(), "QR request already in progress") {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "QR request em andamento",
				"message": "Já existe uma solicitação de QR code em andamento para esta sessão. Aguarde a conclusão da solicitação atual.",
				"code":    "QR_REQUEST_IN_PROGRESS",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao gerar QR code", "details": err.Error()})
		}
		return
	}

	// Iniciar processamento de stream como SSE (Server-Sent Events)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Enviar um evento inicial para confirmar conexão
	c.SSEvent("status", gin.H{"message": "Aguardando QR code..."})
	c.Writer.Flush()

	// Função para verificar estado de conexão periodicamente
	connectionCheckTicker := time.NewTicker(5 * time.Second)
	defer connectionCheckTicker.Stop()

	// Context to handle client disconnection
	clientGone := c.Request.Context().Done()

	// Monitorar o canal do cliente para enviar atualizações
	for {
		select {
		case evt, ok := <-qrChan:
			if !ok {
				// Canal fechado
				c.SSEvent("error", gin.H{"message": "Canal de QR code fechado"})
				c.Writer.Flush()
				return
			}

			logger.Info("Evento QR recebido", "event", evt.Event, "user_id", userIDStr)

			if evt.Event == "code" {
				// Imprimir QR no terminal para debug
				fmt.Println("\n===== QR CODE para sessão", userIDStr, "=====")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("\nEscaneie o código acima com o seu WhatsApp")

				// Gerar QR para cliente
				qrImg, err := qrcode.Encode(evt.Code, qrcode.Medium, 256)
				if err != nil {
					c.SSEvent("error", gin.H{"message": "Falha ao gerar QR code"})
					c.Writer.Flush()
					return
				}

				// Enviar QR code como evento SSE
				qrBase64 := base64.StdEncoding.EncodeToString(qrImg)
				c.SSEvent("qrcode", gin.H{
					"qrcode": qrBase64,
					"data":   evt.Code,
				})
				c.Writer.Flush()
			} else if evt.Event == "success" {
				// Login realizado com sucesso
				c.SSEvent("success", gin.H{"message": "Login realizado com sucesso"})
				c.Writer.Flush()
				return
			} else {
				// Outros eventos (timeout, etc)
				c.SSEvent("status", gin.H{"event": evt.Event})
				c.Writer.Flush()
			}

		case <-connectionCheckTicker.C:
			// Verificar se o cliente está conectado a cada 5 segundos
			updatedClient, exists := h.sessionManager.GetSession(userIDStr)
			if exists && updatedClient.Connected && updatedClient.WAClient.Store.ID != nil {
				c.SSEvent("success", gin.H{
					"message": "Cliente autenticado e conectado",
				})
				c.Writer.Flush()
				return
			}

		case <-clientGone:
			// Client disconnected - clean up session to prevent goroutine conflicts
			logger.Info("Cliente SSE desconectado, limpando sessão", "user_id", userIDStr)
			if err := h.sessionManager.ResetSession(context.Background(), userIDStr); err != nil {
				logger.Error("Falha ao limpar sessão após desconexão SSE", "error", err, "user_id", userIDStr)
			}
			return

		case <-ctx.Done():
			// Timeout geral
			c.SSEvent("error", gin.H{"message": "Timeout ao aguardar QR code"})
			c.Writer.Flush()
			// Clean up session after timeout
			if err := h.sessionManager.ResetSession(context.Background(), userIDStr); err != nil {
				logger.Error("Falha ao limpar sessão após timeout", "error", err, "user_id", userIDStr)
			}
			return
		}
	}
}

// DeleteSession encerra uma sessão
func (h *SessionHandler) DeleteSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Excluir sessão
	if err := h.sessionManager.DeleteSession(c.Request.Context(), userIDStr); err != nil {
		logger.Error("Falha ao excluir sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao excluir sessão", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sessão encerrada com sucesso"})
}

// ConnectSession inicia a conexão com o WhatsApp
func (h *SessionHandler) ConnectSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Iniciar conexão
	err := h.sessionManager.Connect(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao conectar sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao conectar ao WhatsApp", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conectando ao WhatsApp"})
}

// DisconnectSession encerra a conexão com o WhatsApp
func (h *SessionHandler) DisconnectSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Encerrar conexão
	err := h.sessionManager.Disconnect(userIDStr)
	if err != nil {
		logger.Error("Falha ao desconectar sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao desconectar do WhatsApp", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Desconectado do WhatsApp"})
}

// LogoutSession encerra e remove a sessão do WhatsApp
func (h *SessionHandler) LogoutSession(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User ID not found in context"})
		return
	}

	userIDStr := userID.(string)

	// Fazer logout da sessão
	err := h.sessionManager.Logout(c.Request.Context(), userIDStr)
	if err != nil {
		logger.Error("Falha ao fazer logout da sessão", "error", err, "user_id", userIDStr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Falha ao fazer logout do WhatsApp", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logout realizado com sucesso"})
}
