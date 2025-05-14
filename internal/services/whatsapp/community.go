// internal/services/whatsapp/community.go
package whatsapp

import (
	"context"
	"fmt"
	"time"
	
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	
	"yourproject/pkg/logger"
)

// CommunityInfo representa informações de uma comunidade
type CommunityInfo struct {
	JID           string       `json:"jid"`
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	Created       time.Time    `json:"created"`
	Creator       string       `json:"creator"`
	LinkedGroups  []GroupInfo  `json:"linked_groups"`
	AnnouncementGroups []GroupInfo `json:"announcement_groups"`
}

// CreateCommunity cria uma nova comunidade
func (sm *SessionManager) CreateCommunity(userID, name, description string) (*CommunityInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	// Criar comunidade
	communityJID, err := client.WAClient.CreateCommunity(ctx, name, description)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Comunidade criada", "user_id", userID, "community_name", name, "community_jid", communityJID.String())
	
	// Obter informações da comunidade recém-criada
	return sm.GetCommunityInfo(userID, communityJID.String())
}

// GetCommunityInfo obtém informações de uma comunidade
func (sm *SessionManager) GetCommunityInfo(userID, communityJID string) (*CommunityInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID
	jid, err := types.ParseJID(communityJID)
	if err != nil {
		return nil, fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !jid.IsCommunity() {
		return nil, fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Obter informações da comunidade
	info, err := client.WAClient.GetCommunityInfo(ctx, jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter informações da comunidade: %w", err)
	}
	
	// Obter grupos vinculados
	linkedGroups := make([]GroupInfo, 0)
	announcementGroups := make([]GroupInfo, 0)
	
	// Processar grupos vinculados
	for _, groupJID := range info.LinkedGroups {
		groupInfo, err := sm.GetGroupInfo(userID, groupJID.String())
		if err != nil {
			logger.Warn("Falha ao obter informações do grupo vinculado", 
				"user_id", userID, 
				"community_jid", communityJID, 
				"group_jid", groupJID.String(), 
				"error", err)
			continue
		}
		linkedGroups = append(linkedGroups, *groupInfo)
	}
	
	// Processar grupos de anúncios
	for _, groupJID := range info.AnnouncementGroups {
		groupInfo, err := sm.GetGroupInfo(userID, groupJID.String())
		if err != nil {
			logger.Warn("Falha ao obter informações do grupo de anúncios", 
				"user_id", userID, 
				"community_jid", communityJID, 
				"group_jid", groupJID.String(), 
				"error", err)
			continue
		}
		announcementGroups = append(announcementGroups, *groupInfo)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	return &CommunityInfo{
		JID:           jid.String(),
		Name:          info.Name,
		Description:   info.Description,
		Created:       time.Unix(int64(info.Creation), 0),
		Creator:       info.Creator.String(),
		LinkedGroups:  linkedGroups,
		AnnouncementGroups: announcementGroups,
	}, nil
}

// UpdateCommunityName atualiza o nome de uma comunidade
func (sm *SessionManager) UpdateCommunityName(userID, communityJID, newName string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID
	jid, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !jid.IsCommunity() {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Atualizar nome da comunidade
	err = client.WAClient.UpdateCommunityName(ctx, jid, newName)
	if err != nil {
		return fmt.Errorf("falha ao atualizar nome da comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Nome da comunidade atualizado", 
		"user_id", userID, 
		"community_jid", communityJID, 
		"new_name", newName)
	
	return nil
}

// UpdateCommunityDescription atualiza a descrição de uma comunidade
func (sm *SessionManager) UpdateCommunityDescription(userID, communityJID, newDescription string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID
	jid, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !jid.IsCommunity() {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Atualizar descrição da comunidade
	err = client.WAClient.UpdateCommunityDescription(ctx, jid, newDescription)
	if err != nil {
		return fmt.Errorf("falha ao atualizar descrição da comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Descrição da comunidade atualizada", 
		"user_id", userID, 
		"community_jid", communityJID)
	
	return nil
}

// LeaveCommunity sai de uma comunidade
func (sm *SessionManager) LeaveCommunity(userID, communityJID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID
	jid, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !jid.IsCommunity() {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Sair da comunidade
	err = client.WAClient.LeaveCommunity(ctx, jid)
	if err != nil {
		return fmt.Errorf("falha ao sair da comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Saiu da comunidade", 
		"user_id", userID, 
		"community_jid", communityJID)
	
	return nil
}

// GetJoinedCommunities obtém lista de comunidades em que o usuário é membro
func (sm *SessionManager) GetJoinedCommunities(userID string) ([]CommunityInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	// Obter lista de comunidades
	communities, err := client.WAClient.GetJoinedCommunities(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter lista de comunidades: %w", err)
	}
	
	// Converter para formato de resposta
	result := make([]CommunityInfo, len(communities))
	for i, community := range communities {
		// Obter grupos vinculados
		linkedGroups := make([]GroupInfo, 0)
		announcementGroups := make([]GroupInfo, 0)
		
		// Processar grupos vinculados
		for _, groupJID := range community.LinkedGroups {
			groupInfo, err := sm.GetGroupInfo(userID, groupJID.String())
			if err != nil {
				logger.Warn("Falha ao obter informações do grupo vinculado", 
					"user_id", userID, 
					"community_jid", community.JID.String(), 
					"group_jid", groupJID.String(), 
					"error", err)
				continue
			}
			linkedGroups = append(linkedGroups, *groupInfo)
		}
		
		// Processar grupos de anúncios
		for _, groupJID := range community.AnnouncementGroups {
			groupInfo, err := sm.GetGroupInfo(userID, groupJID.String())
			if err != nil {
				logger.Warn("Falha ao obter informações do grupo de anúncios", 
					"user_id", userID, 
					"community_jid", community.JID.String(), 
					"group_jid", groupJID.String(), 
					"error", err)
				continue
			}
			announcementGroups = append(announcementGroups, *groupInfo)
		}
		
		result[i] = CommunityInfo{
			JID:           community.JID.String(),
			Name:          community.Name,
			Description:   community.Description,
			Created:       time.Unix(int64(community.Creation), 0),
			Creator:       community.Creator.String(),
			LinkedGroups:  linkedGroups,
			AnnouncementGroups: announcementGroups,
		}
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Lista de comunidades obtida", 
		"user_id", userID, 
		"communities_count", len(communities))
	
	return result, nil
}

// CreateGroupForCommunity cria um novo grupo dentro de uma comunidade
func (sm *SessionManager) CreateGroupForCommunity(userID, communityJID, groupName string, participants []string) (*GroupInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID de comunidade
	communityID, err := types.ParseJID(communityJID)
	if err != nil {
		return nil, fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !communityID.IsCommunity() {
		return nil, fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := ParseJID(p)
		if err != nil {
			return nil, fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	// Criar grupo na comunidade
	groupJID, err := client.WAClient.CreateGroupInCommunity(ctx, communityID, groupName, jids)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar grupo na comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Grupo criado na comunidade", 
		"user_id", userID, 
		"community_jid", communityJID, 
		"group_name", groupName, 
		"group_jid", groupJID.String())
	
	// Obter informações do grupo recém-criado
	return sm.GetGroupInfo(userID, groupJID.String())
}

// LinkGroupToCommunity vincula um grupo existente a uma comunidade
func (sm *SessionManager) LinkGroupToCommunity(userID, communityJID, groupJID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID de comunidade
	communityID, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !communityID.IsCommunity() {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Converter para JID de grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}
	
	// Verificar se é realmente um grupo
	if !groupID.IsGroup() {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Vincular grupo à comunidade
	err = client.WAClient.LinkGroupToCommuntiy(ctx, communityID, groupID)
	if err != nil {
		return fmt.Errorf("falha ao vincular grupo à comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Grupo vinculado à comunidade", 
		"user_id", userID, 
		"community_jid", communityJID, 
		"group_jid", groupJID)
	
	return nil
}

// UnlinkGroupFromCommunity desvincula um grupo de uma comunidade
func (sm *SessionManager) UnlinkGroupFromCommunity(userID, communityJID, groupJID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID de comunidade
	communityID, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !communityID.IsCommunity() {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Converter para JID de grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}
	
	// Verificar se é realmente um grupo
	if !groupID.IsGroup() {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Desvincular grupo da comunidade
	err = client.WAClient.UnlinkGroupFromCommunity(ctx, communityID, groupID)
	if err != nil {
		return fmt.Errorf("falha ao desvincular grupo da comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Grupo desvinculado da comunidade", 
		"user_id", userID, 
		"community_jid", communityJID, 
		"group_jid", groupJID)
	
	return nil
}

// SendCommunityAnnouncement envia um anúncio para todos os grupos vinculados a uma comunidade
func (sm *SessionManager) SendCommunityAnnouncement(userID, communityJID, message string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID de comunidade
	communityID, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !communityID.IsCommunity() {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	// Enviar anúncio para a comunidade
	err = client.WAClient.SendCommunityAnnouncement(ctx, communityID, message)
	if err != nil {
		return fmt.Errorf("falha ao enviar anúncio para a comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Anúncio enviado para a comunidade", 
		"user_id", userID, 
		"community_jid", communityJID)
	
	return nil
}

// JoinCommunityWithLink entra em uma comunidade usando um link de convite
func (sm *SessionManager) JoinCommunityWithLink(userID, link string) (*CommunityInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	// Entrar na comunidade
	communityJID, err := client.WAClient.JoinCommunityWithLink(ctx, link)
	if err != nil {
		return nil, fmt.Errorf("falha ao entrar na comunidade: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Entrou na comunidade via link", 
		"user_id", userID, 
		"community_jid", communityJID.String())
	
	// Obter informações da comunidade
	return sm.GetCommunityInfo(userID, communityJID.String())
}

// GetCommunityInviteLink gera e retorna um link de convite para a comunidade
func (sm *SessionManager) GetCommunityInviteLink(userID, communityJID string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID
	jid, err := types.ParseJID(communityJID)
	if err != nil {
		return "", fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !jid.IsCommunity() {
		return "", fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Obter link de convite
	link, err := client.WAClient.GetCommunityInviteLink(ctx, jid, false)
	if err != nil {
		return "", fmt.Errorf("falha ao obter link de convite: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Link de convite obtido", 
		"user_id", userID, 
		"community_jid", communityJID)
	
	return link, nil
}

// RevokeCommunityInviteLink revoga o link atual e gera um novo
func (sm *SessionManager) RevokeCommunityInviteLink(userID, communityJID string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}
	
	// Converter para JID
	jid, err := types.ParseJID(communityJID)
	if err != nil {
		return "", fmt.Errorf("JID de comunidade inválido: %w", err)
	}
	
	// Verificar se é realmente uma comunidade
	if !jid.IsCommunity() {
		return "", fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Revogar link atual e obter novo
	link, err := client.WAClient.GetCommunityInviteLink(ctx, jid, true)
	if err != nil {
		return "", fmt.Errorf("falha ao revogar link de convite: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Link de convite da comunidade revogado e novo link obtido", 
		"user_id", userID, 
		"community_jid", communityJID)
	
	return link, nil
}