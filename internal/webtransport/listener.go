// Package webtransport exposes WebTransport sessions as a net.Listener. Each
// QUIC connection is limited to one session, and each accepted connection is
// that session's single client-opened bidirectional stream. No framing is added
// to the bytes carried by the stream.
package webtransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
)

const (
	// DefaultPath is the HTTP endpoint used when Config.Path is empty.
	DefaultPath = "/v1/btc-p2p"

	// Defaults keep the unauthenticated pre-stream resource use bounded while
	// allowing an idle Bitcoin peer to live beyond btcd's two-minute ping
	// interval.
	DefaultMaxPendingConnections = 64
	DefaultMaxPendingSessions    = 32
	DefaultFirstRequestTimeout   = 10 * time.Second
	DefaultFirstStreamTimeout    = 10 * time.Second
	DefaultHandshakeIdleTimeout  = 10 * time.Second
	DefaultMaxIdleTimeout        = 5 * time.Minute
	DefaultKeepAlivePeriod       = 15 * time.Second

	maxHeaderBytes = 8 << 10
)

const (
	firstStreamTimeoutCode wt.SessionErrorCode = 1
	extraStreamCode        wt.SessionErrorCode = 2
)

// Config configures a WebTransport listener.
type Config struct {
	// TLSConfig supplies the server certificate.  It is cloned before use.
	TLSConfig *tls.Config

	// Path is the exact URL path accepted by the HTTP/3 endpoint.
	Path string

	// AllowedOrigins contains additional browser origins accepted alongside
	// the safe same-origin default.  Entries are exact origins; wildcards and
	// URL paths are rejected.
	AllowedOrigins []string

	// MaxPendingConnections bounds QUIC connections that have not sent a
	// valid WebTransport CONNECT request.
	MaxPendingConnections int

	// MaxPendingSessions bounds sessions that have upgraded but have not yet
	// been returned by Accept.
	MaxPendingSessions int

	// FirstRequestTimeout bounds how long a QUIC connection may wait before
	// sending its valid WebTransport CONNECT request.
	FirstRequestTimeout time.Duration

	// FirstStreamTimeout bounds how long an upgraded session may wait for its
	// single client-opened bidirectional stream.
	FirstStreamTimeout time.Duration

	// These values configure the underlying QUIC connection.
	HandshakeIdleTimeout time.Duration
	MaxIdleTimeout       time.Duration
	KeepAlivePeriod      time.Duration
}

type listenerConfig struct {
	tlsConfig             *tls.Config
	path                  string
	allowedOrigins        map[string]struct{}
	maxPendingConnections int
	maxPendingSessions    int
	firstRequestTimeout   time.Duration
	firstStreamTimeout    time.Duration
	handshakeIdleTimeout  time.Duration
	maxIdleTimeout        time.Duration
	keepAlivePeriod       time.Duration
}

// Listener adapts WebTransport sessions to net.Listener.
type Listener struct {
	packetConn net.PacketConn
	server     *wt.Server
	config     listenerConfig

	acceptChan  chan net.Conn
	pendingConn chan struct{}
	pending     chan struct{}
	done        chan struct{}
	serveDone   chan struct{}
	serverReady chan struct{}

	closeOnce sync.Once
	readyOnce sync.Once
	errMu     sync.Mutex
	serveErr  error
	closeErr  error
}

type connectionStateKey struct{}

// connectionState moves one QUIC connection through the bounded pre-CONNECT
// phase into ownership by exactly one WebTransport-backed btcd peer.
type connectionState struct {
	conn    *quic.Conn
	allowed bool
	claimed chan struct{}

	claimOnce   sync.Once
	releaseOnce sync.Once
	closeOnce   sync.Once
	release     func()
	closeErr    error
}

func (s *connectionState) claim() bool {
	if !s.allowed {
		return false
	}

	claimed := false
	s.claimOnce.Do(func() {
		claimed = true
		close(s.claimed)
		s.releasePending()
	})

	return claimed
}

func (s *connectionState) releasePending() {
	s.releaseOnce.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
}

func (s *connectionState) close(code http3.ErrCode, reason string) error {
	s.releasePending()
	s.closeOnce.Do(func() {
		s.closeErr = s.conn.CloseWithError(
			quic.ApplicationErrorCode(code), reason,
		)
	})

	return s.closeErr
}

var _ net.Listener = (*Listener)(nil)

// Listen binds a UDP socket and starts a WebTransport listener.  Only UDP
// networks are accepted.
func Listen(network, address string, config Config) (*Listener, error) {
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, fmt.Errorf("webtransport requires a UDP network, got %q", network)
	}

	packetConn, err := net.ListenPacket(network, address)
	if err != nil {
		return nil, err
	}

	listener, err := NewListener(packetConn, config)
	if err != nil {
		_ = packetConn.Close()
		return nil, err
	}

	return listener, nil
}

