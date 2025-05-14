// internal/services/newsletter/newsletter.go
package newsletter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	
	"github.com/google/uuid"
	
	"yourproject/internal/services/whatsapp"
	"yourproject/internal/storage"
	"yourproject/pkg/logger"
)

// Newsletter representa um boletim informativo para distribuição em massa
type Newsletter struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Content      string                 `json:"content"`
	MediaURL     string                 `json:"media_url,omitempty"`
	MediaType    string                 `json:"media_type,omitempty"`
	Buttons      []whatsapp.ButtonData  `json:"buttons,omitempty"`
	Recipients   []string               `json:"recipients"`
	Status       string                 `json:"status"` // draft, scheduled, sending, completed, failed
	ScheduledFor *time.Time             `json:"scheduled_for,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	SentCount    int                    `json:"sent_count"`
	FailedCount  int                    `json:"failed_count"`
	ErrorDetails map[string]string      `json:"error_details,omitempty"`
	CreatedBy    string                 `json:"created_by"`
	SessionID    string                 `json:"session_id"`
}

// DeliveryReport representa o relatório de entrega para um destinatário
type DeliveryReport struct {
	RecipientJID string    `json:"recipient_jid"`
	Status       string    `json:"status"` // sent, delivered, read, failed
	SentAt       time.Time `json:"sent_at"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

// NewsletterService gerencia o envio de newsletters
type NewsletterService struct {
	sessionManager *whatsapp.SessionManager
	fileStore      *storage.FileStore
	newsletterDir  string
	newsletters    map[string]*Newsletter
	mutex          sync.RWMutex
	deliveryReports map[string]map[string]*DeliveryReport // map[newsletterID]map[recipientJID]report
	deliveryMutex   sync.RWMutex
}

// NewNewsletterService cria um novo serviço de newsletter
func NewNewsletterService(sm *whatsapp.SessionManager, fs *storage.FileStore, baseDir string) (*NewsletterService, error) {
	// Configurar diretório para newsletters
	newsletterDir := filepath.Join(baseDir, "newsletters")
	if err := os.MkdirAll(newsletterDir, 0755); err != nil {
		return nil, fmt.Errorf("falha ao criar diretório de newsletters: %w", err)
	}
	
	service := &NewsletterService{
		sessionManager:  sm,
		fileStore:       fs,
		newsletterDir:   newsletterDir,
		newsletters:     make(map[string]*Newsletter),
		deliveryReports: make(map[string]map[string]*DeliveryReport),
	}
	
	// Carregar newsletters salvos
	if err := service.loadNewsletters(); err != nil {
		return nil, fmt.Errorf("falha ao carregar newsletters: %w", err)
	}
	
	// Iniciar processamento de newsletters agendados
	go service.processScheduledNewsletters()
	
	return service, nil
}

// loadNewsletters carrega newsletters do armazenamento
func (s *NewsletterService) loadNewsletters() error {
	files, err := os.ReadDir(s.newsletterDir)
	if err != nil {
		return fmt.Errorf("falha ao ler diretório de newsletters: %w", err)
	}
	
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		
		path := filepath.Join(s.newsletterDir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Warn("Falha ao ler arquivo de newsletter", "path", path, "error", err)
			continue
		}
		
		var newsletter Newsletter
		if err := json.Unmarshal(data, &newsletter); err != nil {
			logger.Warn("Falha ao decodificar newsletter", "path", path, "error", err)
			continue
		}
		
		// Inicializar mapa de relatórios de entrega se não existir
		s.deliveryMutex.Lock()
		if _, exists := s.deliveryReports[newsletter.ID]; !exists {
			s.deliveryReports[newsletter.ID] = make(map[string]*DeliveryReport)
		}
		s.deliveryMutex.Unlock()
		
		// Armazenar no mapa em memória
		s.mutex.Lock()
		s.newsletters[newsletter.ID] = &newsletter
		s.mutex.Unlock()
		
		// Carregar relatórios de entrega se o newsletter estiver em andamento ou concluído
		if newsletter.Status == "sending" || newsletter.Status == "completed" {
			if err := s.loadDeliveryReports(newsletter.ID); err != nil {
				logger.Warn("Falha ao carregar relatórios de entrega", "newsletter_id", newsletter.ID, "error", err)
			}
		}
	}
	
	return nil
}

