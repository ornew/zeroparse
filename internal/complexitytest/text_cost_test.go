package complexitytest

import (
	"os"
	"testing"

	"zeroparse.dev/zp"
)

// The replacement fields of Python f-strings take the text of a capture that holds the nested
// f-strings of the field, at every level of the nesting. The text of a capture costs a constant,
// so the classifier reports no cause for it and the families measure linear on every backend.
func TestPythonFieldTextIsLinear(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("measures the Python grammar")
	}
	src, err := os.ReadFile("../../testdata/grammars/python/python.zp")
	if err != nil {
		t.Fatal(err)
	}
	p, err := zp.CompileSource(string(src), "")
	if err != nil {
		t.Fatal(err)
	}
	families := []TriageFamily{
		{Name: "nested f-strings", Build: func(n int) string { return "x = " + nest("f'{", "1", "}'")(n) + "\n" }, Sizes: []int{24, 48, 96}},
		{Name: "many fields", Build: func(n int) string { return "x = f'" + rep("{a!r:>10}", n) + "'\n" }, Sizes: doubling(500, 3)},
	}
	for _, f := range families {
		var lengths []int
		for _, n := range f.Sizes {
			lengths = append(lengths, len(f.Build(n)))
		}
		for _, b := range Backends {
			s, err := Measure(p, f.Build, f.Sizes, b, triageCap)
			if err != nil {
				t.Fatalf("%s on %s: %v", f.Name, BackendName(b), err)
			}
			if got := ClassifyLengths(s, lengths); got != Linear {
				t.Errorf("%s on %s: measured %v (%v)", f.Name, BackendName(b), got, s.Costs)
			}
		}
	}
}
