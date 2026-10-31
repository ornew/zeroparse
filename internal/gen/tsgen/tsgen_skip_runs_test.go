package tsgen_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ornew/zeroparse/internal/complexitytest"
	"github.com/ornew/zeroparse/internal/gen"
	"github.com/ornew/zeroparse/internal/gentest"
)

// TestSkipRunsTSMatchEngine checks that generated TypeScript parsers, whose recovery skips record
// and reuse the walks of their skip-run sites, give the engine's results on the skip-run grammars.
func TestSkipRunsTSMatchEngine(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs generated TypeScript")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	gentest.TSParsersCorpus(t, node, gentest.SkipRunCases(60), gen.GenOptions{})
}

// tsSkipRunMutations are the changes of the runtime of a generated parser for the negative
// controls: walks that never record, and records stored when their iteration ends that the end of
// the walk leaves as they are.
var tsSkipRunMutations = map[string][][2]string{
	"never record": {{
		"if (!this.skipRecording && this.skipSlots !== undefined && this.skipSlots.marked(this.pos)) {",
		"if (false) {",
	}},
	"early store": {{
		"    v.exp = this.keep(from, to);\n",
		"    v.exp = this.keep(from, to);\n    this.skipSlot(v.pos, { end: this.pos, far, exp: v.exp });\n",
	}, {
		"    this.skipClose();\n    const vs = this.skipVisits;\n",
		"    this.skipClose();\n    this.skipVisits.length = 0;\n    this.skipHitRec = null;\n    if (this.pos >= 0) return;\n    const vs = this.skipVisits;\n",
	}},
}

// tsSkipRunSpends builds the sources as TypeScript parsers with the runtime changed by mutation
// (none if "") and returns, for each source, the least budget with which each input is recognized,
// and the results of the parses of the inputs of results with the last source. The spend of a
// parse also counts the text of each recovered error node, which the walks do not change.
func tsSkipRunSpends(t *testing.T, node string, sources []string, inputs [][]string, results []string, mutation string) (spends [][]uint64, out []string) {
	t.Helper()
	dir := t.TempDir()
	var imports, entries strings.Builder
	for i, src := range sources {
		g, err := parseTest(src)
		if err != nil {
			t.Fatal(err)
		}
		files, err := gentest.GenerateTS(g, gen.GenOptions{Start: "main", BudgetControls: true, Recognize: true})
		if err != nil {
			t.Fatal(err)
		}
		for k, f := range files {
			if f.Path == "runtime.ts" && mutation != "" {
				content := string(f.Content)
				for _, r := range tsSkipRunMutations[mutation] {
					if !strings.Contains(content, r[0]) {
						t.Fatalf("mutation %s: %q not in the runtime", mutation, r[0])
					}
					content = strings.Replace(content, r[0], r[1], 1)
				}
				files[k].Content = []byte(content)
			}
		}
		writeTSFiles(t, filepath.Join(dir, fmt.Sprintf("g%d", i)), files)
		in, _ := json.Marshal(inputs[i])
		fmt.Fprintf(&imports, "import * as g%d from \"./g%d/parser.ts\";\n", i, i)
		fmt.Fprintf(&entries, "  { inputs: %s, recognize: (s: string, budget: number) => { const e = g%[2]d.recognizeWithOptions(s, { budget }); return e instanceof g%[2]d.ParseInterruptionError && e.kind === \"budget-limit\"; }, parse: (s: string) => { const r = g%[2]d.parseWithOptions(s, {}); return JSON.stringify(r.node) + \" \" + String(r.error); } },\n", in, i)
	}
	res, _ := json.Marshal(results)
	consumer := imports.String() + `
const entries = [
` + entries.String() + `];
const lines: string[] = [];
for (const e of entries) {
  if (e.inputs.length === 0) continue;
  const row: number[] = [];
  for (const s of e.inputs) {
    let lo = 1, hi = 1;
    while (e.recognize(s, hi)) { lo = hi + 1; hi *= 2; }
    while (lo < hi) {
      const mid = Math.floor((lo + hi) / 2);
      if (e.recognize(s, mid)) lo = mid + 1; else hi = mid;
    }
    row.push(lo);
  }
  lines.push(row.join(" "));
}
for (const s of ` + string(res) + `) {
  lines.push(entries[entries.length - 1]!.parse(s));
}
console.log(lines.join("\n"));
`
	if err := os.WriteFile(filepath.Join(dir, "consumer.ts"), []byte(consumer), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"type":"module"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, gentest.NodeTSArgs("consumer.ts")...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	n := 0
	for _, in := range inputs {
		if len(in) > 0 {
			n++
		}
	}
	for _, l := range lines[:n] {
		var row []uint64
		for _, f := range strings.Fields(l) {
			var v uint64
			if _, err := fmt.Sscan(f, &v); err != nil {
				t.Fatalf("malformed spend line %q", l)
			}
			row = append(row, v)
		}
		spends = append(spends, row)
	}
	return spends, lines[n:]
}

// TestSkipRunsTSLinear checks that the E8 controls (repeated, nested and alternating walks) spend a
// budget linear in the input in generated TypeScript recognizers, that walks that never record make
// them quadratic, and that records stored before the walk ends change the results of nested
// inputs.
func TestSkipRunsTSLinear(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs generated TypeScript")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	var sources []string
	var inputs [][]string
	sizes := []int{200, 400, 800}
	var names []string
	for _, c := range complexitytest.Controls() {
		if c.Family != "E8" {
			continue
		}
		var in []string
		for _, n := range sizes {
			in = append(in, c.Input(n))
		}
		sources, inputs, names = append(sources, c.Source), append(inputs, in), append(names, c.Name)
	}
	if len(sources) != 3 {
		t.Fatalf("%d E8 controls, want 3", len(sources))
	}
	for _, g := range gentest.SkipRunGrammars {
		if g.Name == "nested recovery" {
			sources, inputs = append(sources, g.Src), append(inputs, []string{})
		}
	}
	nested := []string{"((((((a(", "((((a", "((a((a(((a"}
	slopes := func(row []uint64) (lo, hi float64) {
		lo, hi = math.Inf(1), math.Inf(-1)
		for i := 1; i < len(row); i++ {
			s := math.Log2(float64(row[i]) / float64(row[i-1]))
			lo, hi = min(lo, s), max(hi, s)
		}
		return lo, hi
	}
	spends, results := tsSkipRunSpends(t, node, sources, inputs, nested, "")
	t.Logf("spends at %v: %v", sizes, spends)
	for i, row := range spends {
		if _, hi := slopes(row); hi > 1.2 {
			t.Errorf("%s: spends %v grow faster than linearly", names[i], row)
		}
	}
	spends, _ = tsSkipRunSpends(t, node, sources, inputs, nested, "never record")
	t.Logf("spends of walks that never record: %v", spends)
	for i, row := range spends {
		if lo, _ := slopes(row); lo < 1.7 {
			t.Errorf("%s, walks that never record: spends %v are not quadratic", names[i], row)
		}
	}
	_, early := tsSkipRunSpends(t, node, sources, inputs, nested, "early store")
	differ := false
	for i := range results {
		differ = differ || results[i] != early[i]
	}
	if !differ {
		t.Errorf("records stored before the walk ends change no result of %q", nested)
	}
}