// loadDeliveryReports carrega relatórios de entrega do armazenamento
func (s *NewsletterService) loadDeliveryReports(newsletterID string) error {
	path := filepath.Join(s.newsletterDir, newsletterID + "_reports.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Arquivo não existe ainda
	}
	
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("falha ao ler arquivo de relatórios: %w", err)
	}
	
	reports := make(map[string]*DeliveryReport)
	if err := json.Unmarshal(data, &reports); err != nil {
		return fmt.Errorf("falha ao decodificar relatórios: %w", err)
	}
	
	s.deliveryMutex.Lock()
	s.deliveryReports[newsletterID] = reports
	s.deliveryMutex.Unlock()
	
	return nil
}

// saveNewsletter salva um newsletter no armazenamento
func (s *NewsletterService) saveNewsletter(newsletter *Newsletter) error {
	// Atualizar timestamp
	newsletter.UpdatedAt = time.Now()
	
	// Serializar para JSON
	data, err := json.Marshal(newsletter)
	if err != nil {
		return fmt.Errorf("falha ao codificar newsletter: %w", err)
	}
	
	// Salvar no arquivo
	path := filepath.Join(s.newsletterDir, newsletter.ID + ".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("falha ao salvar newsletter: %w", err)
	}
	
	return nil
}

// saveDeliveryReports salva relatórios de entrega no armazenamento
func (s *NewsletterService) saveDeliveryReports(newsletterID string) error {
	s.deliveryMutex.RLock()
	reports, exists := s.deliveryReports[newsletterID]
	s.deliveryMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("relatórios não encontrados para newsletter: %s", newsletterID)
	}
	
	// Serializar para JSON
	data, err := json.Marshal(reports)
	if err != nil {
		return fmt.Errorf("falha ao codificar relatórios: %w", err)
	}
	
	// Salvar no arquivo
	path := filepath.Join(s.newsletterDir, newsletterID + "_reports.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("falha ao salvar relatórios: %w", err)
	}
	
	return nil
}

// CreateNewsletter cria um novo newsletter
func (s *NewsletterService) CreateNewsletter(title, content, mediaURL, mediaType string, 
	buttons []whatsapp.ButtonData, recipients []string, createdBy, sessionID string) (*Newsletter, error) {
	
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	// Validar sessão
	if _, exists := s.sessionManager.GetSession(sessionID); !exists {
		return nil, fmt.Errorf("sessão inválida: %s", sessionID)
	}
	
	// Gerar ID único
	id := uuid.New().String()
	
	// Criar newsletter
	newsletter := &Newsletter{
		ID:          id,
		Title:       title,
		Content:     content,
		MediaURL:    mediaURL,
		MediaType:   mediaType,
		Buttons:     buttons,
		Recipients:  recipients,
		Status:      "draft",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		CreatedBy:   createdBy,
		SessionID:   sessionID,
		ErrorDetails: make(map[string]string),
	}
	
	// Armazenar newsletter
	s.newsletters[id] = newsletter
	
	// Inicializar relatórios de entrega
	s.deliveryMutex.Lock()
	s.deliveryReports[id] = make(map[string]*DeliveryReport)
	s.deliveryMutex.Unlock()
	
	// Salvar no armazenamento
	if err := s.saveNewsletter(newsletter); err != nil {
		delete(s.newsletters, id)
		return nil, fmt.Errorf("falha ao salvar newsletter: %w", err)
	}
	
	return newsletter, nil
}

// GetNewsletter obtém um newsletter pelo ID
func (s *NewsletterService) GetNewsletter(id string) (*Newsletter, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	newsletter, exists := s.newsletters[id]
	if !exists {
		return nil, fmt.Errorf("newsletter não encontrado: %s", id)
	}
	
	return newsletter, nil
}

// ListNewsletters retorna a lista de newsletters
func (s *NewsletterService) ListNewsletters() []*Newsletter {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	result := make([]*Newsletter, 0, len(s.newsletters))
	for _, newsletter := range s.newsletters {
		result = append(result, newsletter)
	}
	
	return result
}

// ScheduleNewsletter agenda um newsletter para envio
func (s *NewsletterService) ScheduleNewsletter(id string, scheduleTime time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	newsletter, exists := s.newsletters[id]
	if !exists {
		return fmt.Errorf("newsletter não encontrado: %s", id)
	}
	
	// Verificar se já está agendado ou foi enviado
	if newsletter.Status != "draft" {
		return fmt.Errorf("newsletter não está em rascunho: %s", newsletter.Status)
	}
	
	// Definir horário agendado
	newsletter.ScheduledFor = &scheduleTime
	newsletter.Status = "scheduled"
	newsletter.UpdatedAt = time.Now()
	
	// Salvar alterações
	return s.saveNewsletter(newsletter)
}

