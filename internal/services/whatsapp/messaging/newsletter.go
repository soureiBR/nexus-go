// internal/services/whatsapp/newsletter.go
package messaging

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"

	"yourproject/internal/services/whatsapp/extensions"
	"yourproject/internal/services/whatsapp/session"
	"yourproject/pkg/logger"
)

// Newsletter mutation query ID (discovered through WhatsApp Web traffic analysis)
const mutationUpdateNewsletterQueryID = "7150902998257522"

// NewsletterService encapsula a funcionalidade de newsletters (canais) do WhatsApp
type NewsletterService struct {
	newsletterManager session.NewsletterManager
}

// NewNewsletterService cria um novo serviço de newsletter
func NewNewsletterService(newsletterManager session.NewsletterManager) *NewsletterService {
	return &NewsletterService{
		newsletterManager: newsletterManager,
	}
}

// getClient obtém o cliente WhatsApp para uma sessão específica
func (s *NewsletterService) getClient(userID string) (*whatsmeow.Client, error) {
	client, exists := s.newsletterManager.GetSession(userID)
	if !exists {
		return nil, fmt.Errorf("sessão não encontrada: %s", userID)
	}

	if !client.IsConnected() {
		return nil, fmt.Errorf("sessão não está conectada: %s", userID)
	}

	waClient := client.GetWAClient()
	if waClient == nil {
		return nil, fmt.Errorf("cliente WhatsApp não disponível: %s", userID)
	}

	// Update activity
	client.UpdateActivity()

	return waClient, nil
}

// updateNewsletterViaMex sends a newsletter update using MEX (GraphQL-like queries)
func (s *NewsletterService) updateNewsletterViaMex(client *whatsmeow.Client, ctx context.Context, newsletterJID, name string) error {
	internals := client.DangerousInternals()
	
	// Prepare variables for the GraphQL query
	variables := map[string]interface{}{
		"newsletter_id": newsletterJID,
		"updates": map[string]interface{}{
			"name": name,
		},
	}
	
	// Use the confirmed working query ID for newsletter updates
	queryID := mutationUpdateNewsletterQueryID
	
	result, err := internals.SendMexIQ(ctx, queryID, variables)
	if err != nil {
		return fmt.Errorf("MEX query failed: %w", err)
	}
	
	logger.Debug("MEX query succeeded", 
		"query_id", queryID, 
		"result", string(result),
		"newsletter_jid", newsletterJID)
	return nil
}

// updateNewsletterViaIQ sends a newsletter update using direct IQ
func (s *NewsletterService) updateNewsletterViaIQ(client *whatsmeow.Client, ctx context.Context, newsletterJID, name string) error {
	internals := client.DangerousInternals()
	
	// Parse the newsletter JID
	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return fmt.Errorf("invalid newsletter JID: %w", err)
	}
	
	// Build update nodes
	updateNodes := []binary.Node{
		{
			Tag:     "name",
			Content: name,
		},
	}
	
	// Try different namespaces and tags
	namespaces := []string{"newsletter", "w:newsletter", "channel", "meta"}
	tags := []string{"update", "set", "modify", "edit"}
	
	for _, namespace := range namespaces {
		for _, tag := range tags {
			// Create IQ query
			query := whatsmeow.DangerousInfoQuery{
				Namespace: namespace,
				Type:      "set",
				To:        parsedJID,
				Content: []binary.Node{{
					Tag:     tag,
					Content: updateNodes,
				}},
				Context: ctx,
			}
			
			resp, err := internals.SendIQ(query)
			if err != nil {
				logger.Debug("IQ attempt failed", 
					"namespace", namespace, 
					"tag", tag, 
					"error", err,
					"newsletter_jid", newsletterJID)
				continue
			}
			
			logger.Debug("IQ attempt succeeded", 
				"namespace", namespace, 
				"tag", tag, 
				"response", resp,
				"newsletter_jid", newsletterJID)
			return nil
		}
	}
	
	return fmt.Errorf("all IQ namespace/tag combinations failed")
}

