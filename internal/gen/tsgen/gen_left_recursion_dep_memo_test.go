package tsgen_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
)

// depMemoTSCases returns the grammars of gentest.DependencyMemoCases for seeds below seeds that
// generate TypeScript parsers.
func depMemoTSCases(t *testing.T, seeds int) []gentest.Case {
	t.Helper()
	var cases []gentest.Case
	for _, c := range gentest.DependencyMemoCases(seeds) {
		g, err := parseTest(c.Src)
		if err != nil {
			continue
		}
		if _, err := gentest.GenerateTSBundle(g, gen.GenOptions{Start: "main", Recognize: true}); err != nil {
			continue
		}
		cases = append(cases, c)
	}
	if len(cases) < seeds*2 {
		t.Fatalf("only %d grammars of %d seeds generate", len(cases), seeds)
	}
	return cases
}

// TestGeneratedTSLeftRecursionDependencyMemo checks that generated TypeScript parsers give the
// engine's results for grammars whose leaders grow inside the growth of other leaders of their
// cycle at one position, where the dependency memo reuses results by the seeds they read and the
// lookaheads they observed; with source information, the values also tell apart step results that
// differ only in the rules that produced them.
func TestGeneratedTSLeftRecursionDependencyMemo(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs generated TypeScript")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	gentest.TSParsersCorpus(t, node, depMemoTSCases(t, 60), gen.GenOptions{})
	gentest.TSParsersCorpus(t, node, depMemoTSCases(t, 20), gen.GenOptions{SourceInfo: true})
}

// depMemoTSMutations are the changes of the TypeScript runtime for the negative controls of
// TestGeneratedTSLeftRecursionDependencyMemoControls, each a list of replacements: a reuse that
// does not compare the identities of the seeds read, that does not check observations, a result in
// the memo reused whatever its present set, equal step results given the same identity without
// comparing them, a jump that uses observations whose outcomes read the growing seed, a reused
// result that replays the seeds its observations read where it was made, one that passes on only
// those seeds of its observations with current identities, and an observation that summarizes a
// lookahead that recovered errors.
var depMemoTSMutations = map[string][][2]string{
	"seed identities": {
		{"k += this.gamma[i]!.id + \",\";\n", "k += \"0,\";\n"},
		{"if (d.kind === depSeed) k += d.val + \",\";\n", "if (d.kind === depSeed) k += \"0,\";\n"},
	},
	"observations": {
		{"const v = this.obsValue(pos, gp, present, o);\n        if (v === obsNone) {\n", "const v = o * 0 - 2;\n        if (v === obsNone) {\n"},
		{"if (d.kind === depObs) k += d.val + \",\";\n", "if (d.kind === depObs) k += \"-2,\";\n"},
		{"if (o.v !== d.val || !o.free) {", "if (false) {"},
	},
	"present growths": {
		{"return !c.ctx || c.known && (c.present & ~present) === 0;\n", "return true;\n"},
	},
	"canonical seeds": {
		{"if (c.e.errs.length === 0 && this.nodesEqual(", "if (true || this.nodesEqual("},
	},
	"jumps over observations of the growing seed": {
		{"if (o.v !== d.val || !o.free) {", "if (o.v !== d.val) {"},
	},
	"replayed observations": {
		{"buf.push({ kind: d.kind, idx: i, d: depth, val: this.gamma[i]!.id });", "buf.push({ kind: d.kind, idx: i, d: depth, val: d.val });"},
		{"this.replayObs(buf, pos, gp, present, d.site, depth);\n", "\n"},
	},
	"observations replayed in the current context": {
		{"this.replayObs(buf, pos, gp, present, d.site, depth);\n", "\n"},
	},
	"lookaheads with recovered errors": {
		{"if (!read || (ok && this.recovered.length > rec)) {", "if (!read) {"},
	},
}

