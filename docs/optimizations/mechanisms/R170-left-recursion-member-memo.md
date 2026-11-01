# R170: Left-Recursion Member Memoization

The rules of a left-recursion cycle other than its leader are memoized, so a member reached from several
call sites at one position is evaluated once instead of once per call.

## Mechanism and Invariants

The result of a call of a rule of a cycle is a function of the growths of its cycle in progress at its position,
its context: the leaders that grow there and the seeds they have reached. A call records what its result depended
on: the seeds it read, each by an identity that equal step results of a leader share; the outcome of each lookahead
whose evaluation read a seed, in place of those seeds; and the leaders that grew at the position (the present set,
one bit per leader). A result that read no seed and observed no lookahead goes to the memo with its present set and
answers any call whose context it includes. Any other result is kept in the table of its context, which lasts while
the first growth there lasts, and answers a call in a context with the same present set where the seeds it read have
the same identities and the lookaheads it observed have the same outcomes. A growth in a context memoizes its steps
the same way and records runs of steps that read only its own seed as jumps. Leaders grow from failure; a call of a
leader that grows at its position reads the seed. No dependent result reaches the memo of a Document, so later parses
reuse only results that depend on the rule, the position and the variables. The transient policy applies to members.

## Scope and State

All six backends and Documents. A transient member, one called from a single place, is not memoized in
ordinary parses.

## Reference Control

`DisableMemo` disables memoization of members and other rules together. Tests compare the engine with its internal
parse option `ContextMemoOff`, which evaluates every call in a context again.

## Evidence and Implementation

Evidence record: [130](../130-memoizing-members-of-left-recursion-cycles.md).

Source owners: [`CompileProgram`](../../../internal/compile/compile.go) (the memo flags and the cycle numbers),
[`CallBegin`, `CallEnd` and `GrowBegin`](../../../internal/engine/core/runtime.go) with
[the engine's dependency memo](../../../internal/engine/core/context_memo.go),
[the dependency memo of the generated Go runtime](../../../runtime/context_memo.go) and
[its calls and growths](../../../runtime/call.go), [the call protocol in Exec IR](../../../internal/ir/exec/core/core.go)
with [its context and growth operations](../../../internal/ir/exec/ops/transitional.go),
[`Parser.ctxEnter` and the growths of the TypeScript runtime](../../../internal/gen/tsgen/tsrt/runtime.ts).

Related mechanisms: [R020](R020-deferred-memoization.md), [R120](R120-transient-rule-memo-policy.md).

[Optimization Catalog](../README.md)
