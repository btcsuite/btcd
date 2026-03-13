package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSuiteEngine(t *testing.T, new func() Engine) {
	t.Run("TransactionSnapshot", func(t *testing.T) {
		engine := new()
		defer engine.Close()

		// Create new transaction
		tx, err := engine.Transaction()
		require.NoErrorf(t, err, "failed to create transaction")

		// Put some data into the transaction
		key := []byte("key1")
		value := []byte("value1")
		err = tx.Put(key, value)
		require.NoErrorf(t, err, "failed to put data into transaction")

		// Create a snapshot and find empty data
		snapshot, err := engine.Snapshot()
		require.NoErrorf(t, err, "failed to create snapshot")

		has, err := snapshot.Has(key)
		require.NoErrorf(t, err, "failed to check if key exists in snapshot")
		require.Falsef(t, has, "expected key to not exist in snapshot")

		gotValue, err := snapshot.Get(key)
		require.Errorf(t, err, "expected to get error when getting value from snapshot")
		require.Nil(t, gotValue, "expected to get nil value from snapshot")
		snapshot.Release()

		// Commit the transaction
		err = tx.Commit()
		require.NoErrorf(t, err, "failed to commit transaction")

		// Create a snapshot and verify the data
		snapshot, err = engine.Snapshot()
		require.NoErrorf(t, err, "failed to create snapshot")

		gotValue, err = snapshot.Get(key)
		require.NoErrorf(t, err, "failed to get value from snapshot")
		require.Equalf(t, value, gotValue, "snapshot value mismatch")
		snapshot.Release()
	})

	t.Run("TransactionIterator", func(t *testing.T) {
		for _, test := range []struct {
			kvs       map[string]string // random order of key-value pairs
			ranges    *Range
			expectkvs [][2]string
		}{
			{
				kvs:       map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
				ranges:    &Range{Start: []byte("key0"), Limit: []byte("key1")},
				expectkvs: nil,
			},
			{
				kvs:       map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
				ranges:    &Range{Start: []byte("key0"), Limit: []byte("key2")},
				expectkvs: [][2]string{{"key1", "value1"}},
			},
			{
				kvs:       map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
				ranges:    &Range{Start: []byte("key1"), Limit: []byte("key3")},
				expectkvs: [][2]string{{"key1", "value1"}, {"key2", "value2"}},
			},
			{
				kvs:       map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
				ranges:    &Range{Start: []byte("key10"), Limit: []byte("key30")},
				expectkvs: [][2]string{{"key2", "value2"}, {"key3", "value3"}},
			},
			{
				kvs:       map[string]string{"key1": "value1", "key2": "value2", "key3": "value3"},
				ranges:    &Range{Start: []byte("key2"), Limit: []byte("key2")},
				expectkvs: nil,
			},
			{
				kvs:       map[string]string{"key10": "value10", "key11": "value11", "key20": "value20", "key21": "value21"},
				ranges:    BytesPrefix([]byte("key1")),
				expectkvs: [][2]string{{"key10", "value10"}, {"key11", "value11"}},
			},
		} {
			engine := new()
			defer engine.Close()

			// Create new transaction
			tx, err := engine.Transaction()
			require.NoErrorf(t, err, "failed to create transaction")

			// Put some data into the transaction
			for k, v := range test.kvs {
				err = tx.Put([]byte(k), []byte(v))
				require.NoErrorf(t, err, "failed to put data into transaction")
			}
			// Commit the transaction
			err = tx.Commit()
			require.NoErrorf(t, err, "failed to commit transaction")

			// Iterate over the data
			snapshot, err := engine.Snapshot()
			require.NoErrorf(t, err, "failed to create snapshot")

			iter := snapshot.NewIterator(test.ranges)
			var idx int
			for iter.Next() {
				if idx >= len(test.expectkvs) {
					require.FailNowf(t, "unexpected key-value pair", "key: %s, value: %s", iter.Key(), iter.Value())
				}

				require.Equalf(t, []byte(test.expectkvs[idx][0]), iter.Key(), "key mismatch")
				require.Equalf(t, []byte(test.expectkvs[idx][1]), iter.Value(), "value mismatch")
				idx++
			}
			require.Equalf(t, len(test.expectkvs), idx, "key-value pair count mismatch")

			iter.Release()
			snapshot.Release()
		}
	})

	t.Run("DbClose", func(t *testing.T) {
		engine := new()

		// release
		transaction, err := engine.Transaction()
		require.NoErrorf(t, err, "failed to create transaction")

		transaction.Discard()
		transaction.Discard() // multiple calls to discard should be safe
		err = transaction.Commit()
		require.Errorf(t, err, "expected to get error when committing discarded transaction")

		snapshot, err := engine.Snapshot()
		require.NoErrorf(t, err, "failed to create snapshot")

		iterator := snapshot.NewIterator(&Range{})
		require.NoErrorf(t, iterator.Error(), "failed to create iterator")
		iterator.Release()
		iterator.Release() // multiple calls to release should be safe

		snapshot.Release()
		snapshot.Release() // multiple calls to release should be safe
		_, err = snapshot.Get([]byte("key"))
		require.Errorf(t, err, "expected to get error when getting value from released snapshot")

		err = engine.Close()
		require.NoErrorf(t, err, "failed to close engine")

		// Ensure that the engine is closed
		err = engine.Close()
		require.Errorf(t, err, "expected to get error when closing closed engine")

		// Get a transaction from a closed engine
		_, err = engine.Transaction()
		require.Errorf(t, err, "expected to get error when creating transaction from closed engine")

		// Get a snapshot from a closed engine
		_, err = engine.Snapshot()
		require.Errorf(t, err, "expected to get error when creating snapshot from closed engine")
	})

}
