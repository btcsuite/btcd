//go:build !debug

package debugstream

// Stream is a [StreamNOP] when debug flag is not set, effectively doing nothing
// because all its procedures are also NOP.
type Stream = StreamNOP

func New() *Stream {
	return NewStreamNOP()
}
