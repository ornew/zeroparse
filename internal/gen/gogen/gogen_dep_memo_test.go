package gogen_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
	"zeroparse.dev/zp/internal/ir/exec/core"
)

// depMemoGenCases returns the grammars of gentest.DependencyMemoCases for seeds below seeds that
// generate Go parsers.
func depMemoGenCases(t *testing.T, seeds int) []gentest.Case {
	t.Helper()
	var cases []gentest.Case
	for _, c := range gentest.DependencyMemoCases(seeds) {
		g, err := parseTest(c.Src)
		if err != nil {
			continue
		}
		if _, err := gentest.GenerateCode(t, g, gen.GenOptions{Package: "g", Start: "main", Recognize: true}); err != nil {
			continue
		}
		cases = append(cases, c)
	}
	if len(cases) < seeds*2 {
		t.Fatalf("only %d grammars of %d seeds generate", len(cases), seeds)
	}
	return cases
}

// TestGeneratedLeftRecursionDependencyMemo checks that the Parse, Recognize and ParseAST of
// generated Go parsers give the engine's results for grammars whose leaders grow inside the growth
// of other leaders of their cycle at one position, where the dependency memo reuses results by the
// seeds they read and the lookaheads they observed.
func TestGeneratedLeftRecursionDependencyMemo(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testGeneratedLeftRecursionDependencyMemo)
}

func testGeneratedLeftRecursionDependencyMemo(t *testing.T) {
	if testing.Short() {
		t.Skip("builds generated code")
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("go command not found")
	}
	cases := depMemoGenCases(t, 60)
	gentest.GeneratedParsersCorpus(t, goBin, cases, gen.GenOptions{})
	gentest.GeneratedTypesCorpus(t, goBin, cases, gen.GenOptions{ConvertTypes: true}, len(cases))
}

// depMemoMutations are the changes of the embedded runtime for the negative controls of
// TestGeneratedLeftRecursionDependencyMemoControls, each a list of replacements: a reuse that does
// not compare the identities of the seeds read, that does not check observations, a result in the
// memo reused whatever its present set, equal step results given the same identity without
// comparing them, a jump that uses observations whose outcomes read the growing seed, a reused
// result that replays the seeds its observations read where it was made, and an observation that
// summarizes a lookahead that recovered errors.
var depMemoMutations = map[string][][2]string{
	"seed identities": {
		{"k.v[k.n] = p.gamma[i].id\n", "k.v[k.n] = 0\n"},
		{"if d.kind == depSeed {\n\t\t\tk.v[k.n] = d.val\n", "if d.kind == depSeed {\n\t\t\tk.v[k.n] = 0\n"},
	},
	"observations": {
		{"v := p.obsValue(pos, gp, h.present, o)\n", "v := obsUnique + 0*uint64(o)\n"},
		{"if d.kind == depObs {\n\t\t\tk.v[k.n] = d.val\n", "if d.kind == depObs {\n\t\t\tk.v[k.n] = obsUnique\n"},
		{"if v, free := p.obsFree(g.start, g.gp, g.present, d.site, g.growKey()); v != d.val || !free {", "if false {"},
	},
	"present growths": {
		{"return !c.ctx || c.known && c.present&^present == 0\n", "return true\n"},
	},
	"canonical seeds": {
		{"if c.e.examined == e.examined && len(c.e.errs) == 0 && p.nodesEqual(", "if true || p.nodesEqual("},
	},
	"jumps over observations of the growing seed": {
		{"v != d.val || !free {", "v != d.val || !free && false {"},
	},
	"replayed observations": {
		{"buf = append(buf, dep{kind: d.kind, idx: int32(i), d: depth, val: p.gamma[i].id})", "buf = append(buf, dep{kind: d.kind, idx: int32(i), d: depth, val: d.val})"},
		{"buf = p.replayObs(buf, pos, gp, present, d.site, depth)\n", "\n"},
	},
	"lookaheads with recovered errors": {
		{"if !read || ok && len(p.Recovered) > l.rec {", "if !read {"},
	},
}

// TestGeneratedLeftRecursionDependencyMemoControls checks that the grammars of
// TestGeneratedLeftRecursionDependencyMemo detect, in generated Go parsers with and without the
// Exec IR compositions, each check of the dependency memo: with the check broken, some result
// differs from those of the runtime as it is.
func TestGeneratedLeftRecursionDependencyMemoControls(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds generated code")
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("go command not found")
	}
	// The mutated runtimes have no budget controls.
	gentest.PinOrdinary(t)
	cases := depMemoGenCases(t, 120)
	for _, flags := range []string{"-mod=mod -trimpath", core.TagFlags("-mod=mod -trimpath")} {
		want := depMemoOutcomes(t, goBin, cases, "", flags)
		for name := range depMemoMutations {
			if got := depMemoOutcomes(t, goBin, cases, name, flags); got == want {
				t.Errorf("GOFLAGS %q: %s: the mutation went undetected", flags, name)
			}
		}
	}
}

