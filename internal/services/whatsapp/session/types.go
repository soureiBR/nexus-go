package session

import (
	"context"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// Client encapsulates the whatsmeow client with additional metadata
type Client struct {
	ID         string
	WAClient   *whatsmeow.Client
	Connected  bool
	CreatedAt  time.Time
	LastActive time.Time
}

// Implement CommunityClient interface
func (c *Client) GetWAClient() *whatsmeow.Client {
	return c.WAClient
}

func (c *Client) IsConnected() bool {
	return c.Connected && c.WAClient != nil && c.WAClient.IsConnected()
}

func (c *Client) UpdateActivity() {
	c.LastActive = time.Now()
}

func (c *Client) GetUserID() string {
	return c.ID
}

// IsActive checks if the client is active and ready for operations
func (c *Client) IsActive() bool {
	return c.IsConnected() && c.WAClient.IsLoggedIn()
}

// Manager defines the interface for session management
type Manager interface {
	// Session lifecycle
	GetSession(userID string) (*Client, bool)
	CreateSession(ctx context.Context, userID string) (*Client, error)
	DeleteSession(ctx context.Context, userID string) error

	// Connection management
	Connect(ctx context.Context, userID string) error
	Disconnect(userID string) error

	// Authentication
	GetQRChannel(ctx context.Context, userID string) (<-chan whatsmeow.QRChannelItem, error)
	Logout(ctx context.Context, userID string) error
	IsLoggedIn(userID string) bool

	// Status and monitoring
	GetSessionStatus(userID string) (map[string]interface{}, error)
	GetAllSessions() map[string]*Client

	// Event handling
	ProcessEvent(userID string, evt interface{})
	RegisterEventHandler(eventType string, handler EventHandler)

	// Lifecycle management
	Close() error
}

// SessionInterface defines the interface for WhatsApp sessions
type SessionInterface interface {
	Connect() error
	Disconnect()
	IsConnected() bool
	GetQRCode() ([]byte, error)
	SendMessage(to, message string) error
}

// EventHandler defines the interface for handling WhatsApp events
type EventHandler func(userID string, evt interface{}) error

// ButtonData represents a button in a message
type ButtonData struct {
	ID          string `json:"id"`
	DisplayText string `json:"displayText"`
}

// Row represents a row in a list message
type Row struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Section represents a section in a list message
type Section struct {
	Title string `json:"title"`
	Rows  []Row  `json:"rows"`
}

// ParseJID parses a JID string
func ParseJID(jid string) (types.JID, error) {
	return types.ParseJID(jid)
}

// CommunityManager defines interface for community-specific operations
type CommunityManager interface {
	GetSession(userID string) (CommunityClient, bool)
}

// CommunityClient represents a client suitable for community operations
type CommunityClient interface {
	GetWAClient() *whatsmeow.Client
	IsConnected() bool
	UpdateActivity()
	GetUserID() string
}

// GroupManager defines interface for group-specific operations
type GroupManager interface {
	GetSession(userID string) (GroupClient, bool)
}

// GroupClient represents a client suitable for group operations
type GroupClient interface {
	GetWAClient() *whatsmeow.Client
	IsConnected() bool
	UpdateActivity()
	GetUserID() string
}
