// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package connmgr

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// maxFailedAttempts is the maximum number of successive failed connection
// attempts after which network failure is assumed and new connections will
// be delayed by the configured retry duration.
const maxFailedAttempts = 25

var (
	//ErrDialNil is used to indicate that Dial cannot be nil in the configuration.
	ErrDialNil = errors.New("config: dial cannot be nil")

	// maxRetryDuration is the max duration of time retrying of a persistent
	// connection is allowed to grow to.  This is necessary since the retry
	// logic uses a backoff mechanism which increases the interval base times
	// the number of retries that have been done.
	maxRetryDuration = time.Minute * 5

	// defaultRetryDuration is the default duration of time for retrying
	// persistent connections.
	defaultRetryDuration = time.Second * 5

	// defaultTargetOutbound is the default number of outbound connections to
	// maintain.
	defaultTargetOutbound = uint32(8)
)

// ConnState represents the state of the requested connection.
type ConnState uint32

// ConnState can be either pending, established, disconnected or failed.  When
// a new connection is requested, it is attempted and categorized as
// established or failed depending on the connection result.  An established
// connection which was disconnected is categorized as disconnected.
const (
	ConnPending ConnState = iota
	ConnEstablished
	ConnDisconnected
	ConnFailed
)

// ConnReq is the connection request to a network address. If permanent, the
// connection will be retried on disconnection.
type ConnReq struct {
	// The following variables must only be used atomically.
	id    uint64
	state uint32

	retryCount uint32
	conn       net.Conn
	Addr       net.Addr
	Permanent  bool
}

// updateState updates the state of the connection request.
func (c *ConnReq) updateState(state ConnState) {
	atomic.StoreUint32(&c.state, uint32(state))
}

// ID returns a unique identifier for the connection request.
func (c *ConnReq) ID() uint64 {
	return atomic.LoadUint64(&c.id)
}

// State is the connection state of the requested connection.
func (c *ConnReq) State() ConnState {
	return ConnState(atomic.LoadUint32(&c.state))
}

// String returns a human-readable string for the connection request.
func (c *ConnReq) String() string {
	if c.Addr.String() == "" {
		return fmt.Sprintf("reqid %d", atomic.LoadUint64(&c.id))
	}
	return fmt.Sprintf("%s (reqid %d)", c.Addr, atomic.LoadUint64(&c.id))
}

// Config holds the configuration options related to the connection manager.
type Config struct {
	// Listeners defines a slice of listeners for which the connection
	// manager will take ownership of and accept connections.  When a
	// connection is accepted, the OnAccept handler will be invoked with the
	// connection.  Since the connection manager takes ownership of these
	// listeners, they will be closed when the connection manager is
	// stopped.
	//
	// This field will not have any effect if the OnAccept field is not
	// also specified.  It may be nil if the caller does not wish to listen
	// for incoming connections.
	Listeners []net.Listener

	// OnAccept is a callback that is fired when an inbound connection is
	// accepted.  It is the caller's responsibility to close the connection.
	// Failure to close the connection will result in the connection manager
	// believing the connection is still active and thus have undesirable
	// side effects such as still counting toward maximum connection limits.
	//
	// This field will not have any effect if the Listeners field is not
	// also specified since there couldn't possibly be any accepted
	// connections in that case.
	OnAccept func(net.Conn)

	// TargetOutbound is the number of outbound network connections to
	// maintain. Defaults to 8.
	TargetOutbound uint32

	// RetryDuration is the duration to wait before retrying connection
	// requests. Defaults to 5s.
	RetryDuration time.Duration

	// OnConnection is a callback that is fired when a new outbound
	// connection is established.
	OnConnection func(*ConnReq, net.Conn)

	// OnDisconnection is a callback that is fired when an outbound
	// connection is disconnected.
	OnDisconnection func(*ConnReq)

	// GetNewAddress is a way to get an address to make a network connection
	// to.  If nil, no new connections will be made automatically.
	GetNewAddress func() (net.Addr, error)

	// Dial connects to the address on the named network. It cannot be nil.
	Dial func(net.Addr) (net.Conn, error)
}

