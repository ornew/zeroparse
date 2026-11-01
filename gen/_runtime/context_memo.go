package runtime

import "reflect"

// The dependency memo of left recursion, the same as the engine's (see "Evaluations of Rules" in
// spec/runtime/parsing.md and spec/proofs/memoization.md). Parser and TParser share it: the results
// it keeps hold a node or a typed value (ctxResult).
//
// The result of a call of rule A at position p is a function of the growths of A's cycle in
// progress at p, the context: the leaders that grow there and the seeds they have reached (gamma,
// gammaAt). A leader called at p while it grows there reads its seed; a leader that does not grow
// there grows from failure, with the seeds of the context visible. Only growths at p matter: a call
// at p evaluates nothing before p, and a growth at a later position ends before the parse returns
// to p. Nor do growths of other cycles: the call reads none of their seeds.
//
// A rule that is on no left-recursion cycle never reads a seed of the context of its call, and its
// results are memoized as without left recursion. The result of a rule of a cycle called in a
// context records what it depended on:
//
//   - the seeds it read, each by its identity: a number that the growth step that produced the seed
//     gives it, the same for equal results of the same leader at p (canonicalSeed);
//   - for each lookahead at p whose evaluation read a seed, its outcome (an observation), in place
//     of those seeds;
//   - which leaders were growing at p (the present set).
//
// A result that read no seed and made no observation is stored in the memo with its present set
// (memoEntry.present) and is reused in any context whose leaders it includes. Any other result is
// kept in the table of the context (ctxTable), which exists while its first growth lasts, and is
// reused in a context with the same present set where the seeds it read have the same identities
// and the lookaheads it observed have the same outcomes there. An observation is checked against
// the outcome memoized for the lookahead in the current context; when there is none, the check
// fails and the call is evaluated again, which memoizes the lookahead.
//
// A growth that starts in a context memoizes its steps the same way, with its own seed among the
// seeds a step reads, and records runs of steps that read no seed but their own (jumps): from each
// seed of such a run, the seed at its end, with the observations, the expectations and the
// examined input of the steps from there.
//
// The examined input of a result is the Document frontier (Parser.docHigh); outside a Document
// parse it is not tracked and is 0.

// failSeed is the identity of the failure that every growth starts from.
const failSeed uint64 = 1

// maxLeaderBits is the number of leaders of a cycle that have a bit in present sets; a context with
// any other leader cannot be told from others, and its results are evaluated again. A context holds
// leaders of one cycle only, so the bits are given for each cycle.
const maxLeaderBits = 32

// ctxResult is a result that a context keeps: a seed, the result of a growth step, or the result of
// a call that read a seed. It holds a node (Parser) or a typed value (TParser).
type ctxResult struct {
	node     *Node
	val      any
	errs     []*SyntaxError
	expected []ExpID
	end      int
	far      int
	examined int // the frontier of a Document parse (Parser.docHigh)
	ok       bool
	silent   bool
}

// growRec is a growth in progress: the leader's key, its cycle and bit, its current seed and the
// seed's identity. A growth in no context holds its seed in inl (seed is nil), which no table
// shares, so that its steps allocate nothing.
type growRec struct {
	rule, min int32
	pos       int
	cycle     int32
	bit       uint32
	seed      *ctxResult
	id        uint64
	inl       ctxResult
}

// depKind is the kind of a dependency.
type depKind uint8

const (
	depSeed   depKind = iota // read the seed of gamma[idx], with identity val
	depObs                   // the lookahead site idx had the outcome val in the context of depth d
	depShadow                // a seed read by a lookahead that an observation at depth d stands for
)

// dep is a dependency recorded in a frame.
type dep struct {
	kind depKind
	idx  int32
	d    int32
	val  uint64
}

// depFrame records the dependencies of an evaluation at pos in the context gamma[:depth], whose
// growths at pos are gamma[gp:depth]. Its dependencies are deps[base:] while it is the innermost
// frame.
type depFrame struct {
	pos, gp, depth, base int
}

// sdep is a stored dependency: a seed (or shadow) by the leader's key, or an observation by site.
type sdep struct {
	kind depKind
	rule int32
	min  int32
	site int32
	val  uint64
}

// The kinds of memoized evaluations in a context table.
const (
	ctxCall = iota
	ctxStep
	ctxLook
)

type ctxHeadKey struct {
	kind    uint8
	id, min int32
	present uint32
}

// ctxSet is the dependencies of one shape that results of a head have.
type ctxSet struct {
	seeds []growKey
	obs   []int32
	vals  map[valsKey]*ctxSlot
}

type growKey struct {
	rule, min int32
}

const maxCtxDeps = 6

type valsKey struct {
	n uint8
	v [maxCtxDeps]uint64
}

// ctxSlot is a result in a context table: the result, and for a growth step the identity of its
// result and what the step itself recorded (its result is a seed shared by equal results).
type ctxSlot struct {
	e        *ctxResult
	id       uint64 // the identity of a step result; the outcome of a lookahead (obsOutcome)
	far      int
	exp      []ExpID
	examined int
	silent   bool
	env      []any
	deps     []sdep
	next     *ctxSlot
}

