//go:build debug

package debugstream

// Stream is the real implementation of the debug stream, [StreamServer] when
// compiling with debug tag.
type Stream = StreamServer

func New() *Stream {
	return NewStreamServer()
}