// updateNewsletterViaRawNodes sends a newsletter update using raw binary nodes
func (s *NewsletterService) updateNewsletterViaRawNodes(client *whatsmeow.Client, ctx context.Context, newsletterJID, name string) error {
	internals := client.DangerousInternals()
	
	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return fmt.Errorf("invalid newsletter JID: %w", err)
	}
	
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
					Tag:     "name",
					Content: name,
				},
			},
		}},
	}
	
	// Send raw node
	err = internals.SendNode(node)
	if err != nil {
		return fmt.Errorf("failed to send raw node: %w", err)
	}
	
	logger.Debug("Raw node sent successfully", 
		"newsletter_jid", newsletterJID,
		"name", name)
	return nil
}

// updateNewsletterDescriptionViaMex sends a newsletter description update using MEX
func (s *NewsletterService) updateNewsletterDescriptionViaMex(client *whatsmeow.Client, ctx context.Context, newsletterJID, description string) error {
	internals := client.DangerousInternals()
	
	// Prepare variables for the GraphQL query
	variables := map[string]interface{}{
		"newsletter_id": newsletterJID,
		"updates": map[string]interface{}{
			"description": description,
			"settings":    nil,
		},
	}
	
	// Use the confirmed working query ID for newsletter description updates
	queryID := mutationUpdateNewsletterQueryID
	
	result, err := internals.SendMexIQ(ctx, queryID, variables)
	if err != nil {
		return fmt.Errorf("MEX query failed for description: %w", err)
	}
	
	logger.Debug("MEX query succeeded for description", 
		"query_id", queryID, 
		"result", string(result),
		"newsletter_jid", newsletterJID)
	return nil
}

// updateNewsletterDescriptionViaIQ sends a newsletter description update using direct IQ
func (s *NewsletterService) updateNewsletterDescriptionViaIQ(client *whatsmeow.Client, ctx context.Context, newsletterJID, description string) error {
	internals := client.DangerousInternals()
	
	// Parse the newsletter JID
	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return fmt.Errorf("invalid newsletter JID: %w", err)
	}
	
	// Build update nodes
	updateNodes := []binary.Node{
		{
			Tag:     "description",
			Content: description,
		},
	}
	
	// Try different namespaces and tags
	namespaces := []string{"newsletter", "w:newsletter", "channel", "meta"}
	tags := []string{"update", "set", "modify", "edit"}
	
	for _, namespace := range namespaces {
		for _, tag := range tags {
			// Create IQ query
			query := whatsmeow.DangerousInfoQuery{
				Namespace: namespace,
				Type:      "set",
				To:        parsedJID,
				Content: []binary.Node{{
					Tag:     tag,
					Content: updateNodes,
				}},
				Context: ctx,
			}
			
			resp, err := internals.SendIQ(query)
			if err != nil {
				logger.Debug("IQ attempt failed for description", 
					"namespace", namespace, 
					"tag", tag, 
					"error", err,
					"newsletter_jid", newsletterJID)
				continue
			}
			
			logger.Debug("IQ attempt succeeded for description", 
				"namespace", namespace, 
				"tag", tag, 
				"response", resp,
				"newsletter_jid", newsletterJID)
			return nil
		}
	}
	
	return fmt.Errorf("all IQ namespace/tag combinations failed for description")
}

// updateNewsletterDescriptionViaRawNodes sends a newsletter description update using raw binary nodes
func (s *NewsletterService) updateNewsletterDescriptionViaRawNodes(client *whatsmeow.Client, ctx context.Context, newsletterJID, description string) error {
	internals := client.DangerousInternals()
	
	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return fmt.Errorf("invalid newsletter JID: %w", err)
	}
	
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
					Tag:     "description",
					Content: description,
				},
			},
		}},
	}
	
	// Send raw node
	err = internals.SendNode(node)
	if err != nil {
		return fmt.Errorf("failed to send raw node for description: %w", err)
	}
	
	logger.Debug("Raw node sent successfully for description", 
		"newsletter_jid", newsletterJID,
		"description", description)
	return nil
}

