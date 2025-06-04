// internal/services/whatsapp/messaging/types.go
package messaging

import (
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ClientWrapper wraps whatsmeow client with additional metadata for messaging operations
type ClientWrapper struct {
	WAClient   *whatsmeow.Client
	LastActive time.Time
	Connected  bool
	UserID     string
}

// IsActive checks if the client wrapper is active and connected
func (cw *ClientWrapper) IsActive() bool {
	return cw.Connected && cw.WAClient != nil && cw.WAClient.IsConnected()
}

// UpdateActivity updates the last active timestamp
func (cw *ClientWrapper) UpdateActivity() {
	cw.LastActive = time.Now()
}

// ButtonData representa um botão para mensagens interativas
type ButtonData struct {
	ID   string `json:"id" binding:"required"`
	Text string `json:"text" binding:"required"`
}

// SectionRow representa uma linha em uma seção de lista
type SectionRow struct {
	ID          string `json:"id" binding:"required"`
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
}

// Section representa uma seção em uma mensagem de lista
type Section struct {
	Title string       `json:"title"`
	Rows  []SectionRow `json:"rows" binding:"required,min=1"`
}

// GroupParticipant representa um participante do grupo com todas as informações disponíveis
type GroupParticipant struct {
	JID          string                     `json:"jid"`
	PhoneNumber  string                     `json:"phone_number,omitempty"`
	LID          string                     `json:"lid,omitempty"`
	IsAdmin      bool                       `json:"is_admin"`
	IsSuperAdmin bool                       `json:"is_super_admin"`
	DisplayName  string                     `json:"display_name,omitempty"`
	Error        int                        `json:"error,omitempty"`
	AddRequest   *GroupParticipantAddRequest `json:"add_request,omitempty"`
}

// GroupParticipantAddRequest representa uma solicitação para adicionar um participante
type GroupParticipantAddRequest struct {
	Code       string    `json:"code"`
	Expiration time.Time `json:"expiration"`
}

// GroupSettings representa as configurações completas do grupo
type GroupSettings struct {
	// Configurações básicas
	IsLocked               bool   `json:"is_locked"`                 // Apenas admins podem editar info do grupo
	IsAnnounce             bool   `json:"is_announce"`               // Apenas admins podem enviar mensagens
	AnnounceVersionID      string `json:"announce_version_id,omitempty"`
	
	// Mensagens temporárias
	IsEphemeral            bool   `json:"is_ephemeral"`              // Mensagens temporárias ativadas
	DisappearingTimer      uint32 `json:"disappearing_timer"`        // Tempo de expiração das mensagens (segundos)
	
	// Modo incógnito
	IsIncognito            bool   `json:"is_incognito"`
	
	// Configurações de adição de membros
	MemberAddMode          string `json:"member_add_mode"`           // Modo de adição de membros
	IsJoinApprovalRequired bool   `json:"is_join_approval_required"` // Requer aprovação para entrar
	
	// Configurações de grupo pai/comunidade
	IsParent               bool   `json:"is_parent"`
	IsDefaultSubGroup      bool   `json:"is_default_sub_group"`
	LinkedParentJID        string `json:"linked_parent_jid,omitempty"`
	
	// Modo de endereçamento
	AddressingMode         string `json:"addressing_mode,omitempty"`
}

// GroupMetadata representa metadados adicionais do grupo
type GroupMetadata struct {
	GroupCreated           time.Time `json:"group_created"`
	CreatorCountryCode     string    `json:"creator_country_code,omitempty"`
	ParticipantVersionID   string    `json:"participant_version_id,omitempty"`
}

// GroupNameInfo representa informações sobre o nome do grupo
type GroupNameInfo struct {
	Name        string     `json:"name"`
	NameSetAt   *time.Time `json:"name_set_at,omitempty"`
	NameSetBy   string     `json:"name_set_by,omitempty"`
	NameSetByPN string     `json:"name_set_by_pn,omitempty"`
}

// GroupTopicInfo representa informações sobre o tópico do grupo
type GroupTopicInfo struct {
	Topic        string     `json:"topic"`
	TopicID      string     `json:"topic_id,omitempty"`
	TopicSetAt   *time.Time `json:"topic_set_at,omitempty"`
	TopicSetBy   string     `json:"topic_set_by,omitempty"`
	TopicSetByPN string     `json:"topic_set_by_pn,omitempty"`
	TopicDeleted bool       `json:"topic_deleted"`
}

// GroupInviteInfo representa informações sobre convite do grupo
type GroupInviteInfo struct {
	Code        string     `json:"code,omitempty"`
	Expiration  *time.Time `json:"expiration,omitempty"`
	RevokeCount int        `json:"revoke_count"`
}

// GroupInfo representa informações completas de um grupo
type GroupInfo struct {
	// Informações básicas
	JID         string    `json:"jid"`
	OwnerJID    string    `json:"owner_jid,omitempty"`
	OwnerPN     string    `json:"owner_pn,omitempty"`
	
	// Informações de nome
	NameInfo    GroupNameInfo  `json:"name_info"`
	
	// Informações de tópico
	TopicInfo   GroupTopicInfo `json:"topic_info"`
	
	// Metadados
	Metadata    GroupMetadata  `json:"metadata"`
	
	// Participantes
	Participants     []GroupParticipant `json:"participants"`
	ParticipantCount int                `json:"participant_count"`
	
	// Configurações do grupo
	Settings         GroupSettings      `json:"settings"`
	
	// Informações de convite
	InviteInfo       *GroupInviteInfo   `json:"invite_info,omitempty"`
	
	// Status e permissões do usuário atual
	UserPermissions  UserGroupPermissions `json:"user_permissions"`
	
	// Campos de compatibilidade (legacy)
	Name     string `json:"name"`      // Alias para NameInfo.Name
	Topic    string `json:"topic"`     // Alias para TopicInfo.Topic
	Created  *time.Time `json:"created"` // Alias para Metadata.GroupCreated
	Creator  string `json:"creator"`   // Alias para OwnerJID
}

// UserGroupPermissions representa as permissões do usuário atual no grupo
type UserGroupPermissions struct {
	IsParticipant    bool `json:"is_participant"`
	IsAdmin          bool `json:"is_admin"`
	IsSuperAdmin     bool `json:"is_super_admin"`
	CanSendMessages  bool `json:"can_send_messages"`
	CanEditInfo      bool `json:"can_edit_info"`
	CanAddMembers    bool `json:"can_add_members"`
	CanRemoveMembers bool `json:"can_remove_members"`
	CanPromoteMembers bool `json:"can_promote_members"`
}

// GroupCreateResponse representa a resposta da criação de um grupo
type GroupCreateResponse struct {
	GroupInfo         *GroupInfo        `json:"group_info"`
	ParticipantErrors map[string]string `json:"participant_errors,omitempty"`
	SuccessfulAdds    []string          `json:"successful_adds"`
	FailedAdds        []string          `json:"failed_adds"`
}

// GroupUpdateResult representa o resultado de uma atualização de participantes
type GroupUpdateResult struct {
	SuccessfulUpdates []string          `json:"successful_updates"`
	FailedUpdates     map[string]string `json:"failed_updates"`
}

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

// ToGroupInfo converte types.GroupInfo para nossa estrutura expandida
func ToGroupInfo(waGroupInfo *types.GroupInfo, currentUserJID string) *GroupInfo {
	if waGroupInfo == nil {
		return nil
	}

	// Converter participantes e determinar permissões do usuário atual
	participants := make([]GroupParticipant, len(waGroupInfo.Participants))
	userPermissions := UserGroupPermissions{
		IsParticipant: false,
		IsAdmin:       false,
		IsSuperAdmin:  false,
	}

	for i, p := range waGroupInfo.Participants {
		participant := GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
			DisplayName:  p.DisplayName,
			Error:        p.Error,
		}

		// Adicionar informações opcionais se disponíveis
		if !p.PhoneNumber.IsEmpty() {
			participant.PhoneNumber = p.PhoneNumber.String()
		}
		if !p.LID.IsEmpty() {
			participant.LID = p.LID.String()
		}

		// Converter AddRequest se existir
		if p.AddRequest != nil {
			participant.AddRequest = &GroupParticipantAddRequest{
				Code:       p.AddRequest.Code,
				Expiration: p.AddRequest.Expiration,
			}
		}

		// Verificar se é o usuário atual
		if p.JID.String() == currentUserJID {
			userPermissions.IsParticipant = true
			userPermissions.IsAdmin = p.IsAdmin
			userPermissions.IsSuperAdmin = p.IsSuperAdmin
		}

		participants[i] = participant
	}

	// Configurar permissões baseadas no status do usuário
	userPermissions.CanSendMessages = !waGroupInfo.IsAnnounce || userPermissions.IsAdmin
	userPermissions.CanEditInfo = !waGroupInfo.IsLocked || userPermissions.IsAdmin
	userPermissions.CanAddMembers = userPermissions.IsAdmin || (waGroupInfo.MemberAddMode == types.GroupMemberAddModeAllMember)
	userPermissions.CanRemoveMembers = userPermissions.IsAdmin
	userPermissions.CanPromoteMembers = userPermissions.IsSuperAdmin

	// Construir informações de nome
	nameInfo := GroupNameInfo{
		Name: waGroupInfo.Name,
	}
	if !waGroupInfo.NameSetAt.IsZero() {
		nameInfo.NameSetAt = &waGroupInfo.NameSetAt
	}
	if !waGroupInfo.NameSetBy.IsEmpty() {
		nameInfo.NameSetBy = waGroupInfo.NameSetBy.String()
	}
	if !waGroupInfo.NameSetByPN.IsEmpty() {
		nameInfo.NameSetByPN = waGroupInfo.NameSetByPN.String()
	}

	// Construir informações de tópico
	topicInfo := GroupTopicInfo{
		Topic:        waGroupInfo.Topic,
		TopicID:      waGroupInfo.TopicID,
		TopicDeleted: waGroupInfo.TopicDeleted,
	}
	if !waGroupInfo.TopicSetAt.IsZero() {
		topicInfo.TopicSetAt = &waGroupInfo.TopicSetAt
	}
	if !waGroupInfo.TopicSetBy.IsEmpty() {
		topicInfo.TopicSetBy = waGroupInfo.TopicSetBy.String()
	}
	if !waGroupInfo.TopicSetByPN.IsEmpty() {
		topicInfo.TopicSetByPN = waGroupInfo.TopicSetByPN.String()
	}

	// Construir metadados
	metadata := GroupMetadata{
		GroupCreated:         waGroupInfo.GroupCreated,
		CreatorCountryCode:   waGroupInfo.CreatorCountryCode,
		ParticipantVersionID: waGroupInfo.ParticipantVersionID,
	}

	// Construir configurações
	settings := GroupSettings{
		IsLocked:               waGroupInfo.IsLocked,
		IsAnnounce:             waGroupInfo.IsAnnounce,
		AnnounceVersionID:      waGroupInfo.AnnounceVersionID,
		IsEphemeral:            waGroupInfo.IsEphemeral,
		DisappearingTimer:      waGroupInfo.DisappearingTimer,
		IsIncognito:            waGroupInfo.IsIncognito,
		MemberAddMode:          string(waGroupInfo.MemberAddMode),
		IsJoinApprovalRequired: waGroupInfo.IsJoinApprovalRequired,
		IsParent:               waGroupInfo.IsParent,
		IsDefaultSubGroup:      waGroupInfo.IsDefaultSubGroup,
		AddressingMode:         string(waGroupInfo.AddressingMode),
	}

	if !waGroupInfo.LinkedParentJID.IsEmpty() {
		settings.LinkedParentJID = waGroupInfo.LinkedParentJID.String()
	}

	// Construir informações principais
	groupInfo := &GroupInfo{
		JID:              waGroupInfo.JID.String(),
		NameInfo:         nameInfo,
		TopicInfo:        topicInfo,
		Metadata:         metadata,
		Participants:     participants,
		ParticipantCount: len(participants),
		Settings:         settings,
		UserPermissions:  userPermissions,
		
		// Campos de compatibilidade
		Name:    waGroupInfo.Name,
		Topic:   waGroupInfo.Topic,
		Creator: waGroupInfo.OwnerJID.String(),
	}

	// Adicionar campos opcionais
	if !waGroupInfo.OwnerJID.IsEmpty() {
		groupInfo.OwnerJID = waGroupInfo.OwnerJID.String()
	}
	if !waGroupInfo.OwnerPN.IsEmpty() {
		groupInfo.OwnerPN = waGroupInfo.OwnerPN.String()
	}

	// Timestamp de criação para compatibilidade
	if !waGroupInfo.GroupCreated.IsZero() {
		groupInfo.Created = &waGroupInfo.GroupCreated
	}

	return groupInfo
}

// ToGroupParticipant converte types.GroupParticipant para nossa estrutura
func ToGroupParticipant(waParticipant types.GroupParticipant) GroupParticipant {
	participant := GroupParticipant{
		JID:          waParticipant.JID.String(),
		IsAdmin:      waParticipant.IsAdmin,
		IsSuperAdmin: waParticipant.IsSuperAdmin,
		DisplayName:  waParticipant.DisplayName,
		Error:        waParticipant.Error,
	}

	// Adicionar campos opcionais
	if !waParticipant.PhoneNumber.IsEmpty() {
		participant.PhoneNumber = waParticipant.PhoneNumber.String()
	}
	if !waParticipant.LID.IsEmpty() {
		participant.LID = waParticipant.LID.String()
	}

	// Converter AddRequest se existir
	if waParticipant.AddRequest != nil {
		participant.AddRequest = &GroupParticipantAddRequest{
			Code:       waParticipant.AddRequest.Code,
			Expiration: waParticipant.AddRequest.Expiration,
		}
	}

	return participant
}