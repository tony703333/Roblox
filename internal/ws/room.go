package ws

import (
	"errors"
	"sync"
	"time"
)

// ErrClientNotInRoom is returned when a client attempts to interact with a room it does not belong to.
var ErrClientNotInRoom = errors.New("client not part of room")

// Room represents a chat room shared by a player and customer service agents.
type Room struct {
	id            string
	history       []ChatMessage
	clients       map[*Client]struct{}
	players       map[string]*Participant
	agents        map[string]*Participant
	assignedAgent *Participant
	createdAt     time.Time
	lastActivity  time.Time
	nextSequence  int64
	mu            sync.RWMutex
}

func NewRoom(id string) *Room {
	now := time.Now()
	return &Room{
		id:           id,
		history:      make([]ChatMessage, 0, 32),
		clients:      make(map[*Client]struct{}),
		players:      make(map[string]*Participant),
		agents:       make(map[string]*Participant),
		createdAt:    now,
		lastActivity: now,
		nextSequence: 0,
	}
}

func (r *Room) ID() string {
	return r.id
}

func (r *Room) CreatedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.createdAt
}

func (r *Room) LastActivity() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastActivity
}

func (r *Room) AddClient(c *Client) *Participant {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.clients[c]; ok {
		return r.ensureParticipantLocked(c)
	}

	r.clients[c] = struct{}{}
	participant := r.ensureParticipantLocked(c)
	participant.Connected = true
	participant.LastSeen = time.Now()

	r.lastActivity = time.Now()

	return participant
}

func (r *Room) ensureParticipantLocked(c *Client) *Participant {
	var registry map[string]*Participant
	switch c.Role {
	case RolePlayer:
		registry = r.players
	default:
		registry = r.agents
	}

	if p, ok := registry[c.ID]; ok {
		p.DisplayName = c.DisplayName
		p.Role = c.Role
		return p
	}

	participant := &Participant{
		ID:          c.ID,
		DisplayName: c.DisplayName,
		Role:        c.Role,
		Connected:   true,
		LastSeen:    time.Now(),
	}
	registry[c.ID] = participant

	if c.Role == RoleAgent && r.assignedAgent != nil && r.assignedAgent.ID == c.ID {
		r.assignedAgent = participant
	}

	return participant
}

func (r *Room) RemoveClient(c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.clients[c]; !ok {
		return
	}

	delete(r.clients, c)

	registry := r.players
	if c.Role == RoleAgent {
		registry = r.agents
	}

	if participant, ok := registry[c.ID]; ok {
		participant.Connected = false
		participant.LastSeen = time.Now()
	}
}

func (r *Room) Touch(ts time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ts.After(r.lastActivity) {
		r.lastActivity = ts
	}
}

func (r *Room) AddMessage(msg ChatMessage) ChatMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	r.nextSequence++
	msg.Sequence = r.nextSequence

	r.history = append(r.history, msg)
	r.lastActivity = msg.Timestamp

	if participant, ok := r.players[msg.SenderID]; ok {
		participant.LastSeen = msg.Timestamp
	}
	if participant, ok := r.agents[msg.SenderID]; ok {
		participant.LastSeen = msg.Timestamp
	}

	return msg
}

func (r *Room) Messages() []ChatMessage {
	r.mu.RLock()
	defer r.mu.RUnlock()

	history := make([]ChatMessage, len(r.history))
	copy(history, r.history)
	return history
}

func (r *Room) MessagesSince(sequence int64) ([]ChatMessage, int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	start := 0
	if sequence > 0 {
		start = len(r.history)
		for i, msg := range r.history {
			if msg.Sequence > sequence {
				start = i
				break
			}
		}
	}

	if start < 0 || start > len(r.history) {
		start = len(r.history)
	}

	history := make([]ChatMessage, len(r.history[start:]))
	copy(history, r.history[start:])
	return history, r.nextSequence
}

func (r *Room) NextSequence() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nextSequence
}

func (r *Room) Clients() []*Client {
	r.mu.RLock()
	defer r.mu.RUnlock()

	clients := make([]*Client, 0, len(r.clients))
	for client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

func (r *Room) SetAssignedAgent(agentID, displayName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var participant *Participant
	if existing, ok := r.agents[agentID]; ok {
		existing.DisplayName = displayName
		existing.Role = RoleAgent
		participant = existing
	} else {
		participant = &Participant{
			ID:          agentID,
			DisplayName: displayName,
			Role:        RoleAgent,
			LastSeen:    time.Now(),
		}
		r.agents[agentID] = participant
	}

	participant.Connected = false
	r.assignedAgent = participant
}

func (r *Room) AssignedAgent() *Participant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.assignedAgent == nil {
		return nil
	}

	clone := *r.assignedAgent
	return &clone
}

func (r *Room) Snapshot() RoomSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	participants := make([]Participant, 0, len(r.players)+len(r.agents))
	for _, p := range r.players {
		participants = append(participants, *p)
	}
	for _, p := range r.agents {
		participants = append(participants, *p)
	}

	history := make([]ChatMessage, len(r.history))
	copy(history, r.history)

	connectedPlayers := 0
	for _, p := range r.players {
		if p.Connected {
			connectedPlayers++
		}
	}
	connectedAgents := 0
	for _, p := range r.agents {
		if p.Connected {
			connectedAgents++
		}
	}

	summary := RoomSummary{
		RoomID:               r.id,
		CreatedAt:            r.createdAt,
		LastActivity:         r.lastActivity,
		PlayerCount:          len(r.players),
		AgentCount:           len(r.agents),
		ConnectedPlayerCount: connectedPlayers,
		ConnectedAgentCount:  connectedAgents,
	}
	if r.assignedAgent != nil {
		summary.AssignedAgentID = r.assignedAgent.ID
		summary.AssignedAgent = r.assignedAgent.DisplayName
	}
	if len(r.history) > 0 {
		summary.LastMessage = r.history[len(r.history)-1].Content
	}

	return RoomSnapshot{
		Summary:      summary,
		Participants: participants,
		History:      history,
		NextSequence: r.nextSequence,
	}
}

func (r *Room) Summary() RoomSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	connectedPlayers := 0
	for _, p := range r.players {
		if p.Connected {
			connectedPlayers++
		}
	}
	connectedAgents := 0
	for _, p := range r.agents {
		if p.Connected {
			connectedAgents++
		}
	}

	summary := RoomSummary{
		RoomID:               r.id,
		CreatedAt:            r.createdAt,
		LastActivity:         r.lastActivity,
		PlayerCount:          len(r.players),
		AgentCount:           len(r.agents),
		ConnectedPlayerCount: connectedPlayers,
		ConnectedAgentCount:  connectedAgents,
	}
	if r.assignedAgent != nil {
		summary.AssignedAgentID = r.assignedAgent.ID
		summary.AssignedAgent = r.assignedAgent.DisplayName
	}
	if len(r.history) > 0 {
		summary.LastMessage = r.history[len(r.history)-1].Content
	}
	return summary
}

func (r *Room) HasClient(c *Client) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.clients[c]
	return ok
}

func (r *Room) AgentParticipants() []Participant {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]Participant, 0, len(r.agents))
	for _, p := range r.agents {
		agents = append(agents, *p)
	}
	return agents
}
