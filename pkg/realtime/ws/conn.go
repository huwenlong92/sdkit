package ws

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const (
	opText  = 1
	opClose = 8
	opPing  = 9
	opPong  = 10
)

type Conn struct {
	net.Conn
	r       *bufio.Reader
	writeMu sync.Mutex
}

func NewConn(conn net.Conn, reader *bufio.Reader) *Conn {
	if reader == nil {
		reader = bufio.NewReader(conn)
	}
	return &Conn{Conn: conn, r: reader}
}

func (c *Conn) ReadText(max int64) ([]byte, int, error) {
	header, err := c.r.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	opcode := int(header & 0x0f)
	lenByte, err := c.r.ReadByte()
	if err != nil {
		return nil, opcode, err
	}
	masked := lenByte&0x80 != 0
	n := int64(lenByte & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(c.r, b[:]); err != nil {
			return nil, opcode, err
		}
		n = int64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(c.r, b[:]); err != nil {
			return nil, opcode, err
		}
		n = int64(binary.BigEndian.Uint64(b[:]))
	}
	if max > 0 && n > max {
		return nil, opcode, errors.New("websocket message too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.r, mask[:]); err != nil {
			return nil, opcode, err
		}
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return nil, opcode, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return payload, opcode, nil
}

func (c *Conn) WriteText(payload []byte) error {
	return c.writeFrame(opText, payload, false)
}

func (c *Conn) WriteClose() error {
	return c.writeFrame(opClose, nil, false)
}

func (c *Conn) WritePing(payload []byte) error {
	return c.writeFrame(opPing, payload, false)
}

func (c *Conn) WritePong(payload []byte) error {
	return c.writeFrame(opPong, payload, false)
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

func (c *Conn) writeFrame(opcode int, payload []byte, mask bool) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return writeFrame(c.Conn, opcode, payload, mask)
}

func writeFrame(w io.Writer, opcode int, payload []byte, mask bool) error {
	header := []byte{0x80 | byte(opcode)}
	n := len(payload)
	switch {
	case n < 126:
		header = append(header, byte(n))
	case n <= 65535:
		header = append(header, 126, byte(n>>8), byte(n))
	default:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		header = append(header, 127)
		header = append(header, b[:]...)
	}
	if mask {
		header[1] |= 0x80
		var key [4]byte
		if _, err := rand.Read(key[:]); err != nil {
			return err
		}
		header = append(header, key[:]...)
		masked := make([]byte, len(payload))
		for i := range payload {
			masked[i] = payload[i] ^ key[i%4]
		}
		payload = masked
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	_, err := w.Write(payload)
	return err
}
