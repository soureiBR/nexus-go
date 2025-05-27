// internal/services/whatsapp/messaging/types.go
package messaging

import (
	"time"

	"go.mau.fi/whatsmeow"
)

// ClientWrapper wraps whatsmeow client with additional metadata for messaging operations
type ClientWrapper struct {
	WAClient   *whatsmeow.Client
	LastActive time.Time
	Connected  bool
	UserID     string
}

// IsActive checks if the client wrapper is active and connected
func (cw *ClientWrapper) IsActive() bool {
	return cw.Connected && cw.WAClient != nil && cw.WAClient.IsConnected()
}

// UpdateActivity updates the last active timestamp
func (cw *ClientWrapper) UpdateActivity() {
	cw.LastActive = time.Now()
}

// ButtonData representa um botão para mensagens interativas
type ButtonData struct {
	ID   string `json:"id" binding:"required"`
	Text string `json:"text" binding:"required"`
}

// SectionRow representa uma linha em uma seção de lista
type SectionRow struct {
	ID          string `json:"id" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
}

// Section representa uma seção em uma mensagem de lista
type Section struct {
	Title string       `json:"title"`
	Rows  []SectionRow `json:"rows" binding:"required,min=1"`
}

// GroupParticipant representa um participante do grupo
type GroupParticipant struct {
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"is_admin"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

// GroupInfo representa informações de um grupo
type GroupInfo struct {
	JID          string             `json:"jid"`
	Name         string             `json:"name"`
	Topic        string             `json:"topic,omitempty"`
	Created      time.Time          `json:"created"`
	Creator      string             `json:"creator"`
	Participants []GroupParticipant `json:"participants"`
}

// CommunityInfo representa informações de uma comunidade
type CommunityInfo struct {
	JID                string      `json:"jid"`
	Name               string      `json:"name"`
	Description        string      `json:"description,omitempty"`
	Created            time.Time   `json:"created"`
	Creator            string      `json:"creator"`
	LinkedGroups       []GroupInfo `json:"linked_groups"`
	AnnouncementGroups []GroupInfo `json:"announcement_groups"`
}
