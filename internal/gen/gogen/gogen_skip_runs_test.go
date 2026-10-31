package gogen_test

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/ornew/zeroparse/internal/complexitytest"
	"github.com/ornew/zeroparse/internal/gen"
	"github.com/ornew/zeroparse/internal/gentest"
)

// skipRunsGoBin returns the go command for the tests that build generated parsers, or skips.
func skipRunsGoBin(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds generated code")
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("go command not found")
	}
	return goBin
}

// TestSkipRunsGeneratedMatchEngine checks that generated parsers, whose recovery skips record and
// reuse the walks of their skip-run sites, give the engine's results on the skip-run grammars:
// values, recovered errors and syntax errors with their expectations, from the Node and typed
// entry points.
func TestSkipRunsGeneratedMatchEngine(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testSkipRunsGeneratedMatchEngine)
}

func testSkipRunsGeneratedMatchEngine(t *testing.T) {
	goBin := skipRunsGoBin(t)
	cases := gentest.SkipRunCases(60)
	gentest.GeneratedParsersCorpus(t, goBin, cases, gen.GenOptions{})
	gentest.GeneratedTypesCorpus(t, goBin, cases, gen.GenOptions{})
}

// TestSkipRunsGeneratedStream checks a generated stream whose elements recover with skip-run sites
// over input long enough for the stream to drop records at its commits.
func TestSkipRunsGeneratedStream(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testSkipRunsGeneratedStream)
}

func testSkipRunsGeneratedStream(t *testing.T) {
	goBin := skipRunsGoBin(t)
	// The walks before an "e" fail and those before a "d" recover.
	unit := strings.Repeat("a", 40) + "c" + strings.Repeat("a", 9) + "e" + strings.Repeat("a", 20) + "d"
	cases := []gentest.Case{{Name: "skip runs", Src: `rule main = (x / [ace])* #stream $$
rule x = ("b" #recover(skip=[ac]+)) "d"`, Inputs: []string{strings.Repeat(unit, 50), strings.Repeat(unit, 50) + "b"}}}
	testGeneratedStreams(t, goBin, cases, gen.GenOptions{})
}

// TestSkipRunsGeneratedDocuments checks generated Documents, which reuse results by the input they
// examined, against batch parses of the same package along random edits of the skip-run grammars
// whose elements look ahead or at the preceding character.
func TestSkipRunsGeneratedDocuments(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testSkipRunsGeneratedDocuments)
}

