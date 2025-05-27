package whatsapp

import (
	"context"
	"fmt"

	"yourproject/internal/services/whatsapp/session"
)

// Service combines session and messaging functionality
// Implements the worker.SessionManager interface
type Service struct {
	sessionManager session.Manager
}

// NewService creates a new unified WhatsApp service
func NewService(sessionManager session.Manager) *Service {
	return &Service{
		sessionManager: sessionManager,
	}
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
