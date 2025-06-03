package worker

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"yourproject/pkg/logger"
)

// NewWorker cria um novo worker
func NewWorker(id, userID string, workerType WorkerType, sessionMgr SessionManager, coord Coordinator, communitySvc CommunityServiceInterface, groupSvc GroupServiceInterface, messageSvc MessageServiceInterface, newsletterSvc NewsletterServiceInterface, config *WorkerConfig) *Worker {
	return &Worker{
		ID:                id,
		Type:              workerType,
		UserID:            userID,
		Priority:          0,
		status:            StatusStopped,
		metrics:           &WorkerMetrics{StartTime: time.Now()},
		taskQueue:         make(chan Task, config.TaskQueueSize),
		eventQueue:        make(chan interface{}, config.EventQueueSize),
		done:              make(chan struct{}),
		sessionManager:    sessionMgr,
		coordinator:       coord,
		communityService:  communitySvc,
		groupService:      groupSvc,
		messageService:    messageSvc,
		newsletterService: newsletterSvc,
		config:            config,
	}
}

// Start inicia o worker
func (w *Worker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if atomic.LoadInt32(&w.isRunning) == 1 {
		return fmt.Errorf("worker %s já está rodando", w.ID)
	}

	w.status = StatusIdle
	atomic.StoreInt32(&w.isRunning, 1)

	// Iniciar goroutines
	w.wg.Add(2)
	go w.taskProcessor()
	go w.eventProcessor()

	logger.Info("Worker iniciado", "worker_id", w.ID, "user_id", w.UserID, "type", w.Type)
	w.coordinator.NotifyWorkerStatus(w.ID, w.UserID, w.status)

	return nil
}

// Stop para o worker gracefully
func (w *Worker) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if atomic.LoadInt32(&w.isRunning) == 0 {
		return nil // Já parado
	}

	w.status = StatusStopping
	logger.Info("Parando worker", "worker_id", w.ID, "user_id", w.UserID)

	// Sinalizar para parar
	close(w.done)
	atomic.StoreInt32(&w.isRunning, 0)

	// Aguardar conclusão com timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("Worker parado com sucesso", "worker_id", w.ID, "user_id", w.UserID)
	case <-time.After(w.config.WorkerTimeout):
		logger.Warn("Timeout ao parar worker", "worker_id", w.ID, "user_id", w.UserID)
	}

	w.status = StatusStopped
	w.coordinator.NotifyWorkerStatus(w.ID, w.UserID, w.status)

	return nil
}

// IsRunning retorna se o worker está rodando
func (w *Worker) IsRunning() bool {
	return atomic.LoadInt32(&w.isRunning) == 1
}

// GetStatus retorna o status atual do worker
func (w *Worker) GetStatus() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status
}

// GetMetrics retorna as métricas do worker
func (w *Worker) GetMetrics() WorkerMetrics {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return *w.metrics
}

// SendTask envia uma tarefa para o worker
func (w *Worker) SendTask(task Task) error {
	if !w.IsRunning() {
		return fmt.Errorf("worker %s não está rodando", w.ID)
	}

	select {
	case w.taskQueue <- task:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout ao enviar tarefa para worker %s", w.ID)
	}
}

// SendEvent envia um evento para o worker
func (w *Worker) SendEvent(evt interface{}) {
	if !w.IsRunning() {
		return
	}

	select {
	case w.eventQueue <- evt:
	default:
		logger.Warn("Evento descartado, buffer cheio", "worker_id", w.ID, "user_id", w.UserID)
	}
}

// taskProcessor processa tarefas em sua própria goroutine
func (w *Worker) taskProcessor() {
	defer w.wg.Done()

	for {
		select {
		case <-w.done:
			return

		case task := <-w.taskQueue:
			w.processTask(task)
		}
	}
}

// eventProcessor processa eventos em sua própria goroutine
func (w *Worker) eventProcessor() {
	defer w.wg.Done()

	for {
		select {
		case <-w.done:
			return

		case evt := <-w.eventQueue:
			w.processEvent(evt)
		}
	}
}

