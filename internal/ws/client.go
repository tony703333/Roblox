package ws

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"im/internal/simplews"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 8192
)

// Client represents a connected WebSocket participant.
type Client struct {
	hub         *Hub
	conn        *simplews.Conn
	send        chan []byte
	ID          string
	DisplayName string
	Role        string
	RoomID      string
	closeOnce   sync.Once
}

func NewClient(hub *Hub, conn *simplews.Conn, roomID, id, role, displayName string) *Client {
	return &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, 16),
		ID:          id,
		DisplayName: displayName,
		Role:        role,
		RoomID:      roomID,
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.hub.Unregister(c)
		c.Close()
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if simplews.IsUnexpectedCloseError(err, simplews.CloseGoingAway, simplews.CloseAbnormalClosure) {
				log.Printf("websocket unexpected close: %v", err)
			}
			break
		}

		var envelope Envelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			c.sendSystemError("格式錯誤，請重新傳送")
			continue
		}

		envelope.Normalize()

		if err := c.hub.HandleIncoming(c, envelope); err != nil {
			c.sendSystemError(err.Error())
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(simplews.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(simplews.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(simplews.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) SendEnvelope(env Envelope) error {
	if env.Timestamp.IsZero() {
		env.Timestamp = time.Now()
	}

	env.Normalize()

	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}

	select {
	case c.send <- payload:
		return nil
	default:
		return errors.New("send buffer full")
	}
}

func (c *Client) sendSystemError(message string) {
	_ = c.SendEnvelope(Envelope{
		Cmd:       MessageTypeSystem,
		Type:      MessageTypeSystem,
		RoomID:    c.RoomID,
		Timestamp: time.Now(),
		Content:   message,
	})
}

// Close safely closes the outgoing channel once.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.send)
	})
}
