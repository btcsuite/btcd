package change

import (
	"bytes"
	"encoding/binary"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"
)

type ChangeType uint32

const (
	AddClaim ChangeType = iota
	SpendClaim
	UpdateClaim
	AddSupport
	SpendSupport
)

type Change struct {
	Type   ChangeType
	Height int32

	Name     []byte
	ClaimID  ClaimID
	OutPoint wire.OutPoint
	Amount   int64

	ActiveHeight  int32
	VisibleHeight int32 // aka, CreatedAt; used for normalization fork

	SpentChildren map[string]bool
}

func NewChange(typ ChangeType) Change {
	return Change{Type: typ}
}

func (c Change) SetHeight(height int32) Change {
	c.Height = height
	return c
}

func (c Change) SetName(name []byte) Change {
	c.Name = name // need to clone it?
	return c
}

func (c Change) SetOutPoint(op *wire.OutPoint) Change {
	c.OutPoint = *op
	return c
}

func (c Change) SetAmount(amt int64) Change {
	c.Amount = amt
	return c
}

func (c *Change) Marshal(enc *bytes.Buffer) error {
	enc.Write(c.ClaimID[:])
	enc.Write(c.OutPoint.Hash[:])
	var temp [8]byte
	binary.BigEndian.PutUint32(temp[:4], c.OutPoint.Index)
	enc.Write(temp[:4])
	binary.BigEndian.PutUint32(temp[:4], uint32(c.Type))
	enc.Write(temp[:4])
	binary.BigEndian.PutUint32(temp[:4], uint32(c.Height))
	enc.Write(temp[:4])
	binary.BigEndian.PutUint32(temp[:4], uint32(c.ActiveHeight))
	enc.Write(temp[:4])
	binary.BigEndian.PutUint32(temp[:4], uint32(c.VisibleHeight))
	enc.Write(temp[:4])
	binary.BigEndian.PutUint64(temp[:], uint64(c.Amount))
	enc.Write(temp[:])

	if c.SpentChildren != nil {
		binary.BigEndian.PutUint32(temp[:4], uint32(len(c.SpentChildren)))
		enc.Write(temp[:4])
		for key := range c.SpentChildren {
			binary.BigEndian.PutUint16(temp[:2], uint16(len(key))) // technically limited to 255; not sure we trust it
			enc.Write(temp[:2])
			enc.WriteString(key)
		}
	} else {
		binary.BigEndian.PutUint32(temp[:4], 0)
		enc.Write(temp[:4])
	}
	return nil
}

func (c *Change) Unmarshal(dec *bytes.Buffer) error {
	copy(c.ClaimID[:], dec.Next(ClaimIDSize))
	copy(c.OutPoint.Hash[:], dec.Next(chainhash.HashSize))
	c.OutPoint.Index = binary.BigEndian.Uint32(dec.Next(4))
	c.Type = ChangeType(binary.BigEndian.Uint32(dec.Next(4)))
	c.Height = int32(binary.BigEndian.Uint32(dec.Next(4)))
	c.ActiveHeight = int32(binary.BigEndian.Uint32(dec.Next(4)))
	c.VisibleHeight = int32(binary.BigEndian.Uint32(dec.Next(4)))
	c.Amount = int64(binary.BigEndian.Uint64(dec.Next(8)))
	keys := binary.BigEndian.Uint32(dec.Next(4))
	if keys > 0 {
		c.SpentChildren = map[string]bool{}
	}
	for keys > 0 {
		keys--
		keySize := int(binary.BigEndian.Uint16(dec.Next(2)))
		key := string(dec.Next(keySize))
		c.SpentChildren[key] = true
	}
	return nil
}
