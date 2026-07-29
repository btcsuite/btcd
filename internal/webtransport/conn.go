package webtransport

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	wt "github.com/quic-go/webtransport-go"
)

type stream interface {
	io.Reader
	io.Writer
	CancelRead(wt.StreamErrorCode)
	CancelWrite(wt.StreamErrorCode)
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type session interface {
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	CloseWithError(wt.SessionErrorCode, string) error
}

// streamConn turns the two half-close operations of a WebTransport stream into
// the full-close behavior required by net.Conn. Closing the sole stream also
// closes its session and underlying QUIC connection, so no HTTP/3 state can
// outlive the btcd peer that owns it.
type streamConn struct {
	stream          stream
	session         session
	closeConnection func() error

	closeOnce sync.Once
	closeErr  error
}

var _ net.Conn = (*streamConn)(nil)

func newStreamConn(stream stream, session session,
	closeConnection func() error) *streamConn {

	return &streamConn{
		stream:          stream,
		session:         session,
		closeConnection: closeConnection,
	}
}

func (c *streamConn) Read(buffer []byte) (int, error) {
	n, err := c.stream.Read(buffer)
	if isNormalRemoteClose(err) {
		return n, io.EOF
	}

	return n, err
}

func (c *streamConn) Write(buffer []byte) (int, error) {
	n, err := c.stream.Write(buffer)
	if isNormalRemoteClose(err) {
		return n, net.ErrClosed
	}

	return n, err
}

// isNormalRemoteClose gives a peer's normal WebTransport session close the
// same net.Conn semantics as a clean TCP shutdown.  Without this conversion,
// btcd treats the empty SessionError as a malformed Bitcoin message and tries
// to send a reject message to a peer that has already gone away.
func isNormalRemoteClose(err error) bool {
	var sessionErr *wt.SessionError
	return errors.As(err, &sessionErr) && sessionErr.Remote &&
		sessionErr.ErrorCode == 0
}

func (c *streamConn) Close() error {
	c.closeOnce.Do(func() {
		c.stream.CancelRead(0)
		c.stream.CancelWrite(0)
		sessionErr := c.session.CloseWithError(0, "")
		var connectionErr error
		if c.closeConnection != nil {
			connectionErr = c.closeConnection()
		}
		c.closeErr = errors.Join(sessionErr, connectionErr)
	})

	return c.closeErr
}

func (c *streamConn) LocalAddr() net.Addr {
	return c.session.LocalAddr()
}

func (c *streamConn) RemoteAddr() net.Addr {
	return c.session.RemoteAddr()
}

func (c *streamConn) SetDeadline(deadline time.Time) error {
	return c.stream.SetDeadline(deadline)
}

func (c *streamConn) SetReadDeadline(deadline time.Time) error {
	return c.stream.SetReadDeadline(deadline)
}

func (c *streamConn) SetWriteDeadline(deadline time.Time) error {
	return c.stream.SetWriteDeadline(deadline)
}
