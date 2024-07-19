package v2transport

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ellswift"
	"github.com/btcsuite/btcd/wire"
)

type packetBit uint8

const (
	// ignoreBitPos is the position of the ignore bit in the packet's
	// header.
	ignoreBitPos packetBit = 7
)

const (
	// garbageSize is the length in bytes of the garbage terminator that
	// each party sends.
	garbageSize = 16

	// maxGarbageLen is the maximum size of garbage that either peer is allowed
	// to send.
	maxGarbageLen = 4095

	// maxContentLen is the maximum length of content that can be encrypted or
	// decrypted.
	// TODO: This should be revisited. For some reason, the test vectors want
	// us to encrypt 16777215 bytes even though bitcoind will only decrypt up
	// to 1 + 12 + 4_000_000 bytes by default.
	maxContentLen = 1<<24 - 1

	// lengthFieldLen is the length of the length field when encrypting the
	// content's length.
	lengthFieldLen = 3

	// headerLen is the length of the header field. It is composed of a
	// single byte with only the ignoreBitPos having any meaning.
	headerLen = 1

	// chachapoly1305Expansion is the difference in bytes between the
	// plaintext and ciphertext when using chachapoly1305. The ciphertext
	// is larger because of the authentication tag.
	chachapoly1305Expansion = 16
)

var (
	// transportVersion is the transport version we are currently using.
	transportVersion = []byte{}

	// errInsufficientBytes is returned when we haven't received enough
	// bytes to populate their ElligatorSwift encoded public key.
	errInsufficientBytes = fmt.Errorf("insufficient bytes received")

	// ErrUseV1Protocol is returned when the initiating peer is attempting
	// to use the V1 protocol.
	ErrUseV1Protocol = fmt.Errorf("use v1 protocol instead")

	// errWrongNetV1Peer is returned when a v1 peer is using the wrong
	// network.
	errWrongNetV1Peer = fmt.Errorf("peer is v1 and using the wrong network")

	// errGarbageTermNotRecv is returned when a v2 peer never sends us their
	// garbage terminator.
	errGarbageTermNotRecv = fmt.Errorf("no garbage term received")

	// errContentLengthExceeded is returned when trying to encrypt or decrypt
	// more than the maximum content length.
	errContentLengthExceeded = fmt.Errorf("maximum content length exceeded")

	// errFailedToRecv is returned when a Read call fails.
	errFailedToRecv = fmt.Errorf("failed to recv data")

	// errPrefixTooLarge is returned if receivedPrefix is ever too large.
	// This shouldn't happen unless the API is mis-used.
	errPrefixTooLarge = fmt.Errorf("prefix too large - internal error")

	// errGarbageTooLarge is returned if a caller attempts to send garbage
	// larger than normal.
	errGarbageTooLarge = fmt.Errorf("garbage too large")
)

// Peer defines the components necessary for sending/receiving data over the v2
// transport.
type Peer struct {
	// privkeyOurs is our private key
	privkeyOurs *btcec.PrivateKey

	// ellswiftOurs is our ElligatorSwift-encoded public key.
	ellswiftOurs [64]byte

	// sentGarbage is the garbage sent after the public key. This may be up
	// to
	// 4095 bytes.
	sentGarbage []byte

	// receivedPrefix is used to determine which transport protocol we're
	// using.
	receivedPrefix []byte

	// sendL is the cipher used to send encrypted packet lengths.
	sendL *FSChaCha20

	// sendP is the cipher used to send encrypted packets.
	sendP *FSChaCha20Poly1305

	// sendGarbageTerm is the garbage terminator that we send.
	sendGarbageTerm [garbageSize]byte

	// recvL is the cipher used to receive encrypted packet lengths.
	recvL *FSChaCha20

	// recvP is the cipher used to receive encrypted packets.
	recvP *FSChaCha20Poly1305

	// recvGarbageTerm is the garbage terminator our peer sends.
	recvGarbageTerm []byte

	// initiatorL is the key used to seed the sendL cipher.
	initiatorL []byte

	// initiatorP is the key used to seed the sendP cipher.
	initiatorP []byte

	// responderL is the key used to seed the recvL cipher.
	responderL []byte

	// responderP is the key used to seed the recvP cipher.
	responderP []byte

	// sessionID uniquely identifies this encrypted channel. It is
	// currently only used in the test vectors.
	sessionID []byte

	// rw is the underlying object that will be read from / written to in
	// calls to V2EncPacket and V2ReceivePacket.
	rw io.ReadWriter
}

