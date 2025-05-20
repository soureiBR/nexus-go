// internal/services/rabbitmq/consumer.go
package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"yourproject/pkg/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

// EventHandler é a assinatura para handlers de eventos
type EventHandler func(deliveryTag uint64, routingKey string, body []byte) error

// EventConsumer gerencia a conexão e consumo de eventos do RabbitMQ
type EventConsumer struct {
	conn         *amqp.Connection
	channel      *amqp.Channel
	exchangeName string
	queueName    string
	isConnected  bool
	url          string
	handlers     map[string]EventHandler
	handlersLock sync.RWMutex
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// ConsumerConfig representa a configuração do consumidor
type ConsumerConfig struct {
	URL           string
	ExchangeName  string
	QueueName     string
	RoutingKeys   []string // Chaves de roteamento para vincular (ex: ["whatsapp.events.*", "whatsapp.events.message"])
	PrefetchCount int      // Número de mensagens para prefetch
}

// NewEventConsumer cria um novo consumidor de eventos
func NewEventConsumer(config ConsumerConfig) (*EventConsumer, error) {
	if config.PrefetchCount <= 0 {
		config.PrefetchCount = 10 // Valor padrão
	}

	consumer := &EventConsumer{
		url:          config.URL,
		exchangeName: config.ExchangeName,
		queueName:    config.QueueName,
		handlers:     make(map[string]EventHandler),
		stopChan:     make(chan struct{}),
	}

	err := consumer.connect(config.RoutingKeys, config.PrefetchCount)
	if err != nil {
		return nil, err
	}

	// Iniciar loop de reconexão em segundo plano
	go consumer.reconnectLoop(config.RoutingKeys, config.PrefetchCount)

	return consumer, nil
}

// connect estabelece conexão com RabbitMQ
func (c *EventConsumer) connect(routingKeys []string, prefetchCount int) error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("falha ao conectar ao RabbitMQ: %w", err)
	}
	c.conn = conn

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("falha ao abrir canal: %w", err)
	}
	c.channel = ch

	// Configurar prefetch
	err = ch.Qos(
		prefetchCount, // prefetch count
		0,             // prefetch size
		false,         // global
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("falha ao definir QoS: %w", err)
	}

	// Declarar exchange
	err = ch.ExchangeDeclare(
		c.exchangeName, // nome
		"topic",        // tipo
		true,           // durável
		false,          // auto-delete
		false,          // internal
		false,          // no-wait
		nil,            // argumentos
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("falha ao declarar exchange: %w", err)
	}

	// Declarar fila
	q, err := ch.QueueDeclare(
		c.queueName, // nome
		true,        // durável
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // argumentos
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("falha ao declarar fila: %w", err)
	}

	// Vincular a fila à exchange com as chaves de roteamento
	for _, key := range routingKeys {
		err = ch.QueueBind(
			q.Name,         // nome da fila
			key,            // chave de roteamento
			c.exchangeName, // exchange
			false,
			nil,
		)
		if err != nil {
			ch.Close()
			conn.Close()
			return fmt.Errorf("falha ao vincular fila a exchange com chave %s: %w", key, err)
		}
	}

	c.isConnected = true
	logger.Info("Conectado ao RabbitMQ",
		"url", c.url,
		"exchange", c.exchangeName,
		"queue", c.queueName)

	return nil
}

// reconnectLoop tenta reconectar se a conexão for perdida
func (c *EventConsumer) reconnectLoop(routingKeys []string, prefetchCount int) {
	for {
		select {
		case <-c.stopChan:
			return
		default:
			if !c.isConnected {
				logger.Info("Tentando reconectar ao RabbitMQ...")
				for i := 0; i < 5; i++ {
					if err := c.connect(routingKeys, prefetchCount); err != nil {
						logger.Error("Falha ao reconectar ao RabbitMQ",
							"erro", err,
							"tentativa", i+1)
						time.Sleep(time.Second * time.Duration(i+1))
						continue
					}
					break
				}
			}
			time.Sleep(5 * time.Second)
		}
	}
}

// RegisterHandler registra um handler para uma chave de roteamento específica
// Use "*" como chave para processar todas as mensagens
func (c *EventConsumer) RegisterHandler(routingKey string, handler EventHandler) {
	c.handlersLock.Lock()
	defer c.handlersLock.Unlock()

	c.handlers[routingKey] = handler
	logger.Debug("Handler registrado", "routing_key", routingKey)
}

// Start inicia o consumo de mensagens
func (c *EventConsumer) Start(ctx context.Context) error {
	if !c.isConnected {
		return fmt.Errorf("não conectado ao RabbitMQ")
	}

	msgs, err := c.channel.Consume(
		c.queueName, // fila
		"",          // consumer tag
		false,       // auto-ack
		false,       // exclusive
		false,       // no-local
		false,       // no-wait
		nil,         // argumentos
	)
	if err != nil {
		return fmt.Errorf("falha ao iniciar consumidor: %w", err)
	}

	// Contexto para graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Capturar sinais para shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigChan:
			logger.Info("Sinal de shutdown recebido")
			cancel()
		case <-ctx.Done():
			// Contexto cancelado em outro lugar
		}
	}()

	logger.Info("Consumidor iniciado", "queue", c.queueName)

	// Processar mensagens
	c.wg.Add(1)
	go c.processMessages(ctx, msgs)

	return nil
}

// processMessages processa mensagens do canal
func (c *EventConsumer) processMessages(ctx context.Context, msgs <-chan amqp.Delivery) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Parando processamento de mensagens")
			return
		case msg, ok := <-msgs:
			if !ok {
				logger.Error("Canal de mensagens fechado")
				c.isConnected = false
				return
			}

			// Processa a mensagem
			err := c.handleMessage(msg)

			if err != nil {
				logger.Error("Erro ao processar mensagem",
					"error", err,
					"routing_key", msg.RoutingKey)

				// Nack com requeue se for um erro processável
				msg.Nack(false, true)
			} else {
				// Ack se processado com sucesso
				msg.Ack(false)
			}
		}
	}
}

// handleMessage processa uma mensagem individual
func (c *EventConsumer) handleMessage(msg amqp.Delivery) error {
	c.handlersLock.RLock()
	defer c.handlersLock.RUnlock()

	// Verificar se há um handler específico para esta routing key
	if handler, exists := c.handlers[msg.RoutingKey]; exists {
		return handler(msg.DeliveryTag, msg.RoutingKey, msg.Body)
	}

	// Verificar se há um handler wildcard
	if handler, exists := c.handlers["*"]; exists {
		return handler(msg.DeliveryTag, msg.RoutingKey, msg.Body)
	}

	// Se chegou aqui, nenhum handler foi encontrado
	logger.Warn("Nenhum handler encontrado para routing key",
		"routing_key", msg.RoutingKey)

	// Não retornamos erro para que a mensagem seja ack'd e removida da fila
	return nil
}

// Stop interrompe o consumidor
func (c *EventConsumer) Stop() {
	close(c.stopChan)

	// Esperar pelo término dos processadores
	c.wg.Wait()

	if c.channel != nil {
		c.channel.Close()
	}

	if c.conn != nil {
		c.conn.Close()
	}

	c.isConnected = false
	logger.Info("Consumidor parado")
}
