// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package sqlite3_test

import (
	"bytes"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcwire"
	"os"
	"testing"
)

// array of shas
var testShas []btcwire.ShaHash = []btcwire.ShaHash{
	{
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
	},
	{
		0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22,
		0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22,
		0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22,
		0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22,
	},
	{
		0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33,
		0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33,
		0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33,
		0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33,
	},
	{
		0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44,
		0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44,
		0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44,
		0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44, 0x44,
	},
	{
		0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
		0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
		0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
		0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55,
	},
}

// Work around stupid go vet bug where any non array should have named
// initializers. Since ShaHash is a glorified array it shouldn't matter.
var badShaArray = [32]byte{
	0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
	0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
	0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
	0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
}
var badSha btcwire.ShaHash = btcwire.ShaHash(badShaArray)
var zeroSha = btcwire.ShaHash{}
var zeroBlock []byte = make([]byte, 32)

func compareArray(t *testing.T, one, two []btcwire.ShaHash, test string,
	sync string) {
	if len(one) != len(two) {
		t.Errorf("%s: lengths don't match for arrays (%s)", test, sync)
		return
	}

	for i := range one {
		if !one[i].IsEqual(&two[i]) {
			t.Errorf("%s: %dth sha doesn't match (%s)", test, i,
				sync)
		}
	}
}

func testNewestSha(t *testing.T, db btcdb.Db, expSha btcwire.ShaHash,
	expBlk int64, situation string) {

	newestsha, blkid, err := db.NewestSha()
	if err != nil {
		t.Errorf("NewestSha failed %v (%s)", err, situation)
		return
	}
	if blkid != expBlk {
		t.Errorf("NewestSha blkid is %d not %d (%s)", blkid, expBlk,
			situation)
	}
	if !newestsha.IsEqual(&expSha) {
		t.Errorf("Newestsha isn't the last sha we inserted %v %v (%s)",
			newestsha, &expSha, situation)
	}
}

type fetchIdxTest struct {
	start int64
	end   int64
	exp   []btcwire.ShaHash
	test  string
}

func testFetch(t *testing.T, db btcdb.Db, shas []btcwire.ShaHash,
	sync string) {

	// Test the newest sha is what we expect and call it twice to ensure
	// caching is working working properly.
	numShas := int64(len(shas))
	newestSha := shas[numShas-1]
	newestBlockID := int64(numShas)
	testNewestSha(t, db, newestSha, newestBlockID, sync)
	testNewestSha(t, db, newestSha, newestBlockID, sync+" cached")

	for i, sha := range shas {
		// Add one for genesis block skew.
		i = i + 1

		// Ensure the sha exists in the db as expected.
		if !db.ExistsSha(&sha) {
			t.Errorf("testSha %d doesn't exists (%s)", i, sync)
			break
		}

		// Fetch the sha from the db and ensure all fields are expected
		// values.
		buf, pver, idx, err := sqlite3.FetchSha(db, &sha)
		if err != nil {
			t.Errorf("Failed to fetch testSha %d (%s)", i, sync)
		}
		if !bytes.Equal(zeroBlock, buf) {
			t.Errorf("testSha %d incorrect block return (%s)", i,
				sync)
		}
		if pver != 1 {
			t.Errorf("pver is %d and not 1 for testSha %d (%s)",
				pver, i, sync)
		}
		if idx != int64(i) {
			t.Errorf("index isn't as expected %d vs %d (%s)",
				idx, i, sync)
		}

		// Fetch the sha by index and ensure it matches.
		tsha, err := db.FetchBlockShaByHeight(int64(i))
		if err != nil {
			t.Errorf("can't fetch sha at index %d: %v", i, err)
			continue
		}
		if !tsha.IsEqual(&sha) {
			t.Errorf("sha for index %d isn't shas[%d]", i, i)
		}
	}

	endBlockID := numShas + 1
	midBlockID := endBlockID / 2
	fetchIdxTests := []fetchIdxTest{
		// All shas.
		{1, btcdb.AllShas, shas, "fetch all shas"},

		//// All shas using known bounds.
		{1, endBlockID, shas, "fetch all shas2"},

		// Partial list starting at beginning.
		{1, midBlockID, shas[:midBlockID-1], "fetch first half"},

		// Partial list ending at end.
		{midBlockID, endBlockID, shas[midBlockID-1 : endBlockID-1],
			"fetch second half"},

		// Nonexistent off the end.
		{endBlockID, endBlockID * 2, []btcwire.ShaHash{},
			"fetch nonexistent"},
	}

	for _, test := range fetchIdxTests {
		t.Logf("numSha: %d - Fetch from %d to %d\n", numShas, test.start, test.end)
		if shalist, err := db.FetchHeightRange(test.start, test.end); err == nil {
			compareArray(t, shalist, test.exp, test.test, sync)
		} else {
			t.Errorf("failed to fetch index range for %s (%s)",
				test.test, sync)
		}
	}

	// Try and fetch nonexistent sha.
	if db.ExistsSha(&badSha) {
		t.Errorf("nonexistent sha exists (%s)!", sync)
	}
	_, _, _, err := sqlite3.FetchSha(db, &badSha)
	if err == nil {
		t.Errorf("Success when fetching a bad sha! (%s)", sync)
	}
	// XXX if not check to see it is the right value?

	testIterator(t, db, shas, sync)
}