type jumpKey struct {
	g       growKey
	present uint32
	seed    uint64
}

// jumpRec takes a growth from a seed to the seed at the end of a run of steps that read no other
// seed, with what those steps recorded.
type jumpRec struct {
	end      *ctxResult
	id       uint64
	far      int
	exp      []ExpID
	examined int
	silent   bool
	deps     []sdep
}

type canonKey struct {
	g   growKey
	end int
}

type canonRec struct {
	e  *ctxResult
	id uint64
}

// ctxTable is the table of the growths of one cycle at a position: what the calls and growth
// steps there memoized while such a growth is in progress. It is dropped when the first of those
// growths ends.
type ctxTable struct {
	pos   int
	depth int // the index in gamma of the first of the growths
	heads map[ctxHeadKey]*[]*ctxSet
	jumps map[jumpKey][]jumpRec
	canon map[canonKey]canonRec
}

// lookRec is a lookahead in progress: its site (-1 for one that records nothing) and the Document
// frontier from before it.
type lookRec struct {
	site int32
	high int
}

// The values of an observation other than outcomes (obsOutcome).
const (
	obsNone   uint64 = 1 << 63       // no outcome is memoized
	obsUnique uint64 = 1<<63 | 1<<61 // an outcome that obsOutcome cannot encode, which nothing matches
)

// ctxCallState is the context of a memoized call of a rule of a cycle (ctxEnter).
type ctxCallState struct {
	gp      int    // the index in gamma of the first growth of the context
	present uint32 // the present set
	ctx     bool   // the call is in a context
	known   bool   // the present set tells the context from others
	framed  bool   // the call records its dependencies
}

// setLeaderBits gives the first maxLeaderBits leaders of each cycle of rules a bit each
// (Rule.leaderBit).
func setLeaderBits(rules []*Rule) {
	n := map[int32]int{}
	for _, r := range rules {
		r.leaderBit = 0
		if r.Leader && r.Cycle != 0 {
			if k := n[r.Cycle]; k < maxLeaderBits {
				r.leaderBit = 1 << k
			}
			n[r.Cycle]++
		}
	}
}

// seedAt returns the seed of gamma[i].
func (p *Parser) seedAt(i int) *ctxResult {
	if g := &p.gamma[i]; g.seed != nil {
		return g.seed
	}
	return &p.gamma[i].inl
}

// examinedNow returns the examined input of the current evaluation: the Document frontier, or 0
// outside a Document parse.
func (p *Parser) examinedNow() int {
	if p.Document != nil {
		return p.docHigh
	}
	return 0
}

// gammaAt returns the index of the first growth of the cycle at pos that the innermost growths
// there are (len(gamma) if the innermost growth is not one). Only these growths matter to a call of
// a rule of the cycle at pos: the leaders whose seeds the call can read are on the rule's cycle, and
// the growths at pos are nested in an order of their cycles (see the engine's gammaAt).
func (p *Parser) gammaAt(pos int, cycle int32) int {
	i := len(p.gamma)
	for i > 0 && p.gamma[i-1].pos == pos && p.gamma[i-1].cycle == cycle {
		i--
	}
	return i
}

// gammaFind returns the index of the growth of the leader rule at level min from gp on, or -1.
func (p *Parser) gammaFind(gp int, rule, min int32) int {
	for i := len(p.gamma) - 1; i >= gp; i-- {
		if g := &p.gamma[i]; g.rule == rule && g.min == min {
			return i
		}
	}
	return -1
}

// present returns the present set of the context gamma[gp:], and false if a leader in it has no
// bit.
func (p *Parser) present(gp int) (uint32, bool) {
	var m uint32
	for i := gp; i < len(p.gamma); i++ {
		b := p.gamma[i].bit
		if b == 0 {
			return 0, false
		}
		m |= b
	}
	return m, true
}

// ctxEnter computes the context of a memoized call of r, a rule of a cycle, at the current position
// and level min. When r is a leader that grows there, it returns the seed, which the call reads
// whatever its variable values; the seed is valid until the next growth begins.
func (p *Parser) ctxEnter(c *ctxCallState, r *Rule, min int) *ctxResult {
	gp := p.gammaAt(p.Pos, r.Cycle)
	*c = ctxCallState{gp: gp}
	if gp == len(p.gamma) {
		return nil
	}
	if r.Leader {
		if i := p.gammaFind(gp, int32(r.Id), int32(min)); i >= 0 {
			p.noteSeedRead(i)
			return p.seedAt(i)
		}
	}
	c.ctx = true
	c.present, c.known = p.present(gp)
	return nil
}

// memoAllows reports whether a result in the memo with the present set present answers a call
// in the context c: every leader that grows there grew when it was evaluated, and did not matter
// to it.
func (c *ctxCallState) memoAllows(present uint32) bool {
	return !c.ctx || c.known && c.present&^present == 0
}