// updateNewsletterViaRawNodesUnified sends a newsletter update using raw binary nodes
// This unified function accepts both name and description parameters and can update either or both
func (s *NewsletterService) updateNewsletterViaRawNodesUnified(client *whatsmeow.Client, ctx context.Context, newsletterJID, name, description string) error {
	internals := client.DangerousInternals()
	
	parsedJID, err := types.ParseJID(newsletterJID)
	if err != nil {
		return fmt.Errorf("invalid newsletter JID: %w", err)
	}
	
	// Build content nodes based on what's provided
	var contentNodes []binary.Node
	
	if name != "" {
		contentNodes = append(contentNodes, binary.Node{
			Tag:     "name",
			Content: name,
		})
	}
	
	if description != "" {
		contentNodes = append(contentNodes, binary.Node{
			Tag:     "description",
			Content: description,
		})
	}
	
	if len(contentNodes) == 0 {
		return fmt.Errorf("neither name nor description provided")
	}
	
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
			Tag:     "update",
			Content: contentNodes,
		}},
	}
	
	// Send raw node
	err = internals.SendNode(node)
	if err != nil {
		return fmt.Errorf("failed to send raw node: %w", err)
	}
	
	logger.Debug("Raw node sent successfully", 
		"newsletter_jid", newsletterJID,
		"name", name,
		"description", description,
		"content_nodes_count", len(contentNodes))
	return nil
}

// updateNewsletterPictureViaMex sends a newsletter picture update using MEX
func (s *NewsletterService) updateNewsletterPictureViaMex(client *whatsmeow.Client, ctx context.Context, newsletterJID string, imageData []byte) error {
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
	
	queryID := mutationUpdateNewsletterQueryID
	
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
func (s *NewsletterService) updateNewsletterPictureViaRawNodes(client *whatsmeow.Client, ctx context.Context, newsletterJID string, imageData []byte) error {
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

func (s *NewsletterService) CreateChannel(userID, name, description, pictureURL string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Download picture if URL is provided
	var picture []byte
	if pictureURL != "" {
		httpClient := &http.Client{
			Timeout: 30 * time.Second,
		}
		resp, err := httpClient.Get(pictureURL)
		if err != nil {
			return nil, fmt.Errorf("falha ao baixar imagem da URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("falha ao baixar imagem da URL: status %d", resp.StatusCode)
		}

		picture, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("falha ao ler conteúdo da resposta: %w", err)
		}
	}

	// Primeiro, aceitar os Termos de Serviço para criar newsletters
	if err := client.AcceptTOSNotice("20601218", "5"); err != nil {
		logger.Error("Falha ao aceitar Termos de Serviço para newsletters", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao aceitar termos de serviço: %w", err)
	}

	// Então criar o newsletter/canal
	result, err := client.CreateNewsletter(whatsmeow.CreateNewsletterParams{
		Name:        name,
		Description: description,
		Picture:     picture,
	})

	if err != nil {
		logger.Error("Falha ao criar canal", "error", err, "user_id", userID, "name", name)
		return nil, fmt.Errorf("falha ao criar canal: %w", err)
	}

	return result, nil
}

// GetChannelInfo obtém informações sobre um canal específico
func (s *NewsletterService) GetChannelInfo(userID, jid string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("JID inválido: %w", err)
	}

	info, err := client.GetNewsletterInfo(parsedJID)
	if err != nil {
		logger.Error("Falha ao obter informações do canal", "error", err, "user_id", userID, "jid", jid)
		return nil, fmt.Errorf("falha ao obter informações do canal: %w", err)
	}

	return info, nil
}

// GetChannelWithInvite obtém informações do canal usando um link de convite
func (s *NewsletterService) GetChannelWithInvite(userID, inviteLink string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	result, err := client.GetNewsletterInfoWithInvite(inviteLink)
	if err != nil {
		logger.Error("Falha ao obter canal com convite", "error", err, "user_id", userID, "invite", inviteLink)
		return nil, fmt.Errorf("falha ao obter canal com convite: %w", err)
	}

	return result, nil
}

// ListMyChannels retorna todos os canais que o usuário está inscrito
func (s *NewsletterService) ListMyChannels(userID string) (interface{}, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return nil, err
	}

	// Get subscribed newsletters
	subscribed, err := client.GetSubscribedNewsletters()
	if err != nil {
		logger.Error("Falha ao listar canais inscritos", "error", err, "user_id", userID)
		return nil, fmt.Errorf("falha ao listar canais: %w", err)
	}

	return subscribed, nil
}

// FollowChannel inscreve o usuário em um canal
func (s *NewsletterService) FollowChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.FollowNewsletter(parsedJID); err != nil {
		logger.Error("Falha ao seguir canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao seguir canal: %w", err)
	}

	return nil
}

