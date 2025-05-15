// internal/services/whatsapp/group.go
package whatsapp

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"yourproject/pkg/logger"
)

// GroupParticipant representa um participante do grupo
type GroupParticipant struct {
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"is_admin"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

// GroupInfo representa informações de um grupo
type GroupInfo struct {
	JID          string             `json:"jid"`
	Name         string             `json:"name"`
	Topic        string             `json:"topic,omitempty"`
	Created      time.Time          `json:"created"`
	Creator      string             `json:"creator"`
	Participants []GroupParticipant `json:"participants"`
}

// CreateGroup cria um novo grupo
func (sm *SessionManager) CreateGroup(userID, name string, participants []string) (*GroupInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
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

	// Criar grupo
	req := whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: jids,
	}
	group, err := client.WAClient.CreateGroup(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

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
func (sm *SessionManager) GetGroupInfo(userID, groupJID string) (*GroupInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	groupInfo, err := client.WAClient.GetGroupInfo(jid)
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

	// Atualizar última atividade
	client.LastActive = time.Now()

	return &GroupInfo{
		JID:          jid.String(),
		Name:         groupInfo.Name,
		Topic:        groupInfo.Topic,
		Creator:      groupInfo.OwnerJID.String(),
		Participants: participants,
	}, nil
}

// AddGroupParticipants adiciona participantes a um grupo
func (sm *SessionManager) AddGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
		jid, err := ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Adicionar participantes - usando a ação ADD (adicionar)
	_, err = client.WAClient.UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		return fmt.Errorf("falha ao adicionar participantes ao grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Participantes adicionados ao grupo",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// RemoveGroupParticipants remove participantes de um grupo
func (sm *SessionManager) RemoveGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
		jid, err := ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Remover participantes - usando a ação REMOVE (remover)
	_, err = client.WAClient.UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		return fmt.Errorf("falha ao remover participantes do grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Participantes removidos do grupo",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// PromoteGroupParticipants promove participantes a admins
func (sm *SessionManager) PromoteGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
		jid, err := ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Promover participantes
	_, err = client.WAClient.UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangePromote)
	if err != nil {
		return fmt.Errorf("falha ao promover participantes do grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Participantes promovidos a admins",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// DemoteGroupParticipants rebaixa admins para participantes comuns
func (sm *SessionManager) DemoteGroupParticipants(userID, groupJID string, participants []string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
		jid, err := ParseJID(p)
		if err != nil {
			return fmt.Errorf("JID inválido para participante: %s - %w", p, err)
		}
		jids = append(jids, jid)
	}

	// Rebaixar participantes
	_, err = client.WAClient.UpdateGroupParticipants(groupID, jids, whatsmeow.ParticipantChangeDemote)
	if err != nil {
		return fmt.Errorf("falha ao rebaixar participantes do grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Admins rebaixados a participantes comuns",
		"user_id", userID,
		"group_jid", groupJID,
		"participants_count", len(participants))

	return nil
}

// UpdateGroupName atualiza o nome do grupo
func (sm *SessionManager) UpdateGroupName(userID, groupJID, newName string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	err = client.WAClient.SetGroupName(groupID, newName)
	if err != nil {
		return fmt.Errorf("falha ao atualizar nome do grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Nome do grupo atualizado",
		"user_id", userID,
		"group_jid", groupJID,
		"new_name", newName)

	return nil
}

// UpdateGroupTopic atualiza o tópico/descrição do grupo
func (sm *SessionManager) UpdateGroupTopic(userID, groupJID, newTopic string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	err = client.WAClient.SetGroupDescription(groupID, newTopic)
	if err != nil {
		return fmt.Errorf("falha ao atualizar tópico do grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Tópico do grupo atualizado",
		"user_id", userID,
		"group_jid", groupJID)

	return nil
}

// LeaveGroup sai de um grupo
func (sm *SessionManager) LeaveGroup(userID, groupJID string) error {
	client, exists := sm.GetSession(userID)
	if !exists {
		return fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	err = client.WAClient.LeaveGroup(groupID)
	if err != nil {
		return fmt.Errorf("falha ao sair do grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Saiu do grupo",
		"user_id", userID,
		"group_jid", groupJID)

	return nil
}

// GetJoinedGroups obtém lista de grupos em que o usuário é membro
func (sm *SessionManager) GetJoinedGroups(userID string) ([]GroupInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Obter lista de grupos
	groups, err := client.WAClient.GetJoinedGroups()
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

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Lista de grupos obtida",
		"user_id", userID,
		"groups_count", len(groups))

	return result, nil
}

// SendGroupInviteLink gera e retorna um link de convite para o grupo
func (sm *SessionManager) GetGroupInviteLink(userID, groupJID string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	link, err := client.WAClient.GetGroupInviteLink(groupID, false)
	if err != nil {
		return "", fmt.Errorf("falha ao obter link de convite: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Link de convite obtido",
		"user_id", userID,
		"group_jid", groupJID)

	return link, nil
}

// RevokeGroupInviteLink revoga o link atual e gera um novo
func (sm *SessionManager) RevokeGroupInviteLink(userID, groupJID string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	link, err := client.WAClient.GetGroupInviteLink(groupID, true)
	if err != nil {
		return "", fmt.Errorf("falha ao revogar link de convite: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Link de convite revogado e novo link obtido",
		"user_id", userID,
		"group_jid", groupJID)

	return link, nil
}

// JoinGroupWithLink entra em um grupo usando um link de convite
func (sm *SessionManager) JoinGroupWithLink(userID, link string) (*GroupInfo, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

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
	groupID, err := client.WAClient.JoinGroupWithLink(code)
	if err != nil {
		return nil, fmt.Errorf("falha ao entrar no grupo: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Entrou no grupo via link",
		"user_id", userID,
		"group_jid", groupID.String())

	// Obter informações do grupo
	return sm.GetGroupInfo(userID, groupID.String())
}
