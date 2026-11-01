package runtime

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var updateProtocol = flag.Bool("update-protocol", false, "rewrite testdata/protocol.golden with the current protocol interface")

// protocolGolden records the protocol version of the runtime and the interface it names.
const protocolGolden = "testdata/protocol.golden"

// protocolConst matches the names of the constants that declare the implemented protocol versions.
var protocolConst = regexp.MustCompile(`^Protocol([1-9][0-9]*)$`)

// TestProtocol fails when the interface between generated code and the runtime (the exported
// declarations of the package, but the protocol constants) differs from the one recorded for the
// current protocol version, so that a change of the interface comes with a new protocol version.
// After replacing ProtocolN with ProtocolN+1 (protocol.go), go test -run TestProtocol
// -update-protocol records the new interface; it refuses to record a changed interface under the
// same version.
func TestProtocol(t *testing.T) {
	version, surface := protocolSurface(t)
	got := fmt.Sprintf("protocol %d\n%s", version, surface)
	data, err := os.ReadFile(protocolGolden)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	wantVersion, wantSurface := parseProtocolGolden(t, string(data))
	if got == string(data) {
		return
	}
	changed := surface != wantSurface
	if *updateProtocol {
		if changed && version == wantVersion {
			t.Fatalf("the protocol interface changed but the version is still %d: replace Protocol%d with Protocol%d in protocol.go first", version, version, version+1)
		}
		if err := os.WriteFile(protocolGolden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if changed && version == wantVersion {
		t.Fatalf("the interface that generated code uses changed (%s) without a new protocol version: replace Protocol%d with Protocol%d in protocol.go, then run go test -run TestProtocol -update-protocol\n%s",
			protocolGolden, version, version+1, surfaceDiff(wantSurface, surface))
	}
	t.Fatalf("%s records protocol %d, the runtime declares protocol %d: run go test -run TestProtocol -update-protocol\n%s",
		protocolGolden, wantVersion, version, surfaceDiff(wantSurface, surface))
}

func parseProtocolGolden(t *testing.T, data string) (int, string) {
	if data == "" {
		return 0, ""
	}
	first, rest, _ := strings.Cut(data, "\n")
	v, err := strconv.Atoi(strings.TrimPrefix(first, "protocol "))
	if err != nil {
		t.Fatalf("%s: the first line must be \"protocol N\": %q", protocolGolden, first)
	}
	return v, rest
}

// surfaceDiff lists the lines of the interface that only one of want and got has.
func surfaceDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	var b strings.Builder
	for _, l := range w {
		if !slices.Contains(g, l) {
			fmt.Fprintf(&b, "-%s\n", l)
		}
	}
	for _, l := range g {
		if !slices.Contains(w, l) {
			fmt.Fprintf(&b, "+%s\n", l)
		}
	}
	return b.String()
}

// protocolSurface returns the highest protocol version the package declares and its interface, one
// declaration per line. The files of every build configuration of the package must give the same
// interface.
func protocolSurface(t *testing.T) (int, string) {
	var version int
	var surface string
	for i, tags := range [][]string{nil, {"zp_exec"}} {
		v, s := packageSurface(t, ".", tags)
		if i > 0 && (v != version || s != surface) {
			t.Fatalf("the files selected with tags %q declare another interface than the default files:\n%s", tags, surfaceDiff(surface, s))
		}
		version, surface = v, s
	}
	if version == 0 {
		t.Fatal("the package declares no protocol constant")
	}
	return version, surface
}

// packageSurface type-checks the package in dir with the build tags and returns the highest
// protocol version it declares and its interface: every exported declaration of the package scope
// (but the protocol constants), and every type of the package that the interface reaches, exported
// or not (a generated parser can use the exported fields and methods of an unexported type that it
// reaches through an exported field), with its definition, exported fields and exported methods,
// and the alias declarations it reaches with what they stand for.
func packageSurface(t *testing.T, dir string, tags []string) (int, string) {
	ctx := build.Default
	ctx.BuildTags = tags
	bp, err := ctx.ImportDir(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(bp.Dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("zeroparse.dev/zp/runtime", fset, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &surface{pkg: pkg, lines: map[string]bool{}, seen: map[types.Object]bool{}}
	version := 0
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		if m := protocolConst.FindStringSubmatch(name); m != nil {
			if _, ok := obj.(*types.Const); ok {
				n, _ := strconv.Atoi(m[1])
				version = max(version, n)
				continue
			}
		}
		switch obj := obj.(type) {
		case *types.Const:
			s.add(fmt.Sprintf("const %s %s = %s", name, s.typ(obj.Type()), obj.Val().ExactString()))
			s.walk(obj.Type())
		case *types.Var:
			s.add(fmt.Sprintf("var %s %s", name, s.typ(obj.Type())))
			s.walk(obj.Type())
		case *types.Func:
			s.add(fmt.Sprintf("func %s%s", name, s.signature(obj.Signature())))
			s.walk(obj.Signature())
		case *types.TypeName:
			s.typeName(obj)
		}
	}
	lines := make([]string, 0, len(s.lines))
	for l := range s.lines {
		lines = append(lines, l)
	}
	slices.Sort(lines)
	return version, strings.Join(lines, "\n") + "\n"
}

// surface collects the lines of an interface.
type surface struct {
	pkg   *types.Package
	lines map[string]bool
	seen  map[types.Object]bool
}

func (s *surface) add(line string) { s.lines[line] = true }

// typeName records a type or alias declaration of the package and walks what it reaches.
func (s *surface) typeName(obj *types.TypeName) {
	if obj.Pkg() != s.pkg || s.seen[obj] {
		return
	}
	s.seen[obj] = true
	if obj.IsAlias() {
		alias := obj.Type().(*types.Alias)
		s.add(fmt.Sprintf("type %s = %s", obj.Name(), s.typ(alias.Rhs())))
		s.walk(alias.Rhs())
		return
	}
	named := obj.Type().(*types.Named)
	tparams := s.typeParams(named.TypeParams())
	if st, ok := named.Underlying().(*types.Struct); ok {
		var fs []string
		for i := range st.NumFields() {
			f := st.Field(i)
			if !f.Exported() {
				continue
			}
			if f.Embedded() {
				fs = append(fs, s.typ(f.Type()))
			} else {
				fs = append(fs, f.Name()+" "+s.typ(f.Type()))
			}
			s.walk(f.Type())
		}
		s.add(fmt.Sprintf("type %s%s struct{%s}", obj.Name(), tparams, strings.Join(fs, "; ")))
	} else {
		s.add(fmt.Sprintf("type %s%s %s", obj.Name(), tparams, s.typ(named.Underlying())))
		s.walk(named.Underlying())
	}
	for i := range named.NumMethods() {
		m := named.Method(i)
		if !m.Exported() {
			continue
		}
		recv := obj.Name()
		if _, ok := m.Signature().Recv().Type().(*types.Pointer); ok {
			recv = "*" + recv
		}
		s.add(fmt.Sprintf("method (%s) %s%s", recv, m.Name(), s.signature(m.Signature())))
		s.walk(m.Signature())
	}
}

// walk records the types of the package that t reaches.
func (s *surface) walk(t types.Type) {
	switch t := t.(type) {
	case *types.Alias:
		s.typeName(t.Obj())
		for i := range t.TypeArgs().Len() {
			s.walk(t.TypeArgs().At(i))
		}
	case *types.Named:
		s.typeName(t.Origin().Obj())
		for i := range t.TypeArgs().Len() {
			s.walk(t.TypeArgs().At(i))
		}
	case *types.Pointer:
		s.walk(t.Elem())
	case *types.Slice:
		s.walk(t.Elem())
	case *types.Array:
		s.walk(t.Elem())
	case *types.Chan:
		s.walk(t.Elem())
	case *types.Map:
		s.walk(t.Key())
		s.walk(t.Elem())
	case *types.Signature:
		for _, tp := range []*types.Tuple{t.Params(), t.Results()} {
			for i := range tp.Len() {
				s.walk(tp.At(i).Type())
			}
		}
		for i := range t.TypeParams().Len() {
			s.walk(t.TypeParams().At(i).Constraint())
		}
	case *types.Struct:
		for i := range t.NumFields() {
			if f := t.Field(i); f.Exported() {
				s.walk(f.Type())
			}
		}
	case *types.Interface:
		for i := range t.NumExplicitMethods() {
			s.walk(t.ExplicitMethod(i).Type())
		}
		for i := range t.NumEmbeddeds() {
			s.walk(t.EmbeddedType(i))
		}
	case *types.Union:
		for i := range t.Len() {
			s.walk(t.Term(i).Type())
		}
	case *types.TypeParam:
		s.walk(t.Constraint())
	}
}

// typ writes t as source, without the names of parameters and results in function types, which do
// not concern callers, and with the types of the package unqualified.
func (s *surface) typ(t types.Type) string {
	switch t := t.(type) {
	case *types.Alias:
		return s.qualified(t.Obj()) + s.typeArgs(t.TypeArgs())
	case *types.Named:
		return s.qualified(t.Obj()) + s.typeArgs(t.TypeArgs())
	case *types.Pointer:
		return "*" + s.typ(t.Elem())
	case *types.Slice:
		return "[]" + s.typ(t.Elem())
	case *types.Array:
		return fmt.Sprintf("[%d]%s", t.Len(), s.typ(t.Elem()))
	case *types.Map:
		return "map[" + s.typ(t.Key()) + "]" + s.typ(t.Elem())
	case *types.Chan:
		prefix := map[types.ChanDir]string{types.SendRecv: "chan ", types.SendOnly: "chan<- ", types.RecvOnly: "<-chan "}[t.Dir()]
		return prefix + s.typ(t.Elem())
	case *types.Signature:
		return "func" + s.signature(t)
	case *types.Struct:
		var fs []string
		for i := range t.NumFields() {
			f := t.Field(i)
			if f.Embedded() {
				fs = append(fs, s.typ(f.Type()))
			} else {
				fs = append(fs, f.Name()+" "+s.typ(f.Type()))
			}
		}
		return "struct{" + strings.Join(fs, "; ") + "}"
	case *types.Interface:
		var ms []string
		for i := range t.NumEmbeddeds() {
			ms = append(ms, s.typ(t.EmbeddedType(i)))
		}
		for i := range t.NumExplicitMethods() {
			m := t.ExplicitMethod(i)
			ms = append(ms, m.Name()+s.signature(m.Signature()))
		}
		return "interface{" + strings.Join(ms, "; ") + "}"
	case *types.Union:
		var ts []string
		for i := range t.Len() {
			term := t.Term(i)
			p := ""
			if term.Tilde() {
				p = "~"
			}
			ts = append(ts, p+s.typ(term.Type()))
		}
		return strings.Join(ts, " | ")
	default:
		return types.TypeString(t, s.qualifier)
	}
}

func (s *surface) qualifier(p *types.Package) string {
	if p == s.pkg {
		return ""
	}
	return p.Path()
}

func (s *surface) qualified(obj *types.TypeName) string {
	if obj.Pkg() == nil || obj.Pkg() == s.pkg {
		return obj.Name()
	}
	return obj.Pkg().Path() + "." + obj.Name()
}

func (s *surface) typeArgs(args *types.TypeList) string {
	if args.Len() == 0 {
		return ""
	}
	var ts []string
	for i := range args.Len() {
		ts = append(ts, s.typ(args.At(i)))
	}
	return "[" + strings.Join(ts, ", ") + "]"
}

func (s *surface) typeParams(tp *types.TypeParamList) string {
	if tp.Len() == 0 {
		return ""
	}
	var ps []string
	for i := range tp.Len() {
		p := tp.At(i)
		ps = append(ps, p.Obj().Name()+" "+s.typ(p.Constraint()))
	}
	return "[" + strings.Join(ps, ", ") + "]"
}

// signature writes the type parameters, parameters and results of sig without their names.
func (s *surface) signature(sig *types.Signature) string {
	tuple := func(tp *types.Tuple, variadic bool) string {
		var ts []string
		for i := range tp.Len() {
			if variadic && i == tp.Len()-1 {
				ts = append(ts, "..."+s.typ(tp.At(i).Type().(*types.Slice).Elem()))
				continue
			}
			ts = append(ts, s.typ(tp.At(i).Type()))
		}
		return strings.Join(ts, ", ")
	}
	out := s.typeParams(sig.TypeParams()) + "(" + tuple(sig.Params(), sig.Variadic()) + ")"
	switch r := sig.Results(); r.Len() {
	case 0:
	case 1:
		out += " " + tuple(r, false)
	default:
		out += " (" + tuple(r, false) + ")"
	}
	return out
}

// TestProtocolSurfaceSeesReachedTypes is the negative control of TestProtocol: renaming an exported
// method of an unexported type that generated code reaches through an exported field
// ((*nodeKind).NodeType, through Node.RawKind) changes the interface.
func TestProtocolSurfaceSeesReachedTypes(t *testing.T) {
	_, want := packageSurface(t, ".", nil)
	for _, line := range []string{"type nodeKind struct{Base int}", "method (*nodeKind) NodeType() bool"} {
		if !strings.Contains(want, line+"\n") {
			t.Errorf("the interface lacks %q", line)
		}
	}
	dir := t.TempDir()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	rename := regexp.MustCompile(`\bNodeType\(\)`)
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		data, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), rename.ReplaceAll(data, []byte("IsNodeType()")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, got := packageSurface(t, dir, nil)
	if !strings.Contains(surfaceDiff(want, got), "nodeKind) IsNodeType") {
		t.Errorf("renaming (*nodeKind).NodeType does not change the interface:\n%s", surfaceDiff(want, got))
	}
}
