//go:build !zp_budget_controls_reference

package core

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/limits"
)

// TestMemoIndexMatchesChain drives random lookups and stores of both table
// paths against a plain map and checks that the chain holds the same entries.
// The keys cluster on few positions, so the chains pass memoScan and the index
// is built, extended and used for replacement.
func TestMemoIndexMatchesChain(t *testing.T) {
	envs := [][]any{nil, {1}, {int64(1)}, {"a"}, {"ab"}, {true}, {false}, {0.0}, {1.5}, {nil}, {[2]int{1, 2}}, {&struct{}{}}}
	for i := 0; i < 40; i++ {
		envs = append(envs, []any{i, fmt.Sprint("s", i%7)})
	}
	for _, budgeted := range []bool{false, true} {
		t.Run(fmt.Sprint("budgeted=", budgeted), func(t *testing.T) {
			rng := rand.New(rand.NewSource(1))
			table := NewMemoTable()
			p := memoStorageParser(table, limits.Limits{Budget: 1 << 40})
			if !budgeted {
				p.Control = nil
			}
			get := func(k MemoKey) (*MemoEntry, bool) {
				if budgeted {
					return p.MemoGet(k)
				}
				return table.Get(k)
			}
			put := func(k MemoKey, e *MemoEntry) {
				if budgeted {
					p.MemoPut(k, e)
				} else {
					table.Put(k, e)
				}
			}
			type key struct {
				rule, pos, min int
				silent         bool
				env            string
			}
			want := map[key]*MemoEntry{}
			for step := 0; step < 20000; step++ {
				env := envs[rng.Intn(len(envs))]
				k := MemoKey{Rule: rng.Intn(4), Pos: rng.Intn(5), Min: rng.Intn(2), MissingSilent: rng.Intn(4) == 0, Env: env}
				wk := key{k.Rule, k.Pos, k.Min, k.MissingSilent, envKey(env)}
				if env != nil {
					if _, ok := env[0].(*struct{}); ok {
						wk.env = fmt.Sprintf("%p", env[0])
					}
				}
				got, ok := get(k)
				if w := want[wk]; got != w || ok != (w != nil) {
					t.Fatalf("step %d: get %+v = %p,%v want %p", step, k, got, ok, w)
				}
				if rng.Intn(3) == 0 {
					continue
				}
				e := table.Alloc()
				if got != nil && rng.Intn(5) == 0 {
					e = got // storing an entry again under its own key
				}
				put(k, e)
				want[wk] = e
			}
			n := 0
			table.Each(func(e *MemoEntry) { n++ })
			if n != len(want) {
				t.Fatalf("chains hold %d entries, want %d", n, len(want))
			}
			if len(table.Index) == 0 {
				t.Fatal("no position was indexed")
			}
			// Dropping positions drops their indexes; lookups still agree.
			table.Prune(3)
			for p := range table.Index {
				if p < 3 {
					t.Fatalf("index of pruned position %d kept", p)
				}
			}
			table.Splice(3, 4, 2, func(*MemoEntry) bool { return false })
			if table.Index != nil {
				t.Fatal("indexes kept across a splice")
			}
		})
	}
}

// TestMemoIndexLookupCost checks the budget of a lookup in a long chain: it
// does not grow with the number of entries at the position.
func TestMemoIndexLookupCost(t *testing.T) {
	cost := func(entries int) uint64 {
		table := NewMemoTable()
		p := memoStorageParser(table, limits.Limits{Budget: 1 << 40})
		for i := 0; i < entries; i++ {
			p.MemoPut(MemoKey{Rule: 1, Pos: 0, Env: []any{i}}, table.Alloc())
		}
		p.MemoGet(MemoKey{Rule: 1, Pos: 0, Env: []any{-1}}) // builds the index
		before := p.Control.Used
		for i := 0; i < 50; i++ {
			p.MemoGet(MemoKey{Rule: 1, Pos: 0, Env: []any{0}})
		}
		return p.Control.Used - before
	}
	small, large := cost(100), cost(5000)
	if small != large {
		t.Fatalf("lookup cost depends on the chain: %d steps with 100 entries, %d with 5000", small, large)
	}
}

func envKey(env []any) string {
	s := ""
	for _, v := range env {
		s += fmt.Sprintf("%T:%v|", v, v)
	}
	return s
}

// TestMemoIndexBudgetedPrune checks that the budgeted prune of a stream drops the indexes of the
// positions it discards, as the ordinary prune does, and keeps the others usable.
func TestMemoIndexBudgetedPrune(t *testing.T) {
	for _, budgeted := range []bool{false, true} {
		table := NewMemoTable()
		p := memoStorageParser(table, limits.Limits{Budget: 1 << 40})
		if !budgeted {
			p.Control = nil
		}
		for pos := 0; pos < 2000; pos++ {
			for i := 0; i < 40; i++ {
				table.Put(MemoKey{Rule: 1, Pos: pos, Env: []any{i}}, table.Alloc())
			}
			table.Get(MemoKey{Rule: 1, Pos: pos, Env: []any{-1}}) // builds the index
		}
		if len(table.Index) != 2000 {
			t.Fatalf("budgeted=%v: %d indexes before the prune, want 2000", budgeted, len(table.Index))
		}
		p.PruneStreamMemo(1990)
		if len(table.Index) != 10 {
			t.Fatalf("budgeted=%v: %d indexes after the prune, want 10", budgeted, len(table.Index))
		}
		if _, ok := table.Get(MemoKey{Rule: 1, Pos: 1995, Env: []any{7}}); !ok {
			t.Fatalf("budgeted=%v: entry at a kept position lost", budgeted)
		}
	}
}

// TestMemoIndexStreamCommit checks that the commits of a controlled stream keep the number of
// retained indexes bounded by the positions not yet pruned.
func TestMemoIndexStreamCommit(t *testing.T) {
	c := NewBudgetControl(nil, limits.Limits{Budget: 1 << 40})
	p := NewControlledStreamParser(&SharedProgram{}, strings.NewReader(strings.Repeat("a", 6000)), CodePoints, c)
	for pos := 0; pos < 5000; pos++ {
		for i := 0; i < 40; i++ {
			p.MemoPut(MemoKey{Rule: 1, Pos: pos, Env: []any{i}}, p.Memo.Alloc())
		}
		p.MemoGet(MemoKey{Rule: 1, Pos: pos, Env: []any{-1}}) // builds the index
		if pos%100 == 99 {
			p.Pos = pos
			p.Fill(pos + 1)
			p.Commit(pos)
		}
	}
	if n := len(p.Memo.Index); n > 1100 {
		t.Fatalf("a committed stream retains %d indexes of 5000 positions", n)
	}
}