// TestGeneratedTSLeftRecursionDependencyMemoControls checks that the grammars of
// TestGeneratedTSLeftRecursionDependencyMemo detect, in generated TypeScript parsers, each check of
// the dependency memo: with the check broken, some result differs from those of the runtime as it
// is.
func TestGeneratedTSLeftRecursionDependencyMemoControls(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs generated TypeScript")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	cases := depMemoTSCases(t, 120)
	opts := gen.GenOptions{SourceInfo: true}
	want := gentest.TSParsersOutputs(t, node, cases, opts, nil)
	for name, reps := range depMemoTSMutations {
		got := gentest.TSParsersOutputs(t, node, cases, opts, func(src string) string {
			for _, r := range reps {
				if strings.Count(src, r[0]) != 1 {
					t.Fatalf("mutation %s: %q is not in the runtime once", name, r[0])
				}
				src = strings.Replace(src, r[0], r[1], 1)
			}
			return src
		})
		if got == want {
			t.Errorf("%s: the mutation went undetected", name)
		}
	}
}

// TestGeneratedTSLeftRecursionDependencyMemoLinear checks that generated TypeScript parsers of
// gentest.DependencyMemoShapes evaluate a number of calls linear in the length of the input:
// doubling the input at most a little more than doubles the evaluations. Generated with budget
// controls, they parse an input of length 10,000 within the default budget, which grows linearly
// with the input and counts work that evaluations do not show, such as the scan of the jumps
// recorded for a seed.
func TestGeneratedTSLeftRecursionDependencyMemoLinear(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs generated TypeScript")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	dir := t.TempDir()
	write := func(name string, content []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", []byte(`{"type":"module"}`))
	var main strings.Builder
	for i, s := range gentest.DependencyMemoShapes {
		code, err := gentest.GenerateTSBundle(mustParse("@start\nrule main = "+s.Rules), gen.GenOptions{Start: "main", Observability: true})
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		write(fmt.Sprintf("g%d.ts", i), code)
		fmt.Fprintf(&main, "import * as g%d from \"./g%[1]d.ts\";\n", i)
		code, err = gentest.GenerateTSBundle(mustParse("@start\nrule main = "+s.Rules), gen.GenOptions{Start: "main", BudgetControls: true})
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		write(fmt.Sprintf("c%d.ts", i), code)
		fmt.Fprintf(&main, "import * as c%d from \"./c%[1]d.ts\";\n", i)
	}
	main.WriteString("const evals = (m, input) => {\n" +
		"  const profile = new m.Profile();\n" +
		"  const r = m.parseWithOptions(input, { profile });\n" +
		"  if (r.error !== null) throw r.error;\n" +
		"  return profile.snapshot().evals;\n" +
		"};\n" +
		"const withinBudget = (m, input) => m.parseWithOptions(input, {}).error === null;\n")
	for i, s := range gentest.DependencyMemoShapes {
		fmt.Fprintf(&main, "console.log(evals(g%[1]d, %[2]s), evals(g%[1]d, %[3]s), withinBudget(c%[1]d, %[4]s));\n", i, strconv.Quote(s.Input(80)), strconv.Quote(s.Input(160)), strconv.Quote(s.Input(10000)))
	}
	write("main.mjs", []byte(main.String()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "main.mjs")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(gentest.DependencyMemoShapes) {
		t.Fatalf("got %d lines, want %d\n%s", len(lines), len(gentest.DependencyMemoShapes), out)
	}
	for i, s := range gentest.DependencyMemoShapes {
		var small, large int
		var long bool
		if _, err := fmt.Sscan(lines[i], &small, &large, &long); err != nil {
			t.Fatalf("%s: %q: %v", s.Name, lines[i], err)
		}
		if !long {
			t.Errorf("%s: no parse at 10000 within the default budget", s.Name)
		}
		if small == 0 || large > small*5/2 {
			t.Errorf("%s: %d evaluations at 80, %d at 160", s.Name, small, large)
		}
	}
}
