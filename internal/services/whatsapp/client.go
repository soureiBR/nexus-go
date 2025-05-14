// internal/services/whatsapp/client.go
package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	
	"yourproject/pkg/logger"
)

// GetQRChannel obtém o canal QR para autenticação
func (sm *SessionManager) GetQRChannel(userID string) (<-chan string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Verificar se já está conectado
	if client.Connected {
		return nil, fmt.Errorf("cliente já conectado")
	}
	
	// Criar canal para código QR
	qrChan := make(chan string)
	
	// Registrar evento de QR
	qrHandler := func(evt interface{}) {
		if qrEvent, ok := evt.(*events.QR); ok {
			qrChan <- qrEvent.Code
		}
	}
	
	client.WAClient.AddEventHandler(qrHandler)
	
	// Iniciar conexão em goroutine
	go func() {
		// Remover handler depois de um tempo
		time.AfterFunc(2*time.Minute, func() {
			client.WAClient.RemoveEventHandler(qrHandler)
			close(qrChan)
		})
		
		err := client.WAClient.Connect()
		if err != nil {
			logger.Error("Falha ao conectar cliente", "error", err, "user_id", userID)
			// Se falhar, remover handler
			client.WAClient.RemoveEventHandler(qrHandler)
			close(qrChan)
		}
	}()
	
	return qrChan, nil
}

// DisconnectAll desconecta todas as sessões
func (sm *SessionManager) DisconnectAll() {
	sm.clientsMutex.RLock()
	defer sm.clientsMutex.RUnlock()
	
	logger.Info("Desconectando todas as sessões...", "count", len(sm.clients))
	
	// Criar WaitGroup para esperar todas as desconexões
	var wg sync.WaitGroup
	wg.Add(len(sm.clients))
	
	// Desconectar cada cliente
	for userID, client := range sm.clients {
		go func(id string, c *Client) {
			defer wg.Done()
			
			logger.Debug("Desconectando sessão", "user_id", id)
			c.WAClient.Disconnect()
		}(userID, client)
	}
	
	// Aguardar com timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		logger.Info("Todas as sessões foram desconectadas com sucesso")
	case <-time.After(10 * time.Second):
		logger.Warn("Timeout ao aguardar desconexão de todas as sessões")
	}
}

// GetAllSessions retorna todas as sessões
func (sm *SessionManager) GetAllSessions() []*Client {
	sm.clientsMutex.RLock()
	defer sm.clientsMutex.RUnlock()
	
	sessions := make([]*Client, 0, len(sm.clients))
	for _, client := range sm.clients {
		sessions = append(sessions, client)
	}
	
	return sessions
}

// CleanupSessions realiza limpeza de sessões inativas
func (sm *SessionManager) CleanupSessions(maxInactiveTime time.Duration) {
	logger.Info("Iniciando limpeza de sessões inativas...")
	
	now := time.Now()
	var toDelete []string
	
	// Identificar sessões inativas
	sm.clientsMutex.RLock()
	for userID, client := range sm.clients {
		if now.Sub(client.LastActive) > maxInactiveTime && !client.Connected {
			toDelete = append(toDelete, userID)
		}
	}
	sm.clientsMutex.RUnlock()
	
	// Excluir sessões inativas
	for _, userID := range toDelete {
		logger.Info("Removendo sessão inativa", "user_id", userID, "last_active", sm.clients[userID].LastActive)
		sm.DeleteSession(userID)
	}
	
	logger.Info("Limpeza de sessões concluída", "removed", len(toDelete))
}

// StartPeriodicCleanup inicia limpeza periódica de sessões
func (sm *SessionManager) StartPeriodicCleanup(interval, maxInactiveTime time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			sm.CleanupSessions(maxInactiveTime)
		}
	}()
	
	logger.Info("Limpeza periódica de sessões iniciada", 
	            "interval", interval.String(), 
				"max_inactive", maxInactiveTime.String())
}

