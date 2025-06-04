// internal/services/whatsapp/messaging/group.go
package messaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"io"
	"net/http"
	"strings"

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

func (gs *GroupService) CreateGroup(userID, name string, participants []string) (interface{}, error) {
	logger.Info("Iniciando criação de grupo", "user_id", userID, "group_name", name, "participants", participants)
	
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		logger.Error("Sessão não encontrada para usuário", "user_id", userID)
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		logger.Error("Sessão não conectada para usuário", "user_id", userID)
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

	logger.Debug("Sessão encontrada e conectada", "user_id", userID)
	client.UpdateActivity()

	// Validar e processar participantes
	var jids []types.JID
	var participantErrors = make(map[string]string)
	var successfulAdds []string
	var failedAdds []string

	for _, p := range participants {
		logger.Debug("Processando participante", "user_id", userID, "participant", p)
		
		validatedJID, err := gs.validateAndProcessParticipantNumber(userID, p)
		if err != nil {
			logger.Error("Falha ao validar participante", "user_id", userID, "participant", p, "error", err)
			participantErrors[p] = err.Error()
			failedAdds = append(failedAdds, p)
			continue
		}
		
		jid, err := types.ParseJID(validatedJID)
		if err != nil {
			logger.Error("JID inválido para participante", "user_id", userID, "participant", p, "error", err)
			participantErrors[p] = fmt.Sprintf("JID inválido: %v", err)
			failedAdds = append(failedAdds, p)
			continue
		}
		
		jids = append(jids, jid)
		successfulAdds = append(successfulAdds, p)
		logger.Debug("Participante validado", "user_id", userID, "participant", p, "validated_jid", validatedJID)
	}

	if len(jids) == 0 {
		return nil, fmt.Errorf("nenhum participante válido fornecido")
	}

	logger.Debug("Iniciando criação do grupo", "user_id", userID, "group_name", name, "valid_participants", len(jids))

	// Criar grupo
	req := whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: jids,
	}
	
	logger.Info("Tentando criar grupo no WhatsApp", "user_id", userID, "group_name", name, "participants_count", len(jids))
	
	groupResult, err := client.GetWAClient().CreateGroup(req)
	if err != nil {
		logger.Error("Falha ao criar grupo no WhatsApp", "user_id", userID, "group_name", name, "error", err)
		return nil, fmt.Errorf("falha ao criar grupo: %w", err)
	}

	logger.Info("Grupo criado com sucesso no WhatsApp", "user_id", userID, "group_name", name, "group_jid", groupResult.JID.String())

	// Obter informações completas do grupo
	fullGroupInfo, err := client.GetWAClient().GetGroupInfo(groupResult.JID)
	if err != nil {
		logger.Warn("Falha ao obter informações completas do grupo, usando dados básicos", "error", err)
		// Criar uma estrutura básica com os dados disponíveis do resultado
		fullGroupInfo = &types.GroupInfo{
			JID: groupResult.JID,
			GroupName: types.GroupName{
				Name: name,
			},
		}
	}

	// Obter JID do usuário atual para contexto
	currentUserJID := userID
	if !strings.Contains(userID, "@") {
		// Se userID não é um JID completo, tentar construir
		if client.GetWAClient().Store != nil && client.GetWAClient().Store.ID != nil {
			currentUserJID = client.GetWAClient().Store.ID.String()
		}
	}

	// Converter para nossa estrutura expandida
	groupInfo := ToGroupInfo(fullGroupInfo, currentUserJID)

	// Se não houve erros, retornar apenas o GroupInfo para compatibilidade
	if len(participantErrors) == 0 {
		return groupInfo, nil
	}

	// Se houve erros, retornar resposta completa
	response := &GroupCreateResponse{
		GroupInfo:         groupInfo,
		ParticipantErrors: participantErrors,
		SuccessfulAdds:    successfulAdds,
		FailedAdds:        failedAdds,
	}

	return response, nil
}

