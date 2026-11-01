package compile

import (
	"zeroparse.dev/zp/grammar"
)

// maxFiniteValues caps the set of values that finiteVars computes for an int variable.
const maxFiniteValues = 4096

// finiteVars returns the variables whose values in every parse lie in a set bounded by a constant
// of the grammar: every bool variable, and every int variable whose initial value and bindings
// reach at most maxFiniteValues values (see finiteInt).
func (k *classifier) finiteVars() map[string]bool {
	byVar := map[string][]*binding{}
	for _, b := range k.binds {
		byVar[b.b.Name] = append(byVar[b.b.Name], b)
	}
	out := map[string]bool{}
	for _, v := range k.g.Vars() {
		switch k.varType[v.Name] {
		case "bool":
			out[v.Name] = true
		case "int":
			out[v.Name] = k.finiteInt(v, byVar[v.Name])
		}
	}
	return out
}

// finiteInt reports whether the values of the int variable v form a set of at most
// maxFiniteValues values: the least set that holds the initial value and, for each binding of v,
// the value that the binding gives from each value of the set that passes its guards. Each binding
// must read only v and literals. A binding that fails to evaluate, as by a division by zero,
// fails and binds nothing; one whose value eval does not know leaves the set unbounded.
func (k *classifier) finiteInt(v *grammar.VarDef, binds []*binding) bool {
	init := 0
	if v.Value != nil {
		r, res := eval(v.Value, v.Name, 0)
		x, isInt := r.(int)
		if res != evalOK || !isInt {
			return false
		}
		init = x
	}
	guards := make([][]grammar.Term, len(binds))
	for i, b := range binds {
		if !readsOnly(b.b.Value, v.Name) {
			return false
		}
		guards[i] = k.guards(b)
	}
	set := map[int]bool{init: true}
	work := []int{init}
	for len(work) > 0 {
		x := work[len(work)-1]
		work = work[:len(work)-1]
		for i, b := range binds {
			if !passes(guards[i], v.Name, x) {
				continue
			}
			r, res := eval(b.b.Value, v.Name, x)
			if res == evalFailed {
				continue
			}
			y, isInt := r.(int)
			if res != evalOK || !isInt {
				return false
			}
			if set[y] {
				continue
			}
			if len(set) == maxFiniteValues {
				return false
			}
			set[y] = true
			work = append(work, y)
		}
	}
	return true
}

// guards returns the guards of the binding b: the predicates that read only its variable and
// literals, and that every evaluation of b follows with the same value of the variable. Such a
// predicate is an earlier element of a sequence around b, which b reaches through groups,
// options, choices, captures, discards and atomic expressions only, so that no repetition runs
// between them, with no binding of the variable in the elements between them. A binding in a
// called rule is not visible to its caller, so a call between them does not change the variable.
func (k *classifier) guards(b *binding) []grammar.Term {
	v := b.b.Name
	frames := b.c.seqs[b.c.strictFrom:]
	var out []grammar.Term
	for i, f := range frames {
		for g := f.index - 1; g >= 0; g-- {
			it := f.seq.Items[g]
			if p, ok := it.(*grammar.Predicate); ok && isTest(p.Term) && readsOnly(p.Term, v) {
				if !bindsBetween(frames, i, g, v) {
					out = append(out, p.Term)
				}
			}
		}
	}
	return out
}

// bindsBetween reports whether a binding of v lies after element g of frames[i] and before the
// binding at the innermost frame: in the elements of frames[i] after g, or in the elements before
// the path of a later frame.
func bindsBetween(frames []seqFrame, i, g int, v string) bool {
	items := frames[i].seq.Items[g+1 : frames[i].index]
	for _, f := range frames[i+1:] {
		items = append(items[:len(items):len(items)], f.seq.Items[:f.index]...)
	}
	found := false
	for _, it := range items {
		WalkExpr(it, func(e grammar.Expr) {
			if p, ok := e.(*grammar.Predicate); ok {
				WalkTerm(p.Term, func(t grammar.Term) {
					if bd, ok := t.(*grammar.Bind); ok && bd.Name == v {
						found = true
					}
				})
			}
		})
	}
	return found
}

// isTest reports whether t is a test rather than a binding or a block of statements.
func isTest(t grammar.Term) bool {
	switch t.(type) {
	case *grammar.Bind, *grammar.Block:
		return false
	}
	return true
}

