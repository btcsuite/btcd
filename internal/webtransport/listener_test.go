package webtransport

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/require"
)

const testTimeout = 5 * time.Second

type testEndpoint struct {
	listener  *Listener
	transport *wt.Transport
	url       string
	dialed    bool
}

func (e *testEndpoint) dialRawQUIC(t *testing.T,
	config *quic.Config) *quic.Conn {

	t.Helper()
	conn, err := e.dialRawQUICResult(config)
	require.NoError(t, err)
	e.cleanupRawQUIC(t, conn)

	return conn
}

func (e *testEndpoint) dialRawQUICResult(
	config *quic.Config) (*quic.Conn, error) {

	tlsConfig := e.transport.TLSClientConfig.Clone()
	tlsConfig.NextProtos = []string{http3.NextProtoH3}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	return quic.DialAddr(
		ctx, e.listener.Addr().String(), tlsConfig, config,
	)
}

func (e *testEndpoint) cleanupRawQUIC(t *testing.T, conn *quic.Conn) {
	t.Helper()

	t.Cleanup(func() {
		_ = conn.CloseWithError(
			quic.ApplicationErrorCode(http3.ErrCodeNoError), "",
		)
	})
}

func newTestEndpoint(t *testing.T, config Config) *testEndpoint {
	t.Helper()

	serverTLS, clientTLS := testTLSConfigs(t)
	config.TLSConfig = serverTLS

	listener, err := Listen("udp4", "127.0.0.1:0", config)
	require.NoError(t, err)

	transport := &wt.Transport{TLSClientConfig: clientTLS}
	endpoint := &testEndpoint{
		listener:  listener,
		transport: transport,
		url:       "https://" + listener.Addr().String(),
	}

	t.Cleanup(func() {
		require.NoError(t, listener.Close())
		if endpoint.dialed {
			require.NoError(t, transport.Close())
		}
	})

	return endpoint
}

func (e *testEndpoint) dial(
	t *testing.T, path, origin string,
) (*http.Response, *wt.Session, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	header := make(http.Header)
	if origin != "" {
		header.Set("Origin", origin)
	}

	e.dialed = true
	return e.transport.Dial(ctx, e.url+path, header)
}

func TestListenerCarriesRawBytes(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{})
	// Negotiate per-session flow control so the raw byte test catches a zero
	// server receive window.  Other tests leave this disabled where they need
	// to exercise the listener's application-level stream rejection.
	endpoint.transport.Config = &wt.Config{
		MaxIncomingData: defaultSessionReceiveWindow,
	}
	accepted := make(chan net.Conn, 1)
	acceptErrors := make(chan error, 1)
	go func() {
		conn, err := endpoint.listener.Accept()
		if err != nil {
			acceptErrors <- err
			return
		}
		accepted <- conn
	}()

	response, session, err := endpoint.dial(t, DefaultPath, endpoint.url)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	clientStream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	require.NoError(t, clientStream.SetDeadline(time.Now().Add(testTimeout)))
	clientMessage := []byte{0xf9, 0xbe, 0xb4, 0xd9, 0x01, 0x02, 0x03}
	_, err = clientStream.Write(clientMessage)
	require.NoError(t, err)

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case err := <-acceptErrors:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("listener did not accept the WebTransport stream")
	}

	require.NotNil(t, serverConn.LocalAddr())
	require.NotNil(t, serverConn.RemoteAddr())
	require.NoError(t, serverConn.SetDeadline(time.Now().Add(testTimeout)))

	serverMessage := make([]byte, len(clientMessage))
	_, err = io.ReadFull(serverConn, serverMessage)
	require.NoError(t, err)
	require.Equal(t, clientMessage, serverMessage)

	reply := []byte{0x0b, 0x11, 0x09}
	_, err = serverConn.Write(reply)
	require.NoError(t, err)
	clientReply := make([]byte, len(reply))
	_, err = io.ReadFull(clientStream, clientReply)
	require.NoError(t, err)
	require.Equal(t, reply, clientReply)

	// Closing the net.Conn closes both stream directions and the containing
	// session.  A second Close is harmless.
	require.NoError(t, serverConn.Close())
	require.NoError(t, serverConn.Close())
	select {
	case <-session.Context().Done():
	case <-ctx.Done():
		t.Fatal("closing the accepted connection did not close its session")
	}
}

