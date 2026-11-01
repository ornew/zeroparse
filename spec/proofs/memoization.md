# Memoization

Arata Furukawa · Last revised 2026-11-01

This proof shows that [memoization](../runtime/parsing.md#memoization) never
changes the result of a parse. A parse that answers calls from the memo returns
the value, the recovered errors and the syntax error of a parse that evaluates
every call, including calls of the rules of left-recursion cycles and calls
whose results read the growing result of a left recursion. The proof also
states what a memo entry must satisfy to be reused, so that the
[Document proof](documents.md) can extend the same induction to entries and
repetition runs that earlier parses stored.

## Setting

Fix a grammar, a start rule, a position unit and an input $t$ of length $n$.
The parse records expectations or not; the proof covers both.

**Calls and their keys.** A call of a rule $A$ at a position $p$ has a binding
level $\ell$ (none, or the level of `level(A, name)`), the values $\eta$ of
the variables that $A$ reads, directly or through the rules it calls (see
[Interaction With Memoization](../language/predicates.md#interaction-with-memoization)),
and a lookahead state $\lambda$. The lookahead state is $1$ inside a
[lookahead](../language/parser-expressions.md#lookahead) or the
[`skip` of an Operators Rule](../language/operators-rule.md#skip), and $0$
elsewhere. The **key** of the call is $\kappa = (A, p, \ell, \eta)$.

**Outcomes.** The evaluation of an expression or a call has an **outcome**

$$\omega = (s, q, v, \varepsilon, o)$$

where $s$ is match or fail, $q$ is the end position ($q = p$ on failure), $v$
is the value, $\varepsilon$ is the sequence of errors that
[`#recover`](../language/pragmas.md#recover) recovered in it, and $o = (f, X)$
is its farthest failure and the expected items there, with the operation
$\cdot$ of [Skip Runs](skip-runs.md#setting). On failure $\varepsilon$ is
empty, because a failure has no effect
([Evaluation](../language/parser-expressions.md#evaluation)). An evaluation can
instead abort with a run-time error $E$ of an action, which is then its
outcome. Each evaluation also has an **examined range** $\rho$, the interval of
the input that it and the calls within it examined, as defined in the
[Document proof](documents.md#setting).

The caller **records** an outcome by continuing at $q$ when $s$ is match,
appending $\varepsilon$ to its recovered errors, replacing its own record $c$
by $c \cdot o$ and its range by the hull with $\rho$.

**Growths.** A call of a leader $L$ at $p$ is evaluated by growing a seed
([Left Recursion](../language/parser-expressions.md#left-recursion)). The
growth of $L$ at $p$ is **open** from the start of that call to its end, and
while it is open it has a **current result**. A call of $L$ at $p$ while the
growth is open is answered with the current result, whatever its $\eta$. A
call **reads** an open growth when it, or a call evaluated within it, is
answered with that growth's current result. The **context** of a call of $A$
at $p$ is $\Gamma$, the sequence of the growths of the leaders of the cycle of
$A$ that are open at $p$ when it starts, each with its leader and its current
result, and its **present set** $P$ is the set of those leaders. By Lemma 2 a
call reads no other open growth. A lookahead at $p$ in a context **observes** its outcome: whether
its expression matched, and its examined range.

**The two parses.** The **reference parse** $\mathcal{R}$ evaluates every call
as the language chapters define it: the body of the rule is evaluated, except
that a leader grows, and a call of a leader at the position where it grows
reads the current result. Nothing else is reused. The **memoized parse**
$\mathcal{M}$ does the same, and in addition keeps a table of **entries**
$(\kappa, \lambda) \mapsto (\omega, \rho)$. It may store the outcome of a call
that it evaluated, and it may answer a later call from an entry instead of
evaluating it, under these conditions:

- **(S) Storing.** A call that reads a growth that was open when the call
  started is not stored in the table. This applies to every rule, including a
  leader that grew inside the growth of another leader. An entry records the
  present set $P$ of the call that stored it.
- **(L) Lookahead state.** An entry stored in state $\lambda$ answers a call in
  state $\lambda'$ only if $\lambda' = \lambda$, or if $\lambda = 0$,
  $\lambda' = 1$ and its $\varepsilon$ is empty.
- **(C) Contexts.** A call of a rule of a left-recursion cycle with the present
  set $P'$ is answered from an entry with the present set $P$ only if
  $P' \subseteq P$.
- **(D) Dependent results.** While a growth is open at $p$, $\mathcal{M}$ may
  also keep, apart from the table, the outcome and range of a call at $p$ that
  read open growths, with its key, state and present set $P$, the current
  results that it read, and the observations of the lookaheads at $p$ within
  it that read one. It answers a call with the same key and state and the
  present set $P$ from it only if each growth that it read is open with an
  equal current result and each observed lookahead has the same observation,
  as a result kept in the same way gives it. It treats a step of a growth, the
  evaluation of the body of a leader from a current result, in the same way.
  It drops what it keeps when the first growth open at $p$ ends.

(S) is the last item of
[Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules) that
concerns the table, and (C) and (D) state the item on contexts; (L) is
assumption (A2) below. Which outcomes $\mathcal{M}$ stores, and when it uses
them, is otherwise free. Storing none, storing from the second call at a
position, or not storing the results of some rules are all instances of
$\mathcal{M}$. The reference implementation decides that two current results
are equal by their identities: a step result gets the identity of an earlier
equal result of the same growth key at the same position, compared by a
bounded structural comparison of the nodes, and a new identity otherwise.

**Scope.** The proof covers a parse that no limit stops: no
[nesting limit](../runtime/parsing.md#limits), cancellation, budget limit or
`MissingTokenLimitError`. A call answered from an entry does not nest, so
whether a parse reaches the nesting limit can depend on memoization, as
[Memoization](../runtime/parsing.md#memoization) states. It covers parses
without insertion of [`#missing`](../language/pragmas.md#missing) tokens,
since an insertion depends on the path to it and on a budget for the whole
parse. A walk of a [skip-run site](../runtime/parsing.md#skip-runs) that
reuses a record ends where evaluating the rest of the walk would end, with
the same farthest failure, expectations and examined range, as the
[Skip Runs proof](skip-runs.md#lemma-1-exactness) shows, so the proof treats
each walk as evaluated in full in both parses.

## Assumptions

The proof relies on the following. The specification does not state (A1) and
(A2); it states (A3), which is cited here.

- **(A1) Recording in a lookahead.** Inside a lookahead or the `skip` of an
  Operators Rule, an evaluation records neither expected items nor its
  farthest failure, so recording an outcome there is the identity.
  [Syntax Errors](../runtime/parsing.md#syntax-errors) states this for the
  expected items only. The reference implementation also does not track the
  farthest failure there.
- **(A2) Reuse across lookahead states.** Condition (L), which the reference
  implementation applies and the specification does not mention.
- **(A3) Comparisons.** `==` and `!=` never compare nodes: they compare two
  `int`, two `string` or two `bool` values, and any value with `nil`, and every
  other comparison is an error
  ([Operators](../language/actions.md#operators-and-functions)). A comparison
  of two scalars depends only on the values, and a comparison with `nil` only
  on whether a value is `nil`, which is part of the value. By (I1) of the
  theorem a comparison therefore decides alike in both parses. An error is
  raised for the same operands in both, so it is the run-time error of $s$.

## Lemma 1 (Context)

In $\mathcal{R}$, let a call read no growth open at its start. Then its $s$,
its $q$, its run-time error, if any, and its examined range are functions of
its key $\kappa$, and its outcome is a function $F(\kappa, \lambda)$ of its key
and lookahead state. For the two lookahead states, $F(\kappa, 0)$ and
$F(\kappa, 1)$ have recovered errors of the same number, and the same value
when they have none.

*Proof.* By induction on the nesting of evaluations, which is finite
([Termination](../runtime/parsing.md#termination)). The evaluation of the body
of $A$ at $p$ depends on the following state and on nothing else:

- The input, the position $p$ and the level $\ell$.
- The variables. The body sees the variables of its callers, and of those it
  reads only the ones in $\eta$. Its own bindings end with the call
  ([Rule Calls](../language/parser-expressions.md#rule-calls)).
- Captures. A call has a scope of its own, so the captures of the caller are
  not visible ([Rule Calls](../language/parser-expressions.md#rule-calls)).
- Cuts. A cut never extends beyond its rule
  ([Cut](../language/parser-expressions.md#cut)), so the cut state of the
  caller does not enter.
- Actions. An action has no effect other than its value and its run-time
  error ([Memoization](../runtime/parsing.md#memoization)), and the
  evaluation is deterministic
  ([Evaluation](../language/parser-expressions.md#evaluation)).
- The calls it makes. By the induction hypothesis, a call it makes that reads
  no growth open at that call's start is a function of its own key and state.
  A call it makes that reads a growth open at that call's start reads either a
  growth that the evaluation of $A$ started, which is part of this evaluation
  and determined by it, or one that was open at the start of $A$, which the
  assumption excludes.
- The lookahead state. By (A1), $\lambda$ decides only whether failures are
  recorded. That changes $o$. It also changes the errors that `#recover`
  builds, because an error is made from the farthest failure recorded in the
  annotated expression: their positions, their lines and columns, their
  expected items, and the messages of the error values in $v$.

None of what $\lambda$ changes reaches a decision of the parser. An error value
is a node of the kind error whose text is its message
([`#recover`](../language/pragmas.md#recover)), but an action or predicate
cannot read that text: `text` does not accept a value of the type `error`, so
`text($x)` on an error value is a compile-time error, and the type `error`
declares no field ([Built-In Functions](../language/actions.md#built-in-functions)).
By (A3), `==` does not compare an error node, so a comparison never reads its
message either. So $s$, $q$,
the run-time error and the examined range are the same for both states, each
`#recover` recovers in both or in neither, and $\varepsilon$ has the same
length. When $\varepsilon$ is empty, no recovery took place on the path that
matched, so $v$ holds no error value and is the same. $\square$

## Lemma 2 (Reading an Open Growth)

If a call of $A$ at $p$ reads a growth of $L$ that was open at $p_L$ when the
call started, then $p = p_L$ and $A$ is a rule of the cycle of $L$.

*Proof.* The call started while the growth was open, so it was made within
the evaluation of $L$ at $p_L$, where no position is before $p_L$:
$p \ge p_L$. It reads the current result, so a call of $L$ at $p_L$ is made
within it, where no position is before $p$: $p \le p_L$. So $p = p_L$, and
both chains of calls, from $L$ to $A$ and from $A$ to $L$, are made at $p$
without consuming input. Every call on them is a call that may run before
consuming input, which the static analysis of
[Left Recursion](../language/parser-expressions.md#left-recursion) includes,
so $A$ and $L$ are on one left-recursion cycle. $\square$

## Sound Entries

An entry $(\kappa, \lambda) \mapsto (\omega, \rho)$ is **sound** when
$\omega = F(\kappa, \lambda)$ and $\rho$ contains the examined range that
Lemma 1 gives for $\kappa$.

## Lemma 3 (Answering From a Sound Entry)

A call of key $\kappa$ in state $\lambda'$ that the conditions allow to be
answered from a sound entry reads no growth open at its start, and recording
the entry gives the outcome that the call has in $\mathcal{R}$, and a range
that contains the range of the call in $\mathcal{R}$.

*Proof.* Suppose that the call reads a growth open at its start. By Lemma 2
it is a call, at the position $p$ of an open growth, of a rule of its cycle, so
by (C) its present set $P'$ is a subset of the present set $P$ of the call
that stored the entry, which read no growth open at its start. Compare the
two evaluations in $\mathcal{R}$ from their start: they take the same steps
until one of them is answered with a current result. A growth that started
within one started within the other too, so the first difference is a call of
a leader $L$ at $p$ answered with the current result of a growth open at the
start, which by Lemma 2 is on the cycle of the rule, so $L \in P'$. Since
$L \in P$, the corresponding call in the evaluation that
stored the entry is answered with a current result too, which it never is. So
the call reads no growth open at its start, and by Lemma 1 its outcome in
$\mathcal{R}$ is $F(\kappa, \lambda')$, with the range that Lemma 1 gives for
$\kappa$, which $\rho$ contains.

If $\lambda' = \lambda$, the outcome is $\omega$. Otherwise $\lambda = 0$,
$\lambda' = 1$, and $\varepsilon$ is empty by (L). By Lemma 1,
$F(\kappa, 1)$ has the $s$, $q$ and $v$ of $\omega$ and an empty
$\varepsilon$, and by (A1) recording it in state $1$ records no $o$, as
recording $\omega$ there does not. $\square$

## Lemma 4 (Answering From a Dependent Result)

Let a call $x$ in $\mathcal{R}$, of key $\kappa$ in state $\lambda$ at $p$
in a context $\Gamma_1$ with the present set $P$, have the outcome $\omega$
and the range $\rho$, and let a call $y$ of the same key and state in a
context $\Gamma_2$ with the present set $P$ find each growth that $x$ read open
with an equal current result, and each lookahead at $p$ that $x$ observed with
the same observation. Then $y$ has the outcome $\omega$ and the range
$\rho$.

*Proof.* Compare the evaluations of $x$ and $y$ from their start. They take
the same steps as long as each step reads the same state, and, as in Lemma 1,
the state that a step reads differs between them only in the growths open at
$p$ at their start. A call of a leader at $p$ is answered with a current
result in both or in neither: only a growth of the cycle of the rule can
answer it (Lemma 2), and the present sets are equal; when it is, the result is equal, since $x$ read it. A lookahead at
$p$ that reads such a growth has the same outcome and range in both, since $x$
observed it, and by (A1) its expression records nothing outside it. A call at
$p$ within $x$ that grows a leader of the cycle grows it in both, from the same
failing seed; its steps read the growths open at the start of $x$ only in the
ways above, so by induction on the steps they have the same outcomes, and so
does the growth. A call at a later position reads no growth open at the start
of $x$ (Lemma 2). $\square$

## Theorem (Memoization Never Changes the Result)

Let $\mathcal{M}$ start with a table of sound entries, empty for an ordinary
parse. Then the parses $\mathcal{M}$ and $\mathcal{R}$ return the same value,
the same recovered errors and the same syntax error, or abort with the same
run-time error.

*Proof.* Order the evaluations of $\mathcal{M}$, of expressions and of calls,
by their completion, so that an evaluation comes after every evaluation nested
in it. By induction on this order, each evaluation $x$ satisfies the
following invariant when it completes, provided that every evaluation that
completed before it satisfies it:

- **(I1)** The outcome of $x$ in $\mathcal{M}$ is its outcome in $\mathcal{R}$
  when evaluated in the same state: the same position, variables, captures,
  lookahead state, open growths and current results.
- **(I2)** The examined range of $x$ in $\mathcal{M}$ contains its examined
  range in $\mathcal{R}$.
- **(I3)** Every entry in the table is sound.

At the start, (I3) holds by assumption. Let $x$ complete; the evaluations
nested in it completed before it and satisfy the invariant. $x$ is one of the
following.

1. *A terminal, an anchor, a predicate or an action.* These read the same state
   in both parses, so they have the same outcome and examine the same input.
   The values they read are equal by (I1), though they may be different nodes,
   and by (A3) a comparison depends only on scalar values and on whether a
   value is `nil`.
2. *A combinator.* A sequence, choice, repetition, option, lookahead, cut,
   capture or `#recover` combines the outcomes of its parts by the rules of
   [Rule Expressions](../language/parser-expressions.md) and records them in
   order. Its parts satisfy (I1) and (I2), so it does too.
3. *A call answered with the current result of an open growth.* Both parses
   answer it so. The current result is the outcome of a completed body
   evaluation of the growth (item 5), or the failing seed, so it is the same in
   both parses.
4. *A call answered from an entry.* By (I3) the entry is sound, so by Lemma 3
   (I1) and (I2) hold for $x$. A call answered from a result that (D) keeps
   was answered under the conditions of Lemma 4, and that result was the
   outcome and range of an evaluation of $\mathcal{M}$ that completed before,
   which satisfies (I1) and (I2); so by Lemma 4 they hold for $x$. Equal
   current results read in both parses are equal by (I1). A step of a growth
   answered in the same way, or a run of steps, has by Lemma 4 the outcomes
   that evaluating the steps gives.
5. *A call that evaluates the body of a rule, or grows a leader.* A body
   evaluation is nested in $x$ and satisfies (I1) and (I2). A growth evaluates
   the body with the current result fixed, compares the end of its outcome with
   that of the current result, and either replaces the current result and
   evaluates the body again or stops. Both parses start from the same failing
   seed, so by induction on the steps of the growth they replace the current
   result with the same outcomes and stop at the same step.

If $x$ is a call whose outcome $\mathcal{M}$ stores, then by (S) $x$ read no
growth open at its start. By Lemma 1 its outcome in $\mathcal{R}$ is
$F(\kappa, \lambda)$, by (I1) and (I2) the stored entry is sound, and (I3)
holds after storing it.

The parse is the call of the start rule at $0$ with no open growth, followed for
a whole-input parse by the check for the end of the input
([Whole Input](../runtime/parsing.md#whole-input)). By (I1) it has the same
outcome in both parses, so it gives the same value, the same recovered errors,
and the same farthest failure with its expected items, from which the syntax
error is made ([Syntax Errors](../runtime/parsing.md#syntax-errors)).

A run-time error is an outcome like the others. An entry holds only the outcome
of a call that completed without one, since the parse aborts otherwise, and by
Lemma 3 the call that it answers has the same outcome in $\mathcal{R}$, so
evaluating that call raises no run-time error there either. So the first
run-time error that either parse meets is met by the other at the evaluation
of the same expression in the same state. $\square$

## Consequences

**Values.** The two parses return equal values: the same nodes with the same
kinds, ranges, children and fields. Whether two calls return the same node
is not part of the outcome, and no action can observe it, by (A3)
([Memoization](../runtime/parsing.md#memoization)).

**Leaders and members.** A result of a rule of a cycle that read the current
result of a growth open at its start is not stored in the table by (S), and
what (D) keeps of it is dropped with the first growth at its position. This
holds for another leader of the cycle too: a later call of it at that position
grows it again. The result of a call therefore never depends on which calls
the parse evaluated before it.

**Variables.** A call is answered from an entry only with an equal $\eta$, so a
rule that reads variables is memoized per combination of their values. The
exception of Left Recursion, where every call of a growing leader at its
position reads the current result whatever $\eta$ is, holds in both parses
alike.

**Order of storing.** The theorem holds for every choice of which outcomes to
store, so deferring memoization to the second call at a position, or not
memoizing a rule that is called from one place, leaves the result unchanged.
