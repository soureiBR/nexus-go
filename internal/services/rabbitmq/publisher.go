// internal/services/rabbitmq/publisher.go
package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"yourproject/pkg/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

// EventPublisher handles publishing WhatsApp events to RabbitMQ
type EventPublisher struct {
	conn         *amqp.Connection
	channel      *amqp.Channel
	exchangeName string
	isConnected  bool
	url          string
}

// Config represents RabbitMQ configuration
type Config struct {
	URL          string
	ExchangeName string
}

// NewEventPublisher creates a new RabbitMQ event publisher
func NewEventPublisher(config Config) (*EventPublisher, error) {
	publisher := &EventPublisher{
		url:          config.URL,
		exchangeName: config.ExchangeName,
	}

	err := publisher.connect()
	if err != nil {
		return nil, err
	}

	// Start reconnection loop in background
	go publisher.reconnectLoop()

	return publisher, nil
}

// connect establishes connection to RabbitMQ
func (p *EventPublisher) connect() error {
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}
	p.conn = conn

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel: %w", err)
	}
	p.channel = ch

	// Declare topic exchange
	err = ch.ExchangeDeclare(
		p.exchangeName, // name
		"topic",        // type
		true,           // durable
		false,          // auto-deleted
		false,          // internal
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	p.isConnected = true
	logger.Info("Connected to RabbitMQ", "url", p.url, "exchange", p.exchangeName)
	return nil
}

// reconnectLoop attempts to reconnect if connection is lost
func (p *EventPublisher) reconnectLoop() {
	for {
		if !p.isConnected {
			logger.Info("Attempting to reconnect to RabbitMQ...")
			for i := 0; i < 5; i++ {
				if err := p.connect(); err != nil {
					logger.Error("Failed to reconnect to RabbitMQ", "error", err, "attempt", i+1)
					time.Sleep(time.Second * time.Duration(i+1))
					continue
				}
				break
			}
		}
		time.Sleep(5 * time.Second)
	}
}

// PublishEvent publishes an event to RabbitMQ with routing key based on event type
func (p *EventPublisher) PublishEvent(ctx context.Context, userID, eventType string, payload interface{}) error {
	if !p.isConnected {
		return fmt.Errorf("not connected to RabbitMQ")
	}

	// Create event envelope
	event := map[string]interface{}{
		"user_id":    userID,
		"event_type": eventType,
		"payload":    payload,
		"timestamp":  time.Now().UnixMilli(),
	}

	// Serialize event
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create routing key: whatsapp.events.<event_type>
	routingKey := fmt.Sprintf("whatsapp.events.%s", eventType)

	// Publish message
	err = p.channel.PublishWithContext(
		ctx,
		p.exchangeName, // exchange
		routingKey,     // routing key
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
		})
	if err != nil {
		p.isConnected = false
		return fmt.Errorf("failed to publish message: %w", err)
	}

	logger.Debug("Event published to RabbitMQ",
		"user_id", userID,
		"event_type", eventType,
		"routing_key", routingKey)

	return nil
}

// GetChannel returns the RabbitMQ channel for queue setup
func (p *EventPublisher) GetChannel() *amqp.Channel {
	return p.channel
}

// Close shuts down the RabbitMQ connection
func (p *EventPublisher) Close() error {
	if p.channel != nil {
		p.channel.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
	p.isConnected = false
	return nil
}

// QueueSetup handles setting up specific queues for WhatsApp events
type QueueSetup struct {
	channel      *amqp.Channel
	exchangeName string
}

// NewQueueSetup creates a new queue setup manager
func NewQueueSetup(channel *amqp.Channel, exchangeName string) *QueueSetup {
	return &QueueSetup{
		channel:      channel,
		exchangeName: exchangeName,
	}
}

// SetupWhatsAppQueues creates the specific queues for all WhatsApp events
func (qs *QueueSetup) SetupWhatsAppQueues() error {
	queues := []struct {
		name       string
		routingKey string
	}{
		// Connection events
		{
			name:       "whatsapp.events.connection.update",
			routingKey: "whatsapp.events.connection.update",
		},
		// Group member events
		{
			name:       "whatsapp.events.group.members.updated",
			routingKey: "whatsapp.events.group.members.updated",
		},
		{
			name:       "whatsapp.events.group.members.updated",
			routingKey: "whatsapp.events.group.members.added",
		},
		{
			name:       "whatsapp.events.group.members.updated",
			routingKey: "whatsapp.events.group.members.removed",
		},
		{
			name:       "whatsapp.events.group.members.updated",
			routingKey: "whatsapp.events.group.members.promoted",
		},
		{
			name:       "whatsapp.events.group.members.updated",
			routingKey: "whatsapp.events.group.members.demoted",
		},
		// Error events
		{
			name:       "whatsapp.events.send-message-error",
			routingKey: "whatsapp.events.send-message-error",
		},
		// Group settings events
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.updated",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.name.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.topic.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.announce.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.locked.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.ephemeral.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.membership.approval.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.member.add.mode.changed",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.deleted",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.link.enabled",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.link.disabled",
		},
		{
			name:       "whatsapp.events.group.updated",
			routingKey: "whatsapp.events.group.invite.link.changed",
		},
	}

	for _, q := range queues {
		// Declare queue
		_, err := qs.channel.QueueDeclare(
			q.name, // name
			true,   // durable
			false,  // delete when unused
			false,  // exclusive
			false,  // no-wait
			nil,    // arguments
		)
		if err != nil {
			return fmt.Errorf("failed to declare queue %s: %w", q.name, err)
		}

		// Bind queue to exchange
		err = qs.channel.QueueBind(
			q.name,          // queue name
			q.routingKey,    // routing key
			qs.exchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			return fmt.Errorf("failed to bind queue %s: %w", q.name, err)
		}

		logger.Info("Queue setup complete", "queue", q.name, "routing_key", q.routingKey)
	}

	return nil
}
