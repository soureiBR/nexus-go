// internal/services/whatsapp/messaging/group.go
package messaging

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"yourproject/internal/services/whatsapp/session"
	"yourproject/pkg/logger"
)

// GroupService provides group management functionality
type GroupService struct {
	groupManager session.GroupManager
}

// NewGroupService creates a new group service
func NewGroupService(groupManager session.GroupManager) *GroupService {
	return &GroupService{
		groupManager: groupManager,
	}
}

// CreateGroup cria um novo grupoexperimento
func (gs *GroupService) CreateGroup(userID, name string, participants []string) (interface{}, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return nil, fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Criar grupo
	req := whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: jids,
	}
	group, err := client.GetWAClient().CreateGroup(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar grupo: %w", err)
	}

	// Log
	logger.Debug("Grupo criado", "user_id", userID, "group_name", name, "group_jid", group.JID.String())

	// Construir e retornar informações do grupo
	groupParticipants := make([]GroupParticipant, len(group.Participants))
	for i, p := range group.Participants {
		groupParticipants[i] = GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		}
	}

	return &GroupInfo{
		JID:          group.JID.String(),
		Name:         name,
		Created:      time.Now(),
		Creator:      userID,
		Participants: groupParticipants,
	}, nil
}

// GetGroupInfo obtém informações de um grupo
func (gs *GroupService) GetGroupInfo(userID, groupJID string) (interface{}, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if jid.Server != types.GroupServer {
		return nil, fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Obter informações do grupo
	groupInfo, err := client.GetWAClient().GetGroupInfo(jid)
	if err != nil {
		return nil, fmt.Errorf("falha ao obter informações do grupo: %w", err)
	}

	// Converter participantes
	participants := make([]GroupParticipant, len(groupInfo.Participants))
	for i, p := range groupInfo.Participants {
		participants[i] = GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		}
	}

	return &GroupInfo{
		JID:          jid.String(),
		Name:         groupInfo.Name,
		Topic:        groupInfo.Topic,
		Creator:      groupInfo.OwnerJID.String(),
		Participants: participants,
	}, nil
}

// GetJoinedGroups obtém lista de grupos em que o usuário é membro
func (gs *GroupService) GetJoinedGroups(userID string) (interface{}, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Obter lista de grupos
	groups, err := client.GetWAClient().GetJoinedGroups()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter lista de grupos: %w", err)
	}

	// Converter para formato de resposta
	result := make([]GroupInfo, len(groups))
	for i, group := range groups {
		// Converter participantes
		participants := make([]GroupParticipant, len(group.Participants))
		for j, p := range group.Participants {
			participants[j] = GroupParticipant{
				JID:          p.JID.String(),
				IsAdmin:      p.IsAdmin,
				IsSuperAdmin: p.IsSuperAdmin,
			}
		}

		result[i] = GroupInfo{
			JID:          group.JID.String(),
			Name:         group.Name,
			Topic:        group.Topic,
			Creator:      group.OwnerJID.String(),
			Participants: participants,
		}
	}

	// Log
	logger.Debug("Lista de grupos obtida",
		"user_id", userID,
		"groups_count", len(groups))

	return result, nil
}

