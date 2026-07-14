// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func init() {
	// Override the max retry duration when running tests.
	maxRetryDuration = 2 * time.Millisecond
}

// mockAddr mocks a network address
type mockAddr struct {
	net, address string
}

func (m mockAddr) Network() string { return m.net }
func (m mockAddr) String() string  { return m.address }

// mockConn mocks a network connection by implementing the net.Conn interface.
type mockConn struct {
	io.Reader
	io.Writer
	io.Closer

	// local network, address for the connection.
	lnet, laddr string

	// remote network, address for the connection.
	rAddr net.Addr
}

// LocalAddr returns the local address for the connection.
func (c mockConn) LocalAddr() net.Addr {
	return &mockAddr{c.lnet, c.laddr}
}

// RemoteAddr returns the remote address for the connection.
func (c mockConn) RemoteAddr() net.Addr {
	return &mockAddr{c.rAddr.Network(), c.rAddr.String()}
}

// Close handles closing the connection.
func (c mockConn) Close() error {
	return nil
}

func (c mockConn) SetDeadline(t time.Time) error      { return nil }
func (c mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c mockConn) SetWriteDeadline(t time.Time) error { return nil }

// mockDialer mocks the net.Dial interface by returning a mock connection to
// the given address.
func mockDialer(addr net.Addr) (net.Conn, error) {
	r, w := io.Pipe()
	c := &mockConn{rAddr: addr}
	c.Reader = r
	c.Writer = w
	return c, nil
}

// TestNewConfig tests that new ConnManager config is validated as expected.
func TestNewConfig(t *testing.T) {
	_, err := New(&Config{})
	if err == nil {
		t.Fatalf("New expected error: 'Dial can't be nil', got nil")
	}
	_, err = New(&Config{
		Dial: mockDialer,
	})
	if err != nil {
		t.Fatalf("New unexpected error: %v", err)
	}
}

