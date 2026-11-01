<div align="center">

<h1>
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="site/static/assets/logo-white.svg">
  <img src="site/static/assets/logo-black.svg" alt="ZeroParse" height="60">
</picture>
</h1>

**One Grammar. Every Parser.**

[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENCE) [![Go Reference](https://pkg.go.dev/badge/zeroparse.dev/zp.svg)](https://pkg.go.dev/zeroparse.dev/zp) [![Go](https://img.shields.io/badge/go-1.27.1+-00ADD8?logo=go&logoColor=white)](go.mod)

[Website](https://zeroparse.dev/) · [Playground](https://zeroparse.dev/playground/) · [Quick start](docs/tutorial/getting-started.md) · [Guides](docs/guide/README.md) · [Specification](spec/README.md) · [Examples](examples/)

</div>

ZeroParse turns one grammar into every parser you need: generated code or a VM, fast, safe on any input and ready for
your editor. Never write a parser again.

```zp
// calc.zp
type Num = node[string]
type Bin = { Left Expr; Op node[string]; Right Expr }
type Expr ~ Bin | Num

@start
rule main Expr = #operators {
    skip " "*
    operand num
    operand "(" e:main ")" => $e
    level additive { infix left "+" / "-" => Bin{Left: $lhs, Op: $op, Right: $rhs} }
    level multiplicative { infix left "*" / "/" => Bin{Left: $lhs, Op: $op, Right: $rhs} }
    level power { infix right "^" => Bin{Left: $lhs, Op: $op, Right: $rhs} }
}

rule num Num = ([0-9]+) #atomic
```

```bash
zp parse -g calc.zp -f sexpr -i '1 + 2 * 3 ^ 2'
```

```text
Bin{Left: Num("1"), Op: "+", Right: Bin{Left: Num("2"), Op: "*", Right: Bin{Left: Num("3"), Op: "^", Right: Num("2")}}}
```

```bash
zp parse -g calc.zp -i '1 + * 2'
```

```text
zp: 1:5: syntax error: expected "(", [0-9]
```

> [!NOTE]
> ZeroParse is pre-release: the language, the Go API and the file formats may change. Generated Go is not yet as fast
> as Go's hand-written parsers, and closing that gap is the current work. [Why ZeroParse](docs/why-zeroparse.md) has every
> measured number, the current limits and how ZeroParse compares with other tools.

## Why ZeroParse

- **Runs anywhere, any way.** Any target language, any engine, any optimization. One grammar becomes generated code,
  a VM or an interpreter, tuned for speed, memory or size, and every one of them returns the same tree, the same
  positions and the same errors. Whatever you build, ZeroParse fits.
- **Unmatched expressive power.** Build values of any type you define, carry state for context-sensitive syntax and
  write a whole operator precedence table as one rule. Recover from errors with a complete tree, and parse endless
  streams as they arrive. If you can describe a language, you can parse it.
- **Performance pushed to the limit.** A multi-stage IR optimizes at every level, from grammar-wide rewrites down to
  the generated code. Incremental parsing reuses everything an edit did not touch, so a fresh tree is ready between
  keystrokes.
- **Safe on any input.** ZeroParse analyzes your grammar to bound the time and memory a parse can take, and built-in
  budget and nesting limits mean hostile input can never hang the parser or blow its stack.
- **Complete support for building your language.** A built-in language server brings completion, diagnostics,
  navigation and formatting to any editor. Trace, profile, lint and debug show exactly what the parser does, so you
  never have to guess.

## Install

```bash
go get zeroparse.dev/zp                        # library
go install zeroparse.dev/zp/cmd/zp@latest    # command-line tool
```

Go 1.27.1 or later. No dependencies outside the standard library.

## Use It

**From Go**

```go
p, err := zp.CompileSource(grammarSource, "main") // grammar errors come with positions

node, err := p.Parse(input)                       // positions in code points by default
p.Parse(input, zp.WithUnit(zp.Bytes))         // ... or in UTF-8 bytes
p.Parse(input, zp.WithBackend(zp.Bytecode))   // pick a backend
p.Parse(input, zp.RecognizeOnly())              // validate without building a tree
json.Marshal(node)                                // nodes encode with types and positions

err = p.ParseStream(r, func(n *zp.Node) error { return nil }) // one #stream element at a time

doc, _ := p.NewDocument(text)
doc.Edit(10, 12, "new text") // replace [10, 12)
node, err = doc.Parse()      // reuses rule results the edit did not reach

data, _ := p.MarshalBinary() // save; zp.LoadParser(data) skips parsing and checking
```

**From the command line**

```bash
zp parse -g grammar.zp -i 'abc=12'           # parse and print the tree (JSON, or -f sexpr)
zp parse -g grammar.zp -check < input.txt    # validate only
zp explain -g grammar.zp < input.txt         # which rule calls make up a syntax error
zp trace -g grammar.zp -i 'abc=12'           # every rule call as an indented tree (-f json for tools)
zp profile -g grammar.zp < input.txt         # cost per rule, with hints on wasted work
zp lint -g grammar.zp                        # likely mistakes: dead alternatives, unused captures, ...
zp lsp                                         # language server for editors (errors, formatting, rename, ...)
zp fmt -w grammar.zp                         # format in place, comments kept
zp fix -w grammar.zp                         # rewrite older syntax to the current one
zp compile -g grammar.zp -o grammar.zpc    # precompile
zp gen -g grammar.zp -module example.com/calc -o calc  # generate a standalone Go parser
zp gen -g grammar.zp -module example.com/calc -types -o calc # ... with Go types for the grammar's types
zp gen -g grammar.zp -module example.com/calc -recognize -o calc # ... with Recognize, which validates without a tree
zp gen -lang ts -g grammar.zp -o ts-parser    # generate a TypeScript parser into a directory
zp gen -lang ts -g grammar.zp -types -o ts-parser # also emit types.ts and parseAST
zp gen -g grammar.zp -module example.com/calc -max-depth 600000 -o calc # set its nested rule-call default
zp sample -g grammar.zp -n 20 -coverage      # generate inputs the grammar accepts (tests, fuzz seeds)
zp edit-fuzz -g grammar.zp -minimize -save-failures out  # check Document edits against fresh parses
```

Run `zp` without arguments for every command and flag.

## Ready-Made Parsers

[alpha-parsers](https://github.com/zeroparse/alpha-parsers) (a separate repository) has parsers for common languages, written in ZeroParse, generated into Go and tested against the
languages' conformance suites and reference implementations. They are packages of one module that depends only on the
standard library and ZeroParse's runtime package. Use them from a checkout of that repository; `go get` does not fetch
them yet (see [Ready-Made Parsers](docs/guide/ready-made-parsers.md)):

```go
import "zeroparse.dev/alpha-parsers/json"

v, err := json.Decode(`{"name": "zp", "tags": ["peg", "operators-rule"]}`)
```

| Language | Module | Specification |
|:--|:--|:--|
| CEL | [`zeroparse.dev/alpha-parsers/cel`](https://github.com/zeroparse/alpha-parsers/tree/main/cel/) | The CEL language definition |
| CSV | [`zeroparse.dev/alpha-parsers/csv`](https://github.com/zeroparse/alpha-parsers/tree/main/csv/) | RFC 4180 |
| CUE | [`zeroparse.dev/alpha-parsers/cue`](https://github.com/zeroparse/alpha-parsers/tree/main/cue/) | The CUE language specification |
| DuckDB SQL | [`zeroparse.dev/alpha-parsers/duckdb`](https://github.com/zeroparse/alpha-parsers/tree/main/duckdb/) | DuckDB 1.5.6's SQL dialect |
| Go | [`zeroparse.dev/alpha-parsers/golang`](https://github.com/zeroparse/alpha-parsers/tree/main/golang/) | The Go specification (go1.27), as `go/parser` parses it |
| JSON | [`zeroparse.dev/alpha-parsers/json`](https://github.com/zeroparse/alpha-parsers/tree/main/json/) | RFC 8259 |
| Python | [`zeroparse.dev/alpha-parsers/python`](https://github.com/zeroparse/alpha-parsers/tree/main/python/) | Python 3.14 |
| TypeScript | [`zeroparse.dev/alpha-parsers/typescript`](https://github.com/zeroparse/alpha-parsers/tree/main/typescript/) | TypeScript 5.9 and TSX |
| XML | [`zeroparse.dev/alpha-parsers/xml`](https://github.com/zeroparse/alpha-parsers/tree/main/xml/) | XML 1.0 (Fifth Edition), non-validating |
| YAML | [`zeroparse.dev/alpha-parsers/yaml`](https://github.com/zeroparse/alpha-parsers/tree/main/yaml/) | YAML 1.2.2 |

## Examples

Small grammars with tests that show the features live in [examples/](examples/): a calculator (Operators Rule and
left-recursive), an indentation-based outline format and a small programming language. The complete parsers of
[alpha-parsers](https://github.com/zeroparse/alpha-parsers), such as the ones for **Go** and **Python 3**, are the larger examples.

## Documentation

| | |
|:--|:--|
| [Why ZeroParse](docs/why-zeroparse.md) | What ZeroParse aims to be, what it offers today, its limits, and how it compares with other parser tools |
| [Tutorial](docs/tutorial/README.md) | Ten lessons from a first grammar to a shipped, tested and debugged parser |
| [Grammar ASTs and JSON](docs/guide/grammar-asts.md) | Constructing and validating grammars, structured error paths and JSON compatibility |
| [Guides](docs/guide/README.md) | Trees and actions, expressions, errors and recovery, context-sensitive parsing, running parsers, using the ready-made parsers, compiled grammars, code generation (Go and TypeScript), streaming, incremental parsing, debugging and profiling, linting grammars, testing grammars, sampling inputs and fuzzing, editor support |
| [Cookbook](docs/cookbook/README.md) | Short, complete recipes for concrete tasks: a configuration file into Go structs, a filter language, errors with a caret, several errors at once, parsing every keystroke, shipping a generated parser, testing against a grammar |
| [Editor support](docs/guide/editor-support.md) | `zp lsp` and the VS Code extension: errors as you type, formatting, navigation, hover with inferred types, rename, completion |
| [Playground](docs/guide/playground.md) | Try grammars in the browser, inspect parse results and use the WebAssembly API |
| [Website maintenance](docs/site.md) | Build, test and publish the documentation site and playground |
| [Language specification](spec/README.md) | The ZeroParse grammar language |
| [Development guide](docs/development.md) | Repository layout, architecture, language support and tests |
| [Releasing](docs/releasing.md) | What a release promises, supported platforms and Go versions, and how a release is tagged |
| [Bytecode specification](spec/bytecode/README.md) | Instruction set and VM semantics, for porting the runtime |
| [Benchmarks](docs/benchmarks.md) · [Performance](docs/performance.md) · [Optimization catalog](docs/optimizations/README.md) | How fast, and how it got there |
| [Design](docs/design/README.md) · [Design records](docs/design/records/) | How it is built, and the decisions and their rationale |
| [Project strategy](docs/strategy.md) | Target users, competitive position, tracks and measurable targets |

## Contributing

Bug reports, ideas and pull requests are welcome. Run `go test ./... ./runtime/...` before sending changes, and follow the
[commit message guidelines](docs/commit-messages.md). Report security vulnerabilities privately, as the
[security policy](SECURITY.md) describes.

## Sponsoring

If ZeroParse saves you time, you can [buy the project a coffee through GitHub Sponsors](https://github.com/sponsors/zeroparse).

## Citing ZeroParse

If you use ZeroParse in research, cite it as below. GitHub's **Cite this repository** button gives the same entry from
[CITATION.cff](CITATION.cff) in other formats.

```bibtex
@software{zeroparse,
  author = {Furukawa, Arata},
  title  = {{ZeroParse}},
  url    = {https://github.com/zeroparse/zp},
  year   = {2026}
}
```

## Maintainer

ZeroParse is maintained by Arata Furukawa ([@ornew](https://github.com/ornew), old.river.new@gmail.com). Report security
vulnerabilities through the [security policy](SECURITY.md), not by email.

## License

[Apache License 2.0](LICENCE)
