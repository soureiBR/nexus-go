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