// processTask processa uma tarefa individual
func (w *Worker) processTask(task Task) {
	startTime := time.Now()

	w.mu.Lock()
	w.status = StatusBusy
	w.mu.Unlock()

	defer func() {
		duration := time.Since(startTime)

		w.mu.Lock()
		w.status = StatusIdle
		w.metrics.LastTaskTime = time.Now()
		w.metrics.TasksProcessed++

		// Calcular média de tempo de tarefa
		if w.metrics.TasksProcessed == 1 {
			w.metrics.AverageTaskTime = duration
		} else {
			w.metrics.AverageTaskTime = (w.metrics.AverageTaskTime + duration) / 2
		}
		w.mu.Unlock()
	}()

	var response CommandResponse
	response.CommandID = task.ID

	switch task.Type {
	case CmdSendText:
		response = w.handleSendText(task.Payload.(SendTextPayload))
	case CmdSendMedia:
		response = w.handleSendMedia(task.Payload.(SendMediaPayload))
	case CmdSendButtons:
		response = w.handleSendButtons(task.Payload.(SendButtonsPayload))
	case CmdSendList:
		response = w.handleSendList(task.Payload.(SendListPayload))
	case CmdCheckNumber:
		response = w.handleCheckNumber(task.Payload.(CheckNumberPayload))
	case CmdConnect:
		response = w.handleConnect()
	case CmdDisconnect:
		response = w.handleDisconnect()
	case CmdGetStatus:
		response = w.handleGetStatus()
	case CmdGetQR:
		response = w.handleGetQR(context.Background())
	case CmdLogout:
		response = w.handleLogout(context.Background())

		// Community commands
	case CmdCreateCommunity:
		response = w.handleCreateCommunity(task.Payload.(CreateCommunityPayload))
	case CmdGetCommunityInfo:
		response = w.handleGetCommunityInfo(task.Payload.(CommunityInfoPayload))
	case CmdUpdateCommunityName:
		response = w.handleUpdateCommunityName(task.Payload.(UpdateCommunityNamePayload))
	case CmdUpdateCommunityDescription:
		response = w.handleUpdateCommunityDescription(task.Payload.(UpdateCommunityDescriptionPayload))
	case CmdUpdateCommunityPicture:
		response = w.handleUpdateCommunityPicture(task.Payload.(UpdateCommunityPicturePayload))
	case CmdLeaveCommunity:
		response = w.handleLeaveCommunity(task.Payload.(LeaveCommunityPayload))
	case CmdGetJoinedCommunities:
		response = w.handleGetJoinedCommunities()
	case CmdCreateGroupForCommunity:
		response = w.handleCreateGroupForCommunity(task.Payload.(CreateGroupForCommunityPayload))
	case CmdLinkGroupToCommunity:
		response = w.handleLinkGroupToCommunity(task.Payload.(LinkGroupPayload))
	case CmdUnlinkGroupFromCommunity:
		response = w.handleUnlinkGroupFromCommunity(task.Payload.(LinkGroupPayload))
	case CmdGetCommunityInviteLink:
		response = w.handleGetCommunityInviteLink(task.Payload.(GetCommunityInviteLinkPayload))
	case CmdRevokeCommunityInviteLink:
		response = w.handleRevokeCommunityInviteLink(task.Payload.(GetCommunityInviteLinkPayload))
	case CmdGetCommunityLinkedGroups:
		response = w.handleGetCommunityLinkedGroups(task.Payload.(GetCommunityLinkedGroupsPayload))
	case CmdJoinCommunityWithLink:
		response = w.handleJoinCommunityWithLink(task.Payload.(JoinCommunityWithLinkPayload))

		// Group commands
	case CmdCreateGroup:
		response = w.handleCreateGroup(task.Payload.(CreateGroupPayload))
	case CmdGetGroupInfo:
		response = w.handleGetGroupInfo(task.Payload.(GroupInfoPayload))
	case CmdGetJoinedGroups:
		response = w.handleGetJoinedGroups()
	case CmdAddGroupParticipants:
		response = w.handleAddGroupParticipants(task.Payload.(GroupParticipantsPayload))
	case CmdRemoveGroupParticipants:
		response = w.handleRemoveGroupParticipants(task.Payload.(GroupParticipantsPayload))
	case CmdPromoteGroupParticipants:
		response = w.handlePromoteGroupParticipants(task.Payload.(GroupParticipantsPayload))
	case CmdDemoteGroupParticipants:
		response = w.handleDemoteGroupParticipants(task.Payload.(GroupParticipantsPayload))
	case CmdUpdateGroupName:
		response = w.handleUpdateGroupName(task.Payload.(UpdateGroupNamePayload))
	case CmdUpdateGroupTopic:
		response = w.handleUpdateGroupTopic(task.Payload.(UpdateGroupTopicPayload))
	case CmdUpdateGroupPicture:
		response = w.handleUpdateGroupPicture(task.Payload.(UpdateGroupPicturePayload))
	case CmdLeaveGroup:
		response = w.handleLeaveGroup(task.Payload.(LeaveGroupPayload))
	case CmdJoinGroupWithLink:
		response = w.handleJoinGroupWithLink(task.Payload.(JoinGroupWithLinkPayload))
	case CmdGetGroupInviteLink:
		response = w.handleGetGroupInviteLink(task.Payload.(GroupInviteLinkPayload))
	case CmdRevokeGroupInviteLink:
		response = w.handleRevokeGroupInviteLink(task.Payload.(GroupInviteLinkPayload))
	case CmdSetGroupLocked:
		response = w.handleSetGroupLocked(task.Payload.(SetGroupLockedPayload))
	case CmdSetGroupAnnounce:
		response = w.handleSetGroupAnnounce(task.Payload.(SetGroupAnnouncePayload))
	case CmdSetGroupJoinApprovalMode:
		response = w.handleSetGroupJoinApprovalMode(task.Payload.(SetGroupJoinApprovalModePayload))
	case CmdSetGroupMemberAddMode:
		response = w.handleSetGroupMemberAddMode(task.Payload.(SetGroupMemberAddModePayload))

		// Newsletter commands
	case CmdCreateChannel:
		response = w.handleCreateChannel(task.Payload.(CreateChannelPayload))
	case CmdGetChannelInfo:
		response = w.handleGetChannelInfo(task.Payload.(ChannelJIDPayload))
	case CmdGetChannelWithInvite:
		response = w.handleGetChannelWithInvite(task.Payload.(ChannelInvitePayload))
	case CmdListMyChannels:
		response = w.handleListMyChannels()
	case CmdFollowChannel:
		response = w.handleFollowChannel(task.Payload.(ChannelJIDPayload))
	case CmdUnfollowChannel:
		response = w.handleUnfollowChannel(task.Payload.(ChannelJIDPayload))
	case CmdMuteChannel:
		response = w.handleMuteChannel(task.Payload.(ChannelJIDPayload))
	case CmdUnmuteChannel:
		response = w.handleUnmuteChannel(task.Payload.(ChannelJIDPayload))
	case CmdUpdateNewsletterPicture:
		response = w.handleUpdateNewsletterPicture(task.Payload.(UpdateNewsletterPicturePayload))
	case CmdUpdateNewsletterName:
		response = w.handleUpdateNewsletterName(task.Payload.(UpdateNewsletterNamePayload))
	case CmdUpdateNewsletterDescription:
		response = w.handleUpdateNewsletterDescription(task.Payload.(UpdateNewsletterDescriptionPayload))

	default:
		response = CommandResponse{
			CommandID: task.ID,
			Error:     fmt.Errorf("comando não reconhecido: %s", task.Type),
		}
	}

	// Atualizar métricas
	w.mu.Lock()
	if response.Error != nil {
		w.metrics.TasksFailed++
		w.metrics.ErrorCount++
	} else {
		w.metrics.TasksSuccessful++
	}
	w.mu.Unlock()

	// Enviar resposta se solicitada
	if task.Response != nil {
		select {
		case task.Response <- response:
		case <-time.After(5 * time.Second):
			logger.Warn("Timeout ao enviar resposta", "worker_id", w.ID, "task_id", task.ID)
		}
	}
}

