// internal/services/whatsapp/event.go
package session

import (
	"context"
	"fmt"
	"time"
	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// RegisterEventHandler registra um handler para eventos
func (sm *SessionManager) RegisterEventHandler(eventType string, handler EventHandler) {
	sm.clientsMutex.Lock()
	defer sm.clientsMutex.Unlock()

	if _, exists := sm.eventHandlers[eventType]; !exists {
		sm.eventHandlers[eventType] = make([]EventHandler, 0)
	}

	sm.eventHandlers[eventType] = append(sm.eventHandlers[eventType], handler)
}

// ProcessEvent processes WhatsApp events
func (sm *SessionManager) ProcessEvent(userID string, evt interface{}) {
	// Determine event type
	var eventType string
	var eventData map[string]interface{}

	// Atualizar última atividade do cliente
	sm.clientsMutex.Lock()
	if client, exists := sm.clients[userID]; exists {
		client.LastActive = time.Now()
	}
	sm.clientsMutex.Unlock()

	switch typedEvt := evt.(type) {
	case *events.Message:
		eventType = "message"
		eventData = sm.extractMessageData(typedEvt)

	case *events.Connected:
		eventType = "connection.update"
		eventData = map[string]interface{}{
			"status":    "connected",
			"timestamp": time.Now().Unix(),
		}

		// Quando o dispositivo é autenticado, salvar o mapeamento
		client, exists := sm.GetSession(userID)
		if exists && client.WAClient.Store.ID != nil {
			deviceJID := client.WAClient.Store.ID.String()
			if err := sm.sqlStore.SaveUserDeviceMapping(userID, deviceJID); err != nil {
				logger.Error("Falha ao salvar mapeamento", "user_id", userID, "device_jid", deviceJID, "error", err)
			} else {
				logger.Info("Mapeamento salvo", "user_id", userID, "device_jid", deviceJID)
			}

			// Atualizar estado conectado
			sm.clientsMutex.Lock()
			client.Connected = true
			sm.clientsMutex.Unlock()
		}

	case *events.Disconnected:
		eventType = "connection.update"
		eventData = map[string]interface{}{
			"status":    "disconnected",
			"timestamp": time.Now().Unix(),
		}

		// Atualizar estado de conexão
		sm.clientsMutex.Lock()
		if client, exists := sm.clients[userID]; exists {
			client.Connected = false
		}
		sm.clientsMutex.Unlock()

		logger.Info("Cliente desconectado", "user_id", userID, "reason")

	case *events.LoggedOut:
		eventType = "connection.update"
		eventData = map[string]interface{}{
			"status":    "logged_out",
			"reason":    fmt.Sprintf("%d", typedEvt.Reason),
			"timestamp": time.Now().Unix(),
		}

		logger.Info("Evento de logout recebido", "user_id", userID, "reason", typedEvt.Reason)

		// Executar limpeza completa da sessão
		if err := sm.handleDeviceLogout(userID); err != nil {
			logger.Error("Falha ao limpar sessão após logout", "user_id", userID, "error", err)
		}

	case *events.GroupInfo:
		// Handle GroupInfo events with granular action detection
		eventType, eventData = sm.handleGroupInfoEvent(userID, typedEvt)

	case *events.JoinedGroup:
		eventType = "group.members.updated"
		memberCount := len(typedEvt.GroupInfo.Participants)
		participants := sm.extractParticipantDetails(typedEvt.GroupInfo.Participants)
		eventData = map[string]interface{}{
			"group_jid":    typedEvt.JID.String(),
			"action":       "joined",
			"user_jid":     userID,
			"timestamp":    time.Now().Unix(),
			"member_count": memberCount,
			"participants": participants,
		}

	case *events.QR:
		eventType = "qr"
		eventData = map[string]interface{}{
			"codes":     typedEvt.Codes,
			"timestamp": time.Now().Unix(),
		}

	default:
		eventType = "unknown"
		eventData = map[string]interface{}{
			"event_type": fmt.Sprintf("%T", evt),
			"timestamp":  time.Now().Unix(),
		}
		logger.Debug("Evento desconhecido recebido",
			"user_id", userID,
			"event_type", fmt.Sprintf("%T", evt))
	}

	// Add common metadata
	eventData["user_id"] = userID
	eventData["event_type"] = eventType

	// Publish to RabbitMQ if publisher is available
	if sm.eventPublisher != nil && eventType != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := sm.eventPublisher.PublishEvent(ctx, userID, eventType, eventData); err != nil {
			logger.Error("Failed to publish event to RabbitMQ",
				"user_id", userID,
				"event_type", eventType,
				"error", err)
		} else {
			logger.Debug("Event published to RabbitMQ",
				"user_id", userID,
				"event_type", eventType)
		}
	}

	// Chamar handlers
	sm.clientsMutex.RLock()
	handlers, exists := sm.eventHandlers[eventType]
	sm.clientsMutex.RUnlock()

	if exists {
		for _, handler := range handlers {
			if err := handler(userID, evt); err != nil {
				sm.logger.Errorf("Erro ao processar evento %s: %v", eventType, err)
			}
		}
	}
}

