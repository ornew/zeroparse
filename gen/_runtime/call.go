//go:build !zp_exec

package runtime

// The call protocol of a rule, left-recursion growth and the observed calls, as written by hand.
// Built with the tag zp_exec, the runtime uses the functions that internal/ir/exec/core prints
// from the shared compositions instead (call_exec.go).

// Call calls rule r at level min. An observed or Document parse takes
// callTracked, so that the call path of other parses has no observation or
// Document bookkeeping; callBody has the same logic with that bookkeeping.
func (p *Parser) Call(r *Rule, min int) (*Node, bool) {
	if p.Observation != nil || p.Document != nil {
		return p.callTracked(r, min)
	}
	p.Spend()
	start := p.Pos
	if (!r.Memo && !r.Leader) || p.FirstCall(r) {
		// Unmemoized call: no memo bookkeeping, and expectations need no isolation.
		if len(r.Scope) == 0 {
			return p.invokePlainBody(r, min)
		}
		rec := len(p.Recovered)
		v, ok := p.invoke(r, min)
		if !ok {
			p.Pos = start
			p.Recovered = p.Recovered[:rec]
		}
		return v, ok
	}
	return p.callMemo(r, min)
}

// grow grows the seed of a left-recursive leader from a failure (growBegin): it evaluates the body
// again while the match gets longer, and returns the last longer result.
func (p *Parser) grow(r *Rule, min int) (*Node, bool) {
	g := p.growBegin(r, min)
	for p.growNext(&g) {
		v, ok := p.invoke(r, min)
		p.growStep(&g, v, nil, ok)
	}
	best := p.growEnd(&g)
	return best.node, best.ok
}

// callBody is call without observation.
func (p *TParser) callBody(r *TRule, min int) (any, bool) {
	if p.TypedDocument != nil {
		return p.DocumentCall(r, min, nil)
	}
	p.Spend()
	start := p.Pos
	if !r.Memo && !r.Leader || p.FirstCall(r.Rule) {
		if len(r.Scope) == 0 {
			return p.invokePlainBody(r, min)
		}
		rec := len(p.Recovered)
		v, ok := p.invoke(r, min)
		if !ok {
			p.Pos = start
			p.Recovered = p.Recovered[:rec]
		}
		return v, ok
	}
	return p.callMemo(r, min, nil)
}

// CallMemo is the memoized part of callBody, for the rule r invoked by inv (the generated i<id>):
// the rule is called at the position again, or its memoization is no longer deferred.
func (p *TParser) CallMemo(r *TRule, inv func(*TParser) (any, bool)) (any, bool) {
	if p.TypedDocument != nil {
		return p.DocumentCall(r, 0, inv)
	}
	return p.callMemo(r, 0, inv)
}

// callMemo is a memoized call of the typed rule r at level min, whose body inv evaluates if it is
// not nil: it reuses the seed of a growth of r at the position, the result in the memo or the
// result kept in the context, or evaluates and records the result (see Parser.callMemo). In a
// typed Document parse, each entry records how far its call examined the input
// (TypedDocument.meta).
func (p *TParser) callMemo(r *TRule, min int, inv func(*TParser) (any, bool)) (any, bool) {
	start := p.Pos
	env := p.memoEnv(r.Rule)
	var c ctxCallState
	if r.Cycle != 0 && len(p.gamma) != 0 {
		if s := p.ctxEnter(&c, r.Rule, min); s != nil {
			p.replayCtx(s, true)
			if !s.ok {
				return nil, false
			}
			return s.val, true
		}
	}
	old := p.memoGet(r.Id, start, min, env)
	if old != nil && c.memoAllows(old.present) && p.reusable(old.silent, len(old.errs)) {
		p.Observation.memoHit()
		if p.TypedDocument != nil {
			p.documentTouch(p.TypedDocument.meta[old].examined)
			p.Document.stats.Reused++
		}
		p.MergeExpected(old.far, old.expected)
		if !old.ok {
			return nil, false
		}
		p.Pos = old.end
		p.Recovered = p.copyErrors(p.Recovered, old.errs)
		return old.val, true
	}
	if c.ctx {
		if s := p.ctxLookup(&c, r.Id, min, env); s != nil {
			p.replayCtx(s, false)
			if !s.ok {
				return nil, false
			}
			return s.val, true
		}
	}
	rec := len(p.Recovered)
	p.ctxPush(&c)
	// The memoized result includes the expectations recorded in this call.
	m := p.Isolate(start)
	var v any
	var ok bool
	if r.Leader {
		v, ok = p.grow(r, min)
	} else {
		if inv != nil {
			if p.TypedDocument != nil {
				p.Document.stats.Evaluated++
			}
			v, ok = inv(p)
		} else {
			v, ok = p.invoke(r, min)
		}
		if !ok {
			p.Recovered = p.Recovered[:rec]
		}
	}
	far, inner := p.Unisolate(m)
	kept := p.Keep(inner)
	p.documentTouch(p.Pos + 1)
	switch p.ctxPop(&c) {
	case storeMemo:
		if old != nil && p.TypedDocument != nil {
			// The entry is replaced; its metadata must not keep it alive.
			delete(p.TypedDocument.meta, old)
		}
		e := p.memoSlab.one()
		*e = tmemoEntry{val: v, ok: ok, end: p.Pos, silent: p.Silent > 0, present: c.present,
			errs: p.copyErrors(nil, p.Recovered[rec:]), far: far, expected: kept}
		p.memoPut(r.Id, start, min, env, e)
		if p.TypedDocument != nil {
			p.TypedDocument.meta[e] = typedDocumentMeta{examined: p.docHigh}
		}
	case storeCtx:
		p.ctxKeep(&c, r.Id, start, min, env, &ctxResult{val: v, ok: ok, end: p.Pos, examined: p.examinedNow(),
			silent: p.Silent > 0, errs: p.copyErrors(nil, p.Recovered[rec:]), far: far, expected: kept})
	}
	p.ctxDone(&c)
	p.MergeExpected(far, inner)
	if !ok {
		p.Pos = start
	}
	return v, ok
}

