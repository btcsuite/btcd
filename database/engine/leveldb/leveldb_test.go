package leveldb

import (
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/database/engine"
	"github.com/stretchr/testify/require"
)

func TestSuiteLevelDB(t *testing.T) {
	engine.TestSuiteEngine(t, func() engine.Engine {
		dbPath := filepath.Join(t.TempDir(), "leveldb-testsuite")

		leveldb, err := NewDB(dbPath, true)
		require.NoErrorf(t, err, "failed to create leveldb")
		return leveldb
	})
}
