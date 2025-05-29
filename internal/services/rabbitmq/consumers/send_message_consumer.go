// internal/services/rabbitmq/consumers/send_message_consumer.go
package consumers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"yourproject/internal/services/rabbitmq"
	"yourproject/internal/services/whatsapp"
	"yourproject/internal/services/whatsapp/worker"
	"yourproject/pkg/logger"
)

// SendMessagePayload represents the payload for sending WhatsApp messages
type SendMessagePayload struct {
	SessionID string `json:"sessionId" binding:"required"`
	JID       string `json:"jid" binding:"required"`
	Message   struct {
		Text  *string `json:"text,omitempty"`
		Media *struct {
			URL      string  `json:"url" binding:"required"`
			Type     string  `json:"type" binding:"required"` // image, video, audio, document
			Filename *string `json:"filename,omitempty"`
			Caption  *string `json:"caption,omitempty"`
		} `json:"media,omitempty"`
		Buttons *[]struct {
			ButtonID   string `json:"buttonId" binding:"required"`
			ButtonText struct {
				DisplayText string `json:"displayText" binding:"required"`
			} `json:"buttonText" binding:"required"`
			Type int `json:"type" binding:"required"`
		} `json:"buttons,omitempty"`
		Location *struct {
			DegreesLatitude  float64 `json:"degreesLatitude" binding:"required"`
			DegreesLongitude float64 `json:"degreesLongitude" binding:"required"`
			Name             *string `json:"name,omitempty"`
			Address          *string `json:"address,omitempty"`
		} `json:"location,omitempty"`
		Contacts  *[]interface{} `json:"contacts,omitempty"`
		Reactions *struct {
			Key struct {
				RemoteJID   string  `json:"remoteJid" binding:"required"`
				FromMe      bool    `json:"fromMe"`
				ID          string  `json:"id" binding:"required"`
				Participant *string `json:"participant,omitempty"`
			} `json:"key" binding:"required"`
			Text string `json:"text" binding:"required"`
		} `json:"reactions,omitempty"`
		Poll *struct {
			Name                   string   `json:"name" binding:"required"`
			Options                []string `json:"options" binding:"required,min=2,max=12"`
			SelectableOptionsCount int      `json:"selectableOptionsCount,omitempty"`
		} `json:"poll,omitempty"`
		List *struct {
			Text       string `json:"text" binding:"required"`
			Footer     string `json:"footer,omitempty"`
			ButtonText string `json:"buttonText" binding:"required"`
			Sections   []struct {
				Title string `json:"title,omitempty"`
				Rows  []struct {
					ID          string `json:"id" binding:"required"`
					Title       string `json:"title" binding:"required"`
					Description string `json:"description,omitempty"`
				} `json:"rows" binding:"required,min=1"`
			} `json:"sections" binding:"required,min=1"`
		} `json:"list,omitempty"`
	} `json:"message" binding:"required"`
}

// SendMessageConsumer handles WhatsApp send message events via RabbitMQ using session manager directly
type SendMessageConsumer struct {
	consumer       *rabbitmq.EventConsumer
	sessionManager *whatsapp.SessionManager
	publisher      *rabbitmq.EventPublisher
	queueName      string
	cancel         context.CancelFunc
}

// SendMessageConsumerConfig holds configuration for the send message consumer
type SendMessageConsumerConfig struct {
	ConsumerConfig rabbitmq.ConsumerConfig
	SessionManager *whatsapp.SessionManager
	Publisher      *rabbitmq.EventPublisher
}

// NewSendMessageConsumer creates a new send message consumer that calls session manager directly
func NewSendMessageConsumer(config SendMessageConsumerConfig) (*SendMessageConsumer, error) {
	// Ensure routing keys are set
	if len(config.ConsumerConfig.RoutingKeys) == 0 {
		config.ConsumerConfig.RoutingKeys = []string{"whatsapp.events.send-message"}
	}

	// Create consumer
	consumer, err := rabbitmq.NewEventConsumer(config.ConsumerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create RabbitMQ consumer: %w", err)
	}

	smc := &SendMessageConsumer{
		consumer:       consumer,
		sessionManager: config.SessionManager,
		publisher:      config.Publisher,
		queueName:      config.ConsumerConfig.QueueName,
	}

	// Register message handler
	consumer.RegisterHandler("*", smc.handleMessage)

	logger.Info("SendMessage consumer created (using session manager directly)", "queue", config.ConsumerConfig.QueueName)

	return smc, nil
}

// Start starts the consumer (non-blocking)
func (smc *SendMessageConsumer) Start() error {
	logger.Info("🚀 STARTING SendMessage consumer (using session manager directly)", "queue", smc.queueName)

	// Create a context that will be used to control the consumer lifecycle
	ctx, cancel := context.WithCancel(context.Background())
	smc.cancel = cancel

	err := smc.consumer.Start(ctx)
	if err != nil {
		logger.Error("❌ FAILED to start SendMessage consumer", "error", err, "queue", smc.queueName)
		cancel() // Cancel the context on error
		return fmt.Errorf("failed to start consumer: %w", err)
	}

	logger.Info("✅ SendMessage consumer started successfully (using session manager directly)", "queue", smc.queueName)

	return nil
}

