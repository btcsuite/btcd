package wire

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// TestAddrV2Encode checks that encoding an addrv2 message produces the expected
// serialized output, ignoring certain addresses.
func TestAddrV2Encode(t *testing.T) {
	t.Parallel()

	pver := ProtocolVersion
	enc := BaseEncoding

	ts := time.Unix(0x00000001, 0)
	tsStr := "01000000"

	sf := SFNodeNetwork
	sfStr := "01"

	port := uint16(0x0001)
	portStr := "0001"

	// IPV4 host.
	ipv4HostStr := "00000001"
	ipv4Host := ipv4Addr{
		addr:  [ipv4Size]byte(mustHex(ipv4HostStr)),
		netID: ipv4,
	}

	// IPV6 host.
	ipv6HostStr := "20010DB8000000000000000000000001"
	ipv6Host := ipv6Addr{
		addr:  [ipv6Size]byte(mustHex(ipv6HostStr)),
		netID: ipv6,
	}

	// Torv2 host.
	torv2HostStr := "00000000000000000001"
	torv2Host := torv2Addr{
		addr:  [torv2Size]byte(mustHex(torv2HostStr)),
		netID: torv2,
	}

	// Torv3 host.
	torV3HostStr := "0000000000000000000000000000000000000000000000000000" +
		"000000000001"
	torv3Host := torv3Addr{
		addr:  [TorV3Size]byte(mustHex(torV3HostStr)),
		netID: torv3,
	}

	addresses0 := []*NetAddressV2{
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &ipv4Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &ipv6Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &torv2Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &torv3Host,
			Port:      port,
		},
	}

	msg0Encoded := mustHexAppend(
		"03", // addr count

		tsStr,
		sfStr,
		"01", // ipv4 NetId
		"04", // addr length
		ipv4HostStr,
		portStr,

		tsStr,
		sfStr,
		"02", // ipv6 NetId
		"10", // addr length
		ipv6HostStr,
		portStr,

		// torv2 is skipped as required by BIP-0155.

		tsStr,
		sfStr,
		"04", // torv3 NetId
		"20", // addr length
		torV3HostStr,
		portStr,
	)

	tests := []struct {
		name string
		msg  *MsgAddrV2
		want []byte
	}{
		{
			"all supported addresses with torv2 not serialized",
			&MsgAddrV2{AddrList: addresses0},
			msg0Encoded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := test.msg.BtcEncode(&buf, pver, enc)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !bytes.Equal(test.want, buf.Bytes()) {
				t.Fatalf("unexpected encoding; \nwant: %x\n "+
					"got: %x", test.want, buf.Bytes())
			}
		})
	}
}