// NewPeer returns a new instance of Peer.
func NewPeer() *Peer {
	// The keys (initiatorL, initiatorP, responderL, responderP) as well as
	// the sessionID must have space for the hkdf Expand-derived Reader to
	// work.
	return &Peer{
		receivedPrefix: make([]byte, 0),
		initiatorL:     make([]byte, 32),
		initiatorP:     make([]byte, 32),
		responderL:     make([]byte, 32),
		responderP:     make([]byte, 32),
		sessionID:      make([]byte, 32),
	}
}

// createV2Ciphers constructs the packet-length and packet encryption ciphers.
func (p *Peer) createV2Ciphers(ecdhSecret []byte, initiating bool,
	net wire.BitcoinNet) error {

	// Define the salt as the string "bitcoin_v2_shared_secret" followed by
	// the BitcoinNet's "magic" bytes.
	salt := []byte("bitcoin_v2_shared_secret")

	var magic [4]byte
	binary.LittleEndian.PutUint32(magic[:], uint32(net))
	salt = append(salt, magic[:]...)

	// Use the hkdf Extract function to generate a pseudo-random key.
	prk := hkdf.Extract(sha256.New, ecdhSecret, salt)

	// Use the hkdf Expand function with info set to "session_id" to generate a
	// unique sessionID.
	sessionInfo := []byte("session_id")
	sessionReader := hkdf.Expand(sha256.New, prk, sessionInfo)
	_, err := sessionReader.Read(p.sessionID)
	if err != nil {
		return err
	}

	// Use the Expand operation to generate packet and packet-length encryption
	// ciphers.
	initiatorLInfo := []byte("initiator_L")
	initiatorLReader := hkdf.Expand(sha256.New, prk, initiatorLInfo)
	_, err = initiatorLReader.Read(p.initiatorL)
	if err != nil {
		return err
	}

	initiatorPInfo := []byte("initiator_P")
	initiatorPReader := hkdf.Expand(sha256.New, prk, initiatorPInfo)
	_, err = initiatorPReader.Read(p.initiatorP)
	if err != nil {
		return err
	}

	responderLInfo := []byte("responder_L")
	responderLReader := hkdf.Expand(sha256.New, prk, responderLInfo)
	_, err = responderLReader.Read(p.responderL)
	if err != nil {
		return err
	}

	responderPInfo := []byte("responder_P")
	responderPReader := hkdf.Expand(sha256.New, prk, responderPInfo)
	_, err = responderPReader.Read(p.responderP)
	if err != nil {
		return err
	}

	// Create the garbage terminators that each side will use.
	garbageInfo := []byte("garbage_terminators")
	garbageReader := hkdf.Expand(sha256.New, prk, garbageInfo)
	garbageTerminators := make([]byte, 32)
	_, err = garbageReader.Read(garbageTerminators)
	if err != nil {
		return err
	}

	initiatorGarbageTerminator := garbageTerminators[:garbageSize]
	responderGarbageTerminator := garbageTerminators[garbageSize:]

	if initiating {
		p.sendL, err = NewFSChaCha20(p.initiatorL)
		if err != nil {
			return err
		}

		p.sendP, err = NewFSChaCha20Poly1305(p.initiatorP)
		if err != nil {
			return err
		}

		copy(p.sendGarbageTerm[:], initiatorGarbageTerminator)

		p.recvL, err = NewFSChaCha20(p.responderL)
		if err != nil {
			return err
		}

		p.recvP, err = NewFSChaCha20Poly1305(p.responderP)
		if err != nil {
			return err
		}

		p.recvGarbageTerm = responderGarbageTerminator
	} else {
		p.sendL, err = NewFSChaCha20(p.responderL)
		if err != nil {
			return err
		}

		p.sendP, err = NewFSChaCha20Poly1305(p.responderP)
		if err != nil {
			return err
		}

		copy(p.sendGarbageTerm[:], responderGarbageTerminator)

		p.recvL, err = NewFSChaCha20(p.initiatorL)
		if err != nil {
			return err
		}

		p.recvP, err = NewFSChaCha20Poly1305(p.initiatorP)
		if err != nil {
			return err
		}

		p.recvGarbageTerm = initiatorGarbageTerminator
	}

	// TODO:
	//     To achieve forward secrecy we must wipe the key material used to initialize the ciphers:
	//     memory_cleanse(ecdhSecret, prk, initiator_L, initiator_P, responder_L, responder_K)
	// - golang analogue?

	return nil
}

