package routes

import (
	"net/http"
	"time"

	"yourproject/internal/api/handlers"
	"yourproject/internal/api/middlewares"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(
	r *gin.Engine,
	sessionHandler *handlers.SessionHandler,
	messageHandler *handlers.MessageHandler,
	webhookHandler *handlers.WebhookHandler,
	groupHandler *handlers.GroupHandler,
	newsletterHandler *handlers.NewsletterHandler,
	communityHandler *handlers.CommunityHandler,
	authHandler *handlers.AuthHandler,
	authMiddleware *middlewares.AuthMiddleware,
) {
	// Middleware global
	r.Use(middlewares.Logger())

	// Rota de health check (sem autenticação)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"service":   "WhatsApp API",
		})
	})

	// Grupo de rotas para API v1
	v1 := r.Group("/api/v1")
	v1.Use(authMiddleware.AuthenticateAndExtractUserID())

	// Rotas de sessão
	session := v1.Group("/session")
	{
		session.POST("/create", sessionHandler.CreateSession)
		session.GET("/", sessionHandler.GetSession)
		session.POST("/bulk-status", sessionHandler.GetBulkSessionStatus)
		session.GET("/qr", sessionHandler.GetQRCode)
		session.POST("/connect", sessionHandler.ConnectSession)
		session.POST("/disconnect", sessionHandler.DisconnectSession)
		session.DELETE("/", sessionHandler.DeleteSession)
	}

	// Rotas admin (requerem chave especial)
	sessionAdmin := v1.Group("/session/admin")
	sessionAdmin.Use(authMiddleware.ValidateAdminKey())
	{
		sessionAdmin.GET("/all", sessionHandler.GetAllSessionsAdmin)
	}

	// Rotas de autenticação admin (requerem chave admin)
	auth := v1.Group("/auth")
	auth.Use(authMiddleware.ValidateAdminKey())
	{
		auth.POST("/session/encrypt", authHandler.EncryptSession)
	}

	// Rotas de mensagem
	message := v1.Group("/message")
	{
		message.POST("/text", messageHandler.SendText)
		message.POST("/media", messageHandler.SendMedia)
		message.POST("/buttons", messageHandler.SendButtons)
		message.POST("/list", messageHandler.SendList)
		message.POST("/template", messageHandler.SendTemplate)
		message.POST("/check-number", messageHandler.CheckNumber)
	}

	// Rotas de grupos
	group := v1.Group("/group")
	{
		group.POST("/create", groupHandler.CreateGroup)
		group.GET("/info", groupHandler.GetGroupInfo)
		group.GET("/list", groupHandler.GetJoinedGroups)
		group.GET("/list/admin", groupHandler.GetAdminGroups)
		group.POST("/participants/add", groupHandler.AddParticipants)
		group.POST("/participants/remove", groupHandler.RemoveParticipants)
		group.POST("/participants/promote", groupHandler.PromoteParticipants)
		group.POST("/participants/demote", groupHandler.DemoteParticipants)
		group.POST("/update/name", groupHandler.UpdateGroupName)
		group.POST("/update/topic", groupHandler.UpdateGroupTopic)
		group.POST("/update/picture", groupHandler.UpdateGroupPicture)
		group.POST("/leave", groupHandler.LeaveGroup)
		group.POST("/join", groupHandler.JoinGroupWithLink)
		group.GET("/invite-link", groupHandler.GetGroupInviteLink)
		group.POST("/invite-link/revoke", groupHandler.RevokeGroupInviteLink)
		group.POST("/set-locked", groupHandler.SetGroupLocked)
		group.POST("/set-announce", groupHandler.SetGroupAnnounce)
		group.POST("/set-join-approval", groupHandler.SetGroupJoinApprovalMode)
		group.POST("/set-member-add-mode", groupHandler.SetGroupMemberAddMode)
	}

	// Rotas de newsletter
	newsletter := v1.Group("/newsletter")
	{
		newsletter.POST("/create", newsletterHandler.CreateChannel)
		newsletter.GET("/info", newsletterHandler.GetChannelInfo)
		newsletter.POST("/info-invite", newsletterHandler.GetChannelWithInvite)
		newsletter.GET("/list", newsletterHandler.ListMyChannels)
		newsletter.POST("/follow", newsletterHandler.FollowChannel)
		newsletter.POST("/unfollow", newsletterHandler.UnfollowChannel)
		newsletter.POST("/mute", newsletterHandler.MuteChannel)
		newsletter.POST("/unmute", newsletterHandler.UnmuteChannel)
		newsletter.POST("/update/picture", newsletterHandler.UpdateNewsletterPicture)
		newsletter.POST("/update/name", newsletterHandler.UpdateNewsletterName)
		newsletter.POST("/update/description", newsletterHandler.UpdateNewsletterDescription)
	}

	// Rotas de comunidade
	community := v1.Group("/community")
	{
		community.POST("/create", communityHandler.CreateCommunity)
		community.GET("/info", communityHandler.GetCommunityInfo)
		community.GET("/list", communityHandler.GetJoinedCommunities)
		community.POST("/update/name", communityHandler.UpdateCommunityName)
		community.POST("/update/description", communityHandler.UpdateCommunityDescription)
		community.POST("/update/picture", communityHandler.UpdateCommunityPicture)
		community.POST("/leave", communityHandler.LeaveCommunity)
		community.POST("/group/create", communityHandler.CreateGroupForCommunity)
		community.POST("/group/link", communityHandler.LinkGroupToCommunity)
		community.POST("/group/unlink", communityHandler.UnlinkGroupFromCommunity)
		community.POST("/join", communityHandler.JoinCommunityWithLink)
		community.GET("/invite-link", communityHandler.GetCommunityInviteLink)
		community.POST("/invite-link/revoke", communityHandler.RevokeCommunityInviteLink)
		community.GET("/linked-groups", communityHandler.GetCommunityLinkedGroups)
	}

	// Configuração de webhook
	webhook := v1.Group("/webhook")
	{
		webhook.POST("/configure", webhookHandler.Configure)
		webhook.GET("/status", webhookHandler.Status)
		webhook.POST("/test", webhookHandler.Test)
		webhook.POST("/enable", webhookHandler.Enable)
		webhook.POST("/disable", webhookHandler.Disable)
	}

	// Rota para versão da API
	v1.GET("/version", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"version": "1.0.0",
			"name":    "WhatsApp API",
			"build":   time.Now().Format("20060102"),
		})
	})
}