// UnfollowChannel cancela a inscrição do usuário em um canal
func (s *NewsletterService) UnfollowChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.UnfollowNewsletter(parsedJID); err != nil {
		logger.Error("Falha ao deixar de seguir canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao deixar de seguir canal: %w", err)
	}

	return nil
}

// MuteChannel silencia notificações de um canal
func (s *NewsletterService) MuteChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.NewsletterToggleMute(parsedJID, true); err != nil {
		logger.Error("Falha ao silenciar canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao silenciar canal: %w", err)
	}

	return nil
}

// UnmuteChannel reativa notificações de um canal
func (s *NewsletterService) UnmuteChannel(userID, jid string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	if err := client.NewsletterToggleMute(parsedJID, false); err != nil {
		logger.Error("Falha ao reativar notificações do canal", "error", err, "user_id", userID, "jid", jid)
		return fmt.Errorf("falha ao reativar notificações do canal: %w", err)
	}

	return nil
}

// UpdateNewsletterPictureFromURL updates the newsletter picture from a URL
func (s *NewsletterService) UpdateNewsletterPictureFromURL(userID, jid, imageURL string) (string, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return "", err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("JID inválido: %w", err)
	}

	logger.Debug("Download da imagem iniciado para newsletter",
		"user_id", userID,
		"newsletter_jid", jid,
		"image_url", imageURL)

	// Download the image from URL
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("falha ao baixar imagem da URL: %w", err)
	}
	defer resp.Body.Close()

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

	logger.Debug("Baixando imagem para atualizar foto da newsletter",
		"user_id", userID,
		"newsletter_jid", jid,
		"image_url", imageURL,
		"size_bytes", len(imageData),
		"content_type", contentType)

	// Convert image to JPEG format (WhatsApp only supports JPEG for newsletter pictures)
	jpegData, err := convertToJPEG(imageData, 85) // Quality 85 provides good balance between quality and file size
	if err != nil {
		return "", fmt.Errorf("falha ao converter imagem para JPEG: %w", err)
	}

	// Validate JPEG magic bytes (WhatsApp requires strict JPEG validation)
	if len(jpegData) < 2 || jpegData[0] != 0xFF || jpegData[1] != 0xD8 {
		logger.Error("Dados JPEG inválidos - magic bytes incorretos",
			"user_id", userID,
			"newsletter_jid", jid,
			"data_length", len(jpegData),
			"first_bytes", fmt.Sprintf("%X", jpegData[:min(10, len(jpegData))]))
		return "", fmt.Errorf("dados JPEG inválidos: magic bytes incorretos")
	}

	logger.Debug("Imagem convertida para JPEG",
		"user_id", userID,
		"newsletter_jid", jid,
		"original_size", len(imageData),
		"jpeg_size", len(jpegData),
		"jpeg_magic_bytes", fmt.Sprintf("%X %X", jpegData[0], jpegData[1]))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Tentando atualizar foto da newsletter usando múltiplas abordagens",
		"user_id", userID,
		"newsletter_jid", jid,
		"image_size", len(jpegData))

	// Try different methods in order of probability of success
	methods := []struct {
		name string
		fn   func(*whatsmeow.Client, context.Context, string, []byte) error
	}{
		{"MEX GraphQL", s.updateNewsletterPictureViaMex},
		{"Raw Nodes", s.updateNewsletterPictureViaRawNodes},
	}

	for _, method := range methods {
		logger.Debug("Tentando método para picture", "method", method.name, "newsletter_jid", jid)
		err := method.fn(client, ctx, parsedJID.String(), jpegData)
		
		if err == nil {
			logger.Debug("Foto da newsletter atualizada com sucesso",
				"method", method.name,
				"user_id", userID,
				"newsletter_jid", jid,
				"image_size", len(jpegData))
			return "picture_updated", nil
		}
		
		logger.Debug("Método falhou, tentando próximo",
			"method", method.name,
			"error", err,
			"newsletter_jid", jid)
	}

	// If MEX and raw nodes fail, try the traditional extension method as fallback
	logger.Debug("Métodos MEX e Raw Nodes falharam, tentando método de extensão",
		"user_id", userID,
		"newsletter_jid", jid)

	// Set the newsletter photo using custom extension
	pictureID, err := extensions.SetNewsletterPhoto(client, parsedJID, jpegData)
	if err != nil {
		logger.Error("Todos os métodos falharam ao atualizar foto da newsletter",
			"error", err,
			"user_id", userID,
			"newsletter_jid", jid)
		return "", fmt.Errorf("todos os métodos falharam ao atualizar foto da newsletter: %w", err)
	}

	// Log success
	logger.Debug("Foto da newsletter atualizada com sucesso usando método de extensão",
		"user_id", userID,
		"newsletter_jid", jid,
		"picture_id", pictureID)

	return pictureID, nil
}

