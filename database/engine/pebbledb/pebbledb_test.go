package pebbledb

import (
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/database/engine"
	"github.com/stretchr/testify/require"
)

func TestSuitePebbleDB(t *testing.T) {
	engine.TestSuiteEngine(t, func() engine.Engine {
		dbPath := filepath.Join(t.TempDir(), "pebbledb-testsuite")

		pebbledb, err := NewDB(dbPath, true, 0, 0)
		require.NoErrorf(t, err, "failed to create pebbledb")
		return pebbledb
	})
}
