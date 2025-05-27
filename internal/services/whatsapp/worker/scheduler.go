package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"yourproject/pkg/logger"
)

// TaskScheduler gerencia o agendamento e distribuição de tarefas
type TaskScheduler struct {
	taskQueues map[TaskPriority]chan Task
	pool       *WorkerPool

	// Lifecycle
	done      chan struct{}
	wg        sync.WaitGroup
	isRunning bool
	mu        sync.RWMutex
}

// NewTaskScheduler cria um novo scheduler de tarefas
func NewTaskScheduler(taskQueues map[TaskPriority]chan Task, pool *WorkerPool) *TaskScheduler {
	return &TaskScheduler{
		taskQueues: taskQueues,
		pool:       pool,
		done:       make(chan struct{}),
	}
}

// Start inicia o scheduler
func (ts *TaskScheduler) Start() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.isRunning {
		return fmt.Errorf("task scheduler já está rodando")
	}

	ts.isRunning = true

	// Iniciar processadores para cada prioridade
	ts.wg.Add(int(CriticalPriority) + 1)
	for priority := LowPriority; priority <= CriticalPriority; priority++ {
		go ts.processTaskQueue(priority)
	}

	logger.Info("Task scheduler iniciado")
	return nil
}

// Stop para o scheduler
func (ts *TaskScheduler) Stop() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if !ts.isRunning {
		return nil
	}

	logger.Info("Parando task scheduler")
	ts.isRunning = false

	// Sinalizar para parar
	close(ts.done)

	// Aguardar conclusão
	ts.wg.Wait()

	logger.Info("Task scheduler parado")
	return nil
}

// ScheduleTask agenda uma tarefa
func (ts *TaskScheduler) ScheduleTask(task Task) error {
	if !ts.IsRunning() {
		return fmt.Errorf("task scheduler não está rodando")
	}

	// Validar tarefa
	if err := ts.validateTask(task); err != nil {
		return fmt.Errorf("tarefa inválida: %w", err)
	}

	// Selecionar fila baseada na prioridade
	queue, exists := ts.taskQueues[task.Priority]
	if !exists {
		return fmt.Errorf("prioridade inválida: %d", task.Priority)
	}

	// Tentar adicionar à fila
	select {
	case queue <- task:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout ao agendar tarefa, fila cheia")
	}
}

// IsRunning retorna se o scheduler está rodando
func (ts *TaskScheduler) IsRunning() bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.isRunning
}

// processTaskQueue processa tarefas de uma fila específica
func (ts *TaskScheduler) processTaskQueue(priority TaskPriority) {
	defer ts.wg.Done()

	queue := ts.taskQueues[priority]

	for {
		select {
		case <-ts.done:
			return

		case task := <-queue:
			ts.processTask(task)
		}
	}
}

// processTask processa uma tarefa individual
func (ts *TaskScheduler) processTask(task Task) {
	logger.Debug("Processando tarefa", "task_id", task.ID, "user_id", task.UserID, "type", task.Type, "priority", task.Priority)

	// Verificar deadline
	if !task.Deadline.IsZero() && time.Now().After(task.Deadline) {
		logger.Warn("Tarefa expirou", "task_id", task.ID, "deadline", task.Deadline)
		ts.sendTaskResponse(task, CommandResponse{
			CommandID: task.ID,
			Error:     fmt.Errorf("tarefa expirou"),
		})
		return
	}

	// Obter worker para o usuário
	worker, exists := ts.pool.GetWorker(task.UserID)
	if !exists {
		// Tentar criar worker se não existir
		newWorker, err := ts.pool.CreateWorker(task.UserID, DefaultWorkerType)
		if err != nil {
			logger.Error("Falha ao criar worker", "user_id", task.UserID, "error", err)
			ts.sendTaskResponse(task, CommandResponse{
				CommandID: task.ID,
				Error:     fmt.Errorf("falha ao criar worker: %w", err),
			})
			return
		}
		worker = newWorker
	}

	// Enviar tarefa para o worker
	if err := worker.SendTask(task); err != nil {
		logger.Error("Falha ao enviar tarefa para worker", "task_id", task.ID, "worker_id", worker.ID, "error", err)

		// Tentar novamente se não excedeu o limite de tentativas
		if task.Retries < task.MaxRetries {
			task.Retries++
			logger.Info("Reagendando tarefa", "task_id", task.ID, "retries", task.Retries)

			// Reagendar com delay
			go func() {
				time.Sleep(time.Duration(task.Retries) * time.Second)
				if err := ts.ScheduleTask(task); err != nil {
					logger.Error("Falha ao reagendar tarefa", "task_id", task.ID, "error", err)
				}
			}()
		} else {
			ts.sendTaskResponse(task, CommandResponse{
				CommandID: task.ID,
				Error:     fmt.Errorf("máximo de tentativas excedido: %w", err),
			})
		}
	}
}

