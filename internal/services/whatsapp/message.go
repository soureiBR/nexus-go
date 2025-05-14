// internal/services/whatsapp/message.go
package whatsapp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	
	"yourproject/pkg/logger"
)

// ButtonData representa um botão para mensagens interativas
type ButtonData struct {
	ID      string `json:"id" binding:"required"`
	Text    string `json:"text" binding:"required"`
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
	msg, err := client.WAClient.SendMessage(ctx, recipient, &proto.Message{
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
	
	// Abrir arquivo
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("falha ao abrir arquivo: %w", err)
	}
	defer file.Close()
	
	// Obter informações do arquivo
	fileInfo, err := file.Stat()
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
	
	// Enviar conforme o tipo de mídia
	switch mediaType {
	case "image", "img":
		uploadedImg, err := client.WAClient.Upload(ctx, file, whatsmeow.MediaImage)
		if err != nil {
			return "", fmt.Errorf("falha ao fazer upload da imagem: %w", err)
		}
		
		msg, err = client.WAClient.SendMessage(ctx, recipient, &proto.Message{
			ImageMessage: &proto.ImageMessage{
				Caption:       proto.String(caption),
				Url:           proto.String(uploadedImg.URL),
				DirectPath:    proto.String(uploadedImg.DirectPath),
				MediaKey:      uploadedImg.MediaKey,
				Mimetype:      proto.String("image/jpeg"),
				FileEncSha256: uploadedImg.FileEncSHA256,
				FileSha256:    uploadedImg.FileSHA256,
				FileLength:    proto.Uint64(uint64(fileInfo.Size())),
			},
		})
		
	case "video", "vid":
		uploadedVideo, err := client.WAClient.Upload(ctx, file, whatsmeow.MediaVideo)
		if err != nil {
			return "", fmt.Errorf("falha ao fazer upload do vídeo: %w", err)
		}
		
		msg, err = client.WAClient.SendMessage(ctx, recipient, &proto.Message{
			VideoMessage: &proto.VideoMessage{
				Caption:       proto.String(caption),
				Url:           proto.String(uploadedVideo.URL),
				DirectPath:    proto.String(uploadedVideo.DirectPath),
				MediaKey:      uploadedVideo.MediaKey,
				Mimetype:      proto.String("video/mp4"),
				FileEncSha256: uploadedVideo.FileEncSHA256,
				FileSha256:    uploadedVideo.FileSHA256,
				FileLength:    proto.Uint64(uint64(fileInfo.Size())),
			},
		})
		
	case "audio", "voice":
		uploadedAudio, err := client.WAClient.Upload(ctx, file, whatsmeow.MediaAudio)
		if err != nil {
			return "", fmt.Errorf("falha ao fazer upload do áudio: %w", err)
		}
		
		msg, err = client.WAClient.SendMessage(ctx, recipient, &proto.Message{
			AudioMessage: &proto.AudioMessage{
				Url:           proto.String(uploadedAudio.URL),
				DirectPath:    proto.String(uploadedAudio.DirectPath),
				MediaKey:      uploadedAudio.MediaKey,
				Mimetype:      proto.String("audio/mpeg"),
				FileEncSha256: uploadedAudio.FileEncSHA256,
				FileSha256:    uploadedAudio.FileSHA256,
				FileLength:    proto.Uint64(uint64(fileInfo.Size())),
				Ptt:           proto.Bool(mediaType == "voice"),
			},
		})
		
	case "document", "doc", "file":
		uploadedDoc, err := client.WAClient.Upload(ctx, file, whatsmeow.MediaDocument)
		if err != nil {
			return "", fmt.Errorf("falha ao fazer upload do documento: %w", err)
		}
		
		msg, err = client.WAClient.SendMessage(ctx, recipient, &proto.Message{
			DocumentMessage: &proto.DocumentMessage{
				Caption:       proto.String(caption),
				Url:           proto.String(uploadedDoc.URL),
				DirectPath:    proto.String(uploadedDoc.DirectPath),
				MediaKey:      uploadedDoc.MediaKey,
				FileName:      proto.String(fileInfo.Name()),
				Mimetype:      proto.String("application/octet-stream"),
				FileEncSha256: uploadedDoc.FileEncSHA256,
				FileSha256:    uploadedDoc.FileSHA256,
				FileLength:    proto.Uint64(uint64(fileInfo.Size())),
			},
		})
		
	default:
		return "", fmt.Errorf("tipo de mídia não suportado: %s", mediaType)
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
	
	// Criar botões
	var btnItems []*proto.ButtonsMessage_Button
	for _, btn := range buttons {
		btnItems = append(btnItems, &proto.ButtonsMessage_Button{
			ButtonId: proto.String(btn.ID),
			ButtonText: &proto.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String(btn.Text),
			},
			Type: proto.ButtonsMessage_Button_RESPONSE.Enum(),
		})
	}
	
	// Criar mensagem com botões
	buttonsMessage := &proto.ButtonsMessage{
		ContentText: proto.String(text),
		Buttons:     btnItems,
		HeaderType:  proto.ButtonsMessage_EMPTY.Enum(),
	}
	
	if footer != "" {
		buttonsMessage.FooterText = proto.String(footer)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Enviar mensagem com botões
	msg, err := client.WAClient.SendMessage(ctx, recipient, &proto.Message{
		ButtonsMessage: buttonsMessage,
	})
	
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
	
	// Criar seções
	var listSections []*proto.ListMessage_Section
	for _, section := range sections {
		var rows []*proto.ListMessage_Row
		
		for _, row := range section.Rows {
			rows = append(rows, &proto.ListMessage_Row{
				RowId: proto.String(row.ID),
				Title: proto.String(row.Title),
				Description: proto.String(row.Description),
			})
		}
		
		listSections = append(listSections, &proto.ListMessage_Section{
			Title: proto.String(section.Title),
			Rows:  rows,
		})
	}
	
	// Criar mensagem de lista
	listMessage := &proto.ListMessage{
		Description:    proto.String(text),
		Sections:       listSections,
		ButtonText:     proto.String(buttonText),
		ListType:       proto.ListMessage_SINGLE_SELECT.Enum(),
	}
	
	if footer != "" {
		listMessage.FooterText = proto.String(footer)
	}
	
	// Criar contexto com timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Enviar mensagem de lista
	msg, err := client.WAClient.SendMessage(ctx, recipient, &proto.Message{
		ListMessage: listMessage,
	})
	
	if err != nil {
		return "", fmt.Errorf("falha ao enviar mensagem de lista: %w", err)
	}
	
	// Atualizar última atividade
	client.LastActive = time.Now()
	
	// Log
	logger.Debug("Mensagem de lista enviada", "user_id", userID, "to", to, "message_id", msg.ID)
	
	return msg.ID, nil
}