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

// ValidateAndOrganizeRecipient validates and organizes the 'to' field based on different scenarios
// Returns the processed JID string and any validation errors
func (ms *MessageService) ValidateAndOrganizeRecipient(userID, to string) (string, error) {
	// Clean the input
	to = strings.TrimSpace(to)

	if to == "" {
		return "", fmt.Errorf("recipient cannot be empty")
	}

	// Check if it's a special type (group, newsletter, broadcaster, etc.)
	if strings.Contains(to, "@g.us") ||
		strings.Contains(to, "@newsletter") ||
		strings.Contains(to, "@broadcaster") ||
		strings.Contains(to, "@lid") {
		// For special types, just validate the format without WhatsApp number validation
		return ms.validateSpecialJID(to)
	}

	// If it's a phone number, validate and process it
	if ms.isPhoneNumber(to) {
		return ms.validateAndProcessPhoneNumber(userID, to)
	}

	// If it already contains @, try to parse as-is
	if strings.Contains(to, "@") {
		_, err := types.ParseJID(to)
		if err != nil {
			return "", fmt.Errorf("JID inválido: %w", err)
		}
		return to, nil
	}

	// Default: treat as phone number and add @s.whatsapp.net
	return ms.validateAndProcessPhoneNumber(userID, to)
}

// validateSpecialJID validates JIDs for groups, newsletters, broadcasters, etc.
func (ms *MessageService) validateSpecialJID(jid string) (string, error) {
	// Parse to ensure it's a valid JID format
	_, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("JID especial inválido: %w", err)
	}
	return jid, nil
}

// isPhoneNumber checks if the input looks like a phone number
func (ms *MessageService) isPhoneNumber(input string) bool {
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

// validateAndProcessPhoneNumber processes and validates phone numbers
func (ms *MessageService) validateAndProcessPhoneNumber(userID, phoneNumber string) (string, error) {
	// Clean the phone number
	cleaned := ms.cleanPhoneNumber(phoneNumber)

	// Handle Brazilian numbers with the 9-digit rule
	processed := ms.processBrazilianNumber(cleaned)

	// Check if the number exists on WhatsApp
	exists, err := ms.checkNumberExistsOnWhatsApp(userID, processed)
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
				alternative := ms.removeNinthDigitFromBrazilian(processed)
				if alternative != processed {
					alternatives = append(alternatives, alternative)
				}
			} else if len(processed) == 12 {
				// 12 digits: try adding the 9
				alternative := ms.addNinthDigitToBrazilian(processed)
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
				altExists, altErr := ms.checkNumberExistsOnWhatsApp(userID, alt)
				if altErr == nil && altExists {
					processed = alt
					break
				}
			}
		}
	}

	// Format as WhatsApp JID
	jid := processed + "@s.whatsapp.net"

	return jid, nil
}