func (sm *SessionManager) extractParticipantDetails(participants []types.GroupParticipant) []map[string]interface{} {
	result := make([]map[string]interface{}, len(participants))
	for i, p := range participants {
		participant := map[string]interface{}{
			"jid":            p.JID.String(),
			"is_admin":       p.IsAdmin,
			"is_super_admin": p.IsSuperAdmin,
		}

		// Add display name if available
		if p.DisplayName != "" {
			participant["display_name"] = p.DisplayName
		}

		result[i] = participant
	}
	return result
}

// extractMessageData extracts all available data from a message event
func (sm *SessionManager) extractMessageData(msg *events.Message) map[string]interface{} {
	data := map[string]interface{}{
		"message_id":   msg.Info.ID,
		"from":         msg.Info.Sender.String(),
		"chat":         msg.Info.Chat.String(),
		"timestamp":    msg.Info.Timestamp.Unix(),
		"from_me":      msg.Info.IsFromMe,
		"is_group":     msg.Info.IsGroup,
		"push_name":    msg.Info.PushName,
		"message_type": fmt.Sprintf("%T", msg.Message),
	}

	// Add broadcast list owner if available
	if !msg.Info.BroadcastListOwner.IsEmpty() {
		data["broadcast_owner"] = msg.Info.BroadcastListOwner.String()
	}

	// Extract basic message content
	if msg.Message != nil {
		// Add raw message content for debugging
		data["raw_message"] = fmt.Sprintf("%+v", msg.Message)

		// Try to extract text content from common message types
		if msg.Message.GetConversation() != "" {
			data["text"] = msg.Message.GetConversation()
		}
		if msg.Message.GetExtendedTextMessage() != nil {
			data["text"] = msg.Message.GetExtendedTextMessage().GetText()
		}
		if msg.Message.GetImageMessage() != nil {
			data["caption"] = msg.Message.GetImageMessage().GetCaption()
			data["mimetype"] = msg.Message.GetImageMessage().GetMimetype()
		}
		if msg.Message.GetVideoMessage() != nil {
			data["caption"] = msg.Message.GetVideoMessage().GetCaption()
			data["mimetype"] = msg.Message.GetVideoMessage().GetMimetype()
		}
		if msg.Message.GetAudioMessage() != nil {
			data["mimetype"] = msg.Message.GetAudioMessage().GetMimetype()
			data["ptt"] = msg.Message.GetAudioMessage().GetPTT()
		}
		if msg.Message.GetDocumentMessage() != nil {
			data["title"] = msg.Message.GetDocumentMessage().GetTitle()
			data["mimetype"] = msg.Message.GetDocumentMessage().GetMimetype()
		}
	}

	return data
}

