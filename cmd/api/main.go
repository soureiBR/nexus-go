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
	"yourproject/internal/services/rabbitmq"
	"yourproject/internal/services/rabbitmq/consumers"
	"yourproject/internal/services/webhook"
	"yourproject/internal/services/whatsapp"
	"yourproject/internal/storage"
	"yourproject/pkg/logger"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Configure logger
	logger.Setup(cfg.LogLevel)

	// Configure database path
	dbPath := filepath.Join(".", "data", "whatsapp.db")
	if cfg.DBPath != "" {
		dbPath = cfg.DBPath
	}

	// Initialize SQL storage
	sqlStore, err := storage.NewSQLStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to configure SQL storage: %v", err)
	}
	defer sqlStore.Close()

	// Initialize session manager
	sessionManager := whatsapp.NewSessionManager(sqlStore)

	// Initialize RabbitMQ publisher if configured
	var eventPublisher *rabbitmq.EventPublisher
	var consumerManager *consumers.ConsumerManager
	if cfg.RabbitMQURL != "" {
		logger.Info("Initializing RabbitMQ publisher...", "url", cfg.RabbitMQURL)

		rabbitConfig := rabbitmq.Config{
			URL:          cfg.RabbitMQURL,
			ExchangeName: "whatsapp.events",
			TLSConfig:    nil, // Set to your TLS config if needed
			InsecureSkipVerify: true, // Set to true if you want to disable SSL verification
		}

		eventPublisher, err = rabbitmq.NewEventPublisher(rabbitConfig)
		if err != nil {
			logger.Error("Failed to initialize RabbitMQ publisher", "error", err)
			log.Printf("Warning: RabbitMQ publisher failed to initialize: %v", err)
		} else {
			// Set the publisher in session manager
			sessionManager.SetEventPublisher(eventPublisher)

			// Setup specific queues for WhatsApp events
			queueSetup := rabbitmq.NewQueueSetup(eventPublisher.GetChannel(), "whatsapp.events")
			if err := queueSetup.SetupWhatsAppQueues(); err != nil {
				logger.Error("Failed to setup WhatsApp queues", "error", err)
			} else {
				logger.Info("WhatsApp event queues setup successfully")
			}

			// Initialize consumer manager
			logger.Info("Initializing RabbitMQ consumer manager...")
			consumerManagerConfig := consumers.ConsumerManagerConfig{
				RabbitMQURL:    cfg.RabbitMQURL,
				ExchangeName:   "whatsapp.events",
				SessionManager: nil, // Will be set after sessionManager is ready
				Publisher:      eventPublisher,
			}
			consumerManager = consumers.NewConsumerManager(consumerManagerConfig)
			logger.Info("Consumer manager initialized successfully")
		}
	} else {
		logger.Info("RabbitMQ URL not configured, skipping RabbitMQ initialization")
	}

	// Start the coordinator system
	logger.Info("Iniciando sistema de coordenação...")
	if err := sessionManager.StartCoordinator(); err != nil {
		log.Fatalf("Falha ao iniciar coordinator: %v", err)
	}

	// Create a context for initialization
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load existing sessions
	if err := sessionManager.InitSessions(ctx); err != nil {
		log.Printf("Warning: Failed to load all sessions: %v", err)
	}

	// Auto-initialize workers for all existing sessions using coordinator
	logger.Info("Inicializando workers para todas as sessões usando coordinator...")
	if err := sessionManager.GetCoordinator().AutoInitWorkers(); err != nil {
		log.Printf("Warning: Some workers failed to initialize: %v", err)
	} else {
		logger.Info("Workers inicializados com sucesso para todas as sessões")
	}

	// Initialize RabbitMQ consumers if available
	if consumerManager != nil {
		logger.Info("Registering and starting RabbitMQ consumers...")

		// Update consumer manager config with the session manager (for worker-based consumers)
		consumerManagerConfig := consumers.ConsumerManagerConfig{
			RabbitMQURL:    cfg.RabbitMQURL,
			ExchangeName:   "whatsapp.events",
			SessionManager: sessionManager,
			Publisher:      eventPublisher,
		}

		// Register send message consumer
		if err := consumerManager.RegisterSendMessageConsumer(consumerManagerConfig); err != nil {
			logger.Error("Failed to register send message consumer", "error", err)
		} else {
			logger.Info("Send message consumer registered successfully")
		}

		// Start all consumers
		if err := consumerManager.StartAll(); err != nil {
			logger.Error("Failed to start consumers", "error", err)
		} else {
			logger.Info("All consumers started successfully")
		}
	}

	// Start periodic cleanup for inactive sessions (every 30 minutes, remove sessions inactive for 24 hours)
	sessionManager.StartPeriodicCleanup(30*time.Minute, 24*time.Hour)

	// Configure webhook
	webhookService := webhook.NewDispatcher(cfg.WebhookURL)

	// Register event handlers
	sessionManager.RegisterEventHandler("message", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "message", evt)
	})
	sessionManager.RegisterEventHandler("connected", func(userID string, evt interface{}) error {
		// When a session connects, try to initialize a worker for it
		if _, err := sessionManager.InitWorker(userID); err != nil {
			logger.Warn("Falha ao inicializar worker para sessão conectada", "user_id", userID, "error", err)
		}
		return webhookService.DispatchEvent(userID, "connected", evt)
	})
	sessionManager.RegisterEventHandler("disconnected", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "disconnected", evt)
	})
	sessionManager.RegisterEventHandler("logged_out", func(userID string, evt interface{}) error {
		// Stop worker when session logs out
		if err := sessionManager.StopWorker(userID); err != nil {
			logger.Warn("Falha ao parar worker para sessão deslogada", "user_id", userID, "error", err)
		}
		return webhookService.DispatchEvent(userID, "logged_out", evt)
	})
	sessionManager.RegisterEventHandler("qr", func(userID string, evt interface{}) error {
		return webhookService.DispatchEvent(userID, "qr", evt)
	})

	// Configure HTTP handlers
	sessionHandler := handlers.NewSessionHandler(sessionManager, cfg)
	messageHandler := handlers.NewMessageHandler(sessionManager)
	groupHandler := handlers.NewGroupHandler(sessionManager)
	communityHandler := handlers.NewCommunityHandler(sessionManager)
	newsletterHandler := handlers.NewNewsletterHandler(sessionManager)
	webhookHandler := handlers.NewWebhookHandler(webhookService)
	authHandler := handlers.NewAuthHandler(cfg.EncryptionKey)

	// Configure authentication middleware
	authMiddleware := middlewares.NewAuthMiddleware(cfg.APIKey, cfg.AdminAPIKey, authHandler)

	// Configure HTTP server
	r := gin.Default()
	routes.SetupRoutes(r, sessionHandler, messageHandler, webhookHandler, groupHandler, newsletterHandler, communityHandler, authHandler, authMiddleware)

	// Start server with graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Configure graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop coordinator system
	logger.Info("Parando sistema de coordenação...")
	if err := sessionManager.StopCoordinator(); err != nil {
		logger.Error("Erro ao parar coordinator", "error", err)
	}

	// Stop RabbitMQ consumers if initialized
	if consumerManager != nil {
		logger.Info("Stopping RabbitMQ consumers...")
		consumerManager.StopAll()
	}

	// Stop periodic cleanup
	sessionManager.StopPeriodicCleanup()

	// Disconnect all sessions before exiting
	sessionManager.DisconnectAll()

	// Close session manager
	if err := sessionManager.Close(); err != nil {
		logger.Error("Erro ao fechar session manager", "error", err)
	}

	// Close RabbitMQ publisher if initialized
	if eventPublisher != nil {
		logger.Info("Closing RabbitMQ publisher...")
		if err := eventPublisher.Close(); err != nil {
			logger.Error("Error closing RabbitMQ publisher", "error", err)
		}
	}

	// Shutdown server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server gracefully stopped")
}
