# Rewrite a Grammar to Linear Cost

**Problem.** `zp lint -classify` reports that the cost of a parse grows faster than the input, and you want the
grammar to cost the same per character on a long input as on a short one. You need, for each cause the
[specification names](../../spec/runtime/parsing.md#classes-of-growth), a rewrite, its measured effect, and what it changes in the
parse.

Find the cause in the witness that the linter prints, find its row in the table, and apply the rewrite of that section.
Each section shows the grammar that grows, the grammar that replaces it, the cost of both, and what the replacement
changes. A rewrite that accepts the same language can still change the tree; the sections say how. One pattern has no
rewrite that keeps the result.

| Cause | Class reported | Rewrite | What changes |
|:--|:--|:--|:--|
| [A repetition restarted after backtracking](#a-repetition-restarted-after-backtracking) | polynomial | Scan once: put the scan and what may follow it in one alternative | The elements of the repetition |
| [A lookahead over a scan](#a-lookahead-over-a-scan) | polynomial | Check once after the scan instead of at every iteration | Where the repetition is split |
| [A recovery skip followed by failure](#a-recovery-skip-followed-by-failure) | polynomial | Write the repetition inside `skip=` | Nothing |
| [A left-recursive rule or an Operators Rule called again and again](#a-left-recursive-rule-or-an-operators-rule-called-again-and-again) | polynomial | Call the rule once at a position: factor the caller | The tree of runs the choice used to split |
| [Diverging variable bindings](#diverging-variable-bindings) | exponential | Bind a value that does not depend on the earlier value | What the variable holds |
| [Self-growing values](#self-growing-values) | exponential | Count instead of doubling | No rewrite keeps the value |

Every input in the sections is the character the section names, repeated `n` times, except where it says otherwise. The
cost is the number of steps that [budget limits](../../spec/runtime/parsing.md#cancellation-and-budget-limits) count,
measured on the closure backend; the bytecode backends and generated Go parsers give the same orders. The steps do not
depend on the machine.

## A Repetition Restarted After Backtracking

```zp
// scan.zp
@start
rule main = (a / "a")* $$

rule a = "a"* "c"
```

On `a` repeated `n` times, every iteration tries `a`, which scans to the end of the input, finds no `c` and fails; the
alternative `"a"` then takes one character and the next iteration scans again. The memo shares the result of `a` at one
position, not the scan between positions. The linter reports `the cost from start rule main is polynomial (n^2)`.

Make one alternative cover a whole run of `a` and the `c` that may follow it:

```zp
// scan-fast.zp
@start
rule main = item* $$

rule item = "a"+ "c"? / "c"
```

The linter reports `the cost from start rule main is linear`.

| `n` | 100 | 200 | 400 |
|:--|--:|--:|--:|
| `scan.zp` | 21,328 | 82,624 | 325,888 |
| `scan-fast.zp` | 616 | 1,216 | 2,416 |

**What changes.** The two grammars accept the same strings of `a` and `c`. The elements of the repetition differ. In
`scan.zp` an `a` that no `c` follows is an element by itself, so a run of `a` is one element for each character, and a
run followed by `c` is one element. In `scan-fast.zp` every run of `a`, with the `c` that follows it if there is one,
is one element, and so is a lone `c`. A tree that must have one node for each `a` cannot come from this rewrite.

## A Lookahead Over a Scan

```zp
// look.zp
@start
rule main = ("a" !rest)* "a"* $$

rule rest = "a"* "c"
```

The lookahead `!rest` scans the rest of the run of `a` at every iteration, so the loop costs the sum of the lengths of
the remainders. The linter reports polynomial (n^2). When the lookahead scans the input that the loop consumes
anyway, the question it asks is a question about what follows the run, and one check after the run answers it:

```zp
// look-fast.zp
@start
rule main = "a"* !"c" $$
```

The linter reports linear.

| `n` | 100 | 200 | 400 |
|:--|--:|--:|--:|
| `look.zp` | 10,912 | 41,824 | 163,840 |
| `look-fast.zp` | 608 | 1,207 | 2,410 |

**What changes.** Both grammars accept the same strings. In `look.zp` the lookahead decides where the first repetition
ends and the second begins; in `look-fast.zp` the run is one repetition. A grammar that needs the two parts as separate
nodes keeps its lookahead, bounded to what it must see, a literal or one character, which costs a constant.

## A Recovery Skip Followed by Failure

A [`#recover` skip](../../spec/runtime/parsing.md#skip-runs) that is a repetition written in the skip expression is
already linear: the walks of the skip share the rest of the input they have walked. Only a repetition that a skip
reaches through a rule is not shared.

```zp
// skip.zp
@start
rule main = (x / "a")* $$

rule x = (y #recover(skip=s)) "c"

rule y = "b"

rule s = [a]+
```

Each `x` fails at `y`, skips the rest of the run of `a`, then fails at `"c"`, and the recovery is abandoned; the next
iteration starts one character later and walks the run again. Write the repetition in the skip expression:

```zp
// skip-fast.zp
@start
rule main = (x / "a")* $$

rule x = (y #recover(skip=[a]+)) "c"

rule y = "b"
```

| `n` | 100 | 200 | 400 |
|:--|--:|--:|--:|
| `skip.zp` | 13,648 | 47,392 | 174,976 |
| `skip-fast.zp` | 4,504 | 9,112 | 18,304 |

**What changes.** Nothing: the skip matches the same text, so the tree, the errors and the extent of the recovery
are the same. The linter still reports polynomial (n^2) for `skip-fast.zp`, because the classification does not count
the records that make the walks linear, so the measurement above is what shows the difference. The repetition must have
no upper bound and no `#cut`, and neither it nor the rules it calls may read a variable or contain a `#recover`.

## A Left-Recursive Rule or an Operators Rule Called Again and Again

```zp
// lr.zp
@start
rule main = (e "q" / "x")* $$

rule e = e "x" / "x"
```

A left-recursive rule grows to the end of the run of `x` where it is called. Here `e "q"` fails at the end of the run,
the alternative `"x"` takes one character, and `e` grows again from the next one. The linter reports polynomial (n^2),
and so for an Operators Rule with the same shape:

```zp
// ops.zp
@start
rule main = (e "q" / "x")* $$

rule e = #operators {
    operand "x"
    level add { infix left "" }
}
```

The rules are not the problem: called once at a position, each is linear. The caller calls them once for each
character. Factor the choice so that `e` is called once and the part that varies follows it:

```zp
// lr-fast.zp
@start
rule main = (e "q"?)* $$

rule e = e "x" / "x"
```

```zp
// ops-fast.zp
@start
rule main = (e "q"?)* $$

rule e = #operators {
    operand "x"
    level add { infix left "" }
}
```

| `n` | 100 | 200 | 400 |
|:--|--:|--:|--:|
| `lr.zp` | 48,160 | 186,496 | 733,696 |
| `lr-fast.zp` | 1,141 | 2,242 | 4,444 |
| `ops.zp` | 21,232 | 82,432 | 325,120 |
| `ops-fast.zp` | 614 | 1,216 | 2,416 |

**What changes.** The grammars accept the same strings. The associativity of `e` is unchanged: it is still the rule that
groups to the left. What changes is where the groups end: in the original, a run of `x` that no `q` follows is read one
character at a time by the alternative `"x"`; in the rewrite it is one `e` that groups the whole run. A grammar that
needs those characters as separate nodes cannot use this rewrite.

## Diverging Variable Bindings

```zp
// var.zp
var v string

@start
rule main = r $$

rule r = "(" {v := v + "0"} r ")" "y" / "(" {v := v + "1"} r ")" "z" / "n" {len(v) >= 0}
```

Both alternatives of `r` bind `v` from its earlier value before the recursive call, so the two calls at a position
see different values of `v`, and the memo shares neither result: the cost doubles with each level. The linter reports
`the cost from start rule main is exponential`, with a witness at the binding. Bind a value that does not depend on
the earlier one:

```zp
// var-fast.zp
var v string

@start
rule main = r $$

rule r = "(" {v := "0"} r ")" "y" / "(" {v := "1"} r ")" "z" / "n" {len(v) >= 0}
```

The linter reports `the cost from start rule main is linear-env`: linear, given a bound on the values the variable
takes (two here).

| input: `(` `n` times, `n`, then `)z` `n` times | `n` = 4 | `n` = 8 | `n` = 400 |
|:--|--:|--:|--:|
| `var.zp` | 3,352 | 1,512,448 | stopped at 20,000,000 |
| `var-fast.zp` | 428 | 896 | 46,816 |

**What changes.** In `var.zp` the variable is the path from the root to the current level, handed down the recursion;
in `var-fast.zp` it is the marker of the innermost level only. No rewrite keeps both the path and the cost when a
decision at the bottom reads the path, because the decision then depends on the whole path, and results that depend on
different paths cannot be shared. If the decision can wait until the recursion returns, return the information
in the rule's value and decide in an action after the call, where the value of each level is built from its
child's, once.

## Self-Growing Values

```zp
// grow.zp
var s string = "a"

@start
rule main = r $$

rule r = "x" {s := s + s} r / {len(s) > 0}
```

```zp
// fold.zp
@start
rule main = xs:"x"* => len(foldl("a", $xs, (acc, x) => acc + acc))
```

A value built from its own earlier value by `s + s` doubles at every step, so after `n` steps it has `2^n`
characters and building it takes that many steps. The linter warns `growing-variable` about the binding of `grow.zp` and reports the
class exponential for both grammars. **No rewrite keeps the result**: a string of `2^n` characters cannot be built in a
number of steps that is linear in `n`. If what the grammar needs is a property of the value, such as its length, or a
count, keep that property instead:

```zp
// grow-fast.zp
@start
rule main = r $$

rule r int = ("x" n:r => $n + 1) / ("" => 0)
```

```zp
// fold-fast.zp
@start
rule main = xs:"x"* => foldl(0, $xs, (acc, x) => acc + 1)
```

| `n` | 4 | 8 | 400 |
|:--|--:|--:|--:|
| `grow.zp` | 205 | 1,690 | stopped at 20,000,000 |
| `grow-fast.zp` | 109 | 145 | 3,676 |
| `fold.zp` | 209 | 1,768 | stopped at 20,000,000 |
| `fold-fast.zp` | 89 | 173 | 8,416 |

**What changes.** The value is a count, not a string. `grow-fast.zp` carries the count in the value of the rule, which each
level builds from the value of the level below, once; a variable cannot do that, because a binding in a called rule is
not visible to its caller, so a binding after the recursive call (`{k := k + 1}`) counts nothing across levels.

## Expected Output

Save the grammars under the file names in their first lines and classify each:

```bash
zp lint -g scan.zp -classify
zp lint -g scan-fast.zp -classify
```

```text
scan.zp:4:10: hint: the cost can grow as n^2: after the alternative at 2:14 in rule main fails, the later alternative "a" at 2:18 in rule main can start again inside the input that the repetition "a"* at 4:10 in rule a examined; this repeats in the repetition (a / "a")* at 2:13 in rule main [super-linear]
scan.zp: the cost from start rule main is polynomial (n^2)
	witness: scan.zp:4:10: the cost can grow as n^2: after the alternative at 2:14 in rule main fails, the later alternative "a" at 2:18 in rule main can start again inside the input that the repetition "a"* at 4:10 in rule a examined; this repeats in the repetition (a / "a")* at 2:13 in rule main
scan-fast.zp: the cost from start rule main is linear
```

## How It Works

- The memo shares the result of a rule at one position. It does not share the cost inside a rule body or between
  positions, so a scan that runs again from a later position pays again. The rewrites for repetitions, lookaheads and
  called rules make every position scan at most once.
- Left recursion and Operators Rules are linear when called once at a position. The growth restarts when the caller
  fails after the rule has grown and tries the same position range again.
- A rule that reads a variable is memoized per value of the variables it reads. Rewrites that keep the number of values
  bounded, a literal or a count, keep the memo effective.
- Values that grow from their own earlier value cost their size, whatever the memo shares.

## Variations and Limits

- The measurement depends on the input shape. These inputs are the worst case of each pattern; a real input may not
  trigger the growth, and the class that the linter reports is an estimate with a witness
  (see [Classifying the Parse Cost](../guide/linting.md#classifying-the-parse-cost)).
- A rewrite changes only what the section says. Compare the trees of the old and the new grammar on real inputs
  before replacing one with the other; the [testing guide](../guide/testing-grammars.md) shows how.
- The runtime does not share the scan of a repetition between positions, so the first two patterns need the rewrite.
  Recovery skips written in the skip expression, a left-recursive rule called once at a position, and right recursion
  are already linear and need none.
- To find which rule spends the steps in a real grammar, use `zp profile`; see
  [Find Out Why a Grammar Is Slow](slow-grammar.md).
