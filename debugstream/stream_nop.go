package debugstream

// StreamNOP is a NOP stream, can be used in production because all its methods
// are also NOP and therefore the compiler can optimize them out.
type StreamNOP struct {
}

func NewStreamNOP() *StreamNOP {
	return &StreamNOP{}
}

func (s *StreamNOP) Broadcast(_ Event) {
}

func (s *StreamNOP) Listen(_ string) error {
	return nil
}

func (s *StreamNOP) Shutdown() {
}
