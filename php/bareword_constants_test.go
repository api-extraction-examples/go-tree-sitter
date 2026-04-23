package php_test

import (
	"context"
	"strings"
	"testing"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
	"github.com/api-extraction-examples/go-tree-sitter/php"
)

// Regression tests for a local grammar fix against tree-sitter-php
// v0.24.2. Upstream treated `self` and `parent` as reserved keyword
// tokens under $.relative_scope; PHP 8.5's scanner only tokenizes
// `static` as T_STATIC, so valid code like `$x = SELF;` was wrongly
// rejected.

func mustParse(t *testing.T, src string) *sitter.Node {
	t.Helper()
	n, err := sitter.ParseCtx(context.Background(), []byte(src), php.GetLanguage())
	if err != nil {
		t.Fatalf("ParseCtx failed: %v\nsrc:\n%s", err, src)
	}
	return n
}

func assertNoParseError(t *testing.T, name, src string) {
	t.Helper()
	n := mustParse(t, src)
	if n.HasError() {
		t.Errorf("%s: unexpected parse error\nsrc:\n%s\ntree: %s", name, src, n.String())
	}
}

func assertHasParseError(t *testing.T, name, src string) {
	t.Helper()
	n := mustParse(t, src)
	if !n.HasError() {
		t.Errorf("%s: expected parse error, got clean parse\nsrc:\n%s\ntree: %s", name, src, n.String())
	}
}

// Both original failure sites from the 2026-03-13 BeyondTrust trial
// run against sra-builder.
func TestBarewordConstantsOriginalFailureSites(t *testing.T) {
	// ajaxserver.php:640 - SELF used as bareword in an array literal.
	assertNoParseError(t, "Site 1: SELF in array literal", `<?php
$selfHosted = [SELF, SELF_TERM, SELF_EVAL, ASP_RESELLER];
`)

	// defines.php:26 - bareword constants as associative array keys.
	assertNoParseError(t, "Site 2: bareword constants as array keys", `<?php
$HOSTING_TYPES = [
    CLOUD_EVAL => ['name' => 'Bomgar Cloud - EVAL'],
    CLOUD_TERM => ['name' => 'Bomgar Cloud - TERM'],
    BASIC_TERM => ['name' => 'Basic TERM'],
    ASP_EVAL   => ['name' => 'ASP EVAL'],
    SELF_TERM  => ['name' => 'Self TERM'],
    SELF       => ['name' => 'Self'],
];
`)
}

// `self` and `parent` (in any case) must parse as regular bareword
// identifiers in expression position. Only `static`, which is a real
// T_STATIC keyword in PHP, remains reserved.
func TestBarewordSelfParentVariations(t *testing.T) {
	cases := []struct{ name, src string }{
		{"SELF alone", `<?php $x = SELF;`},
		{"self alone", `<?php $x = self;`},
		{"Self mixed-case", `<?php $x = Self;`},
		{"PARENT alone", `<?php $x = PARENT;`},
		{"parent alone", `<?php $x = parent;`},
		{"SELF in echo", `<?php echo SELF;`},
		{"SELF in isset index", `<?php if (isset($map[SELF])) {}`},
		{"self as positional arg", `<?php foo(self);`},
		{"parent as positional arg", `<?php foo(parent);`},
		{"self as array value", `<?php $a = [self, parent];`},
	}
	for _, c := range cases {
		assertNoParseError(t, c.name, c.src)
	}
}

// PHP's scanner tokenizes `static` (and other BASE reserved keywords
// like `list`, `array`, `if`) as keyword tokens; these genuinely cannot
// appear as bareword constants. Tree-sitter should continue to reject
// them, matching PHP 8.5 behavior.
func TestBarewordStaticAndOtherKeywordsStillRejected(t *testing.T) {
	cases := []struct{ name, src string }{
		{"$x = STATIC", `<?php $x = STATIC;`},
		{"$x = static", `<?php $x = static;`},
		{"$x = LIST", `<?php $x = LIST;`},
		{"$x = ARRAY", `<?php $x = ARRAY;`},
	}
	for _, c := range cases {
		assertHasParseError(t, c.name, c.src)
	}
}

// Scope-resolution contexts continue to parse. `static::...` still
// produces a (relative_scope) node; `self::...` and `parent::...` flow
// through $._name (a (name) node), matching PHP's actual grammar.
func TestScopeResolutionStillParses(t *testing.T) {
	assertNoParseError(t, "self::foo()", `<?php class X { function f() { self::bar(); } }`)
	assertNoParseError(t, "parent::foo()", `<?php class X extends Y { function f() { parent::bar(); } }`)
	assertNoParseError(t, "static::foo()", `<?php class X { function f() { static::bar(); } }`)
	assertNoParseError(t, "self::class", `<?php class X { const N = self::class; }`)
	assertNoParseError(t, "new self()", `<?php class X { function f() { return new self(); } }`)
	assertNoParseError(t, "new parent()", `<?php class X extends Y { function f() { return new parent(); } }`)
	assertNoParseError(t, "new static()", `<?php class X { function f() { return new static(); } }`)

	// `static::` still produces the (relative_scope) AST node.
	n := mustParse(t, `<?php static::bar();`)
	if !strings.Contains(n.String(), "(relative_scope)") {
		t.Errorf("expected (relative_scope) in tree for static::, got: %s", n.String())
	}
}

// PHP's soft-reserved type names (bool, int, float, string, object,
// iterable, mixed, never, void, callable) are valid bareword constants
// in non-class contexts under PHP 8.5.
func TestSoftReservedTypeNamesAsBarewordConstants(t *testing.T) {
	cases := []struct{ name, src string }{
		{"BOOL", `<?php $x = BOOL;`},
		{"INT", `<?php $x = INT;`},
		{"FLOAT", `<?php $x = FLOAT;`},
		{"STRING", `<?php $x = STRING;`},
		{"OBJECT", `<?php $x = OBJECT;`},
		{"ITERABLE", `<?php $x = ITERABLE;`},
		{"MIXED", `<?php $x = MIXED;`},
		{"NEVER", `<?php $x = NEVER;`},
		{"VOID", `<?php $x = VOID;`},
		{"CALLABLE", `<?php $x = CALLABLE;`},
	}
	for _, c := range cases {
		assertNoParseError(t, c.name, c.src)
	}
}

// Reserved keywords used as class-member names (constants, methods,
// properties) should continue to parse.
func TestReservedKeywordsAsClassMemberNames(t *testing.T) {
	assertNoParseError(t, "class const named after reserved words", `<?php
class X {
    const SELF = 1;
    const PARENT = 2;
    const LIST = 3;
    const STATIC = 4;
    const IF = 5;
}
`)
	assertNoParseError(t, "method names matching reserved words", `<?php
class X {
    public function list() {}
    public function array() {}
    public function match() {}
    public function fn() {}
}
`)
}
