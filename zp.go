// Package zp is the public API of the ZeroParse parser framework.
//
// A grammar is written in the ZeroParse grammar language (see ParseGrammar), built as an AST with the grammar
// package, or decoded from JSON. Compile turns it into a Parser, and Parser.Parse parses input with it.
package zp

import (
	"context"
	"io"

	"zeroparse.dev/zp/grammar"
	"zeroparse.dev/zp/internal/engine"
	"zeroparse.dev/zp/internal/limits"
	ischema "zeroparse.dev/zp/internal/schema"
	"zeroparse.dev/zp/internal/syntax"
	"zeroparse.dev/zp/internal/value"
	"zeroparse.dev/zp/schema"
)

// Node is a value of a parse result: a node of the input with a range and a string, tuple or list
// value, an error value of recovery, or a value built by an action. It is opaque; its methods
// read it (Kind tells which of them apply). Nodes can be encoded to JSON with encoding/json.
type Node = value.Node

// NodeKind is what a Node stands for (Node.Kind).
type NodeKind = value.Kind

// The kinds of nodes.
const (
	KindString     = value.KindString     // a node whose value is a string: Text
	KindTuple      = value.KindTuple      // a node whose value is a tuple: Len and Elem
	KindList       = value.KindList       // a node whose value is a list: Len and Elem
	KindError      = value.KindError      // an error value of recovery: Text is its message
	KindStruct     = value.KindStruct     // a struct built by an action: Type and Field
	KindTupleValue = value.KindTupleValue // a tuple built by an action, without a range
	KindListValue  = value.KindListValue  // a list built by an action, without a range
	KindValue      = value.KindValue      // another value built by an action: Value
)

// SyntaxError reports that the input does not match the grammar.
type SyntaxError = engine.SyntaxError

// MissingTokenLimitError reports exhaustion of a #missing insertion budget.
type MissingTokenLimitError = engine.MissingTokenLimitError

// SyntaxErrors lists the syntax errors recovered with #recover or #missing.
type SyntaxErrors = engine.SyntaxErrors

// TruncatedSyntaxErrors reports a bounded tail of recovered stream errors.
// Total and Dropped are lower bounds when CountsSaturated is true.
type TruncatedSyntaxErrors = engine.TruncatedSyntaxErrors

// ParseGrammar parses ZeroParse source code into a grammar AST.
func ParseGrammar(src string) (*grammar.Grammar, error) {
	return syntax.Parse(src)
}

// Parser is a compiled grammar. It is safe for concurrent use by multiple goroutines.
type Parser struct {
	prog  *engine.Program
	start string
}

// CompileOption configures compilation.
type CompileOption func(*engine.Options)

// WithSourceInfo records source information in parse results: the rule that produced each node
// and where its captures are. Node.Rule, Node.Field, the rule and capture names of JSON and
// S-expression output, and rule constraints in queries need it. It costs time and memory, so
// parsers omit it by default.
func WithSourceInfo() CompileOption {
	return func(o *engine.Options) { o.SourceInfo = true }
}

// WithDefaultValue makes every rule produce the default value of its expression. Value actions
// (=>) and the declared types of rules are dropped, so the parser returns the default node tree
// even for grammars with actions, and nothing is checked against the types the actions would
// have made. Predicates still run, because they decide the match, and the rules that
// make the values predicates read keep their actions and types. It is a property of the
// parser: a compiled grammar written from it keeps it.
func WithDefaultValue() CompileOption {
	return func(o *engine.Options) { o.DefaultValue = true }
}

// Compile compiles the grammar g into a parser that starts parsing at the rule start, which must
// be a start rule, a rule marked @start. An empty start selects the default start rule.
func Compile(g *grammar.Grammar, start string, opts ...CompileOption) (*Parser, error) {
	var o engine.Options
	for _, opt := range opts {
		opt(&o)
	}
	prog, err := engine.Compile(g, o)
	if err != nil {
		return nil, err
	}
	if start == "" {
		start = prog.DefaultStart()
	} else if err := prog.CheckStart(start); err != nil {
		return nil, err
	}
	return &Parser{prog: prog, start: start}, nil
}

