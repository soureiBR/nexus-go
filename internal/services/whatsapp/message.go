// internal/services/whatsapp/message.go
package whatsapp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"yourproject/pkg/logger"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

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
func (sm *SessionManager) SendText(userID, to, message string) (string, error) {
	client, exists := sm.GetSession(userID)
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
func (sm *SessionManager) SendMedia(userID, to, filePath, mediaType, caption string) (string, error) {
	client, exists := sm.GetSession(userID)
	if !exists {
		return "", fmt.Errorf("sessão não encontrada: %s", userID)
	}

	// Converter para JID
	recipient, err := ParseJID(to)
	if err != nil {
		return "", err
	}

	// Ler arquivo
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("falha ao ler arquivo: %w", err)
	}

	// Obter informações do arquivo
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("falha ao obter informações do arquivo: %w", err)
	}

	// Determinar tipo de mídia se não especificado
	if mediaType == "" {
		// Tentar deduzir da extensão
		switch {
		case strings.HasSuffix(filePath, ".jpg"), strings.HasSuffix(filePath, ".jpeg"), strings.HasSuffix(filePath, ".png"):
			mediaType = "image"
		case strings.HasSuffix(filePath, ".mp4"), strings.HasSuffix(filePath, ".avi"), strings.HasSuffix(filePath, ".mov"):
			mediaType = "video"
		case strings.HasSuffix(filePath, ".mp3"), strings.HasSuffix(filePath, ".wav"), strings.HasSuffix(filePath, ".ogg"):
			mediaType = "audio"
		default:
			mediaType = "document"
		}
	}

	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var msg whatsmeow.SendResponse

	// Determinar tipo de upload baseado no tipo de mídia
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

	// Fazer upload do arquivo
	uploadResp, err := client.WAClient.Upload(ctx, fileData, uploadType)
	if err != nil {
		return "", fmt.Errorf("falha ao fazer upload: %w", err)
	}

	// Enviar conforme o tipo de mídia
	switch mediaType {
	case "image", "img":
		imageMsg := &waE2E.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String("image/jpeg"),
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
			Mimetype:      proto.String("video/mp4"),
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
			Mimetype:      proto.String("audio/mpeg"),
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
			FileName:      proto.String(fileInfo.Name()),
			Mimetype:      proto.String("application/octet-stream"),
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
	logger.Debug("Mensagem de mídia enviada", "user_id", userID, "to", to, "type", mediaType, "message_id", msg.ID)

	return msg.ID, nil
}

// SendButtons envia uma mensagem com botões
func (sm *SessionManager) SendButtons(userID, to, text, footer string, buttons []ButtonData) (string, error) {
	client, exists := sm.GetSession(userID)
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
				DisplayText: proto.String(btn.Text),
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

// SendList envia uma mensagem com lista
func (sm *SessionManager) SendList(userID, to, text, footer, buttonText string, sections []Section) (string, error) {
	client, exists := sm.GetSession(userID)
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