// NewListener starts a WebTransport listener on packetConn.  The listener
// takes ownership of packetConn when this function succeeds.
func NewListener(packetConn net.PacketConn, config Config) (*Listener, error) {
	if packetConn == nil {
		return nil, errors.New("webtransport packet connection is nil")
	}

	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}

	listener := &Listener{
		packetConn:  packetConn,
		config:      normalized,
		acceptChan:  make(chan net.Conn),
		pendingConn: make(chan struct{}, normalized.maxPendingConnections),
		pending:     make(chan struct{}, normalized.maxPendingSessions),
		done:        make(chan struct{}),
		serveDone:   make(chan struct{}),
		serverReady: make(chan struct{}),
	}

	h3Server := &http3.Server{
		TLSConfig: http3.ConfigureTLSConfig(normalized.tlsConfig),
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout: normalized.handshakeIdleTimeout,
			MaxIdleTimeout:       normalized.maxIdleTimeout,
			KeepAlivePeriod:      normalized.keepAlivePeriod,
		},
		Handler:        http.HandlerFunc(listener.handleRequest),
		MaxHeaderBytes: maxHeaderBytes,
		ConnContext:    listener.connectionContext,
	}
	listener.server = &wt.Server{
		H3: h3Server,
		Config: &wt.Config{
			MaxIncomingStreams:    1,
			MaxIncomingUniStreams: -1,
		},
		CheckOrigin: listener.checkOrigin,
	}

	go listener.serve()

	return listener, nil
}

// Accept waits for a session's first client-opened bidirectional stream.
func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.acceptChan:
		return conn, nil

	case <-l.done:
		return nil, l.acceptError()
	}
}

// Close shuts down all HTTP/3 connections and sessions, closes the UDP socket,
// and unblocks Accept.
func (l *Listener) Close() error {
	l.shutdown(nil)

	l.errMu.Lock()
	defer l.errMu.Unlock()

	return l.closeErr
}

// Addr returns the listener's UDP address.
func (l *Listener) Addr() net.Addr {
	return l.packetConn.LocalAddr()
}

func (l *Listener) serve() {
	err := l.server.Serve(l.packetConn)
	close(l.serveDone)
	select {
	case <-l.done:
		return
	default:
	}

	l.shutdown(err)
}

func (l *Listener) shutdown(cause error) {
	l.closeOnce.Do(func() {
		l.errMu.Lock()
		l.serveErr = cause
		l.errMu.Unlock()

		// Unblock Accept and handler delivery before waiting for the HTTP/3
		// server.
		close(l.done)

		var serverErr, packetErr error
		select {
		case <-l.serverReady:
			// Once a request reaches the handler, Server.Serve has finished
			// initialization.  Close WebTransport while the packet socket is
			// still open so close frames can reach established clients.
			serverErr = l.server.Close()
			packetErr = l.packetConn.Close()

		case <-l.serveDone:
			// Serve has already returned, so calling Close cannot race with
			// the server's internal startup bookkeeping.
			serverErr = l.server.Close()
			packetErr = l.packetConn.Close()

		default:
			// webtransport-go's Serve and Close must not race before Serve
			// installs its internal waiter.  With no request yet, there are no
			// WebTransport sessions that need a close frame, so stop the packet
			// listener first and wait for Serve to finish starting.
			packetErr = l.packetConn.Close()
			<-l.serveDone
			serverErr = l.server.Close()
		}

		if errors.Is(serverErr, net.ErrClosed) {
			serverErr = nil
		}
		if errors.Is(packetErr, net.ErrClosed) {
			packetErr = nil
		}

		l.errMu.Lock()
		l.closeErr = errors.Join(serverErr, packetErr)
		l.errMu.Unlock()
	})
}

func (l *Listener) acceptError() error {
	l.errMu.Lock()
	defer l.errMu.Unlock()

	if l.serveErr == nil || errors.Is(l.serveErr, net.ErrClosed) ||
		errors.Is(l.serveErr, context.Canceled) {

		return net.ErrClosed
	}

	return fmt.Errorf("webtransport listener stopped: %w",
		errors.Join(net.ErrClosed, l.serveErr))
}

func (l *Listener) connectionContext(ctx context.Context,
	conn *quic.Conn) context.Context {

	state := &connectionState{
		conn:    conn,
		claimed: make(chan struct{}),
	}

	select {
	case <-l.done:
		go state.close(http3.ErrCodeNoError, "listener is shutting down")
		return context.WithValue(ctx, connectionStateKey{}, state)
	default:
	}

	select {
	case l.pendingConn <- struct{}{}:
		state.allowed = true
		state.release = func() { <-l.pendingConn }
		go l.expirePendingConnection(ctx, state)

	default:
		go state.close(
			http3.ErrCodeExcessiveLoad,
			"too many pending WebTransport connections",
		)
	}

	return context.WithValue(ctx, connectionStateKey{}, state)
}