// Stop stops the consumer
func (smc *SendMessageConsumer) Stop() {
	logger.Info("Stopping SendMessage consumer", "queue", smc.queueName)
	if smc.cancel != nil {
		smc.cancel() // Cancel the context to stop message processing
	}
	smc.consumer.Stop()
}

// handleMessage processes incoming send message requests using session manager directly
func (smc *SendMessageConsumer) handleMessage(deliveryTag uint64, routingKey string, body []byte) error {
	var payload SendMessagePayload
	logger.Info("🔥 CONSUMER: Received message for send message consumer",
		"delivery_tag", deliveryTag,
		"routing_key", routingKey,
		"body_size", len(body),
		"body", string(body))

	// Parse JSON payload
	if err := json.Unmarshal(body, &payload); err != nil {
		logger.Error("❌ CONSUMER: Failed to parse message payload",
			"error", err,
			"delivery_tag", deliveryTag,
			"routing_key", routingKey,
			"body", string(body))

		// Try to extract sessionID from raw body for error event publishing
		sessionID := "unknown-session"
		var rawData map[string]interface{}
		if rawErr := json.Unmarshal(body, &rawData); rawErr == nil {
			if rawSessionID, ok := rawData["sessionId"].(string); ok && rawSessionID != "" {
				sessionID = rawSessionID
			}
		}

		// Publish error event for JSON parsing failure
		smc.publishErrorEvent(sessionID, "send-message-error", string(body), err)

		// For JSON parsing errors, we should acknowledge the message to prevent requeuing
		// as the message will never be parseable
		logger.Info("🚮 CONSUMER: Message acknowledged despite JSON parsing error to prevent queue blocking",
			"error", err.Error())
		return nil
	}

	logger.Info("✅ CONSUMER: Successfully parsed JSON payload",
		"delivery_tag", deliveryTag,
		"session_id", payload.SessionID,
		"jid", payload.JID)

	// Validate required fields
	if payload.SessionID == "" || payload.JID == "" {
		err := fmt.Errorf("sessionId and jid are required")
		logger.Error("❌ CONSUMER: Invalid message payload", "error", err, "payload", payload)
		smc.publishErrorEvent(payload.SessionID, "send-message-error", payload, err)

		// Don't return error to prevent message requeuing for validation errors
		logger.Info("🚮 CONSUMER: Message acknowledged despite validation error to prevent queue blocking",
			"error", err.Error())
		return nil
	}

	logger.Info("🚀 CONSUMER: Starting to process send message request",
		"session_id", payload.SessionID,
		"jid", payload.JID,
		"routing_key", routingKey,
		"has_text", payload.Message.Text != nil,
		"has_media", payload.Message.Media != nil,
		"has_buttons", payload.Message.Buttons != nil,
		"has_list", payload.Message.List != nil)

	// Process the message using session manager directly
	if err := smc.processMessageViaSessionManager(payload); err != nil {
		logger.Error("❌ CONSUMER: Failed to process message via session manager",
			"error", err,
			"session_id", payload.SessionID,
			"jid", payload.JID)

		smc.publishErrorEvent(payload.SessionID, "send-message-error", payload, err)

		// Don't return error to prevent message requeuing and queue blocking
		// The error event has been published, so the error is properly handled
		logger.Info("🚮 CONSUMER: Message acknowledged despite error to prevent queue blocking",
			"session_id", payload.SessionID,
			"jid", payload.JID,
			"error", err.Error())
		return nil
	}

	logger.Info("✅ CONSUMER: Message processed successfully via session manager",
		"session_id", payload.SessionID,
		"jid", payload.JID)

	return nil
}