// InitiateV2Handshake generates our private key and sends our public key as
// well as garbage data to our peer.
func (p *Peer) InitiateV2Handshake(garbageLen int) error {
	var err error
	p.privkeyOurs, p.ellswiftOurs, err = ellswift.EllswiftCreate()
	if err != nil {
		return err
	}

	data, err := p.generateKeyAndGarbage(garbageLen)
	if err != nil {
		return err
	}

	p.Send(data)

	return nil
}

// RespondV2Handshake responds to the initiator, determines if the initiator
// wants to use the v2 protocol and if so returns our ElligatorSwift-encoded
// public key followed by our garbage data over. If the initiator does not want
// to use the v2 protocol, we'll instead revert to the v1 protocol.
func (p *Peer) RespondV2Handshake(garbageLen int, net wire.BitcoinNet) error {
	v1Prefix := createV1Prefix(net)

	var err error

	// Check and see if the received bytes match the v1 protocol's message
	// prefix. If it does, we'll revert to the v1 protocol. If it doesn't,
	// we'll treat this as a v2 peer.
	for len(p.receivedPrefix) < len(v1Prefix) {
		var receiveBytes []byte
		receiveBytes, err = p.Receive(1)
		if err != nil {
			return err
		}

		p.receivedPrefix = append(p.receivedPrefix, receiveBytes...)
		lastIdx := len(p.receivedPrefix) - 1
		if p.receivedPrefix[lastIdx] != v1Prefix[lastIdx] {
			p.privkeyOurs, p.ellswiftOurs, err = ellswift.EllswiftCreate()
			if err != nil {
				return err
			}

			data, err := p.generateKeyAndGarbage(garbageLen)
			if err != nil {
				return err
			}

			// Send over our ElligatorSwift-encoded pubkey followed by our
			// randomly generated garbage.
			p.Send(data)

			return nil
		}
	}

	return ErrUseV1Protocol
}

// generateKeyAndGarbage returns a byte slice containing our ellswift-encoded
// public key followed by the garbage we'll send over.
func (p *Peer) generateKeyAndGarbage(garbageLen int) ([]byte, error) {
	if garbageLen > maxGarbageLen {
		return nil, errGarbageTooLarge
	}

	p.sentGarbage = make([]byte, garbageLen)
	_, err := rand.Read(p.sentGarbage)
	if err != nil {
		return nil, err
	}

	data := make([]byte, 0, 64+garbageLen)
	data = append(data, p.ellswiftOurs[:]...)
	data = append(data, p.sentGarbage...)

	return data, nil
}

// createV1Prefix is a helper function that returns the first 16 bytes of the
// version message's header.
func createV1Prefix(net wire.BitcoinNet) []byte {
	v1Prefix := make([]byte, 0, 4+12)

	// The v1 transport protocol uses the network's 4 magic bytes followed by
	// "version" followed by 5 bytes of 0.
	var magic [4]byte
	binary.LittleEndian.PutUint32(magic[:], uint32(net))

	versionBytes := []byte("version\x00\x00\x00\x00\x00")

	v1Prefix = append(v1Prefix, magic[:]...)
	v1Prefix = append(v1Prefix, versionBytes...)

	return v1Prefix
}

