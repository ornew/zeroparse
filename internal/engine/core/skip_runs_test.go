package core

import "testing"

// walk runs a walk of site 1 from pos that visits n positions one apart and fails at the next,
// and stops before its end if abort.
func walkSkip(p *Parser, pos, n int, abort bool) {
	p.Pos = pos
	p.SkipBegin(1)
	for k := 0; ; k++ {
		if k >= 1 && p.SkipHit() {
			break
		}
		p.SkipVisit()
		if abort && k == n/2 {
			return // the budget stops the parse in the iteration
		}
		if k == n {
			break // the iteration fails at its start
		}
		p.Pos++
	}
	p.SkipEnd()
}

// TestSkipRunsAbortStoresNothing checks that records are stored only when a walk ends: a recording
// walk stopped in an iteration leaves its positions as an earlier walk left them, and a plain walk
// only marks those of its matching iterations.
func TestSkipRunsAbortStoresNothing(t *testing.T) {
	p := &Parser{}
	walkSkip(p, 0, 0, false) // plain, fails at once: touches no slot
	if len(p.skipTables) != 0 {
		t.Fatalf("a walk that fails at once makes slots: %v", p.skipTables)
	}
	plain := func(pos int) uint8 {
		if pos < 10 {
			return skipVisited
		}
		return skipFresh // the iteration at 10 failed
	}
	walkSkip(p, 0, 10, false) // plain: marks 0..9
	s := p.skipTables[2]
	for pos := 0; pos <= 10; pos++ {
		if got := s.get(pos).state; got != plain(pos) {
			t.Fatalf("after a plain walk, position %d has state %d", pos, got)
		}
	}
	walkSkip(p, 2, 8, true) // recording from 2, stopped at 6
	for pos := 0; pos <= 10; pos++ {
		if got := s.get(pos).state; got != plain(pos) {
			t.Fatalf("after a stopped recording walk, position %d has state %d", pos, got)
		}
	}
	p.skip = skipWalk{} // the stopped parse is gone; a later walk starts clean
	walkSkip(p, 3, 7, false)
	for pos := 0; pos <= 10; pos++ {
		want := uint8(skipVisited)
		if pos >= 3 {
			want = skipRecorded
		}
		if got := s.get(pos).state; got != want {
			t.Fatalf("after a recording walk from 3, position %d has state %d, want %d", pos, got, want)
		}
		if want == skipRecorded && s.get(pos).end != 10 {
			t.Fatalf("position %d records end %d, want 10", pos, s.get(pos).end)
		}
	}
}

// TestSkipRunsAbortControl checks that the test above detects records stored when their iteration
// ends: a stopped walk then leaves records behind.
func TestSkipRunsAbortControl(t *testing.T) {
	SkipRecordMutation = SkipEarlyStore
	defer func() { SkipRecordMutation = 0 }()
	p := &Parser{}
	walkSkip(p, 0, 10, false)
	walkSkip(p, 2, 8, true)
	s := p.skipTables[2]
	recorded := 0
	for pos := 0; pos <= 10; pos++ {
		if s.get(pos).state == skipRecorded {
			recorded++
		}
	}
	if recorded == 0 {
		t.Fatal("a stopped walk that stores records early leaves none")
	}
}
