package compile

import (
	"maps"

	"zeroparse.dev/zp/grammar"
)

// The growth of a fold accumulator. One step of a fold maps its accumulator A to the value of its
// lambda. growthShape bounds the size of that value, the size of its largest element and its
// number of elements by forms in the same three quantities of A, the sizes of the values the
// lambda reads from outside the fold, and the size of the element of the step. The forms make the
// step matrix of the fold, from which checkFoldGrowth decides how the accumulator can grow over
// the steps and what the steps cost (see Lemma 7 of spec/proofs/linear-cost.md).
//
// The size of a string is its number of characters; the size of a list, a tuple or a struct is
// one more than its number of elements plus the sizes of its elements, so that a list of N
// elements of total size T has size 1 + N + T.

// The variables of a monomial: the size, the largest element and the number of elements of the
// accumulator; and the size of the current element of a lambda inside the step (see sumOver).
const (
	varS = iota
	varM
	varN
	varE
	vars
)

// mono is a monomial: a product of powers of the variables, of O, the size of a value read from
// outside the fold, and of Y, the size of the element of the step, whose sum over the steps is at
// most the size of the list. The degrees assume that O, the initial value and the list are at most
// linear in the size of the input; a value of a rule that grows faster than the input makes them
// too low.
type mono struct {
	x    [vars]int8
	o, y int8
}

var unitMono = mono{}

// one returns the monomial of variable v.
func one(v int) mono {
	var m mono
	m.x[v] = 1
	return m
}

func (m mono) mul(n mono) mono {
	for i := range m.x {
		m.x[i] += n.x[i]
	}
	m.o += n.o
	m.y += n.y
	return m
}

// accDegree returns the degree of m in the quantities of the accumulator and the current element.
func (m mono) accDegree() int { return int(m.x[varS] + m.x[varM] + m.x[varN] + m.x[varE]) }

// many is a coefficient that stands for any larger constant.
const many = 64

// lin is a polynomial with coefficients from 1 to many. The bounds on values are linear in the
// quantities of the accumulator; the bounds on costs need not be.
type lin map[mono]int8

func addCoef(a, b int8) int8 { return int8(min(int(a)+int(b), many)) }

func mulCoef(a, b int8) int8 { return int8(min(int(a)*int(b), many)) }

func (a lin) plus(b lin) lin {
	out := maps.Clone(a)
	if out == nil {
		out = lin{}
	}
	for m, c := range b {
		out[m] = addCoef(out[m], c)
	}
	return out
}

func (a lin) join(b lin) lin {
	out := maps.Clone(a)
	if out == nil {
		out = lin{}
	}
	for m, c := range b {
		out[m] = max(out[m], c)
	}
	return out
}

func (a lin) times(p lin) lin {
	out := lin{}
	for m, c := range a {
		for n, d := range p {
			k := m.mul(n)
			out[k] = addCoef(out[k], mulCoef(c, d))
		}
	}
	return out
}

// split returns the terms of a of degree 1 in v, divided by v, and the terms without v; ok is
// false when a has a higher power of v.
func (a lin) split(v int) (with, without lin, ok bool) {
	with, without = lin{}, lin{}
	for m, c := range a {
		switch m.x[v] {
		case 0:
			without[m] = c
		case 1:
			m.x[v] = 0
			with[m] = c
		default:
			return nil, nil, false
		}
	}
	return with, without, true
}

func (a lin) has(vs ...int) bool {
	for m := range a {
		for _, v := range vs {
			if m.x[v] != 0 {
				return true
			}
		}
	}
	return false
}

// fromAcc reports whether a depends on the accumulator.
func (a lin) fromAcc() bool { return a.has(varS, varM, varN) }

// linear reports whether every term of a has degree at most 1 in the accumulator and the current
// element.
func (a lin) linear() bool {
	for m := range a {
		if m.accDegree() > 1 {
			return false
		}
	}
	return true
}

