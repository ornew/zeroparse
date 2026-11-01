package engine

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/compile"
	"zeroparse.dev/zp/internal/engine/core"
	"zeroparse.dev/zp/internal/gentest"
	"zeroparse.dev/zp/internal/limits"
	"zeroparse.dev/zp/internal/value"
)

// skipRunGrammars are the grammars with skip-run sites that the tests parse.
var skipRunGrammars = gentest.SkipRunGrammars

// skipDump is dump with the expectations and messages of every syntax error, which the message
// of an error leaves out when it has messages.
func skipDump(t *testing.T, n *value.Node, err error) string {
	t.Helper()
	s := dump(t, n, err)
	var errs []*core.SyntaxError
	var list core.SyntaxErrors
	var one *core.SyntaxError
	switch {
	case errors.As(err, &list):
		errs = list
	case errors.As(err, &one):
		errs = []*core.SyntaxError{one}
	}
	for _, e := range errs {
		s += fmt.Sprintf(" [%d %q %q]", e.Pos, e.Expected, e.Messages)
	}
	return s
}

// skipRunMismatches parses every input with records and without them on the backend and returns
// the descriptions of the parses that differ.
func skipRunMismatches(t *testing.T, prog *Program, inputs []string, b Backend) []string {
	t.Helper()
	var diffs []string
	for _, in := range inputs {
		for _, exp := range []bool{true, false} {
			diffs = append(diffs, skipRunMismatch(t, prog, in, ParseOptions{Backend: b, Expectations: exp})...)
		}
	}
	return diffs
}

// skipRunMismatch parses in with records and without them and returns the description of the
// difference, if any.
func skipRunMismatch(t *testing.T, prog *Program, in string, o ParseOptions) []string {
	t.Helper()
	var diffs []string
	{
		core.SkipRecordsOff = true
		n, err := prog.ParseWith("main", in, o)
		want := skipDump(t, n, err)
		core.SkipRecordsOff = false
		n, err = prog.ParseWith("main", in, o)
		if got := skipDump(t, n, err); got != want {
			diffs = append(diffs, "input "+strings.ReplaceAll(in, "\n", `\n`)+"\n got  "+got+"\n want "+want)
		}
	}
	return diffs
}

// TestSkipRunsMatchParsesWithoutRecords checks that the records of skip runs change no result:
// values, recovered errors and syntax errors, on every backend. It toggles package state, so it
// does not run in parallel.
func TestSkipRunsMatchParsesWithoutRecords(t *testing.T) {
	rng := rand.New(rand.NewSource(75))
	for _, g := range skipRunGrammars {
		prog := mustCompile(t, g.Src)
		inputs := gentest.SkipRunInputs(g.Alphabet, 300, rng)
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			if diffs := skipRunMismatches(t, prog, inputs, b); len(diffs) > 0 {
				t.Errorf("%s, %s: %d parses differ, first:\n%s", g.Name, b, len(diffs), diffs[0])
			}
		}
	}
	for _, c := range gentest.GenCorpus(t) {
		if !strings.Contains(c.Src, "#recover") {
			continue
		}
		g, err := parseTest(c.Src)
		if err != nil {
			t.Fatalf("corpus %s: %v", c.Name, err)
		}
		prog, err := Compile(g, compile.Options{SourceInfo: true})
		if err != nil {
			t.Fatalf("corpus %s: %v", c.Name, err)
		}
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			if diffs := skipRunMismatches(t, prog, c.Inputs, b); len(diffs) > 0 {
				t.Errorf("corpus %s, %s: %d parses differ, first:\n%s", c.Name, b, len(diffs), diffs[0])
			}
		}
	}
}

