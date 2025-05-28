// internal/services/whatsapp/messaging/message.go
package messaging

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"yourproject/internal/services/whatsapp/session"
	"yourproject/internal/services/whatsapp/worker"
	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// MessageService provides messaging functionality and implements worker.MessageServiceInterface
type MessageService struct {
	sessionManager session.Manager
}

// NewMessageService creates a new message service
func NewMessageService(sessionManager session.Manager) *MessageService {
	return &MessageService{
		sessionManager: sessionManager,
	}
}

// ParseJID converte uma string para um JID do WhatsApp
func ParseJID(jid string) (types.JID, error) {
	if !strings.Contains(jid, "@") {
		// Adicionar sufixo se não estiver presente
		jid = jid + "@s.whatsapp.net"
	}

	recipient, err := types.ParseJID(jid)
	if err != nil {
		return types.JID{}, fmt.Errorf("JID inválido: %w", err)
	}

	return recipient, nil
}

// SendText envia uma mensagem de texto
func (ms *MessageService) SendText(userID, to, message string) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Enviar mensagem
	msg, err := client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
		Conversation: proto.String(message),
	})

	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Mensagem de texto enviada", "user_id", userID, "to", to, "message_id", msg.ID)

	return msg.ID, nil
}

// SendMedia envia uma mensagem de mídia
func (ms *MessageService) SendMedia(userID, to, mediaURL, mediaType, caption string) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Extrair o nome do arquivo da URL
	fileName := mediaURL
	if idx := strings.LastIndex(mediaURL, "/"); idx != -1 {
		fileName = mediaURL[idx+1:]
	}

	// Se o tipo de mídia não foi especificado, tente determinar pela extensão do arquivo
	if mediaType == "" {
		switch {
		case strings.HasSuffix(fileName, ".jpg"), strings.HasSuffix(fileName, ".jpeg"), strings.HasSuffix(fileName, ".png"):
			mediaType = "image"
		case strings.HasSuffix(fileName, ".mp4"), strings.HasSuffix(fileName, ".avi"), strings.HasSuffix(fileName, ".mov"):
			mediaType = "video"
		case strings.HasSuffix(fileName, ".mp3"), strings.HasSuffix(fileName, ".wav"), strings.HasSuffix(fileName, ".ogg"):
			mediaType = "audio"
		default:
			mediaType = "document"
		}
	}

	// Determinar tipo de mídia para WhatsApp
	var uploadType whatsmeow.MediaType
	switch mediaType {
	case "image", "img":
		uploadType = whatsmeow.MediaImage
	case "video", "vid":
		uploadType = whatsmeow.MediaVideo
	case "audio", "voice":
		uploadType = whatsmeow.MediaAudio
	case "document", "doc", "file":
		uploadType = whatsmeow.MediaDocument
	default:
		return "", fmt.Errorf("tipo de mídia não suportado: %s", mediaType)
	}

	// Fazer download da mídia da URL
	logger.Debug("Baixando mídia da URL", "url", mediaURL)
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := httpClient.Get(mediaURL)
	if err != nil {
		return "", fmt.Errorf("falha ao baixar mídia da URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("falha ao baixar mídia da URL: status %d", resp.StatusCode)
	}

	// Ler o conteúdo da resposta
	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("falha ao ler conteúdo da resposta: %w", err)
	}

	// Fazer upload do arquivo para o WhatsApp
	uploadResp, err := client.WAClient.Upload(ctx, fileData, uploadType)
	if err != nil {
		return "", fmt.Errorf("falha ao fazer upload: %w", err)
	}

	var msg whatsmeow.SendResponse

	// Enviar conforme o tipo de mídia
	switch mediaType {
	case "image", "img":
		imageMsg := &waE2E.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(resp.Header.Get("Content-Type")),
			URL:           &uploadResp.URL,
			DirectPath:    &uploadResp.DirectPath,
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    &uploadResp.FileLength,
		}

		msg, err = client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
			ImageMessage: imageMsg,
		})

	case "video", "vid":
		videoMsg := &waE2E.VideoMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String(resp.Header.Get("Content-Type")),
			URL:           &uploadResp.URL,
			DirectPath:    &uploadResp.DirectPath,
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    &uploadResp.FileLength,
		}

		msg, err = client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
			VideoMessage: videoMsg,
		})

	case "audio", "voice":
		audioMsg := &waE2E.AudioMessage{
			Mimetype:      proto.String(resp.Header.Get("Content-Type")),
			URL:           &uploadResp.URL,
			DirectPath:    &uploadResp.DirectPath,
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    &uploadResp.FileLength,
			PTT:           proto.Bool(mediaType == "voice"),
		}

		msg, err = client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
			AudioMessage: audioMsg,
		})

	case "document", "doc", "file":
		documentMsg := &waE2E.DocumentMessage{
			Caption:       proto.String(caption),
			FileName:      proto.String(fileName),
			Mimetype:      proto.String(resp.Header.Get("Content-Type")),
			URL:           &uploadResp.URL,
			DirectPath:    &uploadResp.DirectPath,
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    &uploadResp.FileLength,
		}

		msg, err = client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
			DocumentMessage: documentMsg,
		})
	}

	if err != nil {
		return "", fmt.Errorf("falha ao enviar mídia: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Mensagem de mídia da URL enviada", "user_id", userID, "to", to, "type", mediaType, "url", mediaURL, "message_id", msg.ID)

	return msg.ID, nil
}