// GetGroupInfo - versão corrigida
func (gs *GroupService) GetGroupInfo(userID, groupJID string) (interface{}, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

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

	// Obter JID do usuário atual
	currentUserJID := userID
	if !strings.Contains(userID, "@") {
		if client.GetWAClient().Store != nil && client.GetWAClient().Store.ID != nil {
			currentUserJID = client.GetWAClient().Store.ID.String()
		}
	}

	// Converter para nossa estrutura expandida
	expandedGroupInfo := ToGroupInfo(groupInfo, currentUserJID)

	// Tentar obter link de convite (pode falhar se não for admin)
	if expandedGroupInfo.UserPermissions.CanEditInfo {
		inviteLink, err := client.GetWAClient().GetGroupInviteLink(jid, false)
		if err == nil && inviteLink != "" {
			// Extrair código do link
			if strings.HasPrefix(inviteLink, "https://chat.whatsapp.com/") {
				code := strings.TrimPrefix(inviteLink, "https://chat.whatsapp.com/")
				expandedGroupInfo.InviteInfo = &GroupInviteInfo{
					Code: code,
				}
			}
		}
	}

	logger.Debug("Informações do grupo obtidas",
		"user_id", userID,
		"group_jid", groupJID,
		"group_name", expandedGroupInfo.Name,
		"participants_count", expandedGroupInfo.ParticipantCount,
		"is_participant", expandedGroupInfo.UserPermissions.IsParticipant,
		"can_send_messages", expandedGroupInfo.UserPermissions.CanSendMessages,
		"can_edit_info", expandedGroupInfo.UserPermissions.CanEditInfo,
		"can_add_members", expandedGroupInfo.UserPermissions.CanAddMembers)

	return expandedGroupInfo, nil
}


// Método auxiliar para debug das informações do grupo (versão corrigida)
func (gs *GroupService) debugGroupInfo(groupInfo *types.GroupInfo, context string) {
	logger.Debug("Group info debug", 
		"context", context,
		"jid", groupInfo.JID.String(),
		"name", groupInfo.Name,
		"topic", groupInfo.Topic,
		"owner_jid", func() string {
			if !groupInfo.OwnerJID.IsEmpty() {
				return groupInfo.OwnerJID.String()
			}
			return "empty"
		}(),
		"group_created", groupInfo.GroupCreated.String(),
		"participants_count", len(groupInfo.Participants),
		"is_announce", groupInfo.IsAnnounce,
		"is_locked", groupInfo.IsLocked,
		"is_ephemeral", groupInfo.IsEphemeral,
		"member_add_mode", string(groupInfo.MemberAddMode))
}

