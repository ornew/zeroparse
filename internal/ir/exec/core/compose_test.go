package core

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/ornew/zeroparse/internal/ir/exec"
	"github.com/ornew/zeroparse/internal/ir/exec/ops"
	"github.com/ornew/zeroparse/internal/ir/exec/pass"
	"github.com/ornew/zeroparse/internal/ir/exec/ref"
)

// The tests of this file run the compositions on a model of the operations: the reference model of
// the pilot operations (ref.Model) and a model of the transitional ones written from their
// contracts, with rules whose bodies the tests script. They check the contracts of the compositions
// (what a call leaves in the parser state, what it charges, which entries it reuses) without a
// generated parser, so that they hold in a run without the Exec IR switch, and where a generated
// parser cannot tell (it restores the position after a failed call itself).

// trule is a rule of the model: its flags and its scripted body, which runs at the position of the
// machine and returns the value and success of the body.
type trule struct {
	id                           int64
	memo, leader, member, defer_ bool
	body                         func(m *machine) (any, bool)
	runs                         int // the evaluations of the body
}

// record is an expectation record of the model: the farthest position and the expectations there.
type record struct {
	far  int64
	exps []string
}

// machine models the operations of the compositions.
type machine struct {
	*ref.Model
	regs map[string]any
	rec  []string
	// cur is the expectation record; saved holds the records that isolations set aside.
	cur   record
	saved []record
	// exps holds the expectations of each entry (memo.setExpected).
	exps map[*ref.Entry]record
	// pruned makes memo.grown find no entry, as after a stream commit.
	pruned                 bool
	spends, steps, memoHit int
	// growths, seedPos and readDepth are the growth state (ops.Growth); depth holds the depth of each
	// growing entry (memo.mark), and of an unstable entry 0 (memo.settle).
	growths, seedPos, readDepth int64
	depth                       map[*ref.Entry]int64
	// events lists what the observation was told (obs.enter, obs.exit); high is the examined frontier
	// of the active call (doc.examinedBegin, doc.examinedEnd).
	events []string
	high   int64
	// reuse is the reuse of the active Document call (doc.enter, doc.leave), and docMemo tells
	// whether a Document parse memoizes the rules (rule.docMemo).
	reuse   int64
	docMemo bool
	// docActive tells whether the parse reads a Document (doc.active); examined and reuses hold what
	// each entry records (memo.setExamined, memo.setReuse); reused counts the reused entries
	// (doc.reused) and parseOnly lists the entries reused only in this parse (doc.parseOnly).
	docActive bool
	examined  map[*ref.Entry]int64
	reuses    map[*ref.Entry]int64
	reused    int
	parseOnly []string
	// last is the entry stored or grown last (memo.store, memo.grown).
	last *ref.Entry
}

func newMachine() *machine {
	return &machine{Model: ref.New(ref.Config{Rules: 4, MaxDepth: 100}), exps: map[*ref.Entry]record{},
		readDepth: math.MaxInt64, depth: map[*ref.Entry]int64{},
		examined: map[*ref.Entry]int64{}, reuses: map[*ref.Entry]int64{}}
}

func (m *machine) UseRegisters(regs map[string]any) { m.regs = regs }

func (m *machine) pos() int64         { return m.regs[ops.Pos].(int64) }
func (m *machine) setPos(p int64)     { m.regs[ops.Pos] = p }
func (m *machine) silent() bool       { return m.regs[ops.Silent].(int64) > 0 }
func (m *machine) recover(e string)   { m.rec = append(m.rec, e) }
func (m *machine) expect(e string)    { m.merge(m.pos(), []string{e}) }
func (m *machine) advance(n int64)    { m.setPos(m.pos() + n) }
func (m *machine) record() []string   { return slices.Clone(m.rec) }
func (m *machine) farthest() int64    { return m.cur.far }
func (m *machine) expected() []string { return m.cur.exps }

// merge adds the expectations at far to the record, as the runtime's Expect does: outside lookaheads,
// at the farthest position or past it.
func (m *machine) merge(far int64, exps []string) {
	for _, e := range exps {
		if m.silent() || far < m.cur.far {
			continue
		}
		if far > m.cur.far {
			m.cur = record{far: far}
		}
		if !slices.Contains(m.cur.exps, e) {
			m.cur.exps = append(m.cur.exps, e)
		}
	}
}

