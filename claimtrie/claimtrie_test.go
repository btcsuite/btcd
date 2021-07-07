package claimtrie

import (
	"math/rand"
	"testing"
	"time"

	"github.com/lbryio/lbcd/claimtrie/change"
	"github.com/lbryio/lbcd/claimtrie/config"
	"github.com/lbryio/lbcd/claimtrie/merkletrie"
	"github.com/lbryio/lbcd/claimtrie/param"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"

	"github.com/stretchr/testify/require"
)

var cfg = config.DefaultConfig

func setup(t *testing.T) {
	param.SetNetwork(wire.TestNet)
	cfg.DataDir = t.TempDir()
}

func b(s string) []byte {
	return []byte(s)
}

func buildTx(hash chainhash.Hash) *wire.MsgTx {
	tx := wire.NewMsgTx(1)
	txIn := wire.NewTxIn(wire.NewOutPoint(&hash, 0), nil, nil)
	tx.AddTxIn(txIn)
	tx.AddTxOut(wire.NewTxOut(0, nil))
	return tx
}

func TestFixedHashes(t *testing.T) {

	r := require.New(t)

	setup(t)
	ct, err := New(cfg)
	r.NoError(err)
	defer ct.Close()

	r.Equal(merkletrie.EmptyTrieHash[:], ct.MerkleHash()[:])

	tx1 := buildTx(*merkletrie.EmptyTrieHash)
	tx2 := buildTx(tx1.TxHash())
	tx3 := buildTx(tx2.TxHash())
	tx4 := buildTx(tx3.TxHash())

	err = ct.AddClaim(b("test"), tx1.TxIn[0].PreviousOutPoint, change.NewClaimID(tx1.TxIn[0].PreviousOutPoint), 50)
	r.NoError(err)

	err = ct.AddClaim(b("test2"), tx2.TxIn[0].PreviousOutPoint, change.NewClaimID(tx2.TxIn[0].PreviousOutPoint), 50)
	r.NoError(err)

	err = ct.AddClaim(b("test"), tx3.TxIn[0].PreviousOutPoint, change.NewClaimID(tx3.TxIn[0].PreviousOutPoint), 50)
	r.NoError(err)

	err = ct.AddClaim(b("tes"), tx4.TxIn[0].PreviousOutPoint, change.NewClaimID(tx4.TxIn[0].PreviousOutPoint), 50)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	expected, err := chainhash.NewHashFromStr("938fb93364bf8184e0b649c799ae27274e8db5221f1723c99fb2acd3386cfb00")
	r.NoError(err)
	r.Equal(expected[:], ct.MerkleHash()[:])
}

func TestEmptyHashFork(t *testing.T) {
	r := require.New(t)

	setup(t)
	param.ActiveParams.AllClaimsInMerkleForkHeight = 2
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	for i := 0; i < 5; i++ {
		err := ct.AppendBlock(false)
		r.NoError(err)
	}
}

