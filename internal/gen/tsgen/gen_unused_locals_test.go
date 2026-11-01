package tsgen_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
)

// tsFunctionDecl matches the declaration of a top-level function of generated code.
var tsFunctionDecl = regexp.MustCompile(`(?m)^function (\w+)\(`)

// unreferencedFunctions returns the top-level functions of the code that nothing else in it names.
func unreferencedFunctions(code string) []string {
	var out []string
	for _, m := range tsFunctionDecl.FindAllStringSubmatch(code, -1) {
		uses := regexp.MustCompile(`\b`+m[1]+`\b`).FindAllStringIndex(code, 2)
		if len(uses) < 2 {
			out = append(out, m[1])
		}
	}
	return out
}

// TestGeneratedTSCallsOnlyPrintedRules checks that a generated parser leaves out a rule that only
// calls missing from the printed code would reach: in random cycle grammar 120 with cuts, r4 calls
// r3 after !("c"?), which never matches, so the printed code of r4 does not call r3.
func TestGeneratedTSCallsOnlyPrintedRules(t *testing.T) {
	t.Parallel()
	c := gentest.RandomCycleGrammar(120, true)
	if !strings.Contains(c.Src, `rule r4 = !"c"? r3 / "b"`) {
		t.Fatalf("the grammar changed:\n%s", c.Src)
	}
	g, err := parseTest(c.Src)
	if err != nil {
		t.Fatal(err)
	}
	code, err := gentest.GenerateTSBundle(g, gen.GenOptions{Start: "main", Recognize: true})
	if err != nil {
		t.Fatal(err)
	}
	parser := string(code)
	if i := strings.Index(parser, "const G = mkGrammar("); i >= 0 {
		parser = parser[i:]
	}
	if fns := unreferencedFunctions(parser); len(fns) > 0 {
		t.Errorf("functions that nothing calls: %v", fns)
	}
	if strings.Contains(parser, `"r3", [`) {
		t.Errorf("r3 has a rule constant although nothing calls it")
	}
}

// TestGeneratedTSNoUnusedLocals compiles the generated parsers of the checked-in grammars and of
// the random cycle grammars with the strictest checks, among them --noUnusedLocals and
// --noUnusedParameters: a parser declares nothing that it does not use, so that it compiles in a
// project with those checks.
func TestGeneratedTSNoUnusedLocals(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("runs tsc")
	}
	tsc, err := exec.LookPath("tsc")
	if err != nil {
		t.Skip("tsc not found")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"type": "module"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var files []string
	write := func(name string, code []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), code, 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, name)
	}
	paths, err := filepath.Glob("../../../testdata/grammars/*/*.zp")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no grammars: %v", err)
	}
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		g, err := parseTest(string(src))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		// The files as generate writes them: a typed parser's types can have the names of runtime
		// declarations, which the joined module of GenerateTSBundle would declare twice.
		for i, opts := range []gen.GenOptions{{Recognize: true}, {Types: true}} {
			gfs, err := gentest.GenerateTS(g, opts)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			sub := fmt.Sprintf("%s_%d", strings.TrimSuffix(filepath.Base(path), ".zp"), i)
			writeTSFiles(t, filepath.Join(dir, sub), gfs)
			files = append(files, filepath.Join(sub, "parser.ts"))
		}
	}
	seeds := 200
	for s := int64(0); s < int64(seeds); s++ {
		for i, c := range []gentest.Case{gentest.RandomCycleGrammar(s, false), gentest.RandomCycleGrammar(s, true), gentest.RandomRecoveringCycleGrammar(s)} {
			g, err := parseTest(c.Src)
			if err != nil {
				continue
			}
			code, err := gentest.GenerateTSBundle(g, gen.GenOptions{Start: "main", Recognize: true})
			if err != nil {
				continue
			}
			write(fmt.Sprintf("s%d_%d.ts", s, i), code)
		}
	}
	if len(files) < seeds*2 {
		t.Fatalf("only %d parsers generate", len(files))
	}
	args := append(append([]string{"--noEmit", "--allowImportingTsExtensions"}, tsStrictFlags...), files...)
	cmd := exec.Command(tsc, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("tsc: %v\n%s", err, out)
	}
}
