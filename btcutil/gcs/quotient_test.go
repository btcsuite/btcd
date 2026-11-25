package gcs_test

import "testing"

// TestQuotientIgnoresRemainder shows the remainder does not need to be
// subtracted before the shift.
//
// A right shift throws away the low p bits. The remainder is exactly those
// bits, so removing it first makes no difference. Dividing 1234 by 100 gives
// 12 either way, and a shift works the same:
//
//	gap = 0x1234, remainder = 0x34
//	(gap - remainder) >> 8 = 0x12
//	gap               >> 8 = 0x12
func TestQuotientIgnoresRemainder(t *testing.T) {
	for p := uint8(0); p <= 32; p++ {
		lowBits := (uint64(1) << p) - 1
		for _, gap := range []uint64{
			0,
			1,
			0xff,
			0x100,
			0x1ff,
			0x1234,
			0xdeadbeef,
			1 << 40,
			^uint64(0),
		} {
			remainder := gap & lowBits
			withRemainder := (gap - remainder) >> p
			withoutRemainder := gap >> p
			if withRemainder != withoutRemainder {
				t.Fatalf("p=%d gap=%#x: (gap-remainder)>>p = %#x, gap>>p = %#x",
					p, gap, withRemainder, withoutRemainder)
			}
		}
	}
}