// handleGroupInfoEvent processes GroupInfo events and detects specific actions
func (sm *SessionManager) handleGroupInfoEvent(userID string, group *events.GroupInfo) (string, map[string]interface{}) {
	eventType := "group.updated"
	data := map[string]interface{}{
		"group_jid": group.JID.String(),
		"notify":    group.Notify,
		"timestamp": group.Timestamp.Unix(),
	}

	// Add sender information if available
	if group.Sender != nil {
		data["sender"] = group.Sender.String()
	}
	if group.SenderPN != nil {
		data["sender_phone"] = group.SenderPN.String()
	}

	// Get current group info to extract member count
	if client, exists := sm.GetSession(userID); exists && client.WAClient != nil {
		if groupInfo, err := client.WAClient.GetGroupInfo(group.JID); err == nil {
			data["member_count"] = len(groupInfo.Participants)
			data["group_name"] = groupInfo.Name
			data["group_topic"] = groupInfo.Topic
		} else {
			logger.Warn("Failed to get group info for event",
				"group_jid", group.JID.String(),
				"user_id", userID,
				"error", err)
		}
	}

	// Check for specific actions based on what changed
	if len(group.Join) > 0 {
		// Members added
		eventType = "group.members.added"
		data["action"] = "members_added"
		data["added_members"] = sm.convertJIDsToStrings(group.Join)
		data["join_reason"] = group.JoinReason
	} else if len(group.Leave) > 0 {
		// Members removed/left
		eventType = "group.members.removed"
		data["action"] = "members_removed"
		data["removed_members"] = sm.convertJIDsToStrings(group.Leave)
	} else if len(group.Promote) > 0 {
		// Members promoted to admin
		eventType = "group.members.promoted"
		data["action"] = "members_promoted"
		data["promoted_members"] = sm.convertJIDsToStrings(group.Promote)
	} else if len(group.Demote) > 0 {
		// Members demoted from admin
		eventType = "group.members.demoted"
		data["action"] = "members_demoted"
		data["demoted_members"] = sm.convertJIDsToStrings(group.Demote)
	}

	// Check for name changes
	if group.Name != nil {
		eventType = "group.name.changed"
		data["action"] = "name_changed"
		data["new_name"] = group.Name.Name
		if !group.Name.NameSetBy.IsEmpty() {
			data["changed_by"] = group.Name.NameSetBy.String()
		}
		if !group.Name.NameSetByPN.IsEmpty() {
			data["changed_by_phone"] = group.Name.NameSetByPN.String()
		}
		data["name_set_at"] = group.Name.NameSetAt.Unix()
	}

	// Check for topic/description changes
	if group.Topic != nil {
		eventType = "group.topic.changed"
		data["action"] = "topic_changed"
		data["new_topic"] = group.Topic.Topic
		if !group.Topic.TopicSetBy.IsEmpty() {
			data["changed_by"] = group.Topic.TopicSetBy.String()
		}
		if !group.Topic.TopicSetByPN.IsEmpty() {
			data["changed_by_phone"] = group.Topic.TopicSetByPN.String()
		}
		data["topic_set_at"] = group.Topic.TopicSetAt.Unix()
		data["topic_id"] = group.Topic.TopicID
	}

	// Check for announce setting changes
	if group.Announce != nil {
		eventType = "group.announce.changed"
		data["action"] = "announce_changed"
		data["new_announce"] = group.Announce.IsAnnounce
		if group.Announce.AnnounceVersionID != "" {
			data["announce_version_id"] = group.Announce.AnnounceVersionID
		}
	}

	// Check for locked setting changes
	if group.Locked != nil {
		eventType = "group.locked.changed"
		data["action"] = "locked_changed"
		data["new_locked"] = group.Locked.IsLocked
	}

	// Check for ephemeral (disappearing messages) changes
	if group.Ephemeral != nil {
		eventType = "group.ephemeral.changed"
		data["action"] = "ephemeral_changed"
		data["is_ephemeral"] = group.Ephemeral.IsEphemeral
		data["disappearing_timer"] = group.Ephemeral.DisappearingTimer
	}

	// Check for membership approval mode changes
	if group.MembershipApprovalMode != nil {
		eventType = "group.membership.approval.changed"
		data["action"] = "membership_approval_changed"
		data["join_approval_required"] = group.MembershipApprovalMode.IsJoinApprovalRequired
	}

	// Check for group deletion
	if group.Delete != nil {
		eventType = "group.deleted"
		data["action"] = "group_deleted"
		if group.Delete.Deleted {
			data["deleted"] = true
		}
		if group.Delete.DeleteReason != "" {
			data["delete_reason"] = group.Delete.DeleteReason
		}
	}

	// Check for group link changes
	if group.Link != nil {
		eventType = "group.link.enabled"
		data["action"] = "link_enabled"
		data["link_type"] = string(group.Link.Type)
		data["link_group"] = group.Link.Group
	}

	// Check for group unlink changes
	if group.Unlink != nil {
		eventType = "group.link.disabled"
		data["action"] = "link_disabled"
		data["link_type"] = string(group.Unlink.Type)
		data["link_group"] = group.Unlink.Group
	}

	// Check for invite link changes
	if group.NewInviteLink != nil {
		eventType = "group.invite.link.changed"
		data["action"] = "invite_link_changed"
		data["new_invite_link"] = *group.NewInviteLink
	}

	// Add version tracking information
	if group.PrevParticipantVersionID != "" {
		data["prev_participant_version"] = group.PrevParticipantVersionID
	}
	if group.ParticipantVersionID != "" {
		data["participant_version"] = group.ParticipantVersionID
	}

	return eventType, data
}

// convertJIDsToStrings converts a slice of JIDs to a slice of strings
func (sm *SessionManager) convertJIDsToStrings(jids []types.JID) []string {
	result := make([]string, len(jids))
	for i, jid := range jids {
		result[i] = jid.String()
	}
	return result
}
