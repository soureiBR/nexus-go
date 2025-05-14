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
	"yourproject/internal/services/whatsapp"
	"yourproject/internal/services/webhook"
	"yourproject/internal/storage"
	"yourproject/pkg/logger"
)

func main() {
	// Carregar configurações
	cfg := config.LoadConfig()
	
	// Configurar logger
	logger.Setup(cfg.LogLevel)
	
	// Configurar modo do Gin
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	
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
	
	// Iniciar limpeza periódica de sessões
	sessionManager.StartPeriodicCleanup(cfg.CleanupInterval, cfg.MaxInactiveTime)
	
	// Configurar webhook
	webhookService := webhook.NewDispatcher(cfg.WebhookURL)
	if cfg.WebhookURL != "" {
		err := webhookService.Configure(cfg.WebhookURL, []string{"message", "connected", "disconnected", "qr", "logged_out"}, cfg.WebhookSecret)
		if err != nil {
			logger.Warn("Falha ao configurar webhook inicial", "error", err)
		}
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
	
	// Configurar middleware de autenticação
	authMiddleware := middlewares.NewAuthMiddleware(cfg.APIKey)
	
	// Configurar servidor HTTP
	r := gin.New()
	
	// Adicionar middlewares globais
	r.Use(middlewares.Logger())
	r.Use(middlewares.RecoveryWithLogger())
	
	// Configurar limites de upload
	r.MaxMultipartMemory = cfg.MaxUploadSize
	
	// Configurar rotas
	routes.SetupRoutes(r, sessionHandler, messageHandler, webhookHandler, authMiddleware)
	
	// Adicionar rota de healthcheck
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"version":   "1.0.0",
			"timestamp": time.Now().Unix(),
		})
	})
	
	// Iniciar servidor com graceful shutdown
	srv := &http.Server{
		Addr:         cfg.Host + ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  cfg.RequestTimeout,
		WriteTimeout: cfg.RequestTimeout * 2, // Tempo de escrita deve ser maior para permitir uploads
		IdleTimeout:  120 * time.Second,
	}
	
	// Iniciar servidor em goroutine
	go func() {
		logger.Info("Iniciando servidor HTTP", "host", cfg.Host, "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Falha ao iniciar servidor HTTP", "error", err)
			os.Exit(1)
		}
	}()
	
	// Configurar graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	logger.Info("Desligando servidor...")
	
	// Criar contexto com timeout para encerramento
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Desconectar todas as sessões antes de encerrar
	sessionManager.DisconnectAll()
	
	// Encerrar servidor HTTP
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Erro ao desligar servidor HTTP", "error", err)
	}
	
	logger.Info("Servidor encerrado com sucesso")
}