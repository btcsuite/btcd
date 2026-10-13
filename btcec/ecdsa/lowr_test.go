// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ecdsa

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// lowRTests are keys and hashes with the signatures that Bitcoin Core's
// CKey::Sign makes for them without grinding (plain) and with it (low).  They
// were made by libsecp256k1 through electrum-ecc's ecdsa_sign, which runs the
// same loop, and they match btclib-ecc's sign_.
//
// The counter is the first attempt with a low R.
var lowRTests = []struct {
	name    string
	counter int
	key     string
	hash    string
	plain   string
	low     string
}{{
	name:    "first attempt already low",
	counter: 0,
	key: "ac31f3b925f88c82b4cd73562be555a69f773adb35ef7f8b8a434f3c" +
		"61fd9351",
	hash: "7c33a617ca900610a0a7e34f1cdce57b6f369ef5997d4d3a0760595e" +
		"6e464ed5",
	plain: "3044022032eb967b026bf35d692289c863b6fee5ee5b29e763b0a5f4" +
		"f9ecfe0eddaba00102202c2186c60568da08613158c4f9578b4cbe0c" +
		"8901c2047e1b73e9342639b72561",
	low: "3044022032eb967b026bf35d692289c863b6fee5ee5b29e763b0a5f4" +
		"f9ecfe0eddaba00102202c2186c60568da08613158c4f9578b4cbe0c" +
		"8901c2047e1b73e9342639b72561",
}, {
	// A high R with a short S is 70 bytes in DER, as long as the ground
	// signature: a length check would keep it.
	name:    "first attempt high R and 70 bytes",
	counter: 1,
	key: "c439c5a87b874c7c15512ec4dfb7b46724a0791121eb2727dbd642a2" +
		"c51c80c9",
	hash: "d3d6e286ee7b772786653ff8705eff856d8423cf140cd68952358ec6" +
		"ef566c0d",
	plain: "3044022100ab5470cb2f1bb2ceb977128179e71594221e912eaff0ec" +
		"4677cea4942c5adf58021f291dfb56177461c86b45e048b1377958b3" +
		"d952fb0d34fdbb718be52ccd13a7",
	low: "3044022029d509a439214e76dff6e74464215319851e8b8fea73c533" +
		"48ce00ae97d0f9f70220243db5ff0c4f6a4e1ea080d4c6cd983f14e6" +
		"0a0540224cf2e17c36403a7b1535",
}, {
	name:    "first attempt high R and 71 bytes",
	counter: 1,
	key: "29f415c312c35782ff5dfeca33212d6216351627947fd83fcb25a557" +
		"e3eec14d",
	hash: "49a0e2f5972c6beaf3c54415bc5309ff12760113c504af6ad70ea7fb" +
		"65edb0be",
	plain: "3045022100d1db29f6d7edfa5a2d8a6a83ee4e0f17c6535772fbdba8" +
		"b38e338103a644445202200695c05c022d03229c14ea5024e41db633" +
		"be79deab735b9903881a74a209f2f8",
	low: "3044022030b2a2a125fcd1ec6358c301a18c2502e943bafda80ec897" +
		"32b2b33a1699398902206b5215a897c3f27c0fef8bc2e4f85230f690" +
		"206d2661681149e5cbab0fbfa7f6",
}, {
	name:    "counter 2",
	counter: 2,
	key: "2e5a4ead7562fd7071b4478c139ef322656f06bbbf674ec436e81d4a" +
		"b1b40cdc",
	hash: "5978f368cbe846c3037364a2cb7d2c2c6bb436e3c31fa8bd51bb7eb4" +
		"e1331e0a",
	plain: "3045022100f3556b27a23c5b16379b35a03eda690556b5dc2958f055" +
		"4342fd1ec79a585f1d02203f72a57470ef7515813ed54a2e3b3c3b56" +
		"797c37fbe39e17d607be1758caa081",
	low: "304402202f2abc0006d7e66c25968196b6285d57f06ece8748e021dc" +
		"99df876226e1a0de022042714de98a66c5dd65e99841d0e57e8832e7" +
		"ccfcafd33dead649571d52371427",
}, {
	name:    "counter 5",
	counter: 5,
	key: "e8be8e3c161f302151ee5b3b86f01f9dcd8131fc4564f8f62ffb9473" +
		"657f8480",
	hash: "b60ad0b867666e7ebacd6d13bd717315b9df79786394fd38f76c2d66" +
		"8f72f23d",
	plain: "3045022100c29aef1fc3e8f2f02665820f82bcfd2b64d640d30c2353" +
		"fd766196e51d1740410220275e27cf43df5c4fea243c423705610f46" +
		"c50e6e55344f56b633a4c28dfcd07a",
	low: "304402201ec2b1a194e275739c345fe379e8d04c20f806b2afcc4bce" +
		"5ae568aeab32bf0702203b328e46b649aaad8c3f48caaa999bf00da4" +
		"c0e5cd17a2f0cdf235ad6cb6ca7a",
}}

// TestSignLowR checks SignLowR against signatures made by libsecp256k1 with
// Bitcoin Core's grinding, and that Sign still gives the plain signature.
func TestSignLowR(t *testing.T) {
	t.Parallel()

	for _, test := range lowRTests {
		t.Run(test.name, func(t *testing.T) {
			privKey, pubKey := btcec.PrivKeyFromBytes(
				decodeHex(test.key),
			)
			hash := decodeHex(test.hash)

			// The plain signature has the expected R, which is
			// high unless the first attempt is the one kept.
			plain := Sign(privKey, hash)
			require.Equal(t, test.plain, hexString(plain))
			require.Equal(t, test.counter == 0, hasLowR(plain))

			sig := SignLowR(privKey, hash)
			require.Equal(t, test.low, hexString(sig))
			require.True(t, hasLowR(sig))
			require.True(t, sig.Verify(hash, pubKey))

			// The result parses as strict DER with a low S.
			require.NoError(t, VerifyLowS(sig.Serialize()))
		})
	}
}

// TestSignLowRRandom checks that SignLowR gives a low R, a low S and a valid
// signature, and that it is Sign's signature whenever that has a low R.
func TestSignLowRRandom(t *testing.T) {
	t.Parallel()

	var plainLow, ground int
	for i := 0; i < 200; i++ {
		privKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		hash := make([]byte, 32)
		_, err = rand.Read(hash)
		require.NoError(t, err)

		sig := SignLowR(privKey, hash)
		require.True(t, hasLowR(sig))
		require.True(t, sig.Verify(hash, privKey.PubKey()))
		require.NoError(t, VerifyLowS(sig.Serialize()))

		plain := Sign(privKey, hash)
		if hasLowR(plain) {
			plainLow++
			require.True(t, plain.IsEqual(sig))
		} else {
			ground++
			require.False(t, plain.IsEqual(sig))
		}
	}

	// Both cases happen, each with probability 1/2 per signature.
	require.NotZero(t, plainLow)
	require.NotZero(t, ground)
}

func hexString(sig *Signature) string {
	return hex.EncodeToString(sig.Serialize())
}