// TestSkipRunsNegativeControls checks that the comparisons detect a record that leaves out its end
// or its expectations, and records stored when their iteration ends instead of when the walk does.
// Records are reused from the third walk over a position on, which the nested family reaches at
// once.
func TestSkipRunsNegativeControls(t *testing.T) {
	defer func() { core.SkipRecordMutation = 0 }()
	var prog *Program
	for _, g := range skipRunGrammars {
		if g.Name == "nested recovery" {
			prog = mustCompile(t, g.Src)
		}
	}
	for _, tc := range []struct {
		mutation int
		input    string
	}{
		{core.SkipKeepPos, "((((a"},
		{core.SkipNoExpected, "((((a"},
		{core.SkipEarlyStore, "((((((a("},
	} {
		core.SkipRecordMutation = tc.mutation
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			if diffs := skipRunMismatches(t, prog, []string{tc.input}, b); len(diffs) == 0 {
				t.Errorf("mutation %d, %s: no difference on %q", tc.mutation, b, tc.input)
			}
		}
		core.SkipRecordMutation = 0
	}
}

// TestSkipRunsBudgetAbort checks that a parse stopped by the budget in the middle of a walk leaves
// no partial records behind: at every budget, a parse either fails with the budget limit or gives
// the result of the unlimited parse, and an unlimited parse after each stopped one does too. The
// nested family stops in recording walks at most budgets.
func TestSkipRunsBudgetAbort(t *testing.T) {
	var prog *Program
	for _, g := range skipRunGrammars {
		if g.Name == "nested recovery" {
			prog = mustCompile(t, g.Src)
		}
	}
	in := strings.Repeat("(", 30) + "a" + strings.Repeat(")", 5)
	for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
		n, err := prog.ParseWith("main", in, ParseOptions{Backend: b, Expectations: true})
		want := skipDump(t, n, err)
		stopped := 0
		for budget := uint64(1); ; budget++ {
			o := ParseOptions{Backend: b, Expectations: true, Limits: limits.Limits{Budget: budget}}
			n, err := prog.ParseWith("main", in, o)
			if errors.Is(err, limits.ErrLimit) {
				stopped++
				n, err := prog.ParseWith("main", in, ParseOptions{Backend: b, Expectations: true})
				if got := skipDump(t, n, err); got != want {
					t.Fatalf("%s: after a parse stopped at budget %d:\n got  %s\n want %s", b, budget, got, want)
				}
				continue
			}
			if got := skipDump(t, n, err); got != want {
				t.Fatalf("%s: budget %d:\n got  %s\n want %s", b, budget, got, want)
			}
			break
		}
		if stopped < 100 {
			t.Fatalf("%s: only %d budgets stop the parse", b, stopped)
		}
	}
}

// TestSkipRunsDocument checks that Documents, which reuse results by the input range they examined,
// give the results of fresh parses with skip-run records, and that random edits detect records
// that leave out the end or the start of their examined range.
func TestSkipRunsDocument(t *testing.T) {
	defer func() { core.SkipRecordMutation = 0 }()
	var progs []*Program
	var alphabets []string
	for _, g := range skipRunGrammars {
		if g.Name == "line starts in the element" || g.Name == "look-ahead past the end" {
			progs, alphabets = append(progs, mustCompile(t, g.Src)), append(alphabets, g.Alphabet)
		}
	}
	for _, mutation := range []int{0, core.SkipNoExamined, core.SkipNoFrom} {
		core.SkipRecordMutation = mutation
		for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
			detected := false
			for i, prog := range progs {
				diff := skipRunDocumentDiff(t, prog, alphabets[i], b)
				if mutation == 0 && diff != "" {
					t.Errorf("%s: %s", b, diff)
				}
				detected = detected || diff != ""
			}
			if mutation != 0 && !detected {
				t.Errorf("mutation %d, %s: no difference", mutation, b)
			}
		}
	}
}

