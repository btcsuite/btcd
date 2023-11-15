package integration

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/stretchr/testify/require"
)

func getBlockFromString(t *testing.T, hexStr string) *btcutil.Block {
	t.Helper()

	serializedBlock, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf("couldn't decode hex string of %s", hexStr)
	}

	block, err := btcutil.NewBlockFromBytes(serializedBlock)
	if err != nil {
		t.Fatalf("couldn't make a new block from bytes. "+
			"Decoded hex string: %s", hexStr)
	}

	return block
}

// compareMultipleChainTips checks that all the expected chain tips are included in got chain tips and
// verifies that the got chain tip matches the expected chain tip.
func compareMultipleChainTips(t *testing.T, gotChainTips, expectedChainTips []*btcjson.GetChainTipsResult) error {
	if len(gotChainTips) != len(expectedChainTips) {
		return fmt.Errorf("Expected %d chaintips but got %d", len(expectedChainTips), len(gotChainTips))
	}

	gotChainTipsMap := make(map[string]btcjson.GetChainTipsResult)
	for _, gotChainTip := range gotChainTips {
		gotChainTipsMap[gotChainTip.Hash] = *gotChainTip
	}

	for _, expectedChainTip := range expectedChainTips {
		gotChainTip, found := gotChainTipsMap[expectedChainTip.Hash]
		if !found {
			return fmt.Errorf("Couldn't find expected chaintip with hash %s", expectedChainTip.Hash)
		}

		require.Equal(t, gotChainTip, *expectedChainTip)
	}

	return nil
}