// TestAddrV2Decode checks that decoding an addrv2 message off the wire behaves
// as expected, ignoring certain addresses.
func TestAddrV2Decode(t *testing.T) {
	t.Parallel()

	pver := ProtocolVersion
	enc := BaseEncoding

	ts := time.Unix(0x00000001, 0)
	tsStr := "01000000"

	sf := SFNodeNetwork
	sfStr := "01"

	port := uint16(0x0001)
	portStr := "0001"

	// IPV4 host.
	ipv4HostStr := "00000001"
	ipv4Host := ipv4Addr{
		addr:  [ipv4Size]byte(mustHex(ipv4HostStr)),
		netID: ipv4,
	}

	// IPV6 mapped IPV4 host.
	ipv6v4HostStr := "00000000000000000000FFFF" + "00000002"

	// IPV6 host.
	ipv6HostStr := "20010DB8000000000000000000000001"
	ipv6Host := ipv6Addr{
		addr:  [ipv6Size]byte(mustHex(ipv6HostStr)),
		netID: ipv6,
	}

	// Torv2 host.
	torv2HostStr := "00000000000000000001"

	// IPV6 OnionCat encoded torv2 host.
	onionCatPrefix := "FD87D87EEB43"
	ipv6OnionCatTorv2String := onionCatPrefix + "00000000000000000002"

	// Torv3 host.
	torv3HostStr := "0000000000000000000000000000000000000000000000000000" +
		"000000000001"
	torv3Host := torv3Addr{
		addr:  [TorV3Size]byte(mustHex(torv3HostStr)),
		netID: torv3,
	}

	// i2p host.
	i2pHostStr := "000000000000000000000000000000000000000000000000000000" +
		"0000000001"

	// cjdns host.
	cjdnsHostStr := "00000000000000000000000000000001"

	// iggdrasil host.
	iggdrasilHostStr := "00000000000000000000000000000001"

	msg0Encoded := mustHexAppend(
		"09", // addr count

		// ipv4.
		tsStr,
		sfStr,
		"01", // ipv4 NetId
		"04", // addr length
		ipv4HostStr,
		portStr,

		// ipv6 v4 mapped.
		tsStr,
		sfStr,
		"02", // ipv6 NetId
		"10", // addr length
		ipv6v4HostStr,
		portStr,

		// ipv6.
		tsStr,
		sfStr,
		"02", // ipv6 NetId
		"10", // addr length
		ipv6HostStr,
		portStr,

		// ipv6 onioncat torv2.
		tsStr,
		sfStr,
		"02", // ipv6 NetId
		"10", // addr length
		ipv6OnionCatTorv2String,
		portStr,

		// torv2.
		tsStr,
		sfStr,
		"03", // torv2 NetId
		"0A", // addr length
		torv2HostStr,
		portStr,

		// torv3.
		tsStr,
		sfStr,
		"04", // torv3 NetId
		"20", // addr length
		torv3HostStr,
		portStr,

		// i2p.
		tsStr,
		sfStr,
		"05", // i2p NetId
		"20", // addr length
		i2pHostStr,
		portStr,

		// cjdns.
		tsStr,
		sfStr,
		"06", // cjdns NetId
		"10", // addr length
		cjdnsHostStr,
		portStr,

		// iggdrasil.
		tsStr,
		sfStr,
		"07", // iggdrasil NetId
		"10", // addr length
		iggdrasilHostStr,
		portStr,
	)

	addresses0 := []*NetAddressV2{
		// ipv4.
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &ipv4Host,
			Port:      port,
		},

		// ipv6 v4 mapped must be skipped.

		// ipv6.
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &ipv6Host,
			Port:      port,
		},

		// ipv6 onioncat torv2: must be skipped (BIP-0155).

		// torv2: must be skipped (BIP-0155).

		// torv3.
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &torv3Host,
			Port:      port,
		},

		// i2p: should be skipped.

		// cjdns: should be skipped.

		// iggdrasil: should be skipped.
	}

	msg1Encoded := mustHexAppend(
		"03", // addr count

		// torv2 with wrong address size is skipped.
		tsStr,
		sfStr,
		"03", // torv2 NetId
		"01", // addr length
		"00",
		portStr,

		// iggdrasil with wrong address size is skipped.
		tsStr,
		sfStr,
		"07", // iggdrasil NetId
		"01", // addr length
		"00",
		portStr,

		// ipv4.
		tsStr,
		sfStr,
		"01", // ipv4 NetId
		"04", // addr length
		ipv4HostStr,
		portStr,
	)

	addresses1 := []*NetAddressV2{
		{
			Timestamp: ts,
			Services:  sf,
			Addr:      &ipv4Host,
			Port:      port,
		},
	}

	tests := []struct {
		name string
		msg  []byte
		want *MsgAddrV2
	}{
		{
			"all networks with torv2, cjdns, i2p and iggdrasil " +
				"skipped",
			msg0Encoded,
			&MsgAddrV2{AddrList: addresses0},
		},
		{
			"skip torv2 and iggdrasil with wrong address size",
			msg1Encoded,
			&MsgAddrV2{AddrList: addresses1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var msg MsgAddrV2
			err := msg.BtcDecode(
				bytes.NewReader(test.msg), pver, enc,
			)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !reflect.DeepEqual(test.want, &msg) {
				t.Fatalf("unexpected message; \nwant:\n%s\n"+
					"got:\n%s", spew.Sdump(test.want),
					spew.Sdump(&msg))
			}
		})
	}
}

// TestAddrV2DecodeErrors perform negative tests for decoding addrv2 messages,
// asserting that some bad messages causes errors.
func TestAddrV2DecodeErrors(t *testing.T) {
	t.Parallel()

	pver := ProtocolVersion
	enc := BaseEncoding

	tests := []struct {
		name    string
		encoded []byte
	}{
		{
			"exceeding max addresses",
			mustHex("FDE903"), // 1001 in little endian compactSize
		},
		{
			"invalid address size",
			mustHexAppend(
				"01",
				"00000000",
				"00",
				"01", // ipv4
				"05", // ipv4 length is 4, not 5 bytes
			),
		},
		{
			"truncated address",
			mustHexAppend(
				"01",
				"00000000",
				"00",
				"01", // ipv4
				"04",
				"7F0000", // Three byte ipv4 address with
				// missing port.
			),
		},
		{
			"missing port",
			mustHexAppend(
				"01",
				"00000000",
				"00",
				"01", // ipv4
				"04",
				"00000001",
				// port missing here
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var msg MsgAddrV2

			r := bytes.NewReader(test.encoded)
			err := msg.BtcDecode(r, pver, enc)
			if err == nil {
				t.Fatalf("expecting error, but got nil")
			}
		})
	}
}