// DefaultValue reports whether the parser produces default values (WithDefaultValue).
func (p *Parser) DefaultValue() bool { return p.prog.DefaultValue() }

// SourceInfo reports whether the parser records source information (WithSourceInfo).
func (p *Parser) SourceInfo() bool { return p.prog.SourceInfo() }

// CompileSource compiles ZeroParse source code into a parser that starts parsing at the rule start, as
// Compile does.
func CompileSource(src, start string, opts ...CompileOption) (*Parser, error) {
	g, err := ParseGrammar(src)
	if err != nil {
		return nil, err
	}
	return Compile(g, start, opts...)
}

// PositionUnit is the unit of input positions: node Start and End (also in actions), syntax error
// positions and columns, and the len builtin.
type PositionUnit = engine.PositionUnit

const (
	// CodePoints counts Unicode code points. It is the default.
	CodePoints = engine.CodePoints
	// Bytes counts UTF-8 bytes.
	Bytes = engine.Bytes
)

// ParseOption configures a single parse.
type ParseOption func(*engine.ParseOptions)

// Limits bounds one parse invocation. A zero limit selects the default of that limit, and
// Unlimited disables it.
//
// Budget bounds the cumulative spend of the parse: every input unit
// inspected, every rule call and loop iteration, and every unit of value
// construction adds to one counter. The default is the larger of 2^20 and 1024
// units for each unit of input in the position unit of the parse. Counts depend
// on backend, input unit and version.
//
// MaxInput bounds the length of the input in the position unit of the parse. Its default is
// Unlimited. A longer input fails before any work with a LimitError of kind LimitInput; a stream
// fails when the parser reaches a unit beyond the limit.
type Limits = limits.Limits

// Unlimited is the value of a limit that disables it: Limits{Budget: zp.Unlimited}.
const Unlimited = limits.Unlimited

// LimitError describes a reservation that would exceed a limit.
type LimitError = limits.LimitError

// LimitKind identifies the exhausted limit.
type LimitKind = limits.LimitKind

const (
	// LimitBudget is the kind of an exhausted Budget.
	LimitBudget = limits.LimitBudget
	// LimitInput is the kind of an input longer than MaxInput.
	LimitInput = limits.LimitInput
)

// ErrLimit identifies limit failures through errors.Is.
var ErrLimit = limits.ErrLimit

// WithContext selects invocation-local cooperative cancellation. Nil disables it.
func WithContext(ctx context.Context) ParseOption {
	return func(o *engine.ParseOptions) { o.Context = ctx }
}

// WithLimits sets the invocation-local limits. A zero limit selects its default and Unlimited
// disables it.
func WithLimits(limits Limits) ParseOption {
	return func(o *engine.ParseOptions) { o.Limits = limits }
}

// WithMissingTokens bounds attempted #missing literal insertions in one parse.
// Zero disables insertion; negative limits are invalid. Only Closure whole-input
// parsing and recognition support insertion. Positive limits are invalid with
// ParsePrefix, even without #missing in the grammar. ParsePrefix also rejects
// grammars containing #missing when insertion is disabled. Options resolve in order.
func WithMissingTokens(limit int) ParseOption {
	return func(o *engine.ParseOptions) { o.MissingTokens = limit }
}

// WithExpectations records what a syntax error expects. A syntax error then lists the
// expectations at the farthest failure and carries the messages of @error. Without it the error
// reports the farthest failure position, an empty Expected list and no Messages, and parsing does
// not record expectations. A Document fixes it at construction.
func WithExpectations() ParseOption {
	return func(o *engine.ParseOptions) { o.Expectations = true }
}

