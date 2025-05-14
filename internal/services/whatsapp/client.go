// internal/services/whatsapp/client.go
package whatsapp

import (
	"context"
	"fmt"
	"sync"
	"strings"
	"time"
	
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waBinary "go.mau.fi/whatsmeow/binary"
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
			if len(qrEvent.Codes) > 0 {
				qrChan <- qrEvent.Codes[0]
			}
		}
	}
	
	// Armazenar o ID do handler para remoção posterior
	handlerID := client.WAClient.AddEventHandler(qrHandler)
	
	// Iniciar conexão em goroutine
	go func() {
		// Remover handler depois de um tempo
		time.AfterFunc(2*time.Minute, func() {
			client.WAClient.RemoveEventHandler(handlerID)
			close(qrChan)
		})
		
		err := client.WAClient.Connect()
		if err != nil {
			logger.Error("Falha ao conectar cliente", "error", err, "user_id", userID)
			// Se falhar, remover handler
			client.WAClient.RemoveEventHandler(handlerID)
			close(qrChan)
		}
	}()
	
	return qrChan, nil
}

// Já comentado no seu código original, mantive comentado
// DisconnectAll desconecta todas as sessões
// func (sm *SessionManager) DisconnectAll() {
// 	sm.clientsMutex.RLock()
// 	defer sm.clientsMutex.RUnlock()
	
// 	logger.Info("Desconectando todas as sessões...", "count", len(sm.clients))
	
// 	// Criar WaitGroup para esperar todas as desconexões
// 	var wg sync.WaitGroup
// 	wg.Add(len(sm.clients))
	
// 	// Desconectar cada cliente
// 	for userID, client := range sm.clients {
// 		go func(id string, c *Client) {
// 			defer wg.Done()
			
// 			logger.Debug("Desconectando sessão", "user_id", id)
// 			c.WAClient.Disconnect()
// 		}(userID, client)
// 	}
	
// 	// Aguardar com timeout
// 	done := make(chan struct{})
// 	go func() {
// 		wg.Wait()
// 		close(done)
// 	}()
	
// 	select {
// 	case <-done:
// 		logger.Info("Todas as sessões foram desconectadas com sucesso")
// 	case <-time.After(10 * time.Second):
// 		logger.Warn("Timeout ao aguardar desconexão de todas as sessões")
// 	}
// }

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

// DeviceInfo estrutura para armazenar informações do dispositivo
type DeviceInfo struct {
	ID          string            `json:"id"`
	PhoneNumber string            `json:"phone_number"`
	Platform    string            `json:"platform"`
	Connected   bool              `json:"connected"`
	VerifiedName string           `json:"verified_name,omitempty"`
	Status      string            `json:"status,omitempty"`
}

// GetDeviceInfo obtém informações do dispositivo
func (sm *SessionManager) GetDeviceInfo(userID string) (*DeviceInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Obter informações básicas do dispositivo
	deviceID := client.WAClient.Store.ID
	
	// Obter informações do usuário via GetUserInfo
	myJID := deviceID.ToNonAD()
	
	// Criar o objeto DeviceInfo com as informações disponíveis
	deviceInfo := &DeviceInfo{
		ID:          deviceID.String(),
		PhoneNumber: deviceID.User,
		Platform:    "WhatsApp",
		Connected:   client.Connected,
	}
	
	// Se estiver conectado, tentar obter mais informações
	if client.Connected {
		// Tentar obter informações do usuário
		userInfoMap, err := client.WAClient.GetUserInfo([]types.JID{myJID})
		if err == nil {
			if userInfo, ok := userInfoMap[deviceID.ToNonAD()]; ok {
                
                deviceInfo.Status = userInfo.Status
			}
		}
	}
	
	return deviceInfo, nil
}

// GetConnectionState obtém o estado da conexão
func (sm *SessionManager) GetConnectionState(userID string) (string, error) {
    client, exists := sm.GetSession(userID)
    if !exists {
        return "", fmt.Errorf("sessão não encontrada: %s", userID)
    }
    
    // Em vez de tentar usar um método GetConnectionState inexistente,
    // vamos verificar o estado da conexão de outras maneiras
    
    // Verificar se a propriedade client.Connected está definida em nossa estrutura
    if client.Connected {
        return "connected", nil
    }
    
    // Verificar se o cliente tem um ID (está registrado)
    if client.WAClient.Store.ID != nil {
        // O cliente tem um ID, mas não está conectado atualmente
        
        // Podemos verificar quando foi a última conexão bem-sucedida
        if !client.WAClient.LastSuccessfulConnect.IsZero() {
            // Se a última conexão foi recente, considere como "desconectado"
            // caso contrário, pode estar "logged_out"
            if time.Since(client.WAClient.LastSuccessfulConnect) < 24*time.Hour {
                return "disconnected", nil
            }
        }
        
        return "logged_out", nil
    }
    
    // Sem ID, nunca foi autenticado
    return "not_authenticated", nil
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
    
    // Em vez de tentar acessar métodos não exportados, usamos SetGroupPhoto do próprio cliente
    // mas com o JID do usuário em vez de um grupo
    selfJID := client.WAClient.Store.ID.ToNonAD()
    
    // Como SetGroupPhoto usa o mesmo namespace, deveria funcionar de forma semelhante
    _, err := client.WAClient.SetGroupPhoto(selfJID, imageData)
    if err != nil {
        return fmt.Errorf("falha ao atualizar foto de perfil: %w", err)
    }
    
    logger.Debug("Foto de perfil atualizada com sucesso", "user_id", userID)
    
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
    
    // Criar parâmetros vazios, como no exemplo
    // Note que é do pacote "whatsmeow" e não "types"
    params := &whatsmeow.GetProfilePictureParams{}
    
    // Chamar GetProfilePictureInfo com os argumentos corretos
    profilePic, err := client.WAClient.GetProfilePictureInfo(targetJID, params)
    if err != nil {
        return "", fmt.Errorf("falha ao obter foto de perfil: %w", err)
    }
    
    if profilePic == nil {
        return "", fmt.Errorf("foto de perfil não encontrada ou não foi alterada")
    }
    
    if profilePic.URL == "" {
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
    
    // Criar contexto com timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Chamar Logout com o contexto
    err := client.WAClient.Logout(ctx)
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
    
    // IsOnWhatsApp espera números no formato internacional com o prefixo +
    // Verificar status diretamente com o slice de strings
    responses, err := client.WAClient.IsOnWhatsApp(numbers)
    if err != nil {
        return nil, fmt.Errorf("falha ao verificar números: %w", err)
    }
    
    // Converter resultados para um mapa
    statusMap := make(map[string]bool)
    for _, resp := range responses {
        statusMap[resp.Query] = resp.IsIn
    }
    
    return statusMap, nil
}