// subst replaces the size of the current element with v.
func (a lin) subst(v lin) lin {
	out := lin{}
	for m, c := range a {
		k := int(m.x[varE])
		m.x[varE] = 0
		t := lin{m: c}
		for range k {
			t = t.times(v)
		}
		out = out.plus(t)
	}
	return out
}

// constLin returns the form of the constant c.
func constLin(c int) lin {
	if c <= 0 {
		return lin{}
	}
	return lin{unitMono: int8(min(c, many))}
}

var (
	outsideLin = lin{{o: 1}: 1}
	stepLin    = lin{{y: 1}: 1}
	manyLin    = lin{one(varS): many, one(varM): many, one(varN): many, unitMono: many, {o: 1}: many, {y: 1}: many}
)

// linearValue returns a when it is linear in the accumulator and the current element, and otherwise
// many copies of everything, as a bound on the value of a step must be linear in them.
func linearValue(a lin) lin {
	if !a.linear() {
		return manyLin
	}
	return a
}

// growthShape bounds a value: its size S, its number of elements N, and the shape of each of its
// elements, El, or nil when only the size of the value bounds an element.
type growthShape struct {
	S, N lin
	El   *growthShape
}

var manyShape = growthShape{S: manyLin, N: manyLin}

// sized returns the shape of a value of size s whose elements are bounded by s.
func sized(s lin) growthShape { return growthShape{S: s, N: s} }

// elem returns the shape of an element of x.
func (x growthShape) elem() growthShape {
	if x.El != nil {
		return *x.El
	}
	return sized(x.S)
}

// m returns the bound on the size of the largest element of x.
func (x growthShape) m() lin { return x.elem().S }

func (x growthShape) join(y growthShape) growthShape {
	j := growthShape{S: x.S.join(y.S), N: x.N.join(y.N)}
	if x.El != nil || y.El != nil {
		e := x.elem().join(y.elem())
		j.El = &e
	}
	return j
}

// subst replaces the size of the current element with v in every bound of x.
func (x growthShape) subst(v lin) growthShape {
	out := growthShape{S: x.S.subst(v), N: x.N.subst(v)}
	if x.El != nil {
		e := x.El.subst(v)
		out.El = &e
	}
	return out
}

// grows reports whether x depends on the accumulator or on the current element of a lambda.
func (x growthShape) grows() bool {
	return x.S.has(varS, varM, varN, varE) || x.N.has(varS, varM, varN, varE) || x.El != nil && x.El.grows()
}

// hasElem reports whether x depends on the current element of a lambda.
func (x growthShape) hasElem() bool {
	return x.S.has(varE) || x.N.has(varE) || x.El != nil && x.El.hasElem()
}

// accShape is the shape of the accumulator itself.
var accShape = func() growthShape {
	el := sized(lin{one(varM): 1})
	return growthShape{S: lin{one(varS): 1}, N: lin{one(varN): 1}, El: &el}
}()

// elemParam returns the shape of the parameter of a lambda over the elements of x: an element of
// size E, whose number of elements is the bound of the elements of x when that does not depend on
// the accumulator.
func elemParam(x growthShape) growthShape {
	p := sized(lin{one(varE): 1})
	if el := x.elem(); !el.N.fromAcc() && !el.N.has(varE) {
		p.N = el.N
	}
	return p
}

// sumOver bounds the sum, over the elements of the list x, of q, a bound for one element that may
// depend on the size E of the element. With the elements of sizes E_i, their number N and the size
// S of x, the sum of E_i is at most S - N, so q = a E + b sums to at most a S + (b - a) N for
// constants a and b; the sum of E_i^k times what does not depend on the element is at most S^k
// times it, and the sum of what does not depend on the element is N times it.
func sumOver(x growthShape, q lin) lin {
	out := lin{}
	a, b := q[one(varE)], q[unitMono]
	for m, c := range q {
		k := int(m.x[varE])
		m.x[varE] = 0
		t := lin{m: c}
		if k == 0 {
			if m == unitMono && b < many {
				if t[unitMono] = b - a; t[unitMono] <= 0 {
					continue
				}
			}
			out = out.plus(t.times(x.N))
			continue
		}
		for range k {
			t = t.times(x.S)
		}
		out = out.plus(t)
	}
	return out
}

