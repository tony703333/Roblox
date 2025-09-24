package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	// ErrUnknownMessage is returned when a client sends an unsupported message type.
	ErrUnknownMessage = errors.New("unknown message type")
	// ErrRoomNotFound indicates that the requested room does not exist.
	ErrRoomNotFound = errors.New("room not found")
)

// AgentPresence represents an online agent and the rooms they are active in.
type AgentPresence struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"displayName"`
	Rooms       []string  `json:"rooms"`
	LastSeen    time.Time `json:"lastSeen"`
}

// Hub coordinates rooms and broadcasts messages to connected clients.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]*Room),
	}
}

func (h *Hub) Register(c *Client) (*Room, error) {
	if c.RoomID == "" {
		return nil, fmt.Errorf("room id is required")
	}

	room := h.getOrCreateRoom(c.RoomID)
	participant := room.AddClient(c)

	history, nextSeq := room.MessagesSince(0)
	if len(history) > 0 {
		_ = c.SendEnvelope(Envelope{
			Cmd:       MessageTypeHistory,
			Type:      MessageTypeHistory,
			RoomID:    room.ID(),
			Timestamp: time.Now(),
			History:   history,
			Seq:       nextSeq,
			Ack:       nextSeq,
			Payload: map[string]any{
				"messages": history,
				"nextSeq":  nextSeq,
			},
		})
	}

	joinContent := fmt.Sprintf("%s (%s) 加入對話", participant.DisplayName, participant.Role)
	h.broadcast(room, Envelope{
		Cmd:         MessageTypeSystem,
		Type:        MessageTypeSystem,
		RoomID:      room.ID(),
		Timestamp:   time.Now(),
		Content:     joinContent,
		SenderID:    c.ID,
		SenderRole:  c.Role,
		DisplayName: c.DisplayName,
	})

	return room, nil
}

func (h *Hub) Unregister(c *Client) {
	if c.RoomID == "" {
		return
	}

	room := h.getRoom(c.RoomID)
	if room == nil {
		return
	}

	room.RemoveClient(c)

	leaveContent := fmt.Sprintf("%s 離開對話", c.DisplayName)
	h.broadcast(room, Envelope{
		Cmd:         MessageTypeSystem,
		Type:        MessageTypeSystem,
		RoomID:      room.ID(),
		Timestamp:   time.Now(),
		Content:     leaveContent,
		SenderID:    c.ID,
		SenderRole:  c.Role,
		DisplayName: c.DisplayName,
	})
}

func (h *Hub) HandleIncoming(c *Client, env Envelope) error {
	room := h.getRoom(c.RoomID)
	if room == nil {
		return ErrRoomNotFound
	}

	now := time.Now()
	if env.Timestamp.IsZero() {
		env.Timestamp = now
	}

	env.Normalize()

	switch env.Cmd {
	case MessageTypeChat:
		if env.Content == "" {
			return errors.New("content is required")
		}

		chat := ChatMessage{
			RoomID:      c.RoomID,
			SenderID:    c.ID,
			SenderRole:  c.Role,
			DisplayName: c.DisplayName,
			Content:     env.Content,
			Timestamp:   env.Timestamp,
		}
		stored := room.AddMessage(chat)

		env.Cmd = MessageTypeChat
		env.Type = MessageTypeChat
		env.RoomID = c.RoomID
		env.SenderID = c.ID
		env.SenderRole = c.Role
		env.DisplayName = c.DisplayName
		env.Timestamp = stored.Timestamp
		env.Seq = stored.Sequence
		env.Ack = stored.Sequence
		h.broadcast(room, env)
	case MessageTypeTyping:
		env.Cmd = MessageTypeTyping
		env.Type = MessageTypeTyping
		env.RoomID = c.RoomID
		env.SenderID = c.ID
		env.SenderRole = c.Role
		env.DisplayName = c.DisplayName
		room.Touch(env.Timestamp)
		env.Ack = room.NextSequence()
		h.broadcast(room, env)
	case MessageTypeHistory:
		since := env.Seq
		if since == 0 && env.Metadata != nil {
			if value, ok := env.Metadata["since"]; ok {
				if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
					since = parsed
				}
			}
		}

		history, nextSeq := room.MessagesSince(since)
		response := Envelope{
			Cmd:       MessageTypeHistory,
			Type:      MessageTypeHistory,
			RoomID:    c.RoomID,
			Timestamp: env.Timestamp,
			History:   history,
			Seq:       nextSeq,
			Ack:       nextSeq,
			Payload: map[string]any{
				"messages": history,
				"nextSeq":  nextSeq,
			},
		}
		return c.SendEnvelope(response)
	default:
		return ErrUnknownMessage
	}

	return nil
}

func (h *Hub) Rooms() []RoomSummary {
	h.mu.RLock()
	defer h.mu.RUnlock()

	summaries := make([]RoomSummary, 0, len(h.rooms))
	for _, room := range h.rooms {
		summaries = append(summaries, room.Summary())
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActivity.After(summaries[j].LastActivity)
	})
	return summaries
}

func (h *Hub) RoomSnapshot(roomID string) (RoomSnapshot, error) {
	room := h.getRoom(roomID)
	if room == nil {
		return RoomSnapshot{}, ErrRoomNotFound
	}
	return room.Snapshot(), nil
}

func (h *Hub) AssignAgent(roomID, agentID, displayName string) (*Participant, error) {
	room := h.getRoom(roomID)
	if room == nil {
		return nil, ErrRoomNotFound
	}

	if presence := h.agentPresence(agentID); presence != nil && presence.DisplayName != "" {
		displayName = presence.DisplayName
	}

	room.SetAssignedAgent(agentID, displayName)
	assigned := room.AssignedAgent()
	if assigned == nil {
		return nil, errors.New("unable to assign agent")
	}

	h.broadcast(room, Envelope{
		Cmd:         MessageTypeSystem,
		Type:        MessageTypeSystem,
		RoomID:      roomID,
		Timestamp:   time.Now(),
		Content:     fmt.Sprintf("客服 %s 將協助這個對話", displayName),
		SenderID:    agentID,
		SenderRole:  RoleAgent,
		DisplayName: displayName,
		Metadata: map[string]string{
			"assignedAgent":   displayName,
			"assignedAgentId": agentID,
		},
	})

	return assigned, nil
}

func (h *Hub) getOrCreateRoom(roomID string) *Room {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room, ok := h.rooms[roomID]; ok {
		return room
	}

	room := NewRoom(roomID)
	h.rooms[roomID] = room
	return room
}

func (h *Hub) getRoom(roomID string) *Room {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.rooms[roomID]
}

func (h *Hub) broadcast(room *Room, env Envelope) {
	env.Normalize()

	if env.Timestamp.IsZero() {
		env.Timestamp = time.Now()
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return
	}

	clients := room.Clients()
	for _, client := range clients {
		select {
		case client.send <- payload:
		default:
			// drop message if buffer is full to avoid blocking
		}
	}
}

// MessagesSince returns chat history newer than the provided sequence number.
func (h *Hub) MessagesSince(roomID string, sequence int64) ([]ChatMessage, int64, error) {
	room := h.getRoom(roomID)
	if room == nil {
		return nil, 0, ErrRoomNotFound
	}
	history, next := room.MessagesSince(sequence)
	return history, next, nil
}

// OnlineAgents returns unique connected agents across all rooms.
func (h *Hub) OnlineAgents() []AgentPresence {
	h.mu.RLock()
	defer h.mu.RUnlock()

	catalog := make(map[string]*AgentPresence)
	for _, room := range h.rooms {
		agents := room.AgentParticipants()
		for _, agent := range agents {
			if !agent.Connected {
				continue
			}
			entry, ok := catalog[agent.ID]
			if !ok {
				entry = &AgentPresence{
					ID:          agent.ID,
					DisplayName: agent.DisplayName,
					Rooms:       make([]string, 0, 1),
					LastSeen:    agent.LastSeen,
				}
				catalog[agent.ID] = entry
			}
			entry.Rooms = append(entry.Rooms, room.ID())
			if agent.LastSeen.After(entry.LastSeen) {
				entry.LastSeen = agent.LastSeen
			}
		}
	}

	presences := make([]AgentPresence, 0, len(catalog))
	for _, presence := range catalog {
		presences = append(presences, *presence)
	}
	sort.Slice(presences, func(i, j int) bool {
		if presences[i].DisplayName == presences[j].DisplayName {
			return presences[i].ID < presences[j].ID
		}
		return presences[i].DisplayName < presences[j].DisplayName
	})
	return presences
}

func (h *Hub) agentPresence(agentID string) *AgentPresence {
	agents := h.OnlineAgents()
	for _, agent := range agents {
		if agent.ID == agentID {
			clone := agent
			clone.Rooms = append([]string(nil), agent.Rooms...)
			return &clone
		}
	}
	return nil
}
