package ws

import (
	"encoding/json"
	"testing"
	"time"
)

type testClient struct {
	*Client
}

func newTestClient(h *Hub, roomID, role, id, name string) *testClient {
	c := &Client{
		hub:         h,
		conn:        nil,
		send:        make(chan []byte, 10),
		ID:          id,
		DisplayName: name,
		Role:        role,
		RoomID:      roomID,
	}
	return &testClient{Client: c}
}

func (c *testClient) nextEnvelope(t *testing.T) Envelope {
	select {
	case payload := <-c.send:
		var env Envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			t.Fatalf("failed to decode envelope: %v", err)
		}
		env.Normalize()
		return env
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
	return Envelope{}
}

func TestHubMessageFlow(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "room-1", RolePlayer, "p1", "玩家1")

	if _, err := hub.Register(client.Client); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Expect system join notification.
	join := client.nextEnvelope(t)
	if join.Cmd != MessageTypeSystem {
		t.Fatalf("expected system message, got %s", join.Cmd)
	}

	if err := hub.HandleIncoming(client.Client, Envelope{Cmd: MessageTypeChat, Content: "您好"}); err != nil {
		t.Fatalf("handle incoming failed: %v", err)
	}

	msg := client.nextEnvelope(t)
	if msg.Cmd != MessageTypeChat {
		t.Fatalf("expected chat message, got %s", msg.Cmd)
	}
	if msg.Content != "您好" {
		t.Fatalf("unexpected content: %s", msg.Content)
	}
	if msg.Seq == 0 {
		t.Fatalf("expected sequence to be assigned")
	}

	snapshot, err := hub.RoomSnapshot("room-1")
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}
	if len(snapshot.History) != 1 {
		t.Fatalf("expected 1 history message, got %d", len(snapshot.History))
	}
	if snapshot.History[0].Content != "您好" {
		t.Fatalf("unexpected history content: %s", snapshot.History[0].Content)
	}
	if snapshot.NextSequence == 0 {
		t.Fatalf("expected next sequence to be tracked")
	}

	if _, err := hub.AssignAgent("room-1", "a1", "客服A"); err != nil {
		t.Fatalf("assign agent failed: %v", err)
	}

	assign := client.nextEnvelope(t)
	if assign.Cmd != MessageTypeSystem {
		t.Fatalf("expected system message after assign, got %s", assign.Cmd)
	}
	if assign.Metadata["assignedAgent"] != "客服A" {
		t.Fatalf("expected metadata assignedAgent, got %+v", assign.Metadata)
	}

	snapshot, err = hub.RoomSnapshot("room-1")
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}
	if snapshot.Summary.AssignedAgent != "客服A" {
		t.Fatalf("expected assigned agent 客服A, got %s", snapshot.Summary.AssignedAgent)
	}
}

func TestRoomsOrdering(t *testing.T) {
	hub := NewHub()
	c1 := newTestClient(hub, "room-a", RolePlayer, "p1", "玩家1")
	c2 := newTestClient(hub, "room-b", RolePlayer, "p2", "玩家2")

	if _, err := hub.Register(c1.Client); err != nil {
		t.Fatalf("register c1 failed: %v", err)
	}
	// drain join
	_ = c1.nextEnvelope(t)

	if _, err := hub.Register(c2.Client); err != nil {
		t.Fatalf("register c2 failed: %v", err)
	}
	_ = c2.nextEnvelope(t)

	// Send message on room-b to bump activity.
	if err := hub.HandleIncoming(c2.Client, Envelope{Cmd: MessageTypeChat, Content: "Hi"}); err != nil {
		t.Fatalf("handle incoming failed: %v", err)
	}
	_ = c2.nextEnvelope(t)

	rooms := hub.Rooms()
	if len(rooms) != 2 {
		t.Fatalf("expected 2 rooms, got %d", len(rooms))
	}
	if rooms[0].RoomID != "room-b" {
		t.Fatalf("expected room-b to be first, got %s", rooms[0].RoomID)
	}
}

func TestMessagesSince(t *testing.T) {
	hub := NewHub()
	client := newTestClient(hub, "room-x", RolePlayer, "px", "玩家X")

	if _, err := hub.Register(client.Client); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	_ = client.nextEnvelope(t) // consume join

	if err := hub.HandleIncoming(client.Client, Envelope{Cmd: MessageTypeChat, Content: "hello"}); err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	first := client.nextEnvelope(t)
	if first.Seq == 0 {
		t.Fatalf("expected first sequence to be set")
	}

	if err := hub.HandleIncoming(client.Client, Envelope{Cmd: MessageTypeChat, Content: "world"}); err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	_ = client.nextEnvelope(t)

	history, nextSeq, err := hub.MessagesSince("room-x", first.Seq)
	if err != nil {
		t.Fatalf("messages since failed: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 message newer than seq, got %d", len(history))
	}
	if history[0].Content != "world" {
		t.Fatalf("expected to receive latest message, got %s", history[0].Content)
	}
	if nextSeq <= first.Seq {
		t.Fatalf("expected next sequence to advance")
	}
}