// TestInboundLimit verifies that accepted inbound connections hold slots for
// their entire lifetime and release them exactly once on close.
func TestInboundLimit(t *testing.T) {
	maxInbound := uint32(2)

	cmgr, err := New(&Config{
		Dial:       mockDialer,
		MaxInbound: &maxInbound,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	newConn := func(port int) net.Conn {
		return &mockConn{rAddr: &net.TCPAddr{
			IP:   net.ParseIP("192.0.2.1"),
			Port: port,
		}}
	}

	first, ok := cmgr.limitInbound(newConn(1001))
	if !ok {
		t.Fatal("first inbound connection was rejected")
	}
	second, ok := cmgr.limitInbound(newConn(1002))
	if !ok {
		t.Fatal("second inbound connection was rejected")
	}
	if _, ok := cmgr.limitInbound(newConn(1003)); ok {
		t.Fatal("connection above the inbound limit was accepted")
	}

	if got := len(cmgr.inboundSlots); got != int(maxInbound) {
		t.Fatalf("unexpected occupied slots: got %d, want %d", got,
			maxInbound)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
	if got := len(cmgr.inboundSlots); got != 1 {
		t.Fatalf("double close released the slot more than once: got %d", got)
	}

	replacement, ok := cmgr.limitInbound(newConn(1004))
	if !ok {
		t.Fatal("replacement connection was rejected after slot release")
	}

	_ = second.Close()
	_ = replacement.Close()
	if got := len(cmgr.inboundSlots); got != 0 {
		t.Fatalf("inbound slots leaked: got %d", got)
	}
}

// TestInboundLimitDisabled verifies that an explicit zero limit rejects every
// accepted inbound connection.
func TestInboundLimitDisabled(t *testing.T) {
	maxInbound := uint32(0)
	cmgr, err := New(&Config{
		Dial:       mockDialer,
		MaxInbound: &maxInbound,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	conn := &mockConn{rAddr: &net.TCPAddr{
		IP: net.ParseIP("192.0.2.1"), Port: 1001,
	}}
	if _, ok := cmgr.limitInbound(conn); ok {
		t.Fatal("inbound connection accepted with a zero limit")
	}
	if got := atomic.LoadUint64(&cmgr.inboundRejected); got != 1 {
		t.Fatalf("unexpected rejection count: got %d, want 1", got)
	}
}

// TestInboundLimitConcurrent verifies that concurrent admission never exceeds
// the configured accepted-connection bound.
func TestInboundLimitConcurrent(t *testing.T) {
	const attempts = 100
	maxInbound := uint32(8)

	cmgr, err := New(&Config{
		Dial:       mockDialer,
		MaxInbound: &maxInbound,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	var (
		wg      sync.WaitGroup
		live    int32
		maxLive int32
	)
	release := make(chan struct{})

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()

			conn, ok := cmgr.limitInbound(&mockConn{
				rAddr: &net.TCPAddr{
					IP:   net.ParseIP("198.51.100.1"),
					Port: port,
				},
			})
			if !ok {
				return
			}

			current := atomic.AddInt32(&live, 1)
			for {
				observed := atomic.LoadInt32(&maxLive)
				if current <= observed || atomic.CompareAndSwapInt32(
					&maxLive, observed, current,
				) {
					break
				}
			}

			<-release
			atomic.AddInt32(&live, -1)
			_ = conn.Close()
		}(10000 + i)
	}

	deadline := time.After(time.Second)
	for len(cmgr.inboundSlots) < int(maxInbound) {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for inbound slots to fill")
		case <-time.After(time.Millisecond):
		}
	}
	close(release)
	wg.Wait()

	if maxLive > int32(maxInbound) {
		t.Fatalf("live inbound connections exceeded limit: got %d, want <= %d",
			maxLive, maxInbound)
	}
	if got := len(cmgr.inboundSlots); got != 0 {
		t.Fatalf("inbound slots leaked: got %d", got)
	}
}

// TestStartStop tests that the connection manager starts and stops as
// expected.
func TestStartStop(t *testing.T) {
	connected := make(chan *ConnReq)
	disconnected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		TargetOutbound: 1,
		GetNewAddress: func() (net.Addr, error) {
			return &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 18555,
			}, nil
		},
		Dial: mockDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
		OnDisconnection: func(c *ConnReq) {
			disconnected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()
	gotConnReq := <-connected
	cmgr.Stop()
	// already stopped
	cmgr.Stop()
	// ignored
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	cmgr.Connect(cr)
	if cr.ID() != 0 {
		t.Fatalf("start/stop: got id: %v, want: 0", cr.ID())
	}
	cmgr.Disconnect(gotConnReq.ID())
	cmgr.Remove(gotConnReq.ID())
	select {
	case <-disconnected:
		t.Fatalf("start/stop: unexpected disconnection")
	case <-time.Tick(10 * time.Millisecond):
		break
	}
}

// TestConnectMode tests that the connection manager works in the connect mode.
//
// In connect mode, automatic connections are disabled, so we test that
// requests using Connect are handled and that no other connections are made.
func TestConnectMode(t *testing.T) {
	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		TargetOutbound: 2,
		Dial:           mockDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	cmgr.Start()
	cmgr.Connect(cr)
	gotConnReq := <-connected
	wantID := cr.ID()
	gotID := gotConnReq.ID()
	if gotID != wantID {
		t.Fatalf("connect mode: %v - want ID %v, got ID %v", cr.Addr, wantID, gotID)
	}
	gotState := cr.State()
	wantState := ConnEstablished
	if gotState != wantState {
		t.Fatalf("connect mode: %v - want state %v, got state %v", cr.Addr, wantState, gotState)
	}
	select {
	case c := <-connected:
		t.Fatalf("connect mode: got unexpected connection - %v", c.Addr)
	case <-time.After(time.Millisecond):
		break
	}
	cmgr.Stop()
}

// TestTargetOutbound tests the target number of outbound connections.
//
// We wait until all connections are established, then test they there are the
// only connections made.
func TestTargetOutbound(t *testing.T) {
	targetOutbound := uint32(10)
	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		TargetOutbound: targetOutbound,
		Dial:           mockDialer,
		GetNewAddress: func() (net.Addr, error) {
			return &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 18555,
			}, nil
		},
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()
	for i := uint32(0); i < targetOutbound; i++ {
		<-connected
	}

	select {
	case c := <-connected:
		t.Fatalf("target outbound: got unexpected connection - %v", c.Addr)
	case <-time.After(time.Millisecond):
		break
	}
	cmgr.Stop()
}

// TestRetryPermanent tests that permanent connection requests are retried.
//
// We make a permanent connection request using Connect, disconnect it using
// Disconnect and we wait for it to be connected back.
func TestRetryPermanent(t *testing.T) {
	connected := make(chan *ConnReq)
	disconnected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		RetryDuration:  time.Millisecond,
		TargetOutbound: 1,
		Dial:           mockDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
		OnDisconnection: func(c *ConnReq) {
			disconnected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(cr)
	cmgr.Start()
	gotConnReq := <-connected
	wantID := cr.ID()
	gotID := gotConnReq.ID()
	if gotID != wantID {
		t.Fatalf("retry: %v - want ID %v, got ID %v", cr.Addr, wantID, gotID)
	}
	gotState := cr.State()
	wantState := ConnEstablished
	if gotState != wantState {
		t.Fatalf("retry: %v - want state %v, got state %v", cr.Addr, wantState, gotState)
	}

	cmgr.Disconnect(cr.ID())
	gotConnReq = <-disconnected
	wantID = cr.ID()
	gotID = gotConnReq.ID()
	if gotID != wantID {
		t.Fatalf("retry: %v - want ID %v, got ID %v", cr.Addr, wantID, gotID)
	}
	gotState = cr.State()
	wantState = ConnPending
	if gotState != wantState {
		t.Fatalf("retry: %v - want state %v, got state %v", cr.Addr, wantState, gotState)
	}

	gotConnReq = <-connected
	wantID = cr.ID()
	gotID = gotConnReq.ID()
	if gotID != wantID {
		t.Fatalf("retry: %v - want ID %v, got ID %v", cr.Addr, wantID, gotID)
	}
	gotState = cr.State()
	wantState = ConnEstablished
	if gotState != wantState {
		t.Fatalf("retry: %v - want state %v, got state %v", cr.Addr, wantState, gotState)
	}

	cmgr.Remove(cr.ID())
	gotConnReq = <-disconnected
	wantID = cr.ID()
	gotID = gotConnReq.ID()
	if gotID != wantID {
		t.Fatalf("retry: %v - want ID %v, got ID %v", cr.Addr, wantID, gotID)
	}
	gotState = cr.State()
	wantState = ConnDisconnected
	if gotState != wantState {
		t.Fatalf("retry: %v - want state %v, got state %v", cr.Addr, wantState, gotState)
	}
	cmgr.Stop()
}

// TestMaxRetryDuration tests the maximum retry duration.
//
// We have a timed dialer which initially returns err but after RetryDuration
// hits maxRetryDuration returns a mock conn.
func TestMaxRetryDuration(t *testing.T) {
	networkUp := make(chan struct{})
	time.AfterFunc(5*time.Millisecond, func() {
		close(networkUp)
	})
	timedDialer := func(addr net.Addr) (net.Conn, error) {
		select {
		case <-networkUp:
			return mockDialer(addr)
		default:
			return nil, errors.New("network down")
		}
	}

	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		RetryDuration:  time.Millisecond,
		TargetOutbound: 1,
		Dial:           timedDialer,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(cr)
	cmgr.Start()
	// retry in 1ms
	// retry in 2ms - max retry duration reached
	// retry in 2ms - timedDialer returns mockDial
	select {
	case <-connected:
	case <-time.Tick(100 * time.Millisecond):
		t.Fatalf("max retry duration: connection timeout")
	}
}

// TestNetworkFailure tests that the connection manager handles a network
// failure gracefully.
func TestNetworkFailure(t *testing.T) {
	var dials uint32
	errDialer := func(net net.Addr) (net.Conn, error) {
		atomic.AddUint32(&dials, 1)
		return nil, errors.New("network down")
	}
	cmgr, err := New(&Config{
		TargetOutbound: 5,
		RetryDuration:  5 * time.Millisecond,
		Dial:           errDialer,
		GetNewAddress: func() (net.Addr, error) {
			return &net.TCPAddr{
				IP:   net.ParseIP("127.0.0.1"),
				Port: 18555,
			}, nil
		},
		OnConnection: func(c *ConnReq, conn net.Conn) {
			t.Fatalf("network failure: got unexpected connection - %v", c.Addr)
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()
	time.AfterFunc(10*time.Millisecond, cmgr.Stop)
	cmgr.Wait()
	wantMaxDials := uint32(75)
	if atomic.LoadUint32(&dials) > wantMaxDials {
		t.Fatalf("network failure: unexpected number of dials - got %v, want < %v",
			atomic.LoadUint32(&dials), wantMaxDials)
	}
}

// TestStopFailed tests that failed connections are ignored after connmgr is
// stopped.
//
// We have a dailer which sets the stop flag on the conn manager and returns an
// err so that the handler assumes that the conn manager is stopped and ignores
// the failure.
func TestStopFailed(t *testing.T) {
	done := make(chan struct{}, 1)
	waitDialer := func(addr net.Addr) (net.Conn, error) {
		done <- struct{}{}
		time.Sleep(time.Millisecond)
		return nil, errors.New("network down")
	}
	cmgr, err := New(&Config{
		Dial: waitDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()
	go func() {
		<-done
		atomic.StoreInt32(&cmgr.stop, 1)
		time.Sleep(2 * time.Millisecond)
		atomic.StoreInt32(&cmgr.stop, 0)
		cmgr.Stop()
	}()
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(cr)
	cmgr.Wait()
}

// TestRemovePendingConnection tests that it's possible to cancel a pending
// connection, removing its internal state from the ConnMgr.
func TestRemovePendingConnection(t *testing.T) {
	// Create a ConnMgr instance with an instance of a dialer that'll never
	// succeed.
	wait := make(chan struct{})
	indefiniteDialer := func(addr net.Addr) (net.Conn, error) {
		<-wait
		return nil, fmt.Errorf("error")
	}
	cmgr, err := New(&Config{
		Dial: indefiniteDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()

	// Establish a connection request to a random IP we've chosen.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
		Permanent: true,
	}
	go cmgr.Connect(cr)

	time.Sleep(10 * time.Millisecond)

	if cr.State() != ConnPending {
		t.Fatalf("pending request hasn't been registered, status: %v",
			cr.State())
	}

	// The request launched above will actually never be able to establish
	// a connection. So we'll cancel it _before_ it's able to be completed.
	cmgr.Remove(cr.ID())

	time.Sleep(10 * time.Millisecond)

	// Now examine the status of the connection request, it should read a
	// status of ConnCanceled.
	if cr.State() != ConnCanceled {
		t.Fatalf("request wasn't canceled, status is: %v", cr.State())
	}

	close(wait)
	cmgr.Stop()
}

// TestCancelIgnoreDelayedConnection tests that a canceled connection request will
// not execute the on connection callback, even if an outstanding retry
// succeeds.
func TestCancelIgnoreDelayedConnection(t *testing.T) {
	retryTimeout := 10 * time.Millisecond

	// Setup a dialer that will continue to return an error until the
	// connect chan is signaled, the dial attempt immediately after will
	// succeed in returning a connection.
	connect := make(chan struct{})
	failingDialer := func(addr net.Addr) (net.Conn, error) {
		select {
		case <-connect:
			return mockDialer(addr)
		default:
		}

		return nil, fmt.Errorf("error")
	}

	connected := make(chan *ConnReq)
	cmgr, err := New(&Config{
		Dial:          failingDialer,
		RetryDuration: retryTimeout,
		OnConnection: func(c *ConnReq, conn net.Conn) {
			connected <- c
		},
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()
	defer cmgr.Stop()

	// Establish a connection request to a random IP we've chosen.
	cr := &ConnReq{
		Addr: &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: 18555,
		},
	}
	cmgr.Connect(cr)

	// Allow for the first retry timeout to elapse.
	time.Sleep(2 * retryTimeout)

	// Connection be marked as failed, even after reattempting to
	// connect.
	if cr.State() != ConnFailing {
		t.Fatalf("failing request should have status failed, status: %v",
			cr.State())
	}

	// Remove the connection, and then immediately allow the next connection
	// to succeed.
	cmgr.Remove(cr.ID())
	close(connect)

	// Allow the connection manager to process the removal.
	time.Sleep(5 * time.Millisecond)

	// Now examine the status of the connection request, it should read a
	// status of canceled.
	if cr.State() != ConnCanceled {
		t.Fatalf("request wasn't canceled, status is: %v", cr.State())
	}

	// Finally, the connection manager should not signal the on-connection
	// callback, since we explicitly canceled this request. We give a
	// generous window to ensure the connection manager's lienar backoff is
	// allowed to properly elapse.
	select {
	case <-connected:
		t.Fatalf("on-connect should not be called for canceled req")
	case <-time.After(5 * retryTimeout):
	}

}

// mockListener implements the net.Listener interface and is used to test
// code that deals with net.Listeners without having to actually make any real
// connections.
type mockListener struct {
	localAddr   string
	provideConn chan net.Conn
}

// Accept returns a mock connection when it receives a signal via the Connect
// function.
//
// This is part of the net.Listener interface.
func (m *mockListener) Accept() (net.Conn, error) {
	for conn := range m.provideConn {
		return conn, nil
	}
	return nil, errors.New("network connection closed")
}

// Close closes the mock listener which will cause any blocked Accept
// operations to be unblocked and return errors.
//
// This is part of the net.Listener interface.
func (m *mockListener) Close() error {
	close(m.provideConn)
	return nil
}

// Addr returns the address the mock listener was configured with.
//
// This is part of the net.Listener interface.
func (m *mockListener) Addr() net.Addr {
	return &mockAddr{"tcp", m.localAddr}
}

// Connect fakes a connection to the mock listener from the provided remote
// address.  It will cause the Accept function to return a mock connection
// configured with the provided remote address and the local address for the
// mock listener.
func (m *mockListener) Connect(ip string, port int) {
	m.provideConn <- &mockConn{
		laddr: m.localAddr,
		lnet:  "tcp",
		rAddr: &net.TCPAddr{
			IP:   net.ParseIP(ip),
			Port: port,
		},
	}
}

// newMockListener returns a new mock listener for the provided local address
// and port.  No ports are actually opened.
func newMockListener(localAddr string) *mockListener {
	return &mockListener{
		localAddr:   localAddr,
		provideConn: make(chan net.Conn),
	}
}

// TestListeners ensures providing listeners to the connection manager along
// with an accept callback works properly.
func TestListeners(t *testing.T) {
	// Setup a connection manager with a couple of mock listeners that
	// notify a channel when they receive mock connections.
	receivedConns := make(chan net.Conn)
	listener1 := newMockListener("127.0.0.1:8333")
	listener2 := newMockListener("127.0.0.1:9333")
	listeners := []net.Listener{listener1, listener2}
	cmgr, err := New(&Config{
		Listeners: listeners,
		OnAccept: func(conn net.Conn) {
			receivedConns <- conn
		},
		Dial: mockDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()

	// Fake a couple of mock connections to each of the listeners.
	go func() {
		for i, listener := range listeners {
			l := listener.(*mockListener)
			l.Connect("127.0.0.1", 10000+i*2)
			l.Connect("127.0.0.1", 10000+i*2+1)
		}
	}()

	// Tally the receive connections to ensure the expected number are
	// received.  Also, fail the test after a timeout so it will not hang
	// forever should the test not work.
	expectedNumConns := len(listeners) * 2
	var numConns int
out:
	for {
		select {
		case <-receivedConns:
			numConns++
			if numConns == expectedNumConns {
				break out
			}

		case <-time.After(time.Millisecond * 50):
			t.Fatalf("Timeout waiting for %d expected connections",
				expectedNumConns)
		}
	}

	cmgr.Stop()
	cmgr.Wait()
}

// TestListenerInboundLimit verifies excess sockets are rejected before the
// accept callback is started and capacity returns when an admitted socket
// closes.
func TestListenerInboundLimit(t *testing.T) {
	maxInbound := uint32(1)
	listener := newMockListener("127.0.0.1:8333")
	accepted := make(chan net.Conn, 2)

	cmgr, err := New(&Config{
		Listeners:  []net.Listener{listener},
		OnAccept:   func(conn net.Conn) { accepted <- conn },
		MaxInbound: &maxInbound,
		Dial:       mockDialer,
	})
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	cmgr.Start()
	defer func() {
		cmgr.Stop()
		cmgr.Wait()
	}()

	go listener.Connect("127.0.0.1", 10001)
	var first net.Conn
	select {
	case first = <-accepted:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("first inbound callback did not run")
	}

	go listener.Connect("127.0.0.1", 10002)
	deadline := time.After(time.Second)
	for atomic.LoadUint64(&cmgr.inboundRejected) != 1 {
		select {
		case <-deadline:
			t.Fatal("connection above the limit was not rejected")

		default:
			time.Sleep(time.Millisecond)
		}
	}
	select {
	case <-accepted:
		t.Fatal("connection above the limit reached the accept callback")

	default:
	}

	_ = first.Close()
	go listener.Connect("127.0.0.1", 10003)
	select {
	case replacement := <-accepted:
		_ = replacement.Close()
	case <-time.After(50 * time.Millisecond):
		t.Fatal("released inbound slot was not reused")
	}
}
