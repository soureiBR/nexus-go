// internal/services/whatsapp/client.go
package whatsapp

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"

	"yourproject/internal/services/whatsapp/messaging"
	"yourproject/internal/services/whatsapp/session"
	"yourproject/internal/services/whatsapp/worker"
	"yourproject/internal/storage"
	"yourproject/pkg/logger"
)

// Client represents a WhatsApp client with worker integration
type Client struct {
	*session.Client
	WorkerID string
}

// SessionManager is the main session manager that integrates with workers
type SessionManager struct {
	sessionManager *session.SessionManager
	coordinator    *Coordinator
}

// NewSessionManager creates a new session manager with worker integration
func NewSessionManager(sqlStore interface{}) *SessionManager {
	// Create the underlying session manager
	sessionMgr := session.NewSessionManager(sqlStore.(*storage.SQLStore))

	// Create coordinator with worker integration
	coord := NewCoordinator(sessionMgr)

	return &SessionManager{
		sessionManager: sessionMgr,
		coordinator:    coord,
	}
}

// GetAllSessions returns all active sessions
func (sm *SessionManager) GetAllSessions() map[string]*Client {
	sessions := sm.sessionManager.GetAllSessions()

	// Convert to Client type with worker information
	result := make(map[string]*Client, len(sessions))
	for id, client := range sessions {
		workerInfo := ""
		if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
			if worker, exists := workerPool.GetWorker(id); exists {
				workerInfo = worker.ID
			}
		}

		result[id] = &Client{
			Client:   client,
			WorkerID: workerInfo,
		}
	}

	return result
}

// GetSession gets an existing session with worker information
func (sm *SessionManager) GetSession(userID string) (*Client, bool) {
	client, exists := sm.sessionManager.GetSession(userID)
	if !exists {
		return nil, false
	}

	workerInfo := ""
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if worker, exists := workerPool.GetWorker(userID); exists {
			workerInfo = worker.ID
		}
	}

	return &Client{
		Client:   client,
		WorkerID: workerInfo,
	}, true
}

// CreateSession creates a new session and initializes worker
func (sm *SessionManager) CreateSession(ctx context.Context, userID string) (*Client, error) {
	// Create session using underlying session manager
	client, err := sm.sessionManager.CreateSession(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Initialize worker for this session
	workerID := ""
	if err := sm.coordinator.CreateWorker(userID); err != nil {
		logger.Warn("Failed to create worker for session", "user_id", userID, "error", err)
	} else {
		// Get the worker ID after successful creation
		if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
			if worker, exists := workerPool.GetWorker(userID); exists {
				workerID = worker.ID
			}
		}
	}

	return &Client{
		Client:   client,
		WorkerID: workerID,
	}, nil
}

// Connect connects a session using worker if available
func (sm *SessionManager) Connect(ctx context.Context, userID string) error {
	// Try to use worker first
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if _, exists := workerPool.GetWorker(userID); exists {
			task := worker.Task{
				ID:       fmt.Sprintf("connect_%s_%d", userID, time.Now().Unix()),
				Type:     worker.CmdConnect,
				UserID:   userID,
				Priority: worker.NormalPriority,
				Created:  time.Now(),
			}
			return workerPool.SubmitTask(task)
		}
	}

	// Fallback to direct session manager
	return sm.sessionManager.Connect(ctx, userID)
}

// Disconnect disconnects a session using worker if available
func (sm *SessionManager) Disconnect(userID string) error {
	// Try to use worker first
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if _, exists := workerPool.GetWorker(userID); exists {
			task := worker.Task{
				ID:       fmt.Sprintf("disconnect_%s_%d", userID, time.Now().Unix()),
				Type:     worker.CmdDisconnect,
				UserID:   userID,
				Priority: worker.NormalPriority,
				Created:  time.Now(),
			}
			return workerPool.SubmitTask(task)
		}
	}

	// Fallback to direct session manager
	return sm.sessionManager.Disconnect(userID)
}

// GetQRChannel gets QR channel for authentication
func (sm *SessionManager) GetQRChannel(ctx context.Context, userID string) (<-chan whatsmeow.QRChannelItem, error) {
	return sm.sessionManager.GetQRChannel(ctx, userID)
}

