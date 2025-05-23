package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"time"

	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// CommandType define os tipos de comandos que podem ser enviados para o worker
type CommandType string

const (
	CmdSendText    CommandType = "send_text"
	CmdSendMedia   CommandType = "send_media"
	CmdSendButtons CommandType = "send_buttons"
	CmdSendList    CommandType = "send_list"
	CmdConnect     CommandType = "connect"
	CmdDisconnect  CommandType = "disconnect"
	CmdGetStatus   CommandType = "get_status"
	CmdShutdown    CommandType = "shutdown"
	CmdGetQR       CommandType = "get_qr"
	CmdLogout      CommandType = "logout"
)

// Command representa um comando para ser executado pelo worker
type Command struct {
	Type     CommandType
	Payload  interface{}
	Response chan CommandResponse
}

// CommandResponse representa a resposta de um comando
type CommandResponse struct {
	Data  interface{}
	Error error
}

// SendTextPayload payload para envio de texto
type SendTextPayload struct {
	To      string
	Message string
}

// SendMediaPayload payload para envio de mídia
type SendMediaPayload struct {
	To        string
	MediaURL  string
	MediaType string
	Caption   string
}

// SendButtonsPayload payload para envio de botões
type SendButtonsPayload struct {
	To      string
	Text    string
	Footer  string
	Buttons []ButtonData
}

// SendListPayload payload para envio de lista
type SendListPayload struct {
	To         string
	Text       string
	Footer     string
	ButtonText string
	Sections   []Section
}

// SessionWorker gerencia uma sessão WhatsApp em sua própria goroutine
type SessionWorker struct {
	userID     string
	client     *Client
	commands   chan Command
	events     chan interface{}
	done       chan struct{}
	wg         sync.WaitGroup
	mu         sync.RWMutex
	sessionMgr *SessionManager
	isRunning  bool
}

// NewSessionWorker cria um novo worker de sessão
func NewSessionWorker(userID string, client *Client, sessionMgr *SessionManager) *SessionWorker {
	sw := &SessionWorker{
		userID:     userID,
		client:     client,
		commands:   make(chan Command, 100),      // Buffer de 100 comandos
		events:     make(chan interface{}, 1000), // Buffer de 1000 eventos
		done:       make(chan struct{}),
		sessionMgr: sessionMgr,
		isRunning:  false,
	}

	return sw
}

// Start inicia o worker em uma goroutine
func (sw *SessionWorker) Start() {
	sw.mu.Lock()
	if sw.isRunning {
		sw.mu.Unlock()
		return
	}
	sw.isRunning = true
	sw.mu.Unlock()

	sw.wg.Add(2) // Uma para comandos, outra para eventos

	// Goroutine para processar comandos
	go sw.commandProcessor()

	// Goroutine para processar eventos
	go sw.eventProcessor()

	logger.Info("Session worker iniciado", "user_id", sw.userID)
}

// Stop para o worker gracefully
func (sw *SessionWorker) Stop() {
	sw.mu.Lock()
	if !sw.isRunning {
		sw.mu.Unlock()
		return
	}
	sw.isRunning = false
	sw.mu.Unlock()

	logger.Info("Parando session worker", "user_id", sw.userID)

	// Sinalizar para parar
	close(sw.done)

	// Aguardar conclusão das goroutines
	sw.wg.Wait()

	// Fechar channels
	close(sw.commands)
	close(sw.events)

	logger.Info("Session worker parado", "user_id", sw.userID)
}

// commandProcessor processa comandos em sua própria goroutine
func (sw *SessionWorker) commandProcessor() {
	defer sw.wg.Done()

	for {
		select {
		case <-sw.done:
			return

		case cmd := <-sw.commands:
			sw.handleCommand(cmd)
		}
	}
}

// eventProcessor processa eventos do WhatsApp em sua própria goroutine
func (sw *SessionWorker) eventProcessor() {
	defer sw.wg.Done()

	for {
		select {
		case <-sw.done:
			return

		case evt := <-sw.events:
			sw.handleEvent(evt)
		}
	}
}

// handleCommand processa um comando específico
func (sw *SessionWorker) handleCommand(cmd Command) {
	logger.Debug("Processando comando", "user_id", sw.userID, "type", cmd.Type)

	var response CommandResponse

	switch cmd.Type {
	case CmdSendText:
		response = sw.handleSendText(cmd.Payload.(SendTextPayload))

	case CmdSendMedia:
		response = sw.handleSendMedia(cmd.Payload.(SendMediaPayload))

	case CmdSendButtons:
		response = sw.handleSendButtons(cmd.Payload.(SendButtonsPayload))

	case CmdSendList:
		response = sw.handleSendList(cmd.Payload.(SendListPayload))

	case CmdConnect:
		response = sw.handleConnect()

	case CmdDisconnect:
		response = sw.handleDisconnect()

	case CmdGetStatus:
		response = sw.handleGetStatus()

	case CmdGetQR:
		response = sw.handleGetQR(cmd.Payload.(context.Context))

	case CmdLogout:
		response = sw.handleLogout(cmd.Payload.(context.Context))

	default:
		response = CommandResponse{
			Error: fmt.Errorf("comando não reconhecido: %s", cmd.Type),
		}
	}

	// Enviar resposta se o canal foi fornecido
	if cmd.Response != nil {
		cmd.Response <- response
	}

	// Atualizar última atividade
	sw.client.LastActive = time.Now()
}

// handleEvent processa eventos do WhatsApp
func (sw *SessionWorker) handleEvent(evt interface{}) {
	// Usar o processEvent do SessionManager existente
	sw.sessionMgr.processEvent(sw.userID, evt)
}

