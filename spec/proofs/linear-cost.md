# Linear Cost

Arata Furukawa · Last revised 2026-11-01

This proof shows that the [Conditions for Linear Cost](../runtime/parsing.md#conditions-for-linear-cost)
bound the [cost](../runtime/parsing.md#cost) of every parse, so that the
classes Linear and Linear Given an Environment Bound of
[Classification](../runtime/parsing.md#classification) hold their promise. It
splits the cost into the parts that the specification counts and bounds each
part by one lemma, so that a new condition is added as a lemma, or as a case of
one, without changing the others.

Lemmas 1, 2, 7 and 8 are proved, and the theorem is proved from the others.
Lemmas 3 to 6 are given as outlines: each states its claim, the conditions it uses,
and the steps that its proof must still make precise, listed under
[Open Steps](#open-steps) with the step that the theorem needs besides them.

## Setting

Fix a grammar and a start rule such that every rule that the start rule
reaches satisfies conditions 1 to 5, and an input of length $n$. Let $K$ and
$L$ be as in [Classification](../runtime/parsing.md#classification), and
$\hat{n} = n + 1$. The parse is the memoized parse of the
[memoization proof](memoization.md#setting) with the memoization of
[Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules).
"A constant" below means a number that depends only on the grammar and the
backend.

**Cost.** By [Cost](../runtime/parsing.md#cost), the cost of a parse is
$C = C_{\mathrm{body}} + C_{\mathrm{in}} + C_{\mathrm{it}} + C_{\mathrm{val}}$:
the rule bodies evaluated with each growth step counted, the input that
terminals, repetitions of character classes and lookaheads examine, the
iterations of repetitions and the operators applied, and the size of the
values that actions and predicates build.

**Input that a lookahead examines.** This proof counts each examination of
the input once, for the terminal, character class or `.` that makes it,
whether or not a lookahead encloses it. The cost of a lookahead itself is
then a constant for entering and leaving it, and what its expression costs is
counted for that expression: its scanning sources, such as the repetition in
`&([^!]* "!")`, are scanning sources of the body, and its other parts are
local cost. [Cost](../runtime/parsing.md#cost) lists the input that lookaheads
examine beside the input that terminals examine. If a lookahead were to count
again what the terminals inside it examine, the count would grow with the
number of lookaheads that enclose a terminal, which rule calls do not bound.

**Activations.** A growth step is a body evaluation of the leader. An
**activation** of a scanning source is one unit of its work that evaluates a
fixed part of the grammar once: an iteration of a repetition without an upper
bound, an operator that an Operators Rule applies, or an evaluation of the
skip expression of a `#recover`. The growth of a cycle and the copying
operations, the other scanning sources, make no calls of their own beyond the
body evaluations of the growth.

**Local and scanned cost.** An evaluation of a rule body has a **local cost**,
the cost of the steps that it makes outside the calls it makes and outside its
scanning sources, and a **scanned cost**, the cost of the steps of its
scanning sources outside the calls they make. Each call is counted once, in
the evaluation that makes it, as a body evaluation of the callee, the steps of
a growth, or the reuse of an entry. Every unit of $C$ belongs to exactly one
of these: the local or scanned cost of one body evaluation, or one call.

## Lemma 1 (Local Cost)

The local cost of one body evaluation is at most a constant $c_0$ times
$L + 1$.

*Proof.* Outside its scanning sources, a body contains no repetition without
an upper bound, no `#recover` skip, no operators of an Operators Rule, and no
copying operation whose operand is unbounded in size other than the value of a
variable ([Conditions for Linear Cost](../runtime/parsing.md#conditions-for-linear-cost),
the definition of a scanning source). The body is a finite expression, and
each of its parts is evaluated at most as many times as the product of the
upper bounds of the repetitions that enclose it, a constant. Each such
evaluation outside a call costs a constant, except a copy of the value of a
variable, which costs at most $L$ by the definition of $L$. $\square$

## Lemma 2 (Calls)

*The number of calls is at most a constant times the number of body
evaluations, growth steps included, plus the number of activations.*

*Proof.* Every call is made by a step of a body evaluation, growth steps
included. That step is either outside the scanning sources of the body, or
within an activation of one of them, outside the activations of the scanning
sources nested in it. By the argument of Lemma 1 the first kind makes a
constant number of calls per body evaluation. An activation evaluates a fixed
part of the grammar once, and outside its nested scanning sources that part
makes a constant number of calls, again by the argument of Lemma 1. $\square$

## Lemma 3 (Body Evaluations)

*Outline.* The number of body evaluations, growth steps excluded, is at most a
constant times $K \hat{n}$ plus the number of calls of rules that are not
memoized.

The memo evaluates the body of a memoized rule at most twice for each
position, level and combination of the values of the variables it reads
([Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules)), which is
$2 \cdot |\text{rules}| \cdot |\text{levels}| \cdot K \hat{n}$. Each exception
of that section is accounted for elsewhere:

- a rule that is not memoized is evaluated once for each of its calls, which
  open step 7 bounds;
- a call of a rule of a cycle in the context of growths at its position that
  finds no result computed with the same growing results and the same outcomes
  of the lookaheads it observed is evaluated again; such calls are a constant
  number per growth step, which Lemma 5 counts;
- a result that read the growing result of an open growth is evaluated again
  in the steps whose growing results differ, which Lemma 5 counts;
- a leader that grows again at its position, inside another growth there, is
  counted by Lemma 5, under condition 5.

## Lemma 4 (Scanned Cost)

*Outline.* Under condition 2, the scanned cost of all body evaluations, together
with the activations of their scanning sources, is at most a constant times
$K(L + 1)\hat{n}$.

For the walks of a [skip-run site](../runtime/parsing.md#skip-runs) in the
skip expression of a `#recover`, the [Skip Runs proof](skip-runs.md#lemma-2-cost)
bounds the evaluations of the element by $2(n + 1)$ plus a constant for each
walk, and each walk is one activation of the skip expression. That lemma is
the case of this one for skip-run sites.

The proof charges each step of a scanning source to a slot of the input that
it examines, or to the iteration or operator that makes it, and shows that
each slot is charged a constant number of times for each combination of the
values of variables. A slot is charged again only when a scanning source that
examined it is evaluated again over it, which happens when a unit that
contains the source fails after the source examined the slot, and the parser
continues at the start of the unit. Condition 2 requires that such a unit be
committing, which gives the following cases, one lemma each.

1. *Disjoint first characters.* The continuation does not consume the
   character at which it and the unit part, so nothing evaluated after the
   failure starts again inside the input that the unit examined beyond its
   shared calls.
2. *Shared calls.* A call that the unit and a continuation both evaluate first
   has the same result in both, by the [memoization theorem](memoization.md#theorem-memoization-never-changes-the-result)
   when the rule is memoized, or by repeating only its own scan when it calls
   no rule, so the continuation pays for it at most what the unit paid.
3. *No source after the shared calls.* The input that the unit examines beyond
   its shared calls is bounded by Lemma 1.
4. *Words that exclude.* The continuation fails, or stops examining, before the
   end of the word with which the unit began, which the unit examined at most
   once more, with the character after it.

A unit that fails only before its sources examined input gives back no more
input than its parts before the sources examined, which Lemma 1 bounds. A unit
that succeeds is not restarted by its own continuations, and a failure after
it is a failure of an enclosing unit, to which the same cases apply. The
expression of a lookahead is abandoned whether it matches or not, so it is
checked as a unit in every case.

## Lemma 5 (Growth)

*Outline.* Under conditions 2 and 3, the growth steps of all growths, with the
local and scanned cost of the bodies evaluated in them, cost at most a
constant times $K(L + 1)\hat{n}$ more than Lemma 4 counts.

A growth at $p$ takes at most two more steps than the length it consumes
([Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules)). By
condition 3 no step evaluates a scanning source at $p$ outside the left call
that grows and outside memoized calls of rules off the cycle, so each step
costs a constant at $p$, and its growing units examine the input from the end
of the seed. The input from the end of one seed to the end of the next is
examined by one step, and what a step examines beyond the end of its own
result is the failure of a growing unit after a scanning source, which
condition 2 checks. A growth is itself a scanning source at each call of a
rule of its cycle other than a left call, so its repetition at another
position is counted by Lemma 4.

A leader R called at $p$ in a step of the growth of another leader S of its
cycle grows there in the context of that growth. A step of R from a result
that read no growing result of S, or the same one, is the step that an earlier
growth of R took from it: equal results of a step share their identity, the
steps are reused, and a run of steps that read only R's own result is taken at
once, so these steps cost a constant for each step of S beyond their first
evaluation. A growth of R from a result that includes the growing result of S
is new in each step of S. By condition 5 either the step of S keeps the result
of R, and the input that R examines past the end of the seed of S is the input
that this step consumes, or R grows past that end only where the growth of S
stops, which happens once per growth.

## Lemma 6 (Values and Copies)

*Outline.* Under conditions 1 and 4, the size of the values that actions and
predicates build is at most a constant times $K(L + 1)\hat{n}$.

A copy of a value that does not include the value of a call on its own
recursion, nor a fold accumulator, does not add up over the levels of a
recursion; when it is unbounded it is a scanning source, which Lemma 4 counts.
A copy in the lambda of `map`, `filter`, `foldl` or `foldr` over a list that
is not bounded runs once per element and copies at most a constant from
outside the lambda; over a bounded list, Lemma 7 makes the copies of the call
those of a constant number of copies outside the lambda. A copy of the
value of a variable costs at most $L$. Condition 1 keeps the value of a
variable from growing from its own earlier value, or, by Lemma 8, keeps it in
a set of constant size, so the number of combinations at a position is $K$, a
constant when every binding is a literal or binds a variable with finite
values.

Taking the text of a node is not a copy: by the
[Constant-Cost Text proof](constant-cost-text.md#lemma-1-text-is-a-constant-step)
it is a step of constant cost, apart from work of at most a constant times
$\hat{n}$ in the whole parse, which the constant of the theorem absorbs. A
stream pays the length of each text besides, as
[Lemma 4 of that proof](constant-cost-text.md#lemma-4-streams) states.

## Lemma 7 (Lambdas Over Bounded Lists)

Let a call of `map`, `filter`, `foldl` or `foldr` satisfy condition 4 and
have a [bounded](../runtime/parsing.md#conditions-for-linear-cost) list, with at most
$m$ elements, where $m$ is a constant. In one evaluation of the call, let $I$
be the size of the initial value of a fold ($I = 0$ otherwise), $E$ the total
size of the elements, and $S$ the total size of the values that the lambda
reads other than its parameters, each measured as condition 4 measures what a
copy of it copies. Then the call evaluates the lambda at most
$m$ times, the first parameter of a fold has size at most $a^m (I + E + S + d)$,
and the copying operations of the lambda cost at most $c (I + E + S + 1)$, for
constants $a$, $c$ and $d$.

*Proof.* By [Built-In Functions](../language/actions.md#built-in-functions),
the call evaluates its lambda once for each element of the list, so at most
$m$ times. Let $r$ be the number of references to parameters, captures and
variables in the lambda, and $d$ the total size of its literals; both are
constants. A value that the lambda builds is made of the values that it
references and of its literals, so its size is at most $r$ times the largest
value that it references, plus $d$. This holds as well when the lambda
contains a call with a lambda of its own, by induction on the nesting of
lambdas: over a bounded list, by this lemma for the inner call, with larger
constants; over a list that is not bounded, the inner lambda copies nothing
from outside it that is not bounded by a constant, nor its fold accumulator,
by condition 4, so the value of the inner call is at most a constant times the
sizes of its list and its initial value, which the lambda references. One evaluation of the lambda references
the first parameter, an element and the values counted in $S$, so with $A$ the
size of the first parameter and $B = E + S + d$, its value has size at most
$r(A + B) + d \le r A + (r + 1) B$, and its copying operations, each of which
costs at most a constant times the sizes that it copies, cost at most a
constant times $A + B$.

In a fold, let $A_j$ be the size of the first parameter after $j$ steps, so
$A_0 = I$ and $A_{j+1} \le r A_j + (r + 1) B$. With $a = 2r + 1$, induction
gives $A_j \le a^j (I + B)$: the case $j = 0$ holds, and
$A_{j+1} \le r a^j (I + B) + (r + 1) B \le (2r + 1) a^j (I + B)$. So
$A_j \le a^m (I + B)$ for $j \le m$. The first parameter of a `map` or a
`filter` is an element, of size at most $E$. Summing the copying operations
over at most $m$ evaluations gives at most a constant times
$m (a^m (I + B) + B)$, which is $c (I + E + S + 1)$ for a constant $c$.
$\square$

When the value of the lambda holds the first parameter at most once,
counting the copies in its elements and in the elements of the lists it
builds, so that its total size is at most that of the first parameter plus
$(r + 1) B$, the recurrence is $A_{j+1} \le A_j + (r + 1) B$, so $A_j \le I + j (r + 1) B$,
which grows linearly with $m$ rather than as $a^m$. Condition 4 applies the
lemma only to such folds: a fold whose lambda can hold the first parameter
twice multiplies it by a factor that is exponential in $m$, a constant that
a bound such as `#repeat(,30)` makes larger than any input.

The copies of such a call are therefore those of a constant number of copies,
in the context of the call, of the initial value, the list and the values from
outside the lambda. This is how condition 4 checks them: they repeat once for
each element only when a lambda around the call is over a list that is not
bounded, and the first parameter of a fold includes what the initial value
and the value of the lambda include, so that a copy of it that includes the
value of a call on the recursion is checked as such a copy. The lemma itself
needs $a^m$ only to be a constant; the restriction to folds that hold the first
parameter once keeps the constant of the class proportional to $m$.

## Lemma 8 (Finite Values)

Let $v$ be a variable with
[finite values](../runtime/parsing.md#conditions-for-linear-cost) and $S_v$
its set of values, of 2 values for a `bool`; the product of the value sets of
all tracked variables is at most 128.
Then $v$ holds a value of $S_v$ at every step of every parse, and the
variables with finite values contribute at most the product of their $|S_v|$,
a constant, to $K$.

*Proof.* For a `bool`, $S_v$ is the set of both values, and the claim holds.
For an `int`, by induction on the steps of the parse. By
[Variables](../language/predicates.md#variables), the parse starts with $v$
bound to its initial value, which is in $S_v$, and afterwards a lookup of $v$
gives the nearest binding of $v$ that has not been undone, or the initial
value. Bindings made in a called rule are not visible to its caller, and
backtracking undoes bindings, so every value that a lookup gives is the
initial value or a value that an evaluation of a binding bound earlier.

Let a binding $b$ of $v$ be evaluated, and let $x$ be the value of $v$ that
it reads. We show that $x$ is one of the values that reach $b$, computed from
$S_v$. Let $F$ be the outermost sequence that $b$ is in through groups,
options, choices, captures, discards and atomic expressions only. In the
evaluation of the rule body that evaluates $b$, $F$ starts with a value $x_0$
of $v$ that is in $S_v$ by the induction hypothesis, and the earlier elements
of $F$ and of the sequences inside it that hold $b$, then the earlier
statements of the block of $b$, have each been evaluated once on this path
and succeeded, in order, or $b$ would not be evaluated. Each step maps a set
that holds the value of $v$ before it to a set that holds the value after it:

- a binding of $v$ binds a value that its evaluation gives from the value
  before it, which the step computes as for $b$ below;
- a predicate that reads only $v$ and literals succeeded, so the value passes
  it; the step keeps every value on which its evaluation is true or not known;
- a choice succeeds through one of its alternatives and an option through its
  expression or by matching nothing, so the value after it is in the union of
  the values after its paths;
- a negative lookahead undoes its bindings, and a call returns with the
  value before it, since bindings in a called rule are not visible to its
  caller;
- any other element that binds $v$ leaves a value that $v$ holds at some step
  of the parse, which is in $S_v$ by the induction hypothesis; an element that
  does not bind $v$ keeps it.

So $x$ reaches $b$. The binding reads only $v$, literals and operands
`len($c)`, and the length of the value of each capture $c$ is at most its
bound, since the value is the text of an expression that matches at most that
many characters, a list or tuple with at most that many elements, or the
value of a rule whose action gives such a value. A sequence builds a tuple of
the values of its items, in which a plain group of a sequence is replaced by
the elements of its own tuple, so the tuple has at most the number of items
counted with those groups spliced in; a sequence whose value is that of a
single item has the bound of that item. The value of $b$ is
therefore determined by $x$ and lengths within the bounds, by the rules of
[Operators and Functions](../language/actions.md#operators-and-functions),
for ints within $2^{53}$ in magnitude, which every backend computes alike:
it is in $S_v$, which holds the value that $b$ binds from each value that
reaches it at each combination of lengths, or the evaluation is an error and
binds nothing.

The values of the variables that memoized rules read at a position are
therefore drawn, for the variables with finite values, from the product of
their sets, so they contribute at most the product of the $|S_v|$ to the
number of combinations $K$. $\square$

The bound of 128 on the product keeps the constant $K$ small: a memoized
rule can be evaluated once for each combination of values at a position. The
bounds of 256 for a length and of 4096 combinations of lengths keep $S_v$
cheap to compute. The proof needs only that the bounds are finite. The values that reach a binding depend on the order of the elements
before it: a test after the binding, or after a call that follows it, has not
been evaluated when the binding reads $v$, so it filters nothing that the
binding reads.

## Theorem (Linear Cost)

When every rule that the start rule reaches satisfies conditions 1 to 5, every
parse from the start rule has a cost of at most $c \cdot K \cdot (L + 1) \cdot (n + 1)$
for a constant $c$. When no variable holds a string and every binding binds a
literal or a variable with finite values, $K$ is a constant by Lemma 8 and
$L = 0$, so the cost is at most $c' \cdot (n + 1)$.

*Proof, given Lemmas 3 to 6 and open step 7.* Each unit of $C$ is the local
cost of a body evaluation, the scanned cost of one, or a call. By Lemma 1 the
local costs add up to $c_0 (L + 1)$ times the number of body evaluations, which
Lemmas 3 and 5 bound by a constant times $K \hat{n}$ plus the evaluations of
rules that are not memoized. Open step 7 bounds those by a constant times
$K \hat{n}$ plus the activations. By Lemma 2 the calls are at most a constant
times the body evaluations plus the activations, which Lemmas 4 and 5 bound.
The scanned cost is bounded by Lemmas 4 and 5 and the values by Lemma 6. Each
bound is a constant times $K (L + 1) \hat{n}$. $\square$

The classifier reports the two classes only when the conditions hold, and it
over-approximates where it cannot decide them
([Classification](../runtime/parsing.md#classification)). That it does so is a
property of its implementation, checked by its tests; this proof covers the
step from the conditions to the bound.

## Adding a Condition

A new condition enters the proof in one of two ways.

- A new way for a unit to be committing is a new case of Lemma 4. Its lemma
  shows that after the unit is abandoned, what the parser evaluates next
  examines again at most a constant amount of the input that the unit examined
  beyond its shared calls.
- A new bound on a part of the cost, or on a cost that the specification
  starts to count, is a new lemma with the part that it bounds, and the
  theorem adds its bound.

## Open Steps

The outlines leave these steps to be made precise.

1. **Charging in Lemma 4.** The scheme that assigns each scanned step to a
   slot, and the proof that a slot is charged a constant number of times,
   across nested units, rule calls and the two evaluations that the memo
   allows at a position. The specification argues each case for one unit; the
   proof must combine them over the units that enclose one another.
2. **"A unit that succeeds is not restarted."** A body that contains the unit
   can itself be evaluated again: by the memo, at most twice at a position;
   when its rule is not memoized, at each of its calls. The proof must show
   that these evaluations are counted by Lemma 3 and do not multiply the
   scanned cost beyond a constant.
3. **Continuations of growing units.** The case where the first step of a
   growth matches a part before the left call, as through `F` in
   `E = F "+" X / Y; F = E / M`, and the unit is also checked as an
   alternative that is not growing.
4. **Operators Rules.** The units of an Operators Rule, its `skip`, and right
   operands that apply operators themselves, in Lemmas 4 and 5.
5. **Lemma 3 and left recursion.** The proof must bound, in Lemma 5, the
   evaluations that a context of growths repeats: the calls that find no result
   with the same growing results and lookahead outcomes, and the growths of
   leaders inside other growths at their position, whose steps are shared only
   through equal results. It must show that the comparison of results by
   structure, which is bounded, loses only reuse, and that condition 5 covers
   every growth that is new in each step.
6. **Variables in Lemma 6.** The bound $L = O(n)$ that
   [Classification](../runtime/parsing.md#classification) states, and the
   comparisons of memo keys that hold strings, which cost up to $L$ per lookup.
7. **Rules that are not memoized.** A rule that calls no rule is a leaf of the
   calls and is evaluated once per call, which Lemma 2 counts. A rule called
   from at most one place is not memoized either, and it can be recursive
   through the start rule, which is entered without a call: when that place is
   its own body, as in `main = "a" main "b" / "x"`, or when it lies on a cycle
   of rules that are each called from one place and that passes through the
   start rule, as in `main = "a" B / "x"; B = main "b"`. Their calls then form
   a recursion that Lemma 3 does not bound, so counting their evaluations by
   their calls, and their calls by Lemma 2, is circular. The step must bound
   the evaluations of such rules directly. A cycle of calls that consume no
   input before them is a left recursion. The reference implementation always
   memoizes its leaders, since a growth keeps its seed in the memo table, but
   the first exception of
   [Evaluations of Rules](../runtime/parsing.md#evaluations-of-rules) does not
   exempt a leader called from one place, such as the start rule in
   `main = main "a" / "a"`. Once the specification states that leaders are
   memoized, each call on such a cycle follows input that a body consumed, and
   the calls along one chain are at increasing positions. A call inside an unbounded
   repetition of a body makes a tree of calls instead, which the activations of
   Lemma 4 must account for.
