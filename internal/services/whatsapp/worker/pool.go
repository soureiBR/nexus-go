package worker

import (
	"fmt"
	"sync"
	"time"

	"yourproject/pkg/logger"
)

// DefaultWorkerConfig retorna a configuração padrão para workers
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		TaskQueueSize:  100,
		EventQueueSize: 1000,
		WorkerTimeout:  30 * time.Second,
		MaxWorkers:     50,
		MinWorkers:     1,
		IdleTimeout:    30 * time.Minute,
		ProcessTimeout: 60 * time.Second,
	}
}

// PoolConfig contém as configurações específicas do pool
type PoolConfig struct {
	MaxWorkersPerSession int           `json:"max_workers_per_session"`
	HealthCheckInterval  time.Duration `json:"health_check_interval"`
	CleanupInterval      time.Duration `json:"cleanup_interval"`
	MaxRetries           int           `json:"max_retries"`
	TaskTimeout          time.Duration `json:"task_timeout"`
}

// DefaultPoolConfig retorna a configuração padrão do pool
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxWorkersPerSession: 3,
		HealthCheckInterval:  30 * time.Second,
		CleanupInterval:      5 * time.Minute,
		MaxRetries:           3,
		TaskTimeout:          60 * time.Second,
	}
}

// PoolMetrics coleta métricas do pool de workers
type PoolMetrics struct {
	ActiveWorkers   int
	TotalWorkers    int
	PendingTasks    int
	CompletedTasks  int64
	FailedTasks     int64
	AverageTaskTime time.Duration
	StartTime       time.Time
	mu              sync.RWMutex
}

// GetSnapshot retorna um snapshot das métricas
func (pm *PoolMetrics) GetSnapshot() PoolMetrics {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Return a copy without the mutex
	return PoolMetrics{
		ActiveWorkers:   pm.ActiveWorkers,
		TotalWorkers:    pm.TotalWorkers,
		PendingTasks:    pm.PendingTasks,
		CompletedTasks:  pm.CompletedTasks,
		FailedTasks:     pm.FailedTasks,
		AverageTaskTime: pm.AverageTaskTime,
		StartTime:       pm.StartTime,
	}
}

// WorkerPool gerencia um pool de workers
type WorkerPool struct {
	workers    map[string]*Worker
	workersMu  sync.RWMutex
	metrics    *PoolMetrics
	config     *WorkerConfig
	poolConfig *PoolConfig

	// Task scheduling
	taskQueues map[TaskPriority]chan Task
	scheduler  *TaskScheduler

	// Dependencies
	sessionManager SessionManager
	coordinator    Coordinator

	// Services
	communityService  CommunityServiceInterface
	groupService      GroupServiceInterface
	messageService    MessageServiceInterface
	newsletterService NewsletterServiceInterface

	// Lifecycle
	done      chan struct{}
	wg        sync.WaitGroup
	isRunning bool
	mu        sync.RWMutex
}

// NewWorkerPool cria um novo pool de workers
func NewWorkerPool(sessionMgr SessionManager, coord Coordinator, config *WorkerConfig) *WorkerPool {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	pool := &WorkerPool{
		workers:        make(map[string]*Worker),
		metrics:        &PoolMetrics{StartTime: time.Now()},
		config:         config,
		poolConfig:     DefaultPoolConfig(),
		taskQueues:     make(map[TaskPriority]chan Task),
		sessionManager: sessionMgr,
		coordinator:    coord,
		done:           make(chan struct{}),
	}

	// Inicializar filas de tarefas por prioridade
	for priority := LowPriority; priority <= CriticalPriority; priority++ {
		pool.taskQueues[priority] = make(chan Task, config.TaskQueueSize)
	}

	pool.scheduler = NewTaskScheduler(pool.taskQueues, pool)

	return pool
}

