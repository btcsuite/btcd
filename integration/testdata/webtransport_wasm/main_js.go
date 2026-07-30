//go:build js && wasm

// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// The program in this file is a browser-only integration-test fixture.  It
// connects btcd's peer package to the browser WebTransport API and reports the
// completed Bitcoin handshake to the native test process.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"syscall/js"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire/v2"
)

const (
	operationTimeout = 15 * time.Second
	testUserAgent    = "btcd-wasm-itest"
	testVersion      = "0.0.1"
)

type promiseResult struct {
	value js.Value
	err   error
}

type readResult struct {
	data []byte
	err  error
}

type browserConn struct {
	transport js.Value
	reader    js.Value
	writer    js.Value
	localAddr net.Addr
	peerAddr  net.Addr

	readMu  sync.Mutex
	readBuf []byte
	readCh  chan readResult

	writeMu sync.Mutex

	errMu sync.Mutex
	err   error

	closeOnce sync.Once
	closed    chan struct{}
}

type testResult struct {
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	ServerUserAgent string `json:"server_user_agent,omitempty"`
	ServerProtocol  int32  `json:"server_protocol,omitempty"`
	SendAddrV2      bool   `json:"send_addr_v2,omitempty"`
	Pong            bool   `json:"pong,omitempty"`
	Transport       string `json:"transport,omitempty"`
}

func main() {
	result, p, err := run()
	if err != nil {
		result = testResult{Status: "error", Error: err.Error()}
	}

	if reportErr := reportResult(result); reportErr != nil {
		js.Global().Get("console").Call(
			"error", "unable to report WebTransport test result: "+
				reportErr.Error(),
		)
	}

	if p != nil && err == nil {
		// Keep the peer connected while the native test confirms that btcd's
		// RPC server sees this browser connection as an inbound peer.
		p.WaitForDisconnect()

		// The browser can still dispatch a settled Promise callback after the
		// peer connection closes. Keep the Go runtime alive until the test
		// closes the tab so wasm_exec.js never calls back into an exited Go
		// program.
		select {}
	}
}

func run() (testResult, *peer.Peer, error) {
	params := js.Global().Get("URLSearchParams").New(
		js.Global().Get("location").Get("search"),
	)
	endpoint := params.Call("get", "endpoint")
	certHash := params.Call("get", "cert_hash")
	if endpoint.IsNull() || certHash.IsNull() {
		return testResult{}, nil, fmt.Errorf(
			"endpoint and cert_hash query parameters are required",
		)
	}

	hash, err := hex.DecodeString(certHash.String())
	if err != nil {
		return testResult{}, nil, fmt.Errorf(
			"decode certificate hash: %w", err,
		)
	}
	if len(hash) != 32 {
		return testResult{}, nil, fmt.Errorf(
			"certificate hash has %d bytes, want 32", len(hash),
		)
	}

	conn, err := dialWebTransport(endpoint.String(), hash)
	if err != nil {
		return testResult{}, nil, err
	}

	versionChan := make(chan *wire.MsgVersion, 1)
	verAckChan := make(chan struct{}, 1)
	sendAddrV2Chan := make(chan struct{}, 1)
	pongChan := make(chan uint64, 1)
	peerConfig := &peer.Config{
		UserAgentName:    testUserAgent,
		UserAgentVersion: testVersion,
		ChainParams:      &chaincfg.SimNetParams,
		ProtocolVersion:  peer.MaxProtocolVersion,
		Listeners: peer.MessageListeners{
			OnVersion: func(_ *peer.Peer,
				msg *wire.MsgVersion) *wire.MsgReject {

				select {
				case versionChan <- msg:
				default:
				}
				return nil
			},
			OnVerAck: func(_ *peer.Peer, _ *wire.MsgVerAck) {
				select {
				case verAckChan <- struct{}{}:
				default:
				}
			},
			OnSendAddrV2: func(_ *peer.Peer, _ *wire.MsgSendAddrV2) {
				select {
				case sendAddrV2Chan <- struct{}{}:
				default:
				}
			},
			OnPong: func(_ *peer.Peer, msg *wire.MsgPong) {
				select {
				case pongChan <- msg.Nonce:
				default:
				}
			},
		},
	}

	parsedEndpoint, err := url.Parse(endpoint.String())
	if err != nil {
		_ = conn.Close()
		return testResult{}, nil, fmt.Errorf("parse endpoint: %w", err)
	}
	p, err := peer.NewOutboundPeer(peerConfig, parsedEndpoint.Host)
	if err != nil {
		_ = conn.Close()
		return testResult{}, nil, fmt.Errorf("create outbound peer: %w", err)
	}
	p.AssociateConnection(conn)

	var serverVersion *wire.MsgVersion
	select {
	case serverVersion = <-versionChan:
	case <-p.Done():
		return testResult{}, p, fmt.Errorf(
			"peer disconnected before receiving version",
		)
	case <-time.After(operationTimeout):
		p.Disconnect()
		return testResult{}, p, fmt.Errorf("timed out waiting for version")
	}

	select {
	case <-verAckChan:
	case <-p.Done():
		return testResult{}, p, fmt.Errorf(
			"peer disconnected before receiving verack",
		)
	case <-time.After(operationTimeout):
		p.Disconnect()
		return testResult{}, p, fmt.Errorf("timed out waiting for verack")
	}
	select {
	case <-sendAddrV2Chan:
	default:
		p.Disconnect()
		return testResult{}, p, fmt.Errorf(
			"verack arrived without sendaddrv2",
		)
	}

	const pingNonce = uint64(0x7765627472616e73)
	written := make(chan struct{}, 1)
	p.QueueMessage(wire.NewMsgPing(pingNonce), written)
	select {
	case <-written:
	case <-p.Done():
		return testResult{}, p, fmt.Errorf(
			"peer disconnected before writing ping",
		)
	case <-time.After(operationTimeout):
		p.Disconnect()
		return testResult{}, p, fmt.Errorf("timed out writing ping")
	}

	select {
	case nonce := <-pongChan:
		if nonce != pingNonce {
			p.Disconnect()
			return testResult{}, p, fmt.Errorf(
				"pong nonce %d, want %d", nonce, pingNonce,
			)
		}
	case <-p.Done():
		return testResult{}, p, fmt.Errorf(
			"peer disconnected before receiving pong",
		)
	case <-time.After(operationTimeout):
		p.Disconnect()
		return testResult{}, p, fmt.Errorf("timed out waiting for pong")
	}

	return testResult{
		Status:          "ok",
		ServerUserAgent: serverVersion.UserAgent,
		ServerProtocol:  serverVersion.ProtocolVersion,
		SendAddrV2:      true,
		Pong:            true,
		Transport:       "webtransport",
	}, p, nil
}