// handleConnected is used to queue a successful connection.
type handleConnected struct {
	c    *ConnReq
	conn net.Conn
}

// handleDisconnected is used to remove a connection.
type handleDisconnected struct {
	id    uint64
	retry bool
}

// handleFailed is used to remove a pending connection.
type handleFailed struct {
	c   *ConnReq
	err error
}

// ConnManager provides a manager to handle network connections.
type ConnManager struct {
	// The following variables must only be used atomically.
	connReqCount uint64
	start        int32
	stop         int32

	cfg            Config
	wg             sync.WaitGroup
	failedAttempts uint64
	requests       chan interface{}
	quit           chan struct{}
}

// handleFailedConn handles a connection failed due to a disconnect or any
// other failure. If permanent, it retries the connection after the configured
// retry duration. Otherwise, if required, it makes a new connection request.
// After maxFailedConnectionAttempts new connections will be retried after the
// configured retry duration.
func (cm *ConnManager) handleFailedConn(c *ConnReq) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	if c.Permanent {
		c.retryCount++
		d := time.Duration(c.retryCount) * cm.cfg.RetryDuration
		if d > maxRetryDuration {
			d = maxRetryDuration
		}
		log.Debugf("Retrying connection to %v in %v", c, d)
		time.AfterFunc(d, func() {
			cm.Connect(c)
		})
	} else if cm.cfg.GetNewAddress != nil {
		cm.failedAttempts++
		if cm.failedAttempts >= maxFailedAttempts {
			log.Debugf("Max failed connection attempts reached: [%d] "+
				"-- retrying connection in: %v", maxFailedAttempts,
				cm.cfg.RetryDuration)
			time.AfterFunc(cm.cfg.RetryDuration, func() {
				cm.NewConnReq()
			})
		} else {
			go cm.NewConnReq()
		}
	}
}

// connHandler handles all connection related requests.  It must be run as a
// goroutine.
//
// The connection handler makes sure that we maintain a pool of active outbound
// connections so that we remain connected to the network.  Connection requests
// are processed and mapped by their assigned ids.
func (cm *ConnManager) connHandler() {
	conns := make(map[uint64]*ConnReq, cm.cfg.TargetOutbound)
out:
	for {
		select {
		case req := <-cm.requests:
			switch msg := req.(type) {

			case handleConnected:
				connReq := msg.c
				connReq.updateState(ConnEstablished)
				connReq.conn = msg.conn
				conns[connReq.id] = connReq
				log.Debugf("Connected to %v", connReq)
				connReq.retryCount = 0
				cm.failedAttempts = 0

				if cm.cfg.OnConnection != nil {
					go cm.cfg.OnConnection(connReq, msg.conn)
				}

			case handleDisconnected:
				if connReq, ok := conns[msg.id]; ok {
					connReq.updateState(ConnDisconnected)
					if connReq.conn != nil {
						connReq.conn.Close()
					}
					log.Debugf("Disconnected from %v", connReq)
					delete(conns, msg.id)

					if cm.cfg.OnDisconnection != nil {
						go cm.cfg.OnDisconnection(connReq)
					}

					if uint32(len(conns)) < cm.cfg.TargetOutbound && msg.retry {
						cm.handleFailedConn(connReq)
					}
				} else {
					log.Errorf("Unknown connection: %d", msg.id)
				}

			case handleFailed:
				connReq := msg.c
				connReq.updateState(ConnFailed)
				log.Debugf("Failed to connect to %v: %v", connReq, msg.err)
				cm.handleFailedConn(connReq)
			}

		case <-cm.quit:
			break out
		}
	}

	cm.wg.Done()
	log.Trace("Connection handler done")
}