func (m *machine) growing(e *ref.Entry) bool {
	return m.Model.Call(&exec.Op{Name: "memo.growing"}, []any{e})[0].(bool)
}

func (m *machine) entrySilent(e *ref.Entry) bool {
	return m.Model.Call(&exec.Op{Name: "memo.silent"}, []any{e})[0].(bool)
}

func (m *machine) noErrs(e *ref.Entry) bool {
	errs, _ := m.Model.Call(&exec.Op{Name: "memo.errs"}, []any{e})[0].([]string)
	return len(errs) == 0
}

func entryOf(x any) *ref.Entry {
	e, _ := x.(*ref.Entry)
	if e == nil {
		panic(exec.Interrupt{Reason: "the null entry is used"})
	}
	return e
}

func (m *machine) Call(o *exec.Op, args []any) []any {
	switch o.Name {
	case "call.tracked":
		return []any{false}
	case "call.spend":
		m.spends++
		return nil
	case "call.growStep":
		m.steps++
		return nil
	case "call.first":
		r := args[0].(*trule)
		if !r.defer_ {
			return []any{false}
		}
		return m.Model.Call(&exec.Op{Name: "seen.first"}, []any{r.id, m.pos()})
	case "rule.id":
		return []any{args[0].(*trule).id}
	case "rule.memo":
		return []any{args[0].(*trule).memo}
	case "rule.leader":
		return []any{args[0].(*trule).leader}
	case "rule.plain":
		return []any{false}
	case "rule.invoke":
		r := args[0].(*trule)
		r.runs++
		v, ok := r.body(m)
		return []any{v, ok}
	case "env.memoKey":
		return []any{int64(0)}
	case "rec.mark":
		return []any{int64(len(m.rec))}
	case "rec.truncate":
		m.rec = m.rec[:args[0].(int64)]
		return nil
	case "rec.since":
		return []any{slices.Clone(m.rec[args[0].(int64):])}
	case "rec.add":
		errs, _ := args[0].([]string)
		m.rec = append(m.rec, errs...)
		return nil
	case "rec.none":
		errs, _ := args[0].([]string)
		return []any{len(errs) == 0}
	case "exp.isolate":
		m.saved = append(m.saved, m.cur)
		m.cur = record{far: args[0].(int64)}
		return []any{len(m.saved)}
	case "exp.unisolate":
		if args[0].(int) != len(m.saved) {
			panic("exp.unisolate of a record that is not the last set aside")
		}
		ended := m.cur
		m.cur, m.saved = m.saved[len(m.saved)-1], m.saved[:len(m.saved)-1]
		return []any{ended.far, ended.exps}
	case "exp.keep":
		s, _ := args[0].([]string)
		return []any{slices.Clone(s)}
	case "exp.merge", "exp.mergeKept":
		s, _ := args[1].([]string)
		m.merge(args[0].(int64), s)
		return nil
	case "memo.far":
		return []any{m.exps[entryOf(args[0])].far}
	case "memo.expected":
		return []any{m.exps[entryOf(args[0])].exps}
	case "memo.setExpected":
		s, _ := args[2].([]string)
		m.exps[entryOf(args[0])] = record{far: args[1].(int64), exps: s}
		return nil
	case "memo.grown":
		res := m.Model.Call(&exec.Op{Name: "memo.lookup"}, args)
		if m.pruned || !res[1].(bool) {
			return []any{nil}
		}
		m.last = res[0].(*ref.Entry)
		return []any{res[0]}
	case "memo.detached":
		return []any{&ref.Entry{}}
	case "obs.memoHit":
		m.memoHit++
		return nil
	case "obs.enter":
		m.events = append(m.events, fmt.Sprintf("enter %d at %d level %d", args[0].(*trule).id, args[1], args[2]))
		return nil
	case "obs.exit":
		m.events = append(m.events, fmt.Sprintf("exit %v at %d examined %d %v", args[0], args[1], args[2], args[3]))
		return nil
	case "doc.examinedBegin":
		h := m.high
		m.high = m.pos()
		return []any{h}
	case "doc.examinedEnd":
		x := m.high
		m.high = max(args[0].(int64), x)
		return []any{x, true}
	case "doc.enter":
		h, u := m.high, m.reuse
		m.high, m.reuse = args[0].(int64)+1, 0
		return []any{h, u}
	case "doc.leave":
		m.high = max(args[0].(int64), m.high)
		m.reuse = max(args[1].(int64), m.reuse)
		return nil
	case "rule.docMemo":
		return []any{m.docMemo}
	case "doc.active":
		return []any{m.docActive}
	case "doc.touch":
		m.high = max(m.high, args[0].(int64))
		return nil
	case "doc.high":
		return []any{m.high}
	case "doc.reuse":
		return []any{m.reuse}
	case "doc.setReuse":
		m.reuse = args[0].(int64)
		return nil
	case "doc.limitReuse":
		m.reuse = max(m.reuse, args[0].(int64))
		return nil
	case "doc.reused":
		m.reused++
		return nil
	case "doc.parseOnly":
		m.parseOnly = append(m.parseOnly, fmt.Sprintf("%d reuse %d", args[0], m.reuses[entryOf(args[1])]))
		return nil
	case "reuse.untilEdit":
		return []any{int64(1)}
	case "reuse.inParse":
		return []any{int64(2)}
	case "memo.examined":
		return []any{m.examined[entryOf(args[0])]}
	case "memo.reuse":
		return []any{m.reuses[entryOf(args[0])]}
	case "memo.setExamined":
		m.examined[entryOf(args[0])] = args[1].(int64)
		return nil
	case "memo.setReuse":
		m.reuses[entryOf(args[0])] = args[1].(int64)
		return nil
	case "call.body":
		r := args[0].(*trule)
		r.runs++
		v, ok := r.body(m)
		return []any{v, ok}
	case "lr.stale":
		return []any{args[0].(*trule).member && m.growths != 0 && m.seedPos == args[1].(int64)}
	case "lr.read":
		e := entryOf(args[0])
		if d, ok := m.depth[e]; ok && (d == 0 || m.growing(e)) {
			m.readDepth = min(m.readDepth, d)
		}
		return nil
	case "lr.depth":
		return []any{m.growths}
	case "lr.enter":
		read := m.readDepth
		m.readDepth = math.MaxInt64
		return []any{read}
	case "lr.leave":
		m.readDepth = min(m.readDepth, args[0].(int64))
		return nil
	case "lr.changeable":
		return []any{m.readDepth <= args[0].(int64)}
	case "lr.begin":
		prev := m.seedPos
		m.growths++
		m.seedPos = args[0].(int64)
		return []any{prev}
	case "lr.end":
		m.growths--
		m.seedPos = args[0].(int64)
		return nil
	case "memo.mark":
		m.depth[entryOf(args[0])] = m.growths
		return nil
	case "memo.settle":
		e := entryOf(args[0])
		delete(m.depth, e)
		if m.readDepth <= m.growths && (m.entrySilent(e) || !m.noErrs(e)) {
			m.depth[e] = 0
		}
		return nil
	}
	res := m.Model.Call(o, args)
	if o.Name == "memo.store" {
		m.last = res[0].(*ref.Entry)
	}
	return res
}