// processEvent processa um evento
func (w *Worker) processEvent(evt interface{}) {
	if err := w.coordinator.ProcessEvent(w.UserID, evt); err != nil {
		logger.Error("Erro ao processar evento", "worker_id", w.ID, "error", err)

		w.mu.Lock()
		w.metrics.ErrorCount++
		w.mu.Unlock()
	}
}

// Command handlers
func (w *Worker) handleSendText(payload SendTextPayload) CommandResponse {
	msgID, err := w.messageService.SendText(w.UserID, payload.To, payload.Message)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao enviar texto: %w", err)}
	}
	return CommandResponse{Data: msgID}
}

func (w *Worker) handleSendMedia(payload SendMediaPayload) CommandResponse {
	msgID, err := w.messageService.SendMedia(w.UserID, payload.To, payload.MediaURL, payload.MediaType, payload.Caption)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao enviar mídia: %w", err)}
	}
	return CommandResponse{Data: msgID}
}

func (w *Worker) handleSendButtons(payload SendButtonsPayload) CommandResponse {
	msgID, err := w.messageService.SendButtons(w.UserID, payload.To, payload.Text, payload.Footer, payload.Buttons)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao enviar botões: %w", err)}
	}
	return CommandResponse{Data: msgID}
}

func (w *Worker) handleSendList(payload SendListPayload) CommandResponse {
	msgID, err := w.messageService.SendList(w.UserID, payload.To, payload.Text, payload.Footer, payload.ButtonText, payload.Sections)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao enviar lista: %w", err)}
	}
	return CommandResponse{Data: msgID}
}

