# 130. Memoizing the Members of Left-Recursion Cycles

- **Status**: Implemented
- **Date**: 2026-10-31

## Problem and Mechanism

The rules of a left-recursion cycle other than its leader used to be evaluated at every call. With

```
main = A $$
A = C "+" / "n"
C = "(" C ")" "y" / "(" C ")" "z" / A
```

and the input `(` n times, `n`, `)z` n times, each call of `C` at a position evaluates `C` at the next
position through two alternatives, so the evaluations double with each level: 2^n on every backend. The same
grammar with `A = "n"` is linear, because `C` is then memoized.

A member of a cycle is memoized like any other rule. While leaders of its cycle grow at its position, its result
depends on the growths there, and the parser records what it depended on so that every reused result equals the
result of an evaluation:

- **The context of a call.** The context of a call of a rule of a cycle is the run of growths of that cycle at the
  call's position: the leaders that grow there and the seeds they have reached. Growths of other cycles do not
  matter, since the call reads none of their seeds, and neither do growths at other positions. A call of a leader
  that grows at its position reads the seed. A leader that does not grow there grows from failure, with the seeds
  of the context visible.
- **What a result depends on.** A call records the seeds it read, each by its identity, a number that the growth
  step that produced the seed gives it; for each lookahead whose evaluation read a seed and that leaves nothing
  after it but its outcome, that outcome and its examined range, in place of those seeds; and the present set, the
  leaders that grew at the position, one bit each for the first 32 leaders of a cycle. A positive lookahead whose
  expression makes a capture or binds a variable outside the rules it calls, or that matches after recovering from
  an error, leaves more than its outcome, so the seeds it read count as read by the call.
- **Where a result goes.** A result that read no seed and observed no lookahead goes to the memo with its present
  set, and answers a call in any context whose leaders it includes. Any other result goes to the table of its
  context, which lasts while the first growth of the context lasts, and answers a call in a context with the same
  present set where the seeds it read have the same identities and the lookaheads it observed have the same
  outcomes. A context whose leaders do not all have a bit cannot be told from others, and its results are
  evaluated again.
- **Growths in a context.** A growth in a context memoizes its steps in the table, with its own seed among the seeds
  a step reads, and records runs of steps that read no seed but their own as jumps: from each seed of the run, the
  seed at its end, with the observations, expectations and examined input of the steps. A later growth of the leader
  in the same context takes a run at once when each lookahead that the steps observed has the same outcome now and
  that outcome read no seed of the leader, so that it holds for every seed of the run. Equal step results of a
  leader at a position get the same identity, so a result keyed by an earlier seed matches a later equal one.
- **Reuse passes on what the result read.** A reused result passes on to the calls around it the seeds it read with
  their current identities, and for each lookahead it observed, the seeds that the lookahead's outcome read in the
  current context, as an evaluation there would.

The leaders keep their own memoization. The engine, the generated Go runtime (untyped and typed, with and without
the Exec IR compositions) and the TypeScript runtime implement the same rules. The compiled program numbers the
cycles (`Cycle` in the rule table and the rule flags of generated code), so finding the context of a call is a scan
of the growths at its position. Documents add a rule across parses: no result that read a seed reaches the memo of
a later parse (see Correctness and Tradeoffs).

In the example, `C` at the next position is reached from the growth of `A` at the earlier one, but outside the
growth of `A` at the next position. It reads no seed, is stored by its first evaluation with an empty present set,
and the second alternative reuses it.

## Applicability

| Execution profile | Applied / Partial / Not applied / Not applicable | Reason |
|:--|:--|:--|
| Closure, bytecode, iterative bytecode | Applied | One shared call-end rule and context tables in the engine |
| Generated Go, ordinary and budget-controlled | Applied | The call paths, growths and context tables of the runtime, and the same steps in the Exec IR compositions |
| Generated typed Go | Applied | The `TParser` call paths and growths share the context tables |
| Generated TypeScript | Applied | `callBody`, `grow` and the context tables of the runtime |
| Documents | Applied | Results in the memo are reused across parses; results in a context table are dropped with the table |

A member that is called from one place is still transient, so a grammar like the left-recursive calculator
(`add`, `mul`) keeps its call code; its generated parsers only carry the cycle numbers of those rules.

## Reproduction