// NewWorkerPoolWithServices cria um novo pool de workers com serviços específicos
func NewWorkerPoolWithServices(sessionMgr SessionManager, coord Coordinator, communityService CommunityServiceInterface, groupService GroupServiceInterface, newsletterService NewsletterServiceInterface, config *WorkerConfig) *WorkerPool {
	if config == nil {
		config = DefaultWorkerConfig()
	}

	pool := &WorkerPool{
		workers:           make(map[string]*Worker),
		metrics:           &PoolMetrics{StartTime: time.Now()},
		config:            config,
		poolConfig:        DefaultPoolConfig(),
		taskQueues:        make(map[TaskPriority]chan Task),
		sessionManager:    sessionMgr,
		coordinator:       coord,
		communityService:  communityService,
		groupService:      groupService,
		newsletterService: newsletterService,
		done:              make(chan struct{}),
	}

	// Inicializar filas de tarefas por prioridade
	for priority := LowPriority; priority <= CriticalPriority; priority++ {
		pool.taskQueues[priority] = make(chan Task, config.TaskQueueSize)
	}

	pool.scheduler = NewTaskScheduler(pool.taskQueues, pool)

	return pool
}

// Start inicia o pool de workers
func (wp *WorkerPool) Start() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.isRunning {
		return fmt.Errorf("worker pool já está rodando")
	}

	wp.isRunning = true

	// Iniciar scheduler
	if err := wp.scheduler.Start(); err != nil {
		return fmt.Errorf("falha ao iniciar scheduler: %w", err)
	}

	// Iniciar rotinas de manutenção
	wp.wg.Add(2)
	go wp.healthChecker()
	go wp.cleaner()

	logger.Info("Worker pool iniciado")
	return nil
}

// Stop para o pool de workers
func (wp *WorkerPool) Stop() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.isRunning {
		return nil
	}

	logger.Info("Parando worker pool")
	wp.isRunning = false

	// Parar scheduler
	wp.scheduler.Stop()

	// Parar todos os workers
	wp.workersMu.Lock()
	workers := make([]*Worker, 0, len(wp.workers))
	for _, worker := range wp.workers {
		workers = append(workers, worker)
	}
	wp.workersMu.Unlock()

	// Parar workers em paralelo
	var workerWg sync.WaitGroup
	for _, worker := range workers {
		workerWg.Add(1)
		go func(w *Worker) {
			defer workerWg.Done()
			w.Stop()
		}(worker)
	}
	workerWg.Wait()

	// Sinalizar para parar e aguardar rotinas de manutenção
	close(wp.done)
	wp.wg.Wait()

	logger.Info("Worker pool parado")
	return nil
}

// CreateWorker cria um novo worker para um usuário
func (wp *WorkerPool) CreateWorker(userID string, workerType WorkerType) (*Worker, error) {
	wp.workersMu.Lock()
	defer wp.workersMu.Unlock()

	// Verificar se já existe um worker para este usuário
	if existingWorker, exists := wp.workers[userID]; exists {
		if existingWorker.IsRunning() {
			return existingWorker, nil
		}
		// Se existe mas não está rodando, remover
		delete(wp.workers, userID)
	}

	// Verificar limite de workers
	if len(wp.workers) >= wp.poolConfig.MaxWorkersPerSession*10 { // Limite geral
		return nil, fmt.Errorf("limite de workers atingido")
	}

	// Criar novo worker
	workerID := fmt.Sprintf("worker_%s_%d", userID, time.Now().Unix())

	worker := NewWorker(workerID, userID, workerType, wp.sessionManager, wp.coordinator, wp.communityService, wp.groupService, wp.messageService, wp.newsletterService, wp.config)

	// Iniciar worker
	if err := worker.Start(); err != nil {
		return nil, fmt.Errorf("falha ao iniciar worker: %w", err)
	}

	// Armazenar worker
	wp.workers[userID] = worker

	// Atualizar métricas
	wp.metrics.mu.Lock()
	wp.metrics.ActiveWorkers++
	wp.metrics.TotalWorkers++
	wp.metrics.mu.Unlock()

	logger.Info("Worker criado", "worker_id", workerID, "user_id", userID, "type", workerType)
	return worker, nil
}

// GetWorker retorna o worker para um usuário
func (wp *WorkerPool) GetWorker(userID string) (*Worker, bool) {
	wp.workersMu.RLock()
	defer wp.workersMu.RUnlock()

	worker, exists := wp.workers[userID]
	if !exists || !worker.IsRunning() {
		return nil, false
	}

	return worker, true
}

