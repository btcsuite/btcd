package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

const (
	// testWSUser and testWSPass are the RPC credentials of the websocket
	// test server.
	testWSUser = "user"
	testWSPass = "pass"
)

// testRPCLog collects what the RPC server logs during the tests. TestMain
// installs it.
var testRPCLog = &syncBuffer{}

// lastTestClientPort numbers the in-memory connections of the websocket tests,
// so that each gets its own client address and rpcLogFor tells their log
// lines apart.
var lastTestClientPort atomic.Int32

// rpcLogFor returns the lines of testRPCLog about the client connected through
// conn. The server logs clients by their address.
func rpcLogFor(conn *websocket.Conn) string {
	addr := conn.LocalAddr().String()

	var lines []string
	for _, line := range strings.Split(testRPCLog.String(), "\n") {
		if strings.Contains(line, addr) {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// syncBuffer is a bytes.Buffer that is safe for concurrent use, so that
// server goroutines can log into it while a test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the buffer.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

// String returns the contents of the buffer.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// pipeListener is a net.Listener whose connections are in-memory pipes made by
// its dial method. Unlike loopback connections, pipes let the websocket tests
// run in a synctest bubble.
type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

// newPipeListener returns a listener for in-memory connections.
func newPipeListener() *pipeListener {
	return &pipeListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

// Accept returns the server end of the next connection made by dial.
func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil

	case <-l.closed:
		return nil, net.ErrClosed
	}
}

// Close stops the listener.
func (l *pipeListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
	})

	return nil
}

// Addr returns the address of the listener.
func (l *pipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}
}

// dial connects to the listener through an in-memory pipe. Each connection
// gets its own client address, which the server sees as the remote address.
func (l *pipeListener) dial(ctx context.Context, _, _ string) (net.Conn,
	error) {

	port := 50000 + int(lastTestClientPort.Add(1))
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}

	server, client := net.Pipe()
	select {
	case l.conns <- &addrConn{Conn: server, remoteAddr: addr}:
		return &addrConn{Conn: client, localAddr: addr}, nil

	case <-l.closed:
		_, _ = server.Close(), client.Close()
		return nil, net.ErrClosed

	case <-ctx.Done():
		_, _ = server.Close(), client.Close()
		return nil, ctx.Err()
	}
}

// addrConn is a connection that reports the given local and remote addresses,
// when set, instead of those of the underlying connection.
type addrConn struct {
	net.Conn

	localAddr  net.Addr
	remoteAddr net.Addr
}

// LocalAddr returns the local address of the connection.
func (c *addrConn) LocalAddr() net.Addr {
	if c.localAddr != nil {
		return c.localAddr
	}

	return c.Conn.LocalAddr()
}

// RemoteAddr returns the remote address of the connection.
func (c *addrConn) RemoteAddr() net.Addr {
	if c.remoteAddr != nil {
		return c.remoteAddr
	}

	return c.Conn.RemoteAddr()
}

// testWebsocketServer serves the websocket endpoint of an RPC server that has
// no chain behind it, which is enough to exercise authentication and request
// parsing.
type testWebsocketServer struct {
	// listener makes the in-memory connections to the server.
	listener *pipeListener

	// done receives a value each time the server finishes handling a
	// connection.
	done chan struct{}
}

// newTestWebsocketServer starts a websocket test server. When authenticated
// is set, clients are treated as having passed HTTP basic auth during the
// upgrade; otherwise they have to send the authenticate command.
//
// It must be called in a synctest bubble, so the server runs on the bubble's
// fake clock, and a server that never does what a test waits for makes the
// bubble deadlock, which fails the test. The server swaps the global config,
// so tests using it must not run in parallel.
func newTestWebsocketServer(t *testing.T,
	authenticated bool) *testWebsocketServer {

	t.Helper()

	oldCfg := cfg
	cfg = &config{RPCMaxWebsockets: 10, RPCMaxConcurrentReqs: 10}
	t.Cleanup(func() {
		cfg = oldCfg
	})

	login := testWSUser + ":" + testWSPass
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
	s := &rpcServer{authsha: sha256.Sum256([]byte(auth))}
	s.ntfnMgr = newWsNotificationManager(s)
	s.ntfnMgr.Start()
	t.Cleanup(func() {
		s.ntfnMgr.Shutdown()
		s.ntfnMgr.WaitForShutdown()
	})

	// Wait for all connections to be handled before the config is restored.
	// Hijacked connections aren't tracked by the HTTP server.
	var handlers sync.WaitGroup
	t.Cleanup(handlers.Wait)

	done := make(chan struct{}, 10)
	srv := &http.Server{Handler: http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			handlers.Add(1)
			defer handlers.Done()

			ws, err := websocket.Upgrade(w, r, nil, 0, 0)
			if err != nil {
				return
			}
			s.WebsocketHandler(
				ws, r.RemoteAddr, authenticated, true,
			)
			done <- struct{}{}
		},
	)}
	listener := newPipeListener()
	go func() {
		_ = srv.Serve(listener)
	}()
	t.Cleanup(func() {
		_ = srv.Close()
	})

	return &testWebsocketServer{
		listener: listener,
		done:     done,
	}
}

