package gogen_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
	"zeroparse.dev/zp/internal/ir/exec/core"
)

// memberDocumentProgram parses each input with the Documents of generated parsers (Parse and the
// typed ParseAST): twice without an edit, and, for each position, after deleting the character
// there and inserting it again. It prints a line (with quoted outcomes) for each outcome that differs from a parse of the
// text from scratch, then the number of outcomes it compared.
const memberDocumentProgram = `
type parser struct {
	parse    func(string) (any, error)
	parseAST func(string) (any, error)
	newDoc   func(string) (doc, error)
}

type doc struct {
	parse, parseAST func() (any, error)
	edit            func(start, end int, text string) error
}

func outcome(v any, err error) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("%s | %v", b, err)
}

func main() {
	var inputs [][]string
	data, _ := os.ReadFile(os.Args[1])
	json.Unmarshal(data, &inputs)
	n := 0
	for i, ins := range inputs {
		p := parsers[i]
		for _, in := range ins {
			for _, typed := range []bool{false, true} {
				fresh, parse := p.parse, func(d doc) (any, error) { return d.parse() }
				if typed {
					fresh, parse = p.parseAST, func(d doc) (any, error) { return d.parseAST() }
				}
				want := outcome(fresh(in))
				d, err := p.newDoc(in)
				if err != nil {
					panic(err)
				}
				check := func(step string) {
					n++
					if got := outcome(parse(d)); got != want {
						fmt.Printf("DIFF g%d %q typed=%v %s: got %q, want %q\n", i, in, typed, step, got, want)
					}
				}
				check("first parse")
				check("second parse")
				for k := range len(in) {
					if err := d.edit(k, k+1, ""); err != nil {
						panic(err)
					}
					parse(d)
					if err := d.edit(k, k, in[k:k+1]); err != nil {
						panic(err)
					}
					check(fmt.Sprintf("after deleting and inserting again %d", k))
					check(fmt.Sprintf("again after deleting and inserting again %d", k))
				}
			}
		}
	}
	fmt.Printf("compared %d\n", n)
}
`