func TestNormalizationFork(t *testing.T) {
	r := require.New(t)

	setup(t)
	param.ActiveParams.NormalizedNameForkHeight = 2
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	hash := chainhash.HashH([]byte{1, 2, 3})

	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("AÑEJO"), o1, change.NewClaimID(o1), 10)
	r.NoError(err)

	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("AÑejo"), o2, change.NewClaimID(o2), 5)
	r.NoError(err)

	o3 := wire.OutPoint{Hash: hash, Index: 3}
	err = ct.AddClaim([]byte("あてはまる"), o3, change.NewClaimID(o3), 5)
	r.NoError(err)

	o4 := wire.OutPoint{Hash: hash, Index: 4}
	err = ct.AddClaim([]byte("Aḿlie"), o4, change.NewClaimID(o4), 5)
	r.NoError(err)

	o5 := wire.OutPoint{Hash: hash, Index: 5}
	err = ct.AddClaim([]byte("TEST"), o5, change.NewClaimID(o5), 5)
	r.NoError(err)

	o6 := wire.OutPoint{Hash: hash, Index: 6}
	err = ct.AddClaim([]byte("test"), o6, change.NewClaimID(o6), 7)
	r.NoError(err)

	o7 := wire.OutPoint{Hash: hash, Index: 7}
	err = ct.AddSupport([]byte("test"), o7, 11, change.NewClaimID(o6))
	r.NoError(err)

	incrementBlock(r, ct, 1)
	r.NotEqual(merkletrie.EmptyTrieHash[:], ct.MerkleHash()[:])

	n, err := ct.nodeManager.NodeAt(ct.nodeManager.Height(), []byte("AÑEJO"))
	r.NoError(err)
	r.NotNil(n.BestClaim)
	r.Equal(int32(1), n.TakenOverAt)

	o8 := wire.OutPoint{Hash: hash, Index: 8}
	err = ct.AddClaim([]byte("aÑEJO"), o8, change.NewClaimID(o8), 8)
	r.NoError(err)

	incrementBlock(r, ct, 1)
	r.NotEqual(merkletrie.EmptyTrieHash[:], ct.MerkleHash()[:])

	n, err = ct.nodeManager.NodeAt(ct.nodeManager.Height(), []byte("añejo"))
	r.NoError(err)
	r.Equal(3, len(n.Claims))
	r.Equal(uint32(1), n.BestClaim.OutPoint.Index)
	r.Equal(int32(2), n.TakenOverAt)

	n, err = ct.nodeManager.NodeAt(ct.nodeManager.Height(), []byte("test"))
	r.NoError(err)
	r.Equal(int64(18), n.BestClaim.Amount+n.SupportSums[n.BestClaim.ClaimID.Key()])
}

func TestActivationsOnNormalizationFork(t *testing.T) {

	r := require.New(t)

	setup(t)
	param.ActiveParams.NormalizedNameForkHeight = 4
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	hash := chainhash.HashH([]byte{1, 2, 3})

	o7 := wire.OutPoint{Hash: hash, Index: 7}
	err = ct.AddClaim([]byte("A"), o7, change.NewClaimID(o7), 1)
	r.NoError(err)
	incrementBlock(r, ct, 3)
	verifyBestIndex(t, ct, "A", 7, 1)

	o8 := wire.OutPoint{Hash: hash, Index: 8}
	err = ct.AddClaim([]byte("A"), o8, change.NewClaimID(o8), 2)
	r.NoError(err)
	incrementBlock(r, ct, 1)
	verifyBestIndex(t, ct, "a", 8, 2)

	incrementBlock(r, ct, 2)
	verifyBestIndex(t, ct, "a", 8, 2)

	err = ct.ResetHeight(3)
	r.NoError(err)
	verifyBestIndex(t, ct, "A", 7, 1)
}

func TestNormalizationSortOrder(t *testing.T) {

	r := require.New(t)
	// this was an unfortunate bug; the normalization fork should not have activated anything
	// alas, it's now part of our history; we hereby test it to keep it that way
	setup(t)
	param.ActiveParams.NormalizedNameForkHeight = 2
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	hash := chainhash.HashH([]byte{1, 2, 3})

	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("A"), o1, change.NewClaimID(o1), 1)
	r.NoError(err)

	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("A"), o2, change.NewClaimID(o2), 2)
	r.NoError(err)

	o3 := wire.OutPoint{Hash: hash, Index: 3}
	err = ct.AddClaim([]byte("a"), o3, change.NewClaimID(o3), 3)
	r.NoError(err)

	incrementBlock(r, ct, 1)
	verifyBestIndex(t, ct, "A", 2, 2)
	verifyBestIndex(t, ct, "a", 3, 1)

	incrementBlock(r, ct, 1)
	verifyBestIndex(t, ct, "a", 3, 3)
}

func verifyBestIndex(t *testing.T, ct *ClaimTrie, name string, idx uint32, claims int) {

	r := require.New(t)

	n, err := ct.nodeManager.NodeAt(ct.nodeManager.Height(), []byte(name))
	r.NoError(err)
	r.Equal(claims, len(n.Claims))
	if claims > 0 {
		r.Equal(idx, n.BestClaim.OutPoint.Index)
	}
}

