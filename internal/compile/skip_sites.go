package compile

import "github.com/ornew/zeroparse/grammar"

// Skip-run sites.
//
// A #recover skip expression runs at every position where its expression fails, and a repetition
// in it walks forward from there. When many positions fail one after another, as when recovery
// keeps failing in a loop, each walk covers the input that the walks before it covered, and the
// parse takes time quadratic in the length of the skipped input. The walk of an unbounded
// repetition from a position ends where the walk from any later position it passes ends, so a
// parser records, for each position the walk visited, what the rest of the walk did (its end, the
// expectations at its farthest failure and the input range it examined), and a later walk that
// reaches a recorded position takes the rest from the record (see "Skip Runs" in
// spec/runtime/parsing.md).
//
// The record holds only if the rest of the walk depends on nothing but the position and the
// input. A repetition is a skip-run site when it is written in a skip expression, not inside
// another repetition of that expression that is a site, and
//   - it has no maximum;
//   - its element has no predicate that reads or binds a variable (a skip expression has no
//     captures: the compiler rejects them);
//   - its element has no #cut that applies to the repetition (one outside every choice, option,
//     repetition and lookahead of the element);
//   - neither its element nor a rule the element calls, directly or indirectly, reads a variable,
//     contains #recover, or is left-recursive (a leader or member of a left-recursion cycle).
//
// Bindings made inside a called rule are undone when the rule returns, so only the element's own
// predicates count. A parse with missing-token insertion records nothing (the runtime checks it).

// markSkipSites numbers the skip-run sites of the program from 1, in the order of the rules and of
// the expressions in them.
func (prog *Program) markSkipSites(control *Control) {
	defs := make([]*grammar.RuleDef, 0, len(prog.RuleList))
	calls := map[string][]string{}
	recovers, recursive := map[string]bool{}, map[string]bool{}
	for _, r := range prog.RuleList {
		control.poll()
		defs = append(defs, r.Def)
		calls[r.Name] = allCallsControlled(r.Def.Expr, nil, control)
		if r.Leader || r.Member {
			recursive[r.Name] = true
		}
		walkExprControlled(r.Def.Expr, func(x grammar.Expr) {
			if isRecover(x) {
				recovers[r.Name] = true
			}
		}, control)
	}
	propagate(defs, calls, recovers, control)
	propagate(defs, calls, recursive, control)
	eligible := func(e *grammar.Repeat) bool {
		if e.Max >= 0 || escapingCut(e.Expr) {
			return false
		}
		ok := true
		walkExprControlled(e.Expr, func(x grammar.Expr) {
			switch x := x.(type) {
			case *grammar.Predicate:
				if readsOrBindsVariable(x.Term) {
					ok = false
				}
			case *grammar.Ref:
				r := prog.ByName[x.Name]
				if r == nil || len(r.Vars) > 0 || recovers[x.Name] || recursive[x.Name] {
					ok = false
				}
			default:
				if isRecover(x) {
					ok = false
				}
			}
		}, control)
		return ok
	}
	// mark marks the outermost eligible repetitions of the skip expression e.
	var mark func(e grammar.Expr)
	mark = func(e grammar.Expr) {
		walkExprControlled(e, func(x grammar.Expr) {
			if rep, ok := x.(*grammar.Repeat); ok && prog.skipSites[rep] == 0 && !prog.inSite[rep] && eligible(rep) {
				prog.NumSkipSites++
				prog.skipSites[rep] = prog.NumSkipSites
				// Nothing inside a site is one.
				walkExprControlled(rep.Expr, func(y grammar.Expr) {
					if r, ok := y.(*grammar.Repeat); ok {
						prog.inSite[r] = true
					}
				}, control)
			}
		}, control)
	}
	prog.skipSites, prog.inSite = map[*grammar.Repeat]int{}, map[*grammar.Repeat]bool{}
	for _, r := range prog.RuleList {
		control.poll()
		walkExprControlled(r.Def.Expr, func(x grammar.Expr) {
			if skip := skipOf(x); skip != nil {
				mark(skip)
			}
		}, control)
	}
	prog.inSite = nil
}

// SkipSite returns the number of the skip-run site e (from 1), or 0 if e is not one.
func (prog *Program) SkipSite(e *grammar.Repeat) int { return prog.skipSites[e] }

// isRecover reports whether e is an expression with #recover.
func isRecover(e grammar.Expr) bool { return skipOf(e) != nil }

// skipOf returns the skip expression of e if e is an expression with #recover, or nil.
func skipOf(e grammar.Expr) grammar.Expr {
	switch e := e.(type) {
	case *grammar.Recovery:
		return e.Skip
	case *grammar.Attributed:
		return recoverSkip(e)
	}
	return nil
}

// escapingCut reports whether e has a #cut outside every choice, option, repetition and lookahead
// in it: one that applies to the choice, option or repetition around e.
func escapingCut(e grammar.Expr) bool {
	switch e := e.(type) {
	case *grammar.Cut:
		return true
	case *grammar.Choice, *grammar.Optional, *grammar.Repeat, *grammar.And, *grammar.Not, *grammar.Ref:
		return false
	case *grammar.Seq:
		for _, it := range e.Items {
			if escapingCut(it) {
				return true
			}
		}
		return false
	case *grammar.Group:
		return escapingCut(e.Expr)
	case *grammar.InlineAction:
		return escapingCut(e.Expr)
	case *grammar.Atomic:
		return escapingCut(e.Expr)
	case *grammar.Stream:
		return escapingCut(e.Expr)
	case *grammar.Recovery:
		return escapingCut(e.Expr)
	case *grammar.Discard:
		return escapingCut(e.Expr)
	case *grammar.Capture:
		return escapingCut(e.Expr)
	case *grammar.Attributed:
		return escapingCut(e.Expr)
	}
	return false
}

// readsOrBindsVariable reports whether the predicate term t reads or binds a variable.
func readsOrBindsVariable(t grammar.Term) bool {
	found := false
	WalkTerm(t, func(x grammar.Term) {
		switch x.(type) {
		case *grammar.VarRef, *grammar.Bind:
			found = true
		}
	})
	return found
}
