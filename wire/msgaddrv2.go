package wire

import (
	"fmt"
	"io"
)

// MaxV2AddrPerMsg is the maximum number of version 2 addresses that will exist
// in a single addrv2 message (MsgAddrV2).
const MaxV2AddrPerMsg = 1000

// MsgAddrV2 implements the Message interface and represents a bitcoin addrv2
// message that can support longer-length addresses like torv3, cjdns, and i2p.
// It is used to gossip addresses on the network. Each message is limited to
// MaxV2AddrPerMsg addresses. This is the same limit as MsgAddr.
type MsgAddrV2 struct {
	AddrList []*NetAddressV2
}

// BtcDecode decodes r using the bitcoin protocol into a MsgAddrV2.
func (m *MsgAddrV2) BtcDecode(r io.Reader, pver uint32,
	enc MessageEncoding) error {

	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Limit to max addresses per message.
	if count > MaxV2AddrPerMsg {
		str := fmt.Sprintf("too many addresses for message [count %v,"+
			" max %v]", count, MaxV2AddrPerMsg)
		return messageError("MsgAddrV2.BtcDecode", str)
	}

	addrList := make([]NetAddressV2, count)
	m.AddrList = make([]*NetAddressV2, 0, count)
	for i := uint64(0); i < count; i++ {
		na := &addrList[i]
		err := readNetAddressV2(r, pver, na)
		switch err {
		case ErrSkippedNetworkID:
			// This may be a network ID we don't know of, but is
			// still valid. We can safely skip those.
			continue
		case ErrInvalidAddressSize:
			// The encoding used by the peer does not follow
			// BIP-155 and we should stop processing this message.
			return err
		}

		m.AddrList = append(m.AddrList, na)
	}

	return nil
}

// BtcEncode encodes the MsgAddrV2 into a writer w.
func (m *MsgAddrV2) BtcEncode(w io.Writer, pver uint32,
	enc MessageEncoding) error {

	count := len(m.AddrList)
	if count > MaxV2AddrPerMsg {
		str := fmt.Sprintf("too many addresses for message [count %v,"+
			" max %v]", count, MaxV2AddrPerMsg)
		return messageError("MsgAddrV2.BtcEncode", str)
	}

	err := WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}

	for _, na := range m.AddrList {
		err = writeNetAddressV2(w, pver, na)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for MsgAddrV2.
func (m *MsgAddrV2) Command() string {
	return CmdAddrV2
}

// MaxPayloadLength returns the maximum length payload possible for MsgAddrV2.
func (m *MsgAddrV2) MaxPayloadLength(pver uint32) uint32 {
	// The varint that can store the maximum number of addresses is 3 bytes
	// long. The maximum payload is then 3 + 1000 * maxNetAddressV2Payload.
	return 3 + (MaxV2AddrPerMsg * maxNetAddressV2Payload())
}

// NewMsgAddrV2 returns a new bitcoin addrv2 message that conforms to the
// Message interface.
func NewMsgAddrV2() *MsgAddrV2 {
	return &MsgAddrV2{
		AddrList: make([]*NetAddressV2, 0, MaxV2AddrPerMsg),
	}
}
