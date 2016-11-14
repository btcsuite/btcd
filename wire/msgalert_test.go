// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestMsgAlert tests the MsgAlert API.
func TestMsgAlert(t *testing.T) {
	pver := ProtocolVersion
	serializedpayload := []byte("some message")
	signature := []byte("some sig")

	// Ensure we get the same payload and signature back out.
	msg := NewMsgAlert(serializedpayload, signature)
	if !reflect.DeepEqual(msg.SerializedPayload, serializedpayload) {
		t.Errorf("NewMsgAlert: wrong serializedpayload - got %v, want %v",
			msg.SerializedPayload, serializedpayload)
	}
	if !reflect.DeepEqual(msg.Signature, signature) {
		t.Errorf("NewMsgAlert: wrong signature - got %v, want %v",
			msg.Signature, signature)
	}

	// Ensure the command is expected value.
	wantCmd := "alert"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgAlert: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
	wantPayload := uint32(1024 * 1024 * 32)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Test BtcEncode with Payload == nil
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Error(err.Error())
	}
	// expected = 0x0c + serializedpayload + 0x08 + signature
	expectedBuf := append([]byte{0x0c}, serializedpayload...)
	expectedBuf = append(expectedBuf, []byte{0x08}...)
	expectedBuf = append(expectedBuf, signature...)
	if !bytes.Equal(buf.Bytes(), expectedBuf) {
		t.Errorf("BtcEncode got: %s want: %s",
			spew.Sdump(buf.Bytes()), spew.Sdump(expectedBuf))
	}

	// Test BtcEncode with Payload != nil
	// note: Payload is an empty Alert but not nil
	msg.Payload = new(Alert)
	buf = *new(bytes.Buffer)
	err = msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Error(err.Error())
	}
	// empty Alert is 45 null bytes, see Alert comments
	// for details
	// expected = 0x2d + 45*0x00 + 0x08 + signature
	expectedBuf = append([]byte{0x2d}, bytes.Repeat([]byte{0x00}, 45)...)
	expectedBuf = append(expectedBuf, []byte{0x08}...)
	expectedBuf = append(expectedBuf, signature...)
	if !bytes.Equal(buf.Bytes(), expectedBuf) {
		t.Errorf("BtcEncode got: %s want: %s",
			spew.Sdump(buf.Bytes()), spew.Sdump(expectedBuf))
	}
}

