package worker

// Payload structures for different command types

// Message payload structures
type SendTextPayload struct {
	To      string `json:"to"`
	Message string `json:"message"`
}

type SendMediaPayload struct {
	To        string `json:"to"`
	MediaURL  string `json:"media_url"`
	MediaType string `json:"media_type"`
	Caption   string `json:"caption"`
}

type SendButtonsPayload struct {
	To      string       `json:"to"`
	Text    string       `json:"text"`
	Footer  string       `json:"footer"`
	Buttons []ButtonData `json:"buttons"`
}

type SendListPayload struct {
	To         string    `json:"to"`
	Text       string    `json:"text"`
	Footer     string    `json:"footer"`
	ButtonText string    `json:"button_text"`
	Sections   []Section `json:"sections"`
}

type CheckNumberPayload struct {
	Number string `json:"number"`
}

// ButtonData represents a button in a message
type ButtonData struct {
	ID          string `json:"id"`
	DisplayText string `json:"displayText"`
}

// Row represents a row in a list message
type Row struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Section represents a section in a list message
type Section struct {
	Title string `json:"title"`
	Rows  []Row  `json:"rows"`
}

// Community payload structures
type CreateCommunityPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CommunityInfoPayload struct {
	CommunityJID string `json:"community_jid"`
}

type UpdateCommunityNamePayload struct {
	CommunityJID string `json:"community_jid"`
	NewName      string `json:"new_name"`
}

type UpdateCommunityDescriptionPayload struct {
	CommunityJID   string `json:"community_jid"`
	NewDescription string `json:"new_description"`
}

type UpdateCommunityPicturePayload struct {
	CommunityJID string `json:"community_jid"`
	ImageURL     string `json:"image_url"`
}

type LeaveCommunityPayload struct {
	CommunityJID string `json:"community_jid"`
}

type CreateGroupForCommunityPayload struct {
	CommunityJID string   `json:"community_jid"`
	GroupName    string   `json:"group_name"`
	Participants []string `json:"participants"`
}

type LinkGroupPayload struct {
	CommunityJID string `json:"community_jid"`
	GroupJID     string `json:"group_jid"`
}

type JoinCommunityWithLinkPayload struct {
	Link string `json:"link"`
}

type GetCommunityInviteLinkPayload struct {
	CommunityJID string `json:"community_jid"`
}

type GetCommunityLinkedGroupsPayload struct {
	CommunityJID string `json:"community_jid"`
}

// Group payload structures
type CreateGroupPayload struct {
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

type GroupInfoPayload struct {
	GroupJID string `json:"group_jid"`
}

type GroupParticipantsPayload struct {
	GroupJID     string   `json:"group_jid"`
	Participants []string `json:"participants"`
}

type UpdateGroupNamePayload struct {
	GroupJID string `json:"group_jid"`
	NewName  string `json:"new_name"`
}

type UpdateGroupTopicPayload struct {
	GroupJID string `json:"group_jid"`
	NewTopic string `json:"new_topic"`
}

type UpdateGroupPicturePayload struct {
	GroupJID string `json:"group_jid"`
	ImageURL string `json:"image_url"`
}

type LeaveGroupPayload struct {
	GroupJID string `json:"group_jid"`
}

type JoinGroupWithLinkPayload struct {
	Link string `json:"link"`
}

type GroupInviteLinkPayload struct {
	GroupJID string `json:"group_jid"`
}

// Newsletter payload structures
type CreateChannelPayload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	PictureURL  string `json:"picture_url"`
}

type ChannelJIDPayload struct {
	JID string `json:"jid"`
}

type ChannelInvitePayload struct {
	InviteLink string `json:"invite_link"`
}

type ListChannelsPayload struct {
	// Empty payload for listing channels
}
