// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
)

func countPayload(t *testing.T, prefix []byte, count uint64) []byte {
	t.Helper()

	var payload bytes.Buffer
	_, err := payload.Write(prefix)
	if err != nil {
		t.Fatalf("unable to write payload prefix: %v", err)
	}
	if err := WriteVarInt(&payload, ProtocolVersion, count); err != nil {
		t.Fatalf("unable to write element count: %v", err)
	}

	return payload.Bytes()
}

// TestTruncatedVectorDecodeBudget ensures a legal maximum element count cannot
// reserve the corresponding maximum slice when no elements follow it.  Each
// decode must fail on the truncated payload while having reserved no element
// storage at all.
func TestTruncatedVectorDecodeBudget(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		decode  func(io.Reader) (int, error)
	}{
		{
			name:    "inventory",
			payload: countPayload(t, nil, MaxInvPerMsg),
			decode: func(r io.Reader) (int, error) {
				var msg MsgInv
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.InvList), err
			},
		},
		{
			name:    "getdata",
			payload: countPayload(t, nil, MaxInvPerMsg),
			decode: func(r io.Reader) (int, error) {
				var msg MsgGetData
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.InvList), err
			},
		},
		{
			name:    "notfound",
			payload: countPayload(t, nil, MaxInvPerMsg),
			decode: func(r io.Reader) (int, error) {
				var msg MsgNotFound
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.InvList), err
			},
		},
		{
			name:    "addresses",
			payload: countPayload(t, nil, MaxAddrPerMsg),
			decode: func(r io.Reader) (int, error) {
				var msg MsgAddr
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.AddrList), err
			},
		},
		{
			name:    "addresses v2",
			payload: countPayload(t, nil, MaxV2AddrPerMsg),
			decode: func(r io.Reader) (int, error) {
				var msg MsgAddrV2
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.AddrList), err
			},
		},
		{
			name:    "headers",
			payload: countPayload(t, nil, MaxBlockHeadersPerMsg),
			decode: func(r io.Reader) (int, error) {
				var msg MsgHeaders
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.Headers), err
			},
		},
		{
			name: "getblocks locators",
			payload: countPayload(
				t, make([]byte, 4), MaxBlockLocatorsPerMsg,
			),
			decode: func(r io.Reader) (int, error) {
				var msg MsgGetBlocks
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.BlockLocatorHashes), err
			},
		},
		{
			name: "getheaders locators",
			payload: countPayload(
				t, make([]byte, 4), MaxBlockLocatorsPerMsg,
			),
			decode: func(r io.Reader) (int, error) {
				var msg MsgGetHeaders
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.BlockLocatorHashes), err
			},
		},
		{
			name: "compact filter headers",
			payload: countPayload(
				t, make([]byte, 65), MaxCFHeadersPerMsg,
			),
			decode: func(r io.Reader) (int, error) {
				var msg MsgCFHeaders
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.FilterHashes), err
			},
		},
		{
			name: "compact filter checkpoints",
			payload: countPayload(
				t, make([]byte, 33), maxCFHeadersLen,
			),
			decode: func(r io.Reader) (int, error) {
				var msg MsgCFCheckpt
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.FilterHeaders), err
			},
		},
		{
			name: "merkle block hashes",
			payload: countPayload(
				t, make([]byte, MaxBlockHeaderPayload+4), maxTxPerBlock,
			),
			decode: func(r io.Reader) (int, error) {
				var msg MsgMerkleBlock
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.Hashes), err
			},
		},
		{
			name: "block transactions",
			payload: countPayload(
				t, make([]byte, MaxBlockHeaderPayload), maxTxPerBlock,
			),
			decode: func(r io.Reader) (int, error) {
				var msg MsgBlock
				err := msg.BtcDecode(r, ProtocolVersion, BaseEncoding)
				return cap(msg.Transactions), err
			},
		},
		{
			name: "block transaction locations",
			payload: countPayload(
				t, make([]byte, MaxBlockHeaderPayload), maxTxPerBlock,
			),
			decode: func(r io.Reader) (int, error) {
				// DeserializeTxLoc takes a *bytes.Buffer rather
				// than a generic io.Reader, so copy the payload
				// into one.  The copy is made by the test, not
				// the decode being measured.
				data, readErr := io.ReadAll(r)
				if readErr != nil {
					return 0, readErr
				}
				var msg MsgBlock
				_, err := msg.DeserializeTxLoc(bytes.NewBuffer(data))
				return cap(msg.Transactions), err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capacity, err := test.decode(bytes.NewReader(test.payload))
			if err == nil {
				t.Fatal("expected a truncated payload error")
			}
			if capacity != 0 {
				t.Fatalf("decode reserved %d elements, want 0",
					capacity)
			}
		})
	}
}