// ctxLookup returns the result kept in the context c for a call of rule at level min with the
// variable values env, or nil.
func (p *Parser) ctxLookup(c *ctxCallState, rule, min int, env []any) *ctxResult {
	if !c.known {
		return nil
	}
	s := p.ctxFind(p.Pos, c.gp, ctxHeadKey{kind: ctxCall, id: int32(rule), min: int32(min), present: c.present}, env)
	if s == nil || !p.reusable(s.e.silent, len(s.e.errs)) {
		return nil
	}
	p.replayDeps(c.gp, s.deps)
	return s.e
}

// ctxPush starts recording the dependencies of a call in the context c.
func (p *Parser) ctxPush(c *ctxCallState) {
	if c.known {
		p.pushFrame(p.Pos, c.gp)
		c.framed = true
	}
}

// The ways a memoized call stores its result (ctxPop).
const (
	storeNone = iota // the call is in a context that cannot be told from others
	storeMemo        // in the memo, with the present set of the call
	storeCtx         // in the table of the context (ctxKeep)
)

// ctxPop ends the dependencies of a call in the context c and returns how the call stores its
// result: in the table of the context if it read a seed or made an observation, and otherwise in
// the memo. The dependencies stay readable for ctxKeep until ctxDone.
func (p *Parser) ctxPop(c *ctxCallState) int {
	switch {
	case c.framed:
		_, p.popped = p.popFrame()
		for _, d := range p.popped {
			if d.kind != depShadow {
				return storeCtx
			}
		}
		return storeMemo
	case c.ctx:
		return storeNone
	}
	return storeMemo
}

// ctxKeep keeps the result res of a call of rule at start and level min with the variable values
// env in the table of the context c, with the dependencies that ctxPop ended.
func (p *Parser) ctxKeep(c *ctxCallState, rule, start, min int, env []any, res *ctxResult) {
	sd, _ := p.stored(p.popped)
	p.ctxStore(start, c.gp, ctxHeadKey{kind: ctxCall, id: int32(rule), min: int32(min), present: c.present}, sd, &ctxSlot{e: res, env: env})
}

// ctxDone passes the dependencies that ctxPop ended to the enclosing frame.
func (p *Parser) ctxDone(c *ctxCallState) {
	if c.framed {
		p.mergeDeps(p.popped)
		p.popped = nil
	}
}

// pushFrame starts recording the dependencies of an evaluation at pos, in the context of the
// growths gamma[gp:] there.
func (p *Parser) pushFrame(pos, gp int) {
	p.frames = append(p.frames, depFrame{pos: pos, gp: gp, depth: len(p.gamma), base: len(p.deps)})
}

// popFrame ends the innermost frame and returns its dependencies, valid until the next push.
func (p *Parser) popFrame() (depFrame, []dep) {
	f := p.frames[len(p.frames)-1]
	p.frames = p.frames[:len(p.frames)-1]
	ds := p.deps[f.base:]
	p.deps = p.deps[:f.base]
	return f, ds
}

// addDep adds d to the innermost frame unless it holds it already.
func (p *Parser) addDep(d dep) {
	f := &p.frames[len(p.frames)-1]
	for i := f.base; i < len(p.deps); i++ {
		x := &p.deps[i]
		if x.kind == d.kind && x.idx == d.idx && (d.kind != depObs || x.d == d.d) {
			return
		}
	}
	p.Spend()
	p.deps = append(p.deps, d)
}

// mergeDeps passes the dependencies of a finished evaluation to the innermost frame: the seeds of
// its context, and the observations made in the same context. A shadow becomes a seed read when
// the frame cannot check the observation it belongs to.
func (p *Parser) mergeDeps(ds []dep) {
	if len(p.frames) == 0 {
		return
	}
	depth := int32(p.frames[len(p.frames)-1].depth)
	for _, d := range ds {
		switch d.kind {
		case depSeed:
			if d.idx < depth {
				p.addDep(d)
			}
		case depObs:
			if d.d == depth {
				p.addDep(d)
			}
		case depShadow:
			if d.d == depth {
				p.addDep(d)
			} else if d.idx < depth {
				p.addDep(dep{kind: depSeed, idx: d.idx, val: d.val})
			}
		}
	}
}

// noteSeedRead records that the evaluation read the seed of gamma[i].
func (p *Parser) noteSeedRead(i int) {
	if len(p.frames) == 0 || i >= p.frames[len(p.frames)-1].depth {
		return
	}
	p.addDep(dep{kind: depSeed, idx: int32(i), val: p.gamma[i].id})
}

