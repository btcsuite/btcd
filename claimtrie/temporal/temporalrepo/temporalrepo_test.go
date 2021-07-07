package temporalrepo

import (
	"testing"

	"github.com/lbryio/lbcd/claimtrie/temporal"

	"github.com/stretchr/testify/require"
)

func TestMemory(t *testing.T) {

	repo := NewMemory()
	testTemporalRepo(t, repo)
}

func TestPebble(t *testing.T) {

	repo, err := NewPebble(t.TempDir())
	require.NoError(t, err)

	testTemporalRepo(t, repo)
}

func testTemporalRepo(t *testing.T, repo temporal.Repo) {

	r := require.New(t)

	nameA := []byte("a")
	nameB := []byte("b")
	nameC := []byte("c")

	testcases := []struct {
		name    []byte
		heights []int32
	}{
		{nameA, []int32{1, 3, 2}},
		{nameA, []int32{2, 3}},
		{nameB, []int32{5, 4}},
		{nameB, []int32{5, 1}},
		{nameC, []int32{4, 3, 8}},
	}

	for _, i := range testcases {
		names := make([][]byte, 0, len(i.heights))
		for range i.heights {
			names = append(names, i.name)
		}
		err := repo.SetNodesAt(names, i.heights)
		r.NoError(err)
	}

	// a: 1, 2, 3
	// b: 1, 5, 4
	// c: 4, 3, 8

	names, err := repo.NodesAt(2)
	r.NoError(err)
	r.ElementsMatch([][]byte{nameA}, names)

	names, err = repo.NodesAt(5)
	r.NoError(err)
	r.ElementsMatch([][]byte{nameB}, names)

	names, err = repo.NodesAt(8)
	r.NoError(err)
	r.ElementsMatch([][]byte{nameC}, names)

	names, err = repo.NodesAt(1)
	r.NoError(err)
	r.ElementsMatch([][]byte{nameA, nameB}, names)

	names, err = repo.NodesAt(4)
	r.NoError(err)
	r.ElementsMatch([][]byte{nameB, nameC}, names)

	names, err = repo.NodesAt(3)
	r.NoError(err)
	r.ElementsMatch([][]byte{nameA, nameC}, names)
}