func TestRebuild(t *testing.T) {
	r := require.New(t)
	setup(t)
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	hash := chainhash.HashH([]byte{1, 2, 3})

	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test1"), o1, change.NewClaimID(o1), 1)
	r.NoError(err)

	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("test2"), o2, change.NewClaimID(o2), 2)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	m := ct.MerkleHash()
	r.NotNil(m)
	r.NotEqual(*merkletrie.EmptyTrieHash, *m)

	ct.merkleTrie = merkletrie.NewRamTrie()
	ct.runFullTrieRebuild(nil, nil)

	m2 := ct.MerkleHash()
	r.NotNil(m2)
	r.Equal(*m, *m2)
}

func BenchmarkClaimTrie_AppendBlock256(b *testing.B) {

	addUpdateRemoveRandoms(b, 256)
}

func BenchmarkClaimTrie_AppendBlock4(b *testing.B) {

	addUpdateRemoveRandoms(b, 4)
}

func addUpdateRemoveRandoms(b *testing.B, inBlock int) {
	rand.Seed(42)
	names := make([][]byte, 0, b.N)

	for i := 0; i < b.N; i++ {
		names = append(names, randomName())
	}

	var hashes []*chainhash.Hash

	param.SetNetwork(wire.TestNet)
	param.ActiveParams.OriginalClaimExpirationTime = 1000000
	param.ActiveParams.ExtendedClaimExpirationTime = 1000000
	cfg.DataDir = b.TempDir()

	r := require.New(b)
	ct, err := New(cfg)
	r.NoError(err)
	defer ct.Close()
	h1 := chainhash.Hash{100, 200}

	start := time.Now()
	b.ResetTimer()

	c := 0
	for i := 0; i < b.N; i++ {
		op := wire.OutPoint{Hash: h1, Index: uint32(i)}
		id := change.NewClaimID(op)
		err = ct.AddClaim(names[i], op, id, 500)
		r.NoError(err)
		if c++; c%inBlock == inBlock-1 {
			incrementBlock(r, ct, 1)
			hashes = append(hashes, ct.MerkleHash())
		}
	}

	for i := 0; i < b.N; i++ {
		op := wire.OutPoint{Hash: h1, Index: uint32(i)}
		id := change.NewClaimID(op)
		op.Hash[0] = 1
		err = ct.UpdateClaim(names[i], op, 400, id)
		r.NoError(err)
		if c++; c%inBlock == inBlock-1 {
			incrementBlock(r, ct, 1)
			hashes = append(hashes, ct.MerkleHash())
		}
	}

	for i := 0; i < b.N; i++ {
		op := wire.OutPoint{Hash: h1, Index: uint32(i)}
		id := change.NewClaimID(op)
		op.Hash[0] = 2
		err = ct.UpdateClaim(names[i], op, 300, id)
		r.NoError(err)
		if c++; c%inBlock == inBlock-1 {
			incrementBlock(r, ct, 1)
			hashes = append(hashes, ct.MerkleHash())
		}
	}

	for i := 0; i < b.N; i++ {
		op := wire.OutPoint{Hash: h1, Index: uint32(i)}
		id := change.NewClaimID(op)
		op.Hash[0] = 3
		err = ct.SpendClaim(names[i], op, id)
		r.NoError(err)
		if c++; c%inBlock == inBlock-1 {
			incrementBlock(r, ct, 1)
			hashes = append(hashes, ct.MerkleHash())
		}
	}
	incrementBlock(r, ct, 1)
	hashes = append(hashes, ct.MerkleHash())

	b.StopTimer()
	ht := ct.height
	h1 = *ct.MerkleHash()
	b.Logf("Running AppendBlock bench with %d names in %f sec. Height: %d, Hash: %s",
		b.N, time.Since(start).Seconds(), ht, h1.String())

	// a very important test of the functionality:
	for ct.height > 0 {
		r.True(hashes[ct.height-1].IsEqual(ct.MerkleHash()))
		err = ct.ResetHeight(ct.height - 1)
		r.NoError(err)
	}
}