// growthEnv maps the parameters of the lambdas around a term to their shapes.
type growthEnv map[string]growthShape

// readsGrowing reports whether t reads a parameter of env whose shape depends on the accumulator
// or on the current element, other than the parameters in shadow.
func readsGrowing(t grammar.Term, env growthEnv, shadow []string) bool {
	return readsParam(t, env, shadow, growthShape.grows)
}

// readsElem reports whether t reads a parameter of env whose shape depends on the current element
// of a lambda, other than the parameters in shadow. A lambda inside it has an element of its own,
// so the forms cannot follow such a read there.
func readsElem(t grammar.Term, env growthEnv, shadow []string) bool {
	return readsParam(t, env, shadow, growthShape.hasElem)
}

func readsParam(t grammar.Term, env growthEnv, shadow []string, pred func(growthShape) bool) bool {
	found := false
	WalkTerm(t, func(x grammar.Term) {
		c, ok := x.(*grammar.CaptureRef)
		if !ok {
			return
		}
		for _, n := range shadow {
			if n == c.Name {
				return
			}
		}
		if p, ok := env[c.Name]; ok && pred(p) {
			found = true
		}
	})
	return found
}

// growthOf bounds the value of t in env, and the cost of evaluating it: the sizes of the values
// that its copying operations build, a string by its characters and a list by its elements, which
// it shares. A term whose value it does not follow, such as a block or a call of an action function,
// can be anything as large as many copies of what it reads, and cost as much.
func (k *classifier) growthOf(t grammar.Term, env growthEnv, tc termCtx) (growthShape, lin) {
	var zero growthShape
	// A list, a tuple or a struct has one element for each of ts, and a size of one more than
	// their number plus their sizes.
	build := func(ts []grammar.Term) (growthShape, lin) {
		out := growthShape{S: constLin(len(ts) + 1), N: constLin(len(ts))}
		cost := constLin(len(ts))
		var el *growthShape
		for _, x := range ts {
			g, c := k.growthOf(x, env, tc)
			out.S, cost = out.S.plus(g.S), cost.plus(c)
			if el == nil {
				el = &g
			} else {
				j := el.join(g)
				el = &j
			}
		}
		out.El = el
		return out, cost
	}
	costOf := func(ts ...grammar.Term) lin {
		cost := lin{}
		for _, x := range ts {
			_, c := k.growthOf(x, env, tc)
			cost = cost.plus(c)
		}
		return cost
	}
	switch t := t.(type) {
	case nil, *grammar.IntLit, *grammar.BoolLit, *grammar.NilLit:
		return zero, nil
	case *grammar.StringLit:
		return sized(constLin(len(t.Value))), nil
	case *grammar.Unary:
		return zero, costOf(t.X)
	case *grammar.CaptureRef:
		if p, ok := env[t.Name]; ok {
			return p, nil
		}
		if k.isInt(t, tc) {
			return zero, nil
		}
		// A value bound outside the fold.
		el := sized(outsideLin)
		return growthShape{S: outsideLin, N: outsideLin, El: &el}, nil
	case *grammar.VarRef, *grammar.IndexRef:
		if k.isInt(t, tc) {
			return zero, nil
		}
		el := sized(outsideLin)
		return growthShape{S: outsideLin, N: outsideLin, El: &el}, nil
	case *grammar.TermGroup:
		return k.growthOf(t.Term, env, tc)
	case *grammar.Cast:
		return k.growthOf(t.X, env, tc)
	case *grammar.Member:
		// A field or a part of the value, at most the value.
		g, c := k.growthOf(t.X, env, tc)
		return sized(g.S), c
	case *grammar.TupleLit:
		return build(t.Elems)
	case *grammar.ListConstructor:
		return build(t.Items)
	case *grammar.New:
		ts := append([]grammar.Term(nil), t.Args...)
		for _, f := range t.Fields {
			ts = append(ts, f.Value)
		}
		return build(ts)
	case *grammar.Binary:
		switch {
		case compareOps[t.Op] && !k.isInt(t.L, tc) && !k.isInt(t.R, tc):
			// A comparison examines its operands.
			l, lc := k.growthOf(t.L, env, tc)
			r, rc := k.growthOf(t.R, env, tc)
			return zero, lc.plus(rc).plus(l.S).plus(r.S)
		case scalarOps[t.Op] || t.Op == "+" && (k.isInt(t.L, tc) || k.isInt(t.R, tc)) || t.Op != "+" && t.Op != "[]":
			return zero, costOf(t.L, t.R)
		case t.Op == "+":
			l, lc := k.growthOf(t.L, env, tc)
			r, rc := k.growthOf(t.R, env, tc)
			s := l.S.plus(r.S)
			return sized(s), lc.plus(rc).plus(s)
		default:
			// An element taken by an index, which is an int and adds nothing to its size.
			l, lc := k.growthOf(t.L, env, tc)
			return l.elem(), lc.plus(costOf(t.R))
		}
	case *grammar.Call:
		switch t.Func {
		case "len":
			// len examines all of a string value.
			cost := lin{}
			for _, a := range t.Args {
				g, c := k.growthOf(a, env, tc)
				cost = cost.plus(c)
				if k.mayBeString(a) {
					cost = cost.plus(g.S)
				}
			}
			return zero, cost
		case "text":
			if len(t.Args) == 1 {
				g, c := k.growthOf(t.Args[0], env, tc)
				return sized(g.S), c.plus(g.S)
			}
		case "list":
			return build(t.Args)
		case "concat":
			// Concatenating lists of sizes S_i gives at most their sum.
			out := growthShape{S: constLin(1), N: lin{}}
			if len(t.Args) > 0 {
				out.S = lin{}
			}
			cost := lin{}
			var el *growthShape
			for _, a := range t.Args {
				g, c := k.growthOf(a, env, tc)
				out.S, out.N, cost = out.S.plus(g.S), out.N.plus(g.N), cost.plus(c)
				e := g.elem()
				if el != nil {
					e = el.join(e)
				}
				el = &e
			}
			out.El = el
			return out, cost.plus(out.N)
		case "filter":
			if lam, ok := lambdaArg(t, 1); ok {
				return k.filterGrowth(t.Args[0], lam, env, tc)
			}
		case "map":
			if lam, ok := lambdaArg(t, 1); ok {
				return k.mapGrowth(t.Args[0], lam, env, tc)
			}
		case "foldl", "foldr":
			if lam, ok := lambdaArg(t, 2); ok {
				return k.foldGrowth(t.Args[0], t.Args[1], lam, env, tc)
			}
		}
	}
	if readsGrowing(t, env, nil) {
		return manyShape, manyLin
	}
	// What a term that the forms do not follow reads from outside the fold and from the element
	// of the step bounds its value.
	s := constLin(many).plus(outsideLin.times(constLin(many))).plus(stepLin.times(constLin(many)))
	return sized(s), s
}