// composition returns the composition fn of the module m specialized for on.
func composition(t *testing.T, m *exec.Module, fn string, on ...string) *exec.Func {
	t.Helper()
	f, err := pass.Specialize(m, m.Func(fn), fn, on)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// callAt calls the rule r at the position pos with the composition call specialized for on, and
// returns its value and success.
func callAt(t *testing.T, mod *exec.Module, m *machine, f *exec.Func, r *trule, pos int64) (any, bool) {
	t.Helper()
	// Run starts with zero registers; the machine keeps the position and depth between calls.
	silent := int64(0)
	if m.regs != nil {
		silent = m.regs[ops.Silent].(int64)
	}
	res, err := exec.Run(mod, f, &start{machine: m, pos: pos, silent: silent}, r, int64(0))
	if err != nil {
		t.Fatal(err)
	}
	return res[0], res[1].(bool)
}

// start sets the registers of a run before the function runs.
type start struct {
	*machine
	pos, silent int64
}

func (s *start) UseRegisters(regs map[string]any) {
	regs[ops.Pos], regs[ops.Silent] = s.pos, s.silent
	s.machine.UseRegisters(regs)
}

// TestCallFailureRestoresState checks that a failed call, unmemoized and through the memo, leaves the
// position where it started and drops the errors its body recovered.
func TestCallFailureRestoresState(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "call")
	m := newMachine()
	r := &trule{id: 1, memo: true, defer_: true, body: func(m *machine) (any, bool) {
		m.recover("skipped")
		m.advance(2)
		return nil, false
	}}
	for i, path := range []string{"unmemoized", "memoized", "reused"} {
		if _, ok := callAt(t, mod, m, f, r, 3); ok {
			t.Fatalf("%s call succeeded", path)
		}
		if m.pos() != 3 || len(m.rec) != 0 {
			t.Errorf("%s call: position %d, recovered %v; want 3 and none", path, m.pos(), m.rec)
		}
		if want := min(i+1, 2); r.runs != want {
			t.Errorf("%s call: %d evaluations of the body, want %d", path, r.runs, want)
		}
	}
	if m.spends != 3 {
		t.Errorf("%d charges for 3 calls", m.spends)
	}
}

