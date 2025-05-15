// internal/services/whatsapp/community.go
package whatsapp

import (
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"yourproject/pkg/logger"
)

// CommunityInfo representa informações de uma comunidade
type CommunityInfo struct {
	JID                string      `json:"jid"`
	Name               string      `json:"name"`
	Description        string      `json:"description,omitempty"`
	Created            time.Time   `json:"created"`
	Creator            string      `json:"creator"`
	LinkedGroups       []GroupInfo `json:"linked_groups"`
	AnnouncementGroups []GroupInfo `json:"announcement_groups"`
}

// CreateCommunity cria uma nova comunidade no WhatsApp
func (sm *SessionManager) CreateCommunity(userID, name string, participants []string) (*types.GroupInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter strings de participantes para JIDs
	var jids []types.JID
	for _, participant := range participants {
		jid, err := types.ParseJID(participant)
		if err != nil {
			return nil, fmt.Errorf("JID inválido para participante %s: %w", participant, err)
		}
		jids = append(jids, jid)
	}

	// Criar a requisição com IsParent definido como true para fazer uma comunidade
	req := whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: jids,
		GroupParent: types.GroupParent{
			IsParent: true,
			// Modo de aprovação padrão para grupos de comunidade
			DefaultMembershipApprovalMode: "request_required", // Opções: "request_required" ou "no_approval"
		},
	}

	// Usar o método CreateGroup para criar a comunidade
	groupInfo, err := client.WAClient.CreateGroup(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar comunidade: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Comunidade criada com sucesso",
		"user_id", userID,
		"name", name,
		"jid", groupInfo.JID.String())

	return groupInfo, nil
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

	// Verificar se é realmente uma comunidade/grupo
	if jid.Server != types.GroupServer {
		return nil, fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Obter informações da comunidade
	info, err := client.WAClient.GetGroupInfo(jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter informações da comunidade: %w", err)
	}

	// Verificar se é uma comunidade (IsParent deve ser true)
	if !info.IsParent {
		return nil, fmt.Errorf("o JID não é uma comunidade, é um grupo normal: %s", communityJID)
	}

	// Obter subgrupos da comunidade
	subGroups, err := client.WAClient.GetSubGroups(jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter subgrupos da comunidade: %w", err)
	}

	// Processar grupos vinculados
	linkedGroups := make([]GroupInfo, 0)
	announcementGroups := make([]GroupInfo, 0)

	// Processar todos os subgrupos
	for _, group := range subGroups {
		groupInfo, err := sm.GetGroupInfo(userID, group.JID.String())
		if err != nil {
			logger.Warn("Falha ao obter informações do grupo vinculado",
				"user_id", userID,
				"community_jid", communityJID,
				"group_jid", group.JID.String(),
				"error", err)
			continue
		}

		// Classificar como grupo de anúncio ou grupo normal
		if group.IsDefaultSubGroup {
			announcementGroups = append(announcementGroups, *groupInfo)
		} else {
			linkedGroups = append(linkedGroups, *groupInfo)
		}
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	return &CommunityInfo{
		JID:                jid.String(),
		Name:               info.Name,
		Description:        info.Topic,
		Creator:            info.OwnerJID.String(),
		Created:            info.GroupCreated,
		LinkedGroups:       linkedGroups,
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
	if jid.Server != types.GroupServer {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Atualizar nome da comunidade
	err = client.WAClient.SetGroupName(jid, newName)
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
	if jid.Server != types.GroupServer {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Atualizar descrição da comunidade usando SetGroupTopic
	// O método SetGroupDescription não recebe o contexto
	err = client.WAClient.SetGroupTopic(jid, "", "", newDescription)
	if err != nil {
		return fmt.Errorf("falha ao atualizar descrição da comunidade: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Descrição da comunidade atualizada",
		"user_id", userID,
		"community_jid", communityJID,
		"new_description", newDescription)

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
	if jid.Server != types.GroupServer {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Sair da comunidade usando LeaveGroup
	err = client.WAClient.LeaveGroup(jid)
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

	// Get all groups
	allGroups, err := client.WAClient.GetJoinedGroups()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter lista de grupos: %w", err)
	}

	// Filter for communities (groups with IsParent=true)
	result := make([]CommunityInfo, 0)
	for _, group := range allGroups {
		if group.IsParent {
			// Get sub-groups for this community
			subGroups, err := client.WAClient.GetSubGroups(group.JID)
			if err != nil {
				logger.Warn("Falha ao obter subgrupos da comunidade",
					"user_id", userID,
					"community_jid", group.JID.String(),
					"error", err)
				continue
			}

			// Process linked groups and announcement groups
			linkedGroups := make([]GroupInfo, 0)
			announcementGroups := make([]GroupInfo, 0)

			for _, subGroup := range subGroups {
				groupInfo, err := sm.GetGroupInfo(userID, subGroup.JID.String())
				if err != nil {
					logger.Warn("Falha ao obter informações do grupo",
						"user_id", userID,
						"community_jid", group.JID.String(),
						"group_jid", subGroup.JID.String(),
						"error", err)
					continue
				}

				if subGroup.IsDefaultSubGroup {
					announcementGroups = append(announcementGroups, *groupInfo)
				} else {
					linkedGroups = append(linkedGroups, *groupInfo)
				}
			}

			// Create community info
			communityInfo := CommunityInfo{
				JID:                group.JID.String(),
				Name:               group.Name,
				Description:        group.Topic,
				Created:            group.GroupCreated,
				Creator:            group.OwnerJID.String(),
				LinkedGroups:       linkedGroups,
				AnnouncementGroups: announcementGroups,
			}

			result = append(result, communityInfo)
		}
	}

	// Update last activity
	client.LastActive = time.Now()

	// Log
	logger.Debug("Lista de comunidades obtida",
		"user_id", userID,
		"communities_count", len(result))

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
	if communityID.Server != types.GroupServer {
		return nil, fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return nil, fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Preparar requisição para criar grupo na comunidade
	req := whatsmeow.ReqCreateGroup{
		Name:         groupName,
		Participants: jids,
		GroupLinkedParent: types.GroupLinkedParent{
			LinkedParentJID: communityID, // Link o grupo à comunidade
		},
	}

	// Criar grupo na comunidade
	waGroupInfo, err := client.WAClient.CreateGroup(req)
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
		"group_jid", waGroupInfo.JID.String())

	// Converter groupInfo do whatsmeow para nosso tipo GroupInfo
	groupParticipants := make([]GroupParticipant, len(waGroupInfo.Participants))
	for i, p := range waGroupInfo.Participants {
		groupParticipants[i] = GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		}
	}

	// Criar e retornar nosso tipo GroupInfo
	groupInfo := &GroupInfo{
		JID:          waGroupInfo.JID.String(),
		Name:         waGroupInfo.Name,
		Topic:        waGroupInfo.Topic,
		Creator:      waGroupInfo.OwnerJID.String(),
		Participants: groupParticipants,
	}

	return groupInfo, nil
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
	if communityID.Server != types.GroupServer {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Converter para JID de grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Link group to community
	err = client.WAClient.LinkGroup(communityID, groupID)
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
	if communityID.Server != types.GroupServer {
		return fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Converter para JID de grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Desvincular grupo da comunidade usando UnlinkGroup
	err = client.WAClient.UnlinkGroup(communityID, groupID)
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
	if jid.Server != types.GroupServer {
		return "", fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Obter link de convite usando GetGroupInviteLink
	link, err := client.WAClient.GetGroupInviteLink(jid, false)
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
	if jid.Server != types.GroupServer {
		return "", fmt.Errorf("JID não é uma comunidade: %s", communityJID)
	}

	// Revogar link atual e obter novo usando GetGroupInviteLink com reset=true
	link, err := client.WAClient.GetGroupInviteLink(jid, true)
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

// JoinCommunityWithLink entra em uma comunidade usando um link de convite
func (sm *SessionManager) JoinCommunityWithLink(userID, link string) (*CommunityInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Entrar na comunidade usando JoinGroupWithLink
	communityJID, err := client.WAClient.JoinGroupWithLink(link)
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