// UpdateNewsletterName updates the newsletter name using multiple approaches
func (s *NewsletterService) UpdateNewsletterName(userID, jid, name string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Tentando atualizar nome da newsletter usando múltiplas abordagens",
		"user_id", userID,
		"newsletter_jid", jid,
		"name", name)

	// Try different methods in order of probability of success
	methods := []struct {
		name string
		fn   func(*whatsmeow.Client, context.Context, string, string) error
	}{
		{"MEX GraphQL", s.updateNewsletterViaMex},
		{"IQ Direct", s.updateNewsletterViaIQ},
		{"Raw Nodes", s.updateNewsletterViaRawNodes},
	}

	for _, method := range methods {
		logger.Debug("Tentando método", "method", method.name, "newsletter_jid", jid)
		err := method.fn(client, ctx, parsedJID.String(), name)
		
		if err == nil {
			logger.Debug("Nome da newsletter atualizado com sucesso",
				"method", method.name,
				"user_id", userID,
				"newsletter_jid", jid,
				"name", name)
			return nil
		}
		
		logger.Debug("Método falhou, tentando próximo",
			"method", method.name,
			"error", err,
			"newsletter_jid", jid)
	}

	// If all dangerous methods fail, log the failure
	logger.Error("Todos os métodos falharam ao atualizar nome da newsletter",
		"user_id", userID,
		"newsletter_jid", jid,
		"name", name)

	return fmt.Errorf("todos os métodos falharam ao atualizar nome da newsletter")
}

