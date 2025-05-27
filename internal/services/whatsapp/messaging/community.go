// internal/services/whatsapp/messaging/community.go
package messaging

import (
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"yourproject/internal/services/whatsapp/session"
	"yourproject/pkg/logger"
)

// CommunityService provides community management functionality
type CommunityService struct {
	communityManager session.CommunityManager
}

// NewCommunityService creates a new community service
func NewCommunityService(communityManager session.CommunityManager) *CommunityService {
	return &CommunityService{
		communityManager: communityManager,
	}
}

// getClient obtém um cliente ativo para operações de comunidade
func (cs *CommunityService) getClient(userID string) (*whatsmeow.Client, error) {
	client, exists := cs.communityManager.GetSession(userID)
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

// CreateCommunity cria uma nova comunidade no WhatsApp
func (cs *CommunityService) CreateCommunity(userID, name, description string) (interface{}, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Create the community first
	req := whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: []types.JID{}, // Empty participants list
		GroupParent: types.GroupParent{
			IsParent:                      true,
			DefaultMembershipApprovalMode: "request_required",
		},
	}

	groupInfo, err := client.CreateGroup(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar comunidade: %w", err)
	}

	// Set the description separately if provided
	if description != "" {
		err = client.SetGroupTopic(groupInfo.JID, "", "", description)
		if err != nil {
			logger.Warn("Falha ao definir descrição da comunidade",
				"user_id", userID,
				"community_jid", groupInfo.JID.String(),
				"error", err)
			// Don't fail the entire operation for description error
		}
	}

	logger.Info("Comunidade criada com sucesso",
		"user_id", userID,
		"community_jid", groupInfo.JID.String(),
		"name", name)

	return groupInfo, nil
}

