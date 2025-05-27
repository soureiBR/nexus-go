package worker

import (
	"sync"
	"time"
)

// WorkerType define o tipo do worker
type WorkerType int

const (
	DefaultWorkerType WorkerType = iota
	CommunityWorkerType
	GroupWorkerType
	MessageWorkerType
)

func (wt WorkerType) String() string {
	switch wt {
	case DefaultWorkerType:
		return "default"
	case CommunityWorkerType:
		return "community"
	case GroupWorkerType:
		return "group"
	case MessageWorkerType:
		return "message"
	default:
		return "unknown"
	}
}

// TaskPriority define a prioridade da tarefa
type TaskPriority int

const (
	LowPriority TaskPriority = iota
	NormalPriority
	HighPriority
	CriticalPriority
)

func (tp TaskPriority) String() string {
	switch tp {
	case LowPriority:
		return "low"
	case NormalPriority:
		return "normal"
	case HighPriority:
		return "high"
	case CriticalPriority:
		return "critical"
	default:
		return "unknown"
	}
}

// WorkerStatus define o status do worker
type WorkerStatus int

const (
	StatusStopped WorkerStatus = iota
	StatusStarting
	StatusIdle
	StatusBusy
	StatusStopping
	StatusError
)

func (ws WorkerStatus) String() string {
	switch ws {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusIdle:
		return "idle"
	case StatusBusy:
		return "busy"
	case StatusStopping:
		return "stopping"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// CommandType define os tipos de comandos suportados
type CommandType string

const (
	// Session commands
	CmdConnect    CommandType = "connect"
	CmdDisconnect CommandType = "disconnect"
	CmdGetStatus  CommandType = "get_status"
	CmdGetQR      CommandType = "get_qr"
	CmdLogout     CommandType = "logout"

	// Message commands
	CmdSendText    CommandType = "send_text"
	CmdSendMedia   CommandType = "send_media"
	CmdSendButtons CommandType = "send_buttons"
	CmdSendList    CommandType = "send_list"

	// Community commands
	CmdCreateCommunity            CommandType = "create_community"
	CmdGetCommunityInfo           CommandType = "get_community_info"
	CmdGetJoinedCommunities       CommandType = "get_joined_communities"
	CmdUpdateCommunityName        CommandType = "update_community_name"
	CmdUpdateCommunityDescription CommandType = "update_community_description"
	CmdLeaveCommunity             CommandType = "leave_community"
	CmdCreateGroupForCommunity    CommandType = "create_group_for_community"
	CmdLinkGroupToCommunity       CommandType = "link_group_to_community"
	CmdUnlinkGroupFromCommunity   CommandType = "unlink_group_from_community"
	CmdGetCommunityInviteLink     CommandType = "get_community_invite_link"
	CmdRevokeCommunityInviteLink  CommandType = "revoke_community_invite_link"
	CmdJoinCommunityWithLink      CommandType = "join_community_with_link"

	// Group commands
	CmdCreateGroup              CommandType = "create_group"
	CmdGetGroupInfo             CommandType = "get_group_info"
	CmdGetJoinedGroups          CommandType = "get_joined_groups"
	CmdAddGroupParticipants     CommandType = "add_group_participants"
	CmdRemoveGroupParticipants  CommandType = "remove_group_participants"
	CmdPromoteGroupParticipants CommandType = "promote_group_participants"
	CmdDemoteGroupParticipants  CommandType = "demote_group_participants"
	CmdUpdateGroupName          CommandType = "update_group_name"
	CmdUpdateGroupTopic         CommandType = "update_group_topic"
	CmdLeaveGroup               CommandType = "leave_group"
	CmdJoinGroupWithLink        CommandType = "join_group_with_link"
	CmdGetGroupInviteLink       CommandType = "get_group_invite_link"
	CmdRevokeGroupInviteLink    CommandType = "revoke_group_invite_link"

	// Newsletter commands
	CmdCreateChannel        CommandType = "create_channel"
	CmdGetChannelInfo       CommandType = "get_channel_info"
	CmdGetChannelWithInvite CommandType = "get_channel_with_invite"
	CmdListMyChannels       CommandType = "list_my_channels"
	CmdFollowChannel        CommandType = "follow_channel"
	CmdUnfollowChannel      CommandType = "unfollow_channel"
	CmdMuteChannel          CommandType = "mute_channel"
	CmdUnmuteChannel        CommandType = "unmute_channel"
)

// Worker represents a worker instance
type Worker struct {
	ID                string
	Type              WorkerType
	UserID            string
	Priority          int
	status            WorkerStatus
	metrics           *WorkerMetrics
	taskQueue         chan Task
	eventQueue        chan interface{}
	done              chan struct{}
	sessionManager    SessionManager
	coordinator       Coordinator
	communityService  CommunityServiceInterface
	groupService      GroupServiceInterface
	messageService    MessageServiceInterface
	newsletterService NewsletterServiceInterface
	config            *WorkerConfig
	mu                sync.RWMutex
	wg                sync.WaitGroup
	isRunning         int32
}

// WorkerMetrics holds metrics for a worker
type WorkerMetrics struct {
	StartTime       time.Time     `json:"start_time"`
	LastTaskTime    time.Time     `json:"last_task_time"`
	TasksProcessed  int64         `json:"tasks_processed"`
	TasksSuccessful int64         `json:"tasks_successful"`
	TasksFailed     int64         `json:"tasks_failed"`
	ErrorCount      int64         `json:"error_count"`
	AverageTaskTime time.Duration `json:"average_task_time"`
}

// WorkerInfo holds basic information about a worker
type WorkerInfo struct {
	ID      string        `json:"id"`
	Type    WorkerType    `json:"type"`
	UserID  string        `json:"user_id"`
	Status  WorkerStatus  `json:"status"`
	Metrics WorkerMetrics `json:"metrics"`
}

// Task represents a task for execution
type Task struct {
	ID         string
	Type       CommandType
	UserID     string
	Priority   TaskPriority
	Payload    interface{}
	Response   chan CommandResponse
	ResultChan chan TaskResult // For backward compatibility
	Created    time.Time
	Deadline   time.Time
	Retries    int
	MaxRetries int
}

// TaskResult represents the result of a task execution
type TaskResult struct {
	Data  interface{}
	Error error
}

// Command represents a command for execution
type Command struct {
	ID       string
	Type     CommandType
	Priority TaskPriority
	Payload  interface{}
	Response chan CommandResponse
	Created  time.Time
	Timeout  time.Duration
}

// CommandResponse represents the response of a command
type CommandResponse struct {
	CommandID string
	Data      interface{}
	Error     error
	Duration  time.Duration
}

// WorkerConfig holds worker configuration
type WorkerConfig struct {
	TaskQueueSize  int
	EventQueueSize int
	WorkerTimeout  time.Duration
	MaxWorkers     int
	MinWorkers     int
	IdleTimeout    time.Duration
	ProcessTimeout time.Duration
}

// DefaultConfig returns a default worker configuration
func DefaultConfig() *WorkerConfig {
	return &WorkerConfig{
		TaskQueueSize:  100,
		EventQueueSize: 50,
		WorkerTimeout:  30 * time.Second,
		MaxWorkers:     10,
		MinWorkers:     1,
		IdleTimeout:    5 * time.Minute,
		ProcessTimeout: 30 * time.Second,
	}
}

// Interfaces

// SessionManager interface for session management
type SessionManager interface {
	Connect(ctx interface{}, userID string) error
	Disconnect(userID string) error
	GetSessionStatus(userID string) (map[string]interface{}, error)
	GetQRChannel(ctx interface{}, userID string) (interface{}, error)
	Logout(ctx interface{}, userID string) error
}

// Coordinator interface for worker coordination
type Coordinator interface {
	NotifyWorkerStatus(workerID, userID string, status WorkerStatus)
	ProcessEvent(userID string, event interface{}) error
	GetCommunityService() CommunityServiceInterface
	GetGroupService() GroupServiceInterface
	GetMessageService() MessageServiceInterface
	GetNewsletterService() NewsletterServiceInterface
}

// CommunityServiceInterface defines community operations interface
type CommunityServiceInterface interface {
	CreateCommunity(userID, name, description string) (interface{}, error)
	GetCommunityInfo(userID, communityJID string) (interface{}, error)
	UpdateCommunityName(userID, communityJID, newName string) error
	UpdateCommunityDescription(userID, communityJID, newDescription string) error
	LeaveCommunity(userID, communityJID string) error
	GetJoinedCommunities(userID string) (interface{}, error)
	CreateGroupForCommunity(userID, communityJID, groupName string, participants []string) (interface{}, error)
	LinkGroupToCommunity(userID, communityJID, groupJID string) error
	UnlinkGroupFromCommunity(userID, communityJID, groupJID string) error
	GetCommunityInviteLink(userID, communityJID string) (string, error)
	RevokeCommunityInviteLink(userID, communityJID string) (string, error)
	JoinCommunityWithLink(userID, link string) (interface{}, error)
}

// GroupServiceInterface defines group operations interface
type GroupServiceInterface interface {
	CreateGroup(userID, name string, participants []string) (interface{}, error)
	GetGroupInfo(userID, groupJID string) (interface{}, error)
	GetJoinedGroups(userID string) (interface{}, error)
	AddGroupParticipants(userID, groupJID string, participants []string) error
	RemoveGroupParticipants(userID, groupJID string, participants []string) error
	PromoteGroupParticipants(userID, groupJID string, participants []string) error
	DemoteGroupParticipants(userID, groupJID string, participants []string) error
	UpdateGroupName(userID, groupJID, newName string) error
	UpdateGroupTopic(userID, groupJID, newTopic string) error
	LeaveGroup(userID, groupJID string) error
	JoinGroupWithLink(userID, link string) (interface{}, error)
	GetGroupInviteLink(userID, groupJID string) (string, error)
	RevokeGroupInviteLink(userID, groupJID string) (string, error)
}

// MessageServiceInterface defines messaging operations interface
type MessageServiceInterface interface {
	SendText(userID, to, message string) (string, error)
	SendMedia(userID, to, mediaURL, mediaType, caption string) (string, error)
	SendButtons(userID, to, text, footer string, buttons []ButtonData) (string, error)
	SendList(userID, to, text, footer, buttonText string, sections []Section) (string, error)
}

// NewsletterServiceInterface defines newsletter operations interface
type NewsletterServiceInterface interface {
	CreateChannel(userID, name, description, pictureURL string) (interface{}, error)
	GetChannelInfo(userID, jid string) (interface{}, error)
	GetChannelWithInvite(userID, inviteLink string) (interface{}, error)
	ListMyChannels(userID string) (interface{}, error)
	FollowChannel(userID, jid string) error
	UnfollowChannel(userID, jid string) error
	MuteChannel(userID, jid string) error
	UnmuteChannel(userID, jid string) error
}