// TestTruncatedTxDecodeBudget covers each count-driven transaction slice.
func TestTruncatedTxDecodeBudget(t *testing.T) {
	// Inspect the internal decoder before BtcDecode clears failed results,
	// so public error cleanup cannot hide an oversized reservation.
	decode := func(t *testing.T, payload []byte) (*MsgTx, error) {
		t.Helper()
		ar := borrowScriptArena()
		t.Cleanup(ar.release)
		buf := binarySerializer.Borrow()
		t.Cleanup(func() { binarySerializer.Return(buf) })
		msg := new(MsgTx)
		err := msg.btcDecode(
			bytes.NewReader(payload), ProtocolVersion, WitnessEncoding,
			buf, ar,
		)
		return msg, err
	}

	t.Run("inputs", func(t *testing.T) {
		payload := countPayload(t, make([]byte, 4), maxTxInPerMessage)
		msg, err := decode(t, payload)
		if err == nil {
			t.Fatal("expected a truncated input error")
		}
		if len(msg.TxIn) != 0 {
			t.Fatalf("decode retained %d inputs", len(msg.TxIn))
		}
	})

	inputPrefix := make([]byte, 4)
	inputPrefix = append(inputPrefix, 1)
	inputPrefix = append(inputPrefix, make([]byte, minTxInPayload)...)

	t.Run("outputs", func(t *testing.T) {
		payload := countPayload(t, inputPrefix, maxTxOutPerMessage)
		msg, err := decode(t, payload)
		if err == nil {
			t.Fatal("expected a truncated output error")
		}
		if len(msg.TxOut) != 0 {
			t.Fatalf("decode retained %d outputs", len(msg.TxOut))
		}
	})

	t.Run("witness items", func(t *testing.T) {
		witnessPrefix := make([]byte, 4)
		witnessPrefix = append(witnessPrefix, TxFlagMarker, WitnessFlag, 1)
		witnessPrefix = append(witnessPrefix, make([]byte, minTxInPayload)...)
		witnessPrefix = append(witnessPrefix, 0)
		payload := countPayload(
			t, witnessPrefix, maxWitnessItemsPerInput,
		)

		msg, err := decode(t, payload)
		if err == nil {
			t.Fatal("expected a truncated witness error")
		}
		if len(msg.TxIn) != 1 {
			t.Fatalf("decoded %d inputs, want 1", len(msg.TxIn))
		}
		if capacity := cap(msg.TxIn[0].Witness); capacity >
			defaultTxInOutAlloc {

			t.Fatalf("decode reserved %d witness items, want at most %d",
				capacity, defaultTxInOutAlloc)
		}
	})
}