// runMemberDocuments generates a parser with Documents and typed values for each case into a
// module that imports the runtime from rt (gentest.RuntimeCopy for the runtime as it is), runs
// memberDocumentProgram, and returns the differences it printed per case index and the number of
// outcomes it compared.
func runMemberDocuments(t *testing.T, cases []gentest.Case, rt string) (map[int][]string, int) {
	t.Helper()
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("go command not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gentest.GoModAt("gentest", rt)), 0o644); err != nil {
		t.Fatal(err)
	}
	var imports, parsers strings.Builder
	var inputs [][]string
	for i, c := range cases {
		gentest.GeneratePackage(t, dir, fmt.Sprintf("g%d", i), mustParse(c.Src), gen.GenOptions{Start: "main", Types: true, Document: true})
		fmt.Fprintf(&imports, "\tg%d \"gentest/g%d\"\n", i, i)
		fmt.Fprintf(&parsers, `	{
		func(s string) (any, error) { return g%[1]d.Parse(s) },
		func(s string) (any, error) { return g%[1]d.ParseAST(s) },
		func(s string) (doc, error) {
			d, err := g%[1]d.NewDocument(s)
			if err != nil {
				return doc{}, err
			}
			return doc{
				func() (any, error) { return d.Parse() },
				func() (any, error) { return d.ParseAST() },
				d.Edit,
			}, nil
		},
	},
`, i)
		inputs = append(inputs, c.Inputs)
	}
	main := "package main\n\nimport (\n\t\"encoding/json\"\n\t\"fmt\"\n\t\"os\"\n" + imports.String() + ")\n\nvar parsers = []parser{\n" + parsers.String() + "}\n" + memberDocumentProgram
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(inputs)
	if err := os.WriteFile(filepath.Join(dir, "inputs.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goBin, "run", ".", "inputs.json")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS="+core.GoFlags("-mod=mod -trimpath"), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	diffs := map[int][]string{}
	compared := 0
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	for i := range lines {
		var g int
		if _, err := fmt.Sscanf(lines[i], "DIFF g%d", &g); err == nil {
			diffs[g] = append(diffs[g], lines[i])
		} else if _, err := fmt.Sscanf(lines[i], "compared %d", &compared); err != nil {
			t.Fatalf("unexpected output %q\n%s", lines[i], out)
		}
	}
	return diffs, compared
}

// TestGeneratedDocumentLeftRecursionMemberReparse checks that the Documents of generated Go parsers
// (untyped and typed) of grammars whose left recursions store results that depend on the order of
// evaluation give the outcome of a parse from scratch, when parsed again without an edit and after
// edits. TestDocumentLeftRecursionMemberReparse checks the engine's Documents.
func TestGeneratedDocumentLeftRecursionMemberReparse(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testGeneratedDocumentLeftRecursionMemberReparse)
}

func testGeneratedDocumentLeftRecursionMemberReparse(t *testing.T) {
	if testing.Short() {
		t.Skip("builds generated code")
	}
	cases := append(gentest.LeftRecursionMemberCases(), gentest.LeftRecursionMemberSeedCases()...)
	cases = append(cases, gentest.LeftRecursionMemberDocumentCases()...)
	diffs, compared := runMemberDocuments(t, cases, gentest.RuntimeCopy())
	for i, c := range cases {
		for _, d := range diffs[i] {
			t.Errorf("%s: %s", c.Name, d)
		}
	}
	if compared == 0 {
		t.Error("no outcome compared")
	}
}

// TestGeneratedDocumentLeftRecursionMemberReparseControl checks that the checks of
// TestGeneratedDocumentLeftRecursionMemberReparse detect a later parse that reuses results that
// read a growing seed: with a runtime that keeps them in the memo of the Document for later parses,
// whatever the leaders growing there, each of LeftRecursionMemberDocumentCases differs from a parse
// from scratch, untyped and typed.
func TestGeneratedDocumentLeftRecursionMemberReparseControl(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds generated code")
	}
	gentest.PinOrdinary(t)
	flush := "for _, f := range keptSeedResults {\n\t\tf()\n\t}\n\tkeptSeedResults = nil\n"
	reps := map[string][2]string{
		"document.go":       {"\t\td.memo = p.memo\n", "\t" + flush + "\t\td.memo = p.memo\n"},
		"typed_document.go": {"\tcache.slots = p.memoSlots\n", flush + "\tcache.slots = p.memoSlots\n"},
	}
	// The results that a call keeps in its context are also put in the memo when the parse ends:
	// before each ctxKeep of a call, in the hand-written calls (call.go) and in the compositions of
	// the Exec IR (call_exec.go).
	keep := func(doc, minExpr, entry, alloc, meta string) string {
		return "if " + doc + " != nil {\n" +
			"\t\t\tentry := " + entry + "\n" +
			"\t\t\texamined, rule := p.docHigh, r.Id\n\t\t\t_ = examined\n" +
			"\t\t\tkeptSeedResults = append(keptSeedResults, func() {\n" +
			"\t\t\t\te := " + alloc + "\n\t\t\t\t*e = entry\n" +
			"\t\t\t\tp.memoPut(rule, start, " + minExpr + ", env, e)\n" + meta +
			"\t\t\t})\n\t\t}\n\t\t"
	}
	nodeKeep := keep("p.Document", "min", "memoEntry{node: v, ok: ok, end: p.Pos, examined: p.docHigh, present: ^uint32(0), errs: p.copyErrors(nil, p.Recovered[rec:]), far: far, expected: kept}", "p.memo.alloc()", "")
	typedKeep := func(minExpr string) string {
		return keep("p.TypedDocument", minExpr, "tmemoEntry{val: v, ok: ok, end: p.Pos, present: ^uint32(0), errs: p.copyErrors(nil, p.Recovered[rec:]), far: far, expected: kept}", "p.memoSlab.one()", "\t\t\t\tp.TypedDocument.meta[e] = typedDocumentMeta{examined: examined}\n")
	}
	// Each anchor is the ctxKeep of a call, which the mutation prefixes, and the number of times it
	// occurs in the file.
	anchors := map[string][]struct {
		at, prefix string
		n          int
	}{
		"call.go": {
			{"p.ctxKeep(&c, r.Id, start, min, env, &ctxResult{node: v,", nodeKeep, 1},
			{"p.ctxKeep(&c, r.Id, start, min, env, &ctxResult{val: v,", typedKeep("min"), 1},
		},
		"call_exec.go": {
			{"p.ctxKeep(&c, r.Id, start, min, env, &ctxResult{node: v,", nodeKeep, -1},
			{"p.ctxKeep(&c, r.Id, start, min, env, &ctxResult{val: v,", typedKeep("min"), -1},
			{"p.ctxKeep(&c, r.Id, start, 0, env, &ctxResult{val: v,", typedKeep("0"), -1},
		},
	}
	rt := gentest.RuntimeCopyWith(t, func(name, src string) string {
		if r, ok := reps[name]; ok {
			if strings.Count(src, r[0]) != 1 {
				t.Fatalf("runtime anchor changed: %q", r[0])
			}
			src = strings.Replace(src, r[0], r[1], 1)
		}
		for _, a := range anchors[name] {
			// -1: at least once.
			if n := strings.Count(src, a.at); n == 0 || a.n > 0 && n != a.n {
				t.Fatalf("%s: runtime anchor changed: %q", name, a.at)
			}
			src = strings.ReplaceAll(src, a.at, a.prefix+a.at)
		}
		return src
	}, map[string]string{"zz_keep.go": "package runtime\n\nvar keptSeedResults []func()\n"})
	cases := gentest.LeftRecursionMemberDocumentCases()
	diffs, _ := runMemberDocuments(t, cases, rt)
	for i, c := range cases {
		for _, typed := range []string{"typed=false", "typed=true"} {
			found := false
			for _, d := range diffs[i] {
				found = found || strings.Contains(d, typed)
			}
			if !found {
				t.Errorf("%s: no difference (%s) with the results kept", c.Name, typed)
			}
		}
	}
}
