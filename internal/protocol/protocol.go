// Package protocol implementa o protocolo de mensagens do cliente Flet.
//
// Cada pacote no socket tem o formato:
//
//	[tamanho:u32 LE][tipo:u8][corpo]
//
// tipo 0x00 = mensagem MessagePack no formato [ação, corpo].
// tipo 0x01 = canal de dados binário (ignorado por enquanto).
package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/vmihailenco/msgpack/v5"
)

// Ações do protocolo. Devem bater com MessageAction do cliente Dart.
const (
	ActionRegisterClient     = 1
	ActionPatchControl       = 2
	ActionControlEvent       = 3
	ActionUpdateControlProps = 4
	ActionInvokeMethod       = 5
	ActionSessionCrashed     = 6
)

// Tipos de operação de patch.
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

// Message é um quadro do protocolo.
type Message struct {
	Action int
	Body   map[string]any
}

// Write codifica a mensagem e escreve o pacote completo em w.
func Write(w io.Writer, m Message) error {
	body, err := msgpack.Marshal([]any{m.Action, m.Body})
	if err != nil {
		return fmt.Errorf("protocol: codificar mensagem: %w", err)
	}
	packet := make([]byte, 4+1+len(body))
	binary.LittleEndian.PutUint32(packet[0:4], uint32(1+len(body)))
	packet[4] = packetMessage
	copy(packet[5:], body)
	_, err = w.Write(packet)
	return err
}

// Read lê o próximo pacote de mensagem. Pacotes de canal de dados são ignorados.
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
		return Message{}, fmt.Errorf("protocol: decodificar mensagem: %w", err)
	}
	frame, ok := v.([]any)
	if !ok || len(frame) != 2 {
		return Message{}, fmt.Errorf("protocol: quadro inválido: %v", v)
	}
	action, ok := ToInt64(frame[0])
	if !ok {
		return Message{}, fmt.Errorf("protocol: ação inválida: %v", frame[0])
	}
	body, _ := frame[1].(map[string]any)
	return Message{Action: int(action), Body: body}, nil
}

// ToInt64 converte qualquer número decodificado do MessagePack em int64.
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
