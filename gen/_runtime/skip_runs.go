package runtime

// Skip runs: the records of the walks of skip-run sites, a mirror of the engine's
// (internal/engine/core/skip_runs.go; see "Skip Runs" in spec/runtime/parsing.md and the proofs in
// spec/proofs/skip-runs.md).
//
// A walk of a site repetition visits positions r0 < r1 < ... with one iteration each. The slot of
// a position in the table of the site (and lookahead state) is fresh, visited or recorded. A walk
// starts plain: it marks each position it leaves after a matching iteration as visited and
// records nothing, so a walk that fails at once touches no slot. At its first
// iteration at a position that is not fresh, which an earlier walk of the site visited, it starts
// recording: from there on, for every position it visits, it records what the rest of the walk
// from there did (where it ended, its farthest failure and the expectations there, and, in a
// Document parse, how far it examined the input). A recording walk that reaches a recorded
// position (once it has matched its minimum) takes the rest from the record: it moves to the end,
// records the expectations again and extends the examined frontier, as a memo hit does. The record
// of a position combines the iteration there with the record of the next position, so a walk
// fills the records of its recording visits backward when it ends: at a failed or empty
// iteration, whose record ends at its own start, or at a recorded position.
//
// The records of a parse last for that parse; a stream drops those before what it committed.

// The states of a slot.
const (
	skipFresh    = iota // no walk visited the position
	skipVisited         // a walk visited it without recording
	skipRecorded        // the record holds the rest of a walk from it
)

// skipRecord is the state of a position and, once recorded, what the rest of a walk from it did.
type skipRecord struct {
	end, far int
	exp      []ExpID
	examined int
	state    uint8
}

// skipSlots holds the slots of a site: a bit for each position from base on, set when the slot is
// not fresh, and the records of the recorded slots by position. The bits take an eighth of a byte
// per position between the first and the last position walked, and a mark costs no allocation once
// they cover the position; the records take memory only for the positions recording walks visited.
// The slots of the positions before floor are dropped: a stream parses there no more.
type skipSlots struct {
	base  int
	marks []uint64
	recs  map[int]skipRecord
	floor int
	// kept is the number of records after dropped ones were last removed.
	kept int
}

// marked reports whether the slot of pos is not fresh. s may be nil, with every slot fresh.
func (s *skipSlots) marked(pos int) bool {
	if s == nil || pos < s.floor {
		return false
	}
	i := pos - s.base
	return i >= 0 && i>>6 < len(s.marks) && s.marks[i>>6]&(1<<(i&63)) != 0
}

// get returns the slot of pos; a fresh slot is the zero record.
func (s *skipSlots) get(pos int) skipRecord {
	if !s.marked(pos) {
		return skipRecord{}
	}
	if r, ok := s.recs[pos]; ok {
		return r
	}
	return skipRecord{state: skipVisited}
}

// put sets the slot of pos to r, which is visited or recorded. Room made for bits before base is as
// large as the bits, so walks that start farther and farther back copy them a logarithmic number of
// times.
func (s *skipSlots) put(pos int, r skipRecord) {
	switch {
	case len(s.marks) == 0:
		s.base = pos &^ 63
	case pos < s.base:
		base := max(0, min(pos, s.base-64*len(s.marks))) &^ 63
		marks := make([]uint64, (s.base-base)>>6+len(s.marks))
		copy(marks[(s.base-base)>>6:], s.marks)
		s.marks, s.base = marks, base
	}
	i := pos - s.base
	if i>>6 >= len(s.marks) {
		s.marks = append(s.marks, make([]uint64, i>>6+1-len(s.marks))...)
	}
	s.marks[i>>6] |= 1 << (i & 63)
	if r.state == skipRecorded {
		if s.recs == nil {
			s.recs = make(map[int]skipRecord)
		}
		s.recs[pos] = r
	}
}

// drop drops the slots of the positions before pos: the words of bits wholly before it at once, and
// the records once they can make up half of them, so its cost is proportional to the records stored.
func (s *skipSlots) drop(pos int) {
	if pos <= s.floor {
		return
	}
	s.floor = pos
	if k := min((pos-s.base)>>6, len(s.marks)); k > 0 {
		s.marks, s.base = s.marks[k:], s.base+64*k
	}
	if len(s.recs) < 2*s.kept+64 {
		return
	}
	for k := range s.recs {
		if k < pos {
			delete(s.recs, k)
		}
	}
	s.kept = len(s.recs)
}

// skipVisit is a recording visit of the walk in progress.
type skipVisit struct {
	pos, far int
	exp      []ExpID
	examined int
}

// skipWalk is the walk in progress. Walks do not nest: a site's element runs no #recover, so no
// other skip expression.
type skipWalk struct {
	// recording reports that the walk records its visits (it met a position that is not fresh).
	recording bool
	// table is the index of the slots in skipTables, and slots the slots, nil until a walk of the
	// site first writes one.
	table int
	slots *skipSlots
	// prev is the position of the last iteration of a plain walk, if prevOK; the walk marks it
	// when the iteration has matched.
	prev   int
	prevOK bool
	visits []skipVisit
	// open reports that the last visit's iteration is running: its expectations are isolated
	// from mk, and the examined frontier from before it is savedHigh.
	open      bool
	mk        ExpMark
	savedHigh int
	hit       skipRecord
	hitOK     bool
}