// stored converts frame dependencies to stored ones and reports whether they include a seed read
// or an observation.
func (p *Parser) stored(ds []dep) ([]sdep, bool) {
	if len(ds) == 0 {
		return nil, false
	}
	out := make([]sdep, 0, len(ds))
	ctx := false
	for _, d := range ds {
		p.Spend()
		switch d.kind {
		case depSeed, depShadow:
			g := &p.gamma[d.idx]
			out = append(out, sdep{kind: d.kind, rule: g.rule, min: g.min, val: d.val})
			ctx = ctx || d.kind == depSeed
		case depObs:
			out = append(out, sdep{kind: depObs, site: d.idx, val: d.val})
			ctx = true
		}
	}
	return out, ctx
}

// replayDeps passes the stored dependencies of a reused result to the innermost frame, as if the
// evaluation that produced them had run in the current context gamma[gp:].
func (p *Parser) replayDeps(gp int, ds []sdep) {
	if len(p.frames) == 0 || len(ds) == 0 {
		return
	}
	depth := int32(len(p.gamma))
	buf := make([]dep, 0, len(ds))
	for _, d := range ds {
		switch d.kind {
		case depSeed, depShadow:
			i := p.gammaFind(gp, d.rule, d.min)
			if i < 0 {
				continue
			}
			buf = append(buf, dep{kind: d.kind, idx: int32(i), d: depth, val: d.val})
		case depObs:
			buf = append(buf, dep{kind: depObs, idx: d.site, d: depth, val: d.val})
		}
	}
	p.mergeDeps(buf)
}

// table returns the table of the growths gamma[gp:] at pos, creating it if create is set, or nil.
func (p *Parser) table(pos, gp int, create bool) *ctxTable {
	if n := len(p.ctxs); n > 0 && p.ctxs[n-1].pos == pos && p.ctxs[n-1].depth == gp {
		return p.ctxs[n-1]
	}
	if !create {
		return nil
	}
	t := &ctxTable{pos: pos, depth: gp, heads: map[ctxHeadKey]*[]*ctxSet{}}
	p.ctxs = append(p.ctxs, t)
	return t
}

// dropTable drops the table of the growth at gamma[i] if that growth is the first of its context.
func (p *Parser) dropTable(i int) {
	if n := len(p.ctxs); n > 0 && p.ctxs[n-1].pos == p.gamma[i].pos && p.ctxs[n-1].depth == i {
		p.ctxs = p.ctxs[:n-1]
	}
}

// obsOutcome encodes the outcome of a lookahead at pos whose expression matched if ok and whose
// Document frontier is examined (pos outside a Document parse): equal values are equal outcomes
// with equal frontiers. ok2 is false for an outcome it cannot encode, whose value is obsUnique.
func obsOutcome(ok bool, pos, examined int) (v uint64, ok2 bool) {
	hi := examined - pos
	if hi < 0 || hi >= 1<<30 {
		return obsUnique, false
	}
	v = uint64(hi) << 1
	if ok {
		v |= 1
	}
	return v, true
}

// obsValue returns the outcome memoized for the lookahead site at pos in the current context, or
// obsNone.
func (p *Parser) obsValue(pos, gp int, present uint32, site int32) uint64 {
	for _, s := range p.obsGuard {
		if s == site {
			return obsNone
		}
	}
	p.obsGuard = append(p.obsGuard, site)
	defer func() { p.obsGuard = p.obsGuard[:len(p.obsGuard)-1] }()
	s := p.ctxFind(pos, gp, ctxHeadKey{kind: ctxLook, id: site, present: present}, nil)
	if s == nil {
		return obsNone
	}
	return s.id
}

// ctxFind looks up a result of head h in the table of the growths gamma[gp:] at pos for the current
// context.
func (p *Parser) ctxFind(pos, gp int, h ctxHeadKey, env []any) *ctxSlot {
	t := p.table(pos, gp, false)
	if t == nil {
		return nil
	}
	p.Spend()
	sets := t.heads[h]
	if sets == nil {
		return nil
	}
next:
	for _, s := range *sets {
		p.Spend()
		var k valsKey
		for _, g := range s.seeds {
			i := p.gammaFind(gp, g.rule, g.min)
			if i < 0 {
				continue next
			}
			k.v[k.n] = p.gamma[i].id
			k.n++
		}
		for _, o := range s.obs {
			v := p.obsValue(pos, gp, h.present, o)
			if v == obsNone {
				continue next
			}
			k.v[k.n] = v
			k.n++
		}
		for x := s.vals[k]; x != nil; x = x.next {
			p.Spend()
			if env == nil && x.env == nil || p.memoKeyEqual(x.env, env) {
				return x
			}
		}
	}
	return nil
}

