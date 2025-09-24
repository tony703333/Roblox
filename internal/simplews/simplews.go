package simplews

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	TextMessage   = 1
	BinaryMessage = 2
	CloseMessage  = 8
	PingMessage   = 9
	PongMessage   = 10

	CloseNormalClosure   = 1000
	CloseGoingAway       = 1001
	CloseAbnormalClosure = 1006
)

var (
	errBadRequest      = errors.New("websocket: bad handshake request")
	errUnsupportedData = errors.New("websocket: unsupported data")
)

// CloseError represents a WebSocket close control frame.
type CloseError struct {
	Code int
	Text string
}

func (e *CloseError) Error() string {
	if e.Text == "" {
		return fmt.Sprintf("websocket: close %d", e.Code)
	}
	return fmt.Sprintf("websocket: close %d (%s)", e.Code, e.Text)
}

// IsUnexpectedCloseError mimics the behaviour of gorilla/websocket.
func IsUnexpectedCloseError(err error, codes ...int) bool {
	ce, ok := err.(*CloseError)
	if !ok {
		return false
	}
	if len(codes) == 0 {
		return ce.Code == CloseGoingAway || ce.Code == CloseAbnormalClosure
	}
	for _, code := range codes {
		if ce.Code == code {
			return true
		}
	}
	return false
}

// Upgrader upgrades HTTP connections to WebSocket connections.
type Upgrader struct {
	ReadBufferSize  int
	WriteBufferSize int
	CheckOrigin     func(r *http.Request) bool
}

// Upgrade performs the WebSocket handshake.
func (u *Upgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (*Conn, error) {
	if !strings.EqualFold(r.Method, http.MethodGet) {
		return nil, errBadRequest
	}

	if u.CheckOrigin != nil && !u.CheckOrigin(r) {
		return nil, errBadRequest
	}

	if !headerContainsToken(r.Header, "Connection", "Upgrade") ||
		!headerContainsToken(r.Header, "Upgrade", "websocket") {
		return nil, errBadRequest
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errBadRequest
	}

	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, errBadRequest
	}

	accept := computeAcceptKey(key)

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("websocket: response does not implement Hijacker")
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	if err := writeHandshake(buf.Writer, accept, responseHeader); err != nil {
		conn.Close()
		return nil, err
	}

	return newConn(conn), nil
}

func computeAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func writeHandshake(w *bufio.Writer, accept string, header http.Header) error {
	if _, err := fmt.Fprintf(w, "HTTP/1.1 101 Switching Protocols\r\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Upgrade: websocket\r\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Connection: Upgrade\r\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Sec-WebSocket-Accept: %s\r\n", accept); err != nil {
		return err
	}
	for k, values := range header {
		for _, v := range values {
			if _, err := fmt.Fprintf(w, "%s: %s\r\n", k, v); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(w, "\r\n"); err != nil {
		return err
	}
	return w.Flush()
}

func headerContainsToken(h http.Header, key, token string) bool {
	value := h.Get(key)
	if value == "" {
		return false
	}
	parts := strings.Split(value, ",")
	for _, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

// Conn represents a WebSocket connection.
type Conn struct {
	conn        net.Conn
	reader      *bufio.Reader
	writeMu     sync.Mutex
	readLimit   int64
	pongHandler func(string) error
}

func newConn(conn net.Conn) *Conn {
	return &Conn{
		conn:        conn,
		reader:      bufio.NewReader(conn),
		readLimit:   0,
		pongHandler: func(string) error { return nil },
	}
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// SetReadLimit sets the maximum incoming payload size.
func (c *Conn) SetReadLimit(limit int64) {
	c.readLimit = limit
}

// SetReadDeadline sets the read deadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// SetPongHandler installs a handler for pong control frames.
func (c *Conn) SetPongHandler(h func(string) error) {
	if h == nil {
		c.pongHandler = func(string) error { return nil }
		return
	}
	c.pongHandler = h
}

// WriteMessage writes a single frame to the connection.
func (c *Conn) WriteMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var opcode byte
	switch messageType {
	case TextMessage:
		opcode = 0x1
	case BinaryMessage:
		opcode = 0x2
	case CloseMessage:
		opcode = 0x8
	case PingMessage:
		opcode = 0x9
	case PongMessage:
		opcode = 0xA
	default:
		return errUnsupportedData
	}

	header := []byte{0x80 | opcode}
	length := len(data)

	switch {
	case length <= 125:
		header = append(header, byte(length))
	case length <= 65535:
		header = append(header, 126)
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], uint16(length))
		header = append(header, buf[:]...)
	default:
		header = append(header, 127)
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(length))
		header = append(header, buf[:]...)
	}

	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	if length > 0 {
		if _, err := c.conn.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// ReadMessage reads the next text or binary message from the connection.
func (c *Conn) ReadMessage() (int, []byte, error) {
	for {
		opcode, payload, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}

		switch opcode {
		case 0x1:
			return TextMessage, payload, nil
		case 0x2:
			return BinaryMessage, payload, nil
		case 0x8:
			code := CloseNormalClosure
			text := ""
			if len(payload) >= 2 {
				code = int(binary.BigEndian.Uint16(payload[:2]))
				text = string(payload[2:])
			}
			return 0, nil, &CloseError{Code: code, Text: text}
		case 0x9:
			// Ping
			_ = c.WriteMessage(PongMessage, payload)
		case 0xA:
			if c.pongHandler != nil {
				_ = c.pongHandler(string(payload))
			}
		default:
			return 0, nil, errUnsupportedData
		}
	}
}

func (c *Conn) readFrame() (byte, []byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return 0, nil, err
	}

	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	payloadLen := int64(header[1] & 0x7F)

	if !fin {
		return 0, nil, errUnsupportedData
	}

	switch payloadLen {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(c.reader, ext); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(c.reader, ext); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext))
	}

	if c.readLimit > 0 && payloadLen > c.readLimit {
		return 0, nil, errors.New("websocket: payload exceeds read limit")
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, maskKey[:]); err != nil {
			return 0, nil, err
		}
	} else {
		return 0, nil, errors.New("websocket: client frames must be masked")
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return 0, nil, err
	}

	for i := int64(0); i < payloadLen; i++ {
		payload[i] ^= maskKey[i%4]
	}

	return opcode, payload, nil
}