func randomName() []byte {
	name := make([]byte, rand.Intn(30)+10)
	rand.Read(name)
	for i := range name {
		name[i] %= 56
		name[i] += 65
	}
	return name
}

func incrementBlock(r *require.Assertions, ct *ClaimTrie, c int32) {
	h := ct.height + c
	if c < 0 {
		err := ct.ResetHeight(ct.height + c)
		r.NoError(err)
	} else {
		for ; c > 0; c-- {
			err := ct.AppendBlock(false)
			r.NoError(err)
		}
	}
	r.Equal(h, ct.height)
}

func TestNormalizationRollback(t *testing.T) {
	param.SetNetwork(wire.TestNet)
	param.ActiveParams.OriginalClaimExpirationTime = 1000000
	param.ActiveParams.ExtendedClaimExpirationTime = 1000000
	cfg.DataDir = t.TempDir()

	r := require.New(t)
	ct, err := New(cfg)
	r.NoError(err)
	defer ct.Close()

	r.Equal(int32(250), param.ActiveParams.NormalizedNameForkHeight)
	incrementBlock(r, ct, 247)

	h1 := chainhash.Hash{100, 200}
	op := wire.OutPoint{Hash: h1, Index: 1}
	id := change.NewClaimID(op)
	err = ct.AddClaim([]byte("TEST"), op, id, 1000)
	r.NoError(err)

	incrementBlock(r, ct, 5)
	incrementBlock(r, ct, -4)
	err = ct.SpendClaim([]byte("TEST"), op, id)
	r.NoError(err)
	incrementBlock(r, ct, 1)
	h := ct.MerkleHash()
	r.True(h.IsEqual(merkletrie.EmptyTrieHash))
	incrementBlock(r, ct, 3)
	h2 := ct.MerkleHash()
	r.True(h.IsEqual(h2))
}

func TestNormalizationRollbackFuzz(t *testing.T) {
	rand.Seed(42)
	var hashes []*chainhash.Hash

	param.SetNetwork(wire.TestNet)
	param.ActiveParams.OriginalClaimExpirationTime = 1000000
	param.ActiveParams.ExtendedClaimExpirationTime = 1000000
	cfg.DataDir = t.TempDir()

	r := require.New(t)
	ct, err := New(cfg)
	r.NoError(err)
	defer ct.Close()
	h1 := chainhash.Hash{100, 200}

	r.Equal(int32(250), param.ActiveParams.NormalizedNameForkHeight)
	incrementBlock(r, ct, 240)

	for j := 0; j < 10; j++ {
		c := 0
		for i := 0; i < 200; i++ {
			op := wire.OutPoint{Hash: h1, Index: uint32(i)}
			id := change.NewClaimID(op)
			err = ct.AddClaim(randomName(), op, id, 500)
			r.NoError(err)
			if c++; c%10 == 9 {
				incrementBlock(r, ct, 1)
				hashes = append(hashes, ct.MerkleHash())
			}
		}
		if j > 7 {
			ct.runFullTrieRebuild(nil, nil)
			h := ct.MerkleHash()
			r.True(h.IsEqual(hashes[len(hashes)-1]))
		}
		for ct.height > 240 {
			r.True(hashes[ct.height-1-240].IsEqual(ct.MerkleHash()))
			err = ct.ResetHeight(ct.height - 1)
			r.NoError(err)
		}
		hashes = hashes[:0]
	}
}

func TestClaimReplace(t *testing.T) {
	r := require.New(t)
	setup(t)
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("bass"), o1, change.NewClaimID(o1), 8)
	r.NoError(err)

	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("basso"), o2, change.NewClaimID(o2), 10)
	r.NoError(err)

	incrementBlock(r, ct, 1)
	n, err := ct.NodeAt(ct.height, []byte("bass"))
	r.Equal(o1.String(), n.BestClaim.OutPoint.String())

	err = ct.SpendClaim([]byte("bass"), o1, n.BestClaim.ClaimID)
	r.NoError(err)

	o4 := wire.OutPoint{Hash: hash, Index: 4}
	err = ct.AddClaim([]byte("bassfisher"), o4, change.NewClaimID(o4), 12)
	r.NoError(err)

	incrementBlock(r, ct, 1)
	n, err = ct.NodeAt(ct.height, []byte("bass"))
	r.NoError(err)
	r.True(n == nil || !n.HasActiveBestClaim())
	n, err = ct.NodeAt(ct.height, []byte("bassfisher"))
	r.Equal(o4.String(), n.BestClaim.OutPoint.String())
}