// processMessageViaSessionManager handles different message types using session manager directly
func (smc *SendMessageConsumer) processMessageViaSessionManager(payload SendMessagePayload) error {
	msg := payload.Message

	logger.Info("📝 HANDLER: Analyzing message type",
		"session_id", payload.SessionID,
		"jid", payload.JID,
		"has_text", msg.Text != nil,
		"has_media", msg.Media != nil,
		"has_buttons", msg.Buttons != nil,
		"has_list", msg.List != nil)

	// Wait for session to be ready with retry mechanism
	_, err := smc.waitForSessionReady(payload.SessionID, 30*time.Second)
	if err != nil {
		return fmt.Errorf("session not ready: %w", err)
	}

	logger.Info("✅ HANDLER: Session is ready for message processing",
		"session_id", payload.SessionID,
		"jid", payload.JID)

	// Handle text message
	if msg.Text != nil {
		logger.Info("📝 HANDLER: Sending text message",
			"session_id", payload.SessionID,
			"jid", payload.JID,
			"text_length", len(*msg.Text))

		_, err := smc.sessionManager.SendText(payload.SessionID, payload.JID, *msg.Text)
		if err != nil {
			return fmt.Errorf("failed to send text message: %w", err)
		}
		return nil
	}

	// Handle media message
	if msg.Media != nil {
		logger.Info("📸 HANDLER: Sending media message",
			"session_id", payload.SessionID,
			"jid", payload.JID,
			"media_type", msg.Media.Type,
			"media_url", msg.Media.URL)

		caption := ""
		if msg.Media.Caption != nil {
			caption = *msg.Media.Caption
		}

		_, err := smc.sessionManager.SendMedia(payload.SessionID, payload.JID, msg.Media.URL, msg.Media.Type, caption)
		if err != nil {
			return fmt.Errorf("failed to send media message: %w", err)
		}
		return nil
	}

	// Handle buttons message
	if msg.Buttons != nil {
		logger.Info("🔘 HANDLER: Sending buttons message",
			"session_id", payload.SessionID,
			"jid", payload.JID,
			"buttons_count", len(*msg.Buttons))

		var buttons []worker.ButtonData
		for _, btn := range *msg.Buttons {
			buttons = append(buttons, worker.ButtonData{
				ID:          btn.ButtonID,
				DisplayText: btn.ButtonText.DisplayText,
			})
		}

		text := ""
		if msg.Text != nil {
			text = *msg.Text
		}

		_, err := smc.sessionManager.SendButtons(payload.SessionID, payload.JID, text, "", buttons)
		if err != nil {
			return fmt.Errorf("failed to send buttons message: %w", err)
		}
		return nil
	}

	// Handle list message
	if msg.List != nil {
		logger.Info("📋 HANDLER: Sending list message",
			"session_id", payload.SessionID,
			"jid", payload.JID,
			"sections_count", len(msg.List.Sections))

		var sections []worker.Section
		for _, section := range msg.List.Sections {
			var rows []worker.Row
			for _, row := range section.Rows {
				rows = append(rows, worker.Row{
					ID:          row.ID,
					Title:       row.Title,
					Description: row.Description,
				})
			}
			sections = append(sections, worker.Section{
				Title: section.Title,
				Rows:  rows,
			})
		}

		_, err := smc.sessionManager.SendList(payload.SessionID, payload.JID, msg.List.Text, msg.List.Footer, msg.List.ButtonText, sections)
		if err != nil {
			return fmt.Errorf("failed to send list message: %w", err)
		}
		return nil
	}

	logger.Error("❌ HANDLER: No valid message type found",
		"session_id", payload.SessionID,
		"jid", payload.JID)
	return fmt.Errorf("no valid message type found in payload")
}

// publishErrorEvent publishes an error event
func (smc *SendMessageConsumer) publishErrorEvent(sessionID, eventType string, payload interface{}, err error) {
	if smc.publisher == nil {
		logger.Warn("Publisher not available for error event", "session_id", sessionID)
		return
	}

	// Use a default sessionID if empty to ensure error event is still published
	if sessionID == "" {
		sessionID = "unknown-session"
	}

	errorEvent := map[string]interface{}{
		"sessionId": sessionID,
		"eventType": eventType,
		"error":     err.Error(),
		"payload":   payload,
		"timestamp": time.Now().Unix(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if publishErr := smc.publisher.PublishEvent(ctx, sessionID, eventType, errorEvent); publishErr != nil {
		logger.Error("Failed to publish error event", "error", publishErr, "session_id", sessionID)
	} else {
		logger.Info("📤 CONSUMER: Error event published successfully",
			"session_id", sessionID,
			"event_type", eventType,
			"error", err.Error())
	}
}

// waitForSessionReady waits for a session to be connected with retry mechanism
func (smc *SendMessageConsumer) waitForSessionReady(sessionID string, timeout time.Duration) (*whatsapp.Client, error) {
	logger.Info("⏳ HANDLER: Waiting for session to be ready",
		"session_id", sessionID,
		"timeout", timeout)

	deadline := time.Now().Add(timeout)
	checkInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		client, exists := smc.sessionManager.GetSession(sessionID)
		if !exists {
			logger.Debug("Session not found, waiting...", "session_id", sessionID)
			time.Sleep(checkInterval)
			continue
		}

		if client.Connected {
			logger.Info("✅ HANDLER: Session is connected and ready",
				"session_id", sessionID)
			return client, nil
		}

		logger.Debug("Session exists but not connected, waiting...",
			"session_id", sessionID,
			"connected", client.Connected)
		time.Sleep(checkInterval)
	}

	return nil, fmt.Errorf("timeout waiting for session %s to be ready after %v", sessionID, timeout)
}