// GetJoinedGroups obtém lista completa de grupos em que o usuário é membro
func (gs *GroupService) GetJoinedGroups(userID string) (interface{}, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não conectada: %s", userID)
	}

	client.UpdateActivity()

	// Obter lista de grupos
	groups, err := client.GetWAClient().GetJoinedGroups()
	if err != nil {
		return nil, fmt.Errorf("falha ao obter lista de grupos: %w", err)
	}

	// Obter JID do usuário atual
	currentUserJID := userID
	if !strings.Contains(userID, "@") {
		if client.GetWAClient().Store != nil && client.GetWAClient().Store.ID != nil {
			currentUserJID = client.GetWAClient().Store.ID.String()
		}
	}

	// Converter para formato expandido
	result := make([]*GroupInfo, len(groups))
	for i, group := range groups {
		result[i] = ToGroupInfo(group, currentUserJID)
	}

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

	// Converter JIDs dos participantes com validação
	var jids []types.JID
	for _, p := range participants {
		// Validate and process each participant phone number
		validatedJID, err := gs.validateAndProcessParticipantNumber(userID, p)
		if err != nil {
			return fmt.Errorf("falha ao validar participante %s: %w", p, err)
		}

		// Parse the validated JID string
		jid, err := types.ParseJID(validatedJID)
		if err != nil {
			return fmt.Errorf("JID inválido para participante %s: %w", p, err)
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

	// Converter JIDs dos participantes com validação
	var jids []types.JID
	for _, p := range participants {
		// Validate and process each participant phone number
		validatedJID, err := gs.validateAndProcessParticipantNumber(userID, p)
		if err != nil {
			return fmt.Errorf("falha ao validar participante %s: %w", p, err)
		}

		// Parse the validated JID string
		jid, err := types.ParseJID(validatedJID)
		if err != nil {
			return fmt.Errorf("JID inválido para participante %s: %w", p, err)
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

	// Converter JIDs dos participantes com validação
	var jids []types.JID
	for _, p := range participants {
		// Validate and process each participant phone number
		validatedJID, err := gs.validateAndProcessParticipantNumber(userID, p)
		if err != nil {
			return fmt.Errorf("falha ao validar participante %s: %w", p, err)
		}

		// Parse the validated JID string
		jid, err := types.ParseJID(validatedJID)
		if err != nil {
			return fmt.Errorf("JID inválido para participante %s: %w", p, err)
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

	// Converter JIDs dos participantes com validação
	var jids []types.JID
	for _, p := range participants {
		// Validate and process each participant phone number
		validatedJID, err := gs.validateAndProcessParticipantNumber(userID, p)
		if err != nil {
			return fmt.Errorf("falha ao validar participante %s: %w", p, err)
		}

		// Parse the validated JID string
		jid, err := types.ParseJID(validatedJID)
		if err != nil {
			return fmt.Errorf("JID inválido para participante %s: %w", p, err)
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

// UpdateGroupPictureFromURL updates the group picture from a URL
func (gs *GroupService) UpdateGroupPictureFromURL(userID, groupJID, imageURL string) (string, error) {
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

	// Download the image from URL
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("falha ao baixar imagem da URL: %w", err)
	}
	defer resp.Body.Close()
	
	logger.Debug("Download da imagem iniciado",
		"user_id", userID,
		"group_jid", groupJID,
		"image_url", imageURL)
	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("falha ao baixar imagem: HTTP %d - %s", resp.StatusCode, resp.Status)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		logger.Warn("Tipo de conteúdo suspeito para imagem",
			"content_type", contentType,
			"image_url", imageURL)
	}

	// Read the image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("falha ao ler dados da imagem: %w", err)
	}

	// Validate image data
	if len(imageData) == 0 {
		return "", fmt.Errorf("dados da imagem estão vazios")
	}

	logger.Debug("Baixando imagem para atualizar foto do grupo",
		"user_id", userID,
		"group_jid", groupJID,
		"image_url", imageURL,
		"size_bytes", len(imageData),
		"content_type", resp.Header.Get("Content-Type"))
	
	// Convert image to JPEG format (WhatsApp only supports JPEG for group pictures)
	jpegData, err := convertToJPEG(imageData, 85) // Quality 85 provides good balance between quality and file size
	if err != nil {
		return "", fmt.Errorf("falha ao converter imagem para JPEG: %w", err)
	}
	
	// Validate JPEG magic bytes (WhatsApp requires strict JPEG validation)
	if len(jpegData) < 2 || jpegData[0] != 0xFF || jpegData[1] != 0xD8 {
		logger.Error("Dados JPEG inválidos - magic bytes incorretos",
			"user_id", userID,
			"group_jid", groupJID,
			"data_length", len(jpegData),
			"first_bytes", fmt.Sprintf("%X", jpegData[:min(10, len(jpegData))]))
		return "", fmt.Errorf("dados JPEG inválidos: magic bytes incorretos")
	}
	
	logger.Debug("Imagem convertida para JPEG",
		"user_id", userID,
		"group_jid", groupJID,
		"original_size", len(imageData),
		"jpeg_size", len(jpegData),
		"jpeg_magic_bytes", fmt.Sprintf("%X %X", jpegData[0], jpegData[1]))
	
	// Set the group photo
	pictureID, err := client.GetWAClient().SetGroupPhoto(groupID, jpegData)
	if err != nil {
		logger.Error("Falha ao definir foto do grupo no WhatsApp",
			"error", err,
			"user_id", userID,
			"group_jid", groupJID)
		return "", fmt.Errorf("falha ao atualizar foto do grupo: %w", err)
	}

	// Log success
	logger.Debug("Foto do grupo atualizada com sucesso",
		"user_id", userID,
		"group_jid", groupJID,
		"picture_id", pictureID)

	return pictureID, nil
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

// convertToJPEG converts image data to JPEG format with WhatsApp-compatible settings
func convertToJPEG(imageData []byte, quality int) ([]byte, error) {
	// First, validate input image data
	if len(imageData) == 0 {
		return nil, fmt.Errorf("dados da imagem estão vazios")
	}

	// Decodifica a imagem (detecta automaticamente o formato)
	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("falha ao decodificar imagem (formato: %s): %w", format, err)
	}

	// Get image dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	logger.Debug("Processando imagem",
		"format", format,
		"width", width,
		"height", height,
		"original_size", len(imageData))

	// WhatsApp has specific requirements for group photos
	// Maximum dimensions are typically 640x640 pixels
	maxDimension := 640
	needsResize := width > maxDimension || height > maxDimension

	if needsResize {
		// Calculate new dimensions maintaining aspect ratio
		var newWidth, newHeight int
		if width > height {
			newWidth = maxDimension
			newHeight = (height * maxDimension) / width
		} else {
			newHeight = maxDimension
			newWidth = (width * maxDimension) / height
		}

		// Create a new image with the resized dimensions
		resizedImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		
		// Simple nearest-neighbor scaling
		for y := 0; y < newHeight; y++ {
			for x := 0; x < newWidth; x++ {
				srcX := (x * width) / newWidth
				srcY := (y * height) / newHeight
				resizedImg.Set(x, y, img.At(srcX, srcY))
			}
		}
		
		img = resizedImg
		logger.Debug("Imagem redimensionada",
			"original_width", width,
			"original_height", height,
			"new_width", newWidth,
			"new_height", newHeight)
	}

	// Always re-encode to ensure WhatsApp compatibility
	// Even if it's already JPEG, we want to ensure proper encoding
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	if err != nil {
		return nil, fmt.Errorf("falha ao codificar imagem como JPEG: %w", err)
	}

	processedData := buf.Bytes()
	
	// Validate JPEG magic bytes immediately after encoding
	if len(processedData) < 2 || processedData[0] != 0xFF || processedData[1] != 0xD8 {
		return nil, fmt.Errorf("dados JPEG gerados são inválidos: magic bytes incorretos (%X %X)", 
			processedData[0], processedData[1])
	}
	
	// Validate the processed image size (WhatsApp typically has a file size limit)
	maxFileSize := 1024 * 1024 // 1MB limit
	if len(processedData) > maxFileSize {
		// Try with lower quality
		lowerQuality := quality - 20
		if lowerQuality < 50 {
			lowerQuality = 50
		}
		
		buf.Reset()
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: lowerQuality})
		if err != nil {
			return nil, fmt.Errorf("falha ao recodificar imagem com qualidade menor: %w", err)
		}
		processedData = buf.Bytes()
		
		// Validate magic bytes again after recompression
		if len(processedData) < 2 || processedData[0] != 0xFF || processedData[1] != 0xD8 {
			return nil, fmt.Errorf("dados JPEG recomprimidos são inválidos: magic bytes incorretos")
		}
		
		logger.Debug("Imagem recomprimida para reduzir tamanho",
			"original_quality", quality,
			"new_quality", lowerQuality,
			"original_size", len(imageData),
			"new_size", len(processedData))
	}

	// Final validation
	logger.Debug("JPEG produzido com sucesso",
		"size", len(processedData),
		"magic_bytes", fmt.Sprintf("%X %X", processedData[0], processedData[1]))

	return processedData, nil
}

// validateAndProcessParticipantNumber validates and processes participant phone numbers
func (gs *GroupService) validateAndProcessParticipantNumber(userID, phoneNumber string) (string, error) {
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
	if gs.isPhoneNumber(phoneNumber) {
		return gs.validateAndProcessPhoneNumber(userID, phoneNumber)
	}

	// Default: treat as phone number
	return gs.validateAndProcessPhoneNumber(userID, phoneNumber)
}

// isPhoneNumber checks if the input looks like a phone number
func (gs *GroupService) isPhoneNumber(input string) bool {
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
func (gs *GroupService) validateAndProcessPhoneNumber(userID, phoneNumber string) (string, error) {
	// Clean the phone number
	cleaned := gs.cleanPhoneNumber(phoneNumber)

	// Handle Brazilian numbers with the 9-digit rule
	processed := gs.processBrazilianNumber(cleaned)

	// Check if the number exists on WhatsApp and get the correct JID
	jid, exists, err := gs.checkNumberExistsOnWhatsAppAndGetJID(userID, processed)
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
				alternative := gs.removeNinthDigitFromBrazilian(processed)
				if alternative != processed {
					alternatives = append(alternatives, alternative)
				}
			} else if len(processed) == 12 {
				// 12 digits: try adding the 9
				alternative := gs.addNinthDigitToBrazilian(processed)
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
				altJID, altExists, altErr := gs.checkNumberExistsOnWhatsAppAndGetJID(userID, alt)
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
func (gs *GroupService) cleanPhoneNumber(phone string) string {
	// Remove all non-digit characters except +
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	cleaned = strings.ReplaceAll(cleaned, ".", "")

	// Remove + if present
	cleaned = strings.TrimPrefix(cleaned, "+")

	return cleaned
}

// processBrazilianNumber handles Brazilian number formatting
func (gs *GroupService) processBrazilianNumber(number string) string {
	// If it doesn't start with country code, check if it's a Brazilian local number
	if !strings.HasPrefix(number, "55") && len(number) >= 10 && len(number) <= 11 {
		// Check if it looks like a Brazilian local number by area code
		if gs.isBrazilianAreaCode(number) {
			// Add Brazilian country code
			if len(number) == 11 || len(number) == 10 {
				number = "55" + number
			}
		}
	}

	return number
}

// removeNinthDigitFromBrazilian removes the 9th digit from Brazilian mobile numbers
func (gs *GroupService) removeNinthDigitFromBrazilian(number string) string {
	// Brazilian format: 55 + 2-digit area code + 9-digit mobile number
	// The 9th digit is the first digit of the mobile number part
	if strings.HasPrefix(number, "55") && len(number) == 13 {
		areaCode := number[2:4]
		mobileNumber := number[4:]

		// Check if it starts with 9 (mobile number indicator)
		if strings.HasPrefix(mobileNumber, "9") && len(mobileNumber) == 9 {
			// Remove the first 9
			return "55" + areaCode + mobileNumber[1:]
		}
	}

	return number
}

// addNinthDigitToBrazilian adds the 9th digit to Brazilian mobile numbers
func (gs *GroupService) addNinthDigitToBrazilian(number string) string {
	// Brazilian format: 55 + 2-digit area code + 8-digit mobile number (old format)
	if strings.HasPrefix(number, "55") && len(number) == 12 {
		areaCode := number[2:4]
		mobileNumber := number[4:]

		// Check if it's a mobile number (starts with 9, 8, 7, or 6) and doesn't already have 9
		if len(mobileNumber) == 8 && (mobileNumber[0] >= '6' && mobileNumber[0] <= '9') {
			if !strings.HasPrefix(mobileNumber, "9") {
				return "55" + areaCode + "9" + mobileNumber
			}
		}
	}

	return number
}

// checkNumberExistsOnWhatsAppAndGetJID verifies if a number exists on WhatsApp and returns the correct JID
func (gs *GroupService) checkNumberExistsOnWhatsAppAndGetJID(userID, number string) (string, bool, error) {
	client, exists := gs.groupManager.GetSession(userID)
	if !exists {
		return "", false, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Verificar se o cliente está conectado
	if !client.IsConnected() {
		return "", false, fmt.Errorf("cliente não está conectado")
	}

	// Format with + for WhatsApp API
	numberWithPlus := "+" + number

	// Check status
	responses, err := client.GetWAClient().IsOnWhatsApp([]string{numberWithPlus})
	if err != nil {
		return "", false, fmt.Errorf("falha ao verificar número: %w", err)
	}

	if len(responses) == 0 {
		return "", false, nil
	}

	response := responses[0]
	
	// Log the response for debugging
	logger.Debug("IsOnWhatsApp response", "query", response.Query, "jid", response.JID.String(), "is_in", response.IsIn)

	if response.IsIn {
		return response.JID.String(), true, nil
	}

	return "", false, nil
}

// checkNumberExistsOnWhatsApp verifies if a number exists on WhatsApp (legacy function for compatibility)
func (gs *GroupService) checkNumberExistsOnWhatsApp(userID, number string) (bool, error) {
	_, exists, err := gs.checkNumberExistsOnWhatsAppAndGetJID(userID, number)
	return exists, err
}

// isBrazilianAreaCode checks if a number starts with a valid Brazilian area code
func (gs *GroupService) isBrazilianAreaCode(number string) bool {
	if len(number) < 2 {
		return false
	}

	// Valid Brazilian area codes (11-99, but not all combinations)
	areaCode := number[:2]

	// List of valid Brazilian area codes
	validAreaCodes := map[string]bool{
		"11": true, "12": true, "13": true, "14": true, "15": true, "16": true, "17": true, "18": true, "19": true,
		"21": true, "22": true, "24": true,
		"27": true, "28": true,
		"31": true, "32": true, "33": true, "34": true, "35": true, "37": true, "38": true,
		"41": true, "42": true, "43": true, "44": true, "45": true, "46": true,
		"47": true, "48": true, "49": true,
		"51": true, "53": true, "54": true, "55": true,
		"61": true, "62": true, "63": true, "64": true, "65": true, "66": true, "67": true, "68": true, "69": true,
		"71": true, "73": true, "74": true, "75": true, "77": true, "79": true,
		"81": true, "82": true, "83": true, "84": true, "85": true, "86": true, "87": true, "88": true, "89": true,
		"91": true, "92": true, "93": true, "94": true, "95": true, "96": true, "97": true, "98": true,
	}

	// For numbers starting with valid area codes, do additional validation
	if validAreaCodes[areaCode] {
		// Additional check: if the number is 11 digits and starts with a typical US pattern (like 1555...),
		// it's probably not Brazilian
		if len(number) == 11 && strings.HasPrefix(number, "1555") {
			return false
		}

		// Additional check: if the number is 10-11 digits starting with "15" and followed by "55",
		// it's probably a US number (1-555-...)
		if areaCode == "15" && len(number) >= 4 && number[2:4] == "55" {
			return false
		}

		return true
	}

	return false
}

// Group permission methods

// SetGroupLocked altera o status de bloqueio do grupo (somente admins podem modificar informações do grupo)
func (gs *GroupService) SetGroupLocked(userID, groupJID string, locked bool) error {
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

	// Alterar status de bloqueio
	err = client.GetWAClient().SetGroupLocked(groupID, locked)
	if err != nil {
		return fmt.Errorf("falha ao alterar status de bloqueio do grupo: %w", err)
	}

	// Log
	logger.Debug("Status de bloqueio do grupo alterado",
		"user_id", userID,
		"group_jid", groupJID,
		"locked", locked)

	return nil
}

// SetGroupAnnounce altera o modo de anúncio do grupo (somente admins podem enviar mensagens)
func (gs *GroupService) SetGroupAnnounce(userID, groupJID string, announce bool) error {
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

	// Alterar modo de anúncio
	err = client.GetWAClient().SetGroupAnnounce(groupID, announce)
	if err != nil {
		return fmt.Errorf("falha ao alterar modo de anúncio do grupo: %w", err)
	}

	// Log
	logger.Debug("Modo de anúncio do grupo alterado",
		"user_id", userID,
		"group_jid", groupJID,
		"announce", announce)

	return nil
}

// SetGroupJoinApprovalMode altera o modo de aprovação de entrada do grupo
func (gs *GroupService) SetGroupJoinApprovalMode(userID, groupJID, mode string) error {
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

	// Converter modo para booleano
	requireApproval := mode == "on"

	// Alterar modo de aprovação de entrada
	err = client.GetWAClient().SetGroupJoinApprovalMode(groupID, requireApproval)
	if err != nil {
		return fmt.Errorf("falha ao alterar modo de aprovação de entrada do grupo: %w", err)
	}

	// Log
	logger.Debug("Modo de aprovação de entrada do grupo alterado",
		"user_id", userID,
		"group_jid", groupJID,
		"mode", mode,
		"require_approval", requireApproval)

	return nil
}

// SetGroupMemberAddMode altera o modo de adição de membros do grupo
func (gs *GroupService) SetGroupMemberAddMode(userID, groupJID, mode string) error {
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

	// Converter modo para o tipo adequado
	var memberAddMode types.GroupMemberAddMode
	switch mode {
	case "admin_add":
		memberAddMode = types.GroupMemberAddModeAdmin
	case "all_member_add":
		memberAddMode = types.GroupMemberAddModeAllMember
	default:
		return fmt.Errorf("modo de adição de membros inválido: %s (deve ser 'admin_add' ou 'all_member_add')", mode)
	}

	// Alterar modo de adição de membros
	err = client.GetWAClient().SetGroupMemberAddMode(groupID, memberAddMode)
	if err != nil {
		return fmt.Errorf("falha ao alterar modo de adição de membros do grupo: %w", err)
	}

	// Log
	logger.Debug("Modo de adição de membros do grupo alterado",
		"user_id", userID,
		"group_jid", groupJID,
		"mode", mode,
		"member_add_mode", memberAddMode)

	return nil
}