// UpdateNewsletterDescription updates the newsletter description using multiple approaches
func (s *NewsletterService) UpdateNewsletterDescription(userID, jid, description string) error {
	client, err := s.getClient(userID)
	if err != nil {
		return err
	}

	// Parse JID
	parsedJID, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger.Debug("Tentando atualizar descrição da newsletter usando múltiplas abordagens",
		"user_id", userID,
		"newsletter_jid", jid,
		"description", description)

	// Try different methods in order of probability of success
	methods := []struct {
		name string
		fn   func(*whatsmeow.Client, context.Context, string, string) error
	}{
		{"MEX GraphQL", s.updateNewsletterDescriptionViaMex},
		{"IQ Direct", s.updateNewsletterDescriptionViaIQ},
		{"Raw Nodes", s.updateNewsletterDescriptionViaRawNodes},
	}

	for _, method := range methods {
		logger.Debug("Tentando método para descrição", "method", method.name, "newsletter_jid", jid)
		err := method.fn(client, ctx, parsedJID.String(), description)
		
		if err == nil {
			logger.Debug("Descrição da newsletter atualizada com sucesso",
				"method", method.name,
				"user_id", userID,
				"newsletter_jid", jid,
				"description", description)
			return nil
		}
		
		logger.Debug("Método falhou para descrição, tentando próximo",
			"method", method.name,
			"error", err,
			"newsletter_jid", jid)
	}

	// If all dangerous methods fail, log the failure
	logger.Error("Todos os métodos falharam ao atualizar descrição da newsletter",
		"user_id", userID,
		"newsletter_jid", jid,
		"description", description)

	return fmt.Errorf("todos os métodos falharam ao atualizar descrição da newsletter")
}

// validateAndProcessParticipantNumber validates and processes participant phone numbers
func (s *NewsletterService) validateAndProcessParticipantNumber(userID, phoneNumber string) (string, error) {
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
	if s.isPhoneNumber(phoneNumber) {
		return s.validateAndProcessPhoneNumber(userID, phoneNumber)
	}

	// Default: treat as phone number
	return s.validateAndProcessPhoneNumber(userID, phoneNumber)
}