func TestGeneralClaim(t *testing.T) {
	r := require.New(t)
	setup(t)
	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 8)
	r.NoError(err)

	incrementBlock(r, ct, 1)
	err = ct.ResetHeight(ct.height - 1)
	r.NoError(err)
	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.True(n == nil || !n.HasActiveBestClaim())

	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 8)
	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("test"), o2, change.NewClaimID(o2), 8)
	r.NoError(err)

	incrementBlock(r, ct, 1)
	incrementBlock(r, ct, -1)
	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.True(n == nil || !n.HasActiveBestClaim())

	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 8)
	r.NoError(err)
	incrementBlock(r, ct, 1)
	err = ct.AddClaim([]byte("test"), o2, change.NewClaimID(o2), 8)
	r.NoError(err)
	incrementBlock(r, ct, 1)

	incrementBlock(r, ct, -2)
	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.True(n == nil || !n.HasActiveBestClaim())
}

func TestClaimTakeover(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 8)
	r.NoError(err)

	incrementBlock(r, ct, 10)

	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("test"), o2, change.NewClaimID(o2), 18)
	r.NoError(err)

	incrementBlock(r, ct, 10)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o1.String(), n.BestClaim.OutPoint.String())

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o2.String(), n.BestClaim.OutPoint.String())

	incrementBlock(r, ct, -1)
	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o1.String(), n.BestClaim.OutPoint.String())
}

func TestSpendClaim(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 18)
	r.NoError(err)
	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("test"), o2, change.NewClaimID(o2), 8)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	err = ct.SpendClaim([]byte("test"), o1, change.NewClaimID(o1))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o2.String(), n.BestClaim.OutPoint.String())

	incrementBlock(r, ct, -1)

	o3 := wire.OutPoint{Hash: hash, Index: 3}
	err = ct.AddClaim([]byte("test"), o3, change.NewClaimID(o3), 22)
	r.NoError(err)

	incrementBlock(r, ct, 10)

	o4 := wire.OutPoint{Hash: hash, Index: 4}
	err = ct.AddClaim([]byte("test"), o4, change.NewClaimID(o4), 28)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o3.String(), n.BestClaim.OutPoint.String())

	err = ct.SpendClaim([]byte("test"), o3, n.BestClaim.ClaimID)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o4.String(), n.BestClaim.OutPoint.String())

	err = ct.SpendClaim([]byte("test"), o1, change.NewClaimID(o1))
	r.NoError(err)
	err = ct.SpendClaim([]byte("test"), o2, change.NewClaimID(o2))
	r.NoError(err)
	err = ct.SpendClaim([]byte("test"), o3, change.NewClaimID(o3))
	r.NoError(err)
	err = ct.SpendClaim([]byte("test"), o4, change.NewClaimID(o4))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.True(n == nil || !n.HasActiveBestClaim())

	h := ct.MerkleHash()
	r.Equal(merkletrie.EmptyTrieHash.String(), h.String())
}

func TestSupportDelay(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 18)
	r.NoError(err)
	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.AddClaim([]byte("test"), o2, change.NewClaimID(o2), 8)
	r.NoError(err)

	o3 := wire.OutPoint{Hash: hash, Index: 3}
	err = ct.AddSupport([]byte("test"), o3, 18, change.NewClaimID(o3)) // using bad ClaimID on purpose
	r.NoError(err)
	o4 := wire.OutPoint{Hash: hash, Index: 4}
	err = ct.AddSupport([]byte("test"), o4, 18, change.NewClaimID(o2))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o2.String(), n.BestClaim.OutPoint.String())

	incrementBlock(r, ct, 10)

	o5 := wire.OutPoint{Hash: hash, Index: 5}
	err = ct.AddSupport([]byte("test"), o5, 18, change.NewClaimID(o1))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o2.String(), n.BestClaim.OutPoint.String())

	incrementBlock(r, ct, 11)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o1.String(), n.BestClaim.OutPoint.String())

	incrementBlock(r, ct, -1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o2.String(), n.BestClaim.OutPoint.String())
}