// ctxStore stores slot as the result of head h in the table of the growths gamma[gp:] at pos, with
// the stored dependencies ds.
func (p *Parser) ctxStore(pos, gp int, h ctxHeadKey, ds []sdep, slot *ctxSlot) {
	var seeds []growKey
	var obs []int32
	var k valsKey
	for _, d := range ds {
		if d.kind == depShadow {
			continue
		}
		if len(seeds)+len(obs) == maxCtxDeps {
			return // too many dependencies to index: the result is evaluated again
		}
		if d.kind == depSeed {
			seeds = append(seeds, growKey{d.rule, d.min})
		} else {
			obs = append(obs, d.site)
		}
	}
	// The values follow the order of the set: seeds first, then observations.
	for _, d := range ds {
		if d.kind == depSeed {
			k.v[k.n] = d.val
			k.n++
		}
	}
	for _, d := range ds {
		if d.kind == depObs {
			k.v[k.n] = d.val
			k.n++
		}
	}
	t := p.table(pos, gp, true)
	p.Spend()
	sets := t.heads[h]
	if sets == nil {
		sets = new([]*ctxSet)
		t.heads[h] = sets
	}
	var set *ctxSet
	for _, s := range *sets {
		p.Spend()
		if equalGrowKeys(s.seeds, seeds) && equalSites(s.obs, obs) {
			set = s
			break
		}
	}
	if set == nil {
		set = &ctxSet{seeds: seeds, obs: obs, vals: map[valsKey]*ctxSlot{}}
		*sets = append(*sets, set)
	}
	slot.deps = ds
	link := set.vals[k]
	for x := link; x != nil; x = x.next {
		p.Spend()
		if x.env == nil && slot.env == nil || p.memoKeyEqual(x.env, slot.env) {
			// Replace the result in place: the other kind of context (inside or outside a lookahead)
			// evaluated it again.
			nx := x.next
			*x = *slot
			x.next = nx
			return
		}
	}
	slot.next = link
	set.vals[k] = slot
}