func TestListenerOriginAndPathPolicy(t *testing.T) {
	t.Parallel()

	const allowedOrigin = "https://wallet.example"
	endpoint := newTestEndpoint(t, Config{
		AllowedOrigins: []string{allowedOrigin},
	})

	tests := []struct {
		name       string
		path       string
		origin     string
		wantStatus int
	}{
		{
			name:       "same origin",
			path:       DefaultPath,
			origin:     endpoint.url,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing origin for non-browser client",
			path:       DefaultPath,
			wantStatus: http.StatusOK,
		},
		{
			name:       "exact allowed cross origin",
			path:       DefaultPath,
			origin:     allowedOrigin,
			wantStatus: http.StatusOK,
		},
		{
			name:       "disallowed cross origin",
			path:       DefaultPath,
			origin:     "https://evil.example",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrong path",
			path:       "/not-p2p",
			origin:     endpoint.url,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, session, err := endpoint.dial(
				t, test.path, test.origin,
			)
			require.NotNil(t, response)
			require.Equal(t, test.wantStatus, response.StatusCode)

			if test.wantStatus == http.StatusOK {
				require.NoError(t, err)
				require.NotNil(t, session)
				require.NoError(t, session.CloseWithError(0, ""))
				return
			}

			require.Error(t, err)
			require.Nil(t, session)
		})
	}
}

func TestListenerAllowsAnyOrigin(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{
		AllowedOrigins: []string{"*"},
	})

	for _, origin := range []string{
		"https://wallet.example",
		"https://other.example:8443",
	} {
		response, session, err := endpoint.dial(
			t, DefaultPath, origin,
		)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.NotNil(t, session)
		require.NoError(t, session.CloseWithError(0, ""))
	}

	for _, origin := range []string{"null", "not-an-origin"} {
		response, session, err := endpoint.dial(
			t, DefaultPath, origin,
		)
		require.Error(t, err)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.Nil(t, session)
	}

	request := &http.Request{
		Host: endpoint.listener.Addr().String(),
		Header: http.Header{
			"Origin": {
				"https://wallet.example",
				"https://other.example",
			},
		},
	}
	require.False(t, endpoint.listener.checkOrigin(request))
}

func TestListenerBoundsPendingSessions(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{
		MaxPendingSessions: 1,
		FirstStreamTimeout: 200 * time.Millisecond,
	})

	response, stalledSession, err := endpoint.dial(
		t, DefaultPath, endpoint.url,
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)

	response, session, err := endpoint.dial(t, DefaultPath, endpoint.url)
	require.Error(t, err)
	require.Nil(t, session)
	require.NotNil(t, response)
	require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)

	select {
	case <-stalledSession.Context().Done():
	case <-time.After(testTimeout):
		t.Fatal("session without a first stream did not time out")
	}

	// The timeout releases the pending slot for a later peer.
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := endpoint.listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	var activeSession *wt.Session
	require.Eventually(t, func() bool {
		response, candidate, dialErr := endpoint.dial(
			t, DefaultPath, endpoint.url,
		)
		if dialErr != nil || response.StatusCode != http.StatusOK {
			return false
		}
		activeSession = candidate
		return true
	}, testTimeout, 20*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	activeStream, err := activeSession.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = activeStream.Write([]byte{0x01})
	require.NoError(t, err)

	select {
	case conn := <-accepted:
		require.NoError(t, conn.Close())
	case <-ctx.Done():
		t.Fatal("listener did not accept the session after the slot was released")
	}
}

func TestListenerExpiresConnectionWithoutCONNECT(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{
		FirstRequestTimeout: 80 * time.Millisecond,
		KeepAlivePeriod:     10 * time.Millisecond,
	})
	conn := endpoint.dialRawQUIC(t, &quic.Config{
		MaxIdleTimeout: 30 * time.Millisecond,
	})

	select {
	case <-conn.Context().Done():
	case <-time.After(testTimeout):
		t.Fatal("connection without CONNECT did not expire")
	}

	var applicationError *quic.ApplicationError
	cause := context.Cause(conn.Context())
	if !errors.As(cause, &applicationError) {
		t.Fatalf("unexpected connection close cause %T: %v", cause, cause)
	}
	require.Equal(t,
		quic.ApplicationErrorCode(http3.ErrCodeRequestRejected),
		applicationError.ErrorCode,
	)
}