// lambdaEnv returns env with the parameters of lam bound to the elements of x.
func lambdaEnv(env growthEnv, lam *grammar.Lambda, x growthShape) growthEnv {
	inner := maps.Clone(env)
	if inner == nil {
		inner = growthEnv{}
	}
	for _, n := range lam.Params {
		inner[n] = elemParam(x)
	}
	return inner
}

// filterGrowth bounds filter(list, lam): at most the list, since it can keep every element.
func (k *classifier) filterGrowth(list grammar.Term, lam *grammar.Lambda, env growthEnv, tc termCtx) (growthShape, lin) {
	x, xc := k.growthOf(list, env, tc)
	if readsElem(lam.Body, env, lam.Params) {
		return manyShape, manyLin
	}
	_, bc := k.growthOf(lam.Body, lambdaEnv(env, lam, x), tc)
	return x, xc.plus(sumOver(x, bc)).plus(x.N)
}

// mapGrowth bounds map(list, lam): an element for each element of the list, each the value of
// the lambda, so a size of one plus the sum over the elements of one more than that value.
func (k *classifier) mapGrowth(list grammar.Term, lam *grammar.Lambda, env growthEnv, tc termCtx) (growthShape, lin) {
	x, xc := k.growthOf(list, env, tc)
	if readsElem(lam.Body, env, lam.Params) {
		// The lambda reads a value from outside it once for each element.
		return manyShape, manyLin
	}
	b, bc := k.growthOf(lam.Body, lambdaEnv(env, lam, x), tc)
	el := b.subst(x.m())
	return growthShape{
		S:  linearValue(constLin(1).plus(sumOver(x, b.S.plus(constLin(1))))),
		N:  x.N,
		El: &el,
	}, xc.plus(sumOver(x, bc)).plus(x.N)
}

