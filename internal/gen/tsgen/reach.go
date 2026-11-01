package tsgen

import (
	"fmt"
	"slices"

	"zeroparse.dev/zp/grammar"
	"zeroparse.dev/zp/internal/compile"
	"zeroparse.dev/zp/internal/ir/core"
	irbuild "zeroparse.dev/zp/internal/ir/core/build"
)

// A generated parser prints only the rules that a parse can enter: the start rules, which parseRule
// names and parse and recognize run, and the rules that the printed bodies call (core.Module.Reach).
// The Core IR also has calls that the printed code leaves out, after an expression that never matches;
// GenerateParser then generates again with the rules that the printed calls reach (printedReach).
// A rule left out has no constant, body, action, plain function or table row, and what only it would
// have declared (helpers, literals, expectations) is not declared either. A rule that returns a tree
// has a value-free twin that every call from a value-free context uses, so the original of a rule
// that only such contexts reference is left out, and so are the originals that only it calls.
//
// Rule IDs and the numbering of memoized rules (Seen) are those of the compiled program, so the
// remaining rules keep their values. The printer checks that no printed call reaches a rule left out.

// tsLive holds, for each table ("rules" or "recRules"), the ways in which the rules are entered.
type tsLive struct {
	reach map[string][]core.Entry
}

// reachableRules returns the rules of prog and of the recognizer rec (nil if there is none) that a
// parse can enter: every start rule of the Node table, which parseRule can name, and the rule start
// that parse and recognize run.
func reachableRules(prog, rec *compile.Program, start *compile.Rule) (*tsLive, error) {
	return reachFrom(prog, rec, start, func(table string, p *compile.Program) (*core.Module, error) {
		return irbuild.CoreFrom(p, nil)
	})
}

// printedReach returns the rules that a parse can enter through the calls that the generated code
// prints (checkCall), from the roots of reachableRules. Printing leaves out the calls that Core IR
// has at points that the lowered code never reaches, such as after a lookahead that always fails.
func (g *Generator) printedReach(prog, rec *compile.Program, start *compile.Rule) *tsLive {
	l, _ := reachFrom(prog, rec, start, func(table string, p *compile.Program) (*core.Module, error) {
		m := &core.Module{Calls: make([][]int, len(p.RuleList)+len(p.Twins))}
		for from, to := range g.printed[table] {
			m.Calls[from] = to
		}
		m.Rules = make([]*core.Rule, len(m.Calls))
		return m, nil
	})
	return l
}

// reachFrom computes the reach of reachableRules over the call graph of the module that module
// returns for each table.
func reachFrom(prog, rec *compile.Program, start *compile.Rule, module func(table string, p *compile.Program) (*core.Module, error)) (*tsLive, error) {
	l := &tsLive{reach: map[string][]core.Entry{}}
	for table, p := range map[string]*compile.Program{"rules": prog, "recRules": rec} {
		if p == nil {
			continue
		}
		m, err := module(table, p)
		if err != nil {
			return nil, err
		}
		var roots []core.Root
		if table == "rules" {
			for _, name := range p.StartRules {
				roots = append(roots, core.Root{Rule: p.ByName[name].ID, Entry: core.EnterSlot})
			}
		}
		roots = append(roots, core.Root{Rule: p.ByName[start.Name].ID, Entry: core.EnterSlot})
		rules := append(append([]*compile.Rule(nil), p.RuleList...), p.Twins...)
		l.reach[table] = m.Reach(roots, func(id int) core.Entry {
			if callsPlain(rules[id]) {
				return core.EnterWrapper
			}
			return core.EnterSlot
		})
	}
	return l, nil
}

// equal reports whether l and o enter the same rules in the same ways.
func (l *tsLive) equal(o *tsLive) bool {
	if len(l.reach) != len(o.reach) {
		return false
	}
	for table, r := range l.reach {
		if !slices.Equal(r, o.reach[table]) {
			return false
		}
	}
	return true
}

// callsPlain reports whether a call of the rule r is printed as a call of its plain function
// (tsFlowPrinter.call) rather than through its constant.
func callsPlain(r *compile.Rule) bool {
	_, operatorsRule := r.Def.Expr.(*grammar.OperatorsRule)
	return r.Plain && !operatorsRule
}

// pruned reports whether the rule r of the current table is left out.
func (g *Generator) pruned(r *compile.Rule) bool {
	return g.live != nil && g.live.reach[g.Table][r.ID] == 0
}

// slotReached reports whether something enters the rule r through its constant (and so its body
// assignment), and not only through its plain function.
func (g *Generator) slotReached(r *compile.Rule) bool {
	return g.live == nil || g.live.reach[g.Table][r.ID]&core.EnterSlot != 0
}

// tableRow reports whether the constant of the rule r is declared: something reads it, which is
// the entry through its slot, or the plain function of a rule with a value (it finishes the value
// with the rule). A rule without a value that only its plain function enters needs no constant.
func (g *Generator) tableRow(r *compile.Rule) bool {
	return g.slotReached(r) || !r.Novalue
}

// checkCall checks that the callee of a printed call was not left out.
func (g *Generator) checkCall(r *compile.Rule) {
	if g.Cur != nil {
		calls := g.printed[g.Table]
		if calls == nil {
			calls = map[int][]int{}
			g.printed[g.Table] = calls
		}
		if !slices.Contains(calls[g.Cur.ID], r.ID) {
			calls[g.Cur.ID] = append(calls[g.Cur.ID], r.ID)
		}
	}
	if g.pruned(r) && g.reachErr == nil {
		g.reachErr = fmt.Errorf("internal error: a generated call reaches the rule %s of %s, whose body is left out", r.Name, g.Table)
	}
}