// TestTruncatedVariableBytesBudget ensures inner length prefixes grow memory
// with bytes delivered rather than with the claimed length.
func TestTruncatedVariableBytesBudget(t *testing.T) {
	var encoded bytes.Buffer
	if err := WriteVarInt(&encoded, ProtocolVersion, MaxMessagePayload); err != nil {
		t.Fatalf("unable to encode length: %v", err)
	}
	prefix := encoded.Bytes()

	truncated := append(bytes.Clone(prefix), 0x01)
	reader := &readSizeRecorder{reader: bytes.NewReader(truncated)}
	decoded, err := ReadVarBytes(
		reader, ProtocolVersion, MaxMessagePayload,
		"payload",
	)
	if err == nil {
		t.Fatal("expected a truncated byte payload error")
	}
	if decoded != nil {
		t.Fatalf("decoded %d bytes on error", len(decoded))
	}
	if reader.max > defaultReadBufferSize {
		t.Fatalf("byte payload read requested %d bytes, want at most %d",
			reader.max, defaultReadBufferSize)
	}

	reader = &readSizeRecorder{reader: bytes.NewReader(truncated)}
	decodedString, err := ReadVarString(
		reader, ProtocolVersion,
	)
	if err == nil {
		t.Fatal("expected a truncated string error")
	}
	if decodedString != "" {
		t.Fatalf("decoded %d string bytes on error", len(decodedString))
	}
	if reader.max > defaultReadBufferSize {
		t.Fatalf("string payload read requested %d bytes, want at most %d",
			reader.max, defaultReadBufferSize)
	}

	result, err := readBytes(
		&readSizeRecorder{reader: bytes.NewReader([]byte{0x01})},
		MaxMessagePayload,
	)
	if err == nil {
		t.Fatal("expected a truncated direct read error")
	}
	if len(result) != 1 {
		t.Fatalf("direct read returned %d bytes, want 1", len(result))
	}
	if capacity := cap(result); capacity > defaultReadBufferSize {
		t.Fatalf("direct read reserved %d bytes, want at most %d", capacity,
			defaultReadBufferSize)
	}

	// A read that claims the maximum but delivers progressively more
	// bytes must reserve memory proportional to what was delivered, not
	// to what was claimed: each truncated read hands back only the bytes
	// that actually arrived, staged in bounded chunks.
	for _, delivered := range []int{
		1, defaultReadBufferSize, 3*defaultReadBufferSize + 7,
	} {
		result, err := readBytes(
			&readSizeRecorder{reader: bytes.NewReader(
				make([]byte, delivered),
			)},
			MaxMessagePayload,
		)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("delivered %d bytes: unexpected error: got %v, "+
				"want %v", delivered, err, io.ErrUnexpectedEOF)
		}
		if len(result) != delivered {
			t.Fatalf("delivered %d bytes: read returned %d bytes, "+
				"want %d", delivered, len(result), delivered)
		}

		// The staged chunks hold the delivered bytes in geometrically
		// sized pieces, so the total reservation may exceed the
		// delivered length by at most one final chunk.
		if capacity := cap(result); capacity > delivered+defaultReadBufferSize {
			t.Fatalf("delivered %d bytes: read reserved %d bytes, "+
				"want at most %d", delivered, capacity,
				delivered+defaultReadBufferSize)
		}
	}
}

// TestCompleteStreamingBytesCapacity ensures a complete opaque read returns an
// exactly sized payload instead of retaining capacity beyond its length.
func TestCompleteStreamingBytesCapacity(t *testing.T) {
	payload := bytes.Repeat(
		[]byte{0x01}, defaultReadBufferSize+1,
	)
	reader := &readSizeRecorder{reader: bytes.NewReader(payload)}
	result, err := readBytes(reader, uint64(len(payload)))
	if err != nil {
		t.Fatalf("unable to read complete payload: %v", err)
	}
	if !bytes.Equal(result, payload) {
		t.Fatal("decoded payload differs from input")
	}
	if cap(result) != len(result) {
		t.Fatalf("payload capacity is %d, want %d", cap(result), len(result))
	}
}

// TestStreamingBytesPreservesBoundaryError ensures an error returned with a
// full internal chunk is not discarded while the payload remains incomplete.
func TestStreamingBytesPreservesBoundaryError(t *testing.T) {
	errBoundary := errors.New("chunk boundary")
	reader := &boundaryErrorReader{
		payload: bytes.Repeat([]byte{0x01}, defaultReadBufferSize),
		err:     errBoundary,
	}
	result, err := readBytes(reader, defaultReadBufferSize+1)
	if !errors.Is(err, errBoundary) {
		t.Fatalf("unexpected read error: got %v, want %v", err, errBoundary)
	}
	if len(result) != defaultReadBufferSize {
		t.Fatalf("payload length is %d, want %d", len(result),
			defaultReadBufferSize)
	}
	if reader.reads != 1 {
		t.Fatalf("reader called %d times, want 1", reader.reads)
	}
}

