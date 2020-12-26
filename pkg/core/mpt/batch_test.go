package mpt

import (
	"encoding/hex"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/storage"
	"github.com/stretchr/testify/require"
)

func TestBatchAdd(t *testing.T) {
	b := new(Batch)
	b.Add([]byte{1}, []byte{2})
	b.Add([]byte{2, 16}, []byte{3})
	b.Add([]byte{2, 0}, []byte{4})
	b.Add([]byte{0, 1}, []byte{5})
	b.Add([]byte{2, 0}, []byte{6})
	expected := []keyValue{
		{[]byte{0, 0, 0, 1}, []byte{5}},
		{[]byte{0, 1}, []byte{2}},
		{[]byte{0, 2, 0, 0}, []byte{6}},
		{[]byte{0, 2, 1, 0}, []byte{3}},
	}
	require.Equal(t, expected, b.kv)
}

type pairs = [][2][]byte

func testPut(t *testing.T, ps pairs, tr1, tr2 *Trie) {
	var b Batch
	for _, p := range ps {
		require.NoError(t, tr1.Put(p[0], p[1]))
		b.Add(p[0], p[1])
	}

	n, err := tr2.PutBatch(b)
	require.NoError(t, err)
	require.Equal(t, len(b.kv), n)
	require.Equal(t, tr1.StateRoot(), tr2.StateRoot())

	t.Run("test restore", func(t *testing.T) {
		tr2.Flush()
		tr3 := NewTrie(NewHashNode(tr2.StateRoot()), false, storage.NewMemCachedStore(tr2.Store))
		for _, p := range ps {
			val, err := tr3.Get(p[0])
			if p[1] == nil {
				require.Error(t, err)
				continue
			}
			require.NoError(t, err, "key: %s", hex.EncodeToString(p[0]))
			require.Equal(t, p[1], val)
		}
	})
}

func TestTrie_PutBatchLeaf(t *testing.T) {
	prepareLeaf := func(t *testing.T) (*Trie, *Trie) {
		tr1 := NewTrie(new(HashNode), false, newTestStore())
		tr2 := NewTrie(new(HashNode), false, newTestStore())
		require.NoError(t, tr1.Put([]byte{}, []byte("value1")))
		require.NoError(t, tr2.Put([]byte{}, []byte("value1")))
		return tr1, tr2
	}

	t.Run("remove", func(t *testing.T) {
		tr1, tr2 := prepareLeaf(t)
		var ps = pairs{{[]byte{}, nil}}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("replace", func(t *testing.T) {
		tr1, tr2 := prepareLeaf(t)
		var ps = pairs{{[]byte{}, []byte("value2")}}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("remove and replace", func(t *testing.T) {
		tr1, tr2 := prepareLeaf(t)
		var ps = pairs{
			{[]byte{}, nil},
			{[]byte{2}, []byte("value2")},
		}
		testPut(t, ps, tr1, tr2)
	})
}

func TestTrie_PutBatchExtension(t *testing.T) {
	prepareExtension := func(t *testing.T) (*Trie, *Trie) {
		tr1 := NewTrie(new(HashNode), false, newTestStore())
		tr2 := NewTrie(new(HashNode), false, newTestStore())
		require.NoError(t, tr1.Put([]byte{1, 2}, []byte("value1")))
		require.NoError(t, tr2.Put([]byte{1, 2}, []byte("value1")))
		return tr1, tr2
	}

	t.Run("split, key len > 1", func(t *testing.T) {
		tr1, tr2 := prepareExtension(t)
		var ps = pairs{{[]byte{2, 3}, []byte("value2")}}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("split, key len = 1", func(t *testing.T) {
		tr1, tr2 := prepareExtension(t)
		var ps = pairs{{[]byte{1, 3}, []byte("value2")}}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("add to next", func(t *testing.T) {
		tr1, tr2 := prepareExtension(t)
		var ps = pairs{{[]byte{1, 2, 3}, []byte("value2")}}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("add to next with leaf", func(t *testing.T) {
		tr1, tr2 := prepareExtension(t)
		var ps = pairs{
			{[]byte{}, []byte("value3")},
			{[]byte{1, 2, 3}, []byte("value2")},
		}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("remove value", func(t *testing.T) {
		tr1, tr2 := prepareExtension(t)
		var ps = pairs{{[]byte{1, 2}, nil}}
		testPut(t, ps, tr1, tr2)
	})
	t.Run("add to next, merge extension", func(t *testing.T) {
		tr1, tr2 := prepareExtension(t)
		var ps = pairs{
			{[]byte{1, 2}, nil},
			{[]byte{1, 2, 3}, []byte("value2")},
		}
		testPut(t, ps, tr1, tr2)
	})
}

func TestTrie_PutBatch(t *testing.T) {
	tr1 := NewTrie(new(HashNode), false, newTestStore())
	tr2 := NewTrie(new(HashNode), false, newTestStore())
	var ps = pairs{
		{[]byte{1}, []byte{1}},
		{[]byte{2}, []byte{3}},
		{[]byte{4}, []byte{5}},
	}
	testPut(t, ps, tr1, tr2)

	ps = pairs{[2][]byte{{4}, {6}}}
	testPut(t, ps, tr1, tr2)

	ps = pairs{[2][]byte{{4}, nil}}
	testPut(t, ps, tr1, tr2)
}
