package slippi

//import (
//	"bytes"
//	"encoding/binary"
//	"github.com/jmank88/ubjson"
//)
//
//// CommunicationType represents the packet types sent from a Slippi instance to the client program.
//type CommunicationType uint8
//
//// CommunicationTypes
//const (
//	HANDSHAKE CommunicationType = iota + 1
//	REPLAY
//	KEEP_ALIVE
//)
//
//// CommunicationMessage contains a message from a Slippi instance to a client.
//type CommunicationMessage struct {
//	Type CommunicationType
//	Payload CommunicationPayload
//}
//
//// CommunicationPayload primary contents of a CommunicationMessage.
//type CommunicationPayload struct {
//	Cursor []uint8
//	ClientToken []uint8
//	Pos []uint8
//	NextPos []uint8
//	Data []uint8
//	Nick string
//	ForcePos bool
//	NintendontVersion string
//}
//
//// ConsoleCommunication handles communication with a Wii.
//type ConsoleCommunication struct {
//	receiveBuf bytes.Buffer
//	messages []CommunicationMessage
//}
//
//func (c *ConsoleCommunication) receive(data bytes.Buffer) error {
//	c.receiveBuf.Write(data.Bytes())
//
//	for c.receiveBuf.Len() >= 4 {
//		sizeBuf := make([]byte, 4)
//		_, err := c.receiveBuf.Read(sizeBuf)
//		if err != nil {
//			return err
//		}
//
//		msgSize := binary.BigEndian.Uint32(sizeBuf)
//
//		if c.receiveBuf.Len() < (int) (msgSize + 4) {
//			return nil
//		}
//
//		ubjson = c.receiveBuf.Read()
//	}
//}
//
//func (c *ConsoleCommunication) getReceiveBuffer() bytes.Buffer {
//	return c.receiveBuf
//}
//
//func (c *ConsoleCommunication) getMessages() []CommunicationMessage {
//	ret := c.messages
//	c.messages = make([]CommunicationMessage, 0)
//	return ret
//}
//
//func (c *ConsoleCommunication) genHandshakeOut(cursor []uint8, clientToken uint32, isRealtime bool) (*bytes.Buffer, error) {
//	clientTokenBytes := make([]byte, 4)
//	binary.BigEndian.PutUint32(clientTokenBytes, clientToken)
//
//	message := CommunicationMessage{
//		Type:    HANDSHAKE,
//		Payload: CommunicationPayload{
//			Cursor: cursor,
//			ClientToken: clientTokenBytes,
//		},
//	}
//	encodedMessageBuf := bytes.NewBuffer([]byte{0, 0, 0, 0})
//	err := ubjson.NewEncoder(encodedMessageBuf).Encode(message)
//	if err != nil {
//		return encodedMessageBuf, err
//	}
//
//	encodedMessageBuf
//}