// TestUnmemoized checks that the unmemoized call of the tracked path evaluates the body each time,
// keeps what a successful body recovered, and restores the position and the recovered errors after
// a failed one.
func TestUnmemoized(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "unmemoized")
	for _, ok := range []bool{true, false} {
		m := newMachine()
		m.rec = []string{"before"}
		r := &trule{id: 1, memo: true, body: func(m *machine) (any, bool) {
			m.recover("skipped")
			m.advance(2)
			return "v", ok
		}}
		for i := range 2 {
			v, got := callAt(t, mod, m, f, r, 3)
			want, wantRec := int64(5), []string{"before", "skipped"}
			if !ok {
				want, wantRec = 3, []string{"before"}
			}
			if got != ok || (ok && v != "v") || m.pos() != want || !slices.Equal(m.rec, wantRec) {
				t.Errorf("%v call %d: %v %v at %d, recovered %v; want %v at %d and %v", ok, i, v, got, m.pos(), m.rec, ok, want, wantRec)
			}
			m.rec = []string{"before"}
		}
		if r.runs != 2 || m.spends != 0 {
			t.Errorf("%v: %d evaluations and %d charges, want 2 and none", ok, r.runs, m.spends)
		}
	}
}

// TestCallSuccessKeepsErrors checks that a successful memoized call keeps the errors its body
// recovered and the expectations it recorded, and that the call that reuses its entry records them
// again and moves to its end.
func TestCallSuccessKeepsErrors(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "call")
	m := newMachine()
	r := &trule{id: 1, memo: true, defer_: true, body: func(m *machine) (any, bool) {
		m.recover("skipped")
		m.advance(2)
		m.expect("x")
		return "v", true
	}}
	for i := range 3 {
		m.rec, m.cur = nil, record{}
		v, ok := callAt(t, mod, m, f, r, 3)
		if !ok || v != "v" || m.pos() != 5 || !slices.Equal(m.rec, []string{"skipped"}) {
			t.Errorf("call %d: %v %v at %d, recovered %v; want v true at 5 and one error", i, v, ok, m.pos(), m.rec)
		}
		if m.farthest() != 5 || !slices.Equal(m.expected(), []string{"x"}) {
			t.Errorf("call %d: the record holds %d %v, want 5 [x]", i, m.farthest(), m.expected())
		}
	}
	if r.runs != 2 {
		t.Errorf("%d evaluations of the body, want 2", r.runs)
	}
}