func dialWebTransport(endpoint string, certHash []byte) (net.Conn, error) {
	constructor := js.Global().Get("WebTransport")
	if constructor.Type() != js.TypeFunction {
		return nil, fmt.Errorf("browser WebTransport API unavailable")
	}

	hashValue := js.Global().Get("Uint8Array").New(len(certHash))
	js.CopyBytesToJS(hashValue, certHash)
	hashEntry := js.Global().Get("Object").New()
	hashEntry.Set("algorithm", "sha-256")
	hashEntry.Set("value", hashValue)
	hashes := js.Global().Get("Array").New()
	hashes.Call("push", hashEntry)
	options := js.Global().Get("Object").New()
	options.Set("serverCertificateHashes", hashes)

	transport, err := safeNew(constructor, endpoint, options)
	if err != nil {
		return nil, fmt.Errorf("create WebTransport session: %w", err)
	}
	if _, err := awaitPromise(
		transport.Get("ready"), operationTimeout,
	); err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("establish WebTransport session: %w", err)
	}

	streamPromise, err := safeCall(
		transport, "createBidirectionalStream",
	)
	if err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("create bidirectional stream: %w", err)
	}
	stream, err := awaitPromise(streamPromise, operationTimeout)
	if err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("open bidirectional stream: %w", err)
	}
	reader, err := safeCall(stream.Get("readable"), "getReader")
	if err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("get stream reader: %w", err)
	}
	writer, err := safeCall(stream.Get("writable"), "getWriter")
	if err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("get stream writer: %w", err)
	}

	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("parse WebTransport endpoint: %w", err)
	}
	peerAddr, err := net.ResolveTCPAddr("tcp", parsedEndpoint.Host)
	if err != nil {
		closeTransport(transport)
		return nil, fmt.Errorf("resolve WebTransport endpoint: %w", err)
	}

	c := &browserConn{
		transport: transport,
		reader:    reader,
		writer:    writer,
		localAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		peerAddr:  peerAddr,
		readCh:    make(chan readResult, 8),
		closed:    make(chan struct{}),
	}
	go c.readPump()

	return c, nil
}

func (c *browserConn) readPump() {
	for {
		promise, err := safeCall(c.reader, "read")
		if err != nil {
			c.finish(err)
			return
		}
		result, err := awaitPromise(promise, 0)
		if err != nil {
			c.finish(err)
			return
		}
		if result.Get("done").Bool() {
			c.finish(io.EOF)
			return
		}

		value := result.Get("value")
		if value.IsNull() || value.IsUndefined() {
			continue
		}
		data := make([]byte, value.Get("byteLength").Int())
		js.CopyBytesToGo(data, value)
		if len(data) == 0 {
			continue
		}

		select {
		case c.readCh <- readResult{data: data}:
		case <-c.closed:
			return
		}
	}
}

