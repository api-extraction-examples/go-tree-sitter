package php_test

import (
	"context"
	"testing"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
	"github.com/api-extraction-examples/go-tree-sitter/php"
)

// Regression tests for NV-4134. PHP allows any reserved keyword as a
// named argument label (PHP manual: "Using reserved keywords as
// parameter names is allowed"). Tree-sitter-php v0.24.2 accepted
// several such keywords (array, fn, function, match, namespace, null,
// static, throw, true, false) but not these nine, which tree-sitter
// eagerly parsed in their expression role before seeing the `:`.

func mustParseClean(t *testing.T, name, src string) {
	t.Helper()
	n, err := sitter.ParseCtx(context.Background(), []byte(src), php.GetLanguage())
	if err != nil {
		t.Fatalf("%s: parse error: %v", name, err)
	}
	if n.HasError() {
		t.Errorf("%s: unexpected parse error\nsrc: %s\ntree: %s", name, src, n.String())
	}
}

func TestNamedArgumentKeywordLabels_NV4134(t *testing.T) {
	cases := []struct{ name, src string }{
		{"list:", `<?php foo(list: $x);`},
		{"clone:", `<?php foo(clone: $x);`},
		{"print:", `<?php foo(print: $x);`},
		{"new:", `<?php foo(new: $x);`},
		{"yield:", `<?php foo(yield: $x);`},
		{"include:", `<?php foo(include: $x);`},
		{"include_once:", `<?php foo(include_once: $x);`},
		{"require:", `<?php foo(require: $x);`},
		{"require_once:", `<?php foo(require_once: $x);`},

		// Case-insensitive.
		{"LIST:", `<?php foo(LIST: $x);`},
		{"Clone:", `<?php foo(Clone: $x);`},

		// Mixed with positional and other named args.
		{"mixed positional and keyword labels",
			`<?php foo($a, list: $b, clone: $c, new: $d);`},

		// In method calls and object creation, too.
		{"method call", `<?php $o->bar(require: $x);`},
		{"constructor call", `<?php new Foo(yield: $x);`},
	}
	for _, c := range cases {
		mustParseClean(t, c.name, c.src)
	}
}

// Keywords that were already accepted as named-argument labels before
// NV-4134 must still work.
func TestNamedArgumentKeywordLabels_PreExisting(t *testing.T) {
	cases := []struct{ name, src string }{
		{"array:", `<?php foo(array: $x);`},
		{"fn:", `<?php foo(fn: $x);`},
		{"function:", `<?php foo(function: $x);`},
		{"match:", `<?php foo(match: $x);`},
		{"namespace:", `<?php foo(namespace: $x);`},
		{"null:", `<?php foo(null: $x);`},
		{"static:", `<?php foo(static: $x);`},
		{"throw:", `<?php foo(throw: $x);`},
		{"true:", `<?php foo(true: $x);`},
		{"false:", `<?php foo(false: $x);`},

		// self and parent still work via reserved('nothing', name).
		{"self:", `<?php foo(self: $x);`},
		{"parent:", `<?php foo(parent: $x);`},

		// A regular identifier label.
		{"foo:", `<?php foo(bar: $x);`},
	}
	for _, c := range cases {
		mustParseClean(t, c.name, c.src)
	}
}

// The expression forms of these keywords must still parse. Adding the
// keywords to _argument_name must not shadow their normal expression
// roles.
func TestNamedArgumentKeywordExpressionRolesStillWork(t *testing.T) {
	cases := []struct{ name, src string }{
		{"list() destructuring", `<?php list($a, $b) = $arr;`},
		{"clone expression", `<?php $x = clone $obj;`},
		{"print expression", `<?php print "hi";`},
		{"new expression", `<?php $x = new Foo();`},
		{"yield expression", `<?php function f() { yield 1; }`},
		{"include expression", `<?php include 'file.php';`},
		{"include_once expression", `<?php include_once 'file.php';`},
		{"require expression", `<?php require 'file.php';`},
		{"require_once expression", `<?php require_once 'file.php';`},
	}
	for _, c := range cases {
		mustParseClean(t, c.name, c.src)
	}
}
