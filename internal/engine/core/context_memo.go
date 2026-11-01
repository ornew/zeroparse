package core

import "zeroparse.dev/zp/internal/value"

// The dependency memo of left recursion (see "Evaluations of Rules" in spec/runtime/parsing.md
// and spec/proofs/memoization.md).
//
// The result of a call of rule A at position p is a function of the growths of A's cycle in
// progress at p, the context: the leaders that grow there and the seeds they have reached (Gamma,
// gammaAt). A leader called at p while it grows there reads its seed; a leader that does not grow
// there grows from failure, with the seeds of the context visible. Only growths at p matter: a call
// at p evaluates nothing before p, and a growth at a later position ends before the parse returns
// to p. Nor do growths of other cycles: the call reads none of their seeds.
//
// A rule that is on no left-recursion cycle never reads a seed of the context of its call: a
// leader that grows at p reaches the call there through left calls, so a call that read the
// leader's seed would put the rule on the leader's cycle. Its results are memoized as without
// left recursion. The result of a rule of a cycle called in a context records what it depended on:
//
//   - the seeds it read, each by its identity: a number that the growth step that produced the seed
//     gives it, the same for equal results of the same leader at p (canonicalSeed);
//   - for each lookahead at p whose evaluation read a seed, its outcome (an observation), in place
//     of those seeds;
//   - which leaders were growing at p (the present set): a leader that was not growing may have
//     grown from failure within the call, and one that was growing and was not read did not matter.
//
// A result that read no seed and made no observation is stored in the memo table with its present
// set (MemoEntry.Present) and is reused in any context whose leaders it includes. Any other result
// is stored in the table of the context (ctxTable), which exists while its first growth lasts,
// and is reused in a context with the same present set where the seeds it read have the same
// identities and the lookaheads it observed have the same outcomes there. An observation is
// checked against the outcome memoized for the lookahead in the current context; when there is
// none, the check fails and the call is evaluated again, which memoizes the lookahead.
//
// A growth that starts in a context memoizes its steps the same way, with its own seed among the
// seeds a step reads, and records runs of steps that read no seed but their own (jumps): from each
// seed of such a run, the seed at its end, with the observations, the expectations and the
// examined input of the steps from there. Steps and jumps make a growth that is evaluated again in
// another context take only the steps that read the seeds that changed.

// failSeed is the identity of the failure that every growth starts from.
const failSeed uint64 = 1

// ContextMemoMutation breaks one check of the dependency memo, for the negative controls of the
// tests that compare it with parses that evaluate every call in a context again
// (Parser.ContextMemoOff).
var ContextMemoMutation = 0

// The checks that ContextMemoMutation breaks.
const (
	CtxIgnoreSeeds     = 1 + iota // a reuse does not compare the identities of the seeds read
	CtxIgnoreObs                  // a reuse does not check observations
	CtxIgnorePresent              // a result in the memo table is reused whatever its present set
	CtxNoCanonical                // equal step results get the same identity without comparing them
	CtxKeepSeedResults            // a Document keeps the results that read a seed for later parses
)

// growRec is a growth in progress: the leader's key, its current seed and the seed's identity.
type growRec struct {
	key   MemoKey
	seed  *MemoEntry
	id    uint64
	bit   uint64 // Rule.LeaderBit
	cycle int    // Rule.Cycle
}

// depKind is the kind of a dependency.
type depKind uint8

const (
	depSeed   depKind = iota // read the seed of Gamma[idx], with identity val
	depObs                   // the lookahead site idx had the outcome val in the context of depth d
	depShadow                // a seed read by a lookahead that an observation at depth d stands for
)

// dep is a dependency recorded in a frame. Gamma[:d] is the context of an observation; a frame
// whose context is shorter cannot check it and takes the seeds that its shadows name instead.
type dep struct {
	kind depKind
	idx  int32
	d    int32
	val  uint64
}

// depFrame records the dependencies of an evaluation at pos in the context Gamma[:depth], whose
// growths at pos are Gamma[gp:depth]. Its dependencies are deps[base:] while it is the innermost
// frame.
type depFrame struct {
	pos, gp, depth, base int
}

// sdep is a stored dependency: a seed (or shadow) by the leader's key, or an observation by site.
type sdep struct {
	kind depKind
	rule int32
	min  int32
	ms   bool
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
	ms      bool
	present uint64
}