func (l *Listener) expirePendingConnection(ctx context.Context,
	state *connectionState) {

	timer := time.NewTimer(l.config.firstRequestTimeout)
	defer timer.Stop()
	defer state.releasePending()

	select {
	case <-state.claimed:
	case <-ctx.Done():
	case <-l.done:
	case <-timer.C:
		_ = state.close(
			http3.ErrCodeRequestRejected,
			"WebTransport CONNECT request was not received in time",
		)
	}
}

func (l *Listener) handleRequest(w http.ResponseWriter, request *http.Request) {
	l.readyOnce.Do(func() { close(l.serverReady) })

	if request.URL.EscapedPath() != l.config.path ||
		request.URL.RawQuery != "" {

		http.NotFound(w, request)
		return
	}
	if request.Method != http.MethodConnect {
		w.Header().Set("Allow", http.MethodConnect)
		http.Error(w, "WebTransport requires CONNECT",
			http.StatusMethodNotAllowed)
		return
	}
	if !l.checkOrigin(request) {
		http.Error(w, "WebTransport origin not allowed", http.StatusForbidden)
		return
	}

	select {
	case l.pending <- struct{}{}:
		defer func() { <-l.pending }()

	case <-l.done:
		http.Error(w, "WebTransport listener is shutting down",
			http.StatusServiceUnavailable)
		return

	default:
		http.Error(w, "too many pending WebTransport sessions",
			http.StatusServiceUnavailable)
		return
	}

	state, ok := request.Context().Value(
		connectionStateKey{},
	).(*connectionState)
	if !ok {
		http.Error(w, "WebTransport connection state unavailable",
			http.StatusInternalServerError)
		return
	}
	if !state.claim() {
		http.Error(w, "only one WebTransport session is allowed per connection",
			http.StatusConflict)
		return
	}

	session, err := l.server.Upgrade(w, request)
	if err != nil {
		http.Error(w, "unable to upgrade WebTransport session",
			http.StatusBadRequest)
		_ = state.close(http3.ErrCodeConnectError,
			"WebTransport session upgrade failed")
		return
	}

	ctx, cancel := context.WithTimeout(
		session.Context(), l.config.firstStreamTimeout,
	)
	stream, err := session.AcceptStream(ctx)
	cancel()
	if err != nil {
		_ = session.CloseWithError(
			firstStreamTimeoutCode,
			"first bidirectional stream was not opened in time",
		)
		_ = state.close(http3.ErrCodeRequestIncomplete,
			"WebTransport stream was not opened in time")
		return
	}

	conn := newStreamConn(stream, session, func() error {
		return state.close(http3.ErrCodeNoError, "")
	})
	l.enforceSingleStream(session)

	select {
	case l.acceptChan <- conn:
	case <-l.done:
		_ = conn.Close()
	case <-session.Context().Done():
		_ = conn.Close()
	}
}

// enforceSingleStream enforces the one-session, one-bidirectional-stream
// mapping even when a peer doesn't negotiate WebTransport stream flow control.
func (l *Listener) enforceSingleStream(session *wt.Session) {
	go func() {
		if _, err := session.AcceptStream(session.Context()); err == nil {
			_ = session.CloseWithError(
				extraStreamCode,
				"only one bidirectional stream is allowed",
			)
		}
	}()

	go func() {
		if _, err := session.AcceptUniStream(session.Context()); err == nil {
			_ = session.CloseWithError(
				extraStreamCode,
				"unidirectional streams are not allowed",
			)
		}
	}()

	go func() {
		if _, err := session.ReceiveDatagram(session.Context()); err == nil {
			_ = session.CloseWithError(
				extraStreamCode,
				"datagrams are not allowed",
			)
		}
	}()
}

func (l *Listener) checkOrigin(request *http.Request) bool {
	origins := request.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}

	origin, err := canonicalOrigin(origins[0])
	if err != nil {
		return false
	}
	requestOrigin, err := canonicalOrigin("https://" + request.Host)
	if err != nil {
		return false
	}
	if origin == requestOrigin {
		return true
	}

	_, ok := l.config.allowedOrigins[origin]
	return ok
}