func testIterator(t *testing.T, db btcdb.Db, shas []btcwire.ShaHash,
	sync string) {

	// Iterate over the whole list of shas.
	iter, err := db.NewIterateBlocks()
	if err != nil {
		t.Errorf("failed to create iterated blocks")
		return
	}

	// Skip the genesis block.
	_ = iter.NextRow()

	i := 0
	for ; iter.NextRow(); i++ {
		key, pver, buf, err := iter.Row()
		if err != nil {
			t.Errorf("iter.NextRow() failed: %v (%s)", err, sync)
			break
		}
		if i >= len(shas) {
			t.Errorf("iterator returned more shas than "+
				"expected - %d (%s)", i, sync)
			break
		}
		if !key.IsEqual(&shas[i]) {
			t.Errorf("iterator test: %dth sha doesn't match (%s)",
				i, sync)
		}
		if !bytes.Equal(zeroBlock, buf) {
			t.Errorf("iterator test: %d buf incorrect (%s)", i,
				sync)
		}
		if pver != 1 {
			t.Errorf("iterator: %dth pver is %d and not 1 (%s)",
				i, pver, sync)
		}
	}
	if i < len(shas) {
		t.Errorf("iterator got no rows on %dth loop, should have %d "+
			"(%s)", i, len(shas), sync)
	}
	if _, _, _, err = iter.Row(); err == nil {
		t.Errorf("done iterator didn't return failure")
	}
	iter.Close()
}

func TestBdb(t *testing.T) {
	// Ignore db remove errors since it means we didn't have an old one.
	_ = os.Remove("tstdb1")
	db, err := btcdb.CreateDB("sqlite", "tstdb1")
	if err != nil {
		t.Errorf("Failed to open test database %v", err)
		return
	}
	defer os.Remove("tstdb1")

	for i := range testShas {
		var previous btcwire.ShaHash
		if i == 0 {
			previous = btcwire.GenesisHash
		} else {
			previous = testShas[i-1]
		}
		_, err := db.InsertBlockData(&testShas[i], &previous, 1, zeroBlock)
		if err != nil {
			t.Errorf("Failed to insert testSha %d.  Error: %v",
				i, err)
			return
		}

		testFetch(t, db, testShas[0:i+1], "pre sync ")
	}

	// XXX insert enough so that we hit the transaction limit
	// XXX try and insert a with a bad previous

	db.Sync()

	testFetch(t, db, testShas, "post sync")

	for i := len(testShas) - 1; i >= 0; i-- {
		err := db.DropAfterBlockBySha(&testShas[i])
		if err != nil {
			t.Errorf("drop after %d failed %v", i, err)
			break
		}
		testFetch(t, db, testShas[:i+1],
			fmt.Sprintf("post DropAfter for sha %d", i))
	}

	// Just tests that it doesn't crash, no return value
	db.Close()
}
