package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

func TestRollingMerkleAdd(t *testing.T) {
	tests := []struct {
		leaves            []chainhash.Hash
		expectedRoots     []chainhash.Hash
		expectedNumLeaves uint64
	}{
		// 00  (00 is also a root)
		{
			leaves: []chainhash.Hash{
				{0x00},
			},
			expectedRoots: []chainhash.Hash{
				{0x00},
			},
			expectedNumLeaves: 1,
		},

		// root
		// |---\
		// 00  01
		{
			leaves: []chainhash.Hash{
				{0x00},
				{0x01},
			},
			expectedRoots: []chainhash.Hash{
				func() chainhash.Hash {
					hash, err := chainhash.NewHashFromStr(
						"c2bf026e62af95cd" +
							"7b785e2cd5a5f1ec" +
							"01fafda85886a8eb" +
							"d34482c0b05dc2c2")
					require.NoError(t, err)
					return *hash
				}(),
			},
			expectedNumLeaves: 2,
		},

		// root
		// |---\
		// 00  01  02
		{
			leaves: []chainhash.Hash{
				{0x00},
				{0x01},
				{0x02},
			},
			expectedRoots: []chainhash.Hash{
				func() chainhash.Hash {
					hash, err := chainhash.NewHashFromStr(
						"c2bf026e62af95cd" +
							"7b785e2cd5a5f1ec" +
							"01fafda85886a8eb" +
							"d34482c0b05dc2c2")
					require.NoError(t, err)
					return *hash
				}(),
				{0x02},
			},
			expectedNumLeaves: 3,
		},

		// root
		// |-------\
		// br      br
		// |---\   |---\
		// 00  01  02  03
		{
			leaves: []chainhash.Hash{
				{0x00},
				{0x01},
				{0x02},
				{0x03},
			},
			expectedRoots: []chainhash.Hash{
				func() chainhash.Hash {
					hash, err := chainhash.NewHashFromStr(
						"270714425ea73eb8" +
							"5942f0f705788f25" +
							"1fefa3f533410a3f" +
							"338de46e641082c4")
					require.NoError(t, err)
					return *hash
				}(),
			},
			expectedNumLeaves: 4,
		},

		// root
		// |-------\
		// br      br
		// |---\   |---\
		// 00  01  02  03  04
		{
			leaves: []chainhash.Hash{
				{0x00},
				{0x01},
				{0x02},
				{0x03},
				{0x04},
			},
			expectedRoots: []chainhash.Hash{
				func() chainhash.Hash {
					hash, err := chainhash.NewHashFromStr(
						"270714425ea73eb8" +
							"5942f0f705788f25" +
							"1fefa3f533410a3f" +
							"338de46e641082c4")
					require.NoError(t, err)
					return *hash
				}(),
				{0x04},
			},
			expectedNumLeaves: 5,
		},

		// root
		// |-------\
		// br      br      root
		// |---\   |---\   |---\
		// 00  01  02  03  04  05
		{
			leaves: []chainhash.Hash{
				{0x00},
				{0x01},
				{0x02},
				{0x03},
				{0x04},
				{0x05},
			},
			expectedRoots: []chainhash.Hash{
				func() chainhash.Hash {
					hash, err := chainhash.NewHashFromStr(
						"270714425ea73eb8" +
							"5942f0f705788f25" +
							"1fefa3f533410a3f" +
							"338de46e641082c4")
					require.NoError(t, err)
					return *hash
				}(),
				func() chainhash.Hash {
					hash, err := chainhash.NewHashFromStr(
						"e5c2407ba454ffeb" +
							"28cf0c50c5c293a8" +
							"4c9a75788f8a8f35" +
							"ccb974e606280377")
					require.NoError(t, err)
					return *hash
				}(),
			},
			expectedNumLeaves: 6,
		},
	}

	for _, test := range tests {
		s := newRollingMerkleTreeStore(uint64(len(test.leaves)))
		for _, leaf := range test.leaves {
			s.add(leaf)
		}

		require.Equal(t, s.roots, test.expectedRoots)
		require.Equal(t, s.numLeaves, test.expectedNumLeaves)
	}
}
