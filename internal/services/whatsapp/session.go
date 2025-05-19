package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"yourproject/internal/storage"
	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// SessionManager manages multiple WhatsApp sessions
type SessionManager struct {
	clients       map[string]*Client
	clientsMutex  sync.RWMutex
	sqlStore      *storage.SQLStore
	eventHandlers map[string][]EventHandler
	logger        waLog.Logger
}

// Client encapsulates the whatsmeow client with additional metadata
type Client struct {
	ID         string
	WAClient   *whatsmeow.Client
	Connected  bool
	CreatedAt  time.Time
	LastActive time.Time
}

// EventHandler processes WhatsApp events
type EventHandler func(userID string, evt interface{}) error

// NewSessionManager creates a new session manager
func NewSessionManager(sqlStore *storage.SQLStore) *SessionManager {
	// Configure WhatsApp logger
	waLogger := waLog.Stdout("whatsapp", "INFO", true)

	return &SessionManager{
		clients:       make(map[string]*Client),
		sqlStore:      sqlStore,
		eventHandlers: make(map[string][]EventHandler),
		logger:        waLogger,
	}
}

// InitSessions carrega e reconecta todas as sessões existentes do banco de dados
func (sm *SessionManager) InitSessions(ctx context.Context) error {
	// Obter todos os mapeamentos userID -> deviceJID
	mappings, err := sm.sqlStore.GetAllUserDeviceMappings()
	if err != nil {
		return fmt.Errorf("falha ao carregar mapeamentos de userID: %w", err)
	}

	logger.Info("Carregando sessões existentes", "count", len(mappings))

	// Configurar nome do dispositivo padrão (antes de criar qualquer cliente)
	store.SetOSInfo("Linux", store.GetWAVersion())
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()

	// Criar e conectar cada sessão
	for _, mapping := range mappings {
		logger.Info("Restaurando sessão", "user_id", mapping.UserID, "device_jid", mapping.DeviceJID)

		// Criar a sessão
		client, err := sm.CreateSession(ctx, mapping.UserID)
		if err != nil {
			logger.Error("Falha ao criar sessão", "user_id", mapping.UserID, "error", err)
			continue
		}

		// Verificar se a sessão foi autenticada
		if client.WAClient.Store.ID == nil {
			logger.Warn("Sessão restaurada, mas não está autenticada", "user_id", mapping.UserID)
			continue
		}

		// Tentar conectar ao WhatsApp
		logger.Info("Reconectando sessão", "user_id", mapping.UserID)

		// Definir timeout para conexão
		_, cancel := context.WithTimeout(ctx, 30*time.Second)

		// Conectar em goroutine separada
		go func(userID string, cancelFunc context.CancelFunc) {
			defer cancelFunc()

			err := client.WAClient.Connect()
			if err != nil {
				logger.Error("Falha ao conectar sessão restaurada",
					"user_id", userID,
					"error", err)
				return
			}

			// Aguardar conexão ser estabelecida
			if client.WAClient.WaitForConnection(20 * time.Second) {
				// Conexão bem sucedida
				client.Connected = true
				client.LastActive = time.Now()
				logger.Info("Sessão reconectada com sucesso", "user_id", userID)
			} else {
				// Timeout ao conectar
				logger.Warn("Timeout ao tentar reconectar sessão", "user_id", userID)
			}
		}(mapping.UserID, cancel)
	}

	// Aguardar um pouco para que as conexões iniciais possam ser estabelecidas
	// Isso é opcional, mas pode ajudar a garantir que algumas sessões já estejam conectadas
	// antes de continuar com a inicialização do serviço
	time.Sleep(5 * time.Second)

	return nil
}