// isPhoneNumber checks if the input looks like a phone number
func (s *NewsletterService) isPhoneNumber(input string) bool {
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
func (s *NewsletterService) validateAndProcessPhoneNumber(userID, phoneNumber string) (string, error) {
	// Clean the phone number
	cleaned := s.cleanPhoneNumber(phoneNumber)

	// Handle Brazilian numbers with the 9-digit rule
	processed := s.processBrazilianNumber(cleaned)

	// Check if the number exists on WhatsApp and get the correct JID
	jid, exists, err := s.checkNumberExistsOnWhatsAppAndGetJID(userID, processed)
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
				alternative := s.removeNinthDigitFromBrazilian(processed)
				if alternative != processed {
					alternatives = append(alternatives, alternative)
				}
			} else if len(processed) == 12 {
				// 12 digits: try adding the 9
				alternative := s.addNinthDigitToBrazilian(processed)
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
				altJID, altExists, altErr := s.checkNumberExistsOnWhatsAppAndGetJID(userID, alt)
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
func (s *NewsletterService) cleanPhoneNumber(phone string) string {
	// Remove common formatting characters
	cleaned := strings.ReplaceAll(phone, "+", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")
	cleaned = strings.ReplaceAll(cleaned, ".", "")

	return cleaned
}

// processBrazilianNumber handles Brazilian number formatting
func (s *NewsletterService) processBrazilianNumber(number string) string {
	// If it doesn't start with country code, check if it's a Brazilian local number
	if !strings.HasPrefix(number, "55") && len(number) >= 10 && len(number) <= 11 {
		// Check if it looks like a Brazilian local number by area code
		if s.isBrazilianAreaCode(number) {
			// Add Brazilian country code
			if len(number) == 11 || len(number) == 10 {
				number = "55" + number
			}
		}
	}

	return number
}

// removeNinthDigitFromBrazilian removes the 9th digit from Brazilian mobile numbers
func (s *NewsletterService) removeNinthDigitFromBrazilian(number string) string {
	// Must start with 55 and have at least 13 digits
	if !strings.HasPrefix(number, "55") || len(number) < 13 {
		return number
	}

	// Get the part after country code (55)
	withoutCountryCode := number[2:]

	// Must have at least 11 digits and the third digit must be 9
	if len(withoutCountryCode) >= 11 && withoutCountryCode[2] == '9' {
		// Check if it's a valid area code
		areaCode := withoutCountryCode[:2]
		if s.isBrazilianAreaCode("55" + areaCode + "1234567890") {
			// Remove the 9 from the third position
			return "55" + areaCode + withoutCountryCode[3:]
		}
	}

	return number
}

// addNinthDigitToBrazilian adds the 9th digit to Brazilian mobile numbers
func (s *NewsletterService) addNinthDigitToBrazilian(number string) string {
	// Must start with 55 and have at least 12 digits
	if !strings.HasPrefix(number, "55") || len(number) < 12 {
		return number
	}

	// Get the part after country code (55)
	withoutCountryCode := number[2:]

	// Must have exactly 10 digits and the third digit must not be 9
	if len(withoutCountryCode) == 10 && withoutCountryCode[2] != '9' {
		// Check if it's a valid area code
		areaCode := withoutCountryCode[:2]
		if s.isBrazilianAreaCode("55" + areaCode + "1234567890") {
			// Add 9 after the area code
			return "55" + areaCode + "9" + withoutCountryCode[2:]
		}
	}

	return number
}

// checkNumberExistsOnWhatsAppAndGetJID verifies if a number exists on WhatsApp and returns the correct JID
func (s *NewsletterService) checkNumberExistsOnWhatsAppAndGetJID(userID, number string) (string, bool, error) {
	client, err := s.getClient(userID)
	if err != nil {
		return "", false, err
	}

	// Use IsOnWhatsApp to check existence
	jids := []string{number}
	responses, err := client.IsOnWhatsApp(jids)
	if err != nil {
		return "", false, fmt.Errorf("falha ao verificar número no WhatsApp: %w", err)
	}

	if len(responses) == 0 {
		return "", false, fmt.Errorf("nenhuma resposta recebida do WhatsApp")
	}

	response := responses[0]
	logger.Debug("IsOnWhatsApp response", "query", response.Query, "jid", response.JID.String(), "is_in", response.IsIn)

	if response.IsIn {
		return response.JID.String(), true, nil
	}

	return "", false, nil
}

// checkNumberExistsOnWhatsApp verifies if a number exists on WhatsApp (legacy function for compatibility)
func (s *NewsletterService) checkNumberExistsOnWhatsApp(userID, number string) (bool, error) {
	_, exists, err := s.checkNumberExistsOnWhatsAppAndGetJID(userID, number)
	return exists, err
}

// isBrazilianAreaCode checks if a number starts with a valid Brazilian area code
func (s *NewsletterService) isBrazilianAreaCode(number string) bool {
	// Brazilian area codes (2 digits after country code 55)
	validAreaCodes := []string{
		"11", "12", "13", "14", "15", "16", "17", "18", "19", // São Paulo
		"21", "22", "24",                                     // Rio de Janeiro
		"27", "28",                                           // Espírito Santo
		"31", "32", "33", "34", "35", "37", "38",             // Minas Gerais
		"41", "42", "43", "44", "45", "46",                   // Paraná
		"47", "48", "49",                                     // Santa Catarina
		"51", "53", "54", "55",                               // Rio Grande do Sul
		"61",                                                 // Distrito Federal
		"62", "64",                                           // Goiás
		"63",                                                 // Tocantins
		"65", "66",                                           // Mato Grosso
		"67",                                                 // Mato Grosso do Sul
		"68",                                                 // Acre
		"69",                                                 // Rondônia
		"71", "73", "74", "75", "77",                         // Bahia
		"79",                                                 // Sergipe
		"81", "87",                                           // Pernambuco
		"82",                                                 // Alagoas
		"83",                                                 // Paraíba
		"84",                                                 // Rio Grande do Norte
		"85", "88",                                           // Ceará
		"86", "89",                                           // Piauí
		"91", "93", "94",                                     // Pará
		"92", "97",                                           // Amazonas
		"95",                                                 // Roraima
		"96",                                                 // Amapá
		"98", "99",                                           // Maranhão
	}

	// Remove country code if present
	if strings.HasPrefix(number, "55") && len(number) > 2 {
		number = number[2:]
	}

	// Check if starts with valid area code
	if len(number) >= 2 {
		areaCode := number[:2]
		for _, valid := range validAreaCodes {
			if areaCode == valid {
				return true
			}
		}
	}

	return false
}
