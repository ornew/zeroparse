package core

import (
	"strings"
	"testing"
	"unsafe"

	"zeroparse.dev/zp/internal/limits"
)

// A code-point source too large for int32 offsets still gives text without copying: the first
// text builds a table of int offsets over the source itself (or over the decoded input when the
// source holds invalid UTF-8).
func TestTextOfSourceBeyondInt32Offsets(t *testing.T) {
	old := offsLimit
	offsLimit = 64
	defer func() { offsLimit = old }()
	valid := strings.Repeat("aé世😀", 50)
	invalid := strings.Repeat("aé\xff😀", 50)
	type maker struct {
		name string
		make func(s string) *Input
	}
	makers := []maker{
		{"NewTextInput", func(s string) *Input { in := NewTextInput(s, CodePoints, true); return &in }},
		{"NewTextInput/lazy", func(s string) *Input { in := NewTextInput(s, CodePoints, false); return &in }},
		{"setSource", func(s string) *Input {
			in := NewTextInput("x", CodePoints, false)
			in.setSource(s)
			return &in
		}},
	}
	if BudgetControlsEnabled {
		makers = append(makers, maker{"controlled", func(s string) *Input {
			c := NewBudgetControl(nil, limits.Limits{Budget: 1 << 30})
			in := NewControlledTextInput(s, CodePoints, true, nil, c)
			return &in
		}})
	}
	for _, m := range makers {
		for _, tc := range []struct {
			name, source string
			shares       bool
		}{{"valid", valid, true}, {"invalid", invalid, false}} {
			in := m.make(tc.source)
			if in.SrcOK || in.Offs != nil {
				t.Fatalf("%s/%s: int32 table above the limit", m.name, tc.name)
			}
			if m.name == "setSource" {
				in.In = []rune(strings.ToValidUTF8(tc.source, "�"))
			}
			want := []rune(strings.ToValidUTF8(tc.source, "�"))
			if len(in.In) != len(want) {
				t.Fatalf("%s/%s: %d code points, want %d", m.name, tc.name, len(in.In), len(want))
			}
			for _, r := range [][2]int{{0, len(want)}, {3, 40}, {10, 10}, {len(want) - 1, len(want)}} {
				if got := in.Text(r[0], r[1]); got != string(want[r[0]:r[1]]) {
					t.Fatalf("%s/%s: text%v = %q", m.name, tc.name, r, got)
				}
			}
			if in.decWide == nil || in.decOffs != nil || len(in.decWide) != len(want)+1 {
				t.Fatalf("%s/%s: wide table not built", m.name, tc.name)
			}
			if tc.shares {
				got := in.Text(1, len(want)-1)
				if unsafe.StringData(got) != unsafe.StringData(tc.source[len("a"):]) {
					t.Fatalf("%s: text copied", m.name)
				}
			}
			if n := testing.AllocsPerRun(10, func() { _ = in.Text(2, len(want)-2) }); n != 0 {
				t.Fatalf("%s/%s: text allocates %v", m.name, tc.name, n)
			}
		}
	}
}