// handleSendText processa envio de texto
func (sw *SessionWorker) handleSendText(payload SendTextPayload) CommandResponse {
	recipient, err := ParseJID(payload.To)
	if err != nil {
		return CommandResponse{Error: err}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	msg, err := sw.client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
		Conversation: proto.String(payload.Message),
	})

	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao enviar mensagem: %w", err)}
	}

	return CommandResponse{Data: msg.ID}
}

// handleSendMedia processa envio de mídia (usando função existente do message.go)
func (sw *SessionWorker) handleSendMedia(payload SendMediaPayload) CommandResponse {
	// Delegar para o método existente do SessionManager
	msgID, err := sw.sessionMgr.SendMedia(sw.userID, payload.To, payload.MediaURL, payload.MediaType, payload.Caption)
	if err != nil {
		return CommandResponse{Error: err}
	}
	return CommandResponse{Data: msgID}
}

// handleSendButtons processa envio de botões (usando função existente do message.go)
func (sw *SessionWorker) handleSendButtons(payload SendButtonsPayload) CommandResponse {
	// Delegar para o método existente do SessionManager
	msgID, err := sw.sessionMgr.SendButtons(sw.userID, payload.To, payload.Text, payload.Footer, payload.Buttons)
	if err != nil {
		return CommandResponse{Error: err}
	}
	return CommandResponse{Data: msgID}
}

// handleSendList processa envio de lista (usando função existente do message.go)
func (sw *SessionWorker) handleSendList(payload SendListPayload) CommandResponse {
	// Delegar para o método existente do SessionManager
	msgID, err := sw.sessionMgr.SendList(sw.userID, payload.To, payload.Text, payload.Footer, payload.ButtonText, payload.Sections)
	if err != nil {
		return CommandResponse{Error: err}
	}
	return CommandResponse{Data: msgID}
}

// handleConnect processa comando de conexão
func (sw *SessionWorker) handleConnect() CommandResponse {
	if sw.client.WAClient.IsConnected() {
		return CommandResponse{Data: "já conectado"}
	}

	err := sw.client.WAClient.Connect()
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao conectar: %w", err)}
	}

	// Aguardar conexão
	if !sw.client.WAClient.WaitForConnection(10 * time.Second) {
		return CommandResponse{Error: fmt.Errorf("timeout ao conectar")}
	}

	sw.client.Connected = true
	return CommandResponse{Data: "conectado"}
}

// handleDisconnect processa comando de desconexão
func (sw *SessionWorker) handleDisconnect() CommandResponse {
	if sw.client.WAClient.IsConnected() {
		sw.client.WAClient.Disconnect()
	}

	sw.client.Connected = false
	return CommandResponse{Data: "desconectado"}
}

// handleGetStatus retorna o status atual
func (sw *SessionWorker) handleGetStatus() CommandResponse {
	status := map[string]interface{}{
		"connected":   sw.client.Connected,
		"last_active": sw.client.LastActive,
		"logged_in":   sw.client.WAClient.IsLoggedIn(),
		"user_id":     sw.userID,
	}

	return CommandResponse{Data: status}
}

// handleGetQR processa comando de obter QR code
func (sw *SessionWorker) handleGetQR(ctx context.Context) CommandResponse {
	qrChan, err := sw.sessionMgr.GetQRChannel(ctx, sw.userID)
	if err != nil {
		return CommandResponse{Error: err}
	}
	return CommandResponse{Data: qrChan}
}

// handleLogout processa comando de logout
func (sw *SessionWorker) handleLogout(ctx context.Context) CommandResponse {
	err := sw.sessionMgr.Logout(ctx, sw.userID)
	if err != nil {
		return CommandResponse{Error: err}
	}
	return CommandResponse{Data: "logout realizado"}
}

// SendCommand envia um comando para o worker e aguarda resposta
func (sw *SessionWorker) SendCommand(cmd Command) CommandResponse {
	// Verificar se o worker está rodando
	sw.mu.RLock()
	if !sw.isRunning {
		sw.mu.RUnlock()
		return CommandResponse{Error: fmt.Errorf("worker não está rodando")}
	}
	sw.mu.RUnlock()

	// Criar canal de resposta se não fornecido
	if cmd.Response == nil {
		cmd.Response = make(chan CommandResponse, 1)
	}

	// Enviar comando
	select {
	case sw.commands <- cmd:
		// Aguardar resposta com timeout
		select {
		case response := <-cmd.Response:
			return response
		case <-time.After(60 * time.Second):
			return CommandResponse{Error: fmt.Errorf("timeout aguardando resposta")}
		}
	case <-time.After(5 * time.Second):
		return CommandResponse{Error: fmt.Errorf("timeout ao enviar comando")}
	}
}

// SendEvent envia um evento para o worker (usado pelo event handler)
func (sw *SessionWorker) SendEvent(evt interface{}) {
	// Verificar se o worker está rodando
	sw.mu.RLock()
	if !sw.isRunning {
		sw.mu.RUnlock()
		return
	}
	sw.mu.RUnlock()

	// Enviar evento de forma não bloqueante
	select {
	case sw.events <- evt:
	default:
		logger.Warn("Evento descartado, buffer cheio", "user_id", sw.userID)
	}
}

// IsRunning retorna se o worker está rodando
func (sw *SessionWorker) IsRunning() bool {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.isRunning
}

// GetClient retorna o cliente da sessão
func (sw *SessionWorker) GetClient() *Client {
	return sw.client
}
