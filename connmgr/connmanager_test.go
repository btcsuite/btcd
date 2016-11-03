// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btclog"
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

// TestUseLogger tests that a logger can be passed to UseLogger
func TestUseLogger(t *testing.T) {
	l, err := btclog.NewLoggerFromWriter(bytes.NewBuffer(nil), btclog.InfoLvl)
	if err != nil {
		t.Fatal(err)
	}
	UseLogger(l)
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
	wantState = ConnDisconnected
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