func TestListenerBoundsConnectionsWithoutCONNECT(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{
		MaxPendingConnections: 1,
		FirstRequestTimeout:   time.Second,
		KeepAlivePeriod:       10 * time.Millisecond,
	})
	first := endpoint.dialRawQUIC(t, &quic.Config{})
	require.Eventually(t, func() bool {
		return len(endpoint.listener.pendingConn) == 1
	}, testTimeout, time.Millisecond)

	second, err := endpoint.dialRawQUICResult(&quic.Config{})
	if err != nil {
		require.ErrorContains(t, err, "APPLICATION_ERROR")
	} else {
		endpoint.cleanupRawQUIC(t, second)
		select {
		case <-second.Context().Done():
		case <-time.After(testTimeout):
			t.Fatal("excess pending connection was not rejected")
		}
	}

	select {
	case <-first.Context().Done():
		t.Fatal("first pending connection was unexpectedly closed")
	default:
	}
}

func TestListenerKeepAlivePreservesIdlePeer(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{
		KeepAlivePeriod: 10 * time.Millisecond,
	})
	endpoint.transport.QUICConfig = &quic.Config{
		EnableDatagrams:                  true,
		EnableStreamResetPartialDelivery: true,
		MaxIdleTimeout:                   40 * time.Millisecond,
	}

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := endpoint.listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	_, session, err := endpoint.dial(t, DefaultPath, endpoint.url)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	stream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = stream.Write([]byte{0x00})
	require.NoError(t, err)

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case <-ctx.Done():
		t.Fatal("listener did not accept idle peer")
	}
	defer func() {
		_ = serverConn.Close()
	}()
	initial := make([]byte, 1)
	_, err = io.ReadFull(serverConn, initial)
	require.NoError(t, err)
	require.Equal(t, []byte{0x00}, initial)

	// The client advertises an idle timeout shorter than btcd's two-minute
	// Bitcoin ping cadence. QUIC keepalives must preserve the peer until
	// application traffic resumes.
	time.Sleep(150 * time.Millisecond)
	select {
	case <-session.Context().Done():
		t.Fatal("idle WebTransport peer closed before Bitcoin ping cadence")
	default:
	}

	require.NoError(t, stream.SetDeadline(time.Now().Add(testTimeout)))
	_, err = stream.Write([]byte{0x01})
	require.NoError(t, err)
	message := make([]byte, 1)
	_, err = io.ReadFull(serverConn, message)
	require.NoError(t, err)
	require.Equal(t, []byte{0x01}, message)
}

func TestListenerRejectsAdditionalStreams(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{})
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := endpoint.listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	_, session, err := endpoint.dial(t, DefaultPath, endpoint.url)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	firstStream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = firstStream.Write([]byte{0x01})
	require.NoError(t, err)

	select {
	case conn := <-accepted:
		defer func() {
			_ = conn.Close()
		}()
	case <-ctx.Done():
		t.Fatal("listener did not accept the first stream")
	}

	secondStream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = secondStream.Write([]byte{0x02})
	require.NoError(t, err)
	select {
	case <-session.Context().Done():
	case <-ctx.Done():
		t.Fatal("session remained open after a second stream")
	}
}

func TestListenerRejectsDatagrams(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{})
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := endpoint.listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	_, session, err := endpoint.dial(t, DefaultPath, endpoint.url)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	firstStream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = firstStream.Write([]byte{0x01})
	require.NoError(t, err)

	select {
	case conn := <-accepted:
		defer func() {
			_ = conn.Close()
		}()
	case <-ctx.Done():
		t.Fatal("listener did not accept the first stream")
	}

	require.NoError(t, session.SendDatagram([]byte{0x02}))
	select {
	case <-session.Context().Done():
	case <-ctx.Done():
		t.Fatal("session remained open after a datagram")
	}
}