func (w *Worker) handleCheckNumber(payload CheckNumberPayload) CommandResponse {
	exists, err := w.messageService.CheckNumberExistsOnWhatsApp(w.UserID, payload.Number)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao verificar número: %w", err)}
	}
	return CommandResponse{Data: exists}
}

func (w *Worker) handleConnect() CommandResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := w.sessionManager.Connect(ctx, w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao conectar: %w", err)}
	}
	return CommandResponse{Data: "conectado"}
}

func (w *Worker) handleDisconnect() CommandResponse {
	err := w.sessionManager.Disconnect(w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao desconectar: %w", err)}
	}
	return CommandResponse{Data: "desconectado"}
}

func (w *Worker) handleGetStatus() CommandResponse {
	status, err := w.sessionManager.GetSessionStatus(w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter status: %w", err)}
	}

	// Adicionar informações do worker
	status["worker_id"] = w.ID
	status["worker_status"] = w.status.String()
	status["worker_metrics"] = w.GetMetrics()

	return CommandResponse{Data: status}
}

func (w *Worker) handleGetQR(ctx context.Context) CommandResponse {
	qrChan, err := w.sessionManager.GetQRChannel(ctx, w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter QR: %w", err)}
	}
	return CommandResponse{Data: qrChan}
}

func (w *Worker) handleLogout(ctx context.Context) CommandResponse {
	err := w.sessionManager.Logout(ctx, w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao fazer logout: %w", err)}
	}
	return CommandResponse{Data: "logout realizado"}
}

// Community command handlers
func (w *Worker) handleCreateCommunity(payload CreateCommunityPayload) CommandResponse {
	result, err := w.communityService.CreateCommunity(w.UserID, payload.Name, payload.Description)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao criar comunidade: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleGetCommunityInfo(payload CommunityInfoPayload) CommandResponse {
	result, err := w.communityService.GetCommunityInfo(w.UserID, payload.CommunityJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter informações da comunidade: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleUpdateCommunityName(payload UpdateCommunityNamePayload) CommandResponse {
	err := w.communityService.UpdateCommunityName(w.UserID, payload.CommunityJID, payload.NewName)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar nome da comunidade: %w", err)}
	}
	return CommandResponse{Data: "nome da comunidade atualizado"}
}

func (w *Worker) handleUpdateCommunityDescription(payload UpdateCommunityDescriptionPayload) CommandResponse {
	err := w.communityService.UpdateCommunityDescription(w.UserID, payload.CommunityJID, payload.NewDescription)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar descrição da comunidade: %w", err)}
	}
	return CommandResponse{Data: "descrição da comunidade atualizada"}
}

func (w *Worker) handleUpdateCommunityPicture(payload UpdateCommunityPicturePayload) CommandResponse {
	pictureID, err := w.communityService.UpdateCommunityPictureFromURL(w.UserID, payload.CommunityJID, payload.ImageURL)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar foto da comunidade: %w", err)}
	}
	return CommandResponse{Data: map[string]interface{}{
		"message":    "foto da comunidade atualizada",
		"picture_id": pictureID,
	}}
}