Base `430194ee`, Go 1.27.1, Apple silicon, benchlock. Test binaries of the `bench` package, one per revision, ran
interleaved for 10 rounds with `-benchtime=200ms -benchmem` over the generated forms of `Parse` and `Recognize`
(left-recursive calculator, JSON, Minilang), `Incremental`, `IncrementalLong` and `Stream`; benchstat compared them.
Tests: `TestLeftRecursionDependencyMemoRandom`, `TestLeftRecursionDependencyMemoControls`,
`TestLeftRecursionDependencyMemoUnexamined`,
`TestLeftRecursionDependencyMemoLinear`, `TestNestedLeaderResultNotReused`, `TestLeftRecursionMemberMemoLinear`,
`TestLeftRecursionMemberMemoMatchesNoMemo`, `TestLeftRecursionMemberMemoStale`, `TestLeftRecursionMemberMemoRandom`,
`TestLeftRecursionMemberMemoRandomRecovering`, `TestGeneratedLeftRecursionDependencyMemo`,
`TestGeneratedLeftRecursionDependencyMemoControls`, `TestGeneratedLeftRecursionDependencyMemoLinear`,
`TestGeneratedLeftRecursionMemberMemoLinear`, `TestGeneratedLeftRecursionMemberMemoStale`,
`TestGeneratedTSLeftRecursionDependencyMemo`, `TestGeneratedTSLeftRecursionDependencyMemoControls`,
`TestGeneratedTSLeftRecursionDependencyMemoLinear`, `TestGeneratedTSLeftRecursionMemberMemoLinear`,
`TestGeneratedTSLeftRecursionMemberMemoStale`, `TestDocumentLeftRecursionMemberReparse`,
`TestDocumentLeftRecursionMemberReparseRandom`, `TestGeneratedDocumentLeftRecursionMemberReparse` and their controls.

## Results

Rule evaluations on the example (`Profile.Evals`, the same on the three engine backends):

| n | 8 | 16 | 32 | 64 |
|:--|--:|--:|--:|--:|
| Two members (`A`, `C`) | 25 | 41 | 73 | 137 |
| Four members, two call sites each | 85 | 149 | 277 | 533 |

Before the change the example runs out of the default budget (2^20 units) at n = 16. After it, generated Go
(ordinary and controlled, untyped and typed) and generated TypeScript parse n = 300.

Speed (medians of 10 interleaved samples against the base; differences within noise unless noted):

| Workload | Change |
|:--|:--|
| Parse, left-recursive calculator | +6.0% (p < 0.001) on generated typed `ParseAST`, +2.5% in code points and +2.8% in bytes (p = 0.003) |
| Recognize, left-recursive calculator | +5.6% (p < 0.001) |
| Parse and Recognize JSON and Minilang | +2.2% to +3.5% (p ≤ 0.001) on Minilang; JSON `ParseAST` 1.2% faster; the other JSON cells within noise |
| Incremental (Minilang), IncrementalLong (CSV), Stream (CSV) | within noise, except a fresh generated Minilang parse +2.3% (p < 0.001), iterative +0.5% and closure IncrementalLong +0.6%, a generated Minilang Document parse 2.7% faster and closure Stream 1.0% faster |

The cost on the calculator comes from the bookkeeping of each growth: a record of the growth in progress, the search
of the context of every call of a rule of a cycle at the position of a growth, and the present set stored with each
result. The calculator has no growth inside another at one position, so it records no dependency and keeps no
context table. Bytes per operation fall on the calculator (−2.0% in code points, −5.0% for `ParseAST`, −20% for
Recognize), since a growth step writes its result in place of allocating a memo entry. Allocations are unchanged
except 3 more per `ParseAST` on the calculator (1%), and one more per operation for the closure Document of
Incremental and for IncrementalLong on the engine backends; generated
`Recognize` on the calculator allocates 21 to 23 times per operation depending on how many iterations
`-benchtime=200ms` runs, as the setup of the run is divided by that count. The memo entry of the generated runtimes
stays at 128 bytes: the present set takes the place of the depth and the unstable mark.

## Correctness and Tradeoffs

[Memoization](../../spec/proofs/memoization.md) proves that a reused result equals the result of an evaluation in
the current context: a result in the memo read no seed and observed no lookahead that read one, so the growths that
its present set includes do not change it; a result in a context table is reused only where every seed it read has
the same identity and every lookahead it observed the same outcome and examined range, and the identities of equal
step results are equal only when the results are. Two step results with the same end are not equal when they differ
in the rule that produced a node. In

```
main = A $$
A = C / "a" / C / "b" C C
B = B "b" / A "a" / C / "a" "a"
C = B A / &A "a" "c"
```

with the input `aaa`, `B` at 0 grows to `("a" "a")@B`, and inside the growth of `A` there, with the seed `"a"@A`,
to `("a"@A "a")@B`, which ends at the same offset. Giving both the same identity reuses the first where the second
belongs, and the tree loses the rule of its first leaf.

Three more shapes show what the other rules exclude. In

```
main = S $$
S = M P S "q" / "y" ("qqqqqqqq")?
P = M ""
M = &(x:S) => $x
U = P P
```

the capture that the lookahead of `M` makes holds, after the lookahead, the seed it read. Reusing a call of `M` by
the outcome of its lookahead returns an earlier seed: on `yqqq` a node ends at 2 instead of 3. The same grammar with
`M = &(S (("!") #recover(skip=[q]))) ""` reuses an error recovered at the end of an earlier seed. In

