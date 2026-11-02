package tsrt

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMemoIndex runs the TypeScript runtime's memo table against a plain map, and checks that the
// budget of a lookup in a long chain does not depend on the length of the chain.
func TestMemoIndex(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	source, err := os.ReadFile("runtime.ts")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "memo_index.ts")
	if err := os.WriteFile(path, append(source, []byte(memoIndexTypeScriptTest)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(node, path).CombinedOutput(); err != nil {
		t.Fatalf("node: %v\n%s", err, output)
	}
}

const memoIndexTypeScriptTest = `

function parser(budget: number): Parser {
  // A counter in place of the budget control: only the units charged matter here.
  const control = budget === 0 ? null : ({ used: 0, charge(n: number) { this.used += n; }, check() {} } as any);
  const p = new Parser(mkGrammar(100_000, true, false), "abcde", CodePoints, 1, 100);
  (p as any).control = control; // after construction, which needs the budget helpers for a real control
  return p;
}
function entry(): MemoEntry {
  return { node: null, errs: [], expected: [], next: null, env: null, end: 0, far: 0, rule: 0, min: 0, ok: true } as any;
}
function keyName(rule: number, pos: number, min: number, env: Value[] | null): string {
  return [rule, pos, min, (env ?? []).map((v) => typeof v + ":" + String(v) + (Object.is(v, -0) ? "" : "")).join("|")].join("/");
}

// Random lookups and stores against a map. The keys cluster on few positions, so the chains pass
// memoScan and the index is built, extended and used for replacement.
for (const budget of [0, 2 ** 40]) {
  const p = parser(budget);
  const envs: (Value[] | null)[] = [null, [1], [0], [-0], [1n], ["a"], ["ab"], [true], [false], [null], [1.5], [{} as any]];
  for (let i = 0; i < 40; i++) envs.push([i, "s" + (i % 7)]);
  const want = new Map<string, MemoEntry>();
  let seed = 1;
  const rnd = (n: number): number => { seed = (Math.imul(seed, 1103515245) + 12345) >>> 0; return (seed >>> 8) % n; };
  for (let step = 0; step < 20000; step++) {
    const env = envs[rnd(envs.length)]!;
    const rule = rnd(4), pos = rnd(5), min = rnd(2);
    // Distinct objects are distinct keys, and -0 equals 0, as with ===.
    const name = keyName(rule, pos, min, env) + (env !== null && typeof env[0] === "object" && env[0] !== null ? "#" + (envs.indexOf(env)) : "");
    const wk = name.replace("number:-0", "number:0");
    const got = p.memoGet(rule, pos, min, env);
    const w = want.get(wk) ?? null;
    if (got !== w) throw new Error("step " + step + ": get " + wk + " differs");
    if (rnd(2) === 0) {
      const e = entry();
      p.memoPut(rule, pos, min, env, e);
      want.set(wk, e);
    }
  }
  for (let pos = 0; pos < 5; pos++) {
    let n = 0;
    for (let e = p.slots[pos] || null; e; e = e.next) n++;
    let m = 0;
    for (const k of want.keys()) if (k.split("/")[1] === String(pos)) m++;
    if (n !== m) throw new Error("budget " + budget + " position " + pos + ": chain has " + n + " entries, map " + m);
  }
}

// The cost of a lookup does not grow with the chain.
function cost(entries: number): number {
  const p = parser(2 ** 40);
  for (let i = 0; i < entries; i++) p.memoPut(1, 0, 0, [i], entry());
  p.memoGet(1, 0, 0, [-1]); // builds the index
  const control = p.control as any;
  const before = control.used;
  for (let i = 0; i < 50; i++) p.memoGet(1, 0, 0, [0]);
  return control.used - before;
}
const small = cost(100), large = cost(5000);
if (small !== large) throw new Error("lookup cost depends on the chain: " + small + " with 100 entries, " + large + " with 5000");

// Storing n entries under n keys costs linear units in n, not quadratic.
function fill(entries: number): number {
  const p = parser(2 ** 40);
  for (let i = 0; i < entries; i++) p.memoPut(1, 0, 0, [i], entry());
  return (p.control as any).used;
}
const f1 = fill(2000), f2 = fill(4000);
if (f2 > 2.5 * f1) throw new Error("storing 4000 entries costs " + f2 + " units, 2000 cost " + f1 + ": not linear");
`