// TestCallExpectationsFromStart checks that a memoized call records its expectations in a record
// that starts at its position: its entry keeps that position as the farthest one when the body
// recorded nothing past it, and the expectations it recorded otherwise.
func TestCallExpectationsFromStart(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "call")
	for _, c := range []struct {
		name string
		exp  bool
	}{{"nothing recorded", false}, {"an expectation", true}} {
		t.Run(c.name, func(t *testing.T) {
			m := newMachine()
			r := &trule{id: 1, memo: true, body: func(m *machine) (any, bool) {
				if c.exp {
					m.advance(1)
					m.expect("x")
				}
				return nil, false
			}}
			callAt(t, mod, m, f, r, 4)
			res := m.Model.Call(&exec.Op{Name: "memo.lookup"}, []any{int64(1), int64(4), int64(0), int64(0)})
			e := res[0].(*ref.Entry)
			want := record{far: 4}
			if c.exp {
				want = record{far: 5, exps: []string{"x"}}
			}
			if got := m.exps[e]; got.far != want.far || !slices.Equal(got.exps, want.exps) {
				t.Errorf("the entry keeps %+v, want %+v", got, want)
			}
			// An empty set of expectations leaves the enclosing record as it is.
			outer := record{}
			if c.exp {
				outer = want
			}
			if m.farthest() != outer.far || !slices.Equal(m.expected(), outer.exps) {
				t.Errorf("the record holds %d %v, want %+v", m.farthest(), m.expected(), outer)
			}
		})
	}
}

// TestCallReuseContexts checks which entries a call reuses: one computed in a lookahead only in
// one, and one with recovered errors only in the kind of context it was computed in.
func TestCallReuseContexts(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "call")
	for _, c := range []struct {
		name                string
		errs                bool
		storeSilent, silent int64
		reused              bool
	}{
		{"plain entry in a lookahead", false, 0, 1, true},
		{"silent entry outside", false, 1, 0, false},
		{"silent entry in a lookahead", false, 1, 1, true},
		{"entry with errors in a lookahead", true, 0, 1, false},
		{"silent entry with errors in a lookahead", true, 1, 1, true},
		{"entry with errors outside", true, 0, 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newMachine()
			r := &trule{id: 1, memo: true, body: func(m *machine) (any, bool) {
				if c.errs {
					m.recover("skipped")
				}
				m.advance(1)
				return "v", true
			}}
			m.regs = map[string]any{ops.Silent: c.storeSilent}
			callAt(t, mod, m, f, r, 0)
			m.regs[ops.Silent] = c.silent
			callAt(t, mod, m, f, r, 0)
			if reused := r.runs == 1; reused != c.reused {
				t.Errorf("reused %v, want %v", reused, c.reused)
			}
		})
	}
}

// TestObserved checks that an observed call reports its beginning and its end to the observation,
// with its outcome, its end and how far its call examined the input, which joins the frontier of the
// enclosing call.
func TestObserved(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "observed", ops.Document, ops.Observe)
	for _, ok := range []bool{true, false} {
		t.Run(fmt.Sprint(ok), func(t *testing.T) {
			m := newMachine()
			m.high = 12
			var at int64 = -1
			r := &trule{id: 2, body: func(m *machine) (any, bool) {
				at = m.high
				m.events = append(m.events, "body")
				m.advance(2)
				m.high = 9
				return "v", ok
			}}
			v, got := callAt(t, mod, m, f, r, 3)
			if got != ok || (ok && v != "v") || r.runs != 1 {
				t.Errorf("%v %v after %d evaluations, want the body's result after one", v, got, r.runs)
			}
			want := []string{"enter 2 at 3 level 0", "body", fmt.Sprintf("exit %v at 5 examined 9 true", ok)}
			if !slices.Equal(m.events, want) {
				t.Errorf("observed %q, want %q", m.events, want)
			}
			if at != 3 || m.high != 12 {
				t.Errorf("the body began with the frontier %d and the call left %d, want 3 and 12", at, m.high)
			}
		})
	}
}

// TestDocument checks that a Document call starts the frontier of the call past its position and
// its reuse at after edits, and leaves the enclosing call the farther frontier and the shorter
// reuse, when the call returns and when an operation in it interrupts the parse.
func TestDocument(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "document", ops.Document)
	checkDocument(t, mod, f)
}

// TestDocumentCleanupMutant checks that TestDocument's checks fail for a document call whose
// cleanup runs before each return of its body but not when an operation interrupts the parse.
func TestDocumentCleanupMutant(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "document", ops.Document)
	stmts := f.Body.Stmts
	p := stmts[len(stmts)-1].(*exec.Protect)
	var unprotect func(b *exec.Block)
	unprotect = func(b *exec.Block) {
		var out []exec.Stmt
		for _, st := range b.Stmts {
			switch st := st.(type) {
			case *exec.Return:
				out = append(out, p.Finally.Stmts...)
			case *exec.If:
				unprotect(st.Then)
			}
			out = append(out, st)
		}
		b.Stmts = out
	}
	unprotect(p.Body)
	f.Body.Stmts = append(stmts[:len(stmts)-1:len(stmts)-1], p.Body.Stmts...)
	ft := &fakeT{}
	checkDocument(ft, mod, f)
	if !ft.failed {
		t.Error("the checks pass for a document call without its cleanup on an interruption")
	}
}