// depMemoOutcomes generates the parsers of cases with the embedded runtime changed by the mutation
// (none if ""), parses their inputs, untyped and typed, with a program built with the GOFLAGS flags
// and returns the outcomes. The values record the rules that produced them: two step results that
// differ only in that are different seeds.
func depMemoOutcomes(t *testing.T, goBin string, cases []gentest.Case, mutation, flags string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, content []byte) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", []byte("module depmemo\n\ngo 1.27.1\n"))
	var imports, calls strings.Builder
	reps := depMemoMutations[mutation]
	applied := make([]bool, len(reps))
	for i, c := range cases {
		g, err := parseTest(c.Src)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		pkg := fmt.Sprintf("g%d", i)
		files, err := gentest.Generate(t, g, gen.GenOptions{Module: "depmemo/" + pkg, Package: pkg, Start: "main", Types: true, SourceInfo: true, Runtime: gen.RuntimeEmbed})
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		for _, f := range files {
			content := string(f.Content)
			if strings.HasPrefix(f.Path, "internal/runtime/") && mutation != "" {
				for k, r := range reps {
					if n := strings.Count(content, r[0]); n > 1 {
						t.Fatalf("mutation %s: %q is not unique", mutation, r[0])
					} else if n == 1 {
						content = strings.Replace(content, r[0], r[1], 1)
						applied[k] = true
					}
				}
			}
			write(pkg+"/"+f.Path, []byte(content))
		}
		fmt.Fprintf(&imports, "\t%q\n", "depmemo/"+pkg)
		for _, in := range c.Inputs {
			fmt.Fprintf(&calls, "\tshow(%s.Parse(%q))\n\tshow(%[1]s.ParseAST(%[2]q))\n", pkg, in)
		}
	}
	for k, ok := range applied {
		if !ok {
			t.Fatalf("mutation %s: %q not in the runtime", mutation, reps[k][0])
		}
	}
	write("main.go", []byte(`package main

import (
	"encoding/json"
	"fmt"
`+imports.String()+`)

func show(n any, err error) {
	j, _ := json.Marshal(n)
	fmt.Println(string(j), err)
}

func main() {
`+calls.String()+`}
`))
	cmd := exec.Command(goBin, "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS="+core.GoFlags(flags), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mutation %q: %v\n%s", mutation, err, out)
	}
	return string(out)
}

// TestGeneratedLeftRecursionDependencyMemoLinear checks that the Parse and ParseAST of generated Go
// parsers of gentest.DependencyMemoShapes evaluate a number of calls linear in the length of the
// input: doubling the input at most a little more than doubles the evaluations.
func TestGeneratedLeftRecursionDependencyMemoLinear(t *testing.T) {
	t.Parallel()
	gentest.EachMode(t, testGeneratedLeftRecursionDependencyMemoLinear)
}

func testGeneratedLeftRecursionDependencyMemoLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("builds generated code")
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(goBin); err != nil {
		t.Skip("go command not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gentest.GoMod("gentest")), 0o644); err != nil {
		t.Fatal(err)
	}
	var imports, calls strings.Builder
	for i, s := range gentest.DependencyMemoShapes {
		pkg := fmt.Sprintf("g%d", i)
		gentest.GeneratePackage(t, dir, pkg, mustParse("@start\nrule main = "+s.Rules), gen.GenOptions{Start: "main", Types: true, Observability: true})
		fmt.Fprintf(&imports, "\t%q\n", "gentest/"+pkg)
		for _, n := range []int{80, 160} {
			in := strconv.Quote(s.Input(n))
			fmt.Fprintf(&calls, "\t{ var p zprt.Profile; if _, err := %s.Parse(%s, zprt.WithProfile(&p)); err != nil { panic(err) }; fmt.Print(p.Snapshot().Evals, \" \") }\n", pkg, in)
			fmt.Fprintf(&calls, "\t{ var p zprt.Profile; if _, err := %s.ParseAST(%s, zprt.WithProfile(&p)); err != nil { panic(err) }; fmt.Print(p.Snapshot().Evals, \" \") }\n", pkg, in)
		}
		calls.WriteString("\tfmt.Println()\n")
	}
	main := `package main

import (
	"fmt"

	zprt "zeroparse.dev/zp/runtime"
` + imports.String() + `)

func main() {
` + calls.String() + `}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goBin, "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS="+gentest.GoFlags(t, "-mod=mod -trimpath"), "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(gentest.DependencyMemoShapes) {
		t.Fatalf("got %d lines, want %d\n%s", len(lines), len(gentest.DependencyMemoShapes), out)
	}
	for i, s := range gentest.DependencyMemoShapes {
		var parse80, ast80, parse160, ast160 int
		if _, err := fmt.Sscan(lines[i], &parse80, &ast80, &parse160, &ast160); err != nil {
			t.Fatalf("%s: %q: %v", s.Name, lines[i], err)
		}
		for _, r := range []struct {
			name         string
			small, large int
		}{{"Parse", parse80, parse160}, {"ParseAST", ast80, ast160}} {
			if r.small == 0 || r.large > r.small*5/2 {
				t.Errorf("%s %s: %d evaluations at 80, %d at 160", s.Name, r.name, r.small, r.large)
			}
		}
	}
}
