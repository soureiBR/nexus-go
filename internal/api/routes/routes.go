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
) {// internal/api/routes/routes.go
	package routes
	
	import (
		"github.com/gin-gonic/gin"
		"yourproject/internal/api/handlers"
		"yourproject/internal/api/middlewares"
	)
	
	func SetupRoutes(
		r *gin.Engine, 
		sessionHandler *handlers.SessionHandler,
		messageHandler *handlers.MessageHandler,
		webhookHandler *handlers.WebhookHandler,
		groupHandler *handlers.GroupHandler,
		newsletterHandler *handlers.NewsletterHandler,
		communityHandler *handlers.CommunityHandler,
		authMiddleware middlewares.AuthMiddleware,
	) {
		// Middleware global
		r.Use(middlewares.Logger())
		
		// Grupo de rotas para API v1
		v1 := r.Group("/api/v1")
		v1.Use(authMiddleware.Authenticate())
		
		// Rotas de sessão
		session := v1.Group("/session")
		{
			session.POST("/create", sessionHandler.CreateSession)
			session.GET("/:id", sessionHandler.GetSession)
			session.GET("/:id/qr", sessionHandler.GetQRCode)
			session.POST("/:id/connect", sessionHandler.ConnectSession)
			session.DELETE("/:id", sessionHandler.DeleteSession)
		}
		
		// Rotas de mensagem
		message := v1.Group("/message")
		{
			message.POST("/text", messageHandler.SendText)
			message.POST("/media", messageHandler.SendMedia)
			message.POST("/buttons", messageHandler.SendButtons)
			message.POST("/list", messageHandler.SendList)
			// Outras rotas de mensagem...
		}
		
		// Rotas de grupos
		group := v1.Group("/group")
		{
			group.POST("/create", groupHandler.CreateGroup)
			group.GET("/info", groupHandler.GetGroupInfo)
			group.GET("/list", groupHandler.GetJoinedGroups)
			group.POST("/participants/add", groupHandler.AddParticipants)
			group.POST("/participants/remove", groupHandler.RemoveParticipants)
			group.POST("/participants/promote", groupHandler.PromoteParticipants)
			group.POST("/participants/demote", groupHandler.DemoteParticipants)
			group.POST("/update/name", groupHandler.UpdateGroupName)
			group.POST("/update/topic", groupHandler.UpdateGroupTopic)
			group.POST("/leave", groupHandler.LeaveGroup)
			group.POST("/join", groupHandler.JoinGroupWithLink)
			group.GET("/invite-link", groupHandler.GetGroupInviteLink)
			group.POST("/invite-link/revoke", groupHandler.RevokeGroupInviteLink)
		}
		
		// Rotas de newsletter
		newsletter := v1.Group("/newsletter")
		{
			newsletter.POST("/create", newsletterHandler.CreateNewsletter)
			newsletter.GET("/:id", newsletterHandler.GetNewsletter)
			newsletter.GET("/list", newsletterHandler.ListNewsletters)
			newsletter.POST("/schedule", newsletterHandler.ScheduleNewsletter)
			newsletter.POST("/send", newsletterHandler.SendNewsletter)
			newsletter.POST("/cancel", newsletterHandler.CancelNewsletter)
			newsletter.DELETE("/:id", newsletterHandler.DeleteNewsletter)
			newsletter.GET("/:id/reports", newsletterHandler.GetDeliveryReports)
		}
		
		// Rotas de comunidade
		community := v1.Group("/community")
		{
			community.POST("/create", communityHandler.CreateCommunity)
			community.GET("/info", communityHandler.GetCommunityInfo)
			community.GET("/list", communityHandler.GetJoinedCommunities)
			community.POST("/update/name", communityHandler.UpdateCommunityName)
			community.POST("/update/description", communityHandler.UpdateCommunityDescription)
			community.POST("/leave", communityHandler.LeaveCommunity)
			community.POST("/group/create", communityHandler.CreateGroupForCommunity)
			community.POST("/group/link", communityHandler.LinkGroupToCommunity)
			community.POST("/group/unlink", communityHandler.UnlinkGroupFromCommunity)
			community.POST("/join", communityHandler.JoinCommunityWithLink)
			community.GET("/invite-link", communityHandler.GetCommunityInviteLink)
			community.POST("/invite-link/revoke", communityHandler.RevokeCommunityInviteLink)
			community.POST("/announcement", communityHandler.SendCommunityAnnouncement)
		}
		
		// Configuração de webhook
		v1.POST("/webhook/configure", webhookHandler.Configure)
		v1.GET("/webhook/status", webhookHandler.GetStatus)
		v1.POST("/webhook/test", webhookHandler.TestWebhook)
	}
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