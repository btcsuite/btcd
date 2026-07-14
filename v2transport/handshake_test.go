package v2transport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// splitReadWriter keeps bytes read by the transport separate from bytes the
// transport writes in response.
type splitReadWriter struct {
	reader      *bytes.Reader
	writes      bytes.Buffer
	beforeWrite func()
}

// testHandshakeAdmission adapts a function to HandshakeAdmission so each test
// can record when the responder enters and leaves the CPU-bound phase.
type testHandshakeAdmission func() (func(), error)

// Acquire invokes the test admission function.
func (a testHandshakeAdmission) Acquire() (func(), error) {
	return a()
}

func newSplitReadWriter(input []byte) *splitReadWriter {
	return &splitReadWriter{reader: bytes.NewReader(input)}
}

func (rw *splitReadWriter) Read(p []byte) (int, error) {
	return rw.reader.Read(p)
}

func (rw *splitReadWriter) Write(p []byte) (int, error) {
	if rw.beforeWrite != nil {
		rw.beforeWrite()
	}

	return rw.writes.Write(p)
}

// bufferedReadWriter is one endpoint of a buffered in-memory duplex pipe.
type bufferedReadWriter struct {
	recv <-chan []byte
	send chan<- []byte
	buf  bytes.Buffer
}

func newBufferedReadWriterPair() (*bufferedReadWriter, *bufferedReadWriter) {
	leftToRight := make(chan []byte, 32)
	rightToLeft := make(chan []byte, 32)

	return &bufferedReadWriter{
			recv: rightToLeft,
			send: leftToRight,
		}, &bufferedReadWriter{
			recv: leftToRight,
			send: rightToLeft,
		}
}

func (rw *bufferedReadWriter) Read(p []byte) (int, error) {
	if rw.buf.Len() == 0 {
		rw.buf.Write(<-rw.recv)
	}

	return rw.buf.Read(p)
}

func (rw *bufferedReadWriter) Write(p []byte) (int, error) {
	msg := append([]byte(nil), p...)
	rw.send <- msg

	return len(p), nil
}

// TestResponderV1Fallback verifies a full v1 prefix does not consume CPU
// admission or generate responder key material.
func TestResponderV1Fallback(t *testing.T) {
	var admissions int
	peer := NewPeerWithOptions(WithResponderHandshakeAdmission(
		testHandshakeAdmission(func() (func(), error) {
			admissions++
			return func() {}, nil
		}),
	))
	rw := newSplitReadWriter(createV1Prefix(BitcoinNet(0xd9b4bef9)))
	peer.UseReadWriter(rw)

	err := peer.RespondV2Handshake(0, BitcoinNet(0xd9b4bef9))
	if !errors.Is(err, ErrUseV1Protocol) {
		t.Fatalf("unexpected fallback error: %v", err)
	}
	if admissions != 0 {
		t.Fatalf("v1 fallback consumed %d CPU admissions", admissions)
	}
	if peer.privkeyOurs != nil {
		t.Fatal("v1 fallback generated responder key material")
	}
	if rw.writes.Len() != 0 {
		t.Fatalf("v1 fallback wrote %d bytes", rw.writes.Len())
	}
}

// TestResponderIncompleteCandidate verifies incomplete v2 candidates do not
// consume CPU admission or generate responder key material.
func TestResponderIncompleteCandidate(t *testing.T) {
	for _, length := range []int{1, 15, 16, 63} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			var admissions int
			peer := NewPeerWithOptions(WithResponderHandshakeAdmission(
				testHandshakeAdmission(func() (func(), error) {
					admissions++
					return func() {}, nil
				}),
			))
			candidate := make([]byte, length)
			candidate[0] = 1
			rw := newSplitReadWriter(candidate)
			peer.UseReadWriter(rw)

			err := peer.RespondV2Handshake(0, BitcoinNet(0xd9b4bef9))
			if err == nil {
				t.Fatal("incomplete candidate unexpectedly succeeded")
			}
			if admissions != 0 {
				t.Fatalf("incomplete candidate consumed %d admissions",
					admissions)
			}
			if peer.privkeyOurs != nil {
				t.Fatal("incomplete candidate generated responder key")
			}
			if rw.writes.Len() != 0 {
				t.Fatalf("incomplete candidate wrote %d bytes",
					rw.writes.Len())
			}
		})
	}
}

