// internal/services/whatsapp/newsletter.go
package messaging

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"yourproject/internal/services/whatsapp/session"
	"yourproject/pkg/logger"
)

// NewsletterService encapsula a funcionalidade de newsletters (canais) do WhatsApp
type NewsletterService struct {
	sessionManager session.Manager
}

// NewNewsletterService cria um novo serviço de newsletter
func NewNewsletterService(sessionManager session.Manager) *NewsletterService {
	return &NewsletterService{
		sessionManager: sessionManager,
	}
}

// getClient obtém o cliente WhatsApp para uma sessão específica
func (s *NewsletterService) getClient(userID string) (*whatsmeow.Client, error) {
	client, exists := s.sessionManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.Connected {
		return nil, fmt.Errorf("sessão não está conectada: %s", userID)
	}

	return client.WAClient, nil
}

// CreateChannel cria um novo canal do WhatsApp com o nome, descrição e imagem opcional
func (s *NewsletterService) CreateChannel(ctx context.Context, userID, name, description string, picture []byte) (*types.NewsletterMetadata, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Primeiro, aceitar os Termos de Serviço para criar newsletters
	if err := client.AcceptTOSNotice("20601218", "5"); err != nil {
		logger.Error("Falha ao aceitar Termos de Serviço para newsletters", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao aceitar termos de serviço: %w", err)
	}

	// Então criar o newsletter/canal
	metadata, err := client.CreateNewsletter(whatsmeow.CreateNewsletterParams{
		Name:        name,
		Description: description,
		Picture:     picture,
	})

	if err != nil {
		logger.Error("Falha ao criar canal", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao criar canal: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return metadata, nil
}

// GetChannelInfo obtém informações sobre um canal específico
func (s *NewsletterService) GetChannelInfo(ctx context.Context, userID string, jid types.JID) (*types.NewsletterMetadata, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	metadata, err := client.GetNewsletterInfo(jid)
	if err != nil {
		logger.Error("Falha ao obter informações do canal", "error", err, "user_id", userID, "jid", jid.String())
		return nil, fmt.Errorf("falha ao obter informações do canal: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return metadata, nil
}

// GetChannelWithInvite obtém informações do canal usando um link de convite
func (s *NewsletterService) GetChannelWithInvite(ctx context.Context, userID string, inviteLink string) (*types.NewsletterMetadata, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	metadata, err := client.GetNewsletterInfoWithInvite(inviteLink)
	if err != nil {
		logger.Error("Falha ao obter informações do canal com convite", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao obter informações do canal com convite: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return metadata, nil
}

// ListMyChannels retorna todos os canais que o usuário está inscrito
func (s *NewsletterService) ListMyChannels(ctx context.Context, userID string) ([]*types.NewsletterMetadata, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	channels, err := client.GetSubscribedNewsletters()
	if err != nil {
		logger.Error("Falha ao listar canais inscritos", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao listar canais inscritos: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return channels, nil
}

// FollowChannel inscreve o usuário em um canal
func (s *NewsletterService) FollowChannel(ctx context.Context, userID string, jid types.JID) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	if err := client.FollowNewsletter(jid); err != nil {
		logger.Error("Falha ao seguir canal", "error", err, "user_id", userID, "jid", jid.String())
		return fmt.Errorf("falha ao seguir canal: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return nil
}

// UnfollowChannel cancela a inscrição do usuário em um canal
func (s *NewsletterService) UnfollowChannel(ctx context.Context, userID string, jid types.JID) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	if err := client.UnfollowNewsletter(jid); err != nil {
		logger.Error("Falha ao deixar de seguir canal", "error", err, "user_id", userID, "jid", jid.String())
		return fmt.Errorf("falha ao deixar de seguir canal: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return nil
}

// MuteChannel silencia notificações de um canal
func (s *NewsletterService) MuteChannel(ctx context.Context, userID string, jid types.JID) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	if err := client.NewsletterToggleMute(jid, true); err != nil {
		logger.Error("Falha ao silenciar canal", "error", err, "user_id", userID, "jid", jid.String())
		return fmt.Errorf("falha ao silenciar canal: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return nil
}

// UnmuteChannel reativa notificações de um canal
func (s *NewsletterService) UnmuteChannel(ctx context.Context, userID string, jid types.JID) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	if err := client.NewsletterToggleMute(jid, false); err != nil {
		logger.Error("Falha ao reativar notificações do canal", "error", err, "user_id", userID, "jid", jid.String())
		return fmt.Errorf("falha ao reativar notificações do canal: %w", err)
	}

	// Atualizar última atividade
	if client, exists := s.sessionManager.GetSession(userID); exists {
		client.LastActive = time.Now()
	}

	return nil
}