// ctxSet is the dependencies of one shape that results of a head have: the seeds they read and the
// lookaheads they observed. vals finds a result by the values of those dependencies.
type ctxSet struct {
	seeds []growKey
	obs   []int32
	vals  map[valsKey]*ctxSlot
}

type growKey struct {
	rule, min int32
	ms        bool
}

const maxCtxDeps = 6

type valsKey struct {
	n uint8
	v [maxCtxDeps]uint64
}

// ctxSlot is a result in a context table: the entry, and for a growth step the identity of its
// result and what the step itself recorded (its result entry is a seed shared by equal results).
type ctxSlot struct {
	e              *MemoEntry
	id             uint64 // the identity of a step result; the outcome of a lookahead (obsOutcome)
	far            int
	exp            []ExpID
	examined, from int
	silent         bool
	env            []any
	deps           []sdep
	next           *ctxSlot
}

type jumpKey struct {
	g       growKey
	present uint64
	seed    uint64
}

// jumpRec takes a growth from a seed to the seed at the end of a run of steps that read no other
// seed, with what those steps recorded.
type jumpRec struct {
	end            *MemoEntry
	id             uint64
	far            int
	exp            []ExpID
	examined, from int
	silent         bool
	deps           []sdep
}

type canonKey struct {
	g   growKey
	end int
	ok  bool
}

type canonRec struct {
	e  *MemoEntry
	id uint64
}

// ctxTable is the context table of the growths of one cycle at a position: what the calls and
// growth steps there memoized while such a growth is in progress. It is dropped when the first of
// those growths ends.
type ctxTable struct {
	pos   int
	depth int // the index in Gamma of the first of the growths
	heads map[ctxHeadKey]*[]*ctxSet
	jumps map[jumpKey][]jumpRec
	canon map[canonKey]canonRec
}

// keptSeedResult is a result that read a seed, kept for CtxKeepSeedResults.
type keptSeedResult struct {
	key MemoKey
	e   *MemoEntry
}

// KeepSeedResults stores in t, under CtxKeepSeedResults, the results of the parse p that read a
// seed, as if they had not: the negative control of the Document checks, which must detect a later
// parse that reuses them.
func KeepSeedResults(t *MemoTable, p *Parser) {
	if ContextMemoMutation != CtxKeepSeedResults {
		return
	}
	for _, k := range p.keptSeedResults {
		k.e.Present = ^uint64(0)
		t.Put(k.key, k.e)
	}
}

// strictCtx reports that results in a context are not memoized in this parse.
func (p *Parser) strictCtx() bool {
	return p.ContextMemoOff || p.Missing != nil
}

// gammaAt returns the index of the first growth of the cycle at pos that the innermost growths
// there are (len(Gamma) if the innermost growth is not one).
//
// Only these growths matter to a call of a rule of the cycle at pos. A call at pos inside the growth
// of a leader at pos is reached from the leader through calls at pos, so the leader reaches the
// rule and a seed the call reads is of a leader that the rule reaches: the leaders whose seeds the
// call can read are on the rule's cycle. Growths at pos are nested in an order of their cycles,
// since a growth inside the growth of a leader of another cycle at pos cannot reach that leader:
// the growths of the rule's cycle at pos are consecutive, and if the innermost growth at pos is of
// another cycle, there is none.
func (p *Parser) gammaAt(pos, cycle int) int {
	i := len(p.Gamma)
	for i > 0 && p.Gamma[i-1].key.Pos == pos && p.Gamma[i-1].cycle == cycle {
		i--
	}
	return i
}

// gammaFind returns the index of the growth of the leader of key at its position, from gp on, or
// -1. A leader that grows is read whatever the values of the variables (Env is not compared).
func (p *Parser) gammaFind(gp int, rule, min int, ms bool) int {
	for i := len(p.Gamma) - 1; i >= gp; i-- {
		k := &p.Gamma[i].key
		if k.Rule == rule && k.Min == min && k.MissingSilent == ms {
			return i
		}
	}
	return -1
}

// present returns the present set of the context Gamma[gp:], and false if a leader in it has no
// bit, so that the set cannot be told from others.
func (p *Parser) present(gp int) (uint64, bool) {
	var m uint64
	for i := gp; i < len(p.Gamma); i++ {
		b := p.Gamma[i].bit
		if b == 0 {
			return 0, false
		}
		m |= b
	}
	return m, true
}