// validateTask valida uma tarefa
func (ts *TaskScheduler) validateTask(task Task) error {
	if task.ID == "" {
		return fmt.Errorf("id da tarefa é obrigatório")
	}

	if task.UserID == "" {
		return fmt.Errorf("userID da tarefa é obrigatório")
	}

	if task.Type == "" {
		return fmt.Errorf("type da tarefa é obrigatório")
	}

	if task.Priority < LowPriority || task.Priority > CriticalPriority {
		return fmt.Errorf("prioridade inválida: %d", task.Priority)
	}

	return nil
}

// sendTaskResponse envia resposta da tarefa se houver canal
func (ts *TaskScheduler) sendTaskResponse(task Task, response CommandResponse) {
	if task.Response != nil {
		select {
		case task.Response <- response:
		case <-time.After(5 * time.Second):
			logger.Warn("Timeout ao enviar resposta da tarefa", "task_id", task.ID)
		}
	}
}

// TaskBuilder helper para construir tarefas facilmente
type TaskBuilder struct {
	task Task
}

// NewTaskBuilder cria um novo builder de tarefas
func NewTaskBuilder() *TaskBuilder {
	return &TaskBuilder{
		task: Task{
			ID:         fmt.Sprintf("task_%d", time.Now().UnixNano()),
			Priority:   NormalPriority,
			Created:    time.Now(),
			MaxRetries: 3,
		},
	}
}

// WithID define o ID da tarefa
func (tb *TaskBuilder) WithID(id string) *TaskBuilder {
	tb.task.ID = id
	return tb
}

// WithUserID define o UserID da tarefa
func (tb *TaskBuilder) WithUserID(userID string) *TaskBuilder {
	tb.task.UserID = userID
	return tb
}

// WithType define o tipo da tarefa
func (tb *TaskBuilder) WithType(taskType CommandType) *TaskBuilder {
	tb.task.Type = taskType
	return tb
}

// WithPriority define a prioridade da tarefa
func (tb *TaskBuilder) WithPriority(priority TaskPriority) *TaskBuilder {
	tb.task.Priority = priority
	return tb
}

// WithPayload define o payload da tarefa
func (tb *TaskBuilder) WithPayload(payload interface{}) *TaskBuilder {
	tb.task.Payload = payload
	return tb
}

// WithDeadline define o deadline da tarefa
func (tb *TaskBuilder) WithDeadline(deadline time.Time) *TaskBuilder {
	tb.task.Deadline = deadline
	return tb
}

// WithMaxRetries define o máximo de tentativas
func (tb *TaskBuilder) WithMaxRetries(maxRetries int) *TaskBuilder {
	tb.task.MaxRetries = maxRetries
	return tb
}

// WithResponse define o canal de resposta
func (tb *TaskBuilder) WithResponse(response chan CommandResponse) *TaskBuilder {
	tb.task.Response = response
	return tb
}

// Build constrói a tarefa
func (tb *TaskBuilder) Build() Task {
	return tb.task
}

// BuildWithContext constrói a tarefa com contexto de timeout
func (tb *TaskBuilder) BuildWithContext(ctx context.Context) Task {
	if deadline, ok := ctx.Deadline(); ok {
		tb.task.Deadline = deadline
	}
	return tb.task
}