func equalGrowKeys(a, b []growKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalSites(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// reusable reports whether a memoized result evaluated inside a lookahead (silent) or with
// recovered errors can be used by a call in the current kind of context, as for memo entries.
func (p *Parser) reusable(silent bool, errs int) bool {
	return (!silent || p.Silent > 0) && (errs == 0 || silent == (p.Silent > 0))
}

// replayCtx applies a result that a context kept, or a seed (seed), as the call that it records
// would have.
func (p *Parser) replayCtx(e *ctxResult, seed bool) {
	p.Observation.memoHit()
	p.documentTouch(e.examined)
	if p.Document != nil {
		p.Document.stats.Reused++
	}
	if !seed {
		p.MergeExpected(e.far, e.expected)
	}
	if e.ok {
		p.Pos = e.end
		p.Recovered = p.copyErrors(p.Recovered, e.errs)
	}
}

// LookBegin starts a lookahead of the given site. In a context at the current position, the
// lookahead records what it reads, so that a result that depends on it records its outcome
// instead of the seeds it read (LookEnd). Where no frame records dependencies, a lookahead records
// nothing, and the frames are the same at its end.
//
// LookBegin is for generated parsers; not a stable API.
func (p *Parser) LookBegin(site int) {
	if len(p.frames) != 0 {
		p.lookBegin(site)
	}
}

// LookEnd ends the innermost lookahead, whose expression matched if ok.
//
// LookEnd is for generated parsers; not a stable API.
func (p *Parser) LookEnd(ok bool) {
	if len(p.frames) != 0 {
		p.lookEnd(ok)
	}
}

func (p *Parser) lookBegin(site int) {
	{
		f := &p.frames[len(p.frames)-1]
		if g := len(p.gamma); f.pos == p.Pos && f.gp < g && p.gamma[g-1].pos == p.Pos && p.gamma[g-1].cycle == p.gamma[f.gp].cycle {
			// The Document frontier of the lookahead is part of its observation.
			p.looks = append(p.looks, lookRec{site: int32(site), high: p.docHigh})
			if p.Document != nil {
				p.docHigh = p.Pos
			}
			p.pushFrame(p.Pos, f.gp)
			return
		}
	}
	p.looks = append(p.looks, lookRec{site: -1})
}

func (p *Parser) lookEnd(ok bool) {
	l := p.looks[len(p.looks)-1]
	p.looks = p.looks[:len(p.looks)-1]
	site := l.site
	if site < 0 {
		return
	}
	f, ds := p.popFrame()
	examined := f.pos
	if p.Document != nil {
		examined = p.docHigh
		p.docHigh = max(l.high, p.docHigh)
	}
	read := false
	for _, d := range ds {
		if d.kind == depSeed {
			read = true
			break
		}
	}
	if !read {
		p.mergeDeps(ds)
		return
	}
	val, ok2 := obsOutcome(ok, f.pos, examined)
	present, known := p.present(f.gp)
	sd, _ := p.stored(ds)
	if ok2 && known {
		p.ctxStore(f.pos, f.gp, ctxHeadKey{kind: ctxLook, id: site, present: present}, sd, &ctxSlot{id: val})
	}
	out := make([]dep, 0, len(ds)+1)
	out = append(out, dep{kind: depObs, idx: site, d: int32(f.depth), val: val})
	for _, d := range ds {
		if d.kind == depSeed {
			out = append(out, dep{kind: depShadow, idx: d.idx, d: int32(f.depth), val: d.val})
		} else {
			out = append(out, d)
		}
	}
	p.mergeDeps(out)
}

// --- Growths ---

// growState is the state of the growth of a leader.
type growState struct {
	rule, min int32
	start     int
	rec       int
	gi        int // the index of the growth in gamma
	gp        int // the index in gamma of the first growth of the leader's cycle at the position
	// ctx reports a growth in a context, whose steps are memoized with the present set present.
	ctx     bool
	present uint32
	done    bool
	// The step being evaluated in a context: the Document frontier and the expectation record it is
	// kept apart from.
	savedHigh int
	exp       ExpMark
	// run holds the steps since the last step that read a seed other than the leader's own.
	run []runStep
}

// runStep is a step of a run of a growth: the seed it started from and what it recorded.
type runStep struct {
	seed     uint64
	far      int
	exp      []ExpID
	examined int
	silent   bool
	deps     []sdep
}

func (g *growState) growKey() growKey { return growKey{g.rule, g.min} }

// growBegin starts the growth of the leader r at level min at the current position from a failure.
// A growth in a context, where another leader of its cycle grows, memoizes its steps (growNext);
// the body is evaluated repeatedly, and the result at the point where the match stops growing is
// adopted.
func (p *Parser) growBegin(r *Rule, min int) growState {
	// One budget unit before each evaluation of the body, the seed's included.
	p.Spend()
	g := growState{rule: int32(r.Id), min: int32(min), start: p.Pos, rec: len(p.Recovered), gi: len(p.gamma)}
	g.gp = p.gammaAt(p.Pos, r.Cycle)
	// The record is written in place: a growth is pushed for every call of a leader that misses.
	if g.gi < cap(p.gamma) {
		p.gamma = p.gamma[:g.gi+1]
	} else {
		p.gamma = append(p.gamma, growRec{})
	}
	rec := &p.gamma[g.gi]
	rec.rule, rec.min, rec.pos, rec.cycle, rec.bit, rec.id = g.rule, g.min, g.start, r.Cycle, r.leaderBit, failSeed
	if g.gp < g.gi {
		// The seeds of a growth in a context can be shared by equal results (canonicalSeed).
		rec.seed = &ctxResult{end: g.start, examined: p.examinedNow()}
		rec.inl = ctxResult{}
	} else {
		// A growth in no context allocates nothing.
		rec.seed = nil
		rec.inl = ctxResult{end: g.start, examined: p.examinedNow()}
	}
	if g.gp < g.gi {
		// The present set of the steps includes the leader's own growth.
		g.present, g.ctx = p.present(g.gp)
	}
	return g
}

// growNext reports whether the body must be evaluated for the next step. In a context it first
// takes the steps it can from the jumps and the memoized steps.
func (p *Parser) growNext(g *growState) bool {
	if g.done {
		return false
	}
	if !g.ctx {
		return true
	}
	for {
		id := p.gamma[g.gi].id
		if j := p.findJump(g, id); j != nil {
			p.replayDeps(g.gp, j.deps)
			p.MergeExpected(j.far, j.exp)
			p.documentTouch(j.examined)
			p.setSeed(g, j.end, j.id)
			continue
		}
		h := ctxHeadKey{kind: ctxStep, id: g.rule, min: g.min, present: g.present}
		if s := p.ctxFind(g.start, g.gp, h, nil); s != nil && p.reusable(s.silent, len(s.e.errs)) {
			p.replayDeps(g.gp, s.deps)
			p.MergeExpected(s.far, s.exp)
			p.documentTouch(s.examined)
			if !p.advance(g, s.e, s.id, runStep{seed: id, far: s.far, exp: s.exp, examined: s.examined, silent: s.silent, deps: s.deps}) {
				return false
			}
			continue
		}
		// Evaluate the step, with its own frame, Document frontier and expectations.
		p.pushFrame(g.start, g.gp)
		if p.Document != nil {
			g.savedHigh = p.docHigh
			p.docHigh = g.start + 1
		}
		g.exp = p.Isolate(g.start)
		return true
	}
}

// growStep receives the result of a step, a node or a typed value, and adopts it as the seed if the
// match grew.
func (p *Parser) growStep(g *growState, node *Node, val any, ok bool) {
	if !g.ctx {
		best := &p.gamma[g.gi].inl
		if !ok || (best.ok && p.Pos <= best.end) {
			g.done = true
			return
		}
		p.Spend()
		*best = ctxResult{node: node, val: val, ok: true, end: p.Pos, examined: p.examinedNow(),
			errs: p.copyErrors(nil, p.Recovered[g.rec:])}
		p.gamma[g.gi].id = p.nextSeedID()
		p.Pos = g.start
		p.Recovered = p.Recovered[:g.rec]
		return
	}
	id := p.gamma[g.gi].id
	_, ds := p.popFrame()
	far, inner := p.Unisolate(g.exp)
	exp := p.Keep(inner)
	examined := p.examinedNow()
	p.MergeExpected(far, inner)
	if p.Document != nil {
		p.docHigh = max(g.savedHigh, p.docHigh)
	}
	res := &ctxResult{node: node, val: val, ok: ok, end: p.Pos, examined: examined, silent: p.Silent > 0}
	if ok {
		res.errs = p.copyErrors(nil, p.Recovered[g.rec:])
	} else {
		res.end = g.start
	}
	res, rid := p.canonicalSeed(g, res)
	sd, _ := p.stored(ds)
	h := ctxHeadKey{kind: ctxStep, id: g.rule, min: g.min, present: g.present}
	p.ctxStore(g.start, g.gp, h, sd, &ctxSlot{e: res, id: rid, far: far, exp: exp, examined: examined, silent: p.Silent > 0})
	p.mergeDeps(ds)
	p.advance(g, res, rid, runStep{seed: id, far: far, exp: exp, examined: examined, silent: p.Silent > 0, deps: sd})
}

// nextSeedID returns a new seed identity.
func (p *Parser) nextSeedID() uint64 {
	p.seedSeq++
	return p.seedSeq + failSeed
}

// advance continues the growth in a context after a step from the current seed with result res: it
// extends or ends the run of steps that read no other seed, and adopts res if the match grew. It
// reports whether the growth continues.
func (p *Parser) advance(g *growState, res *ctxResult, rid uint64, st runStep) bool {
	best := p.gamma[g.gi].seed
	grew := res.ok && (!best.ok || res.end > best.end)
	own := g.growKey()
	other := false
	var obs []sdep
	for _, d := range st.deps {
		switch {
		case d.kind == depSeed && (growKey{d.rule, d.min}) != own:
			other = true
		case d.kind == depObs || d.kind == depShadow:
			obs = append(obs, d)
		}
	}
	if grew && !other && len(res.errs) == 0 {
		st.deps = obs
		g.run = append(g.run, st)
	} else {
		p.recordJumps(g)
	}
	if !grew {
		g.done = true
		return false
	}
	p.Spend()
	p.setSeed(g, res, rid)
	p.Pos = g.start
	p.Recovered = p.Recovered[:g.rec]
	return true
}

// setSeed makes e, with identity id, the seed of the growth in a context.
func (p *Parser) setSeed(g *growState, e *ctxResult, id uint64) {
	p.gamma[g.gi].seed, p.gamma[g.gi].id = e, id
}

// growEnd ends the growth and returns its last result, which the call stores.
func (p *Parser) growEnd(g *growState) ctxResult {
	p.recordJumps(g)
	p.dropTable(g.gi)
	best := *p.seedAt(g.gi)
	clear(p.gamma[g.gi:]) // release the seeds
	p.gamma = p.gamma[:g.gi]
	p.Pos = best.end
	p.Recovered = p.copyErrors(p.Recovered[:g.rec], best.errs)
	return best
}

// recordJumps records, for each seed of the run, a jump to the current seed with what the steps
// from that seed on recorded, and starts a new run.
func (p *Parser) recordJumps(g *growState) {
	if len(g.run) == 0 {
		return
	}
	t := p.table(g.start, g.gp, true)
	if t.jumps == nil {
		t.jumps = map[jumpKey][]jumpRec{}
	}
	cur := &p.gamma[g.gi]
	acc := jumpRec{end: cur.seed, id: cur.id, far: -1, examined: g.start}
	for i := len(g.run) - 1; i >= 0; i-- {
		s := &g.run[i]
		p.Spend()
		switch {
		case s.far > acc.far:
			acc.far, acc.exp = s.far, s.exp
		case s.far == acc.far:
			acc.exp = p.unionExpected(acc.exp, s.exp)
		}
		acc.examined = max(acc.examined, s.examined)
		acc.silent = acc.silent || s.silent
		acc.deps = mergeSdeps(acc.deps, s.deps)
		k := jumpKey{g: g.growKey(), present: g.present, seed: s.seed}
		t.jumps[k] = append(t.jumps[k], acc)
	}
	g.run = g.run[:0]
}

// findJump returns a jump from the seed with identity id that holds in the current context.
func (p *Parser) findJump(g *growState, id uint64) *jumpRec {
	t := p.table(g.start, g.gp, false)
	if t == nil || t.jumps == nil {
		return nil
	}
	p.Spend()
	js := t.jumps[jumpKey{g: g.growKey(), present: g.present, seed: id}]
next:
	for i := range js {
		j := &js[i]
		p.Spend()
		if j.silent && p.Silent == 0 {
			continue
		}
		for _, d := range j.deps {
			if d.kind == depObs && p.obsValue(g.start, g.gp, g.present, d.site) != d.val {
				continue next
			}
		}
		return j
	}
	return nil
}

// mergeSdeps returns the union of two lists of stored dependencies.
func mergeSdeps(a, b []sdep) []sdep {
	if len(b) == 0 {
		return a
	}
	out := append([]sdep(nil), a...)
	for _, d := range b {
		found := false
		for _, x := range out {
			if x == d {
				found = true
				break
			}
		}
		if !found {
			out = append(out, d)
		}
	}
	return out
}

// unionExpected returns the expectations of a and b, at the same farthest position.
func (p *Parser) unionExpected(a, b []ExpID) []ExpID {
	if len(b) == 0 {
		return a
	}
	out := append([]ExpID(nil), a...)
	for _, x := range b {
		p.Spend()
		found := false
		for _, y := range out {
			if y == x {
				found = true
				break
			}
		}
		if !found {
			out = append(out, x)
		}
	}
	return out
}

// canonicalSeed returns the identity of the result e of a growth step, the same as an earlier
// equal result's of the leader at the position, whose result it then returns instead of e.
func (p *Parser) canonicalSeed(g *growState, e *ctxResult) (*ctxResult, uint64) {
	id := p.nextSeedID()
	if !e.ok || len(e.errs) > 0 {
		return e, id
	}
	t := p.table(g.start, g.gp, true)
	if t.canon == nil {
		t.canon = map[canonKey]canonRec{}
	}
	ck := canonKey{g: g.growKey(), end: e.end}
	p.Spend()
	if c, ok := t.canon[ck]; ok {
		budget := 64
		if c.e.examined == e.examined && len(c.e.errs) == 0 && p.nodesEqual(c.e.node, e.node, &budget) && p.valuesEqual(reflect.ValueOf(c.e.val), reflect.ValueOf(e.val), &budget) {
			return c.e, c.id
		}
	}
	t.canon[ck] = canonRec{e: e, id: id}
	return e, id
}

// nodesEqual reports whether two trees are equal, comparing at most *budget pairs of distinct nodes
// (false beyond: two results that are not shown equal get different identities, which costs only
// reuse).
func (p *Parser) nodesEqual(a, b *Node, budget *int) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if *budget--; *budget < 0 {
		return false
	}
	p.Spend()
	if a.RawKind != b.RawKind || a.RawStart != b.RawStart || a.RawEnd != b.RawEnd || a.RawText != b.RawText ||
		a.RawFresh != b.RawFresh || a.RawTerminal != b.RawTerminal || a.labeled != b.labeled || a.box != nil || b.box != nil ||
		len(a.RawChildren) != len(b.RawChildren) || len(a.RawFields) != len(b.RawFields) {
		return false
	}
	for i := range a.RawChildren {
		if !p.nodesEqual(a.RawChildren[i], b.RawChildren[i], budget) {
			return false
		}
	}
	for i := range a.RawFields {
		fa, fb := &a.RawFields[i], &b.RawFields[i]
		if fa.Name != fb.Name {
			return false
		}
		na, aok := fa.Value.(*Node)
		nb, bok := fb.Value.(*Node)
		switch {
		case aok && bok:
			if !p.nodesEqual(na, nb, budget) {
				return false
			}
		case aok || bok:
			return false
		default:
			switch fa.Value.(type) {
			case nil, int, string, bool:
			default:
				return false
			}
			if fa.Value != fb.Value {
				return false
			}
		}
	}
	return true
}

// valuesEqual reports whether two typed values are equal, comparing at most *budget values of
// composite kinds (false beyond, and for kinds it does not compare: functions, channels, maps and
// unsafe pointers).
func (p *Parser) valuesEqual(a, b reflect.Value, budget *int) bool {
	if !a.IsValid() || !b.IsValid() {
		return a.IsValid() == b.IsValid()
	}
	if a.Type() != b.Type() {
		return false
	}
	switch a.Kind() {
	case reflect.Bool:
		return a.Bool() == b.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return a.Int() == b.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return a.Uint() == b.Uint()
	case reflect.Float32, reflect.Float64:
		return a.Float() == b.Float()
	case reflect.String:
		return a.String() == b.String()
	}
	if *budget--; *budget < 0 {
		return false
	}
	p.Spend()
	switch a.Kind() {
	case reflect.Pointer:
		if a.Pointer() == b.Pointer() {
			return true
		}
		if a.IsNil() || b.IsNil() {
			return false
		}
		return p.valuesEqual(a.Elem(), b.Elem(), budget)
	case reflect.Interface:
		if a.IsNil() || b.IsNil() {
			return a.IsNil() == b.IsNil()
		}
		return p.valuesEqual(a.Elem(), b.Elem(), budget)
	case reflect.Slice:
		if a.IsNil() != b.IsNil() || a.Len() != b.Len() {
			return false
		}
		if a.Len() > 0 && a.Pointer() == b.Pointer() {
			return true
		}
		fallthrough
	case reflect.Array:
		for i := range a.Len() {
			if !p.valuesEqual(a.Index(i), b.Index(i), budget) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for i := range a.NumField() {
			if !p.valuesEqual(a.Field(i), b.Field(i), budget) {
				return false
			}
		}
		return true
	}
	return false
}
