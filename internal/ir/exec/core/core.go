// Package core holds the compositions of the parser VM (design 065): the algorithms that the
// hand-written runtimes repeat, written once in Exec IR over the operations of
// internal/ir/exec/ops, and their specializations for each runtime.
//
// The compositions are the call protocol of a rule (deferred memoization, memo lookup with its
// reuse conditions, store and restore on failure), left-recursion growth, the observation of a
// call and the bookkeeping of a call in a Document parse. The Go backend prints
// them into the generated Go runtime (GoRuntime) and the TypeScript backend into the TypeScript
// runtime (TSMethods).
package core

import (
	. "github.com/ornew/zeroparse/internal/ir/exec"
	"github.com/ornew/zeroparse/internal/ir/exec/ops"
)

// Build returns the module of the operations and the compositions, verified.
func Build() *Module {
	m := ops.Pilot()
	ops.Transitional(m)
	rule, value, entry := THandle(ops.Rule), THandle(ops.Value), THandle(ops.Entry)
	result := []Type{value, TBool}
	r := L("r")
	silent := Bin(">", R(ops.Silent), Int(0))
	id := C("rule.id", r)
	locals := func(b *Builder) {
		b.Var("v", value)
		b.Var("ok", TBool)
		b.Var("entry", entry)
	}
	pair := []Place{{Name: "v"}, {Name: "ok"}}
	ret := func(b *Builder) { b.Return(L("v"), L("ok")) }

	// tracked leaves the call protocol for the target's tracked path.
	tracked := func(b *Builder, track *Call) {
		b.If(And(F(ops.Document), C("call.tracked")), func(b *Builder) {
			b.SetN(pair, track)
			ret(b)
		})
	}
	// reuse returns the result of the entry stored for the call at start, if it may be reused: a
	// growing entry always (the left-recursion guard); otherwise one computed in a silent context
	// only in one, and one with recovered errors only in the kind of context it was computed in. A
	// member of a cycle is evaluated again at the position of the innermost growth. Otherwise it
	// starts tracking the reads of the call.
	//
	// With doc, a call in a Document parse also extends its examined frontier to the entry's and
	// limits its reuse to the entry's, and to until the next edit for a growing entry.
	reuse := func(b *Builder, level Expr, doc bool) {
		b.Let("env", C("env.memoKey", r))
		b.LetN([]string{"e", "found"}, C("memo.lookup", id, L("start"), level, L("env")))
		e := L("e")
		reusable := And(And(L("found"), Not(C("lr.stale", r, L("start")))), Bin("||", C("memo.growing", e),
			And(Bin("||", Not(C("memo.silent", e)), silent),
				Bin("||", C("rec.none", C("memo.errs", e)), Eq(C("memo.silent", e), silent)))))
		b.If(reusable, func(b *Builder) {
			b.If(F(ops.Observe), func(b *Builder) { b.Do("obs.memoHit") })
			b.Do("lr.read", e)
			if doc {
				b.If(C("doc.active"), func(b *Builder) {
					b.Do("doc.touch", C("memo.examined", e))
					b.Do("doc.limitReuse", C("memo.reuse", e))
					b.If(C("memo.growing", e), func(b *Builder) { b.Do("doc.limitReuse", C("reuse.untilEdit")) })
					b.Do("doc.reused")
				})
			}
			b.If(Not(C("memo.growing", e)), func(b *Builder) {
				b.Do("exp.mergeKept", C("memo.far", e), C("memo.expected", e))
			})
			b.If(Not(C("memo.ok", e)), func(b *Builder) { b.Return(Null(value), BoolC(false)) })
			b.SetReg(ops.Pos, C("memo.end", e))
			b.Do("rec.add", C("memo.errs", e))
			b.Return(C("memo.value", e), BoolC(true))
		})
		b.Let("rec", C("rec.mark"))
		b.Let("depth", C("lr.depth"))
		b.Let("read", C("lr.enter"))
		// The memoized result includes the expectations recorded in this call.
		b.Let("m", C("exp.isolate", L("start")))
	}
	// store records the result of an evaluated body, unless it read the seed of a growth that began
	// before the call or an unstable result, so that it can change.
	store := func(b *Builder, level Expr) {
		b.If(Not(L("ok")), func(b *Builder) { b.Do("rec.truncate", L("rec")) })
		b.IfElse(C("lr.changeable", L("depth")), func(b *Builder) {
			b.Set("entry", C("memo.detached"))
		}, func(b *Builder) {
			b.SetN([]Place{{Name: "entry"}}, C("memo.store", id, L("start"), level, L("env"), L("ok"), R(ops.Pos), silent,
				L("v"), C("rec.since", L("rec"))))
		})
	}
	// finish ends tracking the reads of the call, keeps the expectations of the call with its entry
	// and returns the result.
	//
	// With doc, a call in a Document parse also records with its entry how far it examined the
	// input and how long its result may be reused: a leader's result is not reused after an edit,
	// and only in this parse when its growth read a result that can still change (it is not
	// settled), because a later parse evaluates it without that growth. The parse forgets the
	// entries reused only in it when it ends.
	finish := func(b *Builder, doc bool) {
		if doc {
			b.Let("settled", Not(C("lr.changeable", L("depth"))))
		}
		b.Do("lr.leave", L("read"))
		b.LetN([]string{"far", "inner"}, C("exp.unisolate", L("m")))
		b.Do("memo.setExpected", L("entry"), L("far"), C("exp.keep", L("inner")))
		if doc {
			b.If(C("doc.active"), func(b *Builder) {
				b.Do("doc.touch", Add(R(ops.Pos), Int(1)))
				b.Do("memo.setExamined", L("entry"), C("doc.high"))
				b.If(C("rule.leader", r), func(b *Builder) {
					b.IfElse(Not(L("settled")), func(b *Builder) {
						b.Do("doc.setReuse", C("reuse.inParse"))
					}, func(b *Builder) { b.Do("doc.limitReuse", C("reuse.untilEdit")) })
				})
				b.Do("memo.setReuse", L("entry"), C("doc.reuse"))
				b.If(And(Eq(C("memo.reuse", L("entry")), C("reuse.inParse")), Bin("||", C("rule.leader", r), L("settled"))), func(b *Builder) {
					b.Do("doc.parseOnly", L("start"), L("entry"))
				})
			})
		}
		b.Do("exp.merge", L("far"), L("inner"))
		b.If(Not(L("ok")), func(b *Builder) { b.SetReg(ops.Pos, L("start")) })
		ret(b)
	}

	// unmemoized is a call without memo bookkeeping, whose expectations need no isolation, from start.
	unmemoized := func(b *Builder) {
		b.If(C("rule.plain", r), func(b *Builder) {
			b.SetN(pair, C("rule.invokePlain", r, L("min")))
			ret(b)
		})
		b.Let("rec", C("rec.mark"))
		b.SetN(pair, C("rule.invoke", r, L("min")))
		b.If(Not(L("ok")), func(b *Builder) {
			b.SetReg(ops.Pos, L("start"))
			b.Do("rec.truncate", L("rec"))
		})
		ret(b)
	}

	// evaluate evaluates the call through the memo after reuse found no entry to reuse: it grows a
	// leader's seed, and otherwise invokes the body and stores its result.
	evaluate := func(b *Builder) {
		b.IfElse(C("rule.leader", r), func(b *Builder) {
			b.SetN(pair, C("grow", r, L("min"), L("env")))
			b.Set("entry", C("memo.grown", id, L("start"), L("min"), L("env")))
			b.If(F(ops.Stream), func(b *Builder) {
				// A stream commit pruned the slot of a growing start rule; its result needs no
				// entry.
				b.If(Eq(L("entry"), Null(entry)), func(b *Builder) { b.Set("entry", C("memo.detached")) })
			})
		}, func(b *Builder) {
			b.SetN(pair, C("rule.invoke", r, L("min")))
			store(b, L("min"))
		})
	}

	call := m.AddFunc("call", []Param{P("r", rule), P("min", TInt)}, result,
		[]string{ops.Document, ops.Observe, ops.Stream, ops.Typed}, func(b *Builder) {
			locals(b)
			tracked(b, C("call.track", r, L("min")))
			b.Do("call.spend")
			b.Let("start", R(ops.Pos))
			b.If(Bin("||", And(Not(C("rule.memo", r)), Not(C("rule.leader", r))), C("call.first", r)), unmemoized)
			reuse(b, L("min"), false)
			evaluate(b)
			finish(b, false)
		})
	call.Doc = `call calls rule r at binding level min: unmemoized while its memoization is deferred, and
otherwise through the memo, growing the seed of a left-recursion leader.`

	callMemo := m.AddFunc("callMemo", []Param{P("r", rule), P("inv", THandle(ops.Body))}, result,
		[]string{ops.Document, ops.Observe, ops.Typed}, func(b *Builder) {
			locals(b)
			tracked(b, C("call.trackBody", r, L("inv")))
			b.Let("start", R(ops.Pos))
			reuse(b, Int(0), false)
			b.SetN(pair, C("rule.run", L("inv")))
			store(b, Int(0))
			finish(b, false)
		})
	callMemo.Doc = `callMemo is the memoized part of a rule call, for the rule r at binding level 0 invoked by inv:
the rule is called at the position again, or its memoization is no longer deferred.`

	grow := m.AddFunc("grow", []Param{P("r", rule), P("min", TInt), P("env", THandle(ops.Values))}, result,
		[]string{ops.Typed}, func(b *Builder) {
			b.Let("start", R(ops.Pos))
			b.Let("rec", C("rec.mark"))
			b.Let("best", C("memo.seed", id, L("start"), L("min"), L("env")))
			b.Let("read", C("lr.enter"))
			b.Let("prev", C("lr.begin", L("start")))
			b.Do("memo.mark", L("best"))
			b.Loop("l", func(b *Builder) {
				b.Do("call.growStep")
				b.LetN([]string{"v", "ok"}, C("rule.invoke", r, L("min")))
				best := L("best")
				b.If(Bin("||", Not(L("ok")), And(C("memo.ok", best), Bin("<=", R(ops.Pos), C("memo.end", best)))), func(b *Builder) {
					b.Break("l")
				})
				b.Set("best", C("memo.reseed", id, L("start"), L("min"), L("env"), R(ops.Pos), L("v"), C("rec.since", L("rec"))))
				b.Do("memo.mark", L("best"))
				b.SetReg(ops.Pos, L("start"))
				b.Do("rec.truncate", L("rec"))
			})
			best := L("best")
			b.Do("memo.finishSeed", best, silent)
			// The growth has read its seed for the last time: those reads are resolved.
			b.Do("lr.end", L("prev"))
			b.Do("memo.settle", best)
			b.Do("lr.leave", L("read"))
			// The call that made the growth reads its final result as on a memo hit.
			b.Do("lr.read", best)
			b.SetReg(ops.Pos, C("memo.end", best))
			b.Do("rec.truncate", L("rec"))
			b.Do("rec.add", C("memo.errs", best))
			b.Return(C("memo.value", best), C("memo.ok", best))
		})
	grow.Doc = `grow grows the seed of a left-recursive leader: it memoizes a failure, then evaluates the
body again while the match gets longer, and keeps the last longer result.`

	observed := m.AddFunc("observed", []Param{P("r", rule), P("min", TInt)}, result,
		[]string{ops.Document, ops.Observe}, func(b *Builder) {
			b.Do("obs.enter", r, R(ops.Pos), L("min"))
			b.Let("h", C("doc.examinedBegin"))
			b.LetN([]string{"v", "ok"}, C("call.body", r, L("min")))
			b.LetN([]string{"x", "known"}, C("doc.examinedEnd", L("h")))
			b.Do("obs.exit", L("ok"), R(ops.Pos), L("x"), L("known"))
			b.Return(L("v"), L("ok"))
		})
	observed.Doc = `observed is a rule call in an observed parse: it reports the call to the observation, with how
far it examined the input in a Document parse, around the call without observation.`
	callUnmemoized := m.AddFunc("unmemoized", []Param{P("r", rule), P("min", TInt)}, result, nil, func(b *Builder) {
		b.Var("v", value)
		b.Var("ok", TBool)
		b.Let("start", R(ops.Pos))
		unmemoized(b)
	})
	callUnmemoized.Doc = `unmemoized calls rule r at binding level min without memo bookkeeping, as call does while the
rule's memoization is deferred: its expectations need no isolation, and a failure restores the
position and the recovered errors.`

	memoized := m.AddFunc("memoized", []Param{P("r", rule), P("min", TInt)}, result,
		[]string{ops.Document, ops.Observe, ops.Stream}, func(b *Builder) {
			locals(b)
			b.Let("start", R(ops.Pos))
			reuse(b, L("min"), true)
			evaluate(b)
			finish(b, true)
		})
	memoized.Doc = `memoized calls rule r at binding level min through the memo, as call does once the rule's
memoization is no longer deferred, with the observation of a reuse and, in a Document parse, the
bookkeeping of how far each call examined the input and how long its result may be reused.`

	document := m.AddFunc("document", []Param{P("r", rule), P("min", TInt)}, result,
		[]string{ops.Document}, func(b *Builder) {
			b.LetN([]string{"oldHigh", "oldReuse"}, C("doc.enter", R(ops.Pos)))
			b.Protect(func(b *Builder) {
				b.If(Not(C("rule.docMemo", r)), func(b *Builder) {
					b.LetN([]string{"v", "ok"}, C("unmemoized", r, L("min")))
					b.Return(L("v"), L("ok"))
				})
				b.LetN([]string{"v", "ok"}, C("memoized", r, L("min")))
				b.Return(L("v"), L("ok"))
			}, func(b *Builder) { b.Do("doc.leave", L("oldHigh"), L("oldReuse")) })
		})
	document.Doc = `document is a rule call in a Document parse, which memoizes the rules that the Document reuses
and records how far each call examined the input and how long its result may be reused. The
enclosing call's frontier and reuse are restored however the call ends, an interruption included,
so that a parse after it starts from the state it would start from otherwise.`
	// ret2 returns the result of the call c of a function.
	ret2 := func(b *Builder, c *Call) {
		b.LetN([]string{"v", "ok"}, c)
		b.Return(L("v"), L("ok"))
	}
	body := m.AddFunc("body", []Param{P("r", rule), P("min", TInt)}, result, []string{ops.Document}, func(b *Builder) {
		b.Do("call.spend")
		b.If(And(F(ops.Document), C("doc.active")), func(b *Builder) { ret2(b, C("document", r, L("min"))) })
		b.If(Bin("||", And(Not(C("rule.memo", r)), Not(C("rule.leader", r))), C("call.first", r)), func(b *Builder) {
			ret2(b, C("unmemoized", r, L("min")))
		})
		ret2(b, C("memoized", r, L("min")))
	})
	body.Doc = `body is a rule call on the tracked path without observation: the Document call in a Document
parse, and otherwise the unmemoized call while the rule's memoization is deferred and the memoized
call after.`
	trackedCall := m.AddFunc("tracked", []Param{P("r", rule), P("min", TInt)}, result, []string{ops.Document, ops.Observe}, func(b *Builder) {
		b.If(And(F(ops.Observe), C("obs.active")), func(b *Builder) { ret2(b, C("observed", r, L("min"))) })
		ret2(b, C("body", r, L("min")))
	})
	trackedCall.Doc = `tracked is a rule call in an observed or Document parse: the observed call in an observed parse,
and otherwise the call without observation.`
	if err := Verify(m); err != nil {
		panic(err)
	}
	return m
}
