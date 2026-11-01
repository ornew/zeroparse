package runtime

import (
	"fmt"
	"math/rand"
	"testing"
)

// TestMemoIndexMatchesChain drives random lookups and stores of the three memo
// paths (the table of Nodes, with and without budget controls, and the typed
// memo, likewise) against a plain map. The keys cluster on few positions, so
// the chains pass memoScan and the index is built, extended and used for
// replacement.
func TestMemoIndexMatchesChain(t *testing.T) {
	envs := [][]any{nil, {1}, {int64(1)}, {"a"}, {"ab"}, {true}, {false}, {0.0}, {1.5}, {nil}, {[2]int{1, 2}}, {&struct{}{}}}
	for i := 0; i < 40; i++ {
		envs = append(envs, []any{i, fmt.Sprint("s", i%7)})
	}
	envKey := func(env []any) string {
		s := ""
		for _, v := range env {
			if _, ok := v.(*struct{}); ok {
				s += fmt.Sprintf("%p|", v)
			} else {
				s += fmt.Sprintf("%T:%v|", v, v)
			}
		}
		return s
	}
	type kind struct {
		name     string
		budgeted bool
		typed    bool
	}
	for _, k := range []kind{{"nodes", false, false}, {"nodes budgeted", true, false}, {"typed", false, true}, {"typed budgeted", true, true}} {
		t.Run(k.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(1))
			p := &Parser{}
			if k.budgeted {
				p.Control = newBudgetControl(nil, Limits{Budget: 1 << 40})
			}
			tp := &TParser{Parser: p}
			var table *memoTable = &p.memo
			type key struct {
				rule, pos, min int
				env            string
			}
			want := map[key]any{}
			for step := 0; step < 20000; step++ {
				env := envs[rng.Intn(len(envs))]
				rule, pos, min := rng.Intn(4), rng.Intn(5), rng.Intn(2)
				wk := key{rule, pos, min, envKey(env)}
				var got any
				if k.typed {
					if e := tp.memoGet(rule, pos, min, env); e != nil {
						got = e
					}
				} else if e := table.getAny(p, rule, pos, min, env); e != nil {
					got = e
				}
				if got != want[wk] {
					t.Fatalf("step %d: get %+v = %p, want %p", step, wk, got, want[wk])
				}
				if rng.Intn(3) == 0 {
					continue
				}
				if k.typed {
					e := new(tmemoEntry)
					if got != nil && rng.Intn(5) == 0 {
						e = got.(*tmemoEntry)
					}
					tp.memoPut(rule, pos, min, env, e)
					want[wk] = e
				} else {
					e := table.alloc()
					if got != nil && rng.Intn(5) == 0 {
						e = got.(*memoEntry)
					}
					table.putAny(p, rule, pos, min, env, e)
					want[wk] = e
				}
			}
			n := 0
			if k.typed {
				for _, head := range tp.memoSlots {
					for e := head; e != nil; e = e.next {
						n++
					}
				}
				if len(tp.memoIndexes) == 0 {
					t.Fatal("no position was indexed")
				}
			} else {
				for _, head := range table.slots {
					for e := head; e != nil; e = e.next {
						n++
					}
				}
				if len(table.index) == 0 {
					t.Fatal("no position was indexed")
				}
			}
			if n != len(want) {
				t.Fatalf("chains hold %d entries, want %d", n, len(want))
			}
		})
	}
}

func (t *memoTable) getAny(p *Parser, rule, pos, min int, env []any) *memoEntry {
	if p.Control != nil {
		return p.memoGetControlled(rule, pos, min, env)
	}
	return t.get(rule, pos, min, env)
}

func (t *memoTable) putAny(p *Parser, rule, pos, min int, env []any, e *memoEntry) {
	if p.Control != nil {
		p.memoPutControlled(rule, pos, min, env, e)
		return
	}
	t.put(rule, pos, min, env, e)
}

// TestMemoIndexStreamPrune checks that pruning a stream's memo keeps the
// indexes of the positions that remain, at their new offsets.
func TestMemoIndexStreamPrune(t *testing.T) {
	p := &Parser{Stream: &streamInput{}}
	for pos := 0; pos < 6; pos++ {
		for i := 0; i < 40; i++ {
			p.memo.put(1, pos, 0, []any{i}, p.memo.alloc())
		}
		p.memo.get(1, pos, 0, []any{-1}) // builds the index
	}
	p.pruneStreamMemo(3)
	if len(p.memo.index) != 3 {
		t.Fatalf("%d indexes after the prune, want 3", len(p.memo.index))
	}
	for pos := 0; pos < 3; pos++ {
		for i := 0; i < 40; i++ {
			if p.memo.get(1, pos, 0, []any{i}) == nil {
				t.Fatalf("entry %d at position %d lost by the prune", i, pos)
			}
		}
	}
}