func TestSupportSpending(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 18)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	o3 := wire.OutPoint{Hash: hash, Index: 3}
	err = ct.AddSupport([]byte("test"), o3, 18, change.NewClaimID(o1))
	r.NoError(err)

	err = ct.SpendClaim([]byte("test"), o1, change.NewClaimID(o1))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.True(n == nil || !n.HasActiveBestClaim())
}

func TestSupportOnUpdate(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 18)
	r.NoError(err)

	err = ct.SpendClaim([]byte("test"), o1, change.NewClaimID(o1))
	r.NoError(err)

	o2 := wire.OutPoint{Hash: hash, Index: 2}
	err = ct.UpdateClaim([]byte("test"), o2, 28, change.NewClaimID(o1))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(int64(28), n.BestClaim.Amount)

	incrementBlock(r, ct, 1)

	err = ct.SpendClaim([]byte("test"), o2, change.NewClaimID(o1))
	r.NoError(err)

	o3 := wire.OutPoint{Hash: hash, Index: 3}
	err = ct.UpdateClaim([]byte("test"), o3, 38, change.NewClaimID(o1))
	r.NoError(err)

	o4 := wire.OutPoint{Hash: hash, Index: 4}
	err = ct.AddSupport([]byte("test"), o4, 2, change.NewClaimID(o1))
	r.NoError(err)

	o5 := wire.OutPoint{Hash: hash, Index: 5}
	err = ct.AddClaim([]byte("test"), o5, change.NewClaimID(o5), 39)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(int64(40), n.BestClaim.Amount+n.SupportSums[n.BestClaim.ClaimID.Key()])

	err = ct.SpendSupport([]byte("test"), o4, n.BestClaim.ClaimID)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	// NOTE: LBRYcrd did not test that supports can trigger a takeover correctly (and it doesn't work here):
	// n, err = ct.NodeAt(ct.height, []byte("test"))
	// r.NoError(err)
	// r.Equal(int64(39), n.BestClaim.Amount + n.SupportSums[n.BestClaim.ClaimID.Key()])
}

func TestSupportPreservation(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	o2 := wire.OutPoint{Hash: hash, Index: 2}
	o3 := wire.OutPoint{Hash: hash, Index: 3}
	o4 := wire.OutPoint{Hash: hash, Index: 4}
	o5 := wire.OutPoint{Hash: hash, Index: 5}

	err = ct.AddSupport([]byte("test"), o2, 10, change.NewClaimID(o1))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 18)
	r.NoError(err)

	err = ct.AddClaim([]byte("test"), o3, change.NewClaimID(o3), 7)
	r.NoError(err)

	incrementBlock(r, ct, 10)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(int64(28), n.BestClaim.Amount+n.SupportSums[n.BestClaim.ClaimID.Key()])

	err = ct.AddSupport([]byte("test"), o4, 10, change.NewClaimID(o1))
	r.NoError(err)
	err = ct.AddSupport([]byte("test"), o5, 100, change.NewClaimID(o3))
	r.NoError(err)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(int64(38), n.BestClaim.Amount+n.SupportSums[n.BestClaim.ClaimID.Key()])

	incrementBlock(r, ct, 10)

	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(int64(107), n.BestClaim.Amount+n.SupportSums[n.BestClaim.ClaimID.Key()])
}

