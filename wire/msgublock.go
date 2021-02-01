package wire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/mit-dci/utreexo/btcacc"
)

type MsgUBlock struct {
	MsgBlock    MsgBlock
	UtreexoData btcacc.UData
}

func (msgu *MsgUBlock) BlockHash() chainhash.Hash {
	return msgu.MsgBlock.BlockHash()
}

func (msgu *MsgUBlock) BtcDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	msgu.MsgBlock = MsgBlock{}
	err := msgu.MsgBlock.BtcDecode(r, pver, enc)
	if err != nil {
		return err
	}

	msgu.UtreexoData = btcacc.UData{}
	err = msgu.UtreexoData.Deserialize(r)

	return nil
}

// Deserialize a UBlock.  It's just a block then udata.
func (msgu *MsgUBlock) Deserialize(r io.Reader) (err error) {
	err = msgu.MsgBlock.Deserialize(r)
	if err != nil {
		return err
	}

	err = msgu.UtreexoData.Deserialize(r)
	return
}

func (msgu *MsgUBlock) BtcEncode(r io.Writer, pver uint32, enc MessageEncoding) error {
	err := msgu.MsgBlock.BtcEncode(r, pver, enc)
	if err != nil {
		return err
	}
	err = msgu.UtreexoData.Serialize(r)

	return nil
}

func (msgu *MsgUBlock) Serialize(w io.Writer) (err error) {
	err = msgu.MsgBlock.Serialize(w)
	if err != nil {
		return
	}
	err = msgu.UtreexoData.Serialize(w)
	return
}

func (msgu *MsgUBlock) SerializeNoWitness(w io.Writer) error {
	return msgu.BtcEncode(w, 0, BaseEncoding)
}

// SerializeSize: how big is it, in bytes.
func (msgb *MsgUBlock) SerializeSize() int {
	return msgb.MsgBlock.SerializeSize() + msgb.UtreexoData.SerializeSize()
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgUBlock) Command() string {
	return CmdUBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgUBlock) MaxPayloadLength(pver uint32) uint32 {
	// Block header at 80 bytes + transaction count + max transactions
	// which can vary up to the MaxBlockPayload (including the block header
	// and transaction count).
	return MaxBlockPayload + 4000000
}

// NewMsgUBlock returns a new bitcoin block message that conforms to the
// Message interface.  See MsgUBlock for details.
func NewMsgUBlock(msgBlock MsgBlock, udata btcacc.UData) *MsgUBlock {
	return &MsgUBlock{
		MsgBlock:    msgBlock,
		UtreexoData: udata,
	}
}
