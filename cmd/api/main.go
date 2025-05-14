// cmd/api/main.go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	
	"github.com/gin-gonic/gin"
	
	"yourproject/internal/api/handlers"
	"yourproject/internal/api/middlewares"
	"yourproject/internal/api/routes"
	"yourproject/internal/config"
	"yourproject/internal/services/newsletter"
	"yourproject/internal/services/webhook"
	"yourproject/internal/services/whatsapp"
	"yourproject/internal/storage"
	"yourproject/pkg/logger"
)

func main() {
	// Carregar configurações
	cfg := config.LoadConfig()
	
	// Configurar logger
	logger.Setup(cfg.LogLevel)
	
	// Configurar diretório de sessões
	sessionDir := filepath.Join(".", "sessions")
	if cfg.SessionDir != "" {
		sessionDir = cfg.SessionDir
	}
	
	// Inicializar armazenamento de arquivo
	fileStore, err := storage.NewFileStore(sessionDir)
	if err != nil {
		log.Fatalf("Falha ao configurar armazenamento de sessões: %v", err)
	}
	
	// Inicializar gerenciador de sessões
	sessionManager := whatsapp.NewSessionManager(fileStore)
	
	// Configurar webhook
	webhookService := webhook.NewDispatcher(cfg.WebhookURL)
	
	// Inicializar serviço de newsletter
	newsletterService, err := newsletter.NewNewsletterService(sessionManager, fileStore, sessionDir)
	if err != nil {
		log.Fatalf("Falha ao inicializar serviço de newsletter: %v", err)
	}
	
	// Registrar handlers de eventos
	sessionManager.RegisterEventHandler("message", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "message", evt)
	})
	sessionManager.RegisterEventHandler("connected", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "connected", evt)
	})
	sessionManager.RegisterEventHandler("disconnected", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "disconnected", evt)
	})
	sessionManager.RegisterEventHandler("logged_out", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "logged_out", evt)
	})
	
	// Configurar handlers HTTP
	sessionHandler := handlers.NewSessionHandler(sessionManager)
	messageHandler := handlers.NewMessageHandler(sessionManager)
	webhookHandler := handlers.NewWebhookHandler(webhookService)
	groupHandler := handlers.NewGroupHandler(sessionManager)
	newsletterHandler := handlers.NewNewsletterHandler(newsletterService)
	communityHandler := handlers.NewCommunityHandler(sessionManager)
	
	// Configurar middleware de autenticação
	authMiddleware := middlewares.NewAuthMiddleware(cfg.APIKey)
	
	// Configurar servidor HTTP
	r := gin.Default()
	routes.SetupRoutes(
		r, 
		sessionHandler, 
		messageHandler, 
		webhookHandler,
		groupHandler,
		newsletterHandler,
		communityHandler,
		authMiddleware,
	)
	
	// Iniciar servidor com graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}
	
	// Iniciar servidor em goroutine
	go func() {
		log.Printf("Iniciando servidor na porta %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro ao iniciar servidor: %v", err)
		}
	}()
	
	// Configurar graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	log.Println("Desligando servidor...")
	
	// Desconectar todas as sessões antes de encerrar
	sessionManager.DisconnectAll()
	
	// Encerrar servidor com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Erro ao desligar servidor: %v", err)
	}
	
	log.Println("Servidor encerrado com sucesso")
}