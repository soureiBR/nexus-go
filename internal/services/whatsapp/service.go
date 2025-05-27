package whatsapp

import (
	"context"
	"fmt"

	"yourproject/internal/services/whatsapp/messaging"
	"yourproject/internal/services/whatsapp/session"
	"yourproject/internal/services/whatsapp/worker"
)

// Service combines session and messaging functionality
// Implements the worker.SessionManager interface
type Service struct {
	sessionManager session.Manager
	messageService *messaging.MessageService
}

// NewService creates a new unified WhatsApp service
func NewService(sessionManager session.Manager) *Service {
	return &Service{
		sessionManager: sessionManager,
		messageService: messaging.NewMessageService(sessionManager),
	}
}

// Messaging operations - delegate to MessageService
func (s *Service) SendText(userID, to, message string) (string, error) {
	return s.messageService.SendText(userID, to, message)
}

func (s *Service) SendMedia(userID, to, mediaURL, mediaType, caption string) (string, error) {
	return s.messageService.SendMedia(userID, to, mediaURL, mediaType, caption)
}

func (s *Service) SendButtons(userID, to, text, footer string, buttons []worker.ButtonData) (string, error) {
	// Convert worker.ButtonData to messaging.ButtonData
	msgButtons := make([]messaging.ButtonData, len(buttons))
	for i, btn := range buttons {
		msgButtons[i] = messaging.ButtonData{
			ID:   btn.ID,
			Text: btn.DisplayText, // Note: worker uses DisplayText, messaging uses Text
		}
	}
	return s.messageService.SendButtons(userID, to, text, footer, msgButtons)
}

func (s *Service) SendList(userID, to, text, footer, buttonText string, sections []worker.Section) (string, error) {
	// Convert worker.Section to messaging.Section
	msgSections := make([]messaging.Section, len(sections))
	for i, section := range sections {
		rows := make([]messaging.SectionRow, len(section.Rows))
		for j, row := range section.Rows {
			rows[j] = messaging.SectionRow{
				ID:          row.ID,
				Title:       row.Title,
				Description: row.Description,
			}
		}
		msgSections[i] = messaging.Section{
			Title: section.Title,
			Rows:  rows,
		}
	}
	return s.messageService.SendList(userID, to, text, footer, buttonText, msgSections)
}

// Session operations - delegate to SessionManager
func (s *Service) Connect(ctx context.Context, userID string) error {
	client, exists := s.sessionManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	return client.WAClient.Connect()
}

func (s *Service) Disconnect(userID string) error {
	client, exists := s.sessionManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	client.WAClient.Disconnect()
	client.Connected = false
	return nil
}

func (s *Service) GetQRChannel(ctx context.Context, userID string) (<-chan interface{}, error) {
	qrChan, err := s.sessionManager.GetQRChannel(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert to interface{} channel
	resultChan := make(chan interface{}, 100)
	go func() {
		defer close(resultChan)
		for item := range qrChan {
			resultChan <- item
		}
	}()

	return resultChan, nil
}

func (s *Service) Logout(ctx context.Context, userID string) error {
	return s.sessionManager.Logout(ctx, userID)
}

func (s *Service) GetSessionStatus(userID string) (map[string]interface{}, error) {
	client, exists := s.sessionManager.GetSession(userID)
	if !exists {
		return map[string]interface{}{
			"connected": false,
			"exists":    false,
		}, nil
	}

	return map[string]interface{}{
		"connected":   client.Connected,
		"exists":      true,
		"created_at":  client.CreatedAt,
		"last_active": client.LastActive,
	}, nil
}