func TestGetChainTips(t *testing.T) {
	// block1Hex is a block that builds on top of the regtest genesis block.
	// Has blockhash of "36c056247e8c0589f6307995e4e13acf2b2b79cad9ecd5a4eeab2131ed0ecde5".
	block1Hex := "0000002006226e46111a0b59caaf126043eb5bbf28c34f3a5e332a1fc7b2b73cf18891" +
		"0f71881025ae0d41ce8748b79ac40e5f3197af3bb83a594def7943aff0fce504c638ea6d63f" +
		"fff7f2000000000010200000000010100000000000000000000000000000000000000000000" +
		"00000000000000000000ffffffff025100ffffffff0200f2052a010000001600149b0f9d020" +
		"8b3b425246e16830562a63bf1c701180000000000000000266a24aa21a9ede2f61c3f71d1de" +
		"fd3fa999dfa36953755c690689799962b48bebd836974e8cf90120000000000000000000000" +
		"000000000000000000000000000000000000000000000000000"

	// block2Hex is a block that builds on top of block1Hex.
	// Has blockhash of "664b51334782a4ad16e8471b530dcd0027c75b8c25187b41dfc85ecd353295c6".
	block2Hex := "00000020e5cd0eed3121abeea4d5ecd9ca792b2bcf3ae1e4957930f689058c7e2456c0" +
		"362a78a11b875d31af2ea493aa5b6b623e0d481f11e69f7147ab974be9da087f3e24696f63f" +
		"fff7f2001000000010200000000010100000000000000000000000000000000000000000000" +
		"00000000000000000000ffffffff025200ffffffff0200f2052a0100000016001470fea1feb" +
		"4969c1f237753ae29c0217c6637835c0000000000000000266a24aa21a9ede2f61c3f71d1de" +
		"fd3fa999dfa36953755c690689799962b48bebd836974e8cf90120000000000000000000000" +
		"000000000000000000000000000000000000000000000000000"

	// block3Hex is a block that builds on top of block2Hex.
	// Has blockhash of "17a5c5cb90ecde5a46dd195d434eea46b653e35e4517070eade429db3ac83944".
	block3Hex := "00000020c6953235cd5ec8df417b18258c5bc72700cd0d531b47e816ada4824733514b" +
		"66c3ad4d567a36c20df07ea0b7fce1e4b4ee5be3eaf0b946b0ae73f3a74d47f0cf99696f63f" +
		"fff7f2000000000010200000000010100000000000000000000000000000000000000000000" +
		"00000000000000000000ffffffff025300ffffffff0200f2052a010000001600140e835869b" +
		"154f647d11376634b5e8c785e7d21060000000000000000266a24aa21a9ede2f61c3f71d1de" +
		"fd3fa999dfa36953755c690689799962b48bebd836974e8cf90120000000000000000000000" +
		"000000000000000000000000000000000000000000000000000"

	// block4Hex is a block that builds on top of block3Hex.
	// Has blockhash of "7b357f3073c4397d6d069a32a09141c32560f3c62233ca138eb5e03c5991f45c".
	block4Hex := "000000204439c83adb29e4ad0e0717455ee353b646ea4e435d19dd465adeec90cbc5a5" +
		"17ab639a5dd622e90f5f9feffc1c7c28f47a2caf85c21d7dd52cd223a7164619e37a6a6f63f" +
		"fff7f2004000000010200000000010100000000000000000000000000000000000000000000" +
		"00000000000000000000ffffffff025400ffffffff0200f2052a01000000160014a157c74b4" +
		"42a3e11b45cf5273f8c0c032c5a40ed0000000000000000266a24aa21a9ede2f61c3f71d1de" +
		"fd3fa999dfa36953755c690689799962b48bebd836974e8cf90120000000000000000000000" +
		"000000000000000000000000000000000000000000000000000"

	// block2aHex is a block that builds on top of block1Hex.
	// Has blockhash of "5181a4e34cc23ed95c69749dedf4cc7ebd659243bc1683372f8940c8cd8f9b68".
	block2aHex := "00000020e5cd0eed3121abeea4d5ecd9ca792b2bcf3ae1e4957930f689058c7e2456c" +
		"036f7d4ebe524260c9b6c2b5e3d105cad0b7ddfaeaa29971363574fc1921a3f2f7ad66b6f63" +
		"ffff7f200100000001020000000001010000000000000000000000000000000000000000000" +
		"000000000000000000000ffffffff025200ffffffff0200f2052a0100000016001466fca22d" +
		"0e4679d119ea1e127c984746a1f7e66c0000000000000000266a24aa21a9ede2f61c3f71d1d" +
		"efd3fa999dfa36953755c690689799962b48bebd836974e8cf9012000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000"

	// block3aHex is a block that builds on top of block2aHex.
	// Has blockhash of "0b0216936d1a5c01362256d06a9c9a2b13768fa2f2748549a71008af36dd167f".
	block3aHex := "00000020689b8fcdc840892f378316bc439265bd7eccf4ed9d74695cd93ec24ce3a48" +
		"15161a430ce5cae955b1254b753bc95854d942947855d3ae59002de9773b7fe65fdf16b6f63" +
		"ffff7f200100000001020000000001010000000000000000000000000000000000000000000" +
		"000000000000000000000ffffffff025300ffffffff0200f2052a0100000016001471da0afb" +
		"883c228b18af6bd0cabc471aebe8d1750000000000000000266a24aa21a9ede2f61c3f71d1d" +
		"efd3fa999dfa36953755c690689799962b48bebd836974e8cf9012000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000"

	// block4aHex is a block that builds on top of block3aHex.
	// Has blockhash of "65a00a026eaa83f6e7a7f4a920faa090f3f9d3565a56df2362db2ab2fa14ccec".
	block4aHex := "000000207f16dd36af0810a7498574f2a28f76132b9a9c6ad0562236015c1a6d93160" +
		"20b951fa5ee5072d88d6aef9601999307dbd8d96dad067b80bfe04afe81c7a8c21beb706f63" +
		"ffff7f200000000001020000000001010000000000000000000000000000000000000000000" +
		"000000000000000000000ffffffff025400ffffffff0200f2052a01000000160014fd1f118c" +
		"95a712b8adef11c3cc0643bcb6b709f10000000000000000266a24aa21a9ede2f61c3f71d1d" +
		"efd3fa999dfa36953755c690689799962b48bebd836974e8cf9012000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000"

	// block5aHex is a block that builds on top of block4aHex.
	// Has blockhash of "5c8814bc034a4c37fa5ccdc05e09b45a771bd7505d68092f21869a912737ee10".
	block5aHex := "00000020eccc14fab22adb6223df565a56d3f9f390a0fa20a9f4a7e7f683aa6e020aa" +
		"0656331bd4fcd3db611de7fbf72ef3dff0b85b244b5a983d5c0270e728214f67f9aaa766f63" +
		"ffff7f200600000001020000000001010000000000000000000000000000000000000000000" +
		"000000000000000000000ffffffff025500ffffffff0200f2052a0100000016001438335896" +
		"ad1d087e3541436a5b293c0d23ad27e60000000000000000266a24aa21a9ede2f61c3f71d1d" +
		"efd3fa999dfa36953755c690689799962b48bebd836974e8cf9012000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000"

	// block4bHex is a block that builds on top of block3aHex.
	// Has blockhash of "130458e795cc46f2759195e92737426fb0ada2a07f98434551ffb7500b23c161".
	block4bHex := "000000207f16dd36af0810a7498574f2a28f76132b9a9c6ad0562236015c1a6d93160" +
		"20b14f9ce93d0144c383fea72f408b06b268a1523a029b825a1edfa15b367f6db2cfd7d6f63" +
		"ffff7f200200000001020000000001010000000000000000000000000000000000000000000" +
		"000000000000000000000ffffffff025400ffffffff0200f2052a0100000016001405b5ba2d" +
		"1e549c4c84a623de3575948d3ef8a27f0000000000000000266a24aa21a9ede2f61c3f71d1d" +
		"efd3fa999dfa36953755c690689799962b48bebd836974e8cf9012000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000"

	// Set up regtest chain.
	r, err := rpctest.New(&chaincfg.RegressionNetParams, nil, nil, "")
	if err != nil {
		t.Fatal("TestGetChainTips fail. Unable to create primary harness: ", err)
	}
	if err := r.SetUp(true, 0); err != nil {
		t.Fatalf("TestGetChainTips fail. Unable to setup test chain: %v", err)
	}
	defer r.TearDown()

	// Immediately call getchaintips after setting up regtest.
	gotChainTips, err := r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	// We expect a single genesis block.
	expectedChainTips := []*btcjson.GetChainTipsResult{
		{
			Height:    0,
			Hash:      chaincfg.RegressionNetParams.GenesisHash.String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit 4 blocks.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	blockStrings := []string{block1Hex, block2Hex, block3Hex, block4Hex}
	for _, blockString := range blockStrings {
		block := getBlockFromString(t, blockString)
		err = r.Client.SubmitBlock(block, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, blockStrings[len(blockStrings)-1]).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit 2 blocks that don't build on top of the current active tip.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4  (active)
	//                    \ -> 2a -> 3a       (valid-fork)
	blockStrings = []string{block2aHex, block3aHex}
	for _, blockString := range blockStrings {
		block := getBlockFromString(t, blockString)
		err = r.Client.SubmitBlock(block, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    3,
			Hash:      getBlockFromString(t, block3aHex).Hash().String(),
			BranchLen: 2,
			Status:    "valid-fork",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit a single block that don't build on top of the current active tip.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4   (active)
	//                    \ -> 2a -> 3a -> 4a  (valid-fork)
	block := getBlockFromString(t, block4aHex)
	err = r.Client.SubmitBlock(block, nil)
	if err != nil {
		t.Fatal(err)
	}

	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4aHex).Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit a single block that changes the active branch to 5a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4         (valid-fork)
	//                    \ -> 2a -> 3a -> 4a -> 5a  (active)
	block = getBlockFromString(t, block5aHex)
	err = r.Client.SubmitBlock(block, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
		{
			Height:    5,
			Hash:      getBlockFromString(t, block5aHex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
	}
	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}

	// Submit a single block that builds on top of 3a.
	//
	// Our chain view looks like so:
	// (genesis block) -> 1 -> 2  -> 3  -> 4         (valid-fork)
	//                    \ -> 2a -> 3a -> 4a -> 5a  (active)
	//                                \ -> 4b        (valid-fork)
	block = getBlockFromString(t, block4bHex)
	err = r.Client.SubmitBlock(block, nil)
	if err != nil {
		t.Fatal(err)
	}
	gotChainTips, err = r.Client.GetChainTips()
	if err != nil {
		t.Fatal(err)
	}
	expectedChainTips = []*btcjson.GetChainTipsResult{
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4Hex).Hash().String(),
			BranchLen: 3,
			Status:    "valid-fork",
		},
		{
			Height:    5,
			Hash:      getBlockFromString(t, block5aHex).Hash().String(),
			BranchLen: 0,
			Status:    "active",
		},
		{
			Height:    4,
			Hash:      getBlockFromString(t, block4bHex).Hash().String(),
			BranchLen: 1,
			Status:    "valid-fork",
		},
	}

	err = compareMultipleChainTips(t, gotChainTips, expectedChainTips)
	if err != nil {
		t.Fatalf("TestGetChainTips fail. Error: %v", err)
	}
}