// skipRunDocumentDiff edits a Document at random and returns the first difference between its parse
// and a fresh parse without records, or "". Under a mutation, a Document that reuses a stale
// result can also run past the end of its input; the panic counts as a difference.
func skipRunDocumentDiff(t *testing.T, prog *Program, alphabet string, b Backend) (diff string) {
	t.Helper()
	if core.SkipRecordMutation != 0 {
		defer func() {
			if e := recover(); e != nil {
				diff = fmt.Sprint("panic: ", e)
			}
		}()
	}
	rng := rand.New(rand.NewSource(7))
	o := ParseOptions{Backend: b, Expectations: true}
	doc, err := prog.NewDocumentWith("main", "!a?ad\n?baz\n!aa?ac\n", o)
	if err != nil {
		t.Fatal(err)
	}
	doc.Parse()
	for i := 0; i < 3000; i++ {
		cur := []rune(doc.Text())
		start := rng.Intn(len(cur) + 1)
		end := start + rng.Intn(min(3, len(cur)-start)+1)
		ins := ""
		for k := rng.Intn(3); k > 0; k-- {
			ins += string(alphabet[rng.Intn(len(alphabet))])
		}
		if len(cur)-(end-start)+len(ins) > 40 {
			ins = ""
		}
		if err := doc.Edit(start, end, ins); err != nil {
			t.Fatal(err)
		}
		n, err := doc.Parse()
		got := skipDump(t, n, err)
		mutation := core.SkipRecordMutation
		core.SkipRecordsOff, core.SkipRecordMutation = true, 0
		fn, ferr := prog.ParseWith("main", doc.Text(), o)
		core.SkipRecordsOff, core.SkipRecordMutation = false, mutation
		if want := skipDump(t, fn, ferr); got != want {
			return "edit " + strings.ReplaceAll(doc.Text(), "\n", `\n`) + "\n got  " + got + "\n want " + want
		}
	}
	return ""
}

// TestSkipRunsStream checks that a stream, which drops the records before each commit it prunes
// at, streams the elements and errors of a parse without records.
func TestSkipRunsStream(t *testing.T) {
	// The walks before an "e" fail and those before a "d" recover.
	prog := mustCompile(t, `rule main = (x / [ace])* #stream $$
rule x = ("b" #recover(skip=[ac]+)) "d"`)
	input := strings.Repeat(strings.Repeat("a", 40)+"c"+strings.Repeat("a", 9)+"e"+strings.Repeat("a", 20)+"d", 50)
	run := func(off bool, b Backend) string {
		core.SkipRecordsOff = off
		defer func() { core.SkipRecordsOff = false }()
		var out strings.Builder
		err := prog.ParseStreamWith("main", strings.NewReader(input), func(n *value.Node) error {
			out.WriteString(skipDump(t, n, nil) + "\n")
			return nil
		}, ParseOptions{Backend: b, Expectations: true})
		return out.String() + skipDump(t, nil, err)
	}
	for _, b := range []Backend{Closure, Bytecode, BytecodeIterative} {
		if got, want := run(false, b), run(true, b); got != want {
			t.Errorf("%s: the streams differ", b)
		}
	}
}

// TestSkipRunsWideMinimum checks a skip-run site whose minimum needs a wide operand, through the
// bytecode and its file.
func TestSkipRunsWideMinimum(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("bounds exceed a 32-bit implementation's source integer range")
	}
	prog := mustCompile(t, `rule main = (x / [ab])* $$
rule x = ("c" #recover(skip=[a] #repeat(4294967296,) / [b]+)) "d"`)
	if dis := prog.Module().Disassemble(); !strings.Contains(dis, "SKIPREPEAT min=4294967296 site=1") || !strings.Contains(dis, "SKIPREPEAT min=1 site=2") {
		t.Fatalf("no wide SKIPREPEAT:\n%s", dis)
	}
	for _, mode := range valueModes {
		saved := saveLoad(t, prog, "main", mode)
		for _, b := range []Backend{Bytecode, BytecodeIterative} {
			for _, in := range []string{"aabbd", "bbbab", "abd"} {
				if diffs := skipRunMismatch(t, saved, in, servedBy(mode, ParseOptions{Backend: b, Expectations: true})); len(diffs) > 0 {
					t.Errorf("%s %s: %s", mode, b, diffs[0])
				}
			}
		}
	}
}
