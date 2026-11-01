package runtime

// The protocol is the interface between generated code and the runtime: the exported declarations
// of this package, the exported fields and methods of the types they reach, and what those
// declarations mean, including the encoding of the data generated parsers pass to the runtime
// (grammar tables, node kinds and expectation IDs). A protocol version names one state of that
// interface, independent of the release version, and increases whenever the interface changes.
// testdata/protocol.golden records the declarations of the current version, and a test fails when
// the two disagree; a change of meaning or data encoding that keeps the declarations needs the
// same bump, which no test detects.
//
// The runtime declares a constant ProtocolN for each protocol version N it implements. A parser
// generated with zp gen -runtime import refers to the constant of the protocol it was generated
// for, so building it against a runtime that does not implement that protocol fails with an
// undefined name; regenerate the parser, or require a version of zeroparse.dev/zp/runtime that
// implements its protocol.

// Protocol1 is declared because the runtime implements protocol version 1.
const Protocol1 = 1
