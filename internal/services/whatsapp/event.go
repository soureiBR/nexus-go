// internal/services/whatsapp/event.go
package whatsapp

// EventHandler processa eventos do WhatsApp
type EventHandler func(userID string, evt interface{}) error

// RegisterEventHandler registra um handler para eventos
func (sm *SessionManager) RegisterEventHandler(eventType string, handler EventHandler) {
    sm.clientsMutex.Lock()
    defer sm.clientsMutex.Unlock()
    
    if _, exists := sm.eventHandlers[eventType]; !exists {
        sm.eventHandlers[eventType] = make([]EventHandler, 0)
    }
    
    sm.eventHandlers[eventType] = append(sm.eventHandlers[eventType], handler)
}

// processEvent processa eventos do WhatsApp
func (sm *SessionManager) processEvent(userID string, evt interface{}) {
    // Atualizar último acesso
    if client, exists := sm.GetSession(userID); exists {
        client.LastActive = time.Now()
    }
    
    // Determinar tipo de evento
    var eventType string
    switch evt.(type) {
    case *events.Message:
        eventType = "message"
    case *events.Connected:
        eventType = "connected"
    case *events.Disconnected:
        eventType = "disconnected"
    case *events.LoggedOut:
        eventType = "logged_out"
    // Adicionar mais tipos conforme necessário
    default:
        eventType = "unknown"
    }
    
    // Chamar handlers
    if handlers, exists := sm.eventHandlers[eventType]; exists {
        for _, handler := range handlers {
            if err := handler(userID, evt); err != nil {
                sm.logger.Errorf("Erro ao processar evento %s: %v", eventType, err)
            }
        }
    }
}