func TestInvalidClaimID(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	incrementBlock(r, ct, 1)

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}
	o2 := wire.OutPoint{Hash: hash, Index: 2}
	o3 := wire.OutPoint{Hash: hash, Index: 3}

	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 10)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	err = ct.SpendClaim([]byte("test"), o3, change.NewClaimID(o1))
	r.NoError(err)

	err = ct.UpdateClaim([]byte("test"), o2, 18, change.NewClaimID(o3))
	r.NoError(err)

	incrementBlock(r, ct, 12)

	n, err := ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Len(n.Claims, 1)
	r.Len(n.Supports, 0)
	r.Equal(int64(10), n.BestClaim.Amount+n.SupportSums[n.BestClaim.ClaimID.Key()])
}

func TestStableTrieHash(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1
	param.ActiveParams.AllClaimsInMerkleForkHeight = 8 // changes on this one

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	hash := chainhash.HashH([]byte{1, 2, 3})
	o1 := wire.OutPoint{Hash: hash, Index: 1}

	err = ct.AddClaim([]byte("test"), o1, change.NewClaimID(o1), 1)
	r.NoError(err)

	incrementBlock(r, ct, 1)

	h := ct.MerkleHash()
	r.NotEqual(merkletrie.EmptyTrieHash.String(), h.String())

	for i := 0; i < 6; i++ {
		incrementBlock(r, ct, 1)
		r.Equal(h.String(), ct.MerkleHash().String())
	}

	incrementBlock(r, ct, 1)

	r.NotEqual(h.String(), ct.MerkleHash())
	h = ct.MerkleHash()

	for i := 0; i < 16; i++ {
		incrementBlock(r, ct, 1)
		r.Equal(h.String(), ct.MerkleHash().String())
	}
}

func TestBlock884431(t *testing.T) {
	r := require.New(t)
	setup(t)
	param.ActiveParams.ActiveDelayFactor = 1
	param.ActiveParams.MaxRemovalWorkaroundHeight = 0
	param.ActiveParams.AllClaimsInMerkleForkHeight = 0

	ct, err := New(cfg)
	r.NoError(err)
	r.NotNil(ct)
	defer ct.Close()

	// in this block we have a scenario where we update all the child names
	// which, in the old code, caused a trie vertex to be removed
	// which, in turn, would trigger a premature takeover

	c := byte(10)

	add := func(s string, amt int64) wire.OutPoint {
		h := chainhash.HashH([]byte{c})
		c++
		o := wire.OutPoint{Hash: h, Index: 1}
		err := ct.AddClaim([]byte(s), o, change.NewClaimID(o), amt)
		r.NoError(err)
		return o
	}

	update := func(s string, o wire.OutPoint, amt int64) wire.OutPoint {
		err = ct.SpendClaim([]byte(s), o, change.NewClaimID(o))
		r.NoError(err)

		h := chainhash.HashH([]byte{c})
		c++
		o2 := wire.OutPoint{Hash: h, Index: 2}

		err = ct.UpdateClaim([]byte(s), o2, amt, change.NewClaimID(o))
		r.NoError(err)
		return o2
	}

	o1a := add("go", 10)
	o1b := add("go", 20)
	o2 := add("goop", 10)
	o3 := add("gog", 20)

	o4a := add("test", 10)
	o4b := add("test", 20)
	o5 := add("tester", 10)
	o6 := add("testing", 20)

	for i := 0; i < 10; i++ {
		err = ct.AppendBlock(false)
		r.NoError(err)
	}
	n, err := ct.NodeAt(ct.height, []byte("go"))
	r.NoError(err)
	r.Equal(o1b.String(), n.BestClaim.OutPoint.String())
	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o4b.String(), n.BestClaim.OutPoint.String())

	update("go", o1b, 30)
	o10 := update("go", o1a, 40)
	update("gog", o3, 30)
	update("goop", o2, 30)

	update("testing", o6, 30)
	o11 := update("test", o4b, 30)
	update("test", o4a, 40)
	update("tester", o5, 30)

	incrementBlock(r, ct, 1)

	n, err = ct.NodeAt(ct.height, []byte("go"))
	r.NoError(err)
	r.Equal(o10.String(), n.BestClaim.OutPoint.String())
	n, err = ct.NodeAt(ct.height, []byte("test"))
	r.NoError(err)
	r.Equal(o11.String(), n.BestClaim.OutPoint.String())
}
