//go:build rpctest

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/btcsuite/btcd/debugstream"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/stretchr/testify/require"
)

func TestDebugStream(t *testing.T) {
	const (
		sBegin = iota
		sNodeStarted
		sNodeShutdown
	)
	var state uint64
	ctx, cancel := context.WithTimeout(t.Context(), time.Second*10)
	defer cancel()
	debHandler := func(e debugstream.Event) {
		switch {
		case state == sBegin && e.Code == debugstream.DEStart:
			state = sNodeStarted

		case state == sNodeStarted && e.Code == debugstream.DEShutdown:
			state = sNodeShutdown
			cancel()
		}
	}

	h, err := rpctest.New(rpctest.Opts{DebugHandler: debHandler})
	require.NoError(t, err)
	h.SetUp()

	err = h.TearDown()
	require.NoError(t, err)

	<-ctx.Done()
	require.Equal(t, true, sNodeShutdown == state)
}