// readSizeRecorder wraps a reader so decoders cannot measure the number of
// bytes it has remaining, which forces every speculative-read guard to take
// its streaming path.  It also records the largest single read the caller
// requested, so a decode that reserved or demanded a huge buffer up front is
// detectable as an oversized request rather than only as an allocation.
type readSizeRecorder struct {
	reader io.Reader
	max    int
}

func (r *readSizeRecorder) Read(p []byte) (int, error) {
	if len(p) > r.max {
		r.max = len(p)
	}

	return r.reader.Read(p)
}

type boundaryErrorReader struct {
	payload []byte
	err     error
	reads   int
}

func (r *boundaryErrorReader) Read(p []byte) (int, error) {
	r.reads++
	if r.reads > 1 {
		return 0, io.EOF
	}

	return copy(p, r.payload), r.err
}

type inflatedLenReader struct {
	reader io.Reader
	length int
}

func (r *inflatedLenReader) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *inflatedLenReader) Len() int {
	return r.length
}

// TestTruncatedMessagePayloadBudget ensures the v1 frame reader does not
// reserve or request the full payload after receiving only its header.
func TestTruncatedMessagePayloadBudget(t *testing.T) {
	header := makeHeader(
		MainNet, CmdBlock, MaxProtocolMessageLength, 0,
	)
	reader := &readSizeRecorder{reader: bytes.NewReader(header)}
	_, _, _, err := ReadMessageN(reader, ProtocolVersion, MainNet)
	if err == nil {
		t.Fatal("expected a truncated payload error")
	}
	if reader.max > defaultReadBufferSize {
		t.Fatalf("payload read requested %d bytes, want at most %d",
			reader.max, defaultReadBufferSize)
	}

	// The header claims the protocol maximum payload, so a decoder that
	// trusted the claim would reserve tens of megabytes before any
	// payload bytes arrived.  Measure the total bytes allocated while
	// reading the truncated message and require the reservation to stay
	// far below the claimed payload.
	runs := 100
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < runs; i++ {
		_, _, _, err := ReadMessageN(
			&readSizeRecorder{reader: bytes.NewReader(header)},
			ProtocolVersion, MainNet,
		)
		if err == nil {
			t.Fatal("expected a truncated payload error")
		}
	}
	runtime.ReadMemStats(&after)
	totalAlloc := after.TotalAlloc - before.TotalAlloc
	if budget := uint64(runs) * (1 << 20); totalAlloc > budget {
		t.Fatalf("truncated header read allocated %d bytes across %d "+
			"runs, want at most %d (the header claims a %d byte "+
			"payload)", totalAlloc, runs, budget,
			MaxProtocolMessageLength)
	}

	payload := append(makeHeader(MainNet, CmdPing, 8, 0), 0x01)
	faultingReader := &inflatedLenReader{
		reader: bytes.NewReader(payload),
		length: len(payload) + 8,
	}
	totalBytes, _, _, err := ReadMessageN(
		faultingReader, ProtocolVersion, MainNet,
	)
	if err == nil {
		t.Fatal("expected a short payload error")
	}
	if totalBytes != len(payload) {
		t.Fatalf("read counted %d bytes, want %d", totalBytes, len(payload))
	}
}

