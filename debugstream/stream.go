package debugstream

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"
)

// S is a global stream variable used to allow the broadcast of debug events.
var S *Stream

type clientState struct {
	conn        *net.TCPConn
	shouldClose bool
	offset      int
}

// StreamServer broadcasts debug events to connected clients.
type StreamServer struct {
	broadcastCh chan struct{}
	events      [][]byte

	mu      sync.Mutex
	clients []clientState

	stop chan struct{}
	done chan struct{}
	wg   sync.WaitGroup
}

// NewStreamServer returns a new stream instance.
func NewStreamServer() *StreamServer {
	done := make(chan struct{})
	close(done)
	stop := make(chan struct{})
	close(stop)
	return &StreamServer{
		broadcastCh: make(chan struct{}, 1),
		stop:        stop,
		done:        done,
	}
}

// Broadcast sends the event to all connected event listeners.
func (s *StreamServer) Broadcast(e Event) {
	select {
	case <-s.stop:
		return
	default:
	}

	var buf bytes.Buffer
	_ = e.write(&buf)
	s.mu.Lock()
	s.events = append(s.events, buf.Bytes())
	s.mu.Unlock()

	select {
	case s.broadcastCh <- struct{}{}:
	default:
	}
}

func (s *StreamServer) sendNewEventsToClient(client *clientState) {
	const maxWait = time.Second

	if client.offset >= len(s.events) {
		// client up to date
		return
	}

	for ; client.offset < len(s.events)-1; client.offset++ {
		client.conn.SetWriteDeadline(time.Now().Add(maxWait))
		_, err := client.conn.Write(s.events[client.offset+1])
		if err != nil {
			strLog.Errorf("send to peer %s: %s",
				client.conn.RemoteAddr(), err)
			client.shouldClose = true
			return
		}
	}
}

func (s *StreamServer) broadcastEvents() {
	const maxConcurrent = 10

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for i := range s.clients {
		peer := &s.clients[i]
		sem <- struct{}{}
		wg.Go(func() {
			s.sendNewEventsToClient(peer)
			<-sem
		})
	}
	wg.Wait()

	df := func(peer clientState) bool {
		if !peer.shouldClose {
			return false
		}
		peer.conn.Close()
		return true
	}
	s.clients = slices.DeleteFunc(s.clients, df)
}

func (s *StreamServer) broadcastLoop() {
	for {
		select {
		case <-s.broadcastCh:
			s.mu.Lock()
			s.broadcastEvents()
			s.mu.Unlock()
		case <-s.stop:
			return
		}
	}
}

type acceptResult struct {
	conn *net.TCPConn
	err  error
}

func (s *StreamServer) connAccepter(connCh chan<- acceptResult,
	lst *net.TCPListener) {

	for {
		conn, err := lst.AcceptTCP()
		if err != nil && errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			connCh <- acceptResult{nil, err}
			continue
		}
		connCh <- acceptResult{conn, nil}
	}
}

func (s *StreamServer) loop(listener *net.TCPListener) {
	broadcastLoopDone := make(chan struct{})
	s.wg.Go(func() {
		s.broadcastLoop()
		close(broadcastLoopDone)
	})

	connCh := make(chan acceptResult)
	connAccepterDone := make(chan struct{})
	s.wg.Go(func() {
		s.connAccepter(connCh, listener)
		close(connAccepterDone)
	})

out:
	for {
		var conn *net.TCPConn

		select {
		case <-s.stop:
			listener.Close()
			break out
		case connRes := <-connCh:
			if connRes.err != nil {
				strLog.Error("accept err: %s", connRes.err)
				continue
			}
			conn = connRes.conn
		}

		s.mu.Lock()
		s.clients = append(s.clients, clientState{
			conn:   conn,
			offset: -1,
		})
		s.mu.Unlock()

		select {
		case s.broadcastCh <- struct{}{}:
		default:
		}
	}

out2:
	for {
		select {
		case <-connAccepterDone:
			break out2
		case <-connCh:
		}
	}

	<-broadcastLoopDone
out3:
	for {
		select {
		case <-s.broadcastCh:
		default:
			break out3
		}
	}

	for _, p := range s.clients {
		p.conn.Close()
	}
	s.clients = nil
	s.events = nil
}

// Listen starts the stream listening at addr.
func (s *StreamServer) Listen(addr string) error {
	select {
	case <-s.done:
		s.done = make(chan struct{})
	default:
		return fmt.Errorf("server is not stopped")
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		close(s.done)
		return err
	}

	s.stop = make(chan struct{})

	s.wg.Go(func() {
		s.loop(listener.(*net.TCPListener))
		close(s.done)
	})

	return nil
}

// Shutdown stops the server.
func (s *StreamServer) Shutdown() {
	select {
	case <-s.stop:
		return
	default:
	}

	close(s.stop)
	s.wg.Wait()
}
