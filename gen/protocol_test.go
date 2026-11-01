package gen_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"zeroparse.dev/zp"
	"zeroparse.dev/zp/gen"
	igen "zeroparse.dev/zp/internal/gen"
	"zeroparse.dev/zp/internal/gentest"
	"zeroparse.dev/zp/internal/ir/exec/core"
)

func TestMain(m *testing.M) {
	// The tests build generated programs; a build cache private to the test process keeps their
	// entries out of the shared cache.
	removeGoCache := gentest.SetGoCache()
	code := m.Run()
	removeGoCache()
	os.Exit(code)
}

// protocolName matches the declaration of the protocol constant of the runtime.
var protocolName = regexp.MustCompile(`\bconst Protocol([1-9][0-9]*) = ([1-9][0-9]*)\b`)

// TestImportProtocol builds a parser generated with RuntimeImport against the runtime module that
// the generator carries, which builds, and against the same runtime declaring the next protocol
// version instead, which must fail to build on the protocol constant the parser refers to.
func TestImportProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a generated parser")
	}
	g, err := zp.ParseGrammar(`@start
rule main = [a-z]+ "=" [0-9]+`)
	if err != nil {
		t.Fatal(err)
	}
	files, err := gen.Go{Module: "example.com/proto", Runtime: gen.RuntimeImport}.Generate(g, "main")
	if err != nil {
		t.Fatal(err)
	}
	runtimeFiles, err := gen.Go{Module: "example.com/proto"}.GenerateRuntime()
	if err != nil {
		t.Fatal(err)
	}
	var protocol string
	for _, f := range runtimeFiles {
		if m := protocolName.FindSubmatch(f.Content); m != nil {
			protocol = "Protocol" + string(m[1])
		}
	}
	if protocol == "" {
		t.Fatal("the runtime declares no protocol constant")
	}
	var parser []byte
	for _, f := range files {
		if f.Path == "parser.go" {
			parser = f.Content
		}
	}
	if want := "const _ = rt." + protocol + "\n"; !bytes.Contains(parser, []byte(want)) {
		t.Fatalf("parser.go does not refer to the protocol with %q", want)
	}
	build := func(bump bool) (string, error) {
		dir := t.TempDir()
		write := func(rel string, data []byte) {
			t.Helper()
			path := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, f := range files {
			write("p/"+f.Path, f.Content)
		}
		write("rt/go.mod", []byte("module zeroparse.dev/zp/runtime\n\ngo 1.27.1\n"))
		for _, f := range runtimeFiles {
			data := f.Content
			if bump {
				data = protocolName.ReplaceAllFunc(data, func(m []byte) []byte {
					n, _ := strconv.Atoi(string(protocolName.FindSubmatch(m)[1]))
					return []byte("const Protocol" + strconv.Itoa(n+1) + " = " + strconv.Itoa(n+1))
				})
			}
			write("rt/"+strings.TrimPrefix(f.Path, "internal/runtime/"), data)
		}
		write("p/go.mod", []byte("module example.com/proto\n\ngo 1.27.1\n\nrequire zeroparse.dev/zp/runtime v0.0.0\n\nreplace zeroparse.dev/zp/runtime => ../rt\n"))
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = filepath.Join(dir, "p")
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS="+core.GoFlags("-mod=mod -trimpath"), "GOWORK=off")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := build(false); err != nil {
		t.Fatalf("the parser does not build against the runtime of its generator: %v\n%s", err, out)
	}
	out, err := build(true)
	if err == nil {
		t.Fatal("the parser builds against a runtime that does not implement its protocol")
	}
	if !strings.Contains(out, "undefined: rt."+protocol) {
		t.Errorf("the build against another protocol fails for another reason than the protocol constant:\n%s", out)
	}
}

// goGenerated matches the line that marks a generated Go file.
var goGenerated = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// TestGeneratedHeader checks that every generated Go and TypeScript file starts with the header that
// names the generator version, and that generating twice gives the same files.
func TestGeneratedHeader(t *testing.T) {
	g, err := zp.ParseGrammar(`type Pair = { Key node[string]; Value node[string] }
@start
rule main Pair = k:[a-z]+ #atomic "=" v:[0-9]+ #atomic => Pair{Key: $k, Value: $v}`)
	if err != nil {
		t.Fatal(err)
	}
	header := igen.Header()
	if !goGenerated.MatchString(header) || !strings.Contains(header, " zp "+igen.Version()+".") {
		t.Fatalf("header %q", header)
	}
	for _, generator := range []gen.Generator{
		gen.Go{Module: "example.com/h", Types: true},
		gen.Go{Module: "example.com/h", Types: true, Runtime: gen.RuntimeImport},
		gen.TypeScript{Types: true},
		gen.TypeScript{Types: true, Runtime: gen.RuntimeImport, RuntimeModule: "./rt.ts"},
	} {
		first, err := generator.Generate(g, "main")
		if err != nil {
			t.Fatal(err)
		}
		second, err := generator.Generate(g, "main")
		if err != nil {
			t.Fatal(err)
		}
		if len(first) != len(second) {
			t.Fatalf("%#v: %d files, then %d", generator, len(first), len(second))
		}
		for i, f := range first {
			if line, _, _ := strings.Cut(string(f.Content), "\n"); line != header {
				t.Errorf("%#v: %s starts with %q, want %q", generator, f.Path, line, header)
			}
			if f.Path != second[i].Path || !bytes.Equal(f.Content, second[i].Content) {
				t.Errorf("%#v: %s differs between two generations", generator, f.Path)
			}
		}
	}
}