func TestListenerCloseUnblocksAcceptAndClosesSessions(t *testing.T) {
	t.Parallel()

	endpoint := newTestEndpoint(t, Config{})
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := endpoint.listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	_, session, err := endpoint.dial(t, DefaultPath, endpoint.url)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()
	clientStream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	_, err = clientStream.Write([]byte{0x01})
	require.NoError(t, err)

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case <-ctx.Done():
		t.Fatal("listener did not accept the active session")
	}
	require.NoError(t, serverConn.SetReadDeadline(time.Now().Add(testTimeout)))
	marker := make([]byte, 1)
	_, err = io.ReadFull(serverConn, marker)
	require.NoError(t, err)

	acceptResult := make(chan error, 1)
	go func() {
		_, err := endpoint.listener.Accept()
		acceptResult <- err
	}()

	require.NoError(t, endpoint.listener.Close())
	select {
	case err := <-acceptResult:
		require.ErrorIs(t, err, net.ErrClosed)
	case <-time.After(testTimeout):
		t.Fatal("closing the listener did not unblock Accept")
	}
	select {
	case <-session.Context().Done():
	case <-ctx.Done():
		t.Fatal("closing the listener did not close the active session")
	}
	_, err = serverConn.Read(marker)
	require.Error(t, err)
	require.NoError(t, serverConn.Close())

	require.NoError(t, endpoint.listener.Close())
}

func TestNormalizeConfig(t *testing.T) {
	t.Parallel()

	tlsConfig := &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return nil, nil
		},
	}
	normalized, err := normalizeConfig(Config{TLSConfig: tlsConfig})
	require.NoError(t, err)
	require.Equal(t, DefaultPath, normalized.path)
	require.False(t, normalized.allowAnyOrigin)
	require.Equal(t, DefaultMaxPendingConnections,
		normalized.maxPendingConnections)
	require.Equal(t, DefaultMaxPendingSessions, normalized.maxPendingSessions)
	require.Equal(t, DefaultFirstRequestTimeout,
		normalized.firstRequestTimeout)
	require.Equal(t, DefaultFirstStreamTimeout, normalized.firstStreamTimeout)
	require.Equal(t, DefaultMaxIdleTimeout, normalized.maxIdleTimeout)
	require.Equal(t, DefaultKeepAlivePeriod, normalized.keepAlivePeriod)
	require.NotSame(t, tlsConfig, normalized.tlsConfig)

	normalized, err = normalizeConfig(Config{
		TLSConfig:      tlsConfig,
		AllowedOrigins: []string{"*"},
	})
	require.NoError(t, err)
	require.True(t, normalized.allowAnyOrigin)

	for _, test := range []struct {
		name   string
		config Config
	}{
		{name: "missing TLS", config: Config{}},
		{
			name: "TLS without certificate",
			config: Config{
				TLSConfig: &tls.Config{},
			},
		},
		{
			name: "relative path",
			config: Config{
				TLSConfig: tlsConfig,
				Path:      "p2p",
			},
		},
		{
			name: "partial origin wildcard",
			config: Config{
				TLSConfig:      tlsConfig,
				AllowedOrigins: []string{"https://*.example.com"},
			},
		},
		{
			name: "allow any does not mask invalid origin",
			config: Config{
				TLSConfig: tlsConfig,
				AllowedOrigins: []string{
					"*", "https://*.example.com",
				},
			},
		},
		{
			name: "origin path",
			config: Config{
				TLSConfig:      tlsConfig,
				AllowedOrigins: []string{"https://example.com/path"},
			},
		},
		{
			name: "negative pending limit",
			config: Config{
				TLSConfig:          tlsConfig,
				MaxPendingSessions: -1,
			},
		},
		{
			name: "negative pending connection limit",
			config: Config{
				TLSConfig:             tlsConfig,
				MaxPendingConnections: -1,
			},
		},
		{
			name: "negative first request timeout",
			config: Config{
				TLSConfig:           tlsConfig,
				FirstRequestTimeout: -time.Second,
			},
		},
		{
			name: "negative timeout",
			config: Config{
				TLSConfig:          tlsConfig,
				FirstStreamTimeout: -time.Second,
			},
		},
		{
			name: "negative keepalive",
			config: Config{
				TLSConfig:       tlsConfig,
				KeepAlivePeriod: -time.Second,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeConfig(test.config)
			require.Error(t, err)
		})
	}
}