// AddGroupParticipants adiciona participantes a um grupo
func (gs *GroupService) AddGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Adicionar participantes
	_, err = client.GetWAClient().UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		return fmt.Errorf("falha ao adicionar participantes ao grupo: %w", err)
	}

	// Log
	logger.Debug("Participantes adicionados ao grupo",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// RemoveGroupParticipants remove participantes de um grupo
func (gs *GroupService) RemoveGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Remover participantes
	_, err = client.GetWAClient().UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		return fmt.Errorf("falha ao remover participantes do grupo: %w", err)
	}

	// Log
	logger.Debug("Participantes removidos do grupo",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// PromoteGroupParticipants promove participantes a admins
func (gs *GroupService) PromoteGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Promover participantes
	_, err = client.GetWAClient().UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangePromote)
	if err != nil {
		return fmt.Errorf("falha ao promover participantes do grupo: %w", err)
	}

	// Log
	logger.Debug("Participantes promovidos a admins",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// DemoteGroupParticipants rebaixa admins para participantes comuns
func (gs *GroupService) DemoteGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Converter JIDs dos participantes
	var jids []types.JID
	for _, p := range participants {
		jid, err := types.ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Rebaixar participantes
	_, err = client.GetWAClient().UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangeDemote)
	if err != nil {
		return fmt.Errorf("falha ao rebaixar participantes do grupo: %w", err)
	}

	// Log
	logger.Debug("Admins rebaixados a participantes comuns",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// UpdateGroupName atualiza o nome do grupo
func (gs *GroupService) UpdateGroupName(userID, groupJID, newName string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Atualizar nome do grupo
	err = client.GetWAClient().SetGroupName(groupID, newName)
	if err != nil {
		return fmt.Errorf("falha ao atualizar nome do grupo: %w", err)
	}

	// Log
	logger.Debug("Nome do grupo atualizado",
		"user_id", userID,
		"group_jid", groupJID,
		"new_name", newName)

	return nil
}

// UpdateGroupTopic atualiza o tópico/descrição do grupo
func (gs *GroupService) UpdateGroupTopic(userID, groupJID, newTopic string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Atualizar tópico do grupo
	err = client.GetWAClient().SetGroupDescription(groupID, newTopic)
	if err != nil {
		return fmt.Errorf("falha ao atualizar tópico do grupo: %w", err)
	}

	// Log
	logger.Debug("Tópico do grupo atualizado",
		"user_id", userID,
		"group_jid", groupJID)

	return nil
}

// LeaveGroup sai de um grupo
func (gs *GroupService) LeaveGroup(userID, groupJID string) error {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Sair do grupo
	err = client.GetWAClient().LeaveGroup(groupID)
	if err != nil {
		return fmt.Errorf("falha ao sair do grupo: %w", err)
	}

	// Log
	logger.Debug("Saiu do grupo",
		"user_id", userID,
		"group_jid", groupJID)

	return nil
}

// GetGroupInviteLink gera e retorna um link de convite para o grupo
func (gs *GroupService) GetGroupInviteLink(userID, groupJID string) (string, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return "", fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return "", fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return "", fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Obter link de convite
	link, err := client.GetWAClient().GetGroupInviteLink(groupID, false)
	if err != nil {
		return "", fmt.Errorf("falha ao obter link de convite: %w", err)
	}

	// Log
	logger.Debug("Link de convite obtido",
		"user_id", userID,
		"group_jid", groupJID)

	return link, nil
}

// RevokeGroupInviteLink revoga o link atual e gera um novo
func (gs *GroupService) RevokeGroupInviteLink(userID, groupJID string) (string, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return "", fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Converter para JID do grupo
	groupID, err := types.ParseJID(groupJID)
	if err != nil {
		return "", fmt.Errorf("JID de grupo inválido: %w", err)
	}

	// Verificar se é realmente um grupo
	if groupID.Server != types.GroupServer {
		return "", fmt.Errorf("JID não é um grupo: %s", groupJID)
	}

	// Revogar link atual e obter novo
	link, err := client.GetWAClient().GetGroupInviteLink(groupID, true)
	if err != nil {
		return "", fmt.Errorf("falha ao revogar link de convite: %w", err)
	}

	// Log
	logger.Debug("Link de convite revogado e novo link obtido",
		"user_id", userID,
		"group_jid", groupJID)

	return link, nil
}

// JoinGroupWithLink entra em um grupo usando um link de convite
func (gs *GroupService) JoinGroupWithLink(userID, link string) (interface{}, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

	// Atualizar atividade
	client.UpdateActivity()

	// Normalizar link de convite
	if !strings.HasPrefix(link, "https://chat.whatsapp.com/") {
		// Verificar se é apenas o código
		if !strings.Contains(link, "/") {
			link = "https://chat.whatsapp.com/" + link
		} else {
			return nil, fmt.Errorf("link de convite inválido: %s", link)
		}
	}

	// Extrair código do link
	code := strings.TrimPrefix(link, "https://chat.whatsapp.com/")

	// Entrar no grupo
	groupID, err := client.GetWAClient().JoinGroupWithLink(code)
	if err != nil {
		return nil, fmt.Errorf("falha ao entrar no grupo: %w", err)
	}

	// Log
	logger.Debug("Entrou no grupo via link",
		"user_id", userID,
		"group_jid", groupID.String())

	// Obter informações do grupo
	return gs.GetGroupInfo(userID, groupID.String())
}
