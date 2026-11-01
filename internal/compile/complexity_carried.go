package compile

import "zeroparse.dev/zp/grammar"

// --- Growth that carries a growing result ---

// checkCarriedGrowth checks condition 5. A leader R of a left-recursion cycle called where another
// leader S of the cycle grows begins a growth of its own there, from a result that includes the
// growing result of S when a left call of R's body reaches S. That growth is a scanning source in
// the step of S's growth that calls R: no two steps share it, since each reads another growing
// result of S. It repeats input that later steps cover when both of these hold.
//
//   - The step can drop R's result and still grow: the call is in a lookahead, an option, a
//     repetition or the base of a #recover, or in an alternative followed by an alternative that
//     begins with a left call, or in the body of a rule of the cycle that is not a leader (the
//     classifier does not follow what the callers of such a rule do after it fails).
//   - The growth of R can go on past the end of the growing result of S over input with which the
//     growth of the cycle goes on too: what follows the left calls of S in R's body, followed by
//     what follows R, and what follows the seed reads of the cycle's leaders outside R's body,
//     can begin with the same character.
//
// When the first characters differ, R grows past the result of S only where the growth of the
// cycle stops, so at most once for each growth.
func (k *classifier) checkCarriedGrowth() {
	seen := map[string]bool{}
	for _, s := range k.sites {
		r := s.callee
		if !k.reach[s.caller] || !k.an.leaders[r] || !k.an.cyclic[s.caller] || !k.sameCycle(s.caller, r) ||
			!s.mayStart || s.caller == r || seen[s.caller+"\x00"+r] {
			continue
		}
		if !k.dropsResult(s) {
			continue
		}
		cycle := k.an.cycle[r]
		for _, lead := range k.rules {
			sname := lead.Name
			if sname == r || !k.an.leaders[sname] || k.an.cycle[sname] != cycle {
				continue
			}
			carry, ok := k.carryPaths(r, sname, cycle)
			if !ok || len(carry) == 0 {
				continue
			}
			if !k.firstPaths(carry, k.followFor(cycle, false)).intersects(k.growthGoesOn(cycle, r)) {
				continue
			}
			seen[s.caller+"\x00"+r] = true
			k.cause(ClassPoly, 2, s.caller, exprPos(s.ref),
				"%s: the call %s at %s grows %s where %s grows, from the growing result of %s, so each step of the growth of %s grows %s again over the same input",
				degreeText(ClassPoly, 2), r, at(s.caller, exprPos(s.ref)), r, sname, sname, sname, r)
			break
		}
	}
}

// dropsResult reports whether the step that makes the call s can drop the result of the call and
// still grow (checkCarriedGrowth).
func (k *classifier) dropsResult(s *site) bool {
	if !k.an.leaders[s.caller] {
		return true
	}
	for _, u := range s.units {
		switch u.kind {
		case unitLookahead, unitOption, unitIteration, unitRecover:
			return true
		case unitAlt:
			for _, later := range u.choice.Alts[u.index+1:] {
				if f := k.firstEvaluated(later); f != nil && k.sameCycle(s.caller, f.Name) && f.Level == "" {
					return true
				}
			}
		}
	}
	return false
}

// carryPaths follows the body of the leader r through the left calls of rules of the cycle that
// are not leaders to the left calls of the leader s, and returns what follows those calls. A left
// call of another leader ends a path, and so does a lookahead, whose result is not part of r's.
func (k *classifier) carryPaths(r, s string, cycle int) (paths []*cont, ok bool) {
	ok = true
	budget := leadBudget
	var visit func(p *cont, depth int)
	visit = func(p *cont, depth int) {
		for {
			k.control.poll()
			budget--
			if budget < 0 {
				ok = false
				return
			}
			if p == nil || len(p.items) == 0 {
				return
			}
			rest := p.rest()
			switch e := p.items[0].(type) {
			case *grammar.Group:
				p = rest.then(e.Expr)
			case *grammar.Capture:
				p = rest.then(e.Expr)
			case *grammar.Discard:
				p = rest.then(e.Expr)
			case *grammar.Atomic:
				p = rest.then(e.Expr)
			case *grammar.Stream:
				p = rest.then(e.Expr)
			case *grammar.InlineAction:
				p = rest.then(e.Expr)
			case *grammar.Optional:
				visit(rest.then(e.Expr), depth)
				p = rest
			case *grammar.Seq:
				p = rest.then(e.Items...)
			case *grammar.Choice:
				for _, alt := range e.Alts {
					visit(rest.then(alt), depth)
				}
				return
			case *grammar.Ref:
				if !k.an.cyclic[e.Name] || k.an.cycle[e.Name] != cycle || e.Level != "" {
					return
				}
				if e.Name == s {
					paths = append(paths, rest)
					return
				}
				if k.an.leaders[e.Name] || depth > len(k.rules) {
					return
				}
				p = rest.then(k.byName[e.Name].Expr)
				depth++
			default:
				if k.consumesNothing(e) {
					p = rest
					continue
				}
				return
			}
		}
	}
	visit(&cont{items: []grammar.Expr{k.byName[r].Expr}, end: endRule, rule: r}, 0)
	return paths, ok
}

// growthGoesOn returns the first characters of what follows the seed reads of the leaders of the
// cycle outside the body of r: the input with which a later step of a growth of the cycle goes on.
func (k *classifier) growthGoesOn(cycle int, r string) charset {
	var out charset
	for _, s := range k.sites {
		if s.caller == r || !k.an.cyclic[s.caller] || k.an.cycle[s.caller] != cycle || !k.an.leaders[s.callee] ||
			k.an.cycle[s.callee] != cycle || !s.mustStart {
			continue
		}
		out = out.union(k.firstCont(s.cont, k.followFor(cycle, true), nil))
	}
	return out
}
