package ops

import "zeroparse.dev/zp/internal/ir/exec"

// State components of the transitional operations.
const (
	// Rules is the grammar's rules: their descriptors and bodies.
	Rules = "rules"
	// Exp is the expectation record.
	Exp = "expected"
	// Call is the call bookkeeping of the target outside the pilot: the tracked path of Documents and
	// observation, and the work charged per call.
	Call = "call"
	// Obs is the observation of a parse.
	Obs = "observe"
	// Growth is the left-recursion growths in progress and the reads of the current call.
	Growth = "growth"
)

// Names of the handle types of the transitional operations.
const (
	Rule  = "Rule"
	Body  = "Body"
	Mark  = "ExpMark"
	Set   = "ExpSet"
	Kept  = "ExpKept"
	Reuse = "Reuse"
)

// Registers of the parser VM.
const (
	Pos    = "pos"
	Silent = "silent"
)

// Transitional adds to m, a module made by Pilot, the registers the call protocol reads and the
// transitional operations it calls: existing functions of the hand-written runtimes, declared with
// their current behavior and no conformance cases (design 065, Migration Order), until the stage
// that migrates their mechanism replaces them. A rule body runs grammar code, which may change any
// state; the declarations name only the operation's own component and the registers.
func Transitional(m *exec.Module) {
	m.AddRegister(Pos, exec.TInt, "The position of the parse in the input.")
	m.AddRegister(Silent, exec.TInt, "The depth of lookaheads, inside which expectations are not recorded.")
	m.AddComponent(Rules, `The rules of the grammar: their descriptors and bodies.`)
	m.AddComponent(Exp, `The expectation record: the expectations at the farthest position.`)
	m.AddComponent(Call, `The call bookkeeping of the target outside the call protocol: the tracked path of
Documents and observation, and the work charged per call.`)
	m.AddComponent(Obs, `The observation of a parse.`)
	m.AddComponent(Growth, `The left-recursion growths in progress, the position of the innermost one, and
the least depth of the growing or unstable results that the current call has read.`)

	rule := m.AddHandle(Rule, Rules, "A rule's descriptor.")
	body := m.AddHandle(Body, Rules, "The invocation of a rule's body that a generated call passes.")
	mark := m.AddHandle(Mark, Exp, "The record that an isolation set aside.")
	set := m.AddHandle(Set, Exp, "The expectations an isolation recorded, valid until the next record.")
	kept := m.AddHandle(Kept, Exp, "Expectations kept beyond the record, as a memo entry keeps them.")
	reuse := m.AddHandle(Reuse, Call, `How long the results of a Document call may be reused: after edits, until
the next edit, or only in this parse.`)
	value, errs, entry, values := exec.THandle(Value), exec.THandle(Errs), exec.THandle(Entry), exec.THandle(Values)
	result := []exec.Type{value, exec.TBool}
	regs := []string{Pos, Silent}
	add := func(o *exec.Op) {
		o.Transitional = true
		m.AddOp(o)
	}
	ruleArg := []exec.Param{exec.P("r", rule)}
	call := []exec.Param{exec.P("r", rule), exec.P("min", exec.TInt)}

	add(&exec.Op{Name: "call.tracked", Component: Call, Results: []exec.Type{exec.TBool}, Reads: []string{Call},
		Features: []string{Document},
		Contract: `Reports whether a rule call takes the target's tracked path: in a Document parse, and in the
node runtime also in an observed parse.`})
	add(&exec.Op{Name: "call.track", Component: Call, Params: call, Results: result, Writes: append([]string{Call}, regs...),
		Features: []string{Document}, NoReturn: true,
		Contract: `Calls the rule at the binding level min on the target's tracked path and returns its result.`})
	add(&exec.Op{Name: "call.trackBody", Component: Call, Params: []exec.Param{exec.P("r", rule), exec.P("inv", body)},
		Results: result, Writes: append([]string{Call}, regs...), Features: []string{Document}, NoReturn: true,
		Contract: `Calls the rule through the invocation inv on the target's tracked path and returns its
result.`})
	add(&exec.Op{Name: "call.spend", Component: Call, Writes: []string{Call}, NoReturn: true,
		Cost:     exec.Cost{Const: 1},
		Contract: `Charges the work of a rule call: one unit, with the checks of the target's Spend.`})
	add(&exec.Op{Name: "call.first", Component: Call, Params: ruleArg, Results: []exec.Type{exec.TBool},
		Reads: []string{Call}, Writes: []string{Call},
		Contract: `Records a call of the rule at the position and reports whether its memoization is deferred:
seen.first for a rule whose memoization the target defers, false for one it memoizes eagerly (in a
stream, or for a rule without a seen index).`})
	add(&exec.Op{Name: "call.growStep", Component: Call, Writes: []string{Call}, NoReturn: true,
		Cost: exec.Cost{Const: 1},
		Contract: `Charges a growth step of a left-recursion leader, before each evaluation of its body
(the evaluation of the seed included): one unit, with the checks of the target's Spend.`})

	for _, a := range []struct{ name, doc string }{
		{"rule.memo", "Reports whether the rule is memoized."},
		{"rule.leader", "Reports whether the rule leads a left-recursive cycle."},
		{"rule.plain", "Reports whether the rule has no captures, so that its plain body serves its invocation."},
		{"rule.docMemo", "Reports whether a Document parse memoizes the rule."},
	} {
		add(&exec.Op{Name: a.name, Component: Rules, Params: ruleArg, Results: []exec.Type{exec.TBool}, Contract: a.doc})
	}
	add(&exec.Op{Name: "rule.id", Component: Rules, Params: ruleArg, Results: []exec.Type{exec.TInt},
		Contract: `Returns the rule's index, the rule of its memo keys.`})
	add(&exec.Op{Name: "rule.invoke", Component: Rules, Params: call, Results: result, Writes: append([]string{Rules}, regs...),
		NoReturn: true,
		Contract: `Runs the rule's body at the binding level min in a frame of its own and makes the rule's
value; on failure the position is unspecified.`})
	add(&exec.Op{Name: "rule.invokePlain", Component: Rules, Params: call, Results: result, Writes: append([]string{Rules}, regs...),
		NoReturn: true,
		Contract: `Runs the plain body of a rule without captures and returns its result, at the start
position on failure.`})
	add(&exec.Op{Name: "rule.run", Component: Rules, Params: []exec.Param{exec.P("inv", body)}, Results: result,
		Writes: append([]string{Rules}, regs...), NoReturn: true,
		Contract: `Runs the invocation inv of a rule at binding level 0 and returns its result.`})
	add(&exec.Op{Name: "env.memoKey", Component: Env, Params: ruleArg, Results: []exec.Type{values}, Reads: []string{Env},
		Contract: `Returns the values of the variables the rule reads, as its memo key.`})

	recMark := []exec.Param{exec.P("mark", exec.TInt)}
	add(&exec.Op{Name: "rec.mark", Component: Rec, Results: []exec.Type{exec.TInt}, Reads: []string{Rec},
		Contract: `Returns the number of recovered errors, which rec.truncate and rec.since take.`})
	add(&exec.Op{Name: "rec.truncate", Component: Rec, Params: recMark, Writes: []string{Rec},
		Contract: `Drops the errors recovered after the mark.`})
	add(&exec.Op{Name: "rec.since", Component: Rec, Params: recMark, Results: []exec.Type{errs}, Reads: []string{Rec},
		Writes: []string{Rec}, NoReturn: true,
		Contract: `Returns a copy of the errors recovered after the mark, charging their copy as the target does.`})
	add(&exec.Op{Name: "rec.add", Component: Rec, Params: []exec.Param{exec.P("errs", errs)}, Writes: []string{Rec},
		NoReturn: true,
		Contract: `Records the errors again, after the recovered errors, charging as the target does.`})
	add(&exec.Op{Name: "rec.none", Component: Rec, Params: []exec.Param{exec.P("errs", errs)}, Results: []exec.Type{exec.TBool},
		Contract: `Reports whether the list of errors is empty.`})

	add(&exec.Op{Name: "exp.isolate", Component: Exp, Params: []exec.Param{exec.P("pos", exec.TInt)}, Results: []exec.Type{mark},
		Writes:   []string{Exp},
		Contract: `Sets the record aside and starts an empty one at pos.`})
	add(&exec.Op{Name: "exp.unisolate", Component: Exp, Params: []exec.Param{exec.P("m", mark)}, Results: []exec.Type{exec.TInt, set},
		Writes: []string{Exp},
		Contract: `Ends the record that the isolation m started and restores the one set aside; returns the
farthest position of the ended record and its expectations.`})
	add(&exec.Op{Name: "exp.keep", Component: Exp, Params: []exec.Param{exec.P("s", set)}, Results: []exec.Type{kept},
		Contract: `Returns the expectations of s as a set that outlives the record.`})
	add(&exec.Op{Name: "exp.merge", Component: Exp, Params: []exec.Param{exec.P("far", exec.TInt), exec.P("s", set)},
		Writes: []string{Exp}, NoReturn: true,
		Contract: `Adds the expectations of s at far to the record.`})
	add(&exec.Op{Name: "exp.mergeKept", Component: Exp, Params: []exec.Param{exec.P("far", exec.TInt), exec.P("k", kept)},
		Writes: []string{Exp}, NoReturn: true,
		Contract: `Adds the kept expectations k at far to the record.`})

	entryArg := []exec.Param{exec.P("e", entry)}
	add(&exec.Op{Name: "memo.far", Component: Memo, Params: entryArg, Results: []exec.Type{exec.TInt}, Reads: []string{Memo},
		Contract: `Returns the farthest position of the expectations of entry e.`})
	add(&exec.Op{Name: "memo.expected", Component: Memo, Params: entryArg, Results: []exec.Type{kept}, Reads: []string{Memo},
		Contract: `Returns the expectations of entry e.`})
	add(&exec.Op{Name: "memo.setExpected", Component: Memo, Params: append(entryArg, exec.P("far", exec.TInt), exec.P("k", kept)),
		Writes:   []string{Memo},
		Contract: `Records the expectations k at far of the call that computed entry e.`})
	add(&exec.Op{Name: "memo.grown", Component: Memo, Params: []exec.Param{exec.P("rule", exec.TInt), exec.P("pos", exec.TInt),
		exec.P("level", exec.TInt), exec.P("key", values)}, Results: []exec.Type{entry}, Reads: []string{Memo},
		Contract: `Returns the entry that a finished growth left under the key, or the null entry when a
stream commit pruned it.`,
		Requires: `A growth under the key has just finished; outside streams it left its entry.`})
	add(&exec.Op{Name: "memo.detached", Component: Memo, Results: []exec.Type{entry},
		Contract: `Returns a new entry under no key, which no lookup finds.`})

	add(&exec.Op{Name: "memo.recordDoc", Component: Memo, Params: entryArg, Reads: []string{Call}, Writes: []string{Memo},
		Features: []string{Document}, Contract: `Records with the entry the examined frontier of the current call and how
long its result may be reused, for the reuse of the entry (doc.readEntry).`})
	add(&exec.Op{Name: "memo.forgetDoc", Component: Memo, Params: entryArg, Writes: []string{Memo}, Features: []string{Document},
		Contract: `Forgets the Document record of the entry e, which the call replaces, where the target keeps
the records beside the entries.`})
	add(&exec.Op{Name: "lr.stale", Component: Growth, Params: []exec.Param{exec.P("r", rule), exec.P("pos", exec.TInt)},
		Results: []exec.Type{exec.TBool}, Reads: []string{Growth, Rules},
		Contract: `Reports whether the result of the rule at pos is evaluated again instead of reused: the rule
is a member of a left-recursive cycle, other than its leader, and the innermost growth is at pos.`})
	add(&exec.Op{Name: "lr.read", Component: Growth, Params: entryArg, Reads: []string{Growth, Memo}, Writes: []string{Growth},
		Contract: `Records that the current call read the result of entry e: the depth of a growing entry, or
zero for an unstable one, lowers the least depth read.`})
	add(&exec.Op{Name: "lr.depth", Component: Growth, Results: []exec.Type{exec.TInt}, Reads: []string{Growth},
		Contract: `Returns the number of growths in progress.`})
	add(&exec.Op{Name: "lr.enter", Component: Growth, Results: []exec.Type{exec.TInt}, Reads: []string{Growth},
		Writes: []string{Growth},
		Contract: `Starts tracking the reads of a call: returns the least depth read so far, for lr.leave, and
resets it to none.`})
	add(&exec.Op{Name: "lr.leave", Component: Growth, Params: []exec.Param{exec.P("read", exec.TInt)},
		Reads: []string{Growth}, Writes: []string{Growth},
		Contract: `Ends tracking the reads of a call, which also count as reads of the call that made it: the
least depth read becomes at most read.`})
	add(&exec.Op{Name: "lr.changeable", Component: Growth, Params: []exec.Param{exec.P("depth", exec.TInt)},
		Results: []exec.Type{exec.TBool}, Reads: []string{Growth},
		Contract: `Reports whether the result of a call that began with depth growths in progress can change:
it read the seed of one of them or an unstable result, so it is not memoized.`})
	add(&exec.Op{Name: "lr.begin", Component: Growth, Params: []exec.Param{exec.P("pos", exec.TInt)},
		Results: []exec.Type{exec.TInt}, Reads: []string{Growth}, Writes: []string{Growth},
		Contract: `Starts a growth at pos: it becomes the innermost growth. Returns the position of the
enclosing innermost growth, for lr.end.`})
	add(&exec.Op{Name: "lr.end", Component: Growth, Params: []exec.Param{exec.P("prev", exec.TInt)},
		Reads: []string{Growth}, Writes: []string{Growth},
		Contract: `Ends the innermost growth and restores prev as the position of the innermost growth.`,
		Requires: `prev is the result of the matching lr.begin.`})
	add(&exec.Op{Name: "memo.mark", Component: Memo, Params: entryArg, Reads: []string{Growth}, Writes: []string{Memo},
		Contract: `Records in the growing entry e the number of growths in progress, its depth.`})
	add(&exec.Op{Name: "memo.settle", Component: Memo, Params: entryArg, Reads: []string{Growth, Memo}, Writes: []string{Memo},
		Contract: `Marks the entry e of a growth that has just ended unstable if the growth read the seed of an
enclosing growth or an unstable result and the entry is reused in one kind of context only: it is
silent or has recovered errors.`,
		Requires: `e was finished by memo.finishSeed, and lr.end has ended its growth.`})

	add(&exec.Op{Name: "obs.memoHit", Component: Obs, Writes: []string{Obs}, Features: []string{Observe},
		Contract: `Counts a reuse of a memo entry in the observation, if the parse is observed.`})
	add(&exec.Op{Name: "obs.active", Component: Obs, Results: []exec.Type{exec.TBool}, Reads: []string{Obs}, Features: []string{Observe},
		Contract: `Reports whether the parse is observed.`})
	add(&exec.Op{Name: "obs.enter", Component: Obs, Params: []exec.Param{exec.P("r", rule), exec.P("pos", exec.TInt), exec.P("min", exec.TInt)},
		Writes: []string{Obs}, Features: []string{Observe}, NoReturn: true,
		Contract: `Reports to the observation of the parse that a call of the rule at pos and binding level
min begins.`,
		Requires: `The parse is observed.`})
	add(&exec.Op{Name: "obs.exit", Component: Obs, Params: []exec.Param{exec.P("ok", exec.TBool), exec.P("pos", exec.TInt),
		exec.P("examined", exec.TInt), exec.P("known", exec.TBool)}, Writes: []string{Obs}, Features: []string{Observe}, NoReturn: true,
		Contract: `Reports to the observation of the parse that the call that the last unmatched obs.enter began
ends, with its success ok, at pos, and with the end of the input it examined if known.`,
		Requires: `The parse is observed.`})
	add(&exec.Op{Name: "doc.examinedBegin", Component: Call, Results: []exec.Type{exec.TInt}, Reads: []string{Call},
		Writes: []string{Call}, Features: []string{Document},
		Contract: `Starts measuring how far an observed call examines the input: in a Document parse, the
frontier of the call starts at the position. Returns the enclosing call's frontier, for
doc.examinedEnd. The call itself, and any memo entry it reads or records, extends the frontier as in
an unobserved parse, so that observation does not change reuse.`})
	add(&exec.Op{Name: "doc.examinedEnd", Component: Call, Params: []exec.Param{exec.P("enclosing", exec.TInt)},
		Results: []exec.Type{exec.TInt, exec.TBool}, Reads: []string{Call}, Writes: []string{Call}, Features: []string{Document},
		Contract: `Ends measuring the frontier of an observed call and merges it into the enclosing one; returns
the call's frontier and true in a Document parse, and false otherwise. In a provisional context, the
result is raised to the position without changing the frontier the Document keeps.`,
		Requires: `enclosing is the result of the matching doc.examinedBegin.`})
	add(&exec.Op{Name: "doc.enter", Component: Call, Params: []exec.Param{exec.P("pos", exec.TInt)},
		Results: []exec.Type{exec.TInt, reuse}, Reads: []string{Call}, Writes: []string{Call}, Features: []string{Document},
		Contract: `Begins a rule call at pos in a Document parse: returns the examined frontier and the reuse of
the enclosing call, and starts the call's frontier past pos and its reuse at after edits.`})
	add(&exec.Op{Name: "doc.leave", Component: Call, Params: []exec.Param{exec.P("high", exec.TInt), exec.P("reuse", reuse)},
		Reads: []string{Call}, Writes: []string{Call}, Features: []string{Document},
		Contract: `Ends a rule call in a Document parse: the enclosing call's frontier becomes the farther of high
and the call's, and its reuse the shorter of reuse and the call's.`,
		Requires: `high and reuse are the results of the matching doc.enter.`})
	doc := []string{Document}
	inDoc := `The parse reads a Document.`
	reuseArg := []exec.Param{exec.P("u", reuse)}
	add(&exec.Op{Name: "doc.active", Component: Call, Results: []exec.Type{exec.TBool}, Reads: []string{Call}, Features: doc,
		Contract: `Reports whether the parse reads a Document.`})
	add(&exec.Op{Name: "doc.touch", Component: Call, Params: []exec.Param{exec.P("end", exec.TInt)}, Reads: []string{Call},
		Writes: []string{Call}, Features: doc,
		Contract: `Extends the examined frontier of the current call to end, if end is past it.`})
	add(&exec.Op{Name: "doc.readEntry", Component: Call, Params: entryArg, Reads: []string{Call, Memo}, Writes: []string{Call},
		Features: doc,
		Contract: `Records that the current call read the entry e: extends its examined frontier to the one recorded
with e and limits its reuse to the one recorded with e.`, Requires: `The parse reads a Document, and e has a record (memo.recordDoc).`})
	add(&exec.Op{Name: "doc.reuse", Component: Call, Results: []exec.Type{reuse}, Reads: []string{Call}, Features: doc,
		Contract: `Returns how long the result of the current call may be reused.`, Requires: inDoc})
	add(&exec.Op{Name: "doc.setReuse", Component: Call, Params: reuseArg, Writes: []string{Call}, Features: doc,
		Contract: `Sets how long the result of the current call may be reused to u.`, Requires: inDoc})
	add(&exec.Op{Name: "doc.limitReuse", Component: Call, Params: reuseArg, Reads: []string{Call}, Writes: []string{Call},
		Features: doc,
		Contract: `Limits how long the result of the current call may be reused to u: the reuse becomes the
shorter of the two.`, Requires: inDoc})
	add(&exec.Op{Name: "doc.reused", Component: Call, Writes: []string{Call}, Features: doc,
		Contract: `Counts a reused memo entry in the statistics of the Document.`, Requires: inDoc})
	add(&exec.Op{Name: "doc.evaluated", Component: Call, Writes: []string{Call}, Features: doc,
		Contract: `Counts an evaluated rule body in the statistics of the Document.`, Requires: inDoc})
	add(&exec.Op{Name: "doc.parseOnly", Component: Call, Params: []exec.Param{exec.P("pos", exec.TInt), exec.P("e", entry)},
		Writes: []string{Call}, Features: doc,
		Contract: `Records the entry e stored at pos as reused only in this parse, so that the parse removes it
from the memo when it ends.`, Requires: inDoc})
	for _, a := range []struct{ name, doc string }{
		{"reuse.untilEdit", "Returns the reuse until the next edit."},
		{"reuse.inParse", "Returns the reuse only in this parse."},
	} {
		add(&exec.Op{Name: a.name, Component: Call, Results: []exec.Type{reuse}, Features: doc, Contract: a.doc})
	}
	add(&exec.Op{Name: "call.body", Component: Call, Params: call, Results: result, Writes: append([]string{Call}, regs...),
		NoReturn: true,
		Contract: `Calls the rule at the binding level min without observation: the call, or the plain invocation,
that the target's observed function wraps.`})
}
