// internal/services/whatsapp/session.go
package whatsapp

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "go.mau.fi/whatsmeow"
    "go.mau.fi/whatsmeow/store"
    "go.mau.fi/whatsmeow/types"
    "go.mau.fi/whatsmeow/types/events"
    waLog "go.mau.fi/whatsmeow/util/log"
    
    "yourproject/internal/storage"
)

// SessionManager gerencia múltiplas sessões de WhatsApp
type SessionManager struct {
    clients      map[string]*Client
    clientsMutex sync.RWMutex
    fileStore    *storage.FileStore
    eventHandlers map[string][]EventHandler
    logger       waLog.Logger
}

// Client encapsula o cliente whatsmeow com metadados adicionais
type Client struct {
    ID          string
    WAClient    *whatsmeow.Client
    Connected   bool
    CreatedAt   time.Time
    LastActive  time.Time
}

// EventHandler processa eventos do WhatsApp
type EventHandler func(userID string, evt interface{}) error

// NewSessionManager cria um novo gerenciador de sessões
func NewSessionManager(fileStore *storage.FileStore) *SessionManager {
    return &SessionManager{
        clients:       make(map[string]*Client),
        fileStore:     fileStore,
        eventHandlers: make(map[string][]EventHandler),
        logger:        waLog.Stdout("whatsapp", "INFO", true),
    }
}

// CreateSession cria uma nova sessão de WhatsApp
func (sm *SessionManager) CreateSession(userID string) (*Client, error) {
    sm.clientsMutex.Lock()
    defer sm.clientsMutex.Unlock()
    
    // Verificar se já existe
    if client, exists := sm.clients[userID]; exists {
        return client, nil
    }
    
    // Criar dispositivo a partir do armazenamento
    deviceStore := store.NewDeviceStore(sm.fileStore, nil)
    
    // Inicializar cliente WhatsApp
    client := whatsmeow.NewClient(deviceStore, sm.logger)
    
    // Registrar handler de eventos
    client.AddEventHandler(func(evt interface{}) {
        sm.processEvent(userID, evt)
    })
    
    // Armazenar cliente
    newClient := &Client{
        ID:        userID,
        WAClient:  client,
        CreatedAt: time.Now(),
        LastActive: time.Now(),
    }
    
    sm.clients[userID] = newClient
    
    return newClient, nil
}

// GetSession obtém uma sessão existente
func (sm *SessionManager) GetSession(userID string) (*Client, bool) {
    sm.clientsMutex.RLock()
    defer sm.clientsMutex.RUnlock()
    
    client, exists := sm.clients[userID]
    return client, exists
}

// Connect inicia a conexão com o WhatsApp
func (sm *SessionManager) Connect(userID string) error {
    client, exists := sm.GetSession(userID)
    if !exists {
        return fmt.Errorf("sessão não encontrada: %s", userID)
    }
    
    return client.WAClient.Connect()
}

// Disconnect encerra a conexão com o WhatsApp
func (sm *SessionManager) Disconnect(userID string) error {
    client, exists := sm.GetSession(userID)
    if !exists {
        return fmt.Errorf("sessão não encontrada: %s", userID)
    }
    
    client.WAClient.Disconnect()
    return nil
}

// DeleteSession remove uma sessão
func (sm *SessionManager) DeleteSession(userID string) error {
    sm.clientsMutex.Lock()
    defer sm.clientsMutex.Unlock()
    
    client, exists := sm.clients[userID]
    if !exists {
        return fmt.Errorf("sessão não encontrada: %s", userID)
    }
    
    // Desconectar cliente
    client.WAClient.Disconnect()
    
    // Remover do cache
    delete(sm.clients, userID)
    
    // Remover dados do armazenamento
    return sm.fileStore.DeleteSession(userID)
}

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