// CreateSession creates a new WhatsApp session
func (sm *SessionManager) CreateSession(ctx context.Context, userID string) (*Client, error) {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	// Check if already exists
	if client, exists := sm.clients[userID]; exists {
		return client, nil
	}

	// Get the database container
	container := sm.sqlStore.GetDBContainer()
	if container == nil {
		return nil, fmt.Errorf("database container is nil")
	}

	// Check if a mapping exists
	deviceJID, err := sm.sqlStore.GetDeviceJIDByUserID(userID)
	var devicePtr *store.Device // Use a pointer type

	if err == nil && deviceJID != "" {
		// If mapping exists, try to load the existing device
		logger.Debug("Loading existing device", "user_id", userID, "device_jid", deviceJID)

		// Parse the JID to ensure it's valid
		jid, jidErr := types.ParseJID(deviceJID)
		if jidErr != nil {
			logger.Warn("Invalid device JID stored", "user_id", userID, "device_jid", deviceJID, "error", jidErr)

			// Delete the invalid mapping
			if delErr := sm.sqlStore.DeleteUserDeviceMapping(userID); delErr != nil {
				logger.Warn("Failed to delete invalid mapping", "error", delErr)
			}

			// Create a new device instead
			devicePtr = container.NewDevice() // Store the pointer directly
		} else {
			// Get the device by JID
			var getErr error
			devicePtr, getErr = container.GetDevice(ctx, jid) // This returns a pointer
			if getErr != nil {
				return nil, fmt.Errorf("failed to get device: %w", getErr)
			}

			// If nil, device no longer exists in database
			if devicePtr == nil {
				// Remove the mapping since device doesn't exist anymore
				if err := sm.sqlStore.DeleteUserDeviceMapping(userID); err != nil {
					logger.Warn("Failed to remove obsolete mapping", "user_id", userID, "error", err)
				}

				// Create a new device
				devicePtr = container.NewDevice() // Store the pointer directly
			}
		}
	} else {
		// If no mapping exists, create a new device
		logger.Debug("Creating new device", "user_id", userID)
		devicePtr = container.NewDevice() // Store the pointer directly
	}

	// Initialize WhatsApp client
	client := whatsmeow.NewClient(devicePtr, sm.logger) // Use the pointer directly

	// Register event handler
	client.AddEventHandler(func(evt interface{}) {
		sm.processEvent(userID, evt)
	})

	// Store client
	newClient := &Client{
		ID:         userID,
		WAClient:   client,
		Connected:  false,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	sm.clients[userID] = newClient
	logger.Debug("Session created", "user_id", userID)

	return newClient, nil
}

// GetSession gets an existing session
func (sm *SessionManager) GetSession(userID string) (*Client, bool) {
	sm.clientsMutex.RLock()
	defer sm.clientsMutex.RUnlock()

	client, exists := sm.clients[userID]
	return client, exists
}

// Connect starts the WhatsApp connection
func (sm *SessionManager) Connect(ctx context.Context, userID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("session not found: %s", userID)
	}

	// If already connected, return without error
	if client.WAClient.IsConnected() {
		client.Connected = true
		return nil
	}

	// Connect client
	err := client.WAClient.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Wait for connection to establish with timeout
	if !client.WAClient.WaitForConnection(time.Second * 10) {
		return fmt.Errorf("connection timed out")
	}

	client.Connected = true
	return nil
}

// GetQRChannel retorna um canal de eventos QR code para autenticação
// GetQRChannel retorna um canal de eventos QR code para autenticação
func (sm *SessionManager) GetQRChannel(ctx context.Context, userID string) (<-chan whatsmeow.QRChannelItem, error) {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	// Verificar se a sessão existe, caso contrário criar uma nova
	client, exists := sm.clients[userID]
	if !exists {
		// Criar nova sessão
		container := sm.sqlStore.GetDBContainer()
		if container == nil {
			return nil, fmt.Errorf("database container is nil")
		}

		// Criar um novo dispositivo
		deviceStore := container.NewDevice()
		waClient := whatsmeow.NewClient(deviceStore, sm.logger)

		// Registrar event handler
		waClient.AddEventHandler(func(evt interface{}) {
			sm.processEvent(userID, evt)
		})

		// Criar e armazenar o cliente
		client = &Client{
			ID:         userID,
			WAClient:   waClient,
			Connected:  false,
			CreatedAt:  time.Now(),
			LastActive: time.Now(),
		}
		sm.clients[userID] = client
	} else {
		// Se o cliente existe, verificar se não está autenticado
		if client.WAClient.Store.ID != nil {
			return nil, fmt.Errorf("cliente já está autenticado")
		}

		// Se está conectado, desconectar primeiro
		if client.Connected {
			client.WAClient.Disconnect()
			client.Connected = false
			time.Sleep(1 * time.Second) // Pequena pausa para garantir desconexão
		}
	}

	// Obter canal QR
	qrChan, err := client.WAClient.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter canal QR: %w", err)
	}

	// Iniciar conexão em goroutine
	go func() {
		logger.Info("Iniciando conexão para QR code", "user_id", userID)

		// Tentar conectar com retry em caso de falhas
		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			err := client.WAClient.Connect()
			if err == nil {
				client.Connected = true
				client.LastActive = time.Now()
				logger.Info("Cliente conectado com sucesso", "user_id", userID)
				break
			}

			logger.Error("Falha ao conectar cliente",
				"error", err,
				"user_id", userID,
				"retry", i+1,
				"max_retries", maxRetries)

			// Aguardar antes de tentar novamente
			time.Sleep(2 * time.Second)
		}
	}()

	return qrChan, nil
}

// IsLoggedIn checks if a session is authenticated
func (sm *SessionManager) IsLoggedIn(userID string) bool {
	client, exists := sm.GetSession(userID)
	if !exists {
		return false
	}

	return client.WAClient.IsLoggedIn()
}