// WithUnit selects the position unit. Matching always proceeds by code point, whatever the unit.
func WithUnit(u PositionUnit) ParseOption {
	return func(o *engine.ParseOptions) { o.Unit = u }
}

// Backend selects how a grammar is executed. All backends produce identical results.
type Backend = engine.Backend

const (
	// DefaultBackend uses Closure for a parser compiled from a grammar and Bytecode for one
	// loaded from a compiled grammar.
	DefaultBackend = engine.Default
	// Closure compiles parsing expressions into Go closures.
	Closure = engine.Closure
	// Bytecode runs language-independent bytecode (see spec/bytecode/README.md) on a VM that uses Go
	// recursion for rule calls.
	Bytecode = engine.Bytecode
	// BytecodeIterative runs the bytecode on a VM that keeps rule calls on its own stack, so deeply
	// nested input does not consume the Go stack.
	BytecodeIterative = engine.BytecodeIterative
)

// WithBackend selects the backend.
func WithBackend(b Backend) ParseOption {
	return func(o *engine.ParseOptions) { o.Backend = b }
}

// RecognizeOnly checks whether the input matches the grammar without building a tree. Parse then returns a
// nil node and the same syntax errors as a full parse. Actions are not evaluated, so runtime errors in
// actions are not reported; captures that predicates read are still built. Recognition is faster and
// allocates less than a full parse. It cannot be used with ParseStream or Document.
func RecognizeOnly() ParseOption {
	return func(o *engine.ParseOptions) { o.Recognize = true }
}

// WithMaxDepth limits the nesting of recursive rule calls to n; deeper nesting makes the parse fail with an
// error. Only the entries of rules that are on a cycle of calls (or call themselves) count; a rule outside
// every cycle adds a bounded, grammar-constant depth to the stack and does not count. The operand of a
// prefix operator and the right operand of an infix operator of an Operators Rule expression nest like
// recursive rule calls, and so does the entry of an Operators Rule.
// The default is 100,000 (10,000,000 for BytecodeIterative). The limit keeps runaway recursion from
// exhausting the stack or memory. The Closure and Bytecode backends nest recursive rule calls on the goroutine
// stack, so raising the limit far above the default can exceed Go's maximum stack size, which aborts
// the program; use BytecodeIterative for deeper nesting. A call answered from the memo does not nest,
// so whether a parse that comes close to the limit succeeds can depend on memoization.
func WithMaxDepth(n int) ParseOption {
	return func(o *engine.ParseOptions) { o.MaxDepth = n }
}

// TraceEvent describes the start (TraceEnter) or the end (TraceExit) of a rule call. See WithTrace.
type TraceEvent = engine.TraceEvent

// TraceKind tells whether a TraceEvent starts or ends a rule call.
type TraceKind = engine.TraceKind

const (
	// TraceEnter is reported when a rule is called, before the memo is consulted.
	TraceEnter = engine.TraceEnter
	// TraceExit is reported when a rule call returns.
	TraceExit = engine.TraceExit
)

// WithTrace calls f at the start and at the end of every rule call of the parse, in order, so that the
// events nest like the calls (the depth of an event is in TraceEvent.Depth). It reports the calls the
// backend makes: a call skipped because the next character cannot start the rule (first-character
// dispatch) is not reported, and the backends do not skip the same calls. Tracing does not change the
// result, but it makes the parse several times slower. The event's methods (LineCol, Text, Failure) read
// the parser's state, so call them from f; its fields can be kept. With several WithTrace options, each
// function is called in order. A nil f is ignored.
//
// Tracing works with every backend, with Parse, ParsePrefix, RecognizeOnly, ParseStream and Document (whose
// option applies to every Document.Parse), but not with generated Go parsers.
func WithTrace(f func(TraceEvent)) ParseOption {
	return func(o *engine.ParseOptions) {
		if f == nil {
			return
		}
		if prev := o.Trace; prev != nil {
			o.Trace = engine.ComposeTrace(prev, f)
			return
		}
		o.Trace = f
	}
}