// CompleteHandshake finishes the v2 protocol negotiation and optionally sends
// decoy packets after sending the garbage terminator.
func (p *Peer) CompleteHandshake(initiating bool, decoyContentLens []int,
	net wire.BitcoinNet) error {

	var receivedPrefix []byte
	if initiating {
		receivedPrefix = make([]byte, 0, 16)
	} else {
		// If we are the responder, we have already received bytes to
		// compare against the v1 transport protocol's starting bytes.
		// We have to account for these when reading the rest of the 64
		// bytes off the wire to properly parse the remote's
		// ellswift-encoded public key.
		receivedPrefix = p.receivedPrefix
	}

	recvData, err := p.Receive(64 - len(receivedPrefix))
	if err != nil {
		return err
	}

	var ellswiftTheirs [64]byte

	if initiating {
		// If we are initiating, read all 64 bytes into ellswiftTheirs.
		copy(ellswiftTheirs[:], recvData)
	} else {
		// If we are the responder, then we need to account for the bytes
		// already received as part of matching against the starting v1
		// transport bytes. We sanity check receivedPrefix in case it is too
		// large for some reason.
		prefixLen := len(receivedPrefix)
		if prefixLen > 16 {
			return errPrefixTooLarge
		}

		copy(ellswiftTheirs[:], receivedPrefix)
		copy(ellswiftTheirs[prefixLen:], recvData)
	}

	// Calculate the v1 protocol's message prefix and see if the bytes read
	// read into ellswiftTheirs matches it.
	v1Prefix := createV1Prefix(net)

	// ellswiftTheirs should be at least 16 bytes if receive succeeded, but
	// just in case, check the size.
	if len(ellswiftTheirs) < 16 {
		return errInsufficientBytes
	}

	if !initiating && bytes.Equal(ellswiftTheirs[4:16], v1Prefix[4:16]) {
		return errWrongNetV1Peer
	}

	// Calculate the shared secret to be used in creating the packet ciphers.
	ecdhSecret, err := ellswift.V2Ecdh(
		p.privkeyOurs, ellswiftTheirs, p.ellswiftOurs, initiating,
	)
	if err != nil {
		return err
	}

	err = p.createV2Ciphers(ecdhSecret[:], initiating, net)
	if err != nil {
		return err
	}

	// Send garbage terminator.
	p.Send(p.sendGarbageTerm[:])

	// Optionally send decoy packets after garbage terminator.
	aad := p.sentGarbage
	for i := 0; i < len(decoyContentLens); i++ {
		decoyContent := make([]byte, decoyContentLens[i])
		encPacket, _, err := p.V2EncPacket(decoyContent, aad, true)
		if err != nil {
			return err
		}

		p.Send(encPacket)

		// TODO: Does this cause anything to leak?
		aad = nil
	}

	// Send version packet.
	_, _, err = p.V2EncPacket(transportVersion, aad, false)
	if err != nil {
		return err
	}

	// Skip garbage until encountering garbage terminator.
	recvGarbage, err := p.Receive(16)
	if err != nil {
		return err
	}

	// The BIP text states that we can read up to 4111 bytes. We've already
	// read 16 bytes for the garbage terminator, so we need to read up to
	// maxGarbageLen more bytes.
	for i := 0; i < maxGarbageLen; i++ {
		recvGarbageLen := len(recvGarbage)

		if bytes.Equal(recvGarbage[recvGarbageLen-16:],
			p.recvGarbageTerm) {

			// Process any potential packet data sent before the
			// terminator.
			_, err = p.V2ReceivePacket(
				recvGarbage[:recvGarbageLen-16],
			)
			return err
		}

		recvData, err := p.Receive(1)
		if err != nil {
			return err
		}
		recvGarbage = append(recvGarbage, recvData...)
	}

	// Return an error if the garbage terminator was not received after 4
	// KiB.
	return errGarbageTermNotRecv
}

