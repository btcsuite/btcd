package debugstream

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"
)

func genTCPListenAddr(t *testing.T) string {
	conn, err := net.Listen("tcp4", "")
	if err != nil {
		t.Fatal(err)
	}
	addrPort := strings.Split(conn.Addr().String(), ":")
	conn.Close()
	return fmt.Sprintf("127.0.0.1:%s", addrPort[1])
}

// TestStreamProcedureSignatures verifies that the same [Stream] public
// interface is used with and without the debug build tag.
func TestStreamProcedureSignatures(t *testing.T) {
	t.Parallel()

	stream := New()
	err := stream.Listen(genTCPListenAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	stream.Broadcast(Event{Code: 0, Data: nil})
	stream.Shutdown()
}

func testStreamServer(t *testing.T) {
	listenAddr := genTCPListenAddr(t)
	stream := NewStreamServer()
	err := stream.Listen(listenAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Shutdown()

	tests := []Event{
		{Code: 1, Data: []byte("event 1")},
		{Code: 2, Data: []byte("")},
		{Code: 3, Data: []byte("event 3")},
	}

	const (
		stStart uint64 = iota
		st1
		st2
		stEnd
	)
	var state uint64
	okCh := make(chan struct{})
	h := func(e Event) {
		switch {
		case state == stStart && e.Code == 1 &&
			bytes.Equal(tests[0].Data, e.Data):

			state = st1

		case state == st1 && e.Code == 2 &&
			bytes.Equal(tests[1].Data, e.Data):

			state = st2

		case state == st2 && e.Code == 3 &&
			bytes.Equal(tests[2].Data, e.Data):

			state = stEnd
			okCh <- struct{}{}
		}
	}

	client := NewClient(listenAddr, h)
	err = client.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Stop()

	// broadcast the debug events
	for _, e := range tests {
		stream.Broadcast(e)
	}

	select {
	case <-okCh:
	case <-time.After(time.Second * 10):
		t.Fatal("timeout")
	}

	if state != stEnd {
		t.Fatal("unexpected end state", state)
	}
}

func maxGoroutineLeak(max int) func(t *testing.T) {
	gstart := runtime.NumGoroutine()
	return func(t *testing.T) {
		time.Sleep(time.Millisecond * 50)
		gend := runtime.NumGoroutine()
		if gend-gstart <= max {
			return
		}
		t.Fatalf("%d goroutine leaks", gend-gstart)
	}
}

func TestStreamServerAndClient(t *testing.T) {
	t.Parallel()
	defer maxGoroutineLeak(0)(t)

	const count = 3
	for range count {
		testStreamServer(t)
	}
}

func TestStreamServerAndClientRestart(t *testing.T) {
	t.Parallel()
	defer maxGoroutineLeak(0)(t)

	listenAddr := genTCPListenAddr(t)
	stream := NewStreamServer()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second*10)
	defer cancel()
	okCh := make(chan struct{}, 1)
	hf := func(e Event) {
		if e.Code != 1 {
			return
		}
		okCh <- struct{}{}
	}
	client := NewClient(listenAddr, hf)

	matchF := func(t *testing.T) {
		err := stream.Listen(listenAddr)
		if err != nil {
			t.Fatal(err)
		}

		// broadcast the debug event
		stream.Broadcast(Event{Code: 1})

		err = client.Start()
		if err != nil {
			t.Fatal(err)
		}

		select {
		case <-okCh:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		client.Stop()
		stream.Shutdown()
	}

	const count = 3
	for range count {
		matchF(t)
	}
}

func setupStream(t *testing.T) (string, func()) {
	listenAddr := genTCPListenAddr(t)
	stream := NewStreamServer()
	err := stream.Listen(listenAddr)
	if err != nil {
		t.Fatal(err)
	}

	return listenAddr, stream.Shutdown
}

func TestClientStartStop(t *testing.T) {
	t.Parallel()
	defer maxGoroutineLeak(0)(t)

	listenAddr, cleanup := setupStream(t)
	defer cleanup()

	client := NewClient(listenAddr, func(_ Event) {})

	// Must not return error when started successfully.
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}

	// Must not be started again while running.
	if err := client.Start(); err == nil {
		t.Fatal(err)
	}

	// Must be started successfully after disconnecting from the server.
	if err := client.conn.Close(); err != nil {
		t.Fatal(err)
	}
	<-client.done
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}

	// Stop can be called multiple times.
	client.Stop()
	client.Stop()
	client.Stop()
}