// WithDiagnosticCallback reports each committed #recover error during ParseStream or Iterate,
// immediately before the corresponding stream element. Speculative recoveries are not reported.
// The callback receives an independent copy; returning an error stops parsing and suppresses
// that element. A nil callback disables delivery. Non-nil callbacks are invalid with Parse,
// ParsePrefix and Document. Recoveries in a successful suffix are reported before the terminal result. With or
// without this option, recovered errors remain available at stream completion.
func WithDiagnosticCallback(f func(*SyntaxError) error) ParseOption {
	return func(o *engine.ParseOptions) { o.DiagnosticCallback = f }
}

// WithDiagnosticLimit retains at most n committed stream recoveries, keeping the latest errors.
// Zero retains every error, as in an unconfigured stream parse. Negative values are invalid.
// This option is valid only with ParseStream and Iterate, including when n is zero.
// When errors are dropped, the terminal error is *TruncatedSyntaxErrors.
func WithDiagnosticLimit(n int) ParseOption {
	return func(o *engine.ParseOptions) {
		o.DiagnosticLimit = n
		o.DiagnosticLimitSet = true
	}
}

// Profile accumulates the cost of each rule over the parses given WithProfile: calls, body evaluations,
// memo hits, matches and failures, the input consumed and the input examined by failed calls, repeated
// evaluations at a position, and time. Hints summarizes where to look. A Profile is not safe for
// concurrent use.
type Profile = engine.Profile

// RuleProfile is the cost of one rule in a Profile.
type RuleProfile = engine.RuleProfile

// Location is a position with its line and column.
type Location = engine.Location

// WithProfile adds the cost of the parse to prof. It traces the parse (see WithTrace), so the parse is
// several times slower, and the times in prof are meaningful relative to each other only. A nil prof is
// ignored.
func WithProfile(prof *Profile) ParseOption {
	if prof == nil {
		return WithTrace(nil)
	}
	return WithTrace(prof.Trace)
}

