package whatsapp

import (
	"context"
	"fmt"
	"sync"

	"yourproject/internal/services/whatsapp/messaging"
	"yourproject/internal/services/whatsapp/session"
	"yourproject/internal/services/whatsapp/worker"
	"yourproject/pkg/logger"
)

// Coordinator orchestrates high-level WhatsApp operations with proper separation of concerns
type Coordinator struct {
	sessionManager    session.Manager
	messageService    *messaging.MessageService
	communityService  *messaging.CommunityService
	groupService      *messaging.GroupService
	newsletterService *messaging.NewsletterService
	workerPool        *worker.WorkerPool
	eventHandlers     map[string][]session.EventHandler
	mu                sync.RWMutex
}

// NewCoordinator creates a new coordinator with proper dependency injection
func NewCoordinator(sessionMgr session.Manager) *Coordinator {
	coord := &Coordinator{
		sessionManager: sessionMgr,
		messageService: messaging.NewMessageService(sessionMgr),
		eventHandlers:  make(map[string][]session.EventHandler),
	}

	// Create community service with session manager that implements CommunityManager
	communityManager := &CommunityManagerAdapter{sessionManager: sessionMgr}
	coord.communityService = messaging.NewCommunityService(communityManager)

	// Create group service with session manager that implements GroupManager
	groupManager := &GroupManagerAdapter{sessionManager: sessionMgr}
	coord.groupService = messaging.NewGroupService(groupManager)

	// Create newsletter service with session manager
	coord.newsletterService = messaging.NewNewsletterService(sessionMgr)

	// Create message service with session manager
	coord.messageService = messaging.NewMessageService(sessionMgr)

	// Create unified service that implements worker.SessionManager (messaging and session operations only)
	unifiedService := &UnifiedWhatsAppService{
		sessionManager: sessionMgr,
		messageService: coord.messageService,
	}

	// Create worker pool with unified service as SessionManager, and separate services for specialized operations
	coord.workerPool = worker.NewWorkerPool(unifiedService, coord, worker.DefaultConfig())

	return coord
}

// CommunityManagerAdapter adapts session.Manager to work with messaging.CommunityService
type CommunityManagerAdapter struct {
	sessionManager session.Manager
}

// GetSession implements session.CommunityManager interface
func (cma *CommunityManagerAdapter) GetSession(userID string) (session.CommunityClient, bool) {
	client, exists := cma.sessionManager.GetSession(userID)
	if !exists {
		return nil, false
	}

	// session.Client already implements session.CommunityClient interface
	return client, true
}

// GroupManagerAdapter adapts session.Manager to work with messaging.GroupService
type GroupManagerAdapter struct {
	sessionManager session.Manager
}

// GetSession implements session.GroupManager interface
func (gma *GroupManagerAdapter) GetSession(userID string) (session.GroupClient, bool) {
	client, exists := gma.sessionManager.GetSession(userID)
	if !exists {
		return nil, false
	}

	// session.Client already implements session.GroupClient interface
	return client, true
}

// UnifiedWhatsAppService combines session and messaging functionality
// This implements worker.SessionManager interface (without newsletter methods)
type UnifiedWhatsAppService struct {
	sessionManager session.Manager
	messageService *messaging.MessageService
}

func (s *UnifiedWhatsAppService) Connect(ctx interface{}, userID string) error {
	// Convert interface{} to context.Context
	context, ok := ctx.(context.Context)
	if !ok {
		return fmt.Errorf("invalid context type")
	}
	return s.sessionManager.Connect(context, userID)
}

func (s *UnifiedWhatsAppService) Disconnect(userID string) error {
	return s.sessionManager.Disconnect(userID)
}