// dial connects a websocket client to the test server. The connection is
// closed when the test ends.
func (s *testWebsocketServer) dial(t *testing.T) *websocket.Conn {
	t.Helper()

	dialer := websocket.Dialer{NetDialContext: s.listener.dial}
	conn, _, err := dialer.Dial("ws://btcd/ws", nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn
}

// waitDone waits for the server to finish handling a connection.
func (s *testWebsocketServer) waitDone() {
	<-s.done
}

// TestWebsocketNormalCloseNotLogged checks that a client closing the
// connection with a normal close frame isn't logged as a receive error.
func TestWebsocketNormalCloseNotLogged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestWebsocketServer(t, true)
		conn := s.dial(t)

		msg := websocket.FormatCloseMessage(
			websocket.CloseNormalClosure, "",
		)
		deadline := time.Now().Add(time.Second)
		err := conn.WriteControl(websocket.CloseMessage, msg, deadline)
		require.NoError(t, err)
		s.waitDone()

		log := rpcLogFor(conn)
		require.Contains(t, log, "Disconnected websocket client")
		require.NotContains(t, log, "Websocket receive error")
	})
}

// authenticateRequest returns an authenticate command with the credentials of
// the websocket test server.
func authenticateRequest(jsonrpc string) string {
	return fmt.Sprintf(`{"jsonrpc":%q,"id":1,"method":"authenticate",`+
		`"params":[%q,%q]}`, jsonrpc, testWSUser, testWSPass)
}

// malformedRequest returns a request without a method whose ID is a string of
// idSize bytes. The server answers it with an error that echoes the ID.
func malformedRequest(idSize int) string {
	return fmt.Sprintf(`{"jsonrpc":"1.0","id":"%s","params":[]}`,
		strings.Repeat("a", idSize))
}

// send writes msg to conn as a text message.
func send(t *testing.T, conn *websocket.Conn, msg string) {
	t.Helper()

	err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
	require.NoError(t, err)
}

// readReply reads the next message from conn.
func readReply(t *testing.T, conn *websocket.Conn) string {
	t.Helper()

	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	return string(msg)
}

// requireClosed checks that the server closes the connection without sending
// anything.
func requireClosed(t *testing.T, s *testWebsocketServer,
	conn *websocket.Conn) {

	t.Helper()

	s.waitDone()

	_, msg, err := conn.ReadMessage()
	require.Error(t, err, "unexpected message %q", msg)
}

// TestWebsocketUnauthenticatedReadLimit checks that a client that hasn't
// authenticated can't send a message bigger than the unauthenticated limit.
func TestWebsocketUnauthenticatedReadLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestWebsocketServer(t, false)
		conn := s.dial(t)

		// The server stops reading once the message exceeds the limit
		// and closes the connection, so sending it fails midway.
		msg := malformedRequest(websocketReadLimitUnauthenticated)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(msg))
		requireClosed(t, s, conn)
		require.Contains(t, rpcLogFor(conn), "read limit exceeded")
	})
}

// TestWebsocketUnauthenticatedMalformedRequest checks that a client that
// hasn't authenticated is disconnected without a reply when it sends a
// request without a method, so it can't make the server queue replies.
func TestWebsocketUnauthenticatedMalformedRequest(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{
			name: "single",
			msg:  `{"jsonrpc":"1.0","id":1}`,
		},
		{
			name: "batch",
			msg:  `[{"jsonrpc":"2.0","id":1}]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := newTestWebsocketServer(t, false)
				conn := s.dial(t)

				send(t, conn, test.msg)
				requireClosed(t, s, conn)
			})
		})
	}
}

// TestWebsocketAuthTimeout checks that a client that doesn't authenticate is
// disconnected once the authentication timeout expires.
func TestWebsocketAuthTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestWebsocketServer(t, false)
		conn := s.dial(t)

		start := time.Now()
		requireClosed(t, s, conn)
		require.Equal(t, websocketAuthTimeout, time.Since(start))
	})
}

// TestWebsocketAuthenticate checks that a client that authenticates with the
// authenticate command, alone or in a batch, stays connected past the
// authentication timeout and may then send messages bigger than the
// unauthenticated limit.
func TestWebsocketAuthenticate(t *testing.T) {
	tests := []struct {
		name string
		msg  string
	}{
		{
			name: "single",
			msg:  authenticateRequest("1.0"),
		},
		{
			name: "batch",
			msg:  "[" + authenticateRequest("2.0") + "]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := newTestWebsocketServer(t, false)
				conn := s.dial(t)

				send(t, conn, test.msg)
				reply := readReply(t, conn)
				require.NotContains(t, reply, `"code"`)

				time.Sleep(2 * websocketAuthTimeout)
				send(t, conn, malformedRequest(1<<20))
				reply = readReply(t, conn)
				require.Contains(
					t, reply, "Invalid request: malformed",
				)
			})
		})
	}
}

// TestWebsocketHTTPAuthenticated checks that a client that authenticated
// during the HTTP upgrade has no authentication timeout and may send messages
// bigger than the unauthenticated limit right away.
func TestWebsocketHTTPAuthenticated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newTestWebsocketServer(t, true)
		conn := s.dial(t)

		time.Sleep(2 * websocketAuthTimeout)
		send(t, conn, malformedRequest(1<<20))
		reply := readReply(t, conn)
		require.Contains(t, reply, "Invalid request: malformed")
	})
}
