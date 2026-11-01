package compile

import "zeroparse.dev/zp/grammar"

// LookKeepsState reports whether a positive lookahead of e can leave a capture or a variable
// binding in effect after it: whether e writes a capture or binds a variable outside the rules it
// calls (the captures and bindings of a called rule end with the call). Such a lookahead is not
// summarized by its outcome in a left-recursion context (see "Evaluations of Rules" in
// spec/runtime/parsing.md): every backend gives it no lookahead site, so the growing results it
// reads stay dependencies of the result around it.
func LookKeepsState(e grammar.Expr) bool {
	found := false
	WalkExpr(e, func(x grammar.Expr) {
		switch x := x.(type) {
		case *grammar.Capture:
			found = true
		case *grammar.Predicate:
			WalkTerm(x.Term, func(t grammar.Term) {
				if _, ok := t.(*grammar.Bind); ok {
					found = true
				}
			})
		}
	})
	return found
}
