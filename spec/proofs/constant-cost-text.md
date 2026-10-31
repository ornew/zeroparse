# Constant-Cost Text

This proof shows that the [Conditions for Linear Cost](../runtime/parsing.md#conditions-for-linear-cost)
stay sufficient when taking the text of a node is not a copying operation,
given the guarantee of [Cost](../runtime/parsing.md#cost) that `text` costs a
constant in a parse of a whole input and in a parse of a Document. It also
shows that comparing two strings may be counted as copying only one of its
operands, and what a stream adds to the promises of
[Classification](../runtime/parsing.md#classification).

## Setting

Fix a grammar, a start rule and an input of length $n$, and let
$\hat{n} = n + 1$. "A constant" below means a number that depends only on the
grammar and the backend.

**Guarantee.** By [Cost](../runtime/parsing.md#cost):

- (G1) in a parse of a whole input, in bytes or in code points, valid UTF-8 or
  not, and in a parse of a Document, each evaluation of `text` costs at most a
  constant $c_t$, apart from the work of (G2);
- (G2) to make (G1) hold, a backend does work that adds up to at most
  $c_p \hat{n}$ in a parse, such as decoding an input that is not valid UTF-8,
  or copying the texts that cross the pieces of a Document until it joins the
  pieces, and only in a parse that takes such a text;
- (G3) materializing a text value as a string of the host language is not
  part of the cost;
- (G4) comparing two strings costs at most $c_t$ plus the length of the shorter
  operand;
- (G5) in a stream, each evaluation of `text` costs at most $c_t$ plus the
  length of its result.

**Two readings of the conditions.** The **copying reading** is the definition
of a copying operation that counts taking the text of a node as a copy of its
range, and the **refined reading** is the definition of
[Conditions for Linear Cost](../runtime/parsing.md#conditions-for-linear-cost),
which does not. The two readings differ only in the operations that are
copying operations; the size of a value, and so what a value includes in the
sense of condition 4, is the same in both: the result of `text` has the size
of its range and includes what that range includes.

**What the conditions bound.** Under the conditions, the cost of a parse is
bounded by bounding each part that [Cost](../runtime/parsing.md#cost) counts:
rule bodies, examined input, iterations and applied operators, and the size of
the values that actions and predicates build. A step of an action, a predicate
or a binding that costs a constant, outside a scanning source, is part of the
constant cost of the evaluation of the body that contains it: a body is a
finite expression, and outside its scanning sources each of its parts is
evaluated at most as many times as the upper bounds of the repetitions around
it allow. A step inside a scanning source is charged to the iteration, applied
operator or element of a list that evaluates it, and the conditions bound
those. These are the local and scanned cost of the
[Linear Cost proof](linear-cost.md#setting), which bounds them by its
[Lemma 1](linear-cost.md#lemma-1-local-cost) and
[Lemma 4](linear-cost.md#lemma-4-scanned-cost).

## Lemma 1 (Text Is a Constant Step)

*In a parse of a whole input or of a Document, the cost of all the
evaluations of `text` is at most $c_t$ times their number plus $c_p \hat{n}$,
and each evaluation is a step of constant cost of the evaluation that makes
it.*

*Proof.* By (G1) each evaluation of `text` costs at most $c_t$ besides the work
of (G2), which adds up to at most $c_p \hat{n}$ in the parse. By (G3), a string that the caller later reads from a text value
is not part of the cost. An evaluation of `text` is a step of an action, a
predicate or a binding, made either outside the scanning sources of a body, or
inside one, in an iteration, an applied operator, or a lambda for one element
of a list, and so it is counted as the other constant steps there are. The
argument of `text` is a node, a string or `nil`
([`text`](../language/actions.md#text)); `text` of a string returns it
unchanged and of `nil` returns `""`, which also cost a constant. $\square$

## Lemma 2 (Refined Reading Is Sound)

*If every rule that the start rule reaches satisfies conditions 1 to 5 in the
refined reading, then in a parse of a whole input or of a Document, the cost
is at most the bound that the conditions give in the copying reading for a
grammar in which every `text` is a constant step, plus $c_p \hat{n}$.*

*Proof.* Compare the copying reading on the grammar in which every `text` is a
constant step, that is, an operation that is not a copying operation and
whose result has the size of the range. The two readings of this grammar and
the refined reading of the given grammar agree on every condition:

- Condition 1 reads bindings, captures and rule values, which do not depend on
  the reading.
- Condition 2 depends on the scanning sources. A copying operation is a
  scanning source only when it has an operand not bounded in size by a
  constant of the grammar; in the refined reading `text` is not one, so it
  adds no scanning source, and every other source is the same in both.
- Condition 3 depends on the scanning sources too, and so agrees.
- Condition 4 constrains the copying operations, by the sizes of their
  operands. The copying operations other than `text` are the same, and the
  sizes are the same, because the result of `text` has the size of its range
  in both: an operation that joins, compares, converts or measures the text of
  a capture of a recursion still copies a value that includes the value of a
  call on that recursion, and condition 4 rejects it.
- Condition 5 does not mention values.

So the given grammar satisfies the conditions in the refined reading exactly
when the grammar with constant-step texts satisfies them in the copying
reading, and the bound that the conditions give applies to every part of the
cost of that grammar, where each `text` is a step of constant cost: part of
the local cost of [Lemma 1 of Linear Cost](linear-cost.md#lemma-1-local-cost)
outside a scanning source, and of the activation that makes it inside one. By Lemma 1,
the cost of the given grammar differs from it only by the work of (G2), at most
$c_p \hat{n}$. $\square$

The promise of the class Linear, $c \cdot \hat{n}$, and of the class Linear
Given an Environment Bound, $c \cdot K \cdot (L + 1) \cdot \hat{n}$, hold with
$c$ replaced by $c + c_p$, since $K \ge 1$ and $L + 1 \ge 1$.

## Lemma 3 (Comparison Counts One Operand)

*Counting a comparison of two strings as a copy of the operand whose size the
classifier bounds better is sound.*

*Proof.* By (G4) a comparison of strings $a$ and $b$ costs at most
$c_t + \min(|a|, |b|)$, which is at most $c_t + |a|$ and at most
$c_t + |b|$. Whichever operand the classifier counts, its size bounds the
cost beyond the constant, which is a step of constant cost as in Lemma 1. A
comparison with a literal costs at most $c_t$ plus the length of the literal,
a constant. $\square$

## Lemma 4 (Streams)

*In a stream, if every rule that the start rule reaches satisfies the
conditions in the refined reading, the cost is at most the bound of its class
plus the total length of the results of `text`.*

*Proof.* By (G5) an evaluation of `text` in a stream costs at most $c_t$, a
step of constant cost as in Lemma 1, plus the length of its result. The
argument of Lemma 2, with (G5) in place of (G1) and no work of (G2), bounds
every part of the cost except these lengths, which add their total. $\square$

## Theorem (Constant-Cost Text)

*With the guarantee of [Cost](../runtime/parsing.md#cost), the promises of the
classes Linear and Linear Given an Environment Bound of
[Classification](../runtime/parsing.md#classification) hold when the
classifier does not count taking the text of a node as a copying operation and
counts a comparison of strings as a copy of one operand: in a parse of a
whole input or of a Document as stated, and in a stream with the total length
of the results of `text` added.*

*Proof.* Lemma 2 gives the bound for a whole input and a Document, Lemma 3 the
counting of comparisons, and Lemma 4 the bound for a stream. $\square$

For a Document, the work of (G2) is the texts that cross two of its pieces:
they are copied, and once the copies since an edit would reach the length of
the document, its pieces are joined. The copies and the join cost at most
twice the length of the document between two edits, so the cost of a parse
after an edit includes at most that much, even when the parse reuses most of
its earlier results; a parse that takes only short texts across the pieces
pays only their lengths, and one that takes none pays nothing.