// Logout logs out a session using worker if available
func (sm *SessionManager) Logout(ctx context.Context, userID string) error {
	// Try to use worker first
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if _, exists := workerPool.GetWorker(userID); exists {
			task := worker.Task{
				ID:       fmt.Sprintf("logout_%s_%d", userID, time.Now().Unix()),
				Type:     worker.CmdLogout,
				UserID:   userID,
				Priority: worker.HighPriority,
				Created:  time.Now(),
			}
			if err := workerPool.SubmitTask(task); err == nil {
				// Also remove the worker after logout
				sm.coordinator.RemoveWorker(userID)
				return nil
			}
		}
	}

	// Fallback to direct session manager
	if err := sm.sessionManager.Logout(ctx, userID); err != nil {
		return err
	}

	// Remove worker if exists
	sm.coordinator.RemoveWorker(userID)
	return nil
}

// DeleteSession removes a session and its worker
func (sm *SessionManager) DeleteSession(ctx context.Context, userID string) error {
	// Remove worker first
	sm.coordinator.RemoveWorker(userID)

	// Then delete session
	return sm.sessionManager.DeleteSession(ctx, userID)
}

// ResetSession resets a session for new authentication
func (sm *SessionManager) ResetSession(ctx context.Context, userID string) error {
	// Remove worker first
	sm.coordinator.RemoveWorker(userID)

	// Reset session
	if err := sm.sessionManager.ResetSession(ctx, userID); err != nil {
		return err
	}

	// Try to create new worker
	if err := sm.coordinator.CreateWorker(userID); err != nil {
		logger.Warn("Failed to create worker after reset", "user_id", userID, "error", err)
	}

	return nil
}

// IsLoggedIn checks if session is authenticated
func (sm *SessionManager) IsLoggedIn(userID string) bool {
	return sm.sessionManager.IsLoggedIn(userID)
}

// GetSessionStatus gets session status including worker information
func (sm *SessionManager) GetSessionStatus(userID string) (map[string]interface{}, error) {
	// Get basic session status
	status, err := sm.sessionManager.GetSessionStatus(userID)
	if err != nil {
		return nil, err
	}

	// Add worker information
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if worker, exists := workerPool.GetWorker(userID); exists {
			status["worker_id"] = worker.ID
			status["worker_status"] = worker.GetStatus().String()
			status["worker_metrics"] = worker.GetMetrics()
			status["has_worker"] = true
		} else {
			status["has_worker"] = false
		}
	} else {
		status["has_worker"] = false
	}

	return status, nil
}

// InitWorker initializes a worker for a session
func (sm *SessionManager) InitWorker(userID string) (*worker.Worker, error) {
	// Create worker through coordinator
	if err := sm.coordinator.CreateWorker(userID); err != nil {
		return nil, err
	}

	// Return the actual worker instance
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if w, exists := workerPool.GetWorker(userID); exists {
			return w, nil
		}
	}

	return nil, fmt.Errorf("worker created but not found in pool")
}

// StopWorker stops a worker for a session
func (sm *SessionManager) StopWorker(userID string) error {
	return sm.coordinator.RemoveWorker(userID)
}

// GetCoordinator returns the coordinator for advanced operations
func (sm *SessionManager) GetCoordinator() *Coordinator {
	return sm.coordinator
}

// AutoInitWorkers initializes workers for all existing sessions
func (sm *SessionManager) AutoInitWorkers() error {
	return sm.coordinator.AutoInitWorkers()
}

// RegisterEventHandler registers an event handler
func (sm *SessionManager) RegisterEventHandler(eventType string, handler session.EventHandler) {
	sm.coordinator.RegisterEventHandler(eventType, handler)
}

// ProcessEvent processes an event
func (sm *SessionManager) ProcessEvent(userID string, evt interface{}) {
	sm.sessionManager.ProcessEvent(userID, evt)
}

// DisconnectAll disconnects all sessions
func (sm *SessionManager) DisconnectAll() {
	sm.sessionManager.DisconnectAll()
}

// Close closes the session manager and all workers
func (sm *SessionManager) Close() error {
	// Stop coordinator (which stops worker pool)
	if err := sm.coordinator.Stop(); err != nil {
		logger.Error("Error stopping coordinator", "error", err)
	}

	// Close session manager
	return sm.sessionManager.Close()
}

// StartCoordinator starts the coordinator system
func (sm *SessionManager) StartCoordinator() error {
	return sm.coordinator.Start()
}

// StopCoordinator stops the coordinator system
func (sm *SessionManager) StopCoordinator() error {
	return sm.coordinator.Stop()
}