// GetCommunityInfo obtém informações de uma comunidade
func (cs *CommunityService) GetCommunityInfo(userID, communityJID string) (interface{}, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return nil, err
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
	info, err := client.GetGroupInfo(jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter informações da comunidade: %w", err)
	}

	// Verificar se é uma comunidade (IsParent deve ser true)
	if !info.IsParent {
		return nil, fmt.Errorf("o JID não é uma comunidade, é um grupo normal: %s", communityJID)
	}

	// Obter subgrupos da comunidade
	subGroups, err := client.GetSubGroups(jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter subgrupos da comunidade: %w", err)
	}

	// Processar grupos vinculados
	linkedGroups := make([]GroupInfo, 0)
	announcementGroups := make([]GroupInfo, 0)

	// Processar todos os subgrupos
	for _, group := range subGroups {
		groupInfo, err := cs.GetGroupInfo(userID, group.JID.String())
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

// GetGroupInfo obtém informações de um grupo específico
func (cs *CommunityService) GetGroupInfo(userID, groupJID string) (*GroupInfo, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Converter para JID
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Obter informações do grupo
	info, err := client.GetGroupInfo(jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter informações do grupo: %w", err)
	}

	// Processar participantes
	participants := make([]GroupParticipant, len(info.Participants))
	for i, p := range info.Participants {
		participants[i] = GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		}
	}

	return &GroupInfo{
		JID:          jid.String(),
		Name:         info.Name,
		Topic:        info.Topic,
		Creator:      info.OwnerJID.String(),
		Participants: participants,
	}, nil
}

// UpdateCommunityName atualiza o nome de uma comunidade
func (cs *CommunityService) UpdateCommunityName(userID, communityJID, newName string) error {
	client, err := cs.getClient(userID)
	if err != nil {
		return err
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
	err = client.SetGroupName(jid, newName)
	if err != nil {
		return fmt.Errorf("falha ao atualizar nome da comunidade: %w", err)
	}

	logger.Debug("Nome da comunidade atualizado",
		"user_id", userID,
		"community_jid", communityJID,
		"new_name", newName)

	return nil
}

// UpdateCommunityDescription atualiza a descrição de uma comunidade
func (cs *CommunityService) UpdateCommunityDescription(userID, communityJID, newDescription string) error {
	client, err := cs.getClient(userID)
	if err != nil {
		return err
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
	err = client.SetGroupTopic(jid, "", "", newDescription)
	if err != nil {
		return fmt.Errorf("falha ao atualizar descrição da comunidade: %w", err)
	}

	logger.Debug("Descrição da comunidade atualizada",
		"user_id", userID,
		"community_jid", communityJID,
		"new_description", newDescription)

	return nil
}

// LeaveCommunity sai de uma comunidade
func (cs *CommunityService) LeaveCommunity(userID, communityJID string) error {
	client, err := cs.getClient(userID)
	if err != nil {
		return err
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
	err = client.LeaveGroup(jid)
	if err != nil {
		return fmt.Errorf("falha ao sair da comunidade: %w", err)
	}

	logger.Debug("Saiu da comunidade",
		"user_id", userID,
		"community_jid", communityJID)

	return nil
}

// GetJoinedCommunities obtém lista de comunidades em que o usuário é membro
func (cs *CommunityService) GetJoinedCommunities(userID string) (interface{}, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Get all groups
	allGroups, err := client.GetJoinedGroups()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter lista de grupos: %w", err)
	}

	// Filter for communities (groups with IsParent=true)
	result := make([]CommunityInfo, 0)
	for _, group := range allGroups {
		if group.IsParent {
			// Get sub-groups for this community
			subGroups, err := client.GetSubGroups(group.JID)
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
				groupInfo, err := cs.GetGroupInfo(userID, subGroup.JID.String())
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

	logger.Debug("Lista de comunidades obtida",
		"user_id", userID,
		"communities_count", len(result))

	return result, nil
}

// CreateGroupForCommunity cria um novo grupo dentro de uma comunidade
func (cs *CommunityService) CreateGroupForCommunity(userID, communityJID, groupName string, participants []string) (interface{}, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return nil, err
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
	waGroupInfo, err := client.CreateGroup(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar grupo na comunidade: %w", err)
	}

	logger.Debug("Novo grupo criado na comunidade",
		"user_id", userID,
		"community_jid", communityJID,
		"group_jid", waGroupInfo.JID.String(),
		"group_name", groupName)

	return waGroupInfo, nil
}

// JoinCommunityWithLink entra em uma comunidade usando um link de convite
func (cs *CommunityService) JoinCommunityWithLink(userID, link string) (interface{}, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Entrar na comunidade usando JoinGroupWithLink
	communityJID, err := client.JoinGroupWithLink(link)
	if err != nil {
		return nil, fmt.Errorf("falha ao entrar na comunidade: %w", err)
	}

	logger.Debug("Entrou na comunidade via link",
		"user_id", userID,
		"community_jid", communityJID.String())

	// Obter informações da comunidade
	return cs.GetCommunityInfo(userID, communityJID.String())
}

// GetCommunityInviteLink obtém o link de convite de uma comunidade
func (cs *CommunityService) GetCommunityInviteLink(userID, communityJID string) (string, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return "", err
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
	link, err := client.GetGroupInviteLink(jid, false)
	if err != nil {
		return "", fmt.Errorf("falha ao obter link de convite da comunidade: %w", err)
	}

	logger.Debug("Link de convite da comunidade obtido",
		"user_id", userID,
		"community_jid", communityJID)

	return link, nil
}

// RevokeCommunityInviteLink revoga o link atual e gera um novo
func (cs *CommunityService) RevokeCommunityInviteLink(userID, communityJID string) (string, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return "", err
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

	// Revogar e obter novo link usando GetGroupInviteLink com reset=true
	link, err := client.GetGroupInviteLink(jid, true)
	if err != nil {
		return "", fmt.Errorf("falha ao revogar link de convite da comunidade: %w", err)
	}

	logger.Debug("Link de convite da comunidade revogado e novo gerado",
		"user_id", userID,
		"community_jid", communityJID)

	return link, nil
}

// LinkGroupToCommunity vincula um grupo existente a uma comunidade
func (cs *CommunityService) LinkGroupToCommunity(userID, communityJID, groupJID string) error {
	client, err := cs.getClient(userID)
	if err != nil {
		return err
	}

	// Converter para JID de comunidade
	communityID, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}

	// Converter para JID de grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se são realmente grupos
	if communityID.Server != types.GroupServer || groupID.Server != types.GroupServer {
		return fmt.Errorf("JIDs devem ser de grupos/comunidades")
	}

	// Vincular grupo à comunidade usando LinkGroup
	err = client.LinkGroup(communityID, groupID)
	if err != nil {
		return fmt.Errorf("falha ao vincular grupo à comunidade: %w", err)
	}

	logger.Debug("Grupo vinculado à comunidade",
		"user_id", userID,
		"community_jid", communityJID,
		"group_jid", groupJID)

	return nil
}

// UnlinkGroupFromCommunity desvincula um grupo de uma comunidade
func (cs *CommunityService) UnlinkGroupFromCommunity(userID, communityJID, groupJID string) error {
	client, err := cs.getClient(userID)
	if err != nil {
		return err
	}

	// Converter para JID de comunidade
	communityID, err := types.ParseJID(communityJID)
	if err != nil {
		return fmt.Errorf("JID de comunidade inválido: %w", err)
	}

	// Converter para JID de grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se são realmente grupos
	if communityID.Server != types.GroupServer || groupID.Server != types.GroupServer {
		return fmt.Errorf("JIDs devem ser de grupos/comunidades")
	}

	// Desvincular grupo da comunidade usando UnlinkGroup
	err = client.UnlinkGroup(communityID, groupID)
	if err != nil {
		return fmt.Errorf("falha ao desvincular grupo da comunidade: %w", err)
	}

	logger.Debug("Grupo desvinculado da comunidade",
		"user_id", userID,
		"community_jid", communityJID,
		"group_jid", groupJID)

	return nil
}
