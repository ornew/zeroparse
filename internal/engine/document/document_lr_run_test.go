package document_test

import (
	"fmt"
	"math/rand"
	"testing"

	. "zeroparse.dev/zp/internal/engine"
	"zeroparse.dev/zp/internal/engine/core"
)

// Grammars where a repetition runs at a position first outside a growth there, where its first
// element grows a leader of the cycle and reads that leader's seed, and later inside a growth of
// another leader of the cycle at the same position, where the element reads the seed of the open
// growth instead: a run recorded the first time must not be resumed the second.
var runGrowthGrammars = []string{
	`rule main = r "!" / m "?"
rule m = m "m" / l
rule l = l "l" / m "k" / r
rule r = x*
rule x = l "c" / "c"`,
	`rule main = r "!" / m "?"
rule m = m "m" / l "k" / r
rule l = l "l" / m / "c"
rule r = x*
rule x = l "c" / "c"`,
}

// runGrowthMismatches parses random texts and edits of runGrowthGrammars as Documents and returns
// the first Document parses that differ from a fresh parse, at most limit of them.
func runGrowthMismatches(t *testing.T, limit int) []string {
	t.Helper()
	defer func(n int) { core.MinRecorded = n }(core.MinRecorded)
	core.MinRecorded = 1
	const alphabet = "cmlk!?"
	var bad []string
	for gi, src := range runGrowthGrammars {
		prog := mustCompile(t, src)
		for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
			opts := ParseOptions{Backend: backend}
			rng := rand.New(rand.NewSource(int64(gi)))
			for range 400 {
				text := make([]byte, 1+rng.Intn(8))
				for i := range text {
					text[i] = alphabet[rng.Intn(len(alphabet))]
				}
				d, err := prog.NewDocumentWith("main", string(text), opts)
				if err != nil {
					t.Fatal(err)
				}
				for step := range 4 {
					got, gotErr := d.Parse()
					want, wantErr := prog.ParseWith("main", d.Text(), opts)
					if resultJSON(got, gotErr) != resultJSON(want, wantErr) {
						bad = append(bad, fmt.Sprintf("grammar %d %s step %d text %q: Document %s; fresh %s", gi, backend, step, d.Text(), resultJSON(got, gotErr), resultJSON(want, wantErr)))
						if len(bad) == limit {
							return bad
						}
						break
					}
					n := len(d.Text())
					start := rng.Intn(n + 1)
					end := start + rng.Intn(min(2, n-start)+1)
					if err := d.Edit(start, end, string(alphabet[rng.Intn(len(alphabet))])); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	return bad
}

// TestDocumentRunReadsGrowthWithin checks that a run whose element read the seed of a growth that
// started and ended within the run is not recorded, so that Document parses equal fresh ones.
func TestDocumentRunReadsGrowthWithin(t *testing.T) {
	for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
		if got, want := firstRunGrowthParse(t, backend); got != want {
			t.Errorf("%s: Document %s; fresh %s", backend, got, want)
		}
	}
	for _, m := range runGrowthMismatches(t, 5) {
		t.Error(m)
	}
}

// TestDocumentRunReadsGrowthWithinControl is the negative control: recording a run that read only
// seeds of growths that started within it makes Document parses differ from fresh ones, already in
// the first parse of "clc?".
func TestDocumentRunReadsGrowthWithinControl(t *testing.T) {
	defer func(m int) { core.RunMutation = m }(core.RunMutation)
	core.RunMutation = core.RunOpenGrowthsOnly
	for _, backend := range []Backend{Closure, Bytecode, BytecodeIterative} {
		if got, want := firstRunGrowthParse(t, backend); got == want {
			t.Errorf("%s: the first Document parse of \"clc?\" equals the fresh one, %s", backend, got)
		}
	}
	if len(runGrowthMismatches(t, 1)) == 0 {
		t.Error("recording runs that read seeds of growths within them changed no Document parse of the random texts")
	}
}

// firstRunGrowthParse returns the first Document parse of "clc?" with the first of
// runGrowthGrammars, and a fresh parse. main tries r at 0 outside any growth, where the first x
// grows the cycle at 0 and reads the seed of that growth, then m at 0, whose growth reaches r at 0
// again: there x reads the seed of the growth that is open.
func firstRunGrowthParse(t *testing.T, backend Backend) (got, want string) {
	t.Helper()
	defer func(n int) { core.MinRecorded = n }(core.MinRecorded)
	core.MinRecorded = 1
	prog := mustCompile(t, runGrowthGrammars[0])
	opts := ParseOptions{Backend: backend}
	d, err := prog.NewDocumentWith("main", "clc?", opts)
	if err != nil {
		t.Fatal(err)
	}
	n, err := d.Parse()
	f, ferr := prog.ParseWith("main", "clc?", opts)
	return resultJSON(n, err), resultJSON(f, ferr)
}