// NewConnReq creates a new connection request and connects to the
// corresponding address.
func (cm *ConnManager) NewConnReq() {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	if cm.cfg.GetNewAddress == nil {
		return
	}

	c := &ConnReq{}
	atomic.StoreUint64(&c.id, atomic.AddUint64(&cm.connReqCount, 1))

	addr, err := cm.cfg.GetNewAddress()
	if err != nil {
		cm.requests <- handleFailed{c, err}
		return
	}

	c.Addr = addr

	cm.Connect(c)
}

// Connect assigns an id and dials a connection to the address of the
// connection request.
func (cm *ConnManager) Connect(c *ConnReq) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	if atomic.LoadUint64(&c.id) == 0 {
		atomic.StoreUint64(&c.id, atomic.AddUint64(&cm.connReqCount, 1))
	}
	log.Debugf("Attempting to connect to %v", c)
	conn, err := cm.cfg.Dial(c.Addr)
	if err != nil {
		cm.requests <- handleFailed{c, err}
	} else {
		cm.requests <- handleConnected{c, conn}
	}
}

// Disconnect disconnects the connection corresponding to the given connection
// id. If permanent, the connection will be retried with an increasing backoff
// duration.
func (cm *ConnManager) Disconnect(id uint64) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	cm.requests <- handleDisconnected{id, true}
}

// Remove removes the connection corresponding to the given connection
// id from known connections.
func (cm *ConnManager) Remove(id uint64) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	cm.requests <- handleDisconnected{id, false}
}

// listenHandler accepts incoming connections on a given listener.  It must be
// run as a goroutine.
func (cm *ConnManager) listenHandler(listener net.Listener) {
	log.Infof("Server listening on %s", listener.Addr())
	for atomic.LoadInt32(&cm.stop) == 0 {
		conn, err := listener.Accept()
		if err != nil {
			// Only log the error if not forcibly shutting down.
			if atomic.LoadInt32(&cm.stop) == 0 {
				log.Errorf("Can't accept connection: %v", err)
			}
			continue
		}
		go cm.cfg.OnAccept(conn)
	}

	cm.wg.Done()
	log.Tracef("Listener handler done for %s", listener.Addr())
}

// Start launches the connection manager and begins connecting to the network.
func (cm *ConnManager) Start() {
	// Already started?
	if atomic.AddInt32(&cm.start, 1) != 1 {
		return
	}

	log.Trace("Connection manager started")
	cm.wg.Add(1)
	go cm.connHandler()

	// Start all the listeners so long as the caller requested them and
	// provided a callback to be invoked when connections are accepted.
	if cm.cfg.OnAccept != nil {
		for _, listner := range cm.cfg.Listeners {
			cm.wg.Add(1)
			go cm.listenHandler(listner)
		}
	}

	for i := atomic.LoadUint64(&cm.connReqCount); i < uint64(cm.cfg.TargetOutbound); i++ {
		go cm.NewConnReq()
	}
}

// Wait blocks until the connection manager halts gracefully.
func (cm *ConnManager) Wait() {
	cm.wg.Wait()
}

// Stop gracefully shuts down the connection manager.
func (cm *ConnManager) Stop() {
	if atomic.AddInt32(&cm.stop, 1) != 1 {
		log.Warnf("Connection manager already stopped")
		return
	}

	// Stop all the listeners.  There will not be any listeners if
	// listening is disabled.
	for _, listener := range cm.cfg.Listeners {
		// Ignore the error since this is shutdown and there is no way
		// to recover anyways.
		_ = listener.Close()
	}

	close(cm.quit)
	log.Trace("Connection manager stopped")
}

// New returns a new connection manager.
// Use Start to start connecting to the network.
func New(cfg *Config) (*ConnManager, error) {
	if cfg.Dial == nil {
		return nil, ErrDialNil
	}
	// Default to sane values
	if cfg.RetryDuration <= 0 {
		cfg.RetryDuration = defaultRetryDuration
	}
	if cfg.TargetOutbound == 0 {
		cfg.TargetOutbound = defaultTargetOutbound
	}
	cm := ConnManager{
		cfg:      *cfg, // Copy so caller can't mutate
		requests: make(chan interface{}),
		quit:     make(chan struct{}),
	}
	return &cm, nil
}