// TestStreamingTxDecodeBudget verifies that transaction vectors grow as a
// reader without a measurable remainder supplies each element. The element
// counts cross the initial streaming capacity to exercise backing slice
// growth and final pointer wiring.
func TestStreamingTxDecodeBudget(t *testing.T) {
	const elementCount = defaultStreamingElementCap + 42

	msg := NewMsgTx(2)
	for i := 0; i < elementCount; i++ {
		hash := chainhash.Hash{byte(i), byte(i >> 8)}
		prevOut := NewOutPoint(&hash, uint32(i))
		witness := make([][]byte, elementCount)
		for j := range witness {
			witness[j] = []byte{byte(i), byte(j)}
		}

		msg.AddTxIn(NewTxIn(
			prevOut, []byte{0x01, byte(i)}, witness,
		))
	}
	for i := 0; i < elementCount; i++ {
		msg.AddTxOut(NewTxOut(
			int64(i), []byte{0x02, byte(i)},
		))
	}

	var encoded bytes.Buffer
	if err := msg.Serialize(&encoded); err != nil {
		t.Fatalf("unable to encode transaction: %v", err)
	}

	// The readSizeRecorder hides the reader's remaining length, which is
	// what forces the decode to grow its element storage instead of
	// preallocating the full count the way a buffered reader would.
	reader := &readSizeRecorder{
		reader: bytes.NewReader(encoded.Bytes()),
	}
	var decoded MsgTx
	if err := decoded.BtcDecode(
		reader, ProtocolVersion, WitnessEncoding,
	); err != nil {
		t.Fatalf("unable to decode transaction: %v", err)
	}

	if len(decoded.TxIn) != elementCount {
		t.Fatalf("decoded %d inputs, want %d", len(decoded.TxIn),
			elementCount)
	}
	if cap(decoded.TxIn) > 2*elementCount {
		t.Fatalf("streaming decode reserved %d input slots, want at "+
			"most %d", cap(decoded.TxIn), 2*elementCount)
	}

	var reencoded bytes.Buffer
	if err := decoded.Serialize(&reencoded); err != nil {
		t.Fatalf("unable to re-encode transaction: %v", err)
	}
	if !bytes.Equal(encoded.Bytes(), reencoded.Bytes()) {
		t.Fatal("streaming transaction decode changed the encoding")
	}
}

// TestStreamingCFCheckptDecodeBudget verifies that compact filter checkpoint
// hashes grow as a reader without a measurable remainder supplies them.
func TestStreamingCFCheckptDecodeBudget(t *testing.T) {
	const headerCount = defaultStreamingElementCap + 42

	stopHash := chainhash.Hash{0x01, 0x02, 0x03}
	msg := NewMsgCFCheckpt(GCSFilterRegular, &stopHash, headerCount)
	for i := 0; i < headerCount; i++ {
		header := chainhash.Hash{byte(i), byte(i >> 8), 0x04}
		if err := msg.AddCFHeader(&header); err != nil {
			t.Fatalf("unable to add filter header: %v", err)
		}
	}

	var encoded bytes.Buffer
	if err := msg.BtcEncode(
		&encoded, ProtocolVersion, BaseEncoding,
	); err != nil {
		t.Fatalf("unable to encode checkpoint: %v", err)
	}

	// As above, the recorder hides the remaining length so the checkpoint
	// hash vector must grow as hashes arrive rather than being
	// preallocated from the claimed count.
	reader := &readSizeRecorder{
		reader: bytes.NewReader(encoded.Bytes()),
	}
	var decoded MsgCFCheckpt
	if err := decoded.BtcDecode(
		reader, ProtocolVersion, BaseEncoding,
	); err != nil {
		t.Fatalf("unable to decode checkpoint: %v", err)
	}

	if len(decoded.FilterHeaders) != headerCount {
		t.Fatalf("decoded %d checkpoint headers, want %d",
			len(decoded.FilterHeaders), headerCount)
	}
	if cap := cap(decoded.FilterHeaders); cap > 2*headerCount {
		t.Fatalf("streaming decode reserved %d checkpoint slots, "+
			"want at most %d", cap, 2*headerCount)
	}

	var reencoded bytes.Buffer
	if err := decoded.BtcEncode(
		&reencoded, ProtocolVersion, BaseEncoding,
	); err != nil {
		t.Fatalf("unable to re-encode checkpoint: %v", err)
	}
	if !bytes.Equal(encoded.Bytes(), reencoded.Bytes()) {
		t.Fatal("streaming checkpoint decode changed the encoding")
	}
}