// fakeT records the failures of checks run on a mutant.
type fakeT struct {
	testing.TB
	failed bool
}

func (f *fakeT) Helper()                           {}
func (f *fakeT) Errorf(format string, args ...any) { f.failed = true }
func (f *fakeT) Fatal(args ...any)                 { f.failed = true; panic(f) }

func checkDocument(t testing.TB, mod *exec.Module, f *exec.Func) {
	t.Helper()
	for _, c := range []struct {
		name        string
		memo, ok    bool
		interrupt   bool
		high, reuse int64
		callHigh    int64
		callReuse   int64
		wantHigh    int64
		wantReuse   int64
	}{
		{name: "memoized", memo: true, ok: true, high: 2, reuse: 1, callHigh: 9, callReuse: 0, wantHigh: 9, wantReuse: 1},
		{name: "unmemoized failure", ok: false, high: 12, reuse: 2, callHigh: 9, callReuse: 1, wantHigh: 12, wantReuse: 2},
		{name: "interrupted", memo: true, interrupt: true, high: 2, reuse: 1, callHigh: 9, callReuse: 0, wantHigh: 9, wantReuse: 1},
		{name: "interrupted unmemoized", interrupt: true, high: 12, reuse: 2, callHigh: 9, callReuse: 1, wantHigh: 12, wantReuse: 2},
	} {
		func() {
			defer func() {
				if x := recover(); x != nil && x != t {
					panic(x)
				}
			}()
			m := newMachine()
			m.high, m.reuse, m.docMemo, m.docActive = c.high, c.reuse, c.memo, true
			var atHigh, atReuse int64 = -1, -1
			r := &trule{id: 2, memo: true, body: func(m *machine) (any, bool) {
				atHigh, atReuse = m.high, m.reuse
				m.advance(2)
				m.high, m.reuse = c.callHigh, c.callReuse
				if c.interrupt {
					panic(exec.Interrupt{Reason: "limit"})
				}
				return "v", c.ok
			}}
			res, err := exec.Run(mod, f, &start{machine: m, pos: 3}, r, int64(0))
			if c.interrupt {
				if _, ok := err.(exec.Interrupt); !ok {
					t.Errorf("%s: %v, %v, want an interruption", c.name, res, err)
				}
			} else if err != nil || res[1] != c.ok {
				t.Errorf("%s: %v, %v, want success %v", c.name, res, err, c.ok)
			}
			if atHigh != 4 || atReuse != 0 {
				t.Errorf("%s: the call began with the frontier %d and reuse %d, want 4 and 0", c.name, atHigh, atReuse)
			}
			if m.high != c.wantHigh || m.reuse != c.wantReuse {
				t.Errorf("%s: the call left the frontier %d and reuse %d, want %d and %d", c.name, m.high, m.reuse, c.wantHigh, c.wantReuse)
			}
			if r.runs != 1 {
				t.Errorf("%s: evaluates the body %d times, want once", c.name, r.runs)
			}
		}()
	}
}