// Disconnect terminates the WhatsApp connection
func (sm *SessionManager) Disconnect(userID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("session not found: %s", userID)
	}

	// Disconnect only if connected
	if client.WAClient.IsConnected() {
		client.WAClient.Disconnect()
		client.Connected = false
	}

	return nil
}

// DisconnectAll disconnects all sessions
func (sm *SessionManager) DisconnectAll() {
	sm.clientsMutex.RLock()
	defer sm.clientsMutex.RUnlock()

	for userID, client := range sm.clients {
		if client.WAClient.IsConnected() {
			logger.Debug("Disconnecting client", "user_id", userID)
			client.WAClient.Disconnect()
			client.Connected = false
		}
	}
}

// ResetSession reseta uma sessão para permitir nova autenticação via QR code
func (sm *SessionManager) ResetSession(ctx context.Context, userID string) error {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	client, exists := sm.clients[userID]
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Desconectar se conectado
	if client.WAClient.IsConnected() {
		client.WAClient.Disconnect()
	}

	// Remover mapeamento de dispositivo no banco
	if err := sm.sqlStore.DeleteUserDeviceMapping(userID); err != nil {
		logger.Warn("Falha ao remover mapeamento durante reset", "user_id", userID, "error", err)
	}

	// Criar novo dispositivo e cliente
	container := sm.sqlStore.GetDBContainer()
	if container == nil {
		return fmt.Errorf("database container is nil")
	}

	deviceStore := container.NewDevice()
	waClient := whatsmeow.NewClient(deviceStore, sm.logger)

	// Registrar handler de eventos
	waClient.AddEventHandler(func(evt interface{}) {
		sm.processEvent(userID, evt)
	})

	// Substituir o cliente existente
	client.WAClient = waClient
	client.Connected = false
	client.LastActive = time.Now()

	logger.Info("Sessão resetada com sucesso", "user_id", userID)
	return nil
}

// Logout logs out and removes a session
func (sm *SessionManager) Logout(ctx context.Context, userID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("session not found: %s", userID)
	}

	// If connected and logged in, properly logout
	if client.WAClient.IsConnected() && client.WAClient.IsLoggedIn() {
		if err := client.WAClient.Logout(context.Background()); err != nil {
			logger.Warn("Error during logout", "user_id", userID, "error", err)
			// Continue anyway to clean up locally
		}
	}

	// Remove from cache
	sm.clientsMutex.Lock()
	delete(sm.clients, userID)
	sm.clientsMutex.Unlock()

	// Remove the mapping
	if err := sm.sqlStore.DeleteUserDeviceMapping(userID); err != nil {
		logger.Warn("Failed to remove mapping during logout", "user_id", userID, "error", err)
	}

	logger.Debug("Session logged out", "user_id", userID)

	return nil
}

// handleDeviceLogout limpa os dados da sessão quando um dispositivo é desconectado
func (sm *SessionManager) handleDeviceLogout(userID string) error {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	client, exists := sm.clients[userID]
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Desconectar cliente se conectado
	if client.WAClient.IsConnected() {
		client.WAClient.Disconnect()
	}

	// Remover mapeamento do banco de dados
	if err := sm.sqlStore.DeleteUserDeviceMapping(userID); err != nil {
		logger.Warn("Falha ao remover mapeamento de dispositivo", "user_id", userID, "error", err)
	}

	// Obter container do banco de dados
	container := sm.sqlStore.GetDBContainer()
	if container == nil {
		return fmt.Errorf("container de banco de dados é nulo")
	}

	// Criar um novo dispositivo
	deviceStore := container.NewDevice()

	// Criar um novo cliente
	waClient := whatsmeow.NewClient(deviceStore, sm.logger)
	waClient.AddEventHandler(func(evt interface{}) {
		sm.processEvent(userID, evt)
	})

	// Atualizar o cliente
	client.WAClient = waClient
	client.Connected = false
	client.LastActive = time.Now()

	logger.Info("Sessão completamente resetada após logout", "user_id", userID)

	return nil
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(ctx context.Context, userID string) error {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	client, exists := sm.clients[userID]
	if !exists {
		return fmt.Errorf("session not found: %s", userID)
	}

	// Disconnect client
	if client.WAClient.IsConnected() {
		client.WAClient.Disconnect()
	}

	// Remove from cache
	delete(sm.clients, userID)

	// Remove the mapping
	if err := sm.sqlStore.DeleteUserDeviceMapping(userID); err != nil {
		logger.Warn("Failed to remove mapping during session deletion", "user_id", userID, "error", err)
	}

	logger.Debug("Session deleted", "user_id", userID)

	return nil
}

// Close shuts down the session manager and releases resources
func (sm *SessionManager) Close() error {
	// Disconnect all clients
	sm.DisconnectAll()

	// Clear client maps
	sm.clientsMutex.Lock()
	sm.clients = make(map[string]*Client)
	sm.clientsMutex.Unlock()

	return nil
}
