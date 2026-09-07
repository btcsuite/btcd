// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"testing"
)

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
}

type readSizeRecorder struct {
	reader io.Reader
	max    int
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

func (r *readSizeRecorder) Read(p []byte) (int, error) {
	if len(p) > r.max {
		r.max = len(p)
	}

	return r.reader.Read(p)
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