```
main = T $$
T = &S S "!" "z" / S "q" "!" "z" "#" / T "!" / "yqq!z#@" / "yqq"
S = &L S "q" / "y"
L = T "!" / &T S "q" "q"
```

the lookahead `&L` reads only `T` while the seed of `T` is `yqq`, and also the seed of `S` once it is `yqq!z`. A run
of `S` recorded under the first seed of `T` is checked under the second only for the first seed of `S`, where `&L`
still matches; taking it skips a step where `&L` fails, and `yqq!z#` does not parse. Last, a reused result that
passed on the seed identities that its lookaheads read where it was made, instead of the current ones, let a result
around it be kept under identities it did not read: random recovering cycle grammar 2964 then rejects `baabcac`
where generated parsers and the engine without examined ranges must accept it.

The engine records the input that every evaluation examines, because its traces report it, so equal step results
and equal lookahead outcomes also need equal examined ranges there. Generated Go records it only in Document parses
and TypeScript never does; elsewhere they take a step or reuse an outcome that differs only in its examined range,
which the engine evaluates again. The results are the same, but traces and evaluation counts can differ. The test that
compares the traces of generated Go with the engine's runs the engine with a test option that leaves examined ranges
out in the same way.

A differential test compares every engine backend with the engine's reference that evaluates every call in a
context again, on the random grammars of 800 seeds: for each seed, a grammar of cycles with several leaders and
members, read inside and outside lookaheads, the same with cuts, and one whose items recover errors. Generated Go (untyped and typed, with and
without the Exec IR compositions, ordinary and budget-controlled) and generated TypeScript are compared with the
engine on the grammars of the first 60 seeds, and the grammars of the first 120 seeds detect, on every
implementation, each check broken in turn: a reuse that ignores the identities of the seeds read, one that ignores
the observations, a memo result reused whatever its present set, equal step results given one identity without
comparing them, a jump over observations whose outcomes read the growing seed, a reuse that passes on the seeds its
observations read where it was made, and an observation of a lookahead that recovered errors. The grammars above are
among the compared grammars on every implementation; the engine detects the broken replay with the test option that
leaves examined ranges out, as generated parsers do. A wider sweep, run once, found no difference: the grammars of seeds 800 to 3,999 on
every engine backend, with and without examined ranges (9,173 grammars), and the grammars of the first 600 seeds with
generated Go and generated TypeScript (1,734 grammars, 103,812 TypeScript results).

A result that read a seed is kept in the table of its context, never in the memo, and the table is dropped when the
first growth of its context ends, so no such result reaches a later parse of a Document. A Document parse that
reused one would depend on the order in which its parse evaluated the calls: on the seed grammars 117 of
`RandomCycleGrammar` and 5077 of `RandomRecoveringCycleGrammar`, a runtime that also puts the results of the
context tables in the memo gives another tree and other syntax errors on the second parse of an unchanged
Document than on the first. Tests parse Documents of the member grammars, the seed grammars and random cycle
grammars twice without an edit and after deleting and inserting again each character, on every engine backend and
with generated Go (untyped and typed, ordinary and budget-controlled, with and without the compositions), and
compare each outcome with a parse from scratch; their controls detect the difference on both seed grammars. A second
parse of an unchanged Document on the left-recursive calculator or on the member example evaluates no rule body.

The dependency bookkeeping charges the budget for each dependency recorded and each table lookup and store. On the
random grammars, against the reference that evaluates every call in a context again (closure backend, inputs that
neither stops at a limit):

| Corpus | Budget units | Reference units | Difference | Evaluations | Reference evaluations |
|:--|--:|--:|--:|--:|--:|
| 400 random cycle grammars | 2,834,471 | 2,820,502 | +0.5% | 116,728 | 195,277 |
| 800 recovering cycle grammars | 7,981,622 | 9,881,280 | −19% | 302,413 | 492,099 |

A grammar without leaders that grow inside each other at one position records nothing: a call of a rule of a cycle
where no growth of its cycle is in progress at its position stores its result in the memo with an empty present set.

## Decision and Limits

Adopted on every backend. A growth that begins from a result that includes the seed of another growth at its
position is new in each step of that growth; when it covers the input that later steps cover, it covers it again in
each step, which [Conditions for Linear Cost](../../spec/runtime/parsing.md#conditions-for-linear-cost) excludes as
condition 5. A context with more than 32 leaders of one cycle, or a result with more than six seeds and observations,
is not memoized, and its calls are evaluated again. The jumps from a seed are a list that a growth scans for one whose
observations hold; in a shape where every step reads a successful lookahead over another leader
(`S = &R S "q" / "y"`, `R = &S R "q" / "y"` on `y` and n `q`), the evaluations stay linear but the list grows with
each step, so the scan is quadratic: generated Go parses n = 1,000, 2,000 and 4,000 in 28, 79 and 281 ms, and the
engine runs out of the default budget from n = 793. Not measured: grammars with many members called from several
places, where memoization could cost more than it saves, as no example grammar of the repository has them.