// pushFrame starts recording the dependencies of an evaluation at pos, in the context of the
// growths Gamma[gp:] there.
func (p *Parser) pushFrame(pos, gp int) {
	p.frames = append(p.frames, depFrame{pos: pos, gp: gp, depth: len(p.Gamma), base: len(p.deps)})
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

// noteSeedRead records that the evaluation read the seed of Gamma[i].
func (p *Parser) noteSeedRead(i int) {
	p.SeedReads++
	if len(p.frames) == 0 || i >= p.frames[len(p.frames)-1].depth {
		return
	}
	p.addDep(dep{kind: depSeed, idx: int32(i), val: p.Gamma[i].id})
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
			k := p.Gamma[d.idx].key
			out = append(out, sdep{kind: d.kind, rule: int32(k.Rule), min: int32(k.Min), ms: k.MissingSilent, val: d.val})
			ctx = ctx || d.kind == depSeed
		case depObs:
			out = append(out, sdep{kind: depObs, site: d.idx, val: d.val})
			ctx = true
		}
	}
	return out, ctx
}

// replayDeps passes the stored dependencies of a reused result to the innermost frame, as if the
// evaluation that produced them had run in the current context Gamma[gp:].
func (p *Parser) replayDeps(gp int, ds []sdep) {
	if len(p.frames) == 0 || len(ds) == 0 {
		return
	}
	depth := int32(len(p.Gamma))
	buf := make([]dep, 0, len(ds))
	for _, d := range ds {
		switch d.kind {
		case depSeed, depShadow:
			i := p.gammaFind(gp, int(d.rule), int(d.min), d.ms)
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

// table returns the context table of the growths Gamma[gp:] at pos, creating it if create is set,
// or nil.
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

// dropTable drops the context table of the growth at Gamma[i] if that growth is the first at its
// position.
func (p *Parser) dropTable(i int) {
	if n := len(p.ctxs); n > 0 && p.ctxs[n-1].pos == p.Gamma[i].key.Pos && p.ctxs[n-1].depth == i {
		p.ctxs = p.ctxs[:n-1]
	}
}

// lookRec is a lookahead in progress: its site (-1 for one that records nothing) and the examined
// range from before it.
type lookRec struct {
	site   int32
	hw, lw int
}

// The values of an observation other than outcomes (obsOutcome).
const (
	obsNone    uint64 = 1 << 63       // no outcome is memoized
	obsIgnored uint64 = 1<<63 | 1<<62 // every outcome under CtxIgnoreObs
	obsUnique  uint64 = 1<<63 | 1<<61 // an outcome that obsOutcome cannot encode, which nothing matches
)

// obsOutcome encodes the outcome of a lookahead at pos, whose expression matched if ok and examined
// [from, examined): equal values are equal outcomes with equal examined ranges. ok2 is false for an
// outcome it cannot encode, whose value is obsUnique.
func obsOutcome(ok bool, pos, examined, from int) (v uint64, ok2 bool) {
	hi, lo := examined-pos, pos-from
	if hi < 0 || lo < 0 || hi >= 1<<30 || lo >= 1<<30 {
		return obsUnique, false
	}
	v = uint64(hi)<<1 | uint64(lo)<<31
	if ok {
		v |= 1
	}
	return v, true
}

// obsValue returns the outcome memoized for the lookahead site at pos in the current context
// (obsOutcome), or obsNone.
func (p *Parser) obsValue(pos, gp int, present uint64, site int32) uint64 {
	if ContextMemoMutation == CtxIgnoreObs {
		return obsIgnored
	}
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

// ctxFind looks up a result of head h in the context table of pos for the current context.
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
			i := p.gammaFind(gp, int(g.rule), int(g.min), g.ms)
			if i < 0 {
				continue next
			}
			v := p.Gamma[i].id
			if ContextMemoMutation == CtxIgnoreSeeds {
				v = 0
			}
			k.v[k.n] = v
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

// ctxStore stores slot as the result of head h in the context of the growths Gamma[gp:] at pos,
// with the stored dependencies ds.
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
			seeds = append(seeds, growKey{d.rule, d.min, d.ms})
		} else {
			obs = append(obs, d.site)
		}
	}
	// The values follow the order of the set: seeds first, then observations.
	for _, d := range ds {
		if d.kind == depSeed {
			v := d.val
			if ContextMemoMutation == CtxIgnoreSeeds {
				v = 0
			}
			k.v[k.n] = v
			k.n++
		}
	}
	for _, d := range ds {
		if d.kind == depObs {
			v := d.val
			if ContextMemoMutation == CtxIgnoreObs {
				v = obsIgnored
			}
			k.v[k.n] = v
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

// LookBegin starts a lookahead of the given site. In a context at the current position, the
// lookahead records what it reads, so that a result that depends on it records its outcome
// instead of the seeds it read (LookEnd).
func (p *Parser) LookBegin(site int) {
	if n := len(p.frames); n > 0 {
		f := &p.frames[n-1]
		if g := len(p.Gamma); f.pos == p.Pos && f.gp < g && p.Gamma[g-1].key.Pos == p.Pos && p.Gamma[g-1].cycle == p.Gamma[f.gp].cycle {
			// The examined range of the lookahead is part of its observation.
			p.looks = append(p.looks, lookRec{site: int32(site), hw: p.Hw, lw: p.lw})
			p.Hw, p.lw = p.Pos, p.Pos
			p.pushFrame(p.Pos, f.gp)
			return
		}
	}
	p.looks = append(p.looks, lookRec{site: -1})
}

// LookEnd ends the innermost lookahead, whose expression matched if ok.
func (p *Parser) LookEnd(ok bool) {
	l := p.looks[len(p.looks)-1]
	p.looks = p.looks[:len(p.looks)-1]
	site := l.site
	if site < 0 {
		return
	}
	examined, from := p.Hw, p.lw
	p.Hw, p.lw = max(l.hw, p.Hw), Min2(l.lw, p.lw)
	f, ds := p.popFrame()
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
	if p.ContextMemoUnexamined {
		examined, from = f.pos, f.pos
	}
	val, ok2 := obsOutcome(ok, f.pos, examined, from)
	gp := f.gp
	present, known := p.present(gp)
	ok2 = ok2 && known
	sd, _ := p.stored(ds)
	if ok2 {
		p.ctxStore(f.pos, gp, ctxHeadKey{kind: ctxLook, id: site, present: present}, sd, &ctxSlot{id: val})
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

// canonicalSeed returns the identity of the result e of a growth step of the leader of g, the
// same as an earlier equal result's, whose entry it then returns instead of e.
func (p *Parser) canonicalSeed(pos, gp int, g growKey, e *MemoEntry) (*MemoEntry, uint64) {
	p.seedSeq++
	id := p.seedSeq + failSeed
	if !e.Ok || len(e.Errs) > 0 {
		return e, id
	}
	t := p.table(pos, gp, true)
	if t.canon == nil {
		t.canon = map[canonKey]canonRec{}
	}
	ck := canonKey{g: g, end: e.End, ok: e.Ok}
	p.Spend()
	if c, ok := t.canon[ck]; ok {
		if ContextMemoMutation == CtxNoCanonical {
			return c.e, c.id
		}
		budget := 64
		if (p.ContextMemoUnexamined || c.e.Examined == e.Examined && c.e.From == e.From) && len(c.e.Errs) == 0 && nodesEqual(p, c.e.Node, e.Node, &budget) {
			return c.e, c.id
		}
	}
	t.canon[ck] = canonRec{e: e, id: id}
	return e, id
}

// nodesEqual reports whether two trees are equal, comparing at most *budget pairs of distinct nodes
// (false beyond: two results that are not shown equal get different identities, which costs only
// reuse).
func nodesEqual(p *Parser, a, b *value.Node, budget *int) bool {
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
	da, db := value.D(a), value.D(b)
	if da.Kind != db.Kind || da.Start != db.Start || da.End != db.End || da.Terminal != db.Terminal ||
		da.Labeled != db.Labeled || da.Gen != db.Gen || da.Box != nil || db.Box != nil ||
		len(da.Children) != len(db.Children) || len(da.Fields) != len(db.Fields) || da.Text != db.Text {
		return false
	}
	for i := range da.Children {
		if !nodesEqual(p, da.Children[i], db.Children[i], budget) {
			return false
		}
	}
	for i := range da.Fields {
		fa, fb := &da.Fields[i], &db.Fields[i]
		if fa.Name != fb.Name {
			return false
		}
		na, aok := fa.Value.(*value.Node)
		nb, bok := fb.Value.(*value.Node)
		switch {
		case aok && bok:
			if !nodesEqual(p, na, nb, budget) {
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
