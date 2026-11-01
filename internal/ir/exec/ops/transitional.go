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
	// Growth is the left-recursion growths in progress and the dependency memo of their contexts.
	Growth = "growth"
)

// Names of the handle types of the transitional operations.
const (
	Rule = "Rule"
	Body = "Body"
	Mark = "ExpMark"
	Set  = "ExpSet"
	Kept = "ExpKept"
	// Ctx is the context state of a memoized call (ctx.enter).
	Ctx = "Ctx"
	// Res is a result that a context keeps, a seed included, or the result of a growth.
	Res = "CtxResult"
	// Grow is the state of the growth of a leader (grow.begin).
	Grow = "Grow"
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
	m.AddComponent(Growth, `The left-recursion growths in progress with their seeds, the frames that record
what the calls in their contexts depend on, and the tables of the results kept in each context (the
dependency memo).`)

	rule := m.AddHandle(Rule, Rules, "A rule's descriptor.")
	body := m.AddHandle(Body, Rules, "The invocation of a rule's body that a generated call passes.")
	mark := m.AddHandle(Mark, Exp, "The record that an isolation set aside.")
	set := m.AddHandle(Set, Exp, "The expectations an isolation recorded, valid until the next record.")
	kept := m.AddHandle(Kept, Exp, "Expectations kept beyond the record, as a memo entry keeps them.")
	ctx := m.AddHandle(Ctx, Growth, `The context state of a memoized call: the growths of its rule's cycle at its
position, their present set, and whether the call records its dependencies.`)
	res := m.AddHandle(Res, Growth, `A result that a context keeps (a seed, or the result of a call or a growth step
that read a seed), or the last result of a growth.`)
	grow := m.AddHandle(Grow, Growth, "The state of the growth of a left-recursion leader.")
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
	add(&exec.Op{Name: "memo.recordDoc", Component: Memo, Params: entryArg, Reads: []string{Call}, Writes: []string{Memo},
		Features: []string{Document}, Contract: `Records with the entry the examined frontier of the current call, for the
reuse of the entry (doc.readEntry).`})
	add(&exec.Op{Name: "memo.forgetDoc", Component: Memo, Params: entryArg, Writes: []string{Memo}, Features: []string{Document},
		Contract: `Forgets the Document record of the entry e, which the call replaces, where the target keeps
the records beside the entries.`})
	ctxArg := exec.P("c", ctx)
	resArg := []exec.Param{exec.P("s", res)}
	add(&exec.Op{Name: "ctx.enter", Component: Growth, Params: call, Results: []exec.Type{ctx, res},
		Reads: []string{Growth, Rules}, Writes: []string{Growth},
		Contract: `Begins a memoized call of the rule at the position: returns its context state and, if the
rule is a leader that grows at the position, its current seed, which the call reads (with the
dependency on its identity); otherwise the null result. The context of a call of a rule of a
left-recursive cycle is the growths of its cycle in progress at the position; another call has
none.`})
	add(&exec.Op{Name: "ctx.found", Component: Growth, Params: resArg, Results: []exec.Type{exec.TBool},
		Contract: `Reports whether s is a result, not the null result.`})
	add(&exec.Op{Name: "ctx.ok", Component: Growth, Params: resArg, Results: []exec.Type{exec.TBool}, Reads: []string{Growth},
		Contract: `Returns whether the result s succeeded.`})
	add(&exec.Op{Name: "ctx.value", Component: Growth, Params: resArg, Results: []exec.Type{value}, Reads: []string{Growth},
		Contract: `Returns the value of the result s.`})
	add(&exec.Op{Name: "ctx.replay", Component: Growth, Params: append(resArg, exec.P("seed", exec.TBool)),
		Reads: []string{Growth}, Writes: append([]string{Growth}, regs...), NoReturn: true,
		Contract: `Applies the result s as the call that it records would have: counts a reuse, extends the
examined frontier of a Document parse, adds its expectations unless it is a seed, and on success moves
the position to its end and records its recovered errors again.`})
	add(&exec.Op{Name: "ctx.allows", Component: Growth, Params: append([]exec.Param{ctxArg}, entryArg...),
		Results: []exec.Type{exec.TBool}, Reads: []string{Growth, Memo},
		Contract: `Reports whether the call with the context state c may reuse the memo entry e: always outside a
context, and in one only when the leaders growing there are among those that grew where e was stored
(its present set).`})
	add(&exec.Op{Name: "ctx.active", Component: Growth, Params: []exec.Param{ctxArg}, Results: []exec.Type{exec.TBool},
		Contract: `Reports whether the call with the context state c is in a context.`})
	add(&exec.Op{Name: "ctx.lookup", Component: Growth, Params: []exec.Param{ctxArg, exec.P("r", rule), exec.P("min", exec.TInt),
		exec.P("key", values)}, Results: []exec.Type{res}, Reads: []string{Growth}, Writes: []string{Growth},
		Contract: `Returns the result kept in the context of the call for the rule at the position, the level
and the key that holds there: the seeds it read have the same identities, and the lookaheads it
observed the same outcomes, in the current context; it records those dependencies for the enclosing
call. Otherwise the null result.`,
		Requires: `c is in a context.`})
	add(&exec.Op{Name: "ctx.push", Component: Growth, Params: []exec.Param{ctxArg}, Writes: []string{Growth},
		Contract: `Starts recording the dependencies of the evaluation of the call, if it is in a context.`})
	add(&exec.Op{Name: "ctx.pop", Component: Growth, Params: []exec.Param{ctxArg}, Results: []exec.Type{exec.TInt},
		Reads: []string{Growth}, Writes: []string{Growth},
		Contract: `Ends recording the dependencies of the evaluation and returns where its result goes:
ctx.storeMemo for the memo (it read no seed and made no observation), ctx.storeCtx for the table of
the context, or another value for nowhere (its context's leaders have no present set).`})
	for _, a := range []struct{ name, doc string }{
		{"ctx.storeMemo", "Returns the value of ctx.pop for a result stored in the memo."},
		{"ctx.storeCtx", "Returns the value of ctx.pop for a result kept in the table of its context."},
	} {
		add(&exec.Op{Name: a.name, Component: Growth, Results: []exec.Type{exec.TInt}, Contract: a.doc})
	}
	add(&exec.Op{Name: "memo.setPresent", Component: Memo, Params: append(entryArg, ctxArg), Reads: []string{Growth},
		Writes:   []string{Memo},
		Contract: `Records with the entry e the present set of the context state c, for ctx.allows.`})
	add(&exec.Op{Name: "ctx.keep", Component: Growth, Params: []exec.Param{ctxArg, exec.P("r", rule), exec.P("pos", exec.TInt),
		exec.P("min", exec.TInt), exec.P("key", values), exec.P("ok", exec.TBool), exec.P("end", exec.TInt), exec.P("silent", exec.TBool),
		exec.P("value", value), exec.P("errs", errs), exec.P("far", exec.TInt), exec.P("k", kept)},
		Reads: []string{Growth, Call}, Writes: []string{Growth}, NoReturn: true,
		Contract: `Keeps the result of the call in the table of its context with the dependencies that ctx.pop
ended, and the examined frontier of a Document parse.`,
		Requires: `The last ctx.pop of c returned ctx.storeCtx.`})
	add(&exec.Op{Name: "ctx.done", Component: Growth, Params: []exec.Param{ctxArg}, Writes: []string{Growth},
		Contract: `Passes the dependencies that ctx.pop ended to the evaluation that encloses the call.`})

	add(&exec.Op{Name: "grow.begin", Component: Growth, Params: call, Results: []exec.Type{grow},
		Reads: []string{Growth, Rules}, Writes: []string{Growth}, NoReturn: true,
		Contract: `Starts the growth of the leader at the binding level min at the position from a failure,
charging one unit: the growth is in a context when other leaders of its cycle grow at the position.`})
	add(&exec.Op{Name: "grow.next", Component: Growth, Params: []exec.Param{exec.P("g", grow)}, Results: []exec.Type{exec.TBool},
		Reads: []string{Growth}, Writes: append([]string{Growth}, regs...), NoReturn: true,
		Contract: `Reports whether the body must be evaluated for the next step of the growth g. In a context
it first takes the steps that the memoized steps and jumps of the context give, and starts recording
what the step it reports depends on.`})
	add(&exec.Op{Name: "grow.step", Component: Growth, Params: []exec.Param{exec.P("g", grow), exec.P("value", value),
		exec.P("ok", exec.TBool)}, Reads: []string{Growth}, Writes: append([]string{Growth}, regs...), NoReturn: true,
		Contract: `Receives the result of a step of the growth g: adopts it as the seed, charging one unit, and
returns to the start of the growth if the match grew, and otherwise ends the growth.`})
	add(&exec.Op{Name: "grow.end", Component: Growth, Params: []exec.Param{exec.P("g", grow)}, Results: []exec.Type{res},
		Reads: []string{Growth}, Writes: append([]string{Growth}, regs...),
		Contract: `Ends the growth g and returns its last seed: the position moves to its end and its recovered
errors replace those recorded since the growth began.`})

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
the call's frontier and true in a Document parse, and false otherwise.`,
		Requires: `enclosing is the result of the matching doc.examinedBegin.`})
	add(&exec.Op{Name: "doc.enter", Component: Call, Params: []exec.Param{exec.P("pos", exec.TInt)},
		Results: []exec.Type{exec.TInt}, Reads: []string{Call}, Writes: []string{Call}, Features: []string{Document},
		Contract: `Begins a rule call at pos in a Document parse: returns the examined frontier of the enclosing
call, and starts the call's frontier past pos.`})
	add(&exec.Op{Name: "doc.leave", Component: Call, Params: []exec.Param{exec.P("high", exec.TInt)},
		Reads: []string{Call}, Writes: []string{Call}, Features: []string{Document},
		Contract: `Ends a rule call in a Document parse: the enclosing call's frontier becomes the farther of high
and the call's.`,
		Requires: `high is the result of the matching doc.enter.`})
	doc := []string{Document}
	inDoc := `The parse reads a Document.`
	add(&exec.Op{Name: "doc.active", Component: Call, Results: []exec.Type{exec.TBool}, Reads: []string{Call}, Features: doc,
		Contract: `Reports whether the parse reads a Document.`})
	add(&exec.Op{Name: "doc.touch", Component: Call, Params: []exec.Param{exec.P("end", exec.TInt)}, Reads: []string{Call},
		Writes: []string{Call}, Features: doc,
		Contract: `Extends the examined frontier of the current call to end, if end is past it.`})
	add(&exec.Op{Name: "doc.readEntry", Component: Call, Params: entryArg, Reads: []string{Call, Memo}, Writes: []string{Call},
		Features: doc,
		Contract: `Records that the current call read the entry e: extends its examined frontier to the one recorded
with e.`, Requires: `The parse reads a Document, and e has a record (memo.recordDoc).`})
	add(&exec.Op{Name: "doc.reused", Component: Call, Writes: []string{Call}, Features: doc,
		Contract: `Counts a reused memo entry in the statistics of the Document.`, Requires: inDoc})
	add(&exec.Op{Name: "doc.evaluated", Component: Call, Writes: []string{Call}, Features: doc,
		Contract: `Counts an evaluated rule body in the statistics of the Document.`, Requires: inDoc})
	add(&exec.Op{Name: "call.body", Component: Call, Params: call, Results: result, Writes: append([]string{Call}, regs...),
		NoReturn: true,
		Contract: `Calls the rule at the binding level min without observation: the call, or the plain invocation,
that the target's observed function wraps.`})
}