// foldGrowth bounds a fold with the initial value init over list inside the lambda of another
// fold. Its value is that of its last step, or the initial value, when its lambda does not read
// its own accumulator, and the initial value joined with the value that each step adds when the
// lambda joins its accumulator once with a value that does not read it. Any other use of its own
// accumulator can double it. Its cost includes the copies of its own accumulator that its steps
// make, once for each element of its list.
func (k *classifier) foldGrowth(initTerm, list grammar.Term, lam *grammar.Lambda, env growthEnv, tc termCtx) (growthShape, lin) {
	init, ic := k.growthOf(initTerm, env, tc)
	x, xc := k.growthOf(list, env, tc)
	cost := ic.plus(xc)
	if readsElem(lam.Body, env, lam.Params) {
		return manyShape, manyLin
	}
	if len(lam.Params) == 0 {
		return init, cost
	}
	inner := lambdaEnv(env, lam, x)
	inner[lam.Params[0]] = growthShape{}
	s := lam.Params[0]
	if !readsName(lam.Body, s) {
		b, bc := k.growthOf(lam.Body, inner, tc)
		return init.join(b.subst(x.m())), cost.plus(sumOver(x, bc))
	}
	rest, isConcat, ok := k.joinsOnce(lam.Body, s)
	if !ok {
		if init.grows() || x.grows() {
			return manyShape, manyLin
		}
		return k.growthOf(&grammar.Block{}, env, tc)
	}
	var q growthShape
	var qc lin
	if isConcat {
		q, qc = k.growthOf(&grammar.Call{Func: "concat", Args: rest}, inner, tc)
	} else {
		q = growthShape{S: lin{}}
		qc = lin{}
		for _, r := range rest {
			g, c := k.growthOf(r, inner, tc)
			q.S, qc = q.S.plus(g.S), qc.plus(c)
		}
		q.N = q.S
	}
	// Each step copies the accumulator, at most the value of the fold, once for each element.
	cost = cost.plus(sumOver(x, qc))
	if !isConcat {
		s := init.S.plus(sumOver(x, q.S))
		return sized(linearValue(s)), cost.plus(x.N.times(s))
	}
	// Joining a list to the accumulator adds its size less one.
	add := q.S
	if c := add[unitMono]; c > 0 && c < many {
		add = maps.Clone(add)
		if c == 1 {
			delete(add, unitMono)
		} else {
			add[unitMono] = c - 1
		}
	}
	el := init.elem().join(q.elem().subst(x.m()))
	n := init.N.plus(sumOver(x, q.N))
	return growthShape{
		S:  linearValue(init.S.plus(sumOver(x, add))),
		N:  linearValue(n),
		El: &el,
	}, cost.plus(x.N.times(n))
}