func (s *UnifiedWhatsAppService) GetQRChannel(ctx interface{}, userID string) (interface{}, error) {
	// Convert interface{} to context.Context
	context, ok := ctx.(context.Context)
	if !ok {
		return nil, fmt.Errorf("invalid context type")
	}

	qrChan, err := s.sessionManager.GetQRChannel(context, userID)
	if err != nil {
		return nil, err
	}

	// Convert channel type safely using buffered channel
	convertedChan := make(chan interface{}, 10)
	go func() {
		defer close(convertedChan)
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in QR channel conversion", "error", r, "user_id", userID)
			}
		}()

		for item := range qrChan {
			select {
			case convertedChan <- item:
			default:
				logger.Warn("QR channel buffer full, dropping item", "user_id", userID)
			}
		}
	}()

	return convertedChan, nil
}

func (s *UnifiedWhatsAppService) Logout(ctx interface{}, userID string) error {
	// Convert interface{} to context.Context
	context, ok := ctx.(context.Context)
	if !ok {
		return fmt.Errorf("invalid context type")
	}
	return s.sessionManager.Logout(context, userID)
}

func (s *UnifiedWhatsAppService) GetSessionStatus(userID string) (map[string]interface{}, error) {
	return s.sessionManager.GetSessionStatus(userID)
}

// GetCommunityService returns the community service
func (c *Coordinator) GetCommunityService() *messaging.CommunityService {
	return c.communityService
}

// GetGroupService returns the group service (for worker pool integration)
func (c *Coordinator) GetGroupService() *messaging.GroupService {
	return c.groupService
}

// GetNewsletterService returns the newsletter service
func (c *Coordinator) GetNewsletterService() *messaging.NewsletterService {
	return c.newsletterService
}

// GetMessageService returns the message service
func (c *Coordinator) GetMessageService() *messaging.MessageService {
	return c.messageService
}

// GetWorkerPool returns the worker pool
func (c *Coordinator) GetWorkerPool() *worker.WorkerPool {
	return c.workerPool
}

// CreateWorker creates a new worker for a user
func (c *Coordinator) CreateWorker(userID string) error {
	if c.workerPool == nil {
		return fmt.Errorf("worker pool not initialized")
	}

	_, err := c.workerPool.CreateWorker(userID, worker.DefaultWorkerType)
	return err
}

// RemoveWorker removes a worker for a user
func (c *Coordinator) RemoveWorker(userID string) error {
	if c.workerPool == nil {
		return fmt.Errorf("worker pool not initialized")
	}

	return c.workerPool.RemoveWorker(userID)
}

// Start starts the coordinator and its components
func (c *Coordinator) Start() error {
	if c.workerPool != nil {
		return c.workerPool.Start()
	}
	return nil
}

// Stop stops the coordinator and its components
func (c *Coordinator) Stop() error {
	if c.workerPool != nil {
		return c.workerPool.Stop()
	}
	return nil
}

// AutoInitWorkers initializes workers for all existing sessions
func (c *Coordinator) AutoInitWorkers() error {
	// This would typically iterate through all active sessions
	// and create workers for them
	logger.Info("Auto-initializing workers for existing sessions")
	return nil
}

// RegisterEventHandler registers an event handler
func (c *Coordinator) RegisterEventHandler(eventType string, handler session.EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.eventHandlers[eventType] == nil {
		c.eventHandlers[eventType] = make([]session.EventHandler, 0)
	}
	c.eventHandlers[eventType] = append(c.eventHandlers[eventType], handler)
}

// ProcessEvent processes an event for a user
func (c *Coordinator) ProcessEvent(userID string, evt interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Process through registered handlers
	for eventType, handlers := range c.eventHandlers {
		for _, handler := range handlers {
			if err := handler(userID, evt); err != nil {
				logger.Warn("Event handler failed", "event_type", eventType, "user_id", userID, "error", err)
			}
		}
	}

	return nil
}

// NotifyWorkerStatus implements worker.Coordinator interface
func (c *Coordinator) NotifyWorkerStatus(userID string, workerID string, status worker.WorkerStatus) {
	logger.Debug("Worker status changed", "user_id", userID, "worker_id", workerID, "status", status.String())
}