func (w *Worker) handleLeaveCommunity(payload LeaveCommunityPayload) CommandResponse {
	err := w.communityService.LeaveCommunity(w.UserID, payload.CommunityJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao sair da comunidade: %w", err)}
	}
	return CommandResponse{Data: "usuário saiu da comunidade"}
}

func (w *Worker) handleGetJoinedCommunities() CommandResponse {
	result, err := w.communityService.GetJoinedCommunities(w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter comunidades do usuário: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleCreateGroupForCommunity(payload CreateGroupForCommunityPayload) CommandResponse {
	result, err := w.communityService.CreateGroupForCommunity(w.UserID, payload.CommunityJID, payload.GroupName, payload.Participants)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao criar grupo para a comunidade: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleLinkGroupToCommunity(payload LinkGroupPayload) CommandResponse {
	err := w.communityService.LinkGroupToCommunity(w.UserID, payload.CommunityJID, payload.GroupJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao vincular grupo à comunidade: %w", err)}
	}
	return CommandResponse{Data: "grupo vinculado à comunidade"}
}

func (w *Worker) handleUnlinkGroupFromCommunity(payload LinkGroupPayload) CommandResponse {
	err := w.communityService.UnlinkGroupFromCommunity(w.UserID, payload.CommunityJID, payload.GroupJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao desvincular grupo da comunidade: %w", err)}
	}
	return CommandResponse{Data: "grupo desvinculado da comunidade"}
}

func (w *Worker) handleGetCommunityInviteLink(payload GetCommunityInviteLinkPayload) CommandResponse {
	link, err := w.communityService.GetCommunityInviteLink(w.UserID, payload.CommunityJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter link de convite da comunidade: %w", err)}
	}
	return CommandResponse{Data: link}
}

func (w *Worker) handleRevokeCommunityInviteLink(payload GetCommunityInviteLinkPayload) CommandResponse {
	link, err := w.communityService.RevokeCommunityInviteLink(w.UserID, payload.CommunityJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao revogar link de convite da comunidade: %w", err)}
	}
	return CommandResponse{Data: link}
}

func (w *Worker) handleGetCommunityLinkedGroups(payload GetCommunityLinkedGroupsPayload) CommandResponse {
	result, err := w.communityService.GetCommunityLinkedGroups(w.UserID, payload.CommunityJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter grupos vinculados da comunidade: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleJoinCommunityWithLink(payload JoinCommunityWithLinkPayload) CommandResponse {
	result, err := w.communityService.JoinCommunityWithLink(w.UserID, payload.Link)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao entrar na comunidade com o link: %w", err)}
	}
	return CommandResponse{Data: result}
}

// Group command handlers - now using groupService directly like communities
func (w *Worker) handleCreateGroup(payload CreateGroupPayload) CommandResponse {
	result, err := w.groupService.CreateGroup(w.UserID, payload.Name, payload.Participants)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao criar grupo: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleGetGroupInfo(payload GroupInfoPayload) CommandResponse {
	result, err := w.groupService.GetGroupInfo(w.UserID, payload.GroupJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter informações do grupo: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleGetJoinedGroups() CommandResponse {
	result, err := w.groupService.GetJoinedGroups(w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter grupos do usuário: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleAddGroupParticipants(payload GroupParticipantsPayload) CommandResponse {
	err := w.groupService.AddGroupParticipants(w.UserID, payload.GroupJID, payload.Participants)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao adicionar participantes ao grupo: %w", err)}
	}
	return CommandResponse{Data: "participantes adicionados ao grupo"}
}

func (w *Worker) handleRemoveGroupParticipants(payload GroupParticipantsPayload) CommandResponse {
	err := w.groupService.RemoveGroupParticipants(w.UserID, payload.GroupJID, payload.Participants)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao remover participantes do grupo: %w", err)}
	}
	return CommandResponse{Data: "participantes removidos do grupo"}
}

func (w *Worker) handlePromoteGroupParticipants(payload GroupParticipantsPayload) CommandResponse {
	err := w.groupService.PromoteGroupParticipants(w.UserID, payload.GroupJID, payload.Participants)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao promover participantes do grupo: %w", err)}
	}
	return CommandResponse{Data: "participantes promovidos no grupo"}
}

func (w *Worker) handleDemoteGroupParticipants(payload GroupParticipantsPayload) CommandResponse {
	err := w.groupService.DemoteGroupParticipants(w.UserID, payload.GroupJID, payload.Participants)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao rebaixar participantes do grupo: %w", err)}
	}
	return CommandResponse{Data: "participantes rebaixados no grupo"}
}

func (w *Worker) handleUpdateGroupName(payload UpdateGroupNamePayload) CommandResponse {
	err := w.groupService.UpdateGroupName(w.UserID, payload.GroupJID, payload.NewName)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar nome do grupo: %w", err)}
	}
	return CommandResponse{Data: "nome do grupo atualizado"}
}

func (w *Worker) handleUpdateGroupTopic(payload UpdateGroupTopicPayload) CommandResponse {
	err := w.groupService.UpdateGroupTopic(w.UserID, payload.GroupJID, payload.NewTopic)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar tópico do grupo: %w", err)}
	}
	return CommandResponse{Data: "tópico do grupo atualizado"}
}

func (w *Worker) handleUpdateGroupPicture(payload UpdateGroupPicturePayload) CommandResponse {
	pictureID, err := w.groupService.UpdateGroupPictureFromURL(w.UserID, payload.GroupJID, payload.ImageURL)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar foto do grupo: %w", err)}
	}
	return CommandResponse{Data: map[string]interface{}{
		"message":    "foto do grupo atualizada",
		"picture_id": pictureID,
	}}
}

func (w *Worker) handleLeaveGroup(payload LeaveGroupPayload) CommandResponse {
	err := w.groupService.LeaveGroup(w.UserID, payload.GroupJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao sair do grupo: %w", err)}
	}
	return CommandResponse{Data: "usuário saiu do grupo"}
}

func (w *Worker) handleJoinGroupWithLink(payload JoinGroupWithLinkPayload) CommandResponse {
	result, err := w.groupService.JoinGroupWithLink(w.UserID, payload.Link)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao entrar no grupo com o link: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleGetGroupInviteLink(payload GroupInviteLinkPayload) CommandResponse {
	link, err := w.groupService.GetGroupInviteLink(w.UserID, payload.GroupJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter link de convite do grupo: %w", err)}
	}
	return CommandResponse{Data: link}
}

func (w *Worker) handleRevokeGroupInviteLink(payload GroupInviteLinkPayload) CommandResponse {
	link, err := w.groupService.RevokeGroupInviteLink(w.UserID, payload.GroupJID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao revogar link de convite do grupo: %w", err)}
	}
	return CommandResponse{Data: link}
}

// Group permission handlers
func (w *Worker) handleSetGroupLocked(payload SetGroupLockedPayload) CommandResponse {
	err := w.groupService.SetGroupLocked(w.UserID, payload.GroupJID, payload.Locked)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao alterar status de bloqueio do grupo: %w", err)}
	}
	return CommandResponse{Data: "status de bloqueio do grupo alterado"}
}

func (w *Worker) handleSetGroupAnnounce(payload SetGroupAnnouncePayload) CommandResponse {
	err := w.groupService.SetGroupAnnounce(w.UserID, payload.GroupJID, payload.Announce)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao alterar modo de anúncio do grupo: %w", err)}
	}
	return CommandResponse{Data: "modo de anúncio do grupo alterado"}
}

func (w *Worker) handleSetGroupJoinApprovalMode(payload SetGroupJoinApprovalModePayload) CommandResponse {
	err := w.groupService.SetGroupJoinApprovalMode(w.UserID, payload.GroupJID, payload.Mode)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao alterar modo de aprovação de entrada do grupo: %w", err)}
	}
	return CommandResponse{Data: "modo de aprovação de entrada do grupo alterado"}
}