// GetDeviceInfo obtém informações do dispositivo
func (sm *SessionManager) GetDeviceInfo(userID string) (*types.DeviceProps, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	return client.WAClient.GetDeviceProps(), nil
}

// GetConnectionState obtém o estado da conexão
func (sm *SessionManager) GetConnectionState(userID string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	state := client.WAClient.GetConnectionState()
	
	switch state {
	case whatsmeow.ConnectionConnected:
		return "connected", nil
	case whatsmeow.ConnectionConnecting:
		return "connecting", nil
	case whatsmeow.ConnectionDisconnected:
		return "disconnected", nil
	case whatsmeow.ConnectionLoggedOut:
		return "logged_out", nil
	default:
		return "unknown", nil
	}
}

// UpdatePresence atualiza o status de presença
func (sm *SessionManager) UpdatePresence(userID string, presence types.Presence) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Verificar se o cliente está conectado
	if !client.Connected {
		return fmt.Errorf("cliente não está conectado")
	}
	
	err := client.WAClient.SendPresence(presence)
	if err != nil {
		return fmt.Errorf("falha ao atualizar presença: %w", err)
	}
	
	return nil
}

// UpdateProfilePicture atualiza a foto de perfil
func (sm *SessionManager) UpdateProfilePicture(userID string, imageData []byte) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Verificar se o cliente está conectado
	if !client.Connected {
		return fmt.Errorf("cliente não está conectado")
	}
	
	err := client.WAClient.SetProfilePictureBytes(imageData)
	if err != nil {
		return fmt.Errorf("falha ao atualizar foto de perfil: %w", err)
	}
	
	return nil
}

// GetProfilePicture obtém a foto de perfil de um contato
func (sm *SessionManager) GetProfilePicture(userID, jid string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Verificar se o cliente está conectado
	if !client.Connected {
		return "", fmt.Errorf("cliente não está conectado")
	}
	
	// Converter para JID
	targetJID, err := ParseJID(jid)
	if err != nil {
		return "", err
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Obter imagem
	profilePic, err := client.WAClient.GetProfilePictureInfo(targetJID, ctx, false)
	if err != nil {
		return "", fmt.Errorf("falha ao obter foto de perfil: %w", err)
	}
	
	if profilePic == nil || profilePic.URL == "" {
		return "", fmt.Errorf("contato não possui foto de perfil")
	}
	
	return profilePic.URL, nil
}

// LogoutSession encerra a sessão no WhatsApp (logout)
func (sm *SessionManager) LogoutSession(userID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Verificar se o cliente está conectado
	if !client.Connected {
		return fmt.Errorf("cliente não está conectado")
	}
	
	err := client.WAClient.Logout()
	if err != nil {
		return fmt.Errorf("falha ao fazer logout: %w", err)
	}
	
	// Após logout, desconectar
	client.WAClient.Disconnect()
	
	// Atualizar estado
	client.Connected = false
	client.LastActive = time.Now()
	
	return nil
}

// CheckNumberStatus verifica se um número está registrado no WhatsApp
func (sm *SessionManager) CheckNumberStatus(userID string, numbers []string) (map[string]bool, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Verificar se o cliente está conectado
	if !client.Connected {
		return nil, fmt.Errorf("cliente não está conectado")
	}
	
	// Converter números para JIDs
	var jids []types.JID
	for _, number := range numbers {
		jid, err := ParseJID(number)
		if err != nil {
			continue // Ignorar números inválidos
		}
		jids = append(jids, jid)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Verificar status
	results, err := client.WAClient.IsOnWhatsApp(jids)
	if err != nil {
		return nil, fmt.Errorf("falha ao verificar números: %w", err)
	}
	
	// Converter resultado
	statusMap := make(map[string]bool)
	for _, result := range results {
		// Remover sufixo @s.whatsapp.net
		number := strings.TrimSuffix(result.JID.String(), "@s.whatsapp.net")
		statusMap[number] = result.IsIn
	}
	
	return statusMap, nil
}