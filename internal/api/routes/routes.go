// internal/api/routes/routes.go
package routes

import (
	"github.com/gin-gonic/gin"
	"yourproject/internal/api/handlers"
	"yourproject/internal/api/middlewares"
)

// SetupRoutes configura todas as rotas da API
func SetupRoutes(
	r *gin.Engine,
	sessionHandler *handlers.SessionHandler,
	messageHandler *handlers.MessageHandler,
	webhookHandler *handlers.WebhookHandler,
	authMiddleware *middlewares.AuthMiddleware,
) {
	// Grupo de rotas para API v1
	v1 := r.Group("/api/v1")
	
	// Aplicar middleware de autenticação
	v1.Use(authMiddleware.Authenticate())
	
	// Rotas de sessão
	session := v1.Group("/session")
	{
		// Criar nova sessão
		session.POST("/create", sessionHandler.CreateSession)
		
		// Obter informações da sessão
		session.GET("/:id", sessionHandler.GetSession)
		
		// Obter QR code para autenticação
		session.GET("/:id/qr", sessionHandler.GetQRCode)
		
		// Conectar sessão existente
		session.POST("/:id/connect", sessionHandler.ConnectSession)
		
		// Encerrar e remover sessão
		session.DELETE("/:id", sessionHandler.DeleteSession)
	}
	
	// Rotas de mensagem
	message := v1.Group("/message")
	{
		// Enviar mensagem de texto
		message.POST("/text", messageHandler.SendText)
		
		// Enviar mídia (imagem, vídeo, etc.)
		message.POST("/media", messageHandler.SendMedia)
		
		// Enviar mensagem com botões
		message.POST("/buttons", messageHandler.SendButtons)
		
		// Enviar mensagem com lista
		message.POST("/list", messageHandler.SendList)
		
		// Enviar mensagem de template (não implementado)
		message.POST("/template", messageHandler.SendTemplate)
	}
	
	// Rotas de webhook
	webhook := v1.Group("/webhook")
	{
		// Configurar URL de webhook
		webhook.POST("/configure", webhookHandler.Configure)
		
		// Verificar status do webhook
		webhook.GET("/status", webhookHandler.Status)
		
		// Testar webhook
		webhook.POST("/test", webhookHandler.Test)
	}
	
	// Rota para documentação da API (opcional)
	r.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/swagger/index.html")
	})
	
	// Rota para versão da API
	v1.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version": "1.0.0",
			"name":    "WhatsApp API",
			"build":   time.Now().Format("20060102"),
		})
	})
}