// SendNewsletter envia um newsletter imediatamente
func (s *NewsletterService) SendNewsletter(id string) error {
	s.mutex.Lock()
	newsletter, exists := s.newsletters[id]
	if !exists {
		s.mutex.Unlock()
		return fmt.Errorf("newsletter não encontrado: %s", id)
	}
	
	// Verificar se já está sendo enviado ou foi enviado
	if newsletter.Status == "sending" || newsletter.Status == "completed" {
		s.mutex.Unlock()
		return fmt.Errorf("newsletter já está %s", newsletter.Status)
	}
	
	// Atualizar status
	newsletter.Status = "sending"
	newsletter.UpdatedAt = time.Now()
	
	// Salvar alterações
	if err := s.saveNewsletter(newsletter); err != nil {
		s.mutex.Unlock()
		return fmt.Errorf("falha ao atualizar status do newsletter: %w", err)
	}
	
	s.mutex.Unlock()
	
	// Iniciar envio em goroutine separada
	go s.processNewsletterSending(newsletter)
	
	return nil
}

// CancelNewsletter cancela um newsletter agendado
func (s *NewsletterService) CancelNewsletter(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	newsletter, exists := s.newsletters[id]
	if !exists {
		return fmt.Errorf("newsletter não encontrado: %s", id)
	}
	
	// Verificar se está agendado
	if newsletter.Status != "scheduled" {
		return fmt.Errorf("newsletter não está agendado: %s", newsletter.Status)
	}
	
	// Atualizar status para rascunho
	newsletter.Status = "draft"
	newsletter.ScheduledFor = nil
	newsletter.UpdatedAt = time.Now()
	
	// Salvar alterações
	return s.saveNewsletter(newsletter)
}

// DeleteNewsletter remove um newsletter
func (s *NewsletterService) DeleteNewsletter(id string) error {
	s.mutex.Lock()
	newsletter, exists := s.newsletters[id]
	if !exists {
		s.mutex.Unlock()
		return fmt.Errorf("newsletter não encontrado: %s", id)
	}
	
	// Verificar se está em andamento
	if newsletter.Status == "sending" {
		s.mutex.Unlock()
		return fmt.Errorf("não é possível excluir um newsletter em envio")
	}
	
	// Remover do mapa em memória
	delete(s.newsletters, id)
	s.mutex.Unlock()
	
	// Remover relatórios
	s.deliveryMutex.Lock()
	delete(s.deliveryReports, id)
	s.deliveryMutex.Unlock()
	
	// Remover arquivos
	newsletterPath := filepath.Join(s.newsletterDir, id + ".json")
	reportsPath := filepath.Join(s.newsletterDir, id + "_reports.json")
	
	if err := os.Remove(newsletterPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("falha ao excluir arquivo de newsletter: %w", err)
	}
	
	if err := os.Remove(reportsPath); err != nil && !os.IsNotExist(err) {
		logger.Warn("Falha ao excluir arquivo de relatórios", "path", reportsPath, "error", err)
	}
	
	return nil
}

// GetDeliveryReports retorna os relatórios de entrega para um newsletter
func (s *NewsletterService) GetDeliveryReports(newsletterID string) (map[string]*DeliveryReport, error) {
	s.deliveryMutex.RLock()
	defer s.deliveryMutex.RUnlock()
	
	reports, exists := s.deliveryReports[newsletterID]
	if !exists {
		return nil, fmt.Errorf("relatórios não encontrados para newsletter: %s", newsletterID)
	}
	
	// Criar cópia para retornar
	result := make(map[string]*DeliveryReport, len(reports))
	for k, v := range reports {
		result[k] = v
	}
	
	return result, nil
}

// processScheduledNewsletters processa newsletters agendados
func (s *NewsletterService) processScheduledNewsletters() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		// Verificar newsletters agendados
		s.mutex.RLock()
		var toSend []*Newsletter
		now := time.Now()
		
		for _, newsletter := range s.newsletters {
			if newsletter.Status == "scheduled" && newsletter.ScheduledFor != nil && now.After(*newsletter.ScheduledFor) {
				toSend = append(toSend, newsletter)
			}
		}
		s.mutex.RUnlock()
		
		// Processar newsletters prontos para envio
		for _, newsletter := range toSend {
			// Atualizar status
			s.mutex.Lock()
			newsletter.Status = "sending"
			newsletter.UpdatedAt = now
			if err := s.saveNewsletter(newsletter); err != nil {
				logger.Error("Falha ao atualizar status do newsletter", "id", newsletter.ID, "error", err)
			}
			s.mutex.Unlock()
			
			// Iniciar envio em goroutine separada
			go s.processNewsletterSending(newsletter)
		}
	}
}

