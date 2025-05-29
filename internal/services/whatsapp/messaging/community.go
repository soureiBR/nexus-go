// internal/services/whatsapp/messaging/community.go
package messaging

import (
	"fmt"
	"strings"

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

	// Converter JIDs dos participantes com validação
	var jids []types.JID
	for _, p := range participants {
		// Validate and process each participant phone number
		validatedJID, err := cs.validateAndProcessParticipantNumber(userID, p)
		if err != nil {
			return nil, fmt.Errorf("falha ao validar participante %s: %w", p, err)
		}

		// Parse the validated JID string
		jid, err := types.ParseJID(validatedJID)
		if err != nil {
			return nil, fmt.Errorf("JID inválido para participante %s: %w", p, err)
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

// validateAndProcessParticipantNumber validates and processes participant phone numbers
func (cs *CommunityService) validateAndProcessParticipantNumber(userID, phoneNumber string) (string, error) {
	// Clean the input
	phoneNumber = strings.TrimSpace(phoneNumber)

	if phoneNumber == "" {
		return "", fmt.Errorf("número do participante não pode estar vazio")
	}

	// Check if it's already a valid JID format
	if strings.Contains(phoneNumber, "@") {
		_, err := types.ParseJID(phoneNumber)
		if err != nil {
			return "", fmt.Errorf("JID inválido: %w", err)
		}
		return phoneNumber, nil
	}

	// If it's a phone number, validate and process it
	if cs.isPhoneNumber(phoneNumber) {
		return cs.validateAndProcessPhoneNumber(userID, phoneNumber)
	}

	// Default: treat as phone number
	return cs.validateAndProcessPhoneNumber(userID, phoneNumber)
}

// isPhoneNumber checks if the input looks like a phone number
func (cs *CommunityService) isPhoneNumber(input string) bool {
	// Remove common phone number characters
	cleaned := strings.ReplaceAll(input, "+", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	// Check if all remaining characters are digits
	for _, r := range cleaned {
		if r < '0' || r > '9' {
			return false
		}
	}

	// Must have at least 10 digits for a valid phone number
	return len(cleaned) >= 10
}

// validateAndProcessPhoneNumber processes and validates phone numbers for participants
func (cs *CommunityService) validateAndProcessPhoneNumber(userID, phoneNumber string) (string, error) {
	// Clean the phone number
	cleaned := cs.cleanPhoneNumber(phoneNumber)

	// Handle Brazilian numbers with the 9-digit rule
	processed := cs.processBrazilianNumber(cleaned)

	// Check if the number exists on WhatsApp and get the correct JID
	jid, exists, err := cs.checkNumberExistsOnWhatsAppAndGetJID(userID, processed)
	if err != nil {
		logger.Debug("Erro ao verificar número no WhatsApp", "number", processed, "error", err)
		// Continue even if verification fails, but log the error
	}

	if !exists {
		// Try with the alternative format for Brazilian numbers
		if strings.HasPrefix(processed, "55") && len(processed) >= 12 {
			var alternatives []string

			if len(processed) == 13 {
				// 13 digits: try removing the 9
				alternative := cs.removeNinthDigitFromBrazilian(processed)
				if alternative != processed {
					alternatives = append(alternatives, alternative)
				}
			} else if len(processed) == 12 {
				// 12 digits: try adding the 9
				alternative := cs.addNinthDigitToBrazilian(processed)
				if alternative != processed {
					alternatives = append(alternatives, alternative)
				}
			} else if len(processed) == 11 {
				// 11 digits: This could be a landline or mobile without country code added incorrectly
				// Try to re-process as 9-digit number by adding country code
				withoutCountryCode := processed[2:] // Remove "55"
				if len(withoutCountryCode) == 9 {
					// Add 55 back and try both with and without 9
					alternatives = append(alternatives, "55"+withoutCountryCode) // As-is

					// Try adding 9 if it doesn't start with 9
					if !strings.HasPrefix(withoutCountryCode, "9") {
						alternatives = append(alternatives, "55"+withoutCountryCode[:2]+"9"+withoutCountryCode[2:])
					}

					// Try removing 9 if it starts with 9
					if strings.HasPrefix(withoutCountryCode, "9") && len(withoutCountryCode) == 9 {
						alternatives = append(alternatives, "55"+withoutCountryCode[1:])
					}
				}
			}

			// Test each alternative
			for _, alt := range alternatives {
				altJID, altExists, altErr := cs.checkNumberExistsOnWhatsAppAndGetJID(userID, alt)
				if altErr == nil && altExists {
					return altJID, nil
				}
			}
		}
	}

	if exists && jid != "" {
		return jid, nil
	}

	// If no valid JID found, fallback to manual construction
	fallbackJID := processed + "@s.whatsapp.net"
	logger.Debug("Usando JID de fallback", "number", processed, "jid", fallbackJID)
	return fallbackJID, nil
}

// cleanPhoneNumber removes formatting characters from phone number
func (cs *CommunityService) cleanPhoneNumber(phone string) string {
	// Remove common formatting characters
	cleaned := strings.ReplaceAll(phone, "+", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	cleaned = strings.ReplaceAll(cleaned, ".", "")

	return cleaned
}

// processBrazilianNumber handles Brazilian number formatting
func (cs *CommunityService) processBrazilianNumber(number string) string {
	// If it doesn't start with country code, check if it's a Brazilian local number
	if !strings.HasPrefix(number, "55") && len(number) >= 10 && len(number) <= 11 {
		// Check if it looks like a Brazilian local number by area code
		if cs.isBrazilianAreaCode(number) {
			// Add Brazilian country code
			if len(number) == 11 || len(number) == 10 {
				number = "55" + number
			}
		}
	}

	return number
}

// removeNinthDigitFromBrazilian removes the 9th digit from Brazilian mobile numbers
func (cs *CommunityService) removeNinthDigitFromBrazilian(number string) string {
	// Must start with 55 and have at least 13 digits
	if !strings.HasPrefix(number, "55") || len(number) < 13 {
		return number
	}

	// Get the part after country code (55)
	withoutCountryCode := number[2:]

	// Must have at least 11 digits and the third digit must be 9
	if len(withoutCountryCode) >= 11 && withoutCountryCode[2] == '9' {
		// Check if it's a valid area code
		areaCode := withoutCountryCode[:2]
		if cs.isBrazilianAreaCode("55" + areaCode + "1234567890") {
			// Remove the 9 from the third position
			return "55" + areaCode + withoutCountryCode[3:]
		}
	}

	return number
}

// addNinthDigitToBrazilian adds the 9th digit to Brazilian mobile numbers
func (cs *CommunityService) addNinthDigitToBrazilian(number string) string {
	// Must start with 55 and have at least 12 digits
	if !strings.HasPrefix(number, "55") || len(number) < 12 {
		return number
	}

	// Get the part after country code (55)
	withoutCountryCode := number[2:]

	// Must have exactly 10 digits and the third digit must not be 9
	if len(withoutCountryCode) == 10 && withoutCountryCode[2] != '9' {
		// Check if it's a valid area code
		areaCode := withoutCountryCode[:2]
		if cs.isBrazilianAreaCode("55" + areaCode + "1234567890") {
			// Add 9 after the area code
			return "55" + areaCode + "9" + withoutCountryCode[2:]
		}
	}

	return number
}

// checkNumberExistsOnWhatsAppAndGetJID verifies if a number exists on WhatsApp and returns the correct JID
func (cs *CommunityService) checkNumberExistsOnWhatsAppAndGetJID(userID, number string) (string, bool, error) {
	client, err := cs.getClient(userID)
	if err != nil {
		return "", false, err
	}

	// Use IsOnWhatsApp to check existence
	jids := []string{number}
	responses, err := client.IsOnWhatsApp(jids)
	if err != nil {
		return "", false, fmt.Errorf("falha ao verificar número no WhatsApp: %w", err)
	}

	if len(responses) == 0 {
		return "", false, fmt.Errorf("nenhuma resposta recebida do WhatsApp")
	}

	response := responses[0]
	logger.Debug("IsOnWhatsApp response", "query", response.Query, "jid", response.JID.String(), "is_in", response.IsIn)

	if response.IsIn {
		return response.JID.String(), true, nil
	}

	return "", false, nil
}

// checkNumberExistsOnWhatsApp verifies if a number exists on WhatsApp (legacy function for compatibility)
func (cs *CommunityService) checkNumberExistsOnWhatsApp(userID, number string) (bool, error) {
	_, exists, err := cs.checkNumberExistsOnWhatsAppAndGetJID(userID, number)
	return exists, err
}

// isBrazilianAreaCode checks if a number starts with a valid Brazilian area code
func (cs *CommunityService) isBrazilianAreaCode(number string) bool {
	// Brazilian area codes (2 digits after country code 55)
	validAreaCodes := []string{
		"11", "12", "13", "14", "15", "16", "17", "18", "19", // São Paulo
		"21", "22", "24",                                     // Rio de Janeiro
		"27", "28",                                           // Espírito Santo
		"31", "32", "33", "34", "35", "37", "38",             // Minas Gerais
		"41", "42", "43", "44", "45", "46",                   // Paraná
		"47", "48", "49",                                     // Santa Catarina
		"51", "53", "54", "55",                               // Rio Grande do Sul
		"61",                                                 // Distrito Federal
		"62", "64",                                           // Goiás
		"63",                                                 // Tocantins
		"65", "66",                                           // Mato Grosso
		"67",                                                 // Mato Grosso do Sul
		"68",                                                 // Acre
		"69",                                                 // Rondônia
		"71", "73", "74", "75", "77",                         // Bahia
		"79",                                                 // Sergipe
		"81", "87",                                           // Pernambuco
		"82",                                                 // Alagoas
		"83",                                                 // Paraíba
		"84",                                                 // Rio Grande do Norte
		"85", "88",                                           // Ceará
		"86", "89",                                           // Piauí
		"91", "93", "94",                                     // Pará
		"92", "97",                                           // Amazonas
		"95",                                                 // Roraima
		"96",                                                 // Amapá
		"98", "99",                                           // Maranhão
	}

	// Remove country code if present
	if strings.HasPrefix(number, "55") && len(number) > 2 {
		number = number[2:]
	}

	// Check if starts with valid area code
	if len(number) >= 2 {
		areaCode := number[:2]
		for _, valid := range validAreaCodes {
			if areaCode == valid {
				return true
			}
		}
	}

	return false
}
