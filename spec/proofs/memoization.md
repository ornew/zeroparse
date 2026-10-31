# Memoization

This proof shows that [memoization](../runtime/parsing.md#memoization) never
changes the result of a parse: a parse that answers calls from the memo, as
[Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules) allows,
returns the value, the recovered errors and the syntax error of a parse that
evaluates every call. This includes the rules of left-recursion cycles and
results that read the growing result of a left recursion. The proof also states
what a memo entry must satisfy to be reused, so that the
[Document proof](documents.md) can use the same argument for entries that
earlier parses stored.

## Setting

Fix a grammar, a start rule, a position unit and an input $t$ of length $n$.

**Calls and their keys.** A call of a rule $A$ at a position $p$ has a binding
level $\ell$ (none, or the level of `level(A, name)`), the values $\eta$ of
the variables that $A$ reads, directly or through the rules it calls (see
[Interaction With Memoization](../language/predicates.md#interaction-with-memoization)),
and a lookahead state $\lambda$, which is $1$ inside a
[lookahead](../language/parser-expressions.md#lookahead) or the
[`skip` of an Operators Rule](../language/operators-rule.md#skip) and $0$
elsewhere. The **key** of the call is $\kappa = (A, p, \ell, \eta)$.

**Outcomes.** The evaluation of a call has an **outcome**

$$\omega = (s, q, v, \varepsilon, o)$$

where $s$ is match or fail, $q$ is the end position ($q = p$ on failure), $v$
is the value, $\varepsilon$ is the sequence of errors that
[`#recover`](../language/pragmas.md#recover) recovered in it, and $o = (f, X)$
is its farthest failure and the expected items there, with the operation
$\cdot$ of [Skip Runs](skip-runs.md#setting). An evaluation can also abort with
a run-time error $E$ of an action, which is then its outcome. On failure
$\varepsilon$ is empty, because a failure has no effect
([Evaluation](../language/parser-expressions.md#evaluation)).

The caller **records** an outcome by continuing at $q$ when $s$ is match,
appending $\varepsilon$ to its recovered errors and replacing its own record
$c$ by $c \cdot o$. Inside a lookahead or a `skip`, failures are not included
in the expected items ([Syntax Errors](../runtime/parsing.md#syntax-errors)),
so there an evaluation records $o = (0, \varnothing)$, the identity.

**Growths.** A call of a leader $L$ at $p$ is evaluated by growing a seed
([Left Recursion](../language/parser-expressions.md#left-recursion)). The
growth of $L$ at $p$ is **open** from the start of that call to its end, and
while it is open it has a **current result**. A call of $L$ at $p$ while the
growth is open is answered with the current result, whatever its $\eta$. A
call **reads** an open growth when it, or a call evaluated within it, is
answered with that growth's current result.

**The two parses.** The **reference parse** $\mathcal{R}$ evaluates every call
as the language chapters define it: the body of the rule is evaluated, except
that a leader grows, and a call of a leader at the position where it grows
reads the current result. Nothing else is reused. The **memoized parse**
$\mathcal{M}$ does the same, and in addition keeps a table of **entries**
$(\kappa, \lambda) \mapsto \omega$. It may store the outcome of a call that it
evaluated, and it may answer a later call from an entry instead of evaluating
it, under the following conditions, which are those of
[Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules):

- **(S) Storing.** A call that reads a growth that was open when the call
  started is not stored. This applies to every rule, a leader that grew inside
  another growth included.
- **(L) Lookahead state.** An entry stored in state $\lambda$ answers a call in
  state $\lambda'$ only if $\lambda' = \lambda$, or if $\lambda = 0$,
  $\lambda' = 1$ and its $\varepsilon$ is empty.
- **(C) Cycles.** While the growth of a leader $L$ is open at $p$, a call at
  $p$ of a rule of the cycle of $L$ other than $L$ is evaluated, not answered
  from an entry.

Which outcomes $\mathcal{M}$ stores, and when it uses them, is otherwise free.
Storing none, storing from the second call at a position, or not storing the
results of some rules are all instances of $\mathcal{M}$.

The theorem below is about parses that no limit stops: no
[nesting limit](../runtime/parsing.md#limits), cancellation or budget limit.
A call answered from an entry does not nest, so whether a parse reaches the
nesting limit can depend on memoization, as
[Memoization](../runtime/parsing.md#memoization) states.

## Lemma 1 (Context)

*In $\mathcal{R}$, the outcome of a call that reads no growth open at its start
is a function $F(\kappa, \lambda)$ of its key and its lookahead state. Two such
evaluations whose $\lambda$ differ have the same $s$, $q$, $v$ and run-time
error, and differ at most in the expected items of $\varepsilon$ and in $o$.*

*Proof.* By induction on the length of the evaluation, which is finite
([Termination](../runtime/parsing.md#termination)). The evaluation of the
body of $A$ at $p$ depends on the following state, and on nothing else:

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
- The calls it makes. By the induction hypothesis each call it makes that
  reads no growth open at that call's start is a function of its own key and
  state. A call it makes that reads a growth open at that call's start reads
  either a growth that the evaluation of $A$ started, which is part of this
  evaluation and determined by it, or one that was open at the start of $A$,
  which the assumption excludes.

The lookahead state enters only through the recording of expected items,
which it turns off. That changes $o$, and the expected items of the errors
that `#recover` builds, and nothing that decides $s$, $q$, $v$ or a run-time
error. $\square$

## Lemma 2 (Reading an Open Growth)

*If a call of $A$ at $p$ reads a growth of $L$ open at $p_L$ when the call
started, then $p = p_L$ and $A$ is a rule of the cycle of $L$.*

*Proof.* The call started while the growth was open, so it was made within
the evaluation of $L$ at $p_L$, where no position is before $p_L$:
$p \ge p_L$. It reads the current result, so a call of $L$ at $p_L$ is made
within it, where no position is before $p$: $p \le p_L$. So $p = p_L$, and
both chains of calls, from $L$ to $A$ and from $A$ to $L$, are made at $p$
without consuming input. Every call on them is a call that may run before
consuming input, which the static analysis of
[Left Recursion](../language/parser-expressions.md#left-recursion) includes,
so $A$ and $L$ are on one left-recursion cycle. $\square$

## Lemma 3 (Sound Entries)

*Call an entry $(\kappa, \lambda) \mapsto \omega$ **sound** when
$\omega = F(\kappa, \lambda)$. A call of key $\kappa$ in state $\lambda'$ that
the conditions allow to be answered from a sound entry has, in $\mathcal{R}$,
the outcome that recording $\omega$ produces in state $\lambda'$.*

*Proof.* The call is not one that condition (C) excludes. If it read a growth
open at its start, by Lemma 2 it would be a call, at the position of an open
growth of $L$, of a rule of the cycle of $L$. If that rule is $L$, the call is
answered with the current result and never from an entry. Otherwise (C)
excludes it. So it reads no open growth, and by Lemma 1 its outcome in
$\mathcal{R}$ is $F(\kappa, \lambda')$.

If $\lambda' = \lambda$, this is $\omega$. Otherwise $\lambda = 0$,
$\lambda' = 1$ and $\varepsilon$ is empty by (L). By Lemma 1,
$F(\kappa, 1)$ has the $s$, $q$ and $v$ of $\omega$, also an empty
$\varepsilon$, since the errors are the same apart from their expected items,
and an $o$ that recording in state $1$ turns into the identity, as it does
the $o$ of $\omega$. $\square$

## Theorem (Memoization Never Changes the Result)

*Let $\mathcal{M}$ start with a table of sound entries, empty for an ordinary
parse. Then every call that $\mathcal{M}$ evaluates or answers has the outcome
that it has in $\mathcal{R}$, in the same state of open growths and current
results, and every entry that $\mathcal{M}$ stores is sound. In particular the
two parses return the same value, the same recovered errors and the same
syntax error, or abort with the same run-time error.*

*Proof.* By induction on the steps of $\mathcal{M}$ in order. Each step is one
of the following.

1. *A terminal, an anchor, a predicate or an action.* These read the same
   state in both parses, by the induction hypothesis for everything before
   them, so they have the same outcome.
2. *A combinator.* A sequence, choice, repetition, option, lookahead, cut,
   capture or `#recover` combines the outcomes of its parts by the rules of
   [Rule Expressions](../language/parser-expressions.md) and records them in
   order. The parts have the same outcomes in both parses, so the combination
   has too.
3. *A call answered with the current result of an open growth.* Both parses
   answer it so, and the current results agree by the induction hypothesis
   (step 5).
4. *A call answered from an entry.* The entry is sound: it was in the initial
   table, or it was stored at an earlier step and is sound by the induction
   hypothesis. By Lemma 3, recording it has the outcome of the call in
   $\mathcal{R}$.
5. *A call that evaluates the body of a rule, or grows a leader.* Each body
   evaluation is a combination of steps that come later, so by the induction
   hypothesis it has its outcome in $\mathcal{R}$, given equal current
   results. A growth evaluates the body with the current result fixed,
   compares how far it ended with the current result, and either replaces the
   current result and evaluates again or stops. Both parses start from the
   same failing seed, so by induction on the steps of the growth they replace
   the current result with the same outcomes and stop at the same step.
6. *Storing an outcome.* By (S), the call that produced it read no growth
   open at its start, so by Lemma 1 its outcome in $\mathcal{R}$ is
   $F(\kappa, \lambda)$. By step 4 or 5 its outcome in $\mathcal{M}$ is the
   same, so the entry is sound.

The parse is the call of the start rule at $0$ with no open growth, followed
for a whole-input parse by the check for the end of the input
([Whole Input](../runtime/parsing.md#whole-input)). Its outcome is the same in
both parses, and so are the value, the recovered errors, and the farthest
failure with its expected items, from which the syntax error is made
([Syntax Errors](../runtime/parsing.md#syntax-errors)).

A run-time error is an outcome like the others. An entry holds only the
outcome of a call that completed, since the parse aborts otherwise, so a call
answered from it would complete in $\mathcal{R}$ too, and an action that aborts
$\mathcal{R}$ aborts $\mathcal{M}$ at the same step. $\square$

## Consequences

**Values.** The two parses return equal values: the same nodes with the same
kinds, ranges, children and fields. Whether two calls return the same node
or equal nodes is not part of the outcome
([Memoization](../runtime/parsing.md#memoization)).

**Leaders and members.** A member of a cycle that the growth of a leader
reaches at its position is evaluated again at each step by (C), and its result
is not stored by (S), because it read the current result. A leader $L'$ that
grows at the position of an open growth of another leader $L$ of its cycle
and reads it is evaluated again at each step of $L$ by (C). By (S) its result
is not stored even after the growth of $L$ ends, so a later call of $L'$ at
that position grows it again. The result of a call therefore never depends
on which calls the parse evaluated before it.

**Variables.** A call is answered from an entry only with an equal $\eta$, so
a rule that reads variables is memoized per combination of their values. The
exception of Left Recursion, where every call of a growing leader at its
position reads the current result whatever $\eta$ is, holds in both parses
alike.

**Order of Storing.** The theorem holds for every choice of which outcomes to
store, so deferring memoization to the second call at a position, or not
memoizing a rule that is called from one place, leaves the result unchanged.
