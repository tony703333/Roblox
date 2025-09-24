package ws

import "time"

const (
	MessageTypeChat    = "chat.message"
	MessageTypeTyping  = "chat.typing"
	MessageTypeHistory = "chat.history"
	MessageTypeSystem  = "system.notice"
)

const (
	RolePlayer = "player"
	RoleAgent  = "agent"
)

// ChatMessage represents a persisted chat message that belongs to a room.
type ChatMessage struct {
	RoomID      string            `json:"roomId"`
	SenderID    string            `json:"senderId"`
	SenderRole  string            `json:"senderRole"`
	DisplayName string            `json:"displayName,omitempty"`
	Content     string            `json:"content"`
	Timestamp   time.Time         `json:"timestamp"`
	Sequence    int64             `json:"sequence"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Envelope is the payload exchanged over WebSocket connections.
type Envelope struct {
	Cmd         string            `json:"cmd"`
	Type        string            `json:"type,omitempty"`
	RoomID      string            `json:"roomId,omitempty"`
	SenderID    string            `json:"senderId,omitempty"`
	SenderRole  string            `json:"senderRole,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	Content     string            `json:"content,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Payload     map[string]any    `json:"payload,omitempty"`
	History     []ChatMessage     `json:"history,omitempty"`
	Seq         int64             `json:"seq,omitempty"`
	Ack         int64             `json:"ack,omitempty"`
}

// Participant describes a connected user (player or agent) of a room.
type Participant struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Role        string    `json:"role"`
	Connected   bool      `json:"connected"`
	LastSeen    time.Time `json:"lastSeen"`
}

// RoomSummary offers a lightweight view of a room for listing in the admin.
type RoomSummary struct {
	RoomID               string    `json:"roomId"`
	CreatedAt            time.Time `json:"createdAt"`
	LastActivity         time.Time `json:"lastActivity"`
	PlayerCount          int       `json:"playerCount"`
	AgentCount           int       `json:"agentCount"`
	ConnectedPlayerCount int       `json:"connectedPlayerCount"`
	ConnectedAgentCount  int       `json:"connectedAgentCount"`
	AssignedAgentID      string    `json:"assignedAgentId,omitempty"`
	AssignedAgent        string    `json:"assignedAgent,omitempty"`
	LastMessage          string    `json:"lastMessage,omitempty"`
}

// RoomSnapshot represents the full state of a room, including the history.
type RoomSnapshot struct {
	Summary      RoomSummary   `json:"summary"`
	Participants []Participant `json:"participants"`
	History      []ChatMessage `json:"history"`
	NextSequence int64         `json:"nextSequence"`
}

// Normalize ensures the envelope uses the canonical command naming and keeps
// legacy clients compatible.
func (e *Envelope) Normalize() {
	e.Cmd = normalizeLegacyType(e.Cmd)
	e.Type = normalizeLegacyType(e.Type)
	if e.Cmd == "" {
		e.Cmd = e.Type
	}
	if e.Type == "" {
		e.Type = e.Cmd
	}
}

func normalizeLegacyType(value string) string {
	switch value {
	case "", MessageTypeChat, MessageTypeTyping, MessageTypeHistory, MessageTypeSystem:
		return value
	case "message":
		return MessageTypeChat
	case "typing":
		return MessageTypeTyping
	case "history":
		return MessageTypeHistory
	case "system":
		return MessageTypeSystem
	default:
		return value
	}
}
