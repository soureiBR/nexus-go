// internal/services/rabbitmq/consumers/manager.go
package consumers

import (
	"context"
	"sync"

	"yourproject/internal/services/rabbitmq"
	"yourproject/internal/services/whatsapp"
	"yourproject/pkg/logger"
)

// ConsumerManager manages multiple RabbitMQ consumers
type ConsumerManager struct {
	consumers      map[string]Consumer
	consumersMutex sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// Consumer interface that all consumers should implement
type Consumer interface {
	Start() error
	Stop()
}

// ConsumerManagerConfig represents configuration for the consumer manager
type ConsumerManagerConfig struct {
	RabbitMQURL    string
	ExchangeName   string
	SessionManager *whatsapp.SessionManager
	Publisher      *rabbitmq.EventPublisher
}

// NewConsumerManager creates a new consumer manager
func NewConsumerManager(config ConsumerManagerConfig) *ConsumerManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ConsumerManager{
		consumers: make(map[string]Consumer),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// RegisterSendMessageConsumer registers the send message consumer
func (cm *ConsumerManager) RegisterSendMessageConsumer(config ConsumerManagerConfig) error {
	cm.consumersMutex.Lock()
	defer cm.consumersMutex.Unlock()

	consumerConfig := SendMessageConsumerConfig{
		ConsumerConfig: rabbitmq.ConsumerConfig{
			URL:           config.RabbitMQURL,
			ExchangeName:  config.ExchangeName,
			QueueName:     "whatsapp.events.send-message",
			RoutingKeys:   []string{"whatsapp.events.send-message"},
			PrefetchCount: 6,
		},
		SessionManager: config.SessionManager,
		Publisher:      config.Publisher,
	}

	consumer, err := NewSendMessageConsumer(consumerConfig)
	if err != nil {
		return err
	}

	cm.consumers["send-message"] = consumer
	logger.Info("SendMessage consumer registered")

	return nil
}

// AddConsumer adds a custom consumer to the manager
func (cm *ConsumerManager) AddConsumer(name string, consumer Consumer) {
	cm.consumersMutex.Lock()
	defer cm.consumersMutex.Unlock()

	cm.consumers[name] = consumer
	logger.Info("Consumer added", "name", name)
}

// StartAll starts all registered consumers
func (cm *ConsumerManager) StartAll() error {
	cm.consumersMutex.RLock()
	defer cm.consumersMutex.RUnlock()

	logger.Info("Starting all consumers", "count", len(cm.consumers))

	for name, consumer := range cm.consumers {
		cm.wg.Add(1)
		go func(name string, consumer Consumer) {
			defer cm.wg.Done()

			logger.Info("Starting consumer", "name", name)
			if err := consumer.Start(); err != nil {
				logger.Error("Failed to start consumer", "name", name, "error", err)
			}
		}(name, consumer)
	}

	return nil
}

// StopAll stops all consumers gracefully
func (cm *ConsumerManager) StopAll() {
	logger.Info("Stopping all consumers")

	cm.cancel()

	cm.consumersMutex.RLock()
	for name, consumer := range cm.consumers {
		logger.Info("Stopping consumer", "name", name)
		consumer.Stop()
	}
	cm.consumersMutex.RUnlock()

	// Wait for all consumers to stop
	cm.wg.Wait()

	logger.Info("All consumers stopped")
}

// GetConsumer returns a consumer by name
func (cm *ConsumerManager) GetConsumer(name string) (Consumer, bool) {
	cm.consumersMutex.RLock()
	defer cm.consumersMutex.RUnlock()

	consumer, exists := cm.consumers[name]
	return consumer, exists
}

// ListConsumers returns a list of all consumer names
func (cm *ConsumerManager) ListConsumers() []string {
	cm.consumersMutex.RLock()
	defer cm.consumersMutex.RUnlock()

	names := make([]string, 0, len(cm.consumers))
	for name := range cm.consumers {
		names = append(names, name)
	}

	return names
}
