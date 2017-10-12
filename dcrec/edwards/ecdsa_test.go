// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package edwards

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"io"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"testing"
)

func TestGolden(t *testing.T) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	// sign.input.gz is a selection of test cases from
	// http://ed25519.cr.yp.to/python/sign.input
	testDataZ, err := os.Open("testdata/sign.input.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer testDataZ.Close()
	testData, err := gzip.NewReader(testDataZ)
	if err != nil {
		t.Fatal(err)
	}
	defer testData.Close()

	in := bufio.NewReaderSize(testData, 1<<12)
	lineNo := 0
	for {
		lineNo++
		lineBytes, err := in.ReadBytes(byte('\n'))
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("error reading test data: %s", err)
		}

		line := string(lineBytes)
		parts := strings.Split(line, ":")
		if len(parts) != 5 {
			t.Fatalf("bad number of parts on line %d (want %v, got %v)", lineNo,
				5, len(parts))
		}

		privBytes, _ := hex.DecodeString(parts[0])
		privArray := copyBytes64(privBytes)

		pubKeyBytes, _ := hex.DecodeString(parts[1])
		pubArray := copyBytes(pubKeyBytes)
		msg, _ := hex.DecodeString(parts[2])
		sig, _ := hex.DecodeString(parts[3])
		sigArray := copyBytes64(sig)
		// The signatures in the test vectors also include the message
		// at the end, but we just want R and S.
		sig = sig[:SignatureSize]

		if l := len(pubKeyBytes); l != PubKeyBytesLen {
			t.Fatalf("bad public key length on line %d: got %d bytes", lineNo, l)
		}

		var priv [PrivKeyBytesLen]byte
		copy(priv[:], privBytes)
		copy(priv[32:], pubKeyBytes)

		// Deserialize privkey and test functions.
		privkeyS1, pubkeyS1 := PrivKeyFromSecret(curve, priv[:32])
		privkeyS2, pubkeyS2 := PrivKeyFromBytes(curve, priv[:])
		pkS1 := privkeyS1.SerializeSecret()
		pkS2 := privkeyS2.SerializeSecret()
		pubkS1 := pubkeyS1.Serialize()
		pubkS2 := pubkeyS2.Serialize()
		cmp := bytes.Equal(pkS1[:], pkS2[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(privArray[:], copyBytes64(pkS1)[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(privArray[:], copyBytes64(pkS2)[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(pubkS1[:], pubkS2[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(pubArray[:], copyBytes(pubkS1)[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(pubArray[:], copyBytes(pubkS2)[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		// Deserialize pubkey and test functions.
		pubkeyP, err := ParsePubKey(curve, pubKeyBytes)
		if err != nil {
			t.Fatalf("ParsePubKey: %v", err)
		}

		pubkP := pubkeyP.Serialize()
		cmp = bytes.Equal(pubkS1[:], pubkP[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(pubkS2[:], pubkP[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		cmp = bytes.Equal(pubArray[:], copyBytes(pubkP)[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		// Deserialize signature and test functions.
		internalSig, err := ParseSignature(curve, sig)
		if err != nil {
			t.Fatalf("ParseSignature failed: %v", err)
		}
		iSigSerialized := internalSig.Serialize()
		cmp = bytes.Equal(sigArray[:], copyBytes64(iSigSerialized)[:])
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		sig2r, sig2s, err := Sign(curve, privkeyS2, msg)
		if err != nil {
			t.Fatalf("Sign failed: %v", err)
		}
		sig2 := &Signature{sig2r, sig2s}
		sig2B := sig2.Serialize()
		if !bytes.Equal(sig, sig2B[:]) {
			t.Errorf("different signature result on line %d: %x vs %x", lineNo,
				sig, sig2B[:])
		}

		var pubKey [PubKeyBytesLen]byte
		copy(pubKey[:], pubKeyBytes)
		if !Verify(pubkeyP, msg, sig2r, sig2s) {
			t.Errorf("signature failed to verify on line %d", lineNo)
		}
	}
}

func randPrivScalarKeyList(curve *TwistedEdwardsCurve, i int) []*PrivateKey {
	r := rand.New(rand.NewSource(54321))

	privKeyList := make([]*PrivateKey, i)
	for j := 0; j < i; j++ {
		for {
			bIn := new([32]byte)
			for k := 0; k < PrivScalarSize; k++ {
				randByte := r.Intn(255)
				bIn[k] = uint8(randByte)
			}

			bInBig := new(big.Int).SetBytes(bIn[:])
			bInBig.Mod(bInBig, curve.N)
			bIn = copyBytes(bInBig.Bytes())
			bIn[31] &= 248

			pks, _, err := PrivKeyFromScalar(curve, bIn[:])
			if err != nil {
				r.Seed(int64(j) + r.Int63n(12345))
				continue
			}

			// No duplicates allowed.
			if j > 0 &&
				(bytes.Equal(pks.Serialize(), privKeyList[j-1].Serialize())) {
				continue
			}

			privKeyList[j] = pks
			r.Seed(int64(j) + 54321)
			break
		}
	}

	return privKeyList
}

func TestNonStandardSignatures(t *testing.T) {
	tRand := rand.New(rand.NewSource(54321))

	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()
	msg := []byte{
		0xbe, 0x13, 0xae, 0xf4,
		0xe8, 0xa2, 0x00, 0xb6,
		0x45, 0x81, 0xc4, 0xd1,
		0x0c, 0xf4, 0x1b, 0x5b,
		0xe1, 0xd1, 0x81, 0xa7,
		0xd3, 0xdc, 0x37, 0x55,
		0x58, 0xc1, 0xbd, 0xa2,
		0x98, 0x2b, 0xd9, 0xfb,
	}

	pks := randPrivScalarKeyList(curve, 50)
	for _, pk := range pks {
		r, s, err := Sign(curve, pk, msg)
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}

		pubX, pubY := pk.Public()
		pub := NewPublicKey(curve, pubX, pubY)
		ok := Verify(pub, msg, r, s)
		if !ok {
			t.Fatalf("expected %v, got %v", true, ok)
		}

		// Test serializing/deserializing.
		privKeyDupTest, _, err := PrivKeyFromScalar(curve,
			copyBytes(pk.ecPk.D.Bytes())[:])

		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}

		cmp := privKeyDupTest.GetD().Cmp(pk.GetD()) == 0
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		privKeyDupTest2, _, err := PrivKeyFromScalar(curve, pk.Serialize())
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}

		cmp = privKeyDupTest2.GetD().Cmp(pk.GetD()) == 0
		if !cmp {
			t.Fatalf("expected %v, got %v", true, cmp)
		}

		// Screw up a random bit in the signature and
		// make sure it still fails.
		sig := NewSignature(r, s)
		sigBad := sig.Serialize()
		pos := tRand.Intn(63)
		bitPos := tRand.Intn(7)
		sigBad[pos] ^= 1 << uint8(bitPos)

		bSig, err := ParseSignature(curve, sigBad)
		if err != nil {
			// Signature failed to parse, continue.
			continue
		}
		ok = Verify(pub, msg, bSig.GetR(), bSig.GetS())
		if ok {
			t.Fatalf("expected %v, got %v", false, ok)
		}

		// Screw up a random bit in the pubkey and
		// make sure it still fails.
		pkBad := pub.Serialize()
		pos = tRand.Intn(31)
		if pos == 0 {
			// 0th bit in first byte doesn't matter
			bitPos = tRand.Intn(6) + 1
		} else {
			bitPos = tRand.Intn(7)
		}
		pkBad[pos] ^= 1 << uint8(bitPos)
		bPub, err := ParsePubKey(curve, pkBad)
		if err == nil && bPub != nil {
			ok = Verify(bPub, msg, r, s)
			if ok {
				t.Fatalf("expected %v, got %v", false, ok)
			}
		}
	}
}

func randPrivKeyList(curve *TwistedEdwardsCurve, i int) []*PrivateKey {
	r := rand.New(rand.NewSource(54321))

	privKeyList := make([]*PrivateKey, i)
	for j := 0; j < i; j++ {
		for {
			bIn := new([32]byte)
			for k := 0; k < fieldIntSize; k++ {
				randByte := r.Intn(255)
				bIn[k] = uint8(randByte)
			}

			pks, _ := PrivKeyFromSecret(curve, bIn[:])
			if pks == nil {
				continue
			}
			if j > 0 &&
				(bytes.Equal(pks.Serialize(), privKeyList[j-1].Serialize())) {
				r.Seed(int64(j) + r.Int63n(12345))
				continue
			}

			privKeyList[j] = pks
			r.Seed(int64(j) + 54321)
			break
		}
	}

	return privKeyList
}

func benchmarkSigning(b *testing.B) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	r := rand.New(rand.NewSource(54321))
	msg := []byte{
		0xbe, 0x13, 0xae, 0xf4,
		0xe8, 0xa2, 0x00, 0xb6,
		0x45, 0x81, 0xc4, 0xd1,
		0x0c, 0xf4, 0x1b, 0x5b,
		0xe1, 0xd1, 0x81, 0xa7,
		0xd3, 0xdc, 0x37, 0x55,
		0x58, 0xc1, 0xbd, 0xa2,
		0x98, 0x2b, 0xd9, 0xfb,
	}

	numKeys := 1024
	privKeyList := randPrivKeyList(curve, numKeys)

	for n := 0; n < b.N; n++ {
		randIndex := r.Intn(numKeys - 1)
		_, _, err := Sign(curve, privKeyList[randIndex], msg)
		if err != nil {
			panic("sign failure")
		}
	}
}

func BenchmarkSigning(b *testing.B) { benchmarkSigning(b) }

func benchmarkSigningNonStandard(b *testing.B) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()

	r := rand.New(rand.NewSource(54321))
	msg := []byte{
		0xbe, 0x13, 0xae, 0xf4,
		0xe8, 0xa2, 0x00, 0xb6,
		0x45, 0x81, 0xc4, 0xd1,
		0x0c, 0xf4, 0x1b, 0x5b,
		0xe1, 0xd1, 0x81, 0xa7,
		0xd3, 0xdc, 0x37, 0x55,
		0x58, 0xc1, 0xbd, 0xa2,
		0x98, 0x2b, 0xd9, 0xfb,
	}

	numKeys := 250
	privKeyList := randPrivScalarKeyList(curve, numKeys)

	for n := 0; n < b.N; n++ {
		randIndex := r.Intn(numKeys - 1)
		_, _, err := Sign(curve, privKeyList[randIndex], msg)
		if err != nil {
			panic("sign failure")
		}
	}
}

func BenchmarkSigningNonStandard(b *testing.B) { benchmarkSigningNonStandard(b) }

type SignatureVerParams struct {
	pubkey *PublicKey
	msg    []byte
	sig    *Signature
}

func randSigList(curve *TwistedEdwardsCurve, i int) []*SignatureVerParams {
	r := rand.New(rand.NewSource(54321))

	privKeyList := make([]*PrivateKey, i)
	for j := 0; j < i; j++ {
		for {
			bIn := new([32]byte)
			for k := 0; k < fieldIntSize; k++ {
				randByte := r.Intn(255)
				bIn[k] = uint8(randByte)
			}

			pks, _ := PrivKeyFromSecret(curve, bIn[:])
			if pks == nil {
				continue
			}
			privKeyList[j] = pks
			r.Seed(int64(j) + 54321)
			break
		}
	}

	msgList := make([][]byte, i)
	for j := 0; j < i; j++ {
		m := make([]byte, 32)
		for k := 0; k < fieldIntSize; k++ {
			randByte := r.Intn(255)
			m[k] = uint8(randByte)
		}
		msgList[j] = m
		r.Seed(int64(j) + 54321)
	}

	sigsList := make([]*Signature, i)
	for j := 0; j < i; j++ {
		r, s, err := Sign(curve, privKeyList[j], msgList[j])
		if err != nil {
			panic("sign failure")
		}
		sig := &Signature{r, s}
		sigsList[j] = sig
	}

	sigStructList := make([]*SignatureVerParams, i)
	for j := 0; j < i; j++ {
		ss := new(SignatureVerParams)
		pkx, pky := privKeyList[j].Public()
		ss.pubkey = NewPublicKey(curve, pkx, pky)
		ss.msg = msgList[j]
		ss.sig = sigsList[j]
		sigStructList[j] = ss
	}

	return sigStructList
}

func benchmarkVerification(b *testing.B) {
	curve := new(TwistedEdwardsCurve)
	curve.InitParam25519()
	r := rand.New(rand.NewSource(54321))

	numSigs := 1024
	sigList := randSigList(curve, numSigs)

	for n := 0; n < b.N; n++ {
		randIndex := r.Intn(numSigs - 1)
		ver := Verify(sigList[randIndex].pubkey,
			sigList[randIndex].msg,
			sigList[randIndex].sig.R,
			sigList[randIndex].sig.S)
		if !ver {
			panic("made invalid sig")
		}
	}
}

func BenchmarkVerification(b *testing.B) { benchmarkVerification(b) }
