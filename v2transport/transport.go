package v2transport

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sync/atomic"

	"golang.org/x/crypto/hkdf"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ellswift"
)

// packetBit is a type used to represent the bits in the packet's header.
type packetBit uint8

const (
	// ignoreBitPos is the position of the ignore bit in the packet's
	// header.
	ignoreBitPos packetBit = 7
)

// BitcoinNet is a type used to represent the Bitcoin network that we're
// connecting to.
//
// NOTE: This is identical to the wire.BitcoinNet type, but allows us to shed a
// large module dependency.
type BitcoinNet uint32

const (
	// garbageSize is the length in bytes of the garbage terminator that
	// each party sends.
	garbageSize = 16

	// MaxGarbageLen is the maximum size of garbage that either peer is
	// allowed to send.
	MaxGarbageLen = 4095

	// maxContentLen is the maximum length of content that can be encrypted
	// or decrypted.
	//
	// TODO: This should be revisited. For some reason, the test vectors
	// want us to encrypt 16777215 bytes even though bitcoind will only
	// decrypt up to 1 + 12 + 4_000_000 bytes by default.
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

	// ErrShouldDowngradeToV1 is returned when we send the peer our
	// ellswift key and they immediately hang up. This indicates that they
	// don't understand v2 transport and interpreted the 64-byte key as a
	// v1 message header + message. This will (always?) decode to an
	// invalid command and checksum. The caller should try connecting to
	// the peer with the OG v1 transport.
	ErrShouldDowngradeToV1 = fmt.Errorf("should downgrade to v1")
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

	// shouldDowngradeToV1 is true if the handshake failed in a way that
	// indicates the peer does not support v2, and a v1 attempt should be
	// made.
	shouldDowngradeToV1 atomic.Bool

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
	net BitcoinNet) error {

	log.Debugf("Creating v2 ciphers (initiating=%v, net=%v)", initiating,
		net)

	// Define the salt as the string "bitcoin_v2_shared_secret" followed by
	// the BitcoinNet's "magic" bytes.
	salt := []byte("bitcoin_v2_shared_secret")

	var magic [4]byte
	binary.LittleEndian.PutUint32(magic[:], uint32(net))
	salt = append(salt, magic[:]...)

	// Use the hkdf Extract function to generate a pseudo-random key.
	prk := hkdf.Extract(sha256.New, ecdhSecret, salt)

	log.Tracef("Using salt=%x for HKDF-Extract", salt)

	// Use the hkdf Expand function with info set to "session_id" to generate a
	// unique sessionID.
	sessionInfo := []byte("session_id")
	sessionReader := hkdf.Expand(sha256.New, prk, sessionInfo)
	_, err := sessionReader.Read(p.sessionID)
	if err != nil {
		log.Errorf("Failed to derive session_id: %v", err)
		return err
	}

	log.Tracef("Using prk=%x for HKDF-Expand", prk)

	log.Tracef("Derived session_id=%x", p.sessionID)

	// Use the Expand operation to generate packet and packet-length encryption
	// ciphers.
	initiatorLInfo := []byte("initiator_L")
	initiatorLReader := hkdf.Expand(sha256.New, prk, initiatorLInfo)
	_, err = initiatorLReader.Read(p.initiatorL)
	if err != nil {
		log.Errorf("Failed to derive initiator_L: %v", err)
		return err
	}

	log.Tracef("Derived initiator_L=%x", p.initiatorL)

	initiatorPInfo := []byte("initiator_P")
	initiatorPReader := hkdf.Expand(sha256.New, prk, initiatorPInfo)
	_, err = initiatorPReader.Read(p.initiatorP)
	if err != nil {
		log.Errorf("Failed to derive initiator_P: %v", err)
		return err
	}

	log.Tracef("Derived initiator_P=%x", p.initiatorP)

	responderLInfo := []byte("responder_L")
	responderLReader := hkdf.Expand(sha256.New, prk, responderLInfo)
	_, err = responderLReader.Read(p.responderL)
	if err != nil {
		log.Errorf("Failed to derive responder_L: %v", err)
		return err
	}

	log.Tracef("Derived responder_L=%x", p.responderL)

	responderPInfo := []byte("responder_P")
	responderPReader := hkdf.Expand(sha256.New, prk, responderPInfo)
	_, err = responderPReader.Read(p.responderP)
	if err != nil {
		log.Errorf("Failed to derive responder_P: %v", err)
		return err
	}

	log.Tracef("Derived responder_P=%x", p.responderP)

	// Create the garbage terminators that each side will use.
	garbageInfo := []byte("garbage_terminators")
	garbageReader := hkdf.Expand(sha256.New, prk, garbageInfo)
	garbageTerminators := make([]byte, 32)
	_, err = garbageReader.Read(garbageTerminators)
	if err != nil {
		log.Errorf("Failed to derive garbage terminators: %v", err)
		return err
	}

	initiatorGarbageTerminator := garbageTerminators[:garbageSize]
	responderGarbageTerminator := garbageTerminators[garbageSize:]

	log.Tracef("Derived initiator garbage terminator=%x",
		initiatorGarbageTerminator)

	log.Tracef("Derived responder garbage terminator=%x",
		responderGarbageTerminator)

	if initiating {
		p.sendL, err = NewFSChaCha20(p.initiatorL)
		if err != nil {
			log.Errorf("Failed to create sendL cipher: %v", err)
			return err
		}

		p.sendP, err = NewFSChaCha20Poly1305(p.initiatorP)
		if err != nil {
			log.Errorf("Failed to create sendP cipher: %v", err)
			return err
		}

		copy(p.sendGarbageTerm[:], initiatorGarbageTerminator)

		p.recvL, err = NewFSChaCha20(p.responderL)
		if err != nil {
			log.Errorf("Failed to create recvL cipher: %v", err)
			return err
		}

		p.recvP, err = NewFSChaCha20Poly1305(p.responderP)
		if err != nil {
			log.Errorf("Failed to create recvP cipher: %v", err)
			return err
		}

		p.recvGarbageTerm = responderGarbageTerminator

		log.Debugf("Initiator ciphers created (sendL, sendP, " +
			"recvL, recvP)")

	} else {
		p.sendL, err = NewFSChaCha20(p.responderL)
		if err != nil {
			log.Errorf("Failed to create sendL cipher: %v", err)
			return err
		}

		p.sendP, err = NewFSChaCha20Poly1305(p.responderP)
		if err != nil {
			log.Errorf("Failed to create sendP cipher: %v", err)
			return err
		}

		copy(p.sendGarbageTerm[:], responderGarbageTerminator)

		p.recvL, err = NewFSChaCha20(p.initiatorL)
		if err != nil {
			log.Errorf("Failed to create recvL cipher: %v", err)
			return err
		}

		p.recvP, err = NewFSChaCha20Poly1305(p.initiatorP)
		if err != nil {
			log.Errorf("Failed to create recvP cipher: %v", err)
			return err
		}

		p.recvGarbageTerm = initiatorGarbageTerminator

		log.Debugf("Responder ciphers created (sendL, sendP, " +
			"recvL, recvP)")
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
	log.Debugf("Initiating v2 handshake (garbageLen=%d)", garbageLen)

	var err error
	p.privkeyOurs, p.ellswiftOurs, err = ellswift.EllswiftCreate()
	if err != nil {
		log.Errorf("Failed to create ellswift keypair: %v", err)
		return err
	}

	log.Tracef("Created ellswift keypair, pubkey=%x", p.ellswiftOurs)

	data, err := p.generateKeyAndGarbage(garbageLen)
	if err != nil {
		return err
	}

	log.Debugf("Sending ellswift pubkey and garbage (total_len=%d)",
		len(data))

	p.Send(data)

	return nil
}

// RespondV2Handshake responds to the initiator, determines if the initiator
// wants to use the v2 protocol and if so returns our ElligatorSwift-encoded
// public key followed by our garbage data over. If the initiator does not want
// to use the v2 protocol, we'll instead revert to the v1 protocol.
func (p *Peer) RespondV2Handshake(garbageLen int, net BitcoinNet) error {
	v1Prefix := createV1Prefix(net)

	log.Debugf("Responding to v2 handshake (garbageLen=%d, net=%v)",
		garbageLen, net)

	log.Tracef("Expecting v1 prefix: %x", v1Prefix)

	var err error

	// Check and see if the received bytes match the v1 protocol's message
	// prefix. If it does, we'll revert to the v1 protocol. If it doesn't,
	// we'll treat this as a v2 peer.
	for len(p.receivedPrefix) < len(v1Prefix) {
		log.Tracef("Received prefix len=%d, need=%d",
			len(p.receivedPrefix), len(v1Prefix))

		var receiveBytes []byte
		receiveBytes, _, err = p.Receive(1)
		if err != nil {
			log.Errorf("Failed to receive byte for v1 prefix "+
				"check: %v", err)
			return err
		}

		log.Tracef("Current received prefix: %x", p.receivedPrefix)

		p.receivedPrefix = append(p.receivedPrefix, receiveBytes...)

		lastIdx := len(p.receivedPrefix) - 1

		if p.receivedPrefix[lastIdx] != v1Prefix[lastIdx] {
			log.Debugf("Received byte %x does not match v1 "+
				"prefix at index %d, assuming v2 peer",
				p.receivedPrefix[lastIdx], lastIdx)

			p.privkeyOurs, p.ellswiftOurs, err = ellswift.EllswiftCreate()
			if err != nil {
				log.Errorf("Failed to create ellswift "+
					"keypair: %v", err)
				return err
			}

			log.Tracef("Created ellswift keypair, pubkey=%x",
				p.ellswiftOurs)

			data, err := p.generateKeyAndGarbage(garbageLen)
			if err != nil {
				return err
			}

			// Send over our ElligatorSwift-encoded pubkey followed
			// by our randomly generated garbage.
			log.Debugf("Sending ellswift pubkey and garbage "+
				"(total_len=%d)", len(data))
			p.Send(data)

			return nil
		}
	}

	log.Infof("Received full v1 prefix match, reverting to v1 protocol")

	return ErrUseV1Protocol
}

// generateKeyAndGarbage returns a byte slice containing our ellswift-encoded
// public key followed by the garbage we'll send over.
func (p *Peer) generateKeyAndGarbage(garbageLen int) ([]byte, error) {
	log.Tracef("Generating key and garbage (garbageLen=%d)", garbageLen)

	if garbageLen > MaxGarbageLen {
		log.Errorf("Requested garbage length %d exceeds max %d",
			garbageLen, MaxGarbageLen)

		return nil, errGarbageTooLarge
	}

	p.sentGarbage = make([]byte, garbageLen)
	_, err := rand.Read(p.sentGarbage)
	if err != nil {
		log.Errorf("Failed to read random bytes for garbage: %v", err)
		return nil, err
	}

	log.Tracef("Generated %d bytes of garbage", garbageLen)

	data := make([]byte, 0, 64+garbageLen)
	data = append(data, p.ellswiftOurs[:]...)
	data = append(data, p.sentGarbage...)

	log.Tracef("Generated key and garbage data (total_len=%d)", len(data))

	return data, nil
}

// createV1Prefix is a helper function that returns the first 16 bytes of the
// version message's header.
func createV1Prefix(net BitcoinNet) []byte {
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
	btcnet BitcoinNet) error {

	log.Debugf("Completing v2 handshake (initiating=%v, "+
		"num_decoys=%d, net=%v)", initiating, len(decoyContentLens),
		btcnet)

	var receivedPrefix []byte
	if initiating {
		log.Trace("Initiator expecting 64 bytes for peer's " +
			"ellswift key")

		receivedPrefix = make([]byte, 0, 16)
	} else {
		// If we are the responder, we have already received bytes to
		// compare against the v1 transport protocol's starting bytes.
		// We have to account for these when reading the rest of the 64
		// bytes off the wire to properly parse the remote's
		// ellswift-encoded public key.
		receivedPrefix = p.receivedPrefix

		log.Tracef("Responder already has prefix_len=%d, expecting %d "+
			"more bytes for peer's ellswift key",
			len(receivedPrefix), 64-len(receivedPrefix))
	}

	recvData, numRead, err := p.Receive(64 - len(receivedPrefix))
	if err != nil {
		// If we receive an error when reading off the wire and we read
		// zero bytes, then we will reconnect to the peer using v1.
		// There are several different errors that Receive can return
		// that indicate we should reconnect. Instead of special-casing
		// them all, just perform these checks if any error was
		// returned.
		if numRead == 0 && initiating {
			// The peer most likely attempted to parse our 64-byte
			// elligator-swift key as a version message and failed
			// when trying to parse the message header into
			// something valid. In this case, return a special
			// error that signals to the server that we can
			// reconnect with the OG v1 scheme.
			log.Debugf("Received transport error during " +
				"v2 handshake, retying downgraded v1 " +
				"connection.")

			p.shouldDowngradeToV1.Store(true)

			return ErrShouldDowngradeToV1
		}

		// If we are the recipient, we can fail.
		log.Errorf("Failed to receive peer's ellswift key data: %v",
			err)
		return err
	}

	log.Tracef("Received %d bytes for peer's ellswift key", len(recvData))

	var ellswiftTheirs [64]byte

	if initiating {
		// If we are initiating, read all 64 bytes into ellswiftTheirs.
		copy(ellswiftTheirs[:], recvData)
	} else {
		// If we are the responder, then we need to account for the
		// bytes already received as part of matching against the
		// starting v1 transport bytes. We sanity check receivedPrefix
		// in case it is too large for some reason.
		prefixLen := len(receivedPrefix)
		if prefixLen > 16 {
			log.Errorf("Responder's received prefix length %d is "+
				"too large (> 16)", prefixLen)

			return errPrefixTooLarge
		}

		copy(ellswiftTheirs[:], receivedPrefix)
		copy(ellswiftTheirs[prefixLen:], recvData)
	}

	log.Tracef("Assembled peer's ellswift key: %x", ellswiftTheirs)

	// Calculate the v1 protocol's message prefix and see if the bytes read
	// read into ellswiftTheirs matches it.
	v1Prefix := createV1Prefix(btcnet)

	// ellswiftTheirs should be at least 16 bytes if receive succeeded, but
	// just in case, check the size.
	if len(ellswiftTheirs) < 16 {
		log.Errorf("Received insufficient bytes (%d) for "+

			"ellswift key", len(ellswiftTheirs))
		return errInsufficientBytes
	}

	if !initiating && bytes.Equal(ellswiftTheirs[4:16], v1Prefix[4:16]) {
		log.Warnf("Peer sent v1 version message for wrong network "+
			"(expected %v)", btcnet)
		return errWrongNetV1Peer
	}

	log.Debug("Calculating ECDH shared secret")

	// Calculate the shared secret to be used in creating the packet
	// ciphers.
	ecdhSecret, err := ellswift.V2Ecdh(
		p.privkeyOurs, ellswiftTheirs, p.ellswiftOurs, initiating,
	)
	if err != nil {
		log.Errorf("Failed to calculate ECDH shared secret: %v", err)
		return err
	}

	log.Tracef("Calculated ECDH shared secret: %x", ecdhSecret)

	err = p.createV2Ciphers(ecdhSecret[:], initiating, btcnet)
	if err != nil {
		return err
	}

	// Send garbage terminator.
	log.Debugf("Sending garbage terminator: %x", p.sendGarbageTerm)
	p.Send(p.sendGarbageTerm[:])

	// Optionally send decoy packets after garbage terminator.
	aad := p.sentGarbage
	for i := 0; i < len(decoyContentLens); i++ {
		log.Tracef("Sending decoy packet %d (content_len=%d)",
			i+1, decoyContentLens[i])

		decoyContent := make([]byte, decoyContentLens[i])

		encPacket, _, err := p.V2EncPacket(decoyContent, aad, true)
		if err != nil {
			log.Errorf("Failed to encrypt/send decoy "+
				"packet %d: %v", i+1, err)
			return err
		}

		p.Send(encPacket)

		// AAD is only used for the first packet after the handshake.
		aad = nil
	}

	// Send version packet.
	log.Debug("Sending version packet")
	_, _, err = p.V2EncPacket(transportVersion, aad, false)
	if err != nil {
		log.Errorf("Failed to encrypt/send version packet: %v", err)
		return err
	}

	log.Debugf("Receiving garbage, looking for terminator: %x",
		p.recvGarbageTerm)

	// Skip garbage until encountering garbage terminator.
	recvGarbage, _, err := p.Receive(16)
	if err != nil {
		log.Errorf("Failed to receive initial 16 bytes of "+
			"garbage: %v", err)
		return err
	}

	log.Tracef("Received initial garbage chunk: %x", recvGarbage)

	// The BIP text states that we can read up to 4111 bytes. We've already
	// read 16 bytes for the garbage terminator, so we need to read up to
	// maxGarbageLen more bytes.
	for i := 0; i < MaxGarbageLen; i++ {
		recvGarbageLen := len(recvGarbage)

		if bytes.Equal(recvGarbage[recvGarbageLen-16:],
			p.recvGarbageTerm) {

			log.Debugf("Found garbage terminator after %d total "+
				"bytes", recvGarbageLen)

			log.Tracef("Processing %d bytes preceding garbage "+
				"terminator", recvGarbageLen-16)

			// Process any potential packet data sent before the
			// terminator.
			_, err = p.V2ReceivePacket(
				recvGarbage[:recvGarbageLen-16],
			)
			if err != nil {
				log.Errorf("Error processing packet data "+
					"before garbage terminator: %v", err)
			}
			return err
		}

		log.Tracef("Garbage terminator not found, receiving 1 more "+
			"byte (total_received=%d)", recvGarbageLen)

		recvData, _, err := p.Receive(1)
		if err != nil {
			log.Errorf("Failed to receive garbage "+
				"byte %d: %v", recvGarbageLen+1, err)
			return err
		}

		recvGarbage = append(recvGarbage, recvData...)
	}

	log.Warnf("Garbage terminator not received after %d "+
		"bytes", len(recvGarbage))

	return errGarbageTermNotRecv
}

// V2EncPacket takes the contents and aad and returns a ciphertext.
func (p *Peer) V2EncPacket(contents []byte, aad []byte, ignore bool) ([]byte,
	int, error) {

	log.Tracef("Encrypting packet (content_len=%d, aad_len=%d, ignore=%v)",
		len(contents), len(aad), ignore)

	contentLen := len(contents)

	if contentLen > maxContentLen {
		log.Errorf("Content length %d exceeds max %d",
			contentLen, maxContentLen)

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

	log.Tracef("Packet header: %x", header)

	plaintext := make([]byte, 0, contentLen+1)
	plaintext = append(plaintext, header...)
	plaintext = append(plaintext, contents...)

	log.Tracef("Plaintext (header + content): %x", plaintext)

	aeadCiphertext, err := p.sendP.Encrypt(aad, plaintext)
	if err != nil {
		log.Errorf("Failed to encrypt packet content: %v", err)
		return nil, 0, err
	}

	log.Tracef("AEAD ciphertext: %x", aeadCiphertext)

	// We cut off a byte when feeding to Crypt.
	contentsLE := make([]byte, 4)
	binary.LittleEndian.PutUint32(contentsLE, uint32(contentLen))

	log.Tracef("Encrypting content length %d (%x)",
		contentLen, contentsLE[:lengthFieldLen])

	encContentsLen, err := p.sendL.Crypt(contentsLE[:lengthFieldLen])
	if err != nil {
		log.Errorf("Failed to encrypt content length: %v", err)
		return nil, 0, err
	}

	log.Tracef("Encrypted content length: %x", encContentsLen)

	encPacket := make([]byte, 0, len(encContentsLen)+len(aeadCiphertext))
	encPacket = append(encPacket, encContentsLen...)
	encPacket = append(encPacket, aeadCiphertext...)

	log.Tracef("Full encrypted packet: %x", encPacket)

	totalBytes, err := p.Send(encPacket)
	if err != nil {
		log.Errorf("Failed to send encrypted packet: %v", err)

		// Return the packet anyway, as some might have been sent.
		return encPacket, totalBytes, err
	}

	log.Tracef("Sent %d bytes for encrypted packet", totalBytes)

	return encPacket, totalBytes, err
}

// V2ReceivePacket takes the aad and decrypts a received packet.
func (p *Peer) V2ReceivePacket(aad []byte) ([]byte, error) {
	log.Tracef("Attempting to receive packet (aad_len=%d)", len(aad))

	for {
		log.Tracef("Receiving %d bytes for encrypted length",
			lengthFieldLen)

		// Decrypt the length field so we know how many more bytes to receive.
		encContentsLen, _, err := p.Receive(lengthFieldLen)
		if err != nil {
			log.Errorf("Failed to receive encrypted length: %v",
				err)
			return nil, err
		}

		log.Tracef("Received encrypted length: %x", encContentsLen)

		contentsLenBytes, err := p.recvL.Crypt(encContentsLen)
		if err != nil {
			log.Errorf("Failed to decrypt content length: %v", err)
			return nil, err
		}

		log.Tracef("Decrypted content length bytes: %x",
			contentsLenBytes)

		var contentsLenLE [4]byte
		copy(contentsLenLE[:], contentsLenBytes)

		contentsLen := binary.LittleEndian.Uint32(contentsLenLE[:])

		log.Tracef("Decrypted content length=%d", contentsLen)

		if contentsLen > maxContentLen {
			log.Errorf("Decrypted content length %d exceeds "+
				"max %d", contentsLen, maxContentLen)

			return nil, errContentLengthExceeded
		}

		// Decrypt the remainder of the packet.
		numBytes := headerLen + int(contentsLen) + chachapoly1305Expansion

		log.Tracef("Receiving %d bytes for encrypted packet body",
			numBytes)

		aeadCiphertext, _, err := p.Receive(numBytes)
		if err != nil {
			log.Errorf("Failed to receive encrypted "+
				"packet body: %v", err)
			return nil, err
		}

		log.Tracef("Received encrypted packet body: %x", aeadCiphertext)

		plaintext, err := p.recvP.Decrypt(aad, aeadCiphertext)
		if err != nil {
			log.Errorf("Failed to decrypt packet body: %v", err)
			return nil, err
		}

		log.Tracef("Decrypted plaintext (header + content): %x",
			plaintext)

		// Only the first packet is expected to have non-empty AAD. If
		// the ignore bit is set, ignore the packet.
		//
		// TODO: will this cause anything to leak?
		// AAD is only used for the first packet after the handshake.
		aad = nil
		header := plaintext[:headerLen]
		log.Tracef("Packet header: %x", header)
		if (header[0] & (1 << ignoreBitPos)) == 0 {
			log.Tracef("Ignore bit not set, returning content "+
				"(len=%d)", len(plaintext[headerLen:]))

			return plaintext[headerLen:], nil
		}

		log.Debugf("Ignore bit set, discarding packet "+
			"(content_len=%d)", contentsLen)
	}
}

// ReceivedPrefix returns the partial header bytes we've already received.
func (p *Peer) ReceivedPrefix() []byte {
	return p.receivedPrefix
}

// ShouldDowngradeToV1 returns true if the v2 handshake failed in a way that
// suggests the peer does not support v2 and a v1 connection should be
// attempted.
func (p *Peer) ShouldDowngradeToV1() bool {
	return p.shouldDowngradeToV1.Load()
}

// UseWriterReader uses the passed-in ReadWriter to Send/Receive to/from.
func (p *Peer) UseReadWriter(rw io.ReadWriter) {
	p.rw = rw
}

// Send sends data to the underlying connection. It returns the number of bytes
// sent or an error.
func (p *Peer) Send(data []byte) (int, error) {
	log.Tracef("Sending %d bytes", len(data))
	n, err := p.rw.Write(data)
	if err != nil {
		log.Errorf("Send failed after %d bytes: %v", n, err)
	} else {
		log.Tracef("Sent %d bytes successfully", n)
	}
	return n, err
}

// Receive receives numBytes bytes from the underlying connection.
func (p *Peer) Receive(numBytes int) ([]byte, int, error) {
	b := make([]byte, numBytes)
	index := 0
	total := 0

	log.Tracef("Attempting to receive %d bytes", numBytes)

	for {
		// TODO: Use something that inherently prevents going over?
		if total > numBytes {
			// This should be logically impossible with io.ReadFull
			// semantics used implicitly by the loop structure.
			log.Criticalf("Receive logic error: total=%d > "+
				"numBytes=%d", total, numBytes)
			return nil, total, errFailedToRecv
		}

		if total == numBytes {
			log.Tracef("Successfully received %d bytes", total)
			return b, total, nil
		}

		log.Tracef("Calling Read (need %d bytes, have "+
			"%d)", numBytes-total, total)

		n, err := p.rw.Read(b[index:])
		if err != nil {
			log.Errorf("Receive failed after reading %d bytes "+
				"(target %d): %v", total+n, numBytes, err)
			return nil, total, err
		}

		log.Tracef("Read returned %d bytes", n)

		total += n
		index += n
	}
}