// SendButtons envia uma mensagem com botões - now using worker types directly
func (ms *MessageService) SendButtons(userID, to, text, footer string, buttons []worker.ButtonData) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Criar botões
	var btnItems []*waE2E.ButtonsMessage_Button
	for _, btn := range buttons {
		btnItems = append(btnItems, &waE2E.ButtonsMessage_Button{
			ButtonID: proto.String(btn.ID),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String(btn.DisplayText),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		})
	}

	// Criar mensagem com botões
	buttonsMessage := &waE2E.ButtonsMessage{
		ContentText: proto.String(text),
		Buttons:     btnItems,
		HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
	}

	if footer != "" {
		buttonsMessage.FooterText = proto.String(footer)
	}

	// Criar message wrapper
	message := &waE2E.Message{
		ButtonsMessage: buttonsMessage,
	}

	// Enviar mensagem com botões
	msg, err := client.WAClient.SendMessage(ctx, recipient, message)
	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem com botões: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Mensagem com botões enviada", "user_id", userID, "to", to, "message_id", msg.ID)

	return msg.ID, nil
}

// SendList envia uma mensagem com lista - now using worker types directly
func (ms *MessageService) SendList(userID, to, text, footer, buttonText string, sections []worker.Section) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Criar seções
	var listSections []*waE2E.ListMessage_Section
	for _, section := range sections {
		var rows []*waE2E.ListMessage_Row

		for _, row := range section.Rows {
			rowEntry := &waE2E.ListMessage_Row{
				RowID: proto.String(row.ID),
				Title: proto.String(row.Title),
			}

			if row.Description != "" {
				rowEntry.Description = proto.String(row.Description)
			}

			rows = append(rows, rowEntry)
		}

		listSections = append(listSections, &waE2E.ListMessage_Section{
			Title: proto.String(section.Title),
			Rows:  rows,
		})
	}

	// Criar mensagem de lista
	listMessage := &waE2E.ListMessage{
		// Nota: o exemplo usa Title e Description, mas seu código usa apenas Description
		// Adaptando para seguir seu modelo original, mas você pode querer adicionar Title se necessário
		Description: proto.String(text),
		Sections:    listSections,
		ButtonText:  proto.String(buttonText),
		ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
	}

	if footer != "" {
		listMessage.FooterText = proto.String(footer)
	}

	// Criar message wrapper
	message := &waE2E.Message{
		ListMessage: listMessage,
	}

	// Enviar mensagem de lista
	msg, err := client.WAClient.SendMessage(ctx, recipient, message)
	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem de lista: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Mensagem de lista enviada", "user_id", userID, "to", to, "message_id", msg.ID)

	return msg.ID, nil
}

// SendLocation envia uma mensagem de localização
func (ms *MessageService) SendLocation(userID, to string, latitude, longitude float64, name, address *string) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Criar mensagem de localização
	locationMsg := &waE2E.LocationMessage{
		DegreesLatitude:  proto.Float64(latitude),
		DegreesLongitude: proto.Float64(longitude),
	}

	if name != nil {
		locationMsg.Name = proto.String(*name)
	}

	if address != nil {
		locationMsg.Address = proto.String(*address)
	}

	// Enviar mensagem de localização
	msg, err := client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
		LocationMessage: locationMsg,
	})

	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem de localização: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Mensagem de localização enviada", "user_id", userID, "to", to, "message_id", msg.ID)

	return msg.ID, nil
}

