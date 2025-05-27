// internal/services/whatsapp/newsletter.go
package messaging

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"yourproject/internal/services/whatsapp/session"
	"yourproject/pkg/logger"
)

// NewsletterService encapsula a funcionalidade de newsletters (canais) do WhatsApp
type NewsletterService struct {
	newsletterManager session.NewsletterManager
}

// NewNewsletterService cria um novo serviço de newsletter
func NewNewsletterService(newsletterManager session.NewsletterManager) *NewsletterService {
	return &NewsletterService{
		newsletterManager: newsletterManager,
	}
}

// getClient obtém o cliente WhatsApp para uma sessão específica
func (s *NewsletterService) getClient(userID string) (*whatsmeow.Client, error) {
	client, exists := s.newsletterManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não está conectada: %s", userID)
	}

	waClient := client.GetWAClient()
	if waClient == nil {
		return nil, fmt.Errorf("cliente WhatsApp não disponível: %s", userID)
	}

	// Update activity
	client.UpdateActivity()

	return waClient, nil
}

// CreateChannel cria um novo canal do WhatsApp com o nome, descrição e imagem opcional
func (s *NewsletterService) CreateChannel(userID, name, description, pictureURL string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Download picture if URL is provided
	var picture []byte
	if pictureURL != "" {
		httpClient := &http.Client{
			Timeout: 30 * time.Second,
		}
		resp, err := httpClient.Get(pictureURL)
		if err != nil {
			return nil, fmt.Errorf("falha ao baixar imagem da URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("falha ao baixar imagem da URL: status %d", resp.StatusCode)
		}

		picture, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler conteúdo da resposta: %w", err)
		}
	}

	// Primeiro, aceitar os Termos de Serviço para criar newsletters
	if err := client.AcceptTOSNotice("20601218", "5"); err != nil {
		logger.Error("Falha ao aceitar Termos de Serviço para newsletters", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao aceitar termos de serviço: %w", err)
	}

	// Então criar o newsletter/canal
	result, err := client.CreateNewsletter(whatsmeow.CreateNewsletterParams{
		Name:        name,
		Description: description,
		Picture:     picture,
	})

	if err != nil {
		logger.Error("Falha ao criar canal", "error", err, "user_id", userID, "name", name)
		return nil, fmt.Errorf("falha ao criar canal: %w", err)
	}

	return result, nil
}

// GetChannelInfo obtém informações sobre um canal específico
func (s *NewsletterService) GetChannelInfo(userID, jid string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("JID inválido: %w", err)
	}

	info, err := client.GetNewsletterInfo(parsedJID)
	if err != nil {
		logger.Error("Falha ao obter informações do canal", "error", err, "user_id", userID, "jid", jid)
		return nil, fmt.Errorf("falha ao obter informações do canal: %w", err)
	}

	return info, nil
}

// GetChannelWithInvite obtém informações do canal usando um link de convite
func (s *NewsletterService) GetChannelWithInvite(userID, inviteLink string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	result, err := client.GetNewsletterInfoWithInvite(inviteLink)
	if err != nil {
		logger.Error("Falha ao obter canal com convite", "error", err, "user_id", userID, "invite", inviteLink)
		return nil, fmt.Errorf("falha ao obter canal com convite: %w", err)
	}

	return result, nil
}

// ListMyChannels retorna todos os canais que o usuário está inscrito
func (s *NewsletterService) ListMyChannels(userID string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Get subscribed newsletters
	subscribed, err := client.GetSubscribedNewsletters()
	if err != nil {
		logger.Error("Falha ao listar canais inscritos", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao listar canais: %w", err)
	}

	return subscribed, nil
}

// FollowChannel inscreve o usuário em um canal
func (s *NewsletterService) FollowChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.FollowNewsletter(parsedJID); err != nil {
		logger.Error("Falha ao seguir canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao seguir canal: %w", err)
	}

	return nil
}

// UnfollowChannel cancela a inscrição do usuário em um canal
func (s *NewsletterService) UnfollowChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.UnfollowNewsletter(parsedJID); err != nil {
		logger.Error("Falha ao deixar de seguir canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao deixar de seguir canal: %w", err)
	}

	return nil
}

// MuteChannel silencia notificações de um canal
func (s *NewsletterService) MuteChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.NewsletterToggleMute(parsedJID, true); err != nil {
		logger.Error("Falha ao silenciar canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao silenciar canal: %w", err)
	}

	return nil
}

// UnmuteChannel reativa notificações de um canal
func (s *NewsletterService) UnmuteChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.NewsletterToggleMute(parsedJID, false); err != nil {
		logger.Error("Falha ao reativar notificações do canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao reativar notificações do canal: %w", err)
	}

	return nil
}