// cleanPhoneNumber removes formatting characters from phone number
func (ms *MessageService) cleanPhoneNumber(phone string) string {
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
func (ms *MessageService) processBrazilianNumber(number string) string {
	// If it doesn't start with country code, check if it's a Brazilian local number
	if !strings.HasPrefix(number, "55") && len(number) >= 10 && len(number) <= 11 {
		// Check if it looks like a Brazilian local number by area code
		if isBrazilianAreaCode(number) {
			// Add Brazilian country code
			if len(number) == 11 || len(number) == 10 {
				number = "55" + number
			}
		}
	}

	return number
}

// removeNinthDigitFromBrazilian removes the 9th digit from Brazilian mobile numbers
func (ms *MessageService) removeNinthDigitFromBrazilian(number string) string {
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
func (ms *MessageService) addNinthDigitToBrazilian(number string) string {
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

// checkNumberExistsOnWhatsApp verifies if a number exists on WhatsApp
func (ms *MessageService) checkNumberExistsOnWhatsApp(userID, number string) (bool, error) {
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return false, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Verificar se o cliente está conectado
	if !client.Connected {
		return false, fmt.Errorf("cliente não está conectado")
	}

	// Format with + for WhatsApp API
	numberWithPlus := "+" + number + "@s.whatsapp.net"

	// Check status
	responses, err := client.WAClient.IsOnWhatsApp([]string{numberWithPlus})
	if err != nil {
		return false, fmt.Errorf("falha ao verificar número: %w", err)
	}

	if len(responses) == 0 {
		return false, nil
	}

	return responses[0].IsIn, nil
}

// CheckNumberExistsOnWhatsApp verifies if a number exists on WhatsApp (public method)
func (ms *MessageService) CheckNumberExistsOnWhatsApp(userID, number string) (bool, error) {
	return ms.checkNumberExistsOnWhatsApp(userID, number)
}

// ParseJID converte uma string para um JID do WhatsApp
// Deprecated: Use ValidateAndOrganizeRecipient for better validation
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

	// Validate and organize recipient
	validatedJID, err := ms.ValidateAndOrganizeRecipient(userID, to)
	if err != nil {
		return "", fmt.Errorf("erro ao validar destinatário: %w", err)
	}

	// Convert to JID
	recipient, err := types.ParseJID(validatedJID)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
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

// ValidateRecipientFormat validates recipient format without WhatsApp connectivity check
// This is useful for testing and validation without requiring an active session
func ValidateRecipientFormat(to string) (string, error) {
	// Clean the input
	to = strings.TrimSpace(to)

	if to == "" {
		return "", fmt.Errorf("recipient cannot be empty")
	}

	// Check if it's a special type (group, newsletter, broadcaster, etc.)
	if strings.Contains(to, "@g.us") ||
		strings.Contains(to, "@newsletter") ||
		strings.Contains(to, "@broadcaster") ||
		strings.Contains(to, "@lid") {
		// For special types, just validate the format
		_, err := types.ParseJID(to)
		if err != nil {
			return "", fmt.Errorf("JID especial inválido: %w", err)
		}
		return to, nil
	}

	// If it's a phone number, process it
	if isPhoneNumberFormat(to) {
		processed := processPhoneNumberFormat(to)
		return processed + "@s.whatsapp.net", nil
	}

	// If it already contains @, try to parse as-is
	if strings.Contains(to, "@") {
		_, err := types.ParseJID(to)
		if err != nil {
			return "", fmt.Errorf("JID inválido: %w", err)
		}
		return to, nil
	}

	// If we reach here, it's not a valid phone number or JID
	return "", fmt.Errorf("formato inválido: deve ser um número de telefone válido ou JID")
}

// isPhoneNumberFormat checks if the input looks like a phone number (standalone version)
func isPhoneNumberFormat(input string) bool {
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

// processPhoneNumberFormat processes phone number format (standalone version)
func processPhoneNumberFormat(phoneNumber string) string {
	// Clean the phone number
	cleaned := cleanPhoneNumberFormat(phoneNumber)

	// Handle Brazilian numbers with the 9-digit rule
	processed := processBrazilianNumberFormat(cleaned)

	return processed
}

// cleanPhoneNumberFormat removes formatting characters from phone number (standalone version)
func cleanPhoneNumberFormat(phone string) string {
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

// processBrazilianNumberFormat handles Brazilian number formatting (standalone version)
func processBrazilianNumberFormat(number string) string {
	// If it doesn't start with country code, check if it's a Brazilian local number
	if !strings.HasPrefix(number, "55") && len(number) >= 10 && len(number) <= 11 {
		// Check if it looks like a Brazilian local number by area code
		if isBrazilianAreaCode(number) {
			// Add Brazilian country code
			if len(number) == 11 || len(number) == 10 {
				number = "55" + number
			}
		}
	}

	return number
}

// isBrazilianAreaCode checks if a number starts with a valid Brazilian area code
func isBrazilianAreaCode(number string) bool {
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
