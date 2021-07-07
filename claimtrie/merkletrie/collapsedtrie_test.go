package merkletrie

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func b(value string) []byte      { return []byte(value) }
func eq(x []byte, y string) bool { return bytes.Equal(x, b(y)) }

func TestInsertAndErase(t *testing.T) {
	trie := NewCollapsedTrie()
	assert.True(t, trie.NodeCount() == 1)
	inserted, node := trie.InsertOrFind(b("abc"))
	assert.True(t, inserted)
	assert.NotNil(t, node)
	assert.Equal(t, 2, trie.NodeCount())
	inserted, node = trie.InsertOrFind(b("abd"))
	assert.True(t, inserted)
	assert.Equal(t, 4, trie.NodeCount())
	assert.NotNil(t, node)
	hit := trie.Find(b("ab"))
	assert.True(t, eq(hit.key, "ab"))
	assert.Equal(t, 2, len(hit.children))
	hit = trie.Find(b("abc"))
	assert.True(t, eq(hit.key, "c"))
	hit = trie.Find(b("abd"))
	assert.True(t, eq(hit.key, "d"))
	hit = trie.Find(b("a"))
	assert.Nil(t, hit)
	indexes, path := trie.FindPath(b("abd"))
	assert.Equal(t, 3, len(indexes))
	assert.True(t, eq(path[1].key, "ab"))
	erased := trie.Erase(b("ab"))
	assert.False(t, erased)
	assert.Equal(t, 4, trie.NodeCount())
	erased = trie.Erase(b("abc"))
	assert.True(t, erased)
	assert.Equal(t, 2, trie.NodeCount())
	erased = trie.Erase(b("abd"))
	assert.True(t, erased)
	assert.Equal(t, 1, trie.NodeCount())
}

func TestNilNameHandling(t *testing.T) {
	trie := NewCollapsedTrie()
	inserted, n := trie.InsertOrFind([]byte("test"))
	assert.True(t, inserted)
	n.claimHash = EmptyTrieHash
	inserted, n = trie.InsertOrFind(nil)
	assert.False(t, inserted)
	n.claimHash = EmptyTrieHash
	n.merkleHash = EmptyTrieHash
	inserted, n = trie.InsertOrFind(nil)
	assert.False(t, inserted)
	assert.NotNil(t, n.claimHash)
	assert.Nil(t, n.merkleHash)
	nodeRemoved := trie.Erase(nil)
	assert.False(t, nodeRemoved)
	inserted, n = trie.InsertOrFind(nil)
	assert.False(t, inserted)
	assert.Nil(t, n.claimHash)
}

func TestCollapsedTriePerformance(t *testing.T) {
	inserts := 100000 // increase this to 1M for more interesting results
	data := make([][]byte, inserts)
	rand.Seed(42)
	for i := 0; i < inserts; i++ {
		size := rand.Intn(70) + 4
		data[i] = make([]byte, size)
		rand.Read(data[i])
		for j := 0; j < size; j++ {
			data[i][j] %= byte(62) // shrink the range to match the old test
		}
	}

	trie := NewCollapsedTrie()
	// doing my own timing because I couldn't get the B.Run method to work:
	start := time.Now()
	for i := 0; i < inserts; i++ {
		_, node := trie.InsertOrFind(data[i])
		assert.NotNil(t, node, "Failure at %d of %d", i, inserts)
	}
	t.Logf("Insertion in %f sec.", time.Since(start).Seconds())

	start = time.Now()
	for i := 0; i < inserts; i++ {
		node := trie.Find(data[i])
		assert.True(t, bytes.HasSuffix(data[i], node.key), "Failure on %d of %d", i, inserts)
	}
	t.Logf("Lookup in %f sec. on %d nodes.", time.Since(start).Seconds(), trie.NodeCount())

	start = time.Now()
	for i := 0; i < inserts; i++ {
		indexes, path := trie.FindPath(data[i])
		assert.True(t, len(indexes) == len(path))
		assert.True(t, len(path) > 1)
		assert.True(t, bytes.HasSuffix(data[i], path[len(path)-1].key))
	}
	t.Logf("Parents in %f sec.", time.Since(start).Seconds())

	start = time.Now()
	for i := 0; i < inserts; i++ {
		trie.Erase(data[i])
	}
	t.Logf("Deletion in %f sec.", time.Since(start).Seconds())
	assert.Equal(t, 1, trie.NodeCount())
}
