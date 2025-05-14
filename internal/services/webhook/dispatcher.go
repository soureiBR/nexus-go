// internal/services/webhook/dispatcher.go
package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
	
	"yourproject/pkg/logger"
)

// Dispatcher gerencia o envio de eventos para webhooks
type Dispatcher struct {
	url           string
	enabledEvents map[string]bool
	secret        string
	client        *http.Client
	
	// Estado
	connected      bool
	lastError      string
	lastSuccessful time.Time
	mutex          sync.RWMutex
}

// WebhookStatus representa o status atual do webhook
type WebhookStatus struct {
	URL            string
	EnabledEvents  []string
	Connected      bool
	LastError      string
	LastSuccessful time.Time
}

// NewDispatcher cria um novo dispatcher de webhook
func NewDispatcher(url string) *Dispatcher {
	// Criar cliente HTTP com timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	
	return &Dispatcher{
		url:           url,
		enabledEvents: make(map[string]bool),
		client:        client,
		mutex:         sync.RWMutex{},
	}
}

// Configure configura o dispatcher com uma nova URL e eventos
func (d *Dispatcher) Configure(url string, events []string, secret string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	
	// Atualizar URL
	d.url = url
	
	// Atualizar eventos habilitados
	d.enabledEvents = make(map[string]bool)
	for _, evt := range events {
		d.enabledEvents[evt] = true
	}
	
	// Atualizar secret
	d.secret = secret
	
	// Testar conexão com webhook
	err := d.testConnection()
	d.connected = (err == nil)
	
	if err != nil {
		d.lastError = err.Error()
		return fmt.Errorf("falha ao testar conexão com webhook: %w", err)
	}
	
	d.lastSuccessful = time.Now()
	logger.Info("Webhook configurado com sucesso", "url", url, "events", events)
	return nil
}

// IsConfigured verifica se o webhook está configurado
func (d *Dispatcher) IsConfigured() bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	return d.url != ""
}

// IsEventEnabled verifica se um tipo de evento está habilitado
func (d *Dispatcher) IsEventEnabled(eventType string) bool {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	// Se nenhum evento específico estiver configurado, todos estão habilitados
	if len(d.enabledEvents) == 0 {
		return true
	}
	
	return d.enabledEvents[eventType]
}

// GetStatus retorna o status atual do webhook
func (d *Dispatcher) GetStatus() WebhookStatus {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	
	// Converter mapa de eventos para slice
	var events []string
	for evt := range d.enabledEvents {
		events = append(events, evt)
	}
	
	return WebhookStatus{
		URL:            d.url,
		EnabledEvents:  events,
		Connected:      d.connected,
		LastError:      d.lastError,
		LastSuccessful: d.lastSuccessful,
	}
}

// DispatchEvent envia um evento para o webhook configurado
func (d *Dispatcher) DispatchEvent(userID string, eventType string, data interface{}) error {
	// Verificar se webhook está configurado
	if !d.IsConfigured() {
		return fmt.Errorf("webhook não configurado")
	}
	
	// Verificar se o evento está habilitado
	if !d.IsEventEnabled(eventType) {
		return nil // Evento não habilitado, ignorar silenciosamente
	}
	
	// Preparar payload
	payload := map[string]interface{}{
		"user_id":    userID,
		"event_type": eventType,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"data":       data,
	}
	
	// Serializar payload
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("falha ao serializar payload: %w", err)
	}
	
	// Criar request
	req, err := http.NewRequest("POST", d.url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("falha ao criar request: %w", err)
	}
	
	// Adicionar headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "YourProject-WhatsApp-API/1.0")
	req.Header.Set("X-WhatsApp-Event", eventType)
	
	// Adicionar signature se secret estiver configurado
	d.mutex.RLock()
	secret := d.secret
	d.mutex.RUnlock()
	
	if secret != "" {
		signature := computeHMAC(jsonPayload, []byte(secret))
		req.Header.Set("X-Hub-Signature", fmt.Sprintf("sha256=%s", signature))
	}
	
	// Enviar request em uma goroutine para não bloquear
	go func() {
		resp, err := d.client.Do(req)
		
		d.mutex.Lock()
		defer d.mutex.Unlock()
		
		if err != nil {
			d.connected = false
			d.lastError = err.Error()
			logger.Error("Falha ao enviar webhook", "error", err, "event", eventType, "user_id", userID)
			return
		}
		
		defer resp.Body.Close()
		
		// Verificar status da resposta
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			d.connected = false
			d.lastError = fmt.Sprintf("status HTTP inesperado: %d", resp.StatusCode)
			logger.Error("Resposta de webhook inválida", "status", resp.StatusCode, "event", eventType, "user_id", userID)
			return
		}
		
		// Webhook enviado com sucesso
		d.connected = true
		d.lastSuccessful = time.Now()
		logger.Debug("Webhook enviado com sucesso", "event", eventType, "user_id", userID)
	}()
	
	return nil
}

// testConnection testa a conexão com o webhook
func (d *Dispatcher) testConnection() error {
	// Payload de teste
	testPayload := map[string]interface{}{
		"event_type": "test",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"data":       map[string]string{"message": "Teste de conexão"},
	}
	
	// Serializar payload
	jsonPayload, err := json.Marshal(testPayload)
	if err != nil {
		return fmt.Errorf("falha ao serializar payload de teste: %w", err)
	}
	
	// Criar request
	req, err := http.NewRequest("POST", d.url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("falha ao criar request de teste: %w", err)
	}
	
	// Adicionar headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "YourProject-WhatsApp-API/1.0")
	req.Header.Set("X-WhatsApp-Event", "test")
	
	// Adicionar signature se secret estiver configurado
	if d.secret != "" {
		signature := computeHMAC(jsonPayload, []byte(d.secret))
		req.Header.Set("X-Hub-Signature", fmt.Sprintf("sha256=%s", signature))
	}
	
	// Enviar request
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao conectar ao webhook: %w", err)
	}
	defer resp.Body.Close()
	
	// Verificar status da resposta
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status HTTP inesperado: %d", resp.StatusCode)
	}
	
	return nil
}

// computeHMAC calcula a assinatura HMAC SHA-256 para um payload
func computeHMAC(message, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}