func normalizeConfig(config Config) (listenerConfig, error) {
	if config.TLSConfig == nil {
		return listenerConfig{}, errors.New("webtransport TLS config is nil")
	}
	if len(config.TLSConfig.Certificates) == 0 &&
		config.TLSConfig.GetCertificate == nil &&
		config.TLSConfig.GetConfigForClient == nil {

		return listenerConfig{}, errors.New("webtransport TLS config has no server certificate")
	}

	path := config.Path
	if path == "" {
		path = DefaultPath
	}
	parsedPath, err := url.ParseRequestURI(path)
	if err != nil || !strings.HasPrefix(path, "/") ||
		strings.HasPrefix(path, "//") || parsedPath.IsAbs() ||
		parsedPath.Host != "" || parsedPath.RawQuery != "" ||
		parsedPath.ForceQuery || parsedPath.Fragment != "" {

		return listenerConfig{}, fmt.Errorf("invalid WebTransport path %q", path)
	}
	path = parsedPath.EscapedPath()

	allowedOrigins := make(map[string]struct{}, len(config.AllowedOrigins))
	for _, rawOrigin := range config.AllowedOrigins {
		if strings.Contains(rawOrigin, "*") {
			return listenerConfig{}, fmt.Errorf(
				"WebTransport origin %q contains a wildcard", rawOrigin,
			)
		}

		origin, err := canonicalOrigin(rawOrigin)
		if err != nil {
			return listenerConfig{}, fmt.Errorf(
				"invalid WebTransport origin %q: %w", rawOrigin, err,
			)
		}
		allowedOrigins[origin] = struct{}{}
	}

	maxPendingConnections := config.MaxPendingConnections
	if maxPendingConnections == 0 {
		maxPendingConnections = DefaultMaxPendingConnections
	}
	if maxPendingConnections < 0 {
		return listenerConfig{}, errors.New("WebTransport pending connection limit must not be negative")
	}

	maxPendingSessions := config.MaxPendingSessions
	if maxPendingSessions == 0 {
		maxPendingSessions = DefaultMaxPendingSessions
	}
	if maxPendingSessions < 0 {
		return listenerConfig{}, errors.New("WebTransport pending session limit must not be negative")
	}

	firstRequestTimeout := config.FirstRequestTimeout
	if firstRequestTimeout == 0 {
		firstRequestTimeout = DefaultFirstRequestTimeout
	}
	if firstRequestTimeout < 0 {
		return listenerConfig{}, errors.New("WebTransport first request timeout must not be negative")
	}

	firstStreamTimeout := config.FirstStreamTimeout
	if firstStreamTimeout == 0 {
		firstStreamTimeout = DefaultFirstStreamTimeout
	}
	if firstStreamTimeout < 0 {
		return listenerConfig{}, errors.New("WebTransport first stream timeout must not be negative")
	}

	handshakeIdleTimeout := config.HandshakeIdleTimeout
	if handshakeIdleTimeout == 0 {
		handshakeIdleTimeout = DefaultHandshakeIdleTimeout
	}
	if handshakeIdleTimeout < 0 {
		return listenerConfig{}, errors.New("WebTransport handshake idle timeout must not be negative")
	}

	maxIdleTimeout := config.MaxIdleTimeout
	if maxIdleTimeout == 0 {
		maxIdleTimeout = DefaultMaxIdleTimeout
	}
	if maxIdleTimeout < 0 {
		return listenerConfig{}, errors.New("WebTransport maximum idle timeout must not be negative")
	}

	keepAlivePeriod := config.KeepAlivePeriod
	if keepAlivePeriod == 0 {
		keepAlivePeriod = DefaultKeepAlivePeriod
	}
	if keepAlivePeriod < 0 {
		return listenerConfig{}, errors.New("WebTransport keepalive period must not be negative")
	}

	return listenerConfig{
		tlsConfig:             config.TLSConfig.Clone(),
		path:                  path,
		allowedOrigins:        allowedOrigins,
		maxPendingConnections: maxPendingConnections,
		maxPendingSessions:    maxPendingSessions,
		firstRequestTimeout:   firstRequestTimeout,
		firstStreamTimeout:    firstStreamTimeout,
		handshakeIdleTimeout:  handshakeIdleTimeout,
		maxIdleTimeout:        maxIdleTimeout,
		keepAlivePeriod:       keepAlivePeriod,
	}, nil
}

func canonicalOrigin(rawOrigin string) (string, error) {
	if rawOrigin == "null" {
		return "", errors.New("opaque origins are not allowed")
	}

	origin, err := url.Parse(rawOrigin)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(origin.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", errors.New("origin scheme must be http or https")
	}
	if origin.User != nil || origin.Host == "" || origin.Path != "" ||
		origin.RawPath != "" || origin.RawQuery != "" || origin.ForceQuery ||
		origin.Fragment != "" {

		return "", errors.New("origin must not contain credentials, a path, query, or fragment")
	}

	hostname := strings.ToLower(origin.Hostname())
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", errors.New("origin hostname is invalid")
	}

	port := origin.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return "", errors.New("origin port must be between 1 and 65535")
	}

	return scheme + "://" + net.JoinHostPort(
		hostname, strconv.FormatUint(portNumber, 10),
	), nil
}
