// Package protocol implements the Flet client message protocol.
//
// Every packet on the socket has the format:
//
//	[length:u32 LE][type:u8][body]
//
// type 0x00 = MessagePack message in the form [action, body].
// type 0x01 = binary data channel (ignored for now).
package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// Protocol actions. Must match MessageAction in the Dart client.
const (
	ActionRegisterClient     = 1
	ActionPatchControl       = 2
	ActionControlEvent       = 3
	ActionUpdateControlProps = 4
	ActionInvokeMethod       = 5
	ActionSessionCrashed     = 6
)

// Patch operation types.
const (
	OpReplace = 0
	OpAdd     = 1
	OpRemove  = 2
	OpMove    = 3
)

const (
	packetMessage     = 0x00
	packetDataChannel = 0x01
)

// Message is a protocol frame.
type Message struct {
	Action int
	Body   map[string]any
}

// Write encodes the message and writes the full packet to w.
func Write(w io.Writer, m Message) error {
	body, err := msgpack.Marshal([]any{m.Action, m.Body})
	if err != nil {
		return fmt.Errorf("protocol: encode message: %w", err)
	}
	packet := make([]byte, 4+1+len(body))
	binary.LittleEndian.PutUint32(packet[0:4], uint32(1+len(body)))
	packet[4] = packetMessage
	copy(packet[5:], body)
	_, err = w.Write(packet)
	return err
}

// Read reads the next message packet. Data channel packets are skipped.
func Read(r io.Reader) (Message, error) {
	for {
		var header [4]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return Message{}, err
		}
		n := binary.LittleEndian.Uint32(header[:])
		if n == 0 {
			continue
		}
		packet := make([]byte, n)
		if _, err := io.ReadFull(r, packet); err != nil {
			return Message{}, err
		}
		if packet[0] != packetMessage {
			continue
		}
		return decode(packet[1:])
	}
}

func decode(b []byte) (Message, error) {
	dec := msgpack.NewDecoder(bytes.NewReader(b))
	dec.UseLooseInterfaceDecoding(true)
	v, err := dec.DecodeInterface()
	if err != nil {
		return Message{}, fmt.Errorf("protocol: decode message: %w", err)
	}
	frame, ok := v.([]any)
	if !ok || len(frame) != 2 {
		return Message{}, fmt.Errorf("protocol: invalid frame: %v", v)
	}
	action, ok := ToInt64(frame[0])
	if !ok {
		return Message{}, fmt.Errorf("protocol: invalid action: %v", frame[0])
	}
	body, _ := frame[1].(map[string]any)
	return Message{Action: int(action), Body: body}, nil
}

// ToInt64 converts any number decoded from MessagePack to int64.
func ToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case uint64:
		return int64(n), true
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}
