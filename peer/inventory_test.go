package peer

import (
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestKnownInventoryByValue checks that known inventory is matched by its
// type and hash rather than by pointer, so that inventory a peer announced is
// known when it arrives again in another message.
func TestKnownInventoryByValue(t *testing.T) {
	p := NewInboundPeer(&Config{})

	hash := chainhash.Hash{1}
	p.AddKnownInventory(wire.NewInvVect(wire.InvTypeTx, &hash))

	require.True(t, p.knowsInventory(
		wire.NewInvVect(wire.InvTypeTx, &hash),
	))
	require.False(t, p.knowsInventory(
		wire.NewInvVect(wire.InvTypeBlock, &hash),
	))

	other := chainhash.Hash{2}
	require.False(t, p.knowsInventory(
		wire.NewInvVect(wire.InvTypeTx, &other),
	))
}
