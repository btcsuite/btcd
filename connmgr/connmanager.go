// Copyright (c) 2016 The btcsuite developers
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
	ErrDialNil = errors.New("Config: Dial cannot be nil")

	// maxRetryDuration is the max duration of time retrying of a persistent
	// connection is allowed to grow to.  This is necessary since the retry
	// logic uses a backoff mechanism which increases the interval base times
	// the number of retries that have been done.
	maxRetryDuration = time.Minute * 5

	// defaultRetryDuration is the default duration of time for retrying
	// persistent connections.
	defaultRetryDuration = time.Second * 5

	// defaultMaxOutbound is the default number of maximum outbound connections
	// to maintain.
	defaultMaxOutbound = uint32(8)
)

// DialFunc defines a function that dials a connection.
type DialFunc func(string, string) (net.Conn, error)

// AddressFunc defines a function that returns a network address to connect to.
type AddressFunc func() (string, error)

// ConnState represents the state of the requested connection.
type ConnState uint8

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

// OnConnectionFunc is the signature of the callback function which is used to
// subscribe to new connections.
type OnConnectionFunc func(*ConnReq, net.Conn)

// OnDisconnectionFunc is the signature of the callback function which is used to
// notify disconnections.
type OnDisconnectionFunc func(*ConnReq)

// ConnReq is the connection request to a network address. If permanent, the
// connection will be retried on disconnection.
type ConnReq struct {
	// The following variables must only be used atomically.
	id uint64

	Addr      string
	Permanent bool

	conn       net.Conn
	state      ConnState
	stateMtx   sync.RWMutex
	retryCount uint32
}

// updateState updates the state of the connection request.
func (c *ConnReq) updateState(state ConnState) {
	c.stateMtx.Lock()
	c.state = state
	c.stateMtx.Unlock()
}

// ID returns a unique identifier for the connection request.
func (c *ConnReq) ID() uint64 {
	return atomic.LoadUint64(&c.id)
}

// State is the connection state of the requested connection.
func (c *ConnReq) State() ConnState {
	c.stateMtx.RLock()
	state := c.state
	c.stateMtx.RUnlock()
	return state
}

// String returns a human-readable string for the connection request.
func (c *ConnReq) String() string {
	if c.Addr == "" {
		return fmt.Sprintf("reqid %d", atomic.LoadUint64(&c.id))
	}
	return fmt.Sprintf("%s (reqid %d)", c.Addr, atomic.LoadUint64(&c.id))
}

// Config holds the configuration options related to the connection manager.
type Config struct {
	// MaxOutbound is the maximum number of outbound network connections to
	// maintain. Defaults to 8.
	MaxOutbound uint32

	// RetryDuration is the duration to wait before retrying connection
	// requests. Defaults to 5s.
	RetryDuration time.Duration

	// OnConnection is a callback that is fired when a new connection is
	// established.
	OnConnection OnConnectionFunc

	// OnDisconnection is a callback that is fired when a connection is
	// disconnected.
	OnDisconnection OnDisconnectionFunc

	// GetNewAddress is a way to get an address to make a network connection
	// to.  If nil, no new connections will be made automatically.
	GetNewAddress AddressFunc

	// Dial connects to the address on the named network. It cannot be nil.
	Dial DialFunc
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
func (cm *ConnManager) handleFailedConn(c *ConnReq, retry bool) {
	if atomic.LoadInt32(&cm.stop) != 0 {
		return
	}
	if retry && c.Permanent {
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
	conns := make(map[uint64]*ConnReq, cm.cfg.MaxOutbound)
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

					cm.handleFailedConn(connReq, msg.retry)
				} else {
					log.Errorf("Unknown connection: %d", msg.id)
				}

			case handleFailed:
				connReq := msg.c
				connReq.updateState(ConnFailed)
				log.Debugf("Failed to connect to %v: %v", connReq, msg.err)
				cm.handleFailedConn(connReq, true)
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
	conn, err := cm.cfg.Dial("tcp", c.Addr)
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

// Start launches the connection manager and begins connecting to the network.
func (cm *ConnManager) Start() {
	// Already started?
	if atomic.AddInt32(&cm.start, 1) != 1 {
		return
	}

	log.Trace("Connection manager started")
	cm.wg.Add(1)
	go cm.connHandler()

	for i := atomic.LoadUint64(&cm.connReqCount); i < uint64(cm.cfg.MaxOutbound); i++ {
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
	if cfg.MaxOutbound == 0 {
		cfg.MaxOutbound = defaultMaxOutbound
	}
	cm := ConnManager{
		cfg:      *cfg, // Copy so caller can't mutate
		requests: make(chan interface{}),
		quit:     make(chan struct{}),
	}
	return &cm, nil
}
