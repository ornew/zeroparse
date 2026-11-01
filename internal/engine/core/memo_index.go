package core

import "math"

// memoScan is how many entries of a position's chain a lookup walks before it
// consults the position's hash index. Most positions hold fewer entries (at most
// one per memoized rule, and up to a few dozen in large grammars), so the common
// lookup is a short walk with no hashing; a position that holds more (a rule that
// reads a variable with many distinct values) is indexed. Measured on the Python
// and DuckDB grammars, a threshold of 8 slowed parsing by 4-10% (building maps for
// positions that a short walk serves), 32 and above by less than the noise. Replacing an
// entry of a long chain still walks the chain to the entry to relink it, which costs its
// position in the chain, one step each; only a store of an existing key does that.
const memoScan = 32

// memoIndex finds the entry of a long chain by the hash of its key, in
// constant time instead of by walking the chain. It is open addressing over a
// Go map: the entry for a key lives at its hash or at the next free hash
// above, and entries are never removed (a position is dropped as a whole). It
// only accelerates lookups; the chain stays the list of entries.
type memoIndex map[uint64]*MemoEntry

// memoKeyHash hashes a memo key deterministically, so that budgeted costs do
// not vary between runs. Values that are not hashed (types other than the
// common scalars) all hash alike and are told apart by the key comparison. It
// returns the number of string bytes hashed, which a budgeted lookup charges.
// Keys that rawValueEqual treats as equal have equal hashes.
func memoKeyHash(rule, min int32, silent bool, env []any) (h uint64, bytes int) {
	h = 14695981039346656037
	mix := func(x uint64) { h = (h ^ x) * 1099511628211; h ^= h >> 29 }
	mix(uint64(uint32(rule)))
	mix(uint64(uint32(min)))
	if silent {
		mix(1)
	}
	for _, v := range env {
		switch v := v.(type) {
		case nil:
			mix(0)
		case string:
			mix(uint64(len(v)))
			for i := 0; i < len(v); i++ {
				mix(uint64(v[i]))
			}
			bytes += len(v)
		case int:
			mix(uint64(v))
		case int64:
			mix(uint64(v))
		case bool:
			if v {
				mix(1)
			} else {
				mix(0)
			}
		case float64:
			if v != 0 { // +0 and -0 are equal
				mix(math.Float64bits(v))
			}
		default:
			mix(0x9e3779b97f4a7c15)
		}
	}
	return h, bytes
}

// probe returns the slot where an entry with hash h is or would be stored,
// and the entry that match accepts there, if any.
func (ix memoIndex) probe(h uint64, match func(*MemoEntry) bool) (slot uint64, e *MemoEntry) {
	for slot = h; ; slot++ {
		e = ix[slot]
		if e == nil || match(e) {
			return slot, e
		}
	}
}

// indexAt returns the index of the entries at pos, whose chain starts at head,
// building it when the position has none yet. charge, if not nil, is called
// for each entry hashed.
func (t *MemoTable) indexAt(pos int, head *MemoEntry, charge func(*MemoEntry)) memoIndex {
	if ix := t.Index[pos]; ix != nil {
		return ix
	}
	ix := make(memoIndex)
	for e := head; e != nil; e = e.Next {
		if charge != nil {
			charge(e)
		}
		h, _ := memoKeyHash(e.Rule, e.Min, e.missingSilent, e.Env)
		slot, _ := ix.probe(h, func(*MemoEntry) bool { return false })
		ix[slot] = e
	}
	if t.Index == nil {
		t.Index = make(map[int]memoIndex)
	}
	t.Index[pos] = ix
	return ix
}

// reindex makes the index of pos, if it has one, hold e where it held x, which
// has the key of e and is replaced by it in the chain. charge is as for indexAt.
func (t *MemoTable) reindex(pos int, x, e *MemoEntry, charge func(*MemoEntry)) {
	ix := t.Index[pos]
	if ix == nil {
		return
	}
	if charge != nil {
		charge(x)
	}
	h, _ := memoKeyHash(x.Rule, x.Min, x.missingSilent, x.Env)
	slot, _ := ix.probe(h, func(y *MemoEntry) bool { return y == x })
	ix[slot] = e
}
