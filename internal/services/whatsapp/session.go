package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"yourproject/internal/storage"
	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow"
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

// InitSessions loads all existing sessions from the database
func (sm *SessionManager) InitSessions(ctx context.Context) error {
	// Get all userID -> deviceJID mappings
	mappings, err := sm.sqlStore.GetAllUserDeviceMappings()
	if err != nil {
		return fmt.Errorf("failed to load userID mappings: %w", err)
	}

	logger.Info("Loading existing sessions", "count", len(mappings))

	for _, mapping := range mappings {
		logger.Debug("Restoring session", "user_id", mapping.UserID, "device_jid", mapping.DeviceJID)

		// Try to create a session for each mapping
		client, err := sm.CreateSession(ctx, mapping.UserID)
		if err != nil {
			logger.Error("Failed to restore session", "user_id", mapping.UserID, "error", err)
			continue
		}

		// Update last active time
		client.LastActive = time.Now()
	}

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

// GetQRChannel returns a QR code channel for authentication
func (sm *SessionManager) GetQRChannel(ctx context.Context, userID string) (<-chan whatsmeow.QRChannelItem, error) {
	// Get or create session
	client, exists := sm.GetSession(userID)
	if !exists {
		var err error
		client, err = sm.CreateSession(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}
	}

	// Check if client is already connected
	if client.WAClient.IsConnected() {
		return nil, fmt.Errorf("client is already connected")
	}

	// Check if a valid session already exists
	if client.WAClient.Store.ID != nil {
		return nil, fmt.Errorf("session is already authenticated, QR code not needed")
	}

	// Get QR channel
	qrChan, err := client.WAClient.GetQRChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	// Connect client in a separate goroutine
	go func() {
		logger.Debug("Attempting to connect client", "user_id", userID)
		err := client.WAClient.Connect()
		if err != nil {
			logger.Error("Failed to connect client", "error", err, "user_id", userID)
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