// joinsOnce matches a lambda body that joins the accumulator s once with other values, which do
// not read it: concat with s as one argument, or a join of strings with + with s as one operand. It
// returns the other values and whether the join is concat.
func (k *classifier) joinsOnce(body grammar.Term, s string) (rest []grammar.Term, isConcat, ok bool) {
	var parts []grammar.Term
	switch b := ungroupTerm(body).(type) {
	case *grammar.Call:
		if b.Func != "concat" {
			return nil, false, false
		}
		parts, isConcat = b.Args, true
	case *grammar.Binary:
		if b.Op != "+" {
			return nil, false, false
		}
		parts = k.joined(b, nil)
	default:
		return nil, false, false
	}
	found := false
	for _, p := range parts {
		if r, isRef := ungroupTerm(p).(*grammar.CaptureRef); isRef && r.Name == s && !found {
			found = true
			continue
		}
		if readsName(p, s) {
			return nil, false, false
		}
		rest = append(rest, p)
	}
	return rest, isConcat, found
}

// readsName reports whether t reads the capture or parameter name.
func readsName(t grammar.Term, name string) bool {
	found := false
	WalkTerm(t, func(x grammar.Term) {
		if c, ok := x.(*grammar.CaptureRef); ok && c.Name == name {
			found = true
		}
	})
	return found
}

// lambdaArg returns argument i of call when it is a lambda.
func lambdaArg(call *grammar.Call, i int) (*grammar.Lambda, bool) {
	if len(call.Args) <= i {
		return nil, false
	}
	lam, ok := call.Args[i].(*grammar.Lambda)
	return lam, ok
}

// stepForms returns the step matrix of the fold call with lambda lam, as forms that bound the
// size, the largest element and the number of elements of the value of the lambda, and the form
// that bounds the cost of one step. ok is false when the accumulator is an int or the lambda has
// no parameter.
func (k *classifier) stepForms(call *grammar.Call, lam *grammar.Lambda, tc termCtx) (rows [3]lin, cost lin, ok bool) {
	if len(lam.Params) == 0 || len(call.Args) > 0 && k.isInt(call.Args[0], tc) {
		return rows, nil, false
	}
	params := maps.Clone(tc.params)
	if params == nil {
		params = map[string]param{}
	}
	for j, n := range lam.Params {
		params[n] = param{acc: j == 0}
	}
	tc.params, tc.lambda = params, true
	env := growthEnv{lam.Params[0]: accShape}
	step := sized(stepLin)
	step.El = &growthShape{S: stepLin, N: stepLin}
	for _, n := range lam.Params[1:] {
		env[n] = step
	}
	v, cost := k.growthOf(lam.Body, env, tc)
	rows = [3]lin{v.S, v.m(), v.N}
	for i, r := range rows {
		if r.has(varE) {
			rows[i] = manyLin
		}
	}
	if cost.has(varE) {
		cost = manyLin
	}
	return rows, cost, true
}

// accVar returns the quantity of the accumulator that the monomial m of a linear form reads, or
// -1 for a monomial in O and Y only.
func accVar(m mono) int {
	for v := range varE {
		if m.x[v] != 0 {
			return v
		}
	}
	return -1
}