type fakeStream struct {
	bytes.Buffer
	cancelReadCount  int
	cancelWriteCount int
	deadlines        []time.Time
	readErr          error
	writeErr         error
}

func (s *fakeStream) Read(buffer []byte) (int, error) {
	if s.readErr != nil {
		return 0, s.readErr
	}

	return s.Buffer.Read(buffer)
}

func (s *fakeStream) Write(buffer []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}

	return s.Buffer.Write(buffer)
}

func (s *fakeStream) CancelRead(wt.StreamErrorCode) {
	s.cancelReadCount++
}

func (s *fakeStream) CancelWrite(wt.StreamErrorCode) {
	s.cancelWriteCount++
}

func (s *fakeStream) SetDeadline(deadline time.Time) error {
	s.deadlines = append(s.deadlines, deadline)
	return nil
}

func (s *fakeStream) SetReadDeadline(deadline time.Time) error {
	s.deadlines = append(s.deadlines, deadline)
	return nil
}

func (s *fakeStream) SetWriteDeadline(deadline time.Time) error {
	s.deadlines = append(s.deadlines, deadline)
	return nil
}

type fakeSession struct {
	mutex      sync.Mutex
	closeCount int
	closeErr   error
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (s *fakeSession) CloseWithError(wt.SessionErrorCode, string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.closeCount++
	return s.closeErr
}

func (s *fakeSession) LocalAddr() net.Addr {
	return s.localAddr
}

func (s *fakeSession) RemoteAddr() net.Addr {
	return s.remoteAddr
}

func TestStreamConnFullClose(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close error")
	stream := &fakeStream{}
	session := &fakeSession{
		closeErr:   closeErr,
		localAddr:  &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4433},
		remoteAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 50000},
	}
	connectionCloseCount := 0
	conn := newStreamConn(stream, session, func() error {
		connectionCloseCount++
		return nil
	})

	deadline := time.Now().Add(time.Second)
	require.NoError(t, conn.SetDeadline(deadline))
	require.NoError(t, conn.SetReadDeadline(deadline))
	require.NoError(t, conn.SetWriteDeadline(deadline))
	require.Len(t, stream.deadlines, 3)
	require.Equal(t, session.localAddr, conn.LocalAddr())
	require.Equal(t, session.remoteAddr, conn.RemoteAddr())

	require.ErrorIs(t, conn.Close(), closeErr)
	require.ErrorIs(t, conn.Close(), closeErr)
	require.Equal(t, 1, stream.cancelReadCount)
	require.Equal(t, 1, stream.cancelWriteCount)
	require.Equal(t, 1, session.closeCount)
	require.Equal(t, 1, connectionCloseCount)
}

func TestStreamConnNormalRemoteClose(t *testing.T) {
	t.Parallel()

	remoteClose := &wt.SessionError{Remote: true, ErrorCode: 0}
	stream := &fakeStream{
		readErr:  remoteClose,
		writeErr: remoteClose,
	}
	conn := newStreamConn(stream, &fakeSession{}, nil)

	_, err := conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
	_, err = conn.Write([]byte{1})
	require.ErrorIs(t, err, net.ErrClosed)

	abnormalClose := &wt.SessionError{
		Remote:    true,
		ErrorCode: 1,
		Message:   "abnormal close",
	}
	stream.readErr = abnormalClose
	_, err = conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, abnormalClose)
}

func testTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "WebTransport test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	certificateDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &privateKey.PublicKey, privateKey,
	)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(certificateDER)
	require.NoError(t, err)

	rootCertificates := x509.NewCertPool()
	rootCertificates.AddCert(certificate)

	return &tls.Config{
			Certificates: []tls.Certificate{{
				Certificate: [][]byte{certificateDER},
				PrivateKey:  privateKey,
			}},
			MinVersion: tls.VersionTLS13,
		}, &tls.Config{
			RootCAs:    rootCertificates,
			ServerName: "127.0.0.1",
			MinVersion: tls.VersionTLS13,
		}
}