// TestMsgAlertWire tests the MsgAlert wire encode and decode for various protocol
// versions.
func TestMsgAlertWire(t *testing.T) {
	baseMsgAlert := NewMsgAlert([]byte("some payload"), []byte("somesig"))
	baseMsgAlertEncoded := []byte{
		0x0c, // Varint for payload length
		0x73, 0x6f, 0x6d, 0x65, 0x20, 0x70, 0x61, 0x79,
		0x6c, 0x6f, 0x61, 0x64, // "some payload"
		0x07,                                     // Varint for signature length
		0x73, 0x6f, 0x6d, 0x65, 0x73, 0x69, 0x67, // "somesig"
	}

	tests := []struct {
		in   *MsgAlert // Message to encode
		out  *MsgAlert // Expected decoded message
		buf  []byte    // Wire encoding
		pver uint32    // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			baseMsgAlert,
			baseMsgAlert,
			baseMsgAlertEncoded,
			ProtocolVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver)
		if err != nil {
			t.Errorf("BtcEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var msg MsgAlert
		rbuf := bytes.NewReader(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestMsgAlertWireErrors performs negative tests against wire encode and decode
// of MsgAlert to confirm error paths work correctly.
func TestMsgAlertWireErrors(t *testing.T) {
	pver := ProtocolVersion

	baseMsgAlert := NewMsgAlert([]byte("some payload"), []byte("somesig"))
	baseMsgAlertEncoded := []byte{
		0x0c, // Varint for payload length
		0x73, 0x6f, 0x6d, 0x65, 0x20, 0x70, 0x61, 0x79,
		0x6c, 0x6f, 0x61, 0x64, // "some payload"
		0x07,                                     // Varint for signature length
		0x73, 0x6f, 0x6d, 0x65, 0x73, 0x69, 0x67, // "somesig"
	}

	tests := []struct {
		in       *MsgAlert // Value to encode
		buf      []byte    // Wire encoding
		pver     uint32    // Protocol version for wire encoding
		max      int       // Max size of fixed buffer to induce errors
		writeErr error     // Expected write error
		readErr  error     // Expected read error
	}{
		// Force error in payload length.
		{baseMsgAlert, baseMsgAlertEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in payload.
		{baseMsgAlert, baseMsgAlertEncoded, pver, 1, io.ErrShortWrite, io.EOF},
		// Force error in signature length.
		{baseMsgAlert, baseMsgAlertEncoded, pver, 13, io.ErrShortWrite, io.EOF},
		// Force error in signature.
		{baseMsgAlert, baseMsgAlertEncoded, pver, 14, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.writeErr {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg MsgAlert
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.readErr {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v", i, err, test.readErr)
				continue
			}
		}
	}

	// Test Error on empty Payload
	baseMsgAlert.SerializedPayload = []byte{}
	w := new(bytes.Buffer)
	err := baseMsgAlert.BtcEncode(w, pver)
	if _, ok := err.(*MessageError); !ok {
		t.Errorf("MsgAlert.BtcEncode wrong error got: %T, want: %T",
			err, MessageError{})
	}

	// Test Payload Serialize error
	// overflow the max number of elements in SetCancel
	baseMsgAlert.Payload = new(Alert)
	baseMsgAlert.Payload.SetCancel = make([]int32, maxCountSetCancel+1)
	buf := *new(bytes.Buffer)
	err = baseMsgAlert.BtcEncode(&buf, pver)
	if _, ok := err.(*MessageError); !ok {
		t.Errorf("MsgAlert.BtcEncode wrong error got: %T, want: %T",
			err, MessageError{})
	}

	// overflow the max number of elements in SetSubVer
	baseMsgAlert.Payload = new(Alert)
	baseMsgAlert.Payload.SetSubVer = make([]string, maxCountSetSubVer+1)
	buf = *new(bytes.Buffer)
	err = baseMsgAlert.BtcEncode(&buf, pver)
	if _, ok := err.(*MessageError); !ok {
		t.Errorf("MsgAlert.BtcEncode wrong error got: %T, want: %T",
			err, MessageError{})
	}
}

// TestAlert tests serialization and deserialization
// of the payload to Alert
func TestAlert(t *testing.T) {
	pver := ProtocolVersion
	alert := NewAlert(
		1, 1337093712, 1368628812, 1015,
		1013, []int32{1014}, 0, 40599, []string{"/Satoshi:0.7.2/"}, 5000, "",
		"URGENT: upgrade required, see http://bitcoin.org/dos for details",
	)
	w := new(bytes.Buffer)
	err := alert.Serialize(w, pver)
	if err != nil {
		t.Error(err.Error())
	}
	serializedpayload := w.Bytes()
	newAlert, err := NewAlertFromPayload(serializedpayload, pver)
	if err != nil {
		t.Error(err.Error())
	}

	if alert.Version != newAlert.Version {
		t.Errorf("NewAlertFromPayload: wrong Version - got %v, want %v ",
			alert.Version, newAlert.Version)
	}
	if alert.RelayUntil != newAlert.RelayUntil {
		t.Errorf("NewAlertFromPayload: wrong RelayUntil - got %v, want %v ",
			alert.RelayUntil, newAlert.RelayUntil)
	}
	if alert.Expiration != newAlert.Expiration {
		t.Errorf("NewAlertFromPayload: wrong Expiration - got %v, want %v ",
			alert.Expiration, newAlert.Expiration)
	}
	if alert.ID != newAlert.ID {
		t.Errorf("NewAlertFromPayload: wrong ID - got %v, want %v ",
			alert.ID, newAlert.ID)
	}
	if alert.Cancel != newAlert.Cancel {
		t.Errorf("NewAlertFromPayload: wrong Cancel - got %v, want %v ",
			alert.Cancel, newAlert.Cancel)
	}
	if len(alert.SetCancel) != len(newAlert.SetCancel) {
		t.Errorf("NewAlertFromPayload: wrong number of SetCancel - got %v, want %v ",
			len(alert.SetCancel), len(newAlert.SetCancel))
	}
	for i := 0; i < len(alert.SetCancel); i++ {
		if alert.SetCancel[i] != newAlert.SetCancel[i] {
			t.Errorf("NewAlertFromPayload: wrong SetCancel[%v] - got %v, want %v ",
				len(alert.SetCancel), alert.SetCancel[i], newAlert.SetCancel[i])
		}
	}
	if alert.MinVer != newAlert.MinVer {
		t.Errorf("NewAlertFromPayload: wrong MinVer - got %v, want %v ",
			alert.MinVer, newAlert.MinVer)
	}
	if alert.MaxVer != newAlert.MaxVer {
		t.Errorf("NewAlertFromPayload: wrong MaxVer - got %v, want %v ",
			alert.MaxVer, newAlert.MaxVer)
	}
	if len(alert.SetSubVer) != len(newAlert.SetSubVer) {
		t.Errorf("NewAlertFromPayload: wrong number of SetSubVer - got %v, want %v ",
			len(alert.SetSubVer), len(newAlert.SetSubVer))
	}
	for i := 0; i < len(alert.SetSubVer); i++ {
		if alert.SetSubVer[i] != newAlert.SetSubVer[i] {
			t.Errorf("NewAlertFromPayload: wrong SetSubVer[%v] - got %v, want %v ",
				len(alert.SetSubVer), alert.SetSubVer[i], newAlert.SetSubVer[i])
		}
	}
	if alert.Priority != newAlert.Priority {
		t.Errorf("NewAlertFromPayload: wrong Priority - got %v, want %v ",
			alert.Priority, newAlert.Priority)
	}
	if alert.Comment != newAlert.Comment {
		t.Errorf("NewAlertFromPayload: wrong Comment - got %v, want %v ",
			alert.Comment, newAlert.Comment)
	}
	if alert.StatusBar != newAlert.StatusBar {
		t.Errorf("NewAlertFromPayload: wrong StatusBar - got %v, want %v ",
			alert.StatusBar, newAlert.StatusBar)
	}
	if alert.Reserved != newAlert.Reserved {
		t.Errorf("NewAlertFromPayload: wrong Reserved - got %v, want %v ",
			alert.Reserved, newAlert.Reserved)
	}
}

// TestAlertErrors performs negative tests against payload serialization,
// deserialization of Alert to confirm error paths work correctly.
func TestAlertErrors(t *testing.T) {
	pver := ProtocolVersion

	baseAlert := NewAlert(
		1, 1337093712, 1368628812, 1015,
		1013, []int32{1014}, 0, 40599, []string{"/Satoshi:0.7.2/"}, 5000, "",
		"URGENT",
	)
	baseAlertEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, 0x50, 0x6e, 0xb2, 0x4f, 0x00, 0x00, 0x00, 0x00, 0x4c, 0x9e, 0x93, 0x51, //|....Pn.O....L..Q|
		0x00, 0x00, 0x00, 0x00, 0xf7, 0x03, 0x00, 0x00, 0xf5, 0x03, 0x00, 0x00, 0x01, 0xf6, 0x03, 0x00, //|................|
		0x00, 0x00, 0x00, 0x00, 0x00, 0x97, 0x9e, 0x00, 0x00, 0x01, 0x0f, 0x2f, 0x53, 0x61, 0x74, 0x6f, //|.........../Sato|
		0x73, 0x68, 0x69, 0x3a, 0x30, 0x2e, 0x37, 0x2e, 0x32, 0x2f, 0x88, 0x13, 0x00, 0x00, 0x00, 0x06, //|shi:0.7.2/......|
		0x55, 0x52, 0x47, 0x45, 0x4e, 0x54, 0x00, //|URGENT.|
	}
	tests := []struct {
		in       *Alert // Value to encode
		buf      []byte // Wire encoding
		pver     uint32 // Protocol version for wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Force error in Version
		{baseAlert, baseAlertEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in SetCancel VarInt.
		{baseAlert, baseAlertEncoded, pver, 28, io.ErrShortWrite, io.EOF},
		// Force error in SetCancel ints.
		{baseAlert, baseAlertEncoded, pver, 29, io.ErrShortWrite, io.EOF},
		// Force error in MinVer
		{baseAlert, baseAlertEncoded, pver, 40, io.ErrShortWrite, io.EOF},
		// Force error in SetSubVer string VarInt.
		{baseAlert, baseAlertEncoded, pver, 41, io.ErrShortWrite, io.EOF},
		// Force error in SetSubVer strings.
		{baseAlert, baseAlertEncoded, pver, 48, io.ErrShortWrite, io.EOF},
		// Force error in Priority
		{baseAlert, baseAlertEncoded, pver, 60, io.ErrShortWrite, io.EOF},
		// Force error in Comment string.
		{baseAlert, baseAlertEncoded, pver, 62, io.ErrShortWrite, io.EOF},
		// Force error in StatusBar string.
		{baseAlert, baseAlertEncoded, pver, 64, io.ErrShortWrite, io.EOF},
		// Force error in Reserved string.
		{baseAlert, baseAlertEncoded, pver, 70, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		w := newFixedWriter(test.max)
		err := test.in.Serialize(w, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("Alert.Serialize #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		var alert Alert
		r := newFixedReader(test.max, test.buf)
		err = alert.Deserialize(r, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("Alert.Deserialize #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}

	// overflow the max number of elements in SetCancel
	// maxCountSetCancel + 1 == 8388575 == \xdf\xff\x7f\x00
	// replace bytes 29-33
	badAlertEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, 0x50, 0x6e, 0xb2, 0x4f, 0x00, 0x00, 0x00, 0x00, 0x4c, 0x9e, 0x93, 0x51, //|....Pn.O....L..Q|
		0x00, 0x00, 0x00, 0x00, 0xf7, 0x03, 0x00, 0x00, 0xf5, 0x03, 0x00, 0x00, 0xfe, 0xdf, 0xff, 0x7f, //|................|
		0x00, 0x00, 0x00, 0x00, 0x00, 0x97, 0x9e, 0x00, 0x00, 0x01, 0x0f, 0x2f, 0x53, 0x61, 0x74, 0x6f, //|.........../Sato|
		0x73, 0x68, 0x69, 0x3a, 0x30, 0x2e, 0x37, 0x2e, 0x32, 0x2f, 0x88, 0x13, 0x00, 0x00, 0x00, 0x06, //|shi:0.7.2/......|
		0x55, 0x52, 0x47, 0x45, 0x4e, 0x54, 0x00, //|URGENT.|
	}
	var alert Alert
	r := bytes.NewReader(badAlertEncoded)
	err := alert.Deserialize(r, pver)
	if _, ok := err.(*MessageError); !ok {
		t.Errorf("Alert.Deserialize wrong error got: %T, want: %T",
			err, MessageError{})
	}

	// overflow the max number of elements in SetSubVer
	// maxCountSetSubVer + 1 == 131071 + 1 == \x00\x00\x02\x00
	// replace bytes 42-46
	badAlertEncoded = []byte{
		0x01, 0x00, 0x00, 0x00, 0x50, 0x6e, 0xb2, 0x4f, 0x00, 0x00, 0x00, 0x00, 0x4c, 0x9e, 0x93, 0x51, //|....Pn.O....L..Q|
		0x00, 0x00, 0x00, 0x00, 0xf7, 0x03, 0x00, 0x00, 0xf5, 0x03, 0x00, 0x00, 0x01, 0xf6, 0x03, 0x00, //|................|
		0x00, 0x00, 0x00, 0x00, 0x00, 0x97, 0x9e, 0x00, 0x00, 0xfe, 0x00, 0x00, 0x02, 0x00, 0x74, 0x6f, //|.........../Sato|
		0x73, 0x68, 0x69, 0x3a, 0x30, 0x2e, 0x37, 0x2e, 0x32, 0x2f, 0x88, 0x13, 0x00, 0x00, 0x00, 0x06, //|shi:0.7.2/......|
		0x55, 0x52, 0x47, 0x45, 0x4e, 0x54, 0x00, //|URGENT.|
	}
	r = bytes.NewReader(badAlertEncoded)
	err = alert.Deserialize(r, pver)
	if _, ok := err.(*MessageError); !ok {
		t.Errorf("Alert.Deserialize wrong error got: %T, want: %T",
			err, MessageError{})
	}
}