// TestMemoizedDocument checks the Document bookkeeping of a memoized call: the call that stores an
// entry records with it the frontier and the reuse of the call, and the call that reuses it extends
// its own frontier to the entry's, limits its reuse to the entry's and counts the reuse. Outside a
// Document parse the call keeps no such record.
func TestMemoizedDocument(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "memoized", ops.Document, ops.Observe, ops.Stream)
	for _, active := range []bool{true, false} {
		m := newMachine()
		m.docActive = active
		r := &trule{id: 1, memo: true, body: func(m *machine) (any, bool) {
			m.advance(2)
			m.high = max(m.high, 4) // the body examined the input up to 4, before the end of the call
			m.reuse = max(m.reuse, 1)
			return "v", true
		}}
		m.high, m.reuse = 2, 0
		if v, ok := callAt(t, mod, m, f, r, 3); !ok || v != "v" {
			t.Fatalf("active=%v: %v %v", active, v, ok)
		}
		e := m.lastEntry(t)
		if active && (m.examined[e] != 6 || m.reuses[e] != 1) {
			t.Errorf("the entry records the frontier %d and reuse %d, want 6 (past its end) and 1", m.examined[e], m.reuses[e])
		}
		if !active && (len(m.examined) != 0 || len(m.reuses) != 0) {
			t.Errorf("outside a Document the entry records %v and %v", m.examined, m.reuses)
		}
		// A later call at the position reuses the entry from a call that examined less.
		m.high, m.reuse = 3, 0
		if v, ok := callAt(t, mod, m, f, r, 3); !ok || v != "v" || m.pos() != 5 || r.runs != 1 || m.memoHit != 1 {
			t.Fatalf("active=%v: reuse gives %v %v at %d after %d evaluations and %d hits", active, v, ok, m.pos(), r.runs, m.memoHit)
		}
		want := struct{ high, reuse int64 }{6, 1}
		wantReused := 1
		if !active {
			want.high, want.reuse, wantReused = 3, 0, 0
		}
		if m.high != want.high || m.reuse != want.reuse || m.reused != wantReused {
			t.Errorf("active=%v: the reuse leaves the frontier %d, reuse %d and %d reuses, want %d, %d and %d",
				active, m.high, m.reuse, m.reused, want.high, want.reuse, wantReused)
		}
	}
}

// TestMemoizedDocumentLeader checks how long a leader's result may be reused: until the next edit
// when its growth read only results that do not change (it is settled), and otherwise only in this
// parse, which then removes the entry when it ends. A reused growing entry also limits the reuse of
// the call that reads it to until the next edit.
func TestMemoizedDocumentLeader(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "memoized", ops.Document, ops.Observe, ops.Stream)
	for _, settled := range []bool{true, false} {
		m := newMachine()
		m.docActive = true
		n := 0
		r := &trule{id: 2, memo: true, leader: true, body: func(m *machine) (any, bool) {
			if n >= 2 {
				return nil, false
			}
			if !settled {
				m.readDepth = 0 // the growth read an unstable result
			}
			m.advance(int64(n + 1))
			n++
			return fmt.Sprint("v", n), true
		}}
		if v, ok := callAt(t, mod, m, f, r, 2); !ok || v != "v2" {
			t.Fatalf("settled=%v: %v %v", settled, v, ok)
		}
		e := m.lastEntry(t)
		wantReuse, wantOnly := int64(1), []string(nil)
		if !settled {
			wantReuse, wantOnly = 2, []string{"2 reuse 2"}
		}
		if m.reuse != wantReuse || m.reuses[e] != wantReuse || !slices.Equal(m.parseOnly, wantOnly) {
			t.Errorf("settled=%v: reuse %d, the entry's %d, reused only in the parse %q; want %d, %d and %q",
				settled, m.reuse, m.reuses[e], m.parseOnly, wantReuse, wantReuse, wantOnly)
		}
	}
	// L = L "x" / "a", where "x" matches while L ends before 3: the call of L in its body reuses the
	// growing seed, which limits its reuse to until the next edit.
	m := newMachine()
	m.docActive = true
	var leader *trule
	var after []int64
	leader = &trule{id: 2, memo: true, leader: true, body: func(m *machine) (any, bool) {
		start := m.pos()
		saved := m.regs
		m.reuse = 0
		_, ok := callAt(t, mod, m, f, leader, start)
		pos := m.pos()
		m.regs = saved
		m.setPos(pos)
		after = append(after, m.reuse)
		if ok && m.pos() < 3 {
			m.advance(1)
			return "x", true
		}
		m.setPos(start)
		m.advance(1)
		return "a", true
	}}
	if _, ok := callAt(t, mod, m, f, leader, 0); !ok || m.pos() != 3 {
		t.Fatalf("leader %v at %d, want a match to 3", ok, m.pos())
	}
	if len(after) == 0 || m.reused != len(after) || slices.ContainsFunc(after, func(u int64) bool { return u != 1 }) {
		t.Errorf("the reads of the growing seed counted %d reuses and left the reuses %v, want one each and until the next edit", m.reused, after)
	}
}