// V2EncPacket takes the contents and aad and returns a ciphertext.
func (p *Peer) V2EncPacket(contents []byte, aad []byte, ignore bool) ([]byte,
	int, error) {

	contentLen := len(contents)

	if contentLen > maxContentLen {
		return nil, 0, errContentLengthExceeded
	}

	// Construct the packet's header based on whether or not the peer
	// should ignore this packet (i.e. if this is a decoy packet).
	ignoreNum := 0
	if ignore {
		ignoreNum = 1
	}

	ignoreNum <<= ignoreBitPos

	header := []byte{byte(ignoreNum)}

	plaintext := make([]byte, 0, contentLen+1)
	plaintext = append(plaintext, header...)
	plaintext = append(plaintext, contents...)

	aeadCiphertext, err := p.sendP.Encrypt(aad, plaintext)
	if err != nil {
		return nil, 0, err
	}

	// We cut off a byte when feeding to Crypt.
	contentsLE := make([]byte, 4)
	binary.LittleEndian.PutUint32(contentsLE, uint32(contentLen))
	encContentsLen, err := p.sendL.Crypt(contentsLE[:lengthFieldLen])
	if err != nil {
		return nil, 0, err
	}

	encPacket := make([]byte, 0, len(encContentsLen)+len(aeadCiphertext))
	encPacket = append(encPacket, encContentsLen...)
	encPacket = append(encPacket, aeadCiphertext...)

	totalBytes, err := p.Send(encPacket)

	return encPacket, totalBytes, err
}

// V2ReceivePacket takes the aad and decrypts a received packet.
func (p *Peer) V2ReceivePacket(aad []byte) ([]byte, error) {
	for {
		// Decrypt the length field so we know how many more bytes to receive.
		encContentsLen, err := p.Receive(lengthFieldLen)
		if err != nil {
			return nil, err
		}

		contentsLenBytes, err := p.recvL.Crypt(encContentsLen)
		if err != nil {
			return nil, err
		}

		var contentsLenLE [4]byte
		copy(contentsLenLE[:], contentsLenBytes)

		contentsLen := binary.LittleEndian.Uint32(contentsLenLE[:])

		if contentsLen > maxContentLen {
			return nil, errContentLengthExceeded
		}

		// Decrypt the remainder of the packet.
		numBytes := headerLen + int(contentsLen) + chachapoly1305Expansion
		aeadCiphertext, err := p.Receive(numBytes)
		if err != nil {
			return nil, err
		}

		plaintext, err := p.recvP.Decrypt(aad, aeadCiphertext)
		if err != nil {
			return nil, err
		}

		// Only the first packet is expected to have non-empty AAD. If the
		// ignore bit is set, ignore the packet.
		// TODO: will this cause anything to leak?
		aad = nil
		header := plaintext[:headerLen]
		if (header[0] & (1 << ignoreBitPos)) == 0 {
			return plaintext[headerLen:], nil
		}
	}
}

// ReceivedPrefix returns the partial header bytes we've already received.
func (p *Peer) ReceivedPrefix() []byte {
	return p.receivedPrefix
}

// UseWriterReader uses the passed-in ReadWriter to Send/Receive to/from.
func (p *Peer) UseReadWriter(rw io.ReadWriter) {
	p.rw = rw
}

// Send sends data to the underlying connection. It returns the number of bytes
// sent or an error.
func (p *Peer) Send(data []byte) (int, error) {
	return p.rw.Write(data)
}

// Receive receives numBytes bytes from the underlying connection.
func (p *Peer) Receive(numBytes int) ([]byte, error) {
	b := make([]byte, numBytes)
	index := 0
	total := 0
	for {
		// TODO: Use something that inherently prevents going over?
		if total > numBytes {
			return nil, errFailedToRecv
		}

		if total == numBytes {
			return b, nil
		}

		n, err := p.rw.Read(b[index:])
		if err != nil {
			return nil, err
		}

		total += n
		index += n
	}
}