// InitSessions initializes all sessions from storage
func (sm *SessionManager) InitSessions(ctx context.Context) error {
	return sm.sessionManager.InitSessions(ctx)
}

// StartPeriodicCleanup starts periodic cleanup
func (sm *SessionManager) StartPeriodicCleanup(interval, threshold time.Duration) {
	sm.sessionManager.StartPeriodicCleanup(interval, threshold)
}

// StopPeriodicCleanup stops periodic cleanup
func (sm *SessionManager) StopPeriodicCleanup() {
	sm.sessionManager.StopPeriodicCleanup()
}

// Worker-integrated utility methods

// SendEventToWorker sends an event to a specific worker
func (sm *SessionManager) SendEventToWorker(userID string, evt interface{}) error {
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		return workerPool.SendEvent(userID, evt)
	}
	return fmt.Errorf("worker pool not available")
}

// GetWorkerMetrics gets metrics for a specific worker
func (sm *SessionManager) GetWorkerMetrics(userID string) (worker.WorkerMetrics, error) {
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		if w, exists := workerPool.GetWorker(userID); exists {
			return w.GetMetrics(), nil
		}
	}
	return worker.WorkerMetrics{}, fmt.Errorf("worker not found for user: %s", userID)
}

// GetPoolMetrics gets overall pool metrics
func (sm *SessionManager) GetPoolMetrics() worker.PoolMetrics {
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		return workerPool.GetMetrics()
	}
	return worker.PoolMetrics{}
}

// ListWorkers lists all active workers
func (sm *SessionManager) ListWorkers() map[string]worker.WorkerInfo {
	if workerPool := sm.coordinator.GetWorkerPool(); workerPool != nil {
		return workerPool.ListWorkers()
	}
	return make(map[string]worker.WorkerInfo)
}

// NewNewsletterService creates a new newsletter service using the coordinator pattern
func NewNewsletterService(sessionManager *SessionManager) *messaging.NewsletterService {
	// Use the coordinator's newsletter service instead of creating a new one
	return sessionManager.coordinator.newsletterService
}

// Messaging methods for worker integration
func (sm *SessionManager) SendText(userID, to, message string) (string, error) {
	// Get the underlying message service
	messageService := messaging.NewMessageService(sm.sessionManager)
	return messageService.SendText(userID, to, message)
}

func (sm *SessionManager) SendMedia(userID, to, mediaURL, mediaType, caption string) (string, error) {
	// Get the underlying message service
	messageService := messaging.NewMessageService(sm.sessionManager)
	return messageService.SendMedia(userID, to, mediaURL, mediaType, caption)
}

func (sm *SessionManager) SendButtons(userID, to, text, footer string, buttons []worker.ButtonData) (string, error) {
	// Get the underlying message service - now uses worker types directly
	messageService := messaging.NewMessageService(sm.sessionManager)
	return messageService.SendButtons(userID, to, text, footer, buttons)
}

func (sm *SessionManager) SendList(userID, to, text, footer, buttonText string, sections []worker.Section) (string, error) {
	// Get the underlying message service - now uses worker types directly
	messageService := messaging.NewMessageService(sm.sessionManager)
	return messageService.SendList(userID, to, text, footer, buttonText, sections)
}

// Newsletter methods for worker integration
func (sm *SessionManager) CreateChannel(userID, name, description, pictureURL string) (interface{}, error) {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().CreateChannel(userID, name, description, pictureURL)
}

func (sm *SessionManager) GetChannelInfo(userID, jid string) (interface{}, error) {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().GetChannelInfo(userID, jid)
}

func (sm *SessionManager) GetChannelWithInvite(userID, inviteLink string) (interface{}, error) {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().GetChannelWithInvite(userID, inviteLink)
}

func (sm *SessionManager) ListMyChannels(userID string) (interface{}, error) {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().ListMyChannels(userID)
}

func (sm *SessionManager) FollowChannel(userID, jid string) error {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().FollowChannel(userID, jid)
}

func (sm *SessionManager) UnfollowChannel(userID, jid string) error {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().UnfollowChannel(userID, jid)
}

func (sm *SessionManager) MuteChannel(userID, jid string) error {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().MuteChannel(userID, jid)
}

func (sm *SessionManager) UnmuteChannel(userID, jid string) error {
	// Use the coordinator's newsletter service directly
	return sm.coordinator.GetNewsletterService().UnmuteChannel(userID, jid)
}
