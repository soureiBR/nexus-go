// internal/services/whatsapp/messaging/message.go
package messaging

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"time"

	"yourproject/internal/services/whatsapp/session"
	"yourproject/internal/services/whatsapp/worker"
	"yourproject/internal/services/whatsapp/extensions"
	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary"
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

	// Check if recipient is a newsletter - handle differently
	if strings.Contains(validatedJID, "@newsletter") {
		return ms.sendMediaToNewsletter(userID, validatedJID, mediaURL, mediaType, caption)
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
		// Calculate audio duration in seconds (placeholder - should be extracted from actual audio file)
		audioDurationInSeconds := uint32(30) // TODO: Extract actual duration from audio file
		
		audioMsg := &waE2E.AudioMessage{
			Mimetype:      proto.String("audio/ogg; codecs=opus"),
			URL:           &uploadResp.URL,
			DirectPath:    &uploadResp.DirectPath,
			MediaKey:      uploadResp.MediaKey,
			FileEncSHA256: uploadResp.FileEncSHA256,
			FileSHA256:    uploadResp.FileSHA256,
			FileLength:    &uploadResp.FileLength,
			PTT:           proto.Bool(true),
			Seconds:       proto.Uint32(audioDurationInSeconds),
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

// sendMediaToNewsletter handles media upload specifically for newsletters using proper WhatsApp newsletter upload method
func (ms *MessageService) sendMediaToNewsletter(userID, newsletterJID, mediaURL, mediaType, caption string) (string, error) {
	logger.Debug("Enviando mídia para newsletter",
		"user_id", userID,
		"newsletter_jid", newsletterJID,
		"media_url", mediaURL,
		"media_type", mediaType)

	// Get client session
	client, exists := ms.sessionManager.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Parse newsletter JID
	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return "", fmt.Errorf("JID da newsletter inválido: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Determine WhatsApp media type for newsletter upload
	var uploadType whatsmeow.MediaType
	switch mediaType {
	case "image", "img":
		uploadType = whatsmeow.MediaImage
	case "video", "vid":
		uploadType = whatsmeow.MediaVideo
	case "audio", "voice":
		uploadType = whatsmeow.MediaAudio
	default:
		return "", fmt.Errorf("newsletters suportam apenas imagens, vídeos e áudios, tipo '%s' não suportado", mediaType)
	}

	// Download the media from URL
	logger.Debug("Baixando mídia da URL para newsletter", "url", mediaURL, "type", mediaType)
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := httpClient.Get(mediaURL)
	if err != nil {
		return "", fmt.Errorf("falha ao baixar mídia da URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("falha ao baixar mídia: HTTP %d - %s", resp.StatusCode, resp.Status)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	switch mediaType {
	case "image", "img":
		if contentType != "" && !strings.HasPrefix(contentType, "image/") {
			logger.Warn("Tipo de conteúdo suspeito para imagem de newsletter",
				"content_type", contentType,
				"image_url", mediaURL)
		}
	case "video", "vid":
		if contentType != "" && !strings.HasPrefix(contentType, "video/") {
			logger.Warn("Tipo de conteúdo suspeito para vídeo de newsletter",
				"content_type", contentType,
				"video_url", mediaURL)
		}
	case "audio", "voice":
		if contentType != "" && !strings.HasPrefix(contentType, "audio/") {
			logger.Warn("Tipo de conteúdo suspeito para áudio de newsletter",
				"content_type", contentType,
				"audio_url", mediaURL)
		}
	}

	// Read the media data
	mediaData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("falha ao ler dados da mídia: %w", err)
	}

	// Validate media data
	if len(mediaData) == 0 {
		return "", fmt.Errorf("dados da mídia estão vazios")
	}

	logger.Debug("Mídia baixada para newsletter",
		"user_id", userID,
		"newsletter_jid", newsletterJID,
		"media_url", mediaURL,
		"media_type", mediaType,
		"size_bytes", len(mediaData),
		"content_type", contentType)

	// For images, convert to JPEG if needed (WhatsApp newsletters prefer JPEG)
	var processedData []byte
	if mediaType == "image" || mediaType == "img" {
		processedData, err = ms.convertToJPEG(mediaData, 85)
		if err != nil {
			logger.Warn("Falha ao converter para JPEG, usando dados originais", "error", err)
			processedData = mediaData
		} else {
			logger.Debug("Imagem convertida para JPEG para newsletter",
				"user_id", userID,
				"newsletter_jid", newsletterJID,
				"original_size", len(mediaData),
				"jpeg_size", len(processedData))
		}
	} else {
		// For video and audio, use data as-is
		processedData = mediaData
	}

	// Upload media specifically for newsletter (unencrypted)
	logger.Debug("Fazendo upload da mídia para newsletter usando UploadNewsletter",
		"newsletter_jid", newsletterJID,
		"media_type", mediaType,
		"upload_type", uploadType)

	uploadResp, err := client.WAClient.UploadNewsletter(ctx, processedData, uploadType)
	if err != nil {
		return "", fmt.Errorf("falha ao fazer upload da mídia para newsletter: %w", err)
	}

	logger.Debug("Upload da mídia para newsletter concluído",
		"newsletter_jid", newsletterJID,
		"media_type", mediaType,
		"upload_url", uploadResp.URL,
		"direct_path", uploadResp.DirectPath,
		"file_length", uploadResp.FileLength,
		"has_handle", uploadResp.Handle != "")

	// Create appropriate message based on media type
	var message *waE2E.Message
	switch mediaType {
	case "image", "img":
		imageMsg := &waE2E.ImageMessage{
			Caption:    proto.String(caption),
			Mimetype:   proto.String(contentType),
			URL:        &uploadResp.URL,
			DirectPath: &uploadResp.DirectPath,
			FileSHA256: uploadResp.FileSHA256,
			FileLength: &uploadResp.FileLength,
			// Note: No MediaKey or FileEncSHA256 for newsletter media (unencrypted)
		}
		message = &waE2E.Message{ImageMessage: imageMsg}

	case "video", "vid":
		videoMsg := &waE2E.VideoMessage{
			Caption:    proto.String(caption),
			Mimetype:   proto.String(contentType),
			URL:        &uploadResp.URL,
			DirectPath: &uploadResp.DirectPath,
			FileSHA256: uploadResp.FileSHA256,
			FileLength: &uploadResp.FileLength,
			// Note: No MediaKey or FileEncSHA256 for newsletter media (unencrypted)
		}
		message = &waE2E.Message{VideoMessage: videoMsg}

	case "audio", "voice":
		// Calculate audio duration in seconds (placeholder - should be extracted from actual audio file)
		audioDurationInSeconds := uint32(30) // TODO: Extract actual duration from audio file
		
		audioMsg := &waE2E.AudioMessage{
			Mimetype:   proto.String("audio/ogg; codecs=opus"),
			URL:        &uploadResp.URL,
			DirectPath: &uploadResp.DirectPath,
			FileSHA256: uploadResp.FileSHA256,
			FileLength: &uploadResp.FileLength,
			PTT:        proto.Bool(true),
			Seconds:    proto.Uint32(audioDurationInSeconds),
			// Note: No MediaKey or FileEncSHA256 for newsletter media (unencrypted)
		}
		message = &waE2E.Message{AudioMessage: audioMsg}
	}

	// Send the message with the media handle (crucial for newsletter media)
	sendExtra := whatsmeow.SendRequestExtra{
		MediaHandle: uploadResp.Handle, // This is the crucial part for newsletter media
	}

	msg, err := client.WAClient.SendMessage(ctx, parsedJID, message, sendExtra)
	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem de mídia para newsletter: %w", err)
	}

	// Update last activity
	client.LastActive = time.Now()

	logger.Debug("Mídia enviada com sucesso para newsletter",
		"user_id", userID,
		"newsletter_jid", newsletterJID,
		"media_type", mediaType,
		"message_id", msg.ID,
		"media_handle", uploadResp.Handle)

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

// Helper methods for newsletter media upload

// convertToJPEG converts image data to JPEG format with WhatsApp-compatible settings
func (ms *MessageService) convertToJPEG(imageData []byte, quality int) ([]byte, error) {
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

	// WhatsApp has specific requirements for newsletter photos
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

// updateNewsletterPictureViaMex sends a newsletter picture update using MEX
func (ms *MessageService) updateNewsletterPictureViaMex(client *whatsmeow.Client, ctx context.Context, newsletterJID string, imageData []byte) error {
	internals := client.DangerousInternals()

	// Converter imagem para base64 (como no Baileys)
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Usar mesma estrutura do name/description
	variables := map[string]interface{}{
		"newsletter_id": newsletterJID,
		"updates": map[string]interface{}{
			"picture":  base64Image, // Imagem em base64
			"settings": nil,
		},
	}

	// Use the confirmed working query ID for newsletter updates
	queryID := "6620195908089573"

	logger.Debug("MEX query para picture - variables completas",
		"query_id", queryID,
		"newsletter_jid", newsletterJID,
		"image_size_bytes", len(imageData),
		"base64_length", len(base64Image))

	result, err := internals.SendMexIQ(ctx, queryID, variables)
	if err != nil {
		logger.Error("MEX query failed for picture",
			"error", err,
			"query_id", queryID,
			"newsletter_jid", newsletterJID)
		return fmt.Errorf("MEX query failed for picture: %w", err)
	}

	logger.Debug("MEX query succeeded for picture",
		"query_id", queryID,
		"result", string(result),
		"newsletter_jid", newsletterJID)
	return nil
}

// updateNewsletterPictureViaRawNodes sends a newsletter picture update using raw binary nodes
func (ms *MessageService) updateNewsletterPictureViaRawNodes(client *whatsmeow.Client, ctx context.Context, newsletterJID string, imageData []byte) error {
	internals := client.DangerousInternals()

	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return fmt.Errorf("invalid newsletter JID: %w", err)
	}

	// Converter imagem para base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Build raw binary node
	node := binary.Node{
		Tag: "iq",
		Attrs: binary.Attrs{
			"id":    internals.GenerateRequestID(),
			"type":  "set",
			"to":    parsedJID.String(),
			"xmlns": "newsletter",
		},
		Content: []binary.Node{{
			Tag: "update",
			Content: []binary.Node{
				{
					Tag:     "picture",
					Content: base64Image,
				},
			},
		}},
	}

	// Send raw node
	err = internals.SendNode(node)
	if err != nil {
		return fmt.Errorf("failed to send raw node for picture: %w", err)
	}

	logger.Debug("Raw node sent successfully for picture",
		"newsletter_jid", newsletterJID,
		"image_size_bytes", len(imageData),
		"base64_length", len(base64Image))
	return nil
}

// setNewsletterPhotoViaExtension sets the newsletter photo using custom extension
func (ms *MessageService) setNewsletterPhotoViaExtension(client *whatsmeow.Client, jid types.JID, imageData []byte) (string, error) {
	// Use the extension method for setting newsletter photo
	return extensions.SetNewsletterPhoto(client, jid, imageData)
}