func testSkipRunsGeneratedDocuments(t *testing.T) {
	goBin := skipRunsGoBin(t)
	dir := t.TempDir()
	write := func(name, content string) {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", gentest.GoMod("skipdoc"))
	var imports, checks strings.Builder
	n := 0
	for _, c := range gentest.SkipRunGrammars {
		switch c.Name {
		case "repeated recovery", "line starts in the element", "look-ahead past the end", "silent walks":
		default:
			continue
		}
		g, err := parseTest(c.Src)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		gentest.GeneratePackageIn(t, dir, "skipdoc", fmt.Sprintf("g%d", n), g, gen.GenOptions{Start: "main", Types: true, Document: true})
		fmt.Fprintf(&imports, "\tg%d \"skipdoc/g%d\"\n", n, n)
		fmt.Fprintf(&checks, `	{%q, %q, func(text string, edits func(string) (int, int, string, bool)) string {
		d, err := g%[3]d.NewDocument(text)
		if err != nil {
			return err.Error()
		}
		for {
			n, err := d.Parse()
			bn, berr := g%[3]d.Parse(d.Text())
			if s := differ("Parse", d.Text(), n, err, bn, berr); s != "" {
				return s
			}
			v, err := d.ParseAST()
			bv, berr := g%[3]d.ParseAST(d.Text())
			if s := differ("ParseAST", d.Text(), v, err, bv, berr); s != "" {
				return s
			}
			start, end, ins, ok := edits(d.Text())
			if !ok {
				return ""
			}
			if err := d.Edit(start, end, ins); err != nil {
				return err.Error()
			}
		}
	}},
`, c.Name, c.Alphabet, n)
		n++
	}
	write("main.go", `package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"unicode/utf8"

`+imports.String()+`)

func differ(what, text string, got any, gotErr error, want any, wantErr error) string {
	g, err1 := json.Marshal(got)
	w, err2 := json.Marshal(want)
	if err1 != nil || err2 != nil || string(g) != string(w) || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
		return fmt.Sprintf("%s %q: Document %s / %v; batch %s / %v", what, text, g, gotErr, w, wantErr)
	}
	return ""
}

var checks = []struct {
	name, alphabet string
	run            func(string, func(string) (int, int, string, bool)) string
}{
`+checks.String()+`}

func main() {
	failed := false
	for _, c := range checks {
		rng := rand.New(rand.NewSource(7))
		left := 30000 // fewer edits miss a record that leaves out its examined frontier
		edits := func(text string) (int, int, string, bool) {
			if left == 0 {
				return 0, 0, "", false
			}
			left--
			n := utf8.RuneCountInString(text)
			start := rng.Intn(n + 1)
			end := start + rng.Intn(min(3, n-start)+1)
			ins := ""
			for k := rng.Intn(3); k > 0; k-- {
				ins += string(c.alphabet[rng.Intn(len(c.alphabet))])
			}
			if n-(end-start)+len(ins) > 40 {
				ins = ""
			}
			return start, end, ins, true
		}
		if s := c.run("!a?ad\n?baz\n!aa?ac\n", edits); s != "" {
			failed = true
			fmt.Printf("%s: %s\n", c.name, s)
		}
	}
	if failed {
		os.Exit(1)
	}
}
`)
	cmd := exec.Command(goBin, "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS="+gentest.GoFlags(t, "-mod=mod -trimpath"), "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// skipRunMutations are the changes of the embedded runtime for the negative controls: walks that
// never record, and records stored when their iteration ends that the end of the walk leaves as
// they are.
var skipRunMutations = map[string][][2]string{
	"never record": {{
		"if !w.recording && w.slots.marked(p.Pos) {",
		"if false {",
	}},
	"early store": {{
		"	v.examined = p.docHigh\n",
		"	v.examined = p.docHigh\n\tp.skipTable().put(v.pos, skipRecord{end: p.Pos, far: v.far, exp: v.exp, examined: v.examined, state: skipRecorded})\n",
	}, {
		"	p.skipClose()\n\tvs := w.visits\n",
		"	p.skipClose()\n\tw.visits = w.visits[:0]\n\tif true {\n\t\treturn\n\t}\n\tvs := w.visits\n",
	}},
}

// skipRunSpends builds the E8 controls with the embedded runtime changed by mutation (none if "")
// and returns, for each control, the least budget of the parse of the input of each size, and the
// results of the parses of the nested inputs with the nested recovery grammar of the skip-run tests.
func skipRunSpends(t *testing.T, goBin string, controls []complexitytest.Control, sizes []int, nested []string, mutation string) (spends [][]uint64, results []string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, content []byte) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", []byte("module skiplin\n\ngo 1.27.1\n"))
	var imports, entries strings.Builder
	sources := controls
	for _, g := range gentest.SkipRunGrammars {
		if g.Name == "nested recovery" {
			sources = append(sources[:len(controls):len(controls)], complexitytest.Control{Name: g.Name, Source: g.Src, Input: func(int) string { return "" }})
		}
	}
	for i, c := range sources {
		g, err := parseTest(c.Source)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		pkg := fmt.Sprintf("g%d", i)
		files, err := gentest.Generate(t, g, gen.GenOptions{Module: "skiplin/" + pkg, Package: pkg, Start: "main", Runtime: gen.RuntimeEmbed, BudgetControls: true})
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for _, f := range files {
			content := string(f.Content)
			if strings.HasSuffix(f.Path, "runtime/skip_runs.go") && mutation != "" {
				for _, r := range skipRunMutations[mutation] {
					if !strings.Contains(content, r[0]) {
						t.Fatalf("mutation %s: %q not in the runtime", mutation, r[0])
					}
					content = strings.Replace(content, r[0], r[1], 1)
				}
			}
			write(pkg+"/"+f.Path, []byte(content))
		}
		var inputs []string // none for the nested recovery grammar, which only gives results
		if i < len(controls) {
			for _, n := range sizes {
				inputs = append(inputs, c.Input(n))
			}
		}
		fmt.Fprintf(&imports, "\t%q\n", "skiplin/"+pkg)
		fmt.Fprintf(&entries, `	{%#v, func(s string, budget uint64) (bool, string) {
		n, err := %[2]s.Parse(s, %[2]s.WithLimits(%[2]s.Limits{Budget: budget}))
		j, _ := json.Marshal(n)
		return errors.Is(err, %[2]s.ErrLimit), string(j) + " " + fmt.Sprint(err)
	}},
`, inputs, pkg)
	}
	write("main.go", []byte(`package main

import (
	"encoding/json"
	"errors"
	"fmt"

`+imports.String()+`)

var entries = []struct {
	inputs []string
	parse  func(string, uint64) (bool, string)
}{
`+entries.String()+`}

var nested = `+fmt.Sprintf("%#v", nested)+`

func main() {
	for _, e := range entries {
		for _, s := range e.inputs {
			lo, hi := uint64(1), uint64(1)
			for {
				if limited, _ := e.parse(s, hi); !limited {
					break
				}
				lo, hi = hi+1, hi*2
			}
			for lo < hi {
				mid := (lo + hi) / 2
				if limited, _ := e.parse(s, mid); limited {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
			fmt.Print(lo, " ")
		}
		if len(e.inputs) > 0 {
			fmt.Println()
		}
	}
	for _, s := range nested {
		_, r := entries[len(entries)-1].parse(s, 1<<40)
		fmt.Println(r)
	}
}
`))
	cmd := exec.Command(goBin, "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS="+gentest.GoFlags(t, "-mod=mod -trimpath"), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	for _, l := range lines[:len(controls)] {
		var row []uint64
		for _, f := range strings.Fields(l) {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				t.Fatalf("malformed spend line %q", l)
			}
			row = append(row, v)
		}
		spends = append(spends, row)
	}
	return spends, lines[len(controls):]
}

// TestSkipRunsGeneratedLinear checks that the E8 controls (repeated, nested and alternating walks)
// spend a budget linear in the input in generated parsers, that walks that never record make them
// quadratic, and that records stored before the walk ends change the results of nested inputs.
func TestSkipRunsGeneratedLinear(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testSkipRunsGeneratedLinear)
}

func testSkipRunsGeneratedLinear(t *testing.T) {
	goBin := skipRunsGoBin(t)
	gentest.NeedsBudgetControls(t)
	var controls []complexitytest.Control
	for _, c := range complexitytest.Controls() {
		if c.Family == "E8" {
			controls = append(controls, c)
		}
	}
	if len(controls) != 3 || !strings.Contains(controls[1].Name, "nested") {
		t.Fatalf("unexpected E8 controls %v", controls)
	}
	sizes := []int{200, 400, 800}
	nested := []string{"((((((a(", "((((a", "((a((a(((a"}
	slopes := func(row []uint64) (lo, hi float64) {
		lo, hi = math.Inf(1), math.Inf(-1)
		for i := 1; i < len(row); i++ {
			s := math.Log2(float64(row[i]) / float64(row[i-1]))
			lo, hi = min(lo, s), max(hi, s)
		}
		return lo, hi
	}
	spends, results := skipRunSpends(t, goBin, controls, sizes, nested, "")
	t.Logf("spends at %v: %v", sizes, spends)
	for i, row := range spends {
		if _, hi := slopes(row); hi > 1.2 {
			t.Errorf("%s: spends %v grow faster than linearly", controls[i].Name, row)
		}
	}
	spends, _ = skipRunSpends(t, goBin, controls, sizes, nested, "never record")
	t.Logf("spends of walks that never record: %v", spends)
	for i, row := range spends {
		if lo, _ := slopes(row); lo < 1.7 {
			t.Errorf("%s, walks that never record: spends %v are not quadratic", controls[i].Name, row)
		}
	}
	_, early := skipRunSpends(t, goBin, controls, sizes, nested, "early store")
	differ := false
	for i := range results {
		differ = differ || results[i] != early[i]
	}
	if !differ {
		t.Errorf("records stored before the walk ends change no result of %q", nested)
	}
}