// grow is Parser.grow for a typed rule.
func (p *TParser) grow(r *TRule, min int) (any, bool) {
	g := p.growBegin(r.Rule, min)
	for p.growNext(&g) {
		v, ok := p.invoke(r, min)
		p.growStep(&g, nil, v, ok)
	}
	best := p.growEnd(&g)
	return best.val, best.ok
}

// callObserved is call in an observed parse.
func (p *Parser) callObserved(r *Rule, min int) (*Node, bool) {
	p.Observation.Enter(r.Name, p.Pos, min)
	h := p.ExaminedBegin()
	v, ok := p.callBody(r, min)
	x, known := p.ExaminedEnd(h)
	p.Observation.Exit(ok, p.Pos, x, known)
	return v, ok
}

// invokePlainObserved is InvokePlain in an observed parse.
func (p *Parser) invokePlainObserved(r *Rule, min int) (*Node, bool) {
	p.Observation.Enter(r.Name, p.Pos, min)
	h := p.ExaminedBegin()
	v, ok := p.invokePlainBody(r, min)
	x, known := p.ExaminedEnd(h)
	p.Observation.Exit(ok, p.Pos, x, known)
	return v, ok
}

// callObserved is call in an observed parse.
func (p *TParser) callObserved(r *TRule, min int) (any, bool) {
	p.Observation.Enter(r.Name, p.Pos, min)
	h := p.ExaminedBegin()
	v, ok := p.callBody(r, min)
	x, known := p.ExaminedEnd(h)
	p.Observation.Exit(ok, p.Pos, x, known)
	return v, ok
}

// invokePlainObserved is InvokePlain in an observed parse.
func (p *TParser) invokePlainObserved(r *TRule, min int) (any, bool) {
	p.Observation.Enter(r.Name, p.Pos, min)
	h := p.ExaminedBegin()
	v, ok := p.invokePlainBody(r, min)
	x, known := p.ExaminedEnd(h)
	p.Observation.Exit(ok, p.Pos, x, known)
	return v, ok
}

// callDocument is callBody in a Document parse, which memoizes the rules that
// the Document reuses and records how far each call examined the input. The
// enclosing call's frontier is restored however the call ends, an
// interruption included.
func (p *Parser) callDocument(r *Rule, min int) (*Node, bool) {
	oldHigh := p.docHigh
	p.docHigh = p.Pos + 1
	defer func() { p.docHigh = max(oldHigh, p.docHigh) }()
	if !r.DocMemo {
		return p.callUnmemoized(r, min)
	}
	return p.callMemo(r, min)
}

// callUnmemoized is a call without memo bookkeeping, whose expectations need no isolation.
func (p *Parser) callUnmemoized(r *Rule, min int) (*Node, bool) {
	if len(r.Scope) == 0 {
		return p.invokePlainBody(r, min)
	}
	start, rec := p.Pos, len(p.Recovered)
	v, ok := p.invoke(r, min)
	if !ok {
		p.Pos = start
		p.Recovered = p.Recovered[:rec]
	}
	return v, ok
}