// SkipBegin starts a walk of the site at the current position.
func (p *Parser) SkipBegin(site int) {
	w := &p.skip
	i := 2 * site
	if p.Silent > 0 {
		i++
	}
	w.table, w.slots = i, nil
	if i < len(p.skipTables) {
		w.slots = p.skipTables[i]
	}
	w.visits, w.recording, w.prevOK, w.open, w.hitOK = w.visits[:0], false, false, false, false
}

// skipTable returns the slots of the walk, making them.
func (p *Parser) skipTable() *skipSlots {
	w := &p.skip
	if w.slots == nil {
		if w.table >= len(p.skipTables) {
			p.skipTables = append(p.skipTables, make([]*skipSlots, w.table+1-len(p.skipTables))...)
		}
		w.slots = &skipSlots{}
		p.skipTables[w.table] = w.slots
	}
	return w.slots
}

// skipMarkPrev marks the position of the last iteration of a plain walk, which matched.
func (p *Parser) skipMarkPrev() {
	w := &p.skip
	if w.prevOK {
		p.skipTable().put(w.prev, skipRecord{state: skipVisited})
		w.prevOK = false
	}
}

// skipRecording reports whether the walk records from the current position on, which it starts
// to at the first position that is not fresh.
func (p *Parser) skipRecording() bool {
	w := &p.skip
	if !w.recording && w.slots.marked(p.Pos) {
		p.skipMarkPrev()
		w.recording = true
	}
	return w.recording
}

// SkipHit is called before an iteration after the minimum. It ends the iteration before and, in a
// recording walk, reports whether the current position has a record, which it then reuses. The
// walk then stops.
func (p *Parser) SkipHit() bool {
	if !p.skipRecording() {
		return false
	}
	w := &p.skip
	p.skipClose()
	p.Spend()
	r := w.slots.get(p.Pos)
	if r.state != skipRecorded {
		return false
	}
	p.Spend()
	p.documentTouch(r.examined)
	p.MergeExpected(r.far, r.exp)
	p.Pos = r.end
	w.hit, w.hitOK = r, true
	return true
}

// SkipVisit begins an iteration at the current position: in a plain walk it marks the position of
// the iteration before, which matched, visited; in a recording walk it ends the iteration before
// and isolates this one.
func (p *Parser) SkipVisit() {
	w := &p.skip
	if !p.skipRecording() {
		p.skipMarkPrev()
		w.prev, w.prevOK = p.Pos, true
		return
	}
	p.skipClose()
	w.visits = append(w.visits, skipVisit{pos: p.Pos})
	w.open, w.savedHigh = true, p.docHigh
	if p.Document != nil {
		p.docHigh = p.Pos
	}
	w.mk = p.Isolate(p.Pos)
}

// skipClose ends the running iteration, if any: it keeps its expectations and examined frontier
// and records them as usual.
func (p *Parser) skipClose() {
	w := &p.skip
	if !w.open {
		return
	}
	w.open = false
	far, inner := p.Unisolate(w.mk)
	v := &w.visits[len(w.visits)-1]
	v.far, v.exp = far, p.Keep(inner)
	p.MergeExpected(far, inner)
	v.examined = p.docHigh
	p.docHigh = max(w.savedHigh, p.docHigh)
}

// SkipEnd ends the walk and records the rest of it from each position it visited recording.
func (p *Parser) SkipEnd() {
	w := &p.skip
	p.skipClose()
	vs := w.visits
	var next skipRecord
	switch {
	case w.hitOK:
		next = w.hit
	case len(vs) > 0:
		// The last iteration failed or matched nothing: the walk ended at its start.
		v := vs[len(vs)-1]
		vs = vs[:len(vs)-1]
		next = skipRecord{end: v.pos, far: v.far, exp: v.exp, examined: v.examined, state: skipRecorded}
		p.skipPut(v.pos, next)
	}
	for i := len(vs) - 1; i >= 0; i-- {
		v := vs[i]
		r := skipRecord{end: next.end, far: next.far, exp: next.exp, examined: max(v.examined, next.examined), state: skipRecorded}
		switch {
		case v.far > next.far:
			r.far, r.exp = v.far, v.exp
		case v.far == next.far:
			r.exp = mergeSkipExpected(v.exp, next.exp)
		}
		p.skipPut(v.pos, r)
		next = r
	}
	w.visits = vs[:0]
}

// mergeSkipExpected returns the expectations a, then those of b not in a.
func mergeSkipExpected(a, b []ExpID) []ExpID {
	if len(a) == 0 {
		return b
	}
	out := a
	for _, x := range b {
		found := false
		for _, y := range a {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			if len(out) == len(a) {
				out = append(make([]ExpID, 0, len(a)+len(b)), a...)
			}
			out = append(out, x)
		}
	}
	return out
}

// skipPut stores the record r of the position pos in the slots of the walk.
func (p *Parser) skipPut(pos int, r skipRecord) {
	p.Spend()
	p.skipTable().put(pos, r)
}

// pruneSkipRecords drops the records of the positions before pos, where a stream that committed
// at pos never parses again.
func (p *Parser) pruneSkipRecords(pos int) {
	for _, s := range p.skipTables {
		if s != nil {
			s.drop(pos)
		}
	}
}