func parseOptions(opts []ParseOption) engine.ParseOptions {
	var o engine.ParseOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// Parse parses the whole input and returns the value of the start rule.
// If the input does not match, it returns a *SyntaxError. If the parse recovered from errors with
// #recover, it returns both the node and the SyntaxErrors.
func (p *Parser) Parse(input string, opts ...ParseOption) (*Node, error) {
	return p.prog.ParseWith(p.start, input, parseOptions(opts))
}

// ParseStream parses input read from r and passes each element of the #stream repetition at the top level
// of the start rule to emit as soon as it matches. Emitted elements and the input before them are not
// retained, so the input never has to fit in memory. If emit returns an error, parsing stops and
// ParseStream returns that error. An element without a value (for example, one that is discarded with -)
// is passed as nil. As in any repetition, an element that matches without consuming input ends it.
// WithContext and WithLimits apply to this invocation. Cancellation cannot interrupt a blocked
// reader or callback; the caller must arrange for it to return. A returned reader or emit error wins
// over cancellation first observed after that call. Delivered elements remain caller-owned, and
// retrying an interrupted stream requires caller-managed input replay.
// WithDiagnosticCallback delivers committed recoveries immediately before their elements; its
// error stops parsing before that element is emitted. Recovered errors are still returned at EOF.
// WithDiagnosticLimit bounds the retained recovery tail and explicitly marks a truncated result.
func (p *Parser) ParseStream(r io.Reader, emit func(*Node) error, opts ...ParseOption) error {
	return p.prog.ParseStreamWith(p.start, r, emit, parseOptions(opts))
}

// Document is a text that is edited repeatedly. Parsing after an edit reuses the results of the previous
// parse that the edit did not affect (incremental parsing), as needed by editors. A Document is not safe
// for concurrent use.
//
// Reused results are shared with the trees returned by earlier parses, and the nodes after an edit are
// moved to their new positions in place: an earlier tree changes when Parse runs again, and is then a
// mix of nodes at new positions (those reused) and at old ones. Parse writes it, so it must not be read
// concurrently with Parse. Use Node.Clone to keep a tree as it was.
type Document struct {
	doc *engine.Document
}

// DocumentVersion identifies one text revision of a Document. Its zero value
// is invalid; tokens are comparable and do not retain document contents.
type DocumentVersion = engine.DocumentVersion

// TextEdit replaces [Start, End) in the document's position unit. EditBatch
// interprets every range in the supplied base version's coordinates.
type TextEdit = engine.TextEdit

// Version returns the current text revision, initially sequence one.
func (d *Document) Version() DocumentVersion { return d.doc.Version() }

// Unit returns the position unit selected when the document was constructed.
func (d *Document) Unit() PositionUnit { return d.doc.Unit() }

// EditBatch atomically applies ordered, nonoverlapping edits in base coordinates.
// It rejects stale or foreign tokens, repeated starts, invalid ranges and byte
// boundaries. An empty batch validates base but changes nothing. A nonempty
// batch advances one revision and clears parse caches without parsing.
func (d *Document) EditBatch(base DocumentVersion, edits ...TextEdit) error {
	return d.doc.EditBatch(base, edits...)
}

// Snapshot is an immutable document parse outcome.
type Snapshot = engine.Snapshot

// SnapshotNode is a comparable read-only node handle; its zero value is nil.
type SnapshotNode = engine.SnapshotNode

// SnapshotField stores a read-only node handle or immutable scalar.
type SnapshotField = engine.SnapshotField

// SnapshotCaptureError reports unsupported mutable or custom field values.
type SnapshotCaptureError = engine.SnapshotCaptureError

// Snapshot parses and freezes this revision using construction defaults plus
// opts. Cancellation and budget limits cover parsing, text, tree and diagnostics;
// interruption returns nil. Ordinary syntax errors retain the failed version.
func (d *Document) Snapshot(opts ...ParseOption) (*Snapshot, error) {
	o := d.doc.ParseDefaults()
	for _, opt := range opts {
		opt(&o)
	}
	return d.doc.SnapshotWith(o)
}

// ParseStats counts rule evaluations and reused results in a Document.Parse.
type ParseStats = engine.Stats

// NewDocument creates a Document for text. opts select its fixed unit and backend
// and the defaults for later Parse calls. Construction and each Parse use fresh
// budget counters. Edit uses the same unit and is outside parse budget controls.
// It returns an error if the backend is unavailable (Closure on a loaded parser) or the parser
// is loaded from a compiled grammar that is not in document mode.
func (p *Parser) NewDocument(text string, opts ...ParseOption) (*Document, error) {
	d, err := p.prog.NewDocumentWith(p.start, text, parseOptions(opts))
	if err != nil {
		return nil, err
	}
	return &Document{doc: d}, nil
}

// Text returns the current text.
func (d *Document) Text() string { return d.doc.Text() }

// Edit replaces the range [start, end) of the text, in the document's position unit, with text.
// With byte positions, start and end must be on character boundaries.
// Each invalid UTF-8 byte is a separate character; edits may join bytes into
// a valid UTF-8 character.
func (d *Document) Edit(start, end int, text string) error { return d.doc.Edit(start, end, text) }

// Parse parses the current text. opts apply once over construction defaults and
// may change context, budget limits, depth and trace, but not unit, effective backend
// or recognition mode. Overrides do not change future defaults. Pre-execution
// interruption preserves caches and statistics; execution or publication
// interruption discards reusable state while preserving text for retry.
// Parser-produced syntax diagnostics belong to that call: mutating them does
// not affect later parses. Returned Nodes may share the mutable Document graph.
func (d *Document) Parse(opts ...ParseOption) (*Node, error) {
	o := d.doc.ParseDefaults()
	for _, opt := range opts {
		opt(&o)
	}
	return d.doc.ParseWith(o)
}

// Stats returns the evaluation and reuse counts of the last Parse.
func (d *Document) Stats() ParseStats { return d.doc.Stats() }

// CompiledMode is the mode of a compiled grammar file, chosen when it is saved. A file holds the
// module of one mode and serves only the parses of that mode; the closure backend, Grammar and
// OutputSchema need the grammar source.
type CompiledMode = engine.CompiledMode

const (
	// CompiledParse serves Parse, ParsePrefix, ParseStream and Iterate without WithTrace or
	// WithProfile.
	CompiledParse = engine.CompiledParse
	// CompiledRecognize serves RecognizeOnly parses without WithTrace or WithProfile.
	CompiledRecognize = engine.CompiledRecognize
	// CompiledDocument serves NewDocument, and Parse, ParsePrefix, ParseStream and Iterate with
	// WithTrace or WithProfile.
	CompiledDocument = engine.CompiledDocument
)

// ParseCompiledMode returns the mode named s: "parse", "recognize" or "document".
func ParseCompiledMode(s string) (CompiledMode, error) { return engine.ParseCompiledMode(s) }

// MarshalBinary encodes the parser in parse mode (CompiledParse); see Marshal.
func (p *Parser) MarshalBinary() ([]byte, error) {
	return p.prog.Marshal(p.start, CompiledParse)
}

// Marshal encodes the parser in the .zpc format: the bytecode of mode and the start rule.
// LoadParser restores it without parsing, analyzing, type checking or compiling the grammar again.
// A loaded parser is saved only in its own mode (CompiledMode), which reproduces the file it was
// loaded from; saving another mode needs the grammar source.
func (p *Parser) Marshal(mode CompiledMode) ([]byte, error) {
	return p.prog.Marshal(p.start, mode)
}

// IsCompiled reports whether data is a compiled grammar written by MarshalBinary or Marshal.
func IsCompiled(data []byte) bool { return engine.IsCompiled(data) }

// LoadParser loads a parser saved by MarshalBinary or Marshal. Corruption is detected by a checksum and
// structural validation, but types are not checked again: load only data from trusted sources, such as
// the output of zp compile or MarshalBinary. The parser runs on the bytecode backends and serves
// the parses of the file's mode only; any other parse returns an error that names the mode it needs.
func LoadParser(data []byte) (*Parser, error) {
	prog, start, err := engine.LoadProgram(data)
	if err != nil {
		return nil, err
	}
	if start == "" {
		start = prog.DefaultStart()
	}
	return &Parser{prog: prog, start: start}, nil
}

// CompiledMode returns the mode of a parser loaded with LoadParser and true, or false for a parser
// compiled from a grammar, which serves every mode.
func (p *Parser) CompiledMode() (CompiledMode, bool) { return p.prog.Mode() }

// WithStart returns a parser that starts at the rule name, which must be a start rule; an empty
// name selects the grammar's default start rule. The compiled grammar is shared.
func (p *Parser) WithStart(name string) (*Parser, error) {
	if err := p.prog.CheckStart(name); err != nil {
		return nil, err
	}
	if name == "" {
		name = p.prog.DefaultStart()
	}
	return &Parser{prog: p.prog, start: name}, nil
}

// Start returns the name of the parser's start rule.
func (p *Parser) Start() string { return p.start }

// Starts returns the start rules of the grammar, the rules marked @start where parsing may begin,
// in declaration order. The one marked @start(default), or the only one, is the default.
func (p *Parser) Starts() []string { return p.prog.Starts() }

// DefaultStart returns the default start rule of the grammar, which Compile selects for an empty
// start.
func (p *Parser) DefaultStart() string { return p.prog.DefaultStart() }

// Grammar returns the parser's grammar, or nil for a parser loaded from a compiled grammar.
func (p *Parser) Grammar() *grammar.Grammar { return p.prog.SourceGrammar() }

// PrefixResult reports a rule's value and successful cursor. Consumption comes
// from the runtime cursor, even when Node is nil or its span differs.
type PrefixResult struct {
	Node          *Node
	Consumed      int // Selected parser Unit; defaults to CodePoints.
	ConsumedBytes int // Exact UTF-8 byte endpoint in the supplied input.
}

// ParsePrefix parses the named start rule at the beginning of input without requiring
// whole-input consumption; an empty name selects the grammar's default start rule. EOF assertions still refer to the complete input.
// Positions remain local to input. For a source slice, add its starting byte
// offset to ConsumedBytes to find the consumed endpoint in the original source.
// Recovered success returns the result and SyntaxErrors. Unrecovered syntax
// failures and fatal errors return a zero result. A zero-width success returns
// zero consumption; a nil Node does not imply failure. Parse remains whole-input.
// WithDiagnosticCallback and positive WithMissingTokens limits are invalid for
// this operation. Grammars containing #missing are unsupported even when
// insertion is disabled.
func (p *Parser) ParsePrefix(name, input string, opts ...ParseOption) (PrefixResult, error) {
	r, err := p.prog.ParsePrefixWith(name, input, parseOptions(opts))
	return PrefixResult{Node: r.Node, Consumed: r.Consumed, ConsumedBytes: r.ConsumedBytes}, err
}

// StreamIterator delivers one committed stream element per Next call.
// It is single-owner; Next and Close must not overlap. The reader remains
// caller-owned, and Close cannot interrupt a blocked Read.
type StreamIterator = engine.StreamIterator

// ErrStreamClosed identifies an iterator closed before its terminal result.
var ErrStreamClosed = engine.ErrStreamClosed

// Iterate validates a #stream parser and returns a lazy pull iterator. Reader
// access begins on Next; lookahead and buffering may still read ahead. With a
// diagnostic callback, Next reports committed recoveries before their element.
func (p *Parser) Iterate(r io.Reader, opts ...ParseOption) (*StreamIterator, error) {
	return p.prog.Iterate(p.start, r, parseOptions(opts))
}

// DocumentChangeKind distinguishes source operations, captured outcomes and
// lost history requiring a full refresh.
type DocumentChangeKind = engine.DocumentChangeKind

const (
	ChangeEdit        = engine.ChangeEdit
	ChangeOutcome     = engine.ChangeOutcome
	ChangeFullRefresh = engine.ChangeFullRefresh
)

// OutputChange is conservative whole-output comparison, never a minimal diff.
type OutputChange = engine.OutputChange

const (
	OutputUnchanged      = engine.OutputUnchanged
	OutputChanged        = engine.OutputChanged
	OutputMayHaveChanged = engine.OutputMayHaveChanged
	OutputUnavailable    = engine.OutputUnavailable
)

// DocumentChange owns an edit operation or an outcome comparison. Edit From/To
// advance source revisions; outcome From/To identify compared snapshots. Consumers
// track those streams separately. FullRefresh requires rebuilding current state.
type DocumentChange = engine.DocumentChange

// EnableChanges opts into bounded notifications and one retained comparison
// snapshot. queueLimit bounds record count, not replacement or snapshot bytes.
// It must be positive; enabling twice is rejected. Future Parse and Snapshot calls
// capture and compare immutable outcomes inside their cumulative spend.
// Comparison is conservative about aliases and nil-versus-empty collections;
// this policy does not define language value equality. Interrupted invocations
// return nil and notify unavailable, breaking the comparison chain.
func (d *Document) EnableChanges(queueLimit int) error { return d.doc.EnableChanges(queueLimit) }

// DrainChanges transfers queued events. Overflow produces one full-refresh
// marker; draining resets the queue but retains the last comparison snapshot.
func (d *Document) DrainChanges() []DocumentChange { return d.doc.DrainChanges() }

// CacheStats describes the incremental reuse history retained by a Document.
// EstimatedBytes is an approximation of cache-owned backing storage, excluding
// text, caller-held trees, shared strings and allocator overhead.
type CacheStats = engine.CacheStats

// ResetHistory discards incremental reuse history while preserving text,
// construction options, version and the statistics of the last parse. Earlier
// result trees are detached from later node movement.
func (d *Document) ResetHistory() { d.doc.ResetHistory() }

// CacheStats inspects retained incremental history without changing it.
// Document methods, including this one, require exclusive ownership.
func (d *Document) CacheStats() CacheStats { return d.doc.CacheStats() }

// WalkAction controls whether a walk descends, skips descendants, or stops.
type WalkAction = value.WalkAction

const (
	WalkContinue = value.WalkContinue
	WalkSkip     = value.WalkSkip
	WalkStop     = value.WalkStop
)

// WalkVisit describes a live value, its incoming edge and nearest owning node.
// Node is non-nil only for node values. Depth counts edges from the root.
type WalkVisit = value.WalkVisit

// WalkEdge identifies a root, child, field, or nested []any list edge.
type WalkEdge = value.WalkEdge

// WalkEdgeKind identifies the kind of a visit's incoming edge.
type WalkEdgeKind = value.WalkEdgeKind

const (
	WalkRoot        = value.WalkRoot
	WalkChild       = value.WalkChild
	WalkField       = value.WalkField
	WalkListElement = value.WalkListElement
)

// CycleError reports the root-to-back-edge path of a cyclic node or []any graph.
type CycleError = value.CycleError

// Cursor selects node spans in an explicitly supplied input position unit.
// At selects a point; Range selects a half-open range. It does not infer or
// convert units and does not prune children based on a parent's span.
type Cursor = engine.Cursor

// Walk visits a live node graph in pre-order: children in slice order, then
// fields in name order, and nested []any elements in index order. Scalar and nil
// values are visits too. Shared subtrees are visited once per incoming edge;
// active-path cycles return CycleError. The graph must not be mutated concurrently.
// A callback error returns unchanged; WalkStop succeeds and WalkSkip skips descendants.
func Walk(root *Node, visit func(WalkVisit) (WalkAction, error)) error {
	return value.Walk(root, visit)
}

// ErrSchemaUnavailable reports that checked grammar metadata is unavailable.
// In particular, a parser loaded from a compiled file cannot export a schema.
var ErrSchemaUnavailable = ischema.ErrUnavailable

type schemaOptions struct{ allRules bool }

// SchemaOption configures one output schema export.
type SchemaOption func(*schemaOptions)

// WithAllSchemaRules includes unreachable rules and declared types in the schema.
// By default, export starts at the parser's configured start rule.
func WithAllSchemaRules() SchemaOption {
	return func(o *schemaOptions) { o.allRules = true }
}

// OutputSchema exports checked grammar metadata. It does not inspect parse
// results or change parsing.
func (p *Parser) OutputSchema(opts ...SchemaOption) (*schema.Output, error) {
	if p == nil || p.prog == nil {
		return nil, ErrSchemaUnavailable
	}
	var o schemaOptions
	for _, opt := range opts {
		opt(&o)
	}
	return outputSchemaPrototype(p.prog, p.start, o.allRules)
}

// outputSchemaPrototype exports the start rule of prog, the rules it calls or whose values its
// values contain, and the struct types they refer to; or every rule and type with allRules. It
// has no effect on parsing or generation. A program loaded from a compiled grammar has no schema
// (engine.Program.SchemaProgram).
func outputSchemaPrototype(prog *engine.Program, start string, allRules bool) (*schema.Output, error) {
	sp, err := prog.SchemaProgram(start)
	if err != nil {
		return nil, err
	}
	return ischema.Export(sp, start, allRules)
}
