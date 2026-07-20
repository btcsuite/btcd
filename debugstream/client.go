package debugstream

import (
	"fmt"
	"net"
	"time"
)

// Client calls handler with events coming from the [StreamServer].
type Client struct {
	addr string

	conn    *net.TCPConn
	handler func(e Event)

	stop chan struct{}
	done chan struct{}
}

// NewClient initializes a [Client] instance.
func NewClient(streamAddr string, handler func(ev Event)) *Client {
	done := make(chan struct{})
	close(done)
	stop := make(chan struct{})
	close(stop)
	return &Client{
		addr:    streamAddr,
		handler: handler,
		done:    done,
		stop:    stop,
	}
}

func (c *Client) connect() (*net.TCPConn, error) {
	const maxConnAttempts = 7
	var (
		conn net.Conn
		err  error
	)

	for i := range maxConnAttempts {
		if i > 0 {
			time.Sleep((time.Millisecond * 100) << (i - 1))
		}
		conn, err = net.Dial("tcp", c.addr)
		if err != nil {
			continue
		}
		break
	}
	if err != nil {
		return nil, err
	}

	return conn.(*net.TCPConn), nil
}

func (c *Client) loop() {
	for {
		var ev Event
		err := ev.read(c.conn)
		if err == nil {
			c.handler(ev)
			continue
		}
		select {
		case <-c.stop:
			return
		default:
			cliLog.Errorf("client loop %s", err)
			return
		}
	}
}

func (c *Client) Start() error {
	select {
	case <-c.done:
		c.done = make(chan struct{})
	default:
		return fmt.Errorf("client running")
	}

	conn, err := c.connect()
	if err != nil {
		close(c.done)
		return err
	}

	c.conn = conn
	c.stop = make(chan struct{})

	go func() {
		c.loop()
		close(c.done)
	}()

	return nil
}

func (c *Client) Stop() {
	select {
	case <-c.stop:
		return
	default:
	}

	close(c.stop)
	c.conn.Close()
	<-c.done

	c.conn = nil
}
