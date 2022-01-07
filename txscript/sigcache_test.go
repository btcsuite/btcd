// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"crypto/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// genRandomSig returns a random message, a signature of the message under the
// public key and the public key. This function is used to generate randomized
// test data.
func genRandomSig() (*chainhash.Hash, *ecdsa.Signature, *btcec.PublicKey, error) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, nil, nil, err
	}

	var msgHash chainhash.Hash
	if _, err := rand.Read(msgHash[:]); err != nil {
		return nil, nil, nil, err
	}

	sig := ecdsa.Sign(privKey, msgHash[:])

	return &msgHash, sig, privKey.PubKey(), nil
}

// TestSigCacheAddExists tests the ability to add, and later check the
// existence of a signature triplet in the signature cache.
func TestSigCacheAddExists(t *testing.T) {
	sigCache := NewSigCache(200)

	// Generate a random sigCache entry triplet.
	msg1, sig1, key1, err := genRandomSig()
	if err != nil {
		t.Errorf("unable to generate random signature test data")
	}

	// Add the triplet to the signature cache.
	sigCache.Add(*msg1, sig1.Serialize(), key1.SerializeCompressed())

	// The previously added triplet should now be found within the sigcache.
	sig1Copy, _ := ecdsa.ParseSignature(sig1.Serialize())
	key1Copy, _ := btcec.ParsePubKey(key1.SerializeCompressed())
	if !sigCache.Exists(*msg1, sig1Copy.Serialize(), key1Copy.SerializeCompressed()) {
		t.Errorf("previously added item not found in signature cache")
	}
}

// TestSigCacheAddEvictEntry tests the eviction case where a new signature
// triplet is added to a full signature cache which should trigger randomized
// eviction, followed by adding the new element to the cache.
func TestSigCacheAddEvictEntry(t *testing.T) {
	// Create a sigcache that can hold up to 100 entries.
	sigCacheSize := uint(100)
	sigCache := NewSigCache(sigCacheSize)

	// Fill the sigcache up with some random sig triplets.
	for i := uint(0); i < sigCacheSize; i++ {
		msg, sig, key, err := genRandomSig()
		if err != nil {
			t.Fatalf("unable to generate random signature test data")
		}

		sigCache.Add(*msg, sig.Serialize(), key.SerializeCompressed())

		sigCopy, err := ecdsa.ParseSignature(sig.Serialize())
		if err != nil {
			t.Fatalf("unable to parse sig: %v", err)
		}
		keyCopy, err := btcec.ParsePubKey(key.SerializeCompressed())
		if err != nil {
			t.Fatalf("unable to parse key: %v", err)
		}
		if !sigCache.Exists(*msg, sigCopy.Serialize(), keyCopy.SerializeCompressed()) {
			t.Errorf("previously added item not found in signature" +
				"cache")
		}
	}

	// The sigcache should now have sigCacheSize entries within it.
	if uint(len(sigCache.validSigs)) != sigCacheSize {
		t.Fatalf("sigcache should now have %v entries, instead it has %v",
			sigCacheSize, len(sigCache.validSigs))
	}

	// Add a new entry, this should cause eviction of a randomly chosen
	// previous entry.
	msgNew, sigNew, keyNew, err := genRandomSig()
	if err != nil {
		t.Fatalf("unable to generate random signature test data")
	}
	sigCache.Add(*msgNew, sigNew.Serialize(), keyNew.SerializeCompressed())

	// The sigcache should still have sigCache entries.
	if uint(len(sigCache.validSigs)) != sigCacheSize {
		t.Fatalf("sigcache should now have %v entries, instead it has %v",
			sigCacheSize, len(sigCache.validSigs))
	}

	// The entry added above should be found within the sigcache.
	sigNewCopy, _ := ecdsa.ParseSignature(sigNew.Serialize())
	keyNewCopy, _ := btcec.ParsePubKey(keyNew.SerializeCompressed())
	if !sigCache.Exists(*msgNew, sigNewCopy.Serialize(), keyNewCopy.SerializeCompressed()) {
		t.Fatalf("previously added item not found in signature cache")
	}
}

// TestSigCacheAddMaxEntriesZeroOrNegative tests that if a sigCache is created
// with a max size <= 0, then no entries are added to the sigcache at all.
func TestSigCacheAddMaxEntriesZeroOrNegative(t *testing.T) {
	// Create a sigcache that can hold up to 0 entries.
	sigCache := NewSigCache(0)

	// Generate a random sigCache entry triplet.
	msg1, sig1, key1, err := genRandomSig()
	if err != nil {
		t.Errorf("unable to generate random signature test data")
	}

	// Add the triplet to the signature cache.
	sigCache.Add(*msg1, sig1.Serialize(), key1.SerializeCompressed())

	// The generated triplet should not be found.
	sig1Copy, _ := ecdsa.ParseSignature(sig1.Serialize())
	key1Copy, _ := btcec.ParsePubKey(key1.SerializeCompressed())
	if sigCache.Exists(*msg1, sig1Copy.Serialize(), key1Copy.SerializeCompressed()) {
		t.Errorf("previously added signature found in sigcache, but" +
			"shouldn't have been")
	}

	// There shouldn't be any entries in the sigCache.
	if len(sigCache.validSigs) != 0 {
		t.Errorf("%v items found in sigcache, no items should have"+
			"been added", len(sigCache.validSigs))
	}
}