func (w *Worker) handleSetGroupMemberAddMode(payload SetGroupMemberAddModePayload) CommandResponse {
	err := w.groupService.SetGroupMemberAddMode(w.UserID, payload.GroupJID, payload.Mode)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao alterar modo de adição de membros do grupo: %w", err)}
	}
	return CommandResponse{Data: "modo de adição de membros do grupo alterado"}
}

// Newsletter command handlers - now using newsletterService directly like communities and groups
func (w *Worker) handleCreateChannel(payload CreateChannelPayload) CommandResponse {
	result, err := w.newsletterService.CreateChannel(w.UserID, payload.Name, payload.Description, payload.PictureURL)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao criar canal: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleGetChannelInfo(payload ChannelJIDPayload) CommandResponse {
	result, err := w.newsletterService.GetChannelInfo(w.UserID, payload.JID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter informações do canal: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleGetChannelWithInvite(payload ChannelInvitePayload) CommandResponse {
	result, err := w.newsletterService.GetChannelWithInvite(w.UserID, payload.InviteLink)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao obter canal com convite: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleListMyChannels() CommandResponse {
	result, err := w.newsletterService.ListMyChannels(w.UserID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao listar canais: %w", err)}
	}
	return CommandResponse{Data: result}
}

func (w *Worker) handleFollowChannel(payload ChannelJIDPayload) CommandResponse {
	err := w.newsletterService.FollowChannel(w.UserID, payload.JID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao seguir canal: %w", err)}
	}
	return CommandResponse{Data: "canal seguido com sucesso"}
}

func (w *Worker) handleUnfollowChannel(payload ChannelJIDPayload) CommandResponse {
	err := w.newsletterService.UnfollowChannel(w.UserID, payload.JID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao deixar de seguir canal: %w", err)}
	}
	return CommandResponse{Data: "inscrição no canal cancelada com sucesso"}
}

func (w *Worker) handleMuteChannel(payload ChannelJIDPayload) CommandResponse {
	err := w.newsletterService.MuteChannel(w.UserID, payload.JID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao silenciar canal: %w", err)}
	}
	return CommandResponse{Data: "canal silenciado com sucesso"}
}

func (w *Worker) handleUnmuteChannel(payload ChannelJIDPayload) CommandResponse {
	err := w.newsletterService.UnmuteChannel(w.UserID, payload.JID)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao reativar notificações do canal: %w", err)}
	}
	return CommandResponse{Data: "notificações do canal reativadas com sucesso"}
}

func (w *Worker) handleUpdateNewsletterPicture(payload UpdateNewsletterPicturePayload) CommandResponse {
	pictureID, err := w.newsletterService.UpdateNewsletterPictureFromURL(w.UserID, payload.JID, payload.ImageURL)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar foto da newsletter: %w", err)}
	}
	return CommandResponse{Data: map[string]interface{}{
		"message":    "foto da newsletter atualizada",
		"picture_id": pictureID,
	}}
}

func (w *Worker) handleUpdateNewsletterName(payload UpdateNewsletterNamePayload) CommandResponse {
	err := w.newsletterService.UpdateNewsletterName(w.UserID, payload.JID, payload.Name)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar nome da newsletter: %w", err)}
	}
	return CommandResponse{Data: "nome da newsletter atualizado com sucesso"}
}

func (w *Worker) handleUpdateNewsletterDescription(payload UpdateNewsletterDescriptionPayload) CommandResponse {
	err := w.newsletterService.UpdateNewsletterDescription(w.UserID, payload.JID, payload.Description)
	if err != nil {
		return CommandResponse{Error: fmt.Errorf("falha ao atualizar descrição da newsletter: %w", err)}
	}
	return CommandResponse{Data: "descrição da newsletter atualizada com sucesso"}
}