// TestResponderWrongNetworkV1 verifies wrong-network v1 is rejected before
// CPU admission and key generation.
func TestResponderWrongNetworkV1(t *testing.T) {
	const expectedNet = BitcoinNet(0xd9b4bef9)

	candidate := make([]byte, 64)
	copy(candidate, createV1Prefix(BitcoinNet(0x0709110b)))

	var admissions int
	peer := NewPeerWithOptions(WithResponderHandshakeAdmission(
		testHandshakeAdmission(func() (func(), error) {
			admissions++
			return func() {}, nil
		}),
	))
	rw := newSplitReadWriter(candidate)
	peer.UseReadWriter(rw)

	err := peer.RespondV2Handshake(0, expectedNet)
	if !errors.Is(err, errWrongNetV1Peer) {
		t.Fatalf("unexpected wrong-network error: %v", err)
	}
	if admissions != 0 {
		t.Fatalf("wrong-network v1 consumed %d admissions", admissions)
	}
	if peer.privkeyOurs != nil {
		t.Fatal("wrong-network v1 generated responder key")
	}
}

// TestResponderAdmissionRejected verifies denial occurs before all responder
// cryptography and network writes.
func TestResponderAdmissionRejected(t *testing.T) {
	errRejected := errors.New("rejected")
	candidate := make([]byte, 64)
	for i := range candidate {
		candidate[i] = byte(i)
	}

	var admissions int
	peer := NewPeerWithOptions(WithResponderHandshakeAdmission(
		testHandshakeAdmission(func() (func(), error) {
			admissions++
			return nil, errRejected
		}),
	))
	rw := newSplitReadWriter(candidate)
	peer.UseReadWriter(rw)

	err := peer.RespondV2Handshake(0, BitcoinNet(0xd9b4bef9))
	if !errors.Is(err, errRejected) {
		t.Fatalf("unexpected admission error: %v", err)
	}
	if admissions != 1 {
		t.Fatalf("admission called %d times, want 1", admissions)
	}
	if peer.privkeyOurs != nil {
		t.Fatal("rejected candidate generated responder key")
	}
	if rw.writes.Len() != 0 {
		t.Fatalf("rejected candidate wrote %d bytes", rw.writes.Len())
	}
}

// TestResponderAdmissionLease verifies the admission lease covers responder
// cryptography and is released before the response is written.
func TestResponderAdmissionLease(t *testing.T) {
	candidate := make([]byte, 64)
	for i := range candidate {
		candidate[i] = byte(i)
	}

	var (
		admissions int
		releases   int
	)
	peer := NewPeerWithOptions(WithResponderHandshakeAdmission(
		testHandshakeAdmission(func() (func(), error) {
			admissions++
			return func() { releases++ }, nil
		}),
	))
	rw := newSplitReadWriter(candidate)
	rw.beforeWrite = func() {
		if releases != 1 {
			t.Fatalf("response write began before lease release: got %d", releases)
		}
	}
	peer.UseReadWriter(rw)

	if err := peer.RespondV2Handshake(0, BitcoinNet(0xd9b4bef9)); err != nil {
		t.Fatalf("responder handshake failed: %v", err)
	}
	if admissions != 1 || releases != 1 {
		t.Fatalf("unexpected lease counts: admissions=%d releases=%d",
			admissions, releases)
	}
	if peer.privkeyOurs == nil || !peer.responderReady {
		t.Fatal("accepted responder did not initialize key material")
	}
	if rw.writes.Len() != 64 {
		t.Fatalf("responder wrote %d bytes, want 64", rw.writes.Len())
	}
}

// TestResponderHandshakeInteroperability verifies the refactored responder
// ordering preserves the complete v2 wire transcript.
func TestResponderHandshakeInteroperability(t *testing.T) {
	const testNet = BitcoinNet(0xd9b4bef9)

	initiatorRW, responderRW := newBufferedReadWriterPair()
	initiator := NewPeer()
	responder := NewPeer()
	initiator.UseReadWriter(initiatorRW)
	responder.UseReadWriter(responderRW)

	errs := make(chan error, 2)
	go func() {
		if err := initiator.InitiateV2Handshake(0); err != nil {
			errs <- err
			return
		}
		errs <- initiator.CompleteHandshake(true, []int{0, 8}, testNet)
	}()
	go func() {
		if err := responder.RespondV2Handshake(0, testNet); err != nil {
			errs <- err
			return
		}
		errs <- responder.CompleteHandshake(false, []int{3}, testNet)
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("handshake failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("handshake timed out")
		}
	}

	payload := []byte("post-handshake payload")
	if _, _, err := initiator.V2EncPacket(payload, nil, false); err != nil {
		t.Fatalf("packet send failed: %v", err)
	}
	got, err := responder.V2ReceivePacket(nil)
	if err != nil {
		t.Fatalf("packet receive failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("packet mismatch: got %x, want %x", got, payload)
	}
}

// TestSendShortWrite verifies short writes are surfaced to callers.
func TestSendShortWrite(t *testing.T) {
	peer := NewPeer()
	peer.UseReadWriter(shortReadWriter{})

	if _, err := peer.Send([]byte{1, 2}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("unexpected short-write error: %v", err)
	}
}

type shortReadWriter struct{}

func (shortReadWriter) Read([]byte) (int, error)  { return 0, io.EOF }
func (shortReadWriter) Write([]byte) (int, error) { return 1, nil }