// callMemo is a memoized call: it reuses the seed of a growth of r at the position, the result in
// the memo or the result kept in the context of the call, or evaluates the call (growing the seed
// of a leader) and records its result, in the memo or in the context (context_memo.go). In a
// Document parse, each entry records how far its call examined the input.
func (p *Parser) callMemo(r *Rule, min int) (*Node, bool) {
	start := p.Pos
	env := p.memoEnv(r)
	var c ctxCallState
	if r.Cycle != 0 && len(p.gamma) != 0 {
		if s := p.ctxEnter(&c, r, min); s != nil {
			p.replayCtx(s, true)
			if !s.ok {
				return nil, false
			}
			return s.node, true
		}
	}
	// See the engine's callBegin: entries with recovered errors are reused only in the same kind of
	// context (inside or outside a lookahead) they were computed in.
	if e := p.memoGet(r.Id, start, min, env); e != nil && c.memoAllows(e.present) && p.reusable(e.silent, len(e.errs)) {
		p.Observation.memoHit()
		if p.Document != nil {
			p.documentTouch(e.examined)
			p.Document.stats.Reused++
		}
		p.MergeExpected(e.far, e.expected)
		if !e.ok {
			return nil, false
		}
		p.Pos = e.end
		p.Recovered = p.copyErrors(p.Recovered, e.errs)
		return e.node, true
	}
	if c.ctx {
		if s := p.ctxLookup(&c, r.Id, min, env); s != nil {
			p.replayCtx(s, false)
			if !s.ok {
				return nil, false
			}
			return s.node, true
		}
	}
	rec := len(p.Recovered)
	p.ctxPush(&c)
	// The memoized result includes the expectations recorded in this call.
	m := p.Isolate(start)
	var v *Node
	var ok bool
	if r.Leader {
		v, ok = p.grow(r, min)
	} else {
		v, ok = p.invoke(r, min)
		if !ok {
			p.Recovered = p.Recovered[:rec]
		}
	}
	far, inner := p.Unisolate(m)
	kept := p.Keep(inner)
	p.documentTouch(p.Pos + 1)
	switch p.ctxPop(&c) {
	case storeMemo:
		e := p.memo.alloc()
		*e = memoEntry{node: v, ok: ok, end: p.Pos, examined: p.docHigh, silent: p.Silent > 0, present: c.present,
			errs: p.copyErrors(nil, p.Recovered[rec:]), far: far, expected: kept}
		p.memoPut(r.Id, start, min, env, e)
	case storeCtx:
		p.ctxKeep(&c, r.Id, start, min, env, &ctxResult{node: v, ok: ok, end: p.Pos, examined: p.examinedNow(),
			silent: p.Silent > 0, errs: p.copyErrors(nil, p.Recovered[rec:]), far: far, expected: kept})
	}
	p.ctxDone(&c)
	p.MergeExpected(far, inner)
	if !ok {
		p.Pos = start
	}
	return v, ok
}

// callTracked is call in an observed or Document parse.
func (p *Parser) callTracked(r *Rule, min int) (*Node, bool) {
	if p.Observation != nil {
		return p.callObserved(r, min)
	}
	return p.callBody(r, min)
}

// callBody is call with the Document bookkeeping and without observation. A
// Document parse takes callDocument.
func (p *Parser) callBody(r *Rule, min int) (*Node, bool) {
	p.Spend()
	if p.Document != nil {
		return p.callDocument(r, min)
	}
	if (!r.Memo && !r.Leader) || p.FirstCall(r) {
		return p.callUnmemoized(r, min)
	}
	return p.callMemo(r, min)
}

// DocumentCall memoizes typed rule entries (rule.DocMemo), including generated
// direct wrappers. The examined frontier of an entry is kept outside batch memo
// entries (TypedDocument.meta).
func (p *TParser) DocumentCall(r *TRule, min int, inv func(*TParser) (any, bool)) (any, bool) {
	p.Spend()
	start := p.Pos
	oldHigh := p.docHigh
	p.docHigh = start + 1
	defer func() { p.docHigh = max(oldHigh, p.docHigh) }()
	if !r.DocMemo {
		rec := len(p.Recovered)
		var v any
		var ok bool
		if inv != nil {
			p.Document.stats.Evaluated++
			v, ok = inv(p)
		} else {
			v, ok = p.invoke(r, min)
		}
		if !ok {
			p.Pos = start
			p.Recovered = p.Recovered[:rec]
		}
		return v, ok
	}
	return p.callMemo(r, min, inv)
}
