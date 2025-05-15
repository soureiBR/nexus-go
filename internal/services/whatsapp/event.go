// internal/services/whatsapp/event.go
package whatsapp

import (
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

// processEvent processes WhatsApp events
func (sm *SessionManager) processEvent(userID string, evt interface{}) {
	// Your event processing logic here
	// For example:

	// Update client in cache
	sm.clientsMutex.Lock()
	if client, exists := sm.clients[userID]; exists {
		client.LastActive = time.Now()
	}
	sm.clientsMutex.Unlock()

	// Determine event type
	var eventType string
	switch evt.(type) {
	case *events.Message:
		eventType = "message"
	case *events.Connected:
		eventType = "connected"

		// When the device is authenticated, save the mapping
		client, exists := sm.GetSession(userID)
		if exists && client.WAClient.Store.ID != nil {
			deviceJID := client.WAClient.Store.ID.String()
			if err := sm.sqlStore.SaveUserDeviceMapping(userID, deviceJID); err != nil {
				logger.Error("Failed to save mapping", "user_id", userID, "device_jid", deviceJID, "error", err)
			} else {
				logger.Info("Mapping saved", "user_id", userID, "device_jid", deviceJID)
			}
		}

	case *events.Disconnected:
		eventType = "disconnected"
	case *events.LoggedOut:
		eventType = "logged_out"

		// When device is logged out, remove the mapping
		if err := sm.sqlStore.DeleteUserDeviceMapping(userID); err != nil {
			logger.Error("Failed to remove mapping after logout", "user_id", userID, "error", err)
		}

	case *events.QR:
		eventType = "qr"
	default:
		eventType = "unknown"
	}

	// Call handlers
	sm.clientsMutex.RLock()
	handlers, exists := sm.eventHandlers[eventType]
	sm.clientsMutex.RUnlock()

	if exists {
		for _, handler := range handlers {
			if err := handler(userID, evt); err != nil {
				sm.logger.Errorf("Error processing event %s: %v", eventType, err)
			}
		}
	}
}