func (c *browserConn) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}

	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(c.readBuf) > 0 {
		return c.copyReadBuffer(buffer), nil
	}

	select {
	case result := <-c.readCh:
		if result.err != nil {
			return 0, result.err
		}
		c.readBuf = result.data
		return c.copyReadBuffer(buffer), nil
	case <-c.closed:
		return 0, c.connectionError()
	}
}

func (c *browserConn) copyReadBuffer(buffer []byte) int {
	n := copy(buffer, c.readBuf)
	c.readBuf = c.readBuf[n:]
	return n
}

func (c *browserConn) Write(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.closed:
		return 0, c.connectionError()
	default:
	}

	value := js.Global().Get("Uint8Array").New(len(buffer))
	js.CopyBytesToJS(value, buffer)
	promise, err := safeCall(c.writer, "write", value)
	if err != nil {
		c.finish(err)
		return 0, err
	}
	if _, err := awaitPromise(promise, operationTimeout); err != nil {
		c.finish(err)
		return 0, err
	}

	return len(buffer), nil
}

func (c *browserConn) Close() error {
	c.finish(io.EOF)
	closeTransport(c.transport)
	return nil
}

func (c *browserConn) LocalAddr() net.Addr  { return c.localAddr }
func (c *browserConn) RemoteAddr() net.Addr { return c.peerAddr }

func (c *browserConn) SetDeadline(time.Time) error      { return nil }
func (c *browserConn) SetReadDeadline(time.Time) error  { return nil }
func (c *browserConn) SetWriteDeadline(time.Time) error { return nil }

func (c *browserConn) finish(err error) {
	c.closeOnce.Do(func() {
		c.errMu.Lock()
		c.err = err
		c.errMu.Unlock()
		close(c.closed)
	})
}

func (c *browserConn) connectionError() error {
	c.errMu.Lock()
	defer c.errMu.Unlock()
	if c.err == nil {
		return io.EOF
	}
	return c.err
}

func reportResult(result testResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	status := js.Global().Get("document").Call("getElementById", "status")
	if !status.IsNull() && !status.IsUndefined() {
		status.Set("textContent", string(body))
	}

	headers := js.Global().Get("Object").New()
	headers.Set("Content-Type", "application/json")
	options := js.Global().Get("Object").New()
	options.Set("method", "POST")
	options.Set("headers", headers)
	options.Set("body", string(body))
	resultURL := js.Global().Get("location").Get("origin").String() +
		"/result"
	promise, err := safeCall(js.Global(), "fetch", resultURL, options)
	if err != nil {
		return err
	}
	_, err = awaitPromise(promise, operationTimeout)
	return err
}

func awaitPromise(promise js.Value, timeout time.Duration) (js.Value, error) {
	results := promiseResults(promise)
	if timeout == 0 {
		result := <-results
		return result.value, result.err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-results:
		return result.value, result.err
	case <-timer.C:
		return js.Undefined(), fmt.Errorf("JavaScript promise timed out")
	}
}

func promiseResults(promise js.Value) <-chan promiseResult {
	results := make(chan promiseResult, 1)
	var resolve, reject js.Func
	resolve = js.FuncOf(func(_ js.Value, args []js.Value) any {
		value := js.Undefined()
		if len(args) != 0 {
			value = args[0]
		}
		results <- promiseResult{value: value}
		resolve.Release()
		reject.Release()
		return nil
	})
	reject = js.FuncOf(func(_ js.Value, args []js.Value) any {
		value := js.Undefined()
		if len(args) != 0 {
			value = args[0]
		}
		results <- promiseResult{err: jsError(value)}
		resolve.Release()
		reject.Release()
		return nil
	})

	if _, err := safeCall(promise, "then", resolve, reject); err != nil {
		resolve.Release()
		reject.Release()
		results <- promiseResult{err: err}
	}
	return results
}

func safeCall(receiver js.Value, method string, args ...any) (
	value js.Value, err error) {

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("JavaScript %s call failed: %v", method,
				recovered)
		}
	}()
	return receiver.Call(method, args...), nil
}

func safeNew(constructor js.Value, args ...any) (value js.Value, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("JavaScript constructor failed: %v", recovered)
		}
	}()
	return constructor.New(args...), nil
}

func jsError(value js.Value) error {
	if value.IsNull() || value.IsUndefined() {
		return fmt.Errorf("JavaScript promise rejected")
	}
	if value.Type() == js.TypeObject {
		message := value.Get("message")
		if message.Type() == js.TypeString {
			return fmt.Errorf("%s", message.String())
		}
	}
	return fmt.Errorf("JavaScript promise rejected: %s", value.String())
}

func closeTransport(transport js.Value) {
	if transport.IsNull() || transport.IsUndefined() {
		return
	}
	info := js.Global().Get("Object").New()
	info.Set("closeCode", 0)
	info.Set("reason", "")
	_, _ = safeCall(transport, "close", info)
}

var _ net.Conn = (*browserConn)(nil)
