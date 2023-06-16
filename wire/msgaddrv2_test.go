package wire

import (
	"bytes"
	"io"
	"testing"
)

// TestAddrV2Decode checks that decoding an addrv2 message off the wire behaves
// as expected. This means ignoring certain addresses, and failing in certain
// failure scenarios.
func TestAddrV2Decode(t *testing.T) {
	tests := []struct {
		buf           []byte
		expectedError bool
		expectedAddrs int
	}{
		// Exceeding max addresses.
		{
			[]byte{0xfd, 0xff, 0xff},
			true,
			0,
		},

		// Invalid address size.
		{
			[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x05},
			true,
			0,
		},

		// One valid address and one skipped address
		{
			[]byte{
				0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x04,
				0x7f, 0x00, 0x00, 0x01, 0x22, 0x22, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x02, 0x10, 0xfd, 0x87, 0xd8,
				0x7e, 0xeb, 0x43, 0xff, 0xfe, 0xcc, 0x39, 0xa8,
				0x73, 0x69, 0x15, 0xff, 0xff, 0x22, 0x22,
			},
			false,
			1,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		r := bytes.NewReader(test.buf)
		m := &MsgAddrV2{}

		err := m.BtcDecode(r, 0, LatestEncoding)
		if test.expectedError {
			if err == nil {
				t.Errorf("Test #%d expected error", i)
			}

			continue
		} else if err != nil {
			t.Errorf("Test #%d unexpected error %v", i, err)
		}

		// Trying to read more should give EOF.
		var b [1]byte
		if _, err := r.Read(b[:]); err != io.EOF {
			t.Errorf("Test #%d did not cleanly finish reading", i)
		}

		if len(m.AddrList) != test.expectedAddrs {
			t.Errorf("Test #%d expected %d addrs, instead of %d",
				i, test.expectedAddrs, len(m.AddrList))
		}
	}
}
