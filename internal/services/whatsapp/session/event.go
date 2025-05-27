// internal/services/whatsapp/event.go
package session

import (
	"fmt"
	"time"
	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow/types/events"
)

// RegisterEventHandler registra um handler para eventos
func (sm *SessionManager) RegisterEventHandler(eventType string, handler EventHandler) {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	if _, exists := sm.eventHandlers[eventType]; !exists {
		sm.eventHandlers[eventType] = make([]EventHandler, 0)
	}

	sm.eventHandlers[eventType] = append(sm.eventHandlers[eventType], handler)
}

// ProcessEvent processes WhatsApp events
func (sm *SessionManager) ProcessEvent(userID string, evt interface{}) {
	// Determine event type
	var eventType string

	// Atualizar última atividade do cliente
	sm.clientsMutex.Lock()
	if client, exists := sm.clients[userID]; exists {
		client.LastActive = time.Now()
	}
	sm.clientsMutex.Unlock()

	switch typedEvt := evt.(type) {
	case *events.Message:
		eventType = "message"

	case *events.Connected:
		eventType = "connected"

		// Quando o dispositivo é autenticado, salvar o mapeamento
		client, exists := sm.GetSession(userID)
		if exists && client.WAClient.Store.ID != nil {
			deviceJID := client.WAClient.Store.ID.String()
			if err := sm.sqlStore.SaveUserDeviceMapping(userID, deviceJID); err != nil {
				logger.Error("Falha ao salvar mapeamento", "user_id", userID, "device_jid", deviceJID, "error", err)
			} else {
				logger.Info("Mapeamento salvo", "user_id", userID, "device_jid", deviceJID)
			}

			// Atualizar estado conectado
			sm.clientsMutex.Lock()
			client.Connected = true
			sm.clientsMutex.Unlock()
		}

	case *events.Disconnected:
		eventType = "disconnected"

		// Atualizar estado de conexão
		sm.clientsMutex.Lock()
		if client, exists := sm.clients[userID]; exists {
			client.Connected = false
		}
		sm.clientsMutex.Unlock()

		logger.Info("Cliente desconectado", "user_id", userID, "reason")

	case *events.LoggedOut:
		eventType = "logged_out"

		logger.Info("Evento de logout recebido", "user_id", userID, "reason", typedEvt.Reason)

		// Executar limpeza completa da sessão
		if err := sm.handleDeviceLogout(userID); err != nil {
			logger.Error("Falha ao limpar sessão após logout", "user_id", userID, "error", err)
		}

	case *events.QR:
		eventType = "qr"

	default:
		eventType = "unknown"
		logger.Debug("Evento desconhecido recebido",
			"user_id", userID,
			"event_type", fmt.Sprintf("%T", evt))
	}

	// Chamar handlers
	sm.clientsMutex.RLock()
	handlers, exists := sm.eventHandlers[eventType]
	sm.clientsMutex.RUnlock()

	if exists {
		for _, handler := range handlers {
			if err := handler(userID, evt); err != nil {
				sm.logger.Errorf("Erro ao processar evento %s: %v", eventType, err)
			}
		}
	}
}