// RemoveWorker remove um worker
func (wp *WorkerPool) RemoveWorker(userID string) error {
	wp.workersMu.Lock()
	defer wp.workersMu.Unlock()

	worker, exists := wp.workers[userID]
	if !exists {
		return fmt.Errorf("worker não encontrado: %s", userID)
	}

	// Parar worker
	if err := worker.Stop(); err != nil {
		logger.Warn("Erro ao parar worker", "user_id", userID, "error", err)
	}

	// Remover do mapa
	delete(wp.workers, userID)

	// Atualizar métricas
	wp.metrics.mu.Lock()
	wp.metrics.ActiveWorkers--
	wp.metrics.mu.Unlock()

	logger.Info("Worker removido", "user_id", userID)
	return nil
}

// SubmitTask submete uma tarefa para execução
func (wp *WorkerPool) SubmitTask(task Task) error {
	if !wp.IsRunning() {
		return fmt.Errorf("worker pool não está rodando")
	}

	// Verificar se existe worker para o usuário
	worker, exists := wp.GetWorker(task.UserID)
	if !exists {
		return fmt.Errorf("worker não encontrado para usuário: %s", task.UserID)
	}

	// Enviar tarefa para o worker
	return worker.SendTask(task)
}

// SendEvent envia um evento para o worker de um usuário
func (wp *WorkerPool) SendEvent(userID string, evt interface{}) error {
	worker, exists := wp.GetWorker(userID)
	if !exists {
		return fmt.Errorf("worker não encontrado para usuário: %s", userID)
	}

	worker.SendEvent(evt)
	return nil
}

// GetMetrics retorna as métricas do pool
func (wp *WorkerPool) GetMetrics() PoolMetrics {
	return wp.metrics.GetSnapshot()
}

// ListWorkers retorna informações de todos os workers
func (wp *WorkerPool) ListWorkers() map[string]WorkerInfo {
	wp.workersMu.RLock()
	defer wp.workersMu.RUnlock()

	workers := make(map[string]WorkerInfo)
	for userID, worker := range wp.workers {
		workers[userID] = WorkerInfo{
			ID:      worker.ID,
			UserID:  worker.UserID,
			Type:    worker.Type,
			Status:  worker.GetStatus(),
			Metrics: worker.GetMetrics(),
		}
	}

	return workers
}

// IsRunning retorna se o pool está rodando
func (wp *WorkerPool) IsRunning() bool {
	wp.mu.RLock()
	defer wp.mu.RUnlock()
	return wp.isRunning
}

// healthChecker monitora a saúde dos workers
func (wp *WorkerPool) healthChecker() {
	defer wp.wg.Done()

	ticker := time.NewTicker(wp.poolConfig.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wp.done:
			return
		case <-ticker.C:
			wp.performHealthCheck()
		}
	}
}

// cleaner limpa workers inativos
func (wp *WorkerPool) cleaner() {
	defer wp.wg.Done()

	ticker := time.NewTicker(wp.poolConfig.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wp.done:
			return
		case <-ticker.C:
			wp.performCleanup()
		}
	}
}

// performHealthCheck verifica a saúde dos workers
func (wp *WorkerPool) performHealthCheck() {
	wp.workersMu.RLock()
	workers := make([]*Worker, 0, len(wp.workers))
	for _, worker := range wp.workers {
		workers = append(workers, worker)
	}
	wp.workersMu.RUnlock()

	for _, worker := range workers {
		if !worker.IsRunning() {
			logger.Warn("Worker não está rodando durante health check", "worker_id", worker.ID, "user_id", worker.UserID)
			wp.RemoveWorker(worker.UserID)
		}
	}
}

// performCleanup limpa workers inativos
func (wp *WorkerPool) performCleanup() {
	wp.workersMu.RLock()
	inactiveWorkers := make([]string, 0)

	for userID, worker := range wp.workers {
		metrics := worker.GetMetrics()
		if time.Since(metrics.LastTaskTime) > 30*time.Minute && worker.GetStatus() == StatusIdle {
			inactiveWorkers = append(inactiveWorkers, userID)
		}
	}
	wp.workersMu.RUnlock()

	for _, userID := range inactiveWorkers {
		logger.Info("Removendo worker inativo", "user_id", userID)
		wp.RemoveWorker(userID)
	}
}