// readsOnly reports whether t is built from the variable v, int and bool literals and the
// operators that eval knows only.
func readsOnly(t grammar.Term, v string) bool {
	switch t := t.(type) {
	case *grammar.IntLit, *grammar.BoolLit:
		return true
	case *grammar.VarRef:
		return t.Name == v
	case *grammar.TermGroup:
		return readsOnly(t.Term, v)
	case *grammar.Unary:
		return (t.Op == "-" || t.Op == "!") && readsOnly(t.X, v)
	case *grammar.Binary:
		return intOps[t.Op] && readsOnly(t.L, v) && readsOnly(t.R, v)
	}
	return false
}

var intOps = map[string]bool{"+": true, "-": true, "*": true, "/": true, "%": true,
	"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true, "&&": true, "||": true}

// passes reports whether every guard can hold when v is x. A guard that fails to evaluate fails,
// as a predicate does; one whose value is not known can hold.
func passes(guards []grammar.Term, v string, x int) bool {
	for _, g := range guards {
		r, res := eval(g, v, x)
		if b, isBool := r.(bool); res == evalFailed || res == evalOK && (!isBool || !b) {
			return false
		}
	}
	return true
}

// evalResult is the outcome of eval.
type evalResult uint8

const (
	evalOK      evalResult = iota
	evalFailed             // an error of the action language, such as a division by zero
	evalUnknown            // an int beyond what every backend computes alike, or a term eval does not know
)

// maxExact bounds the ints that eval computes: every backend computes them alike.
const maxExact = 1 << 53

// eval evaluates t, which readsOnly accepts, with v bound to x, by the rules of the action
// language: division truncates toward zero, the remainder has the sign of the dividend, and
// dividing by zero is an error.
func eval(t grammar.Term, v string, x int) (any, evalResult) {
	switch t := t.(type) {
	case *grammar.IntLit:
		return exact(t.Value)
	case *grammar.BoolLit:
		return t.Value, evalOK
	case *grammar.VarRef:
		if t.Name == v {
			return x, evalOK
		}
	case *grammar.TermGroup:
		return eval(t.Term, v, x)
	case *grammar.Unary:
		r, res := eval(t.X, v, x)
		if res != evalOK {
			return nil, res
		}
		switch r := r.(type) {
		case int:
			if t.Op == "-" {
				return exact(-r)
			}
		case bool:
			if t.Op == "!" {
				return !r, evalOK
			}
		}
	case *grammar.Binary:
		l, res := eval(t.L, v, x)
		if res != evalOK {
			return nil, res
		}
		if lb, isBool := l.(bool); isBool && (t.Op == "&&" || t.Op == "||") {
			if lb == (t.Op == "||") {
				return lb, evalOK
			}
			r, res := eval(t.R, v, x)
			if _, isBool := r.(bool); res == evalOK && !isBool {
				return nil, evalUnknown
			}
			return r, res
		}
		r, res := eval(t.R, v, x)
		if res != evalOK {
			return nil, res
		}
		return binaryOp(t.Op, l, r)
	}
	return nil, evalUnknown
}

func binaryOp(op string, l, r any) (any, evalResult) {
	if lb, isBool := l.(bool); isBool {
		if rb, isBool := r.(bool); isBool {
			switch op {
			case "==":
				return lb == rb, evalOK
			case "!=":
				return lb != rb, evalOK
			}
		}
		return nil, evalUnknown
	}
	li, lInt := l.(int)
	ri, rInt := r.(int)
	if !lInt || !rInt {
		return nil, evalUnknown
	}
	switch op {
	case "+":
		return exact(li + ri)
	case "-":
		return exact(li - ri)
	case "*":
		if li != 0 && (abs(ri) > maxExact/abs(li)) {
			return nil, evalUnknown
		}
		return exact(li * ri)
	case "/", "%":
		if ri == 0 {
			return nil, evalFailed
		}
		if op == "/" {
			return li / ri, evalOK
		}
		return li % ri, evalOK
	case "==":
		return li == ri, evalOK
	case "!=":
		return li != ri, evalOK
	case "<":
		return li < ri, evalOK
	case "<=":
		return li <= ri, evalOK
	case ">":
		return li > ri, evalOK
	case ">=":
		return li >= ri, evalOK
	}
	return nil, evalUnknown
}

func exact(n int) (any, evalResult) {
	if abs(n) > maxExact {
		return nil, evalUnknown
	}
	return n, evalOK
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