// growthOrder decides how the quantities of an accumulator can grow over the steps of a fold whose
// step has the linear forms rows. It reports exp when some cycle of the graph of the quantities,
// with an edge for each monomial that reads one, has a coefficient other than 1 or a factor of O
// or Y, or when two cycles share a quantity. Otherwise deg[i] is the degree in the size of the
// input of the bound on quantity i over all steps (see Lemma 7).
func growthOrder(rows [3]lin) (exp bool, deg [3]int) {
	var reach [3][3]bool
	for i, r := range rows {
		for m := range r {
			if v := accVar(m); v >= 0 {
				reach[i][v] = true
			}
		}
	}
	for m := range 3 {
		for i := range 3 {
			for j := range 3 {
				reach[i][j] = reach[i][j] || reach[i][m] && reach[m][j]
			}
		}
	}
	scc := func(i, j int) bool { return i == j || reach[i][j] && reach[j][i] }
	cyclic := [3]bool{}
	for i, r := range rows {
		if !reach[i][i] {
			continue
		}
		cyclic[i] = true
		// Inside its component, i depends on exactly one quantity, with coefficient 1.
		edges := 0
		for m, c := range r {
			if v := accVar(m); v >= 0 && scc(i, v) {
				edges++
				if c != 1 || m.o != 0 || m.y != 0 {
					return true, deg
				}
			}
		}
		if edges != 1 {
			return true, deg
		}
	}
	// A quantity on no cycle is bounded at each step by what it reads; one on a cycle adds what
	// it reads from outside its cycle on every step, which the sum over the steps raises by one
	// degree, or by the degree of Y, whose sum over the steps is at most the input. The initial
	// value has degree 1.
	for range 4 {
		for i, r := range rows {
			d := 1
			for m := range r {
				base := 0
				if v := accVar(m); v >= 0 {
					if scc(i, v) {
						continue
					}
					base = deg[v]
				}
				if cyclic[i] {
					d = max(d, base+int(m.o)+max(int(m.y), 1))
				} else {
					d = max(d, base+int(m.o)+int(m.y))
				}
			}
			deg[i] = d
		}
		for i := range 3 {
			for j := range 3 {
				if scc(i, j) {
					deg[i] = max(deg[i], deg[j])
				}
			}
		}
	}
	return false, deg
}

// stepsCost returns the degree in the size of the input of the cost of all the steps of a fold
// whose step costs cost, with the quantities of the accumulator of degrees deg: a term costs the
// product of its quantities on each step, which the sum over the steps raises by one degree, or
// by the degree of Y.
func stepsCost(cost lin, deg [3]int) int {
	d := 0
	for m := range cost {
		base := 0
		for v := range varE {
			base += int(m.x[v]) * deg[v]
		}
		d = max(d, base+int(m.o)+max(int(m.y), 1))
	}
	return d
}

// doubles reports whether the accumulator of the fold call with lambda lam can grow
// exponentially with the number of steps.
func (k *classifier) doubles(call *grammar.Call, lam *grammar.Lambda, tc termCtx) bool {
	rows, _, ok := k.stepForms(call, lam, tc)
	if !ok {
		return false
	}
	exp, _ := growthOrder(rows)
	return exp
}

// checkFoldGrowth reports a fold whose accumulator can grow exponentially with its steps, though no
// single copying operation copies it twice, as by duplicating the elements of a list and joining
// them, and a fold over a list that is not bounded whose steps cost a polynomial of degree 3 or more
// in all, times the evaluations of the lambdas around it.
func (k *classifier) checkFoldGrowth(call *grammar.Call, lam *grammar.Lambda, tc termCtx) {
	rows, cost, ok := k.stepForms(call, lam, tc)
	if !ok {
		return
	}
	pos := termPosOf(call)
	exp, deg := growthOrder(rows)
	if exp {
		k.cause(ClassExp, 0, tc.rule, pos, "%s: the value of the lambda of %s at %s can hold its accumulator %s more than once, which can double it on each step",
			degreeText(ClassExp, 0), k.showTerm(call), at(tc.rule, pos), lam.Params[0])
		return
	}
	d := stepsCost(cost, deg)
	if d < 3 || len(call.Args) < 2 || k.sizeOf(call.Args[1], tc, listSize).bounded() {
		return
	}
	// The fold runs once for each evaluation of the lambdas around it.
	d += k.foldRuns(call.Args[1], tc)
	k.cause(ClassPoly, d, tc.rule, pos, "%s: the steps of %s at %s build values that grow with the steps, the input and its accumulator %s",
		degreeText(ClassPoly, d), k.showTerm(call), at(tc.rule, pos), lam.Params[0])
}