// lastEntry returns the entry that the last call stored or grew.
func (m *machine) lastEntry(t *testing.T) *ref.Entry {
	t.Helper()
	if m.last == nil {
		t.Fatal("no entry stored")
	}
	return m.last
}

// TestGrowth checks that a leader's call evaluates its body while the match gets longer, charging
// one growth step per evaluation, and ends at the longest match with its errors only.
func TestGrowth(t *testing.T) {
	mod := Build()
	for _, on := range [][]string{nil, {ops.Stream}} {
		for _, pruned := range []bool{false, true} {
			if pruned && on == nil {
				continue // outside streams a growth leaves its entry
			}
			t.Run(fmt.Sprintf("%v pruned=%v", on, pruned), func(t *testing.T) {
				f := composition(t, mod, "call", on...)
				m := newMachine()
				m.pruned = pruned
				lens := []int64{1, 3, 3}
				n := 0
				r := &trule{id: 2, memo: true, leader: true, body: func(m *machine) (any, bool) {
					if n >= len(lens) {
						return nil, false
					}
					m.recover(fmt.Sprint("e", n))
					m.advance(lens[n])
					n++
					return fmt.Sprint("v", n), true
				}}
				v, ok := callAt(t, mod, m, f, r, 2)
				if !ok || v != "v2" || m.pos() != 5 {
					t.Errorf("%v %v at %d, want v2 true at 5", v, ok, m.pos())
				}
				if !slices.Equal(m.rec, []string{"e1"}) {
					t.Errorf("recovered %v, want the errors of the longest match", m.rec)
				}
				if r.runs != 3 || m.steps != 3 {
					t.Errorf("%d evaluations and %d growth steps, want 3 and 3", r.runs, m.steps)
				}
			})
		}
	}
}

// TestGrowthMemberReads checks the memoization of a member of a left-recursive cycle: while the
// leader grows at the position, the member is evaluated at every step and its results, which read
// the growing seed, are not stored; after the growth its result reads only the final result of the
// leader and is stored, so the next call reuses it.
func TestGrowthMemberReads(t *testing.T) {
	mod := Build()
	f := composition(t, mod, "call")
	m := newMachine()
	// call runs a nested call of r from a body, at the position of the machine.
	call := func(m *machine, r *trule) (any, bool) {
		saved := m.regs
		v, ok := callAt(t, mod, m, f, r, m.pos())
		pos := m.pos()
		m.regs = saved
		m.setPos(pos)
		return v, ok
	}
	// The cycle is L = M "x" / "a" with M = L, where "x" matches while L ends before 3.
	var leader, member *trule
	leader = &trule{id: 2, memo: true, leader: true, body: func(m *machine) (any, bool) {
		start := m.pos()
		if _, ok := call(m, member); ok {
			if m.pos() >= 3 {
				return nil, false
			}
			m.advance(1)
			return "x", true
		}
		m.setPos(start)
		m.advance(1)
		return "a", true
	}}
	member = &trule{id: 1, memo: true, member: true, body: func(m *machine) (any, bool) {
		return call(m, leader)
	}}
	if _, ok := callAt(t, mod, m, f, leader, 0); !ok || m.pos() != 3 {
		t.Fatalf("leader %v at %d, want a match to 3", ok, m.pos())
	}
	if member.runs != 4 {
		t.Errorf("member evaluated %d times in the growth, want 4: once per step", member.runs)
	}
	// The reads of the seed count in the caller, where the growth is over: they keep nothing from
	// memoization at the depth of the caller.
	if m.growths != 0 || m.readDepth <= m.growths {
		t.Errorf("growth state %d, %d after the call, want no growth and no unresolved read", m.growths, m.readDepth)
	}
	for i, want := range []int{5, 5} {
		if _, ok := callAt(t, mod, m, f, member, 0); !ok || m.pos() != 3 {
			t.Fatalf("member %v at %d, want a match to 3", ok, m.pos())
		}
		if member.runs != want {
			t.Errorf("call %d after the growth: member evaluated %d times in all, want %d", i, member.runs, want)
		}
	}
}
