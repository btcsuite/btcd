package merkletrie

import (
	"github.com/lbryio/lbcd/chaincfg/chainhash"
)

type KeyType []byte

type collapsedVertex struct {
	children   []*collapsedVertex
	key        KeyType
	merkleHash *chainhash.Hash
	claimHash  *chainhash.Hash
}

// insertAt inserts v into s at index i and returns the new slice.
// https://stackoverflow.com/questions/42746972/golang-insert-to-a-sorted-slice
func insertAt(data []*collapsedVertex, i int, v *collapsedVertex) []*collapsedVertex {
	if i == len(data) {
		// Insert at end is the easy case.
		return append(data, v)
	}

	// Make space for the inserted element by shifting
	// values at the insertion index up one index. The call
	// to append does not allocate memory when cap(data) is
	// greater than len(data).
	data = append(data[:i+1], data[i:]...)
	data[i] = v
	return data
}

func (ptn *collapsedVertex) Insert(value *collapsedVertex) *collapsedVertex {
	// keep it sorted (and sort.Sort is too slow)
	index := sortSearch(ptn.children, value.key[0])
	ptn.children = insertAt(ptn.children, index, value)

	return value
}

// this sort.Search is stolen shamelessly from search.go,
// and modified for performance to not need a closure
func sortSearch(nodes []*collapsedVertex, b byte) int {
	i, j := 0, len(nodes)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h
		// i ≤ h < j
		if nodes[h].key[0] < b {
			i = h + 1 // preserves f(i-1) == false
		} else {
			j = h // preserves f(j) == true
		}
	}
	// i == j, f(i-1) == false, and f(j) (= f(i)) == true  =>  answer is i.
	return i
}

func (ptn *collapsedVertex) findNearest(key KeyType) (int, *collapsedVertex) {
	// none of the children overlap on the first char or we would have a parent node with that char
	index := sortSearch(ptn.children, key[0])
	hits := ptn.children[index:]
	if len(hits) > 0 {
		return index, hits[0]
	}
	return -1, nil
}

type collapsedTrie struct {
	Root  *collapsedVertex
	Nodes int
}

func NewCollapsedTrie() *collapsedTrie {
	// we never delete the Root node
	return &collapsedTrie{Root: &collapsedVertex{key: make(KeyType, 0)}, Nodes: 1}
}

func (pt *collapsedTrie) NodeCount() int {
	return pt.Nodes
}

func matchLength(a, b KeyType) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return minLen
}

func (pt *collapsedTrie) insert(value KeyType, node *collapsedVertex) (bool, *collapsedVertex) {
	index, child := node.findNearest(value)
	match := 0
	if index >= 0 { // if we found a child
		child.merkleHash = nil
		match = matchLength(value, child.key)
		if len(value) == match && len(child.key) == match {
			return false, child
		}
	}
	if match <= 0 {
		pt.Nodes++
		return true, node.Insert(&collapsedVertex{key: value})
	}
	if match < len(child.key) {
		grandChild := collapsedVertex{key: child.key[match:], children: child.children,
			claimHash: child.claimHash, merkleHash: child.merkleHash}
		newChild := collapsedVertex{key: child.key[0:match], children: []*collapsedVertex{&grandChild}}
		child = &newChild
		node.children[index] = child
		pt.Nodes++
		if len(value) == match {
			return true, child
		}
	}
	return pt.insert(value[match:], child)
}

func (pt *collapsedTrie) InsertOrFind(value KeyType) (bool, *collapsedVertex) {
	pt.Root.merkleHash = nil
	if len(value) <= 0 {
		return false, pt.Root
	}

	// we store the name so we need to make our own copy of it
	// this avoids errors where this function is called via the DB iterator
	v2 := make([]byte, len(value))
	copy(v2, value)
	return pt.insert(v2, pt.Root)
}

func find(value KeyType, node *collapsedVertex, pathIndexes *[]int, path *[]*collapsedVertex) *collapsedVertex {
	index, child := node.findNearest(value)
	if index < 0 {
		return nil
	}
	match := matchLength(value, child.key)
	if len(value) == match && len(child.key) == match {
		if pathIndexes != nil {
			*pathIndexes = append(*pathIndexes, index)
		}
		if path != nil {
			*path = append(*path, child)
		}
		return child
	}
	if match < len(child.key) || match == len(value) {
		return nil
	}
	if pathIndexes != nil {
		*pathIndexes = append(*pathIndexes, index)
	}
	if path != nil {
		*path = append(*path, child)
	}
	return find(value[match:], child, pathIndexes, path)
}

func (pt *collapsedTrie) Find(value KeyType) *collapsedVertex {
	if len(value) <= 0 {
		return pt.Root
	}
	return find(value, pt.Root, nil, nil)
}

func (pt *collapsedTrie) FindPath(value KeyType) ([]int, []*collapsedVertex) {
	pathIndexes := []int{-1}
	path := []*collapsedVertex{pt.Root}
	if len(value) > 0 {
		result := find(value, pt.Root, &pathIndexes, &path)
		if result == nil { // not sure I want this line
			return nil, nil
		}
	}
	return pathIndexes, path
}

// IterateFrom can be used to find a value and run a function on that value.
// If the handler returns true it continues to iterate through the children of value.
func (pt *collapsedTrie) IterateFrom(start KeyType, handler func(name KeyType, value *collapsedVertex) bool) {
	node := find(start, pt.Root, nil, nil)
	if node == nil {
		return
	}
	iterateFrom(start, node, handler)
}

func iterateFrom(name KeyType, node *collapsedVertex, handler func(name KeyType, value *collapsedVertex) bool) {
	for handler(name, node) {
		for _, child := range node.children {
			iterateFrom(append(name, child.key...), child, handler)
		}
	}
}

func (pt *collapsedTrie) Erase(value KeyType) bool {
	indexes, path := pt.FindPath(value)
	if path == nil || len(path) <= 1 {
		if len(path) == 1 {
			path[0].merkleHash = nil
			path[0].claimHash = nil
		}
		return false
	}
	nodes := pt.Nodes
	i := len(path) - 1
	path[i].claimHash = nil // this is the thing we are erasing; the rest is book-keeping
	for ; i > 0; i-- {
		childCount := len(path[i].children)
		noClaimData := path[i].claimHash == nil
		path[i].merkleHash = nil
		if childCount == 1 && noClaimData {
			path[i].key = append(path[i].key, path[i].children[0].key...)
			path[i].claimHash = path[i].children[0].claimHash
			path[i].children = path[i].children[0].children
			pt.Nodes--
			continue
		}
		if childCount == 0 && noClaimData {
			index := indexes[i]
			path[i-1].children = append(path[i-1].children[:index], path[i-1].children[index+1:]...)
			pt.Nodes--
			continue
		}
		break
	}
	for ; i >= 0; i-- {
		path[i].merkleHash = nil
	}
	return nodes > pt.Nodes
}
