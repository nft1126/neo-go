package mpt

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
)

// Batch is batch of storage changes.
// It is convenient to also represent it as a trie, but with a much simpler structure.
type Batch struct {
	kv []keyValue
}

type keyValue struct {
	key   []byte
	value []byte
}

// Add adds key-value pair to batch.
func (b *Batch) Add(key []byte, value []byte) {
	path := toNibbles(key)
	i := sort.Search(len(b.kv), func(i int) bool {
		return bytes.Compare(path, b.kv[i].key) <= 0
	})
	if i == len(b.kv) {
		b.kv = append(b.kv, keyValue{path, value})
	} else if bytes.Equal(b.kv[i].key, path) {
		b.kv[i].value = value
	} else {
		b.kv = append(b.kv, keyValue{})
		copy(b.kv[i+1:], b.kv[i:])
		b.kv[i].key = path
		b.kv[i].value = value
	}
}

// PutBatch puts batch to trie.
// It is not atomical (and probably cannot be without substantial slow-down)
// and returns amount of the elements processed.
// However each element is being put atomically, so Trie is always in a valid state.
// It is used mostly after the block processing to update MPT and error is not expected.
func (t *Trie) PutBatch(b Batch) (int, error) {
	r, n, err := t.putBatch(b.kv)
	if err == nil && n != len(b.kv) {
		panic("lul")
	}
	t.root = r
	return n, err
}

func (t *Trie) putBatch(kv []keyValue) (Node, int, error) {
	return t.putBatchIntoNode(t.root, kv)
}

func (t *Trie) putBatchIntoNode(curr Node, kv []keyValue) (Node, int, error) {
	switch n := curr.(type) {
	case *LeafNode:
		return t.putBatchIntoLeaf(n, kv)
	case *BranchNode:
		return t.putBatchIntoBranch(n, kv)
	case *ExtensionNode:
		return t.putBatchIntoExtension(n, kv)
	case *HashNode:
		return t.putBatchIntoHash(n, kv)
	default:
		panic("invalid MPT node type")
	}
}

func (t *Trie) putBatchIntoLeaf(curr *LeafNode, kv []keyValue) (Node, int, error) {
	t.removeRef(curr.Hash(), curr.Bytes())
	return t.newSubTrieMany(nil, kv, curr.value)
}

func (t *Trie) putBatchIntoBranch(curr *BranchNode, kv []keyValue) (Node, int, error) {
	return t.addToBranch(curr, kv, true)
}

func (t *Trie) mergeExtension(prefix []byte, sub Node, inTrie bool) Node {
	switch sn := sub.(type) {
	case *ExtensionNode:
		if inTrie {
			t.removeRef(sn.Hash(), sn.bytes)
		}
		sn.key = append(prefix, sn.key...)
		sn.invalidateCache()
		t.addRef(sn.Hash(), sn.bytes)
		return sn
	case *HashNode:
		return sn
	default:
		if !inTrie {
			t.addRef(sub.Hash(), sub.Bytes())
		}
		if len(prefix) != 0 {
			e := NewExtensionNode(prefix, sub)
			t.addRef(e.Hash(), e.bytes)
			return e
		}
		return sub
	}
}

func (t *Trie) putBatchIntoExtension(curr *ExtensionNode, kv []keyValue) (Node, int, error) {
	t.removeRef(curr.Hash(), curr.bytes)

	common := lcpMany(kv)
	pref := lcp(common, curr.key)
	if len(pref) == len(curr.key) {
		// Extension must be split into new nodes.
		stripPrefix(len(curr.key), kv)
		sub, n, err := t.putBatchIntoNode(curr.next, kv)
		return t.mergeExtension(pref, sub, true), n, err
	}

	if len(pref) != 0 {
		stripPrefix(len(pref), kv)
		sub, n, err := t.putBatchIntoExtensionNoPrefix(curr.key[len(pref):], curr.next, kv)
		return t.mergeExtension(pref, sub, true), n, err
	}
	return t.putBatchIntoExtensionNoPrefix(curr.key, curr.next, kv)
}

func (t *Trie) putBatchIntoExtensionNoPrefix(key []byte, next Node, kv []keyValue) (Node, int, error) {
	b := NewBranchNode()
	if len(key) > 1 {
		b.Children[key[0]] = t.newSubTrie(key[1:], next, false)
	} else {
		b.Children[key[0]] = next
	}
	return t.addToBranch(b, kv, false)
}

func isEmpty(n Node) bool {
	hn, ok := n.(*HashNode)
	return ok && hn.IsEmpty()
}