// SendContact envia uma mensagem de contato
func (ms *MessageService) SendContact(userID, to string, contacts []interface{}) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Criar mensagem de contatos
	var contactMessages []*waE2E.ContactMessage
	for _, contact := range contacts {
		// Convert interface{} to map for processing
		if contactMap, ok := contact.(map[string]interface{}); ok {
			contactMsg := &waE2E.ContactMessage{}

			if displayName, ok := contactMap["displayName"].(string); ok {
				contactMsg.DisplayName = proto.String(displayName)
			}

			if vcard, ok := contactMap["vcard"].(string); ok {
				contactMsg.Vcard = proto.String(vcard)
			}

			contactMessages = append(contactMessages, contactMsg)
		}
	}

	if len(contactMessages) == 0 {
		return "", fmt.Errorf("no valid contacts found")
	}

	// Para múltiplos contatos, use ContactsArrayMessage
	if len(contactMessages) > 1 {
		contactsArrayMsg := &waE2E.ContactsArrayMessage{
			DisplayName: proto.String("Shared Contacts"),
			Contacts:    contactMessages,
		}

		msg, err := client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
			ContactsArrayMessage: contactsArrayMsg,
		})

		if err != nil {
			return "", fmt.Errorf("falha ao enviar mensagem de contatos: %w", err)
		}

		// Atualizar última atividade
		client.LastActive = time.Now()

		// Log
		logger.Debug("Mensagem de contatos enviada", "user_id", userID, "to", to, "message_id", msg.ID)

		return msg.ID, nil
	}

	// Para um único contato, use ContactMessage
	msg, err := client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
		ContactMessage: contactMessages[0],
	})

	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem de contato: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Mensagem de contato enviada", "user_id", userID, "to", to, "message_id", msg.ID)

	return msg.ID, nil
}

// SendReaction envia uma reação a uma mensagem
func (ms *MessageService) SendReaction(userID, to, targetJID, targetMessageID, emoji string, fromMe bool, participant *string) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Converter target JID
	targetJIDParsed, err := ParseJID(targetJID)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the message key for the target message
	messageKey := &waCommon.MessageKey{
		RemoteJID: proto.String(targetJIDParsed.String()),
		ID:        proto.String(targetMessageID),
		FromMe:    proto.Bool(fromMe),
	}

	if participant != nil {
		participantJID, err := ParseJID(*participant)
		if err != nil {
			return "", err
		}
		messageKey.Participant = proto.String(participantJID.String())
	}

	// Send reaction using whatsmeow's SendMessage with ReactionMessage
	reactionMsg := &waE2E.ReactionMessage{
		Key:  messageKey,
		Text: proto.String(emoji),
	}

	msg, err := client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
		ReactionMessage: reactionMsg,
	})

	if err != nil {
		return "", fmt.Errorf("falha ao enviar reação: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Reação enviada", "user_id", userID, "to", to, "emoji", emoji, "message_id", msg.ID)

	return msg.ID, nil
}

// SendPoll envia uma mensagem de enquete
func (ms *MessageService) SendPoll(userID, to, name string, options []string, selectableCount int) (string, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verificar se há opções suficientes
	if len(options) < 2 {
		return "", fmt.Errorf("poll must have at least 2 options")
	}

	if len(options) > 12 {
		return "", fmt.Errorf("poll cannot have more than 12 options")
	}

	// Validar selectableCount
	if selectableCount < 1 {
		selectableCount = 1
	}
	if selectableCount > len(options) {
		selectableCount = len(options)
	}

	// Criar opções da enquete
	var pollOptions []*waE2E.PollCreationMessage_Option
	for _, option := range options {
		pollOptions = append(pollOptions, &waE2E.PollCreationMessage_Option{
			OptionName: proto.String(option),
		})
	}

	// Criar mensagem de enquete
	pollMsg := &waE2E.PollCreationMessage{
		Name:                   proto.String(name),
		Options:                pollOptions,
		SelectableOptionsCount: proto.Uint32(uint32(selectableCount)),
	}

	// Enviar enquete
	msg, err := client.WAClient.SendMessage(ctx, recipient, &waE2E.Message{
		PollCreationMessage: pollMsg,
	})

	if err != nil {
		return "", fmt.Errorf("falha ao enviar enquete: %w", err)
	}

	// Atualizar última atividade
	client.LastActive = time.Now()

	// Log
	logger.Debug("Enquete enviada", "user_id", userID, "to", to, "name", name, "options_count", len(options), "message_id", msg.ID)

	return msg.ID, nil
}
