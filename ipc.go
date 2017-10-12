// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Messages sent over a pipe are encoded using a simple binary message format:
//
//   - Protocol version (1 byte, currently 1)
//   - Message type length (1 byte)
//   - Message type string (encoded as UTF8, no longer than 255 bytes)
//   - Message payload length (4 bytes, little endian)
//   - Message payload bytes (no longer than 2^32 - 1 bytes)
type pipeMessage interface {
	Type() string
	PayloadSize() uint32
	WritePayload(w io.Writer) error
}

var outgoingPipeMessages = make(chan pipeMessage)

// serviceControlPipeRx reads from the file descriptor fd of a read end pipe.
// This is intended to be used as a simple control mechanism for parent
// processes to communicate with and and manage the lifetime of a dcrd child
// process using a unidirectional pipe (on Windows, this is an anonymous pipe,
// not a named pipe).
//
// When the pipe is closed or any other errors occur reading the control
// message, shutdown begins.  This prevents dcrd from continuing to run
// unsupervised after the parent process closes unexpectedly.
//
// No control messages are currently defined and the only use for the pipe is to
// start clean shutdown when the pipe is closed.  Control messages that follow
// the pipe message format can be added later as needed.
func serviceControlPipeRx(fd uintptr) {
	pipe := os.NewFile(fd, fmt.Sprintf("|%v", fd))
	r := bufio.NewReader(pipe)
	for {
		_, err := r.Discard(1024)
		if err == io.EOF {
			break
		}
		if err != nil {
			dcrdLog.Errorf("Failed to read from pipe: %v", err)
			break
		}
	}

	select {
	case shutdownRequestChannel <- struct{}{}:
	default:
	}
}

// serviceControlPipeTx sends pipe messages to the file descriptor fd of a write
// end pipe.  This is intended to be a simple response and notification system
// for a child dcrd process to communicate with a parent process without the
// need to go through the RPC server.
//
// See the comment on the pipeMessage interface for the binary encoding of a
// pipe message.
func serviceControlPipeTx(fd uintptr) {
	defer drainOutgoingPipeMessages()

	pipe := os.NewFile(fd, fmt.Sprintf("|%v", fd))
	w := bufio.NewWriter(pipe)
	headerBuffer := make([]byte, 0, 1+1+255+4) // capped to max header size
	var err error
	for m := range outgoingPipeMessages {
		const protocolVersion byte = 1

		mtype := m.Type()
		psize := m.PayloadSize()

		headerBuffer = append(headerBuffer, protocolVersion)
		headerBuffer = append(headerBuffer, byte(len(mtype)))
		headerBuffer = append(headerBuffer, mtype...)
		buf := make([]byte, 4)
		binary.LittleEndian.PutUint32(buf, psize)
		headerBuffer = append(headerBuffer, buf...)

		_, err = w.Write(headerBuffer)
		if err != nil {
			break
		}

		err = m.WritePayload(w)
		if err != nil {
			break
		}

		err = w.Flush()
		if err != nil {
			break
		}

		headerBuffer = headerBuffer[:0]
	}

	dcrdLog.Errorf("Failed to write to pipe: %v", err)
}

func drainOutgoingPipeMessages() {
	for range outgoingPipeMessages {
	}
}

// The lifetimeEvent describes a startup or shutdown event.  The message type
// string is "lifetimeevent".
//
// The payload size is always 2 bytes long.  The first byte describes whether a
// service or event is about to run or whether startup has completed.  The
// second byte, when applicable, describes which event or service is about to
// start or stop.
//
//   0 <event id>:  The startup event is about to run
//   1 <ignored>:   All startup tasks have completed
//   2 <event id>:  The shutdown event is about to run
//
// Event IDs can take on the following values:
//
//   0: Database opening/closing
//   1: Ticket database opening/closing
//   2: Peer-to-peer server starting/stopping
//
// Note that not all subsystems are started/stopped or events run during the
// program's lifetime depending on what features are enabled through the config.
//
// As an example, the following messages may be sent during a typical execution:
//
//   0 0: The database is being opened
//   0 1: The ticket DB is being opened
//   0 2: The P2P server is starting
//   1 0: All startup tasks have completed
//   2 2: The P2P server is stopping
//   2 1: The ticket DB is being closed and written to disk
//   2 0: The database is being closed
type lifetimeEvent struct {
	event  lifetimeEventID
	action lifetimeAction
}

var _ pipeMessage = (*lifetimeEvent)(nil)

type lifetimeEventID byte

const (
	startupEvent lifetimeEventID = iota
	startupComplete
	shutdownEvent
)

type lifetimeAction byte

const (
	lifetimeEventDBOpen lifetimeAction = iota
	lifetimeEventP2PServer
)

func (*lifetimeEvent) Type() string          { return "lifetimeevent" }
func (e *lifetimeEvent) PayloadSize() uint32 { return 2 }
func (e *lifetimeEvent) WritePayload(w io.Writer) error {
	_, err := w.Write([]byte{byte(e.event), byte(e.action)})
	return err
}

type lifetimeEventServer chan<- pipeMessage

func newLifetimeEventServer(outChan chan<- pipeMessage) lifetimeEventServer {
	return lifetimeEventServer(outChan)
}

func (s lifetimeEventServer) notifyStartupEvent(action lifetimeAction) {
	if s == nil {
		return
	}
	s <- &lifetimeEvent{
		event:  startupEvent,
		action: action,
	}
}

func (s lifetimeEventServer) notifyStartupComplete() {
	if s == nil {
		return
	}
	s <- &lifetimeEvent{
		event:  startupComplete,
		action: 0,
	}
}

func (s lifetimeEventServer) notifyShutdownEvent(action lifetimeAction) {
	if s == nil {
		return
	}
	s <- &lifetimeEvent{
		event:  shutdownEvent,
		action: action,
	}
}