// addToBranch puts items into the branch node assuming b is not yet in trie.
func (t *Trie) addToBranch(b *BranchNode, kv []keyValue, inTrie bool) (Node, int, error) {
	if inTrie {
		t.removeRef(b.Hash(), b.bytes)
	}
	n, err := t.iterateBatch(kv, func(c byte, kv []keyValue) (int, error) {
		child, n, err := t.putBatchIntoNode(b.Children[c], kv)
		b.Children[c] = child
		return n, err
	})
	if inTrie && n != 0 {
		b.invalidateCache()
	}
	return t.stripBranch(b, false), n, err
}

// stripsBranch strips branch node after incomplete batch put.
// Second parameter specifies if old reference should be removed.
func (t *Trie) stripBranch(b *BranchNode, inTrie bool) Node {
	var n int
	var lastIndex byte
	for i := range b.Children {
		if !isEmpty(b.Children[i]) {
			n += 1
			lastIndex = byte(i)
		}
	}
	switch {
	case n == 0:
		if inTrie {
			t.removeRef(b.Hash(), b.bytes)
		}
		return new(HashNode)
	case n == 1:
		if inTrie {
			t.removeRef(b.Hash(), b.bytes)
		}
		return t.mergeExtension([]byte{lastIndex}, b.Children[lastIndex], true)
	default:
		if !inTrie {
			t.addRef(b.Hash(), b.bytes)
		}
		return b
	}
}

func (t *Trie) iterateBatch(kv []keyValue, f func(c byte, kv []keyValue) (int, error)) (int, error) {
	var n int
	for len(kv) != 0 {
		c, i := getLastIndex(kv)
		if c != lastChild {
			stripPrefix(1, kv[:i])
		}
		sub, err := f(c, kv[:i])
		n += sub
		if err != nil {
			return n, err
		}
		kv = kv[i:]
	}
	return n, nil
}

func (t *Trie) putBatchIntoHash(curr *HashNode, kv []keyValue) (Node, int, error) {
	if curr.IsEmpty() {
		common := lcpMany(kv)
		stripPrefix(len(common), kv)
		return t.newSubTrieMany(common, kv, nil)
	}
	result, err := t.getFromStore(curr.hash)
	if err != nil {
		return curr, 0, err
	}
	return t.putBatchIntoNode(result, kv)
}

// Creates new subtrie from provided key-value pairs.
// Items in kv must have no common prefix.
// If there are any deletions in kv, return error.
// kv is not empty.
// kv is sorted by key.
// value is current value stored by prefix.
func (t *Trie) newSubTrieMany(prefix []byte, kv []keyValue, value []byte) (Node, int, error) {
	if len(kv[0].key) == 0 {
		if len(kv[0].value) == 0 {
			if len(kv) == 1 {
				return new(HashNode), 1, nil
			}
			node, n, err := t.newSubTrieMany(prefix, kv[1:], nil)
			return node, n + 1, err
		}
		if len(kv) == 1 {
			return t.newSubTrie(prefix, NewLeafNode(kv[0].value), true), 1, nil
		}
		value = kv[0].value
	}

	// Prefix is empty and we have at least 2 children.
	b := NewBranchNode()
	if len(value) != 0 {
		// Empty key is always first.
		leaf := NewLeafNode(value)
		t.addRef(leaf.Hash(), leaf.bytes)
		b.Children[lastChild] = leaf
	}
	nd, n, err := t.addToBranch(b, kv, false)
	return t.mergeExtension(prefix, nd, true), n, err
}

func stripPrefix(n int, kv []keyValue) {
	for i := range kv {
		kv[i].key = kv[i].key[n:]
	}
}

func getLastIndex(kv []keyValue) (byte, int) {
	if len(kv[0].key) == 0 {
		return lastChild, 1
	}
	c := kv[0].key[0]
	for i := range kv[1:] {
		if kv[i+1].key[0] != c {
			return c, i + 1
		}
	}
	return c, len(kv)
}

func printNode(prefix string, n Node) {
	switch tn := n.(type) {
	case *HashNode:
		if tn.IsEmpty() {
			fmt.Printf("%s empty\n", prefix)
			return
		}
		fmt.Printf("%s %s\n", prefix, tn.Hash().StringLE())
	case *BranchNode:
		for i, c := range tn.Children {
			if isEmpty(c) {
				continue
			}
			fmt.Printf("%s [%2d] ->\n", prefix, i)
			printNode(prefix+" ", c)
		}
	case *ExtensionNode:
		fmt.Printf("%s extension-> %s\n", prefix, hex.EncodeToString(tn.key))
		printNode(prefix+" ", tn.next)
	case *LeafNode:
		fmt.Printf("%s leaf-> %s\n", prefix, hex.EncodeToString(tn.value))
	}
}
