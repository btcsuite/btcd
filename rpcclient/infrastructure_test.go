package rpcclient

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseAddressString checks different variation of supported and
// unsupported addresses.
func TestParseAddressString(t *testing.T) {
	t.Parallel()

	// Using localhost only to avoid network calls.
	testCases := []struct {
		name          string
		addressString string
		expNetwork    string
		expAddress    string
		expErrStr     string
	}{
		{
			name:          "localhost",
			addressString: "localhost",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost ip",
			addressString: "127.0.0.1",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost ipv6",
			addressString: "::1",
			expNetwork:    "tcp",
			expAddress:    "[::1]:0",
		},
		{
			name:          "localhost and port",
			addressString: "localhost:80",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:80",
		},
		{
			name:          "localhost ipv6 and port",
			addressString: "[::1]:80",
			expNetwork:    "tcp",
			expAddress:    "[::1]:80",
		},
		{
			name:          "colon and port",
			addressString: ":80",
			expNetwork:    "tcp",
			expAddress:    ":80",
		},
		{
			name:          "colon only",
			addressString: ":",
			expNetwork:    "tcp",
			expAddress:    ":0",
		},
		{
			name:          "localhost and path",
			addressString: "localhost/path",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost port and path",
			addressString: "localhost:80/path",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:80",
		},
		{
			name:          "unix prefix",
			addressString: "unix://the/rest/of/the/path",
			expNetwork:    "unix",
			expAddress:    "the/rest/of/the/path",
		},
		{
			name:          "unix prefix",
			addressString: "unixpacket://the/rest/of/the/path",
			expNetwork:    "unixpacket",
			expAddress:    "the/rest/of/the/path",
		},
		{
			name:          "error http prefix",
			addressString: "http://localhost:1010",
			expErrStr:     "unsupported protocol in address",
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			addr, err := ParseAddressString(tc.addressString)
			if tc.expErrStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expNetwork, addr.Network())
			require.Equal(t, tc.expAddress, addr.String())
		})
	}
}

// TestHTTPPostShutdownInterruptsPendingRequest ensures that a client operating
// in HTTP POST mode can interrupt an in-flight request during shutdown.
func TestHTTPPostShutdownInterruptsPendingRequest(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	requestAccepted := make(chan struct{})
	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)

		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() {
			err := conn.Close()
			assert.NoError(t, err)
		}()

		close(requestAccepted)

		_, _ = io.Copy(io.Discard, conn)
	}()

	t.Cleanup(func() {
		err := listener.Close()
		require.NoError(t, err)
		<-serverDone
	})

	connCfg := &ConnConfig{
		Host:         listener.Addr().String(),
		User:         "user",
		Pass:         "pass",
		DisableTLS:   true,
		HTTPPostMode: true,
	}

	client, err := New(connCfg, nil)
	require.NoError(t, err)
	t.Cleanup(client.Shutdown)

	future := client.GetBlockCountAsync()

	select {
	case <-requestAccepted:
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not accept client connection")
	}

	select {
	case <-future:
		t.Fatalf("expected request to remain pending until shutdown")
	case <-time.After(100 * time.Millisecond):
	}

	client.Shutdown()

	waitDone := make(chan struct{})
	go func() {
		client.WaitForShutdown()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("client shutdown did not complete")
	}

	result, err := future.Receive()
	require.Zero(t, result)
	require.ErrorContains(t, err, ErrClientShutdown.Error())
}