// processNewsletterSending processa o envio de um newsletter
func (s *NewsletterService) processNewsletterSending(newsletter *Newsletter) {
	logger.Info("Iniciando envio de newsletter", "id", newsletter.ID, "recipients", len(newsletter.Recipients))
	
	// Obter sessão
	client, exists := s.sessionManager.GetSession(newsletter.SessionID)
	if !exists {
		s.updateNewsletterStatus(newsletter.ID, "failed", "Sessão não encontrada")
		return
	}
	
	// Preparar contexto com cancelamento
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Inicializar contadores
	sentCount := 0
	failedCount := 0
	errorDetails := make(map[string]string)
	
	// Preparar canal para limitar concorrência (máximo 10 envios simultâneos)
	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup
	
	// Processar cada destinatário
	for _, recipient := range newsletter.Recipients {
		// Adicionar ao semáforo
		semaphore <- struct{}{}
		wg.Add(1)
		
		// Processar em goroutine
		go func(jid string) {
			defer func() {
				<-semaphore // Liberar slot do semáforo
				wg.Done()
			}()
			
			// Verificar se contexto foi cancelado
			if ctx.Err() != nil {
				return
			}
			
			// Criar relatório de entrega
			report := &DeliveryReport{
				RecipientJID: jid,
				Status:       "sending",
				SentAt:       time.Now(),
			}
			
			// Salvar relatório inicial
			s.deliveryMutex.Lock()
			s.deliveryReports[newsletter.ID][jid] = report
			s.deliveryMutex.Unlock()
			
			// Tentar enviar mensagem
			var err error
			var messageID string
			
			// Verificar tipo de envio
			if newsletter.MediaURL != "" {
				// Enviar mídia
				messageID, err = s.sessionManager.SendMedia(
					newsletter.SessionID,
					jid,
					newsletter.MediaURL,
					newsletter.MediaType,
					newsletter.Content,
				)
			} else if len(newsletter.Buttons) > 0 {
				// Enviar mensagem com botões
				messageID, err = s.sessionManager.SendButtons(
					newsletter.SessionID,
					jid,
					newsletter.Content,
					newsletter.Title,
					newsletter.Buttons,
				)
			} else {
				// Enviar texto simples
				messageID, err = s.sessionManager.SendText(
					newsletter.SessionID,
					jid,
					newsletter.Content,
				)
			}
			
			// Atualizar relatório com resultado
			s.deliveryMutex.Lock()
			if err != nil {
				report.Status = "failed"
				report.ErrorMessage = err.Error()
				
				// Atualizar contadores
				failedCount++
				errorDetails[jid] = err.Error()
			} else {
				report.Status = "sent"
				sentCount++
			}
			s.deliveryReports[newsletter.ID][jid] = report
			s.deliveryMutex.Unlock()
			
			// Registrar log
			if err != nil {
				logger.Warn("Falha ao enviar newsletter", "id", newsletter.ID, "recipient", jid, "error", err)
			} else {
				logger.Debug("Newsletter enviado", "id", newsletter.ID, "recipient", jid, "message_id", messageID)
			}
		}(recipient)
	}
	
	// Aguardar conclusão de todos os envios
	wg.Wait()
	
	// Salvar relatórios de entrega
	if err := s.saveDeliveryReports(newsletter.ID); err != nil {
		logger.Error("Falha ao salvar relatórios de entrega", "id", newsletter.ID, "error", err)
	}
	
	// Atualizar status do newsletter
	s.mutex.Lock()
	if nl, exists := s.newsletters[newsletter.ID]; exists {
		nl.Status = "completed"
		nl.SentCount = sentCount
		nl.FailedCount = failedCount
		nl.ErrorDetails = errorDetails
		nl.UpdatedAt = time.Now()
		
		if err := s.saveNewsletter(nl); err != nil {
			logger.Error("Falha ao atualizar status do newsletter", "id", newsletter.ID, "error", err)
		}
	}
	s.mutex.Unlock()
	
	logger.Info("Envio de newsletter concluído", 
		"id", newsletter.ID, 
		"sent", sentCount, 
		"failed", failedCount)
}

// updateNewsletterStatus atualiza o status de um newsletter com mensagem de erro
func (s *NewsletterService) updateNewsletterStatus(id, status, errorMsg string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	newsletter, exists := s.newsletters[id]
	if !exists {
		return
	}
	
	newsletter.Status = status
	newsletter.UpdatedAt = time.Now()
	
	if errorMsg != "" && status == "failed" {
		newsletter.ErrorDetails["global"] = errorMsg
	}
	
	if err := s.saveNewsletter(newsletter); err != nil {
		logger.Error("Falha ao atualizar status do newsletter", "id", id, "error", err)
	}
}