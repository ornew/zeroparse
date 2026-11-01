package engine

import (
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/compile"
	"zeroparse.dev/zp/internal/engine/core"
	"zeroparse.dev/zp/internal/gentest"
)

// TestDocumentSeedDependentResultsNotInMemo checks the invariant that lets a Document keep its
// memo table between parses: no entry of the table depends on a growing seed. A result of a rule
// of a cycle that reads a seed is kept only for the growth in progress (context_memo.go); one that
// reads none is stored with the leaders that grew (MemoEntry.Present).
func TestDocumentSeedDependentResultsNotInMemo(t *testing.T) {
	t.Parallel()
	leader := func(id int, bit uint64) *core.Rule {
		r := &core.Rule{}
		r.ID, r.Leader, r.LeaderBit, r.Cycle = id, true, bit, 1
		return r
	}
	member := func(id int) *core.Rule {
		r := &core.Rule{}
		r.ID, r.Member, r.Cycle = id, true, 1
		return r
	}
	t.Run("a member that reads the seed", func(t *testing.T) {
		p := &core.Parser{Memo: core.NewMemoTable(), MemoAll: true}
		head, m := leader(1, 1), member(2)
		headCall, _, _, _ := p.CallBegin(head, 0)
		growth := p.GrowBegin(headCall.Key, head.LeaderBit, head.Cycle)
		if !p.GrowNext(&growth) {
			t.Fatal("the growth did not evaluate its first step")
		}
		mCall, _, _, hit := p.CallBegin(m, 0)
		if hit {
			t.Fatal("fresh member unexpectedly memoized")
		}
		if _, _, _, hit := p.CallBegin(head, 0); !hit || p.SeedReads != 1 {
			t.Fatal("the member did not read the seed")
		}
		p.CallEnd(m, &mCall, nil, false)
		if e, found := p.Memo.Get(mCall.Key); found {
			t.Fatalf("a result that read the growing seed was stored in the memo table: %+v", e)
		}
		p.GrowStep(&growth, nil, false)
		if p.GrowNext(&growth) {
			t.Fatal("a failed step continued the growth")
		}
		v, ok := p.GrowEnd(head, &growth)
		p.CallEnd(head, &headCall, v, ok)
		if e, found := p.Memo.Get(headCall.Key); !found || e.Present != 0 {
			t.Fatal("the result of the growth was not stored without a present set")
		}
	})
	t.Run("a member that reads no seed", func(t *testing.T) {
		p := &core.Parser{Memo: core.NewMemoTable(), MemoAll: true}
		head, m := leader(1, 1), member(2)
		headCall, _, _, _ := p.CallBegin(head, 0)
		growth := p.GrowBegin(headCall.Key, head.LeaderBit, head.Cycle)
		p.GrowNext(&growth)
		mCall, _, _, _ := p.CallBegin(m, 0)
		p.CallEnd(m, &mCall, nil, false)
		if e, found := p.Memo.Get(mCall.Key); !found || e.Present != 1 {
			t.Fatal("a result that read no seed was not stored with the leader that grew")
		}
		if _, _, _, hit := p.CallBegin(m, 0); !hit {
			t.Fatal("the result was not reused while the leader grows")
		}
		p.GrowStep(&growth, nil, false)
		p.GrowNext(&growth)
		v, ok := p.GrowEnd(head, &growth)
		p.CallEnd(head, &headCall, v, ok)
	})
	t.Run("a nested leader that reads the outer seed", func(t *testing.T) {
		p := &core.Parser{Memo: core.NewMemoTable(), MemoAll: true}
		outer, inner := leader(1, 1), leader(2, 2)
		outerCall, _, _, _ := p.CallBegin(outer, 0)
		outerGrow := p.GrowBegin(outerCall.Key, outer.LeaderBit, outer.Cycle)
		p.GrowNext(&outerGrow)
		innerCall, _, _, hit := p.CallBegin(inner, 0)
		if hit {
			t.Fatal("the inner leader was reused before it grew")
		}
		innerGrow := p.GrowBegin(innerCall.Key, inner.LeaderBit, inner.Cycle)
		if !p.GrowNext(&innerGrow) {
			t.Fatal("the inner growth did not evaluate its first step")
		}
		if _, _, _, hit := p.CallBegin(outer, 0); !hit {
			t.Fatal("outer seed was not read")
		}
		p.GrowStep(&innerGrow, nil, false)
		p.GrowNext(&innerGrow)
		v, ok := p.GrowEnd(inner, &innerGrow)
		p.CallEnd(inner, &innerCall, v, ok)
		if _, found := p.Memo.Get(innerCall.Key); found {
			t.Fatal("the inner result, which read the outer seed, was stored in the memo table")
		}
		p.GrowStep(&outerGrow, nil, false)
		p.GrowNext(&outerGrow)
		v, ok = p.GrowEnd(outer, &outerGrow)
		p.CallEnd(outer, &outerCall, v, ok)
		if e, found := p.Memo.Get(outerCall.Key); !found || e.Present != 0 {
			t.Fatal("the outer result was not stored")
		}
	})
}

// BenchmarkDocumentLeftRecursion measures edits to the last line of a thousand
// direct or indirect left-recursive expressions.
func BenchmarkDocumentLeftRecursion(b *testing.B) {
	for _, kind := range []string{"direct", "indirect", "nested"} {
		src := `rule main = (expr "\n")* $$
rule expr = expr "+" num / num
rule num = [0-9]+ #atomic`
		if kind == "indirect" {
			src = `rule main = (expr "\n")* $$
rule expr = sum / num
rule sum = expr "+" num
rule num = [0-9]+ #atomic`
		}
		if kind == "nested" {
			src = `rule main = (expr "\n")* $$
rule expr = expr "+" term / term
rule term = term "*" num / num
rule num = [0-9]+ #atomic`
		}
		prog, err := Compile(mustParse(src), compile.Options{})
		if err != nil {
			b.Fatal(err)
		}
		text := strings.Repeat("1+2+3\n", 1000)
		for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
			b.Run(kind+"/"+backend.String(), func(b *testing.B) {
				doc, err := prog.NewDocumentWith("main", text, ParseOptions{Backend: backend})
				if err != nil {
					b.Fatal(err)
				}
				if _, err := doc.Parse(); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				i := 0
				for b.Loop() {
					if err := doc.Edit(len(text)-2, len(text)-1, string(rune('3'+i%2))); err != nil {
						b.Fatal(err)
					}
					i++
					if _, err := doc.Parse(); err != nil {
						b.Fatal(err)
					}
				}
				got, gotErr := doc.Parse()
				want, wantErr := prog.ParseWith("main", doc.Text(), ParseOptions{Backend: backend})
				if gentest.ResultJSON(got, gotErr) != gentest.ResultJSON(want, wantErr) {
					b.Fatal("edited Document differs from a fresh parse")
				}
			})
		}
	}
}
