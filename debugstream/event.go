package debugstream

import (
	"encoding/binary"
	"fmt"
	"io"
)

// The following are guard event codes, that can be used to assert debug events
// in tests.
const (
	DEStart = iota + 1
	DEShutdown
)

// Event is the message that is sent from the Stream to the Client.
type Event struct {
	Code uint64
	Data []byte
}

func (e *Event) write(w io.Writer) error {
	err := binary.Write(w, binary.BigEndian, e.Code)
	if err != nil {
		return err
	}
	dataLen := uint64(len(e.Data))
	err = binary.Write(w, binary.BigEndian, dataLen)
	if err != nil {
		return err
	}
	n, err := w.Write(e.Data)
	if err != nil {
		return err
	}
	if n != int(dataLen) {
		return fmt.Errorf("nWrite != dataLen")
	}
	return nil
}

func (e *Event) read(r io.Reader) error {
	err := binary.Read(r, binary.BigEndian, &e.Code)
	if err != nil {
		return err
	}
	var dataLen uint64
	err = binary.Read(r, binary.BigEndian, &dataLen)
	if err != nil {
		return err
	}
	e.Data = make([]byte, dataLen)
	n, err := io.ReadFull(r, e.Data)
	if err != nil {
		return err
	}
	if n != int(dataLen) {
		return fmt.Errorf("nRead != dataLen")
	}
	return nil
}
