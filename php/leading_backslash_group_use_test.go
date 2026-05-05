package php_test

import (
	"context"
	"strings"
	"testing"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
	"github.com/api-extraction-examples/go-tree-sitter/php"
)

// PHP accepts a leading backslash on a grouped-use prefix:
//
//	use \Foo\Bar\{ A, B };
//
// PHP 7.0+ has explicitly allowed it (group-use RFC), and it parses
// without warnings on PHP 8.x. The leading backslash is redundant
// because `use` always resolves names against the global namespace,
// but the form appears in real-world code and must round-trip cleanly
// rather than producing an ERROR subtree.
//
// Tree-sitter-php v0.24.2 admitted leading-backslash on a single-name
// `use` and admitted grouped-use without the leading backslash, but
// not the combination. The local fix adds an optional leading `\` to
// _namespace_use_group.

func TestLeadingBackslashGroupUse(t *testing.T) {
	cases := []struct{ name, src string }{
		{"basic group", `<?php use \Foo\Bar\{ A, B };`},
		{"single name in group", `<?php use \Foo\Bar\{ A };`},
		{"function group", `<?php use function \Foo\Bar\{ a, b };`},
		{"const group", `<?php use const \Foo\Bar\{ A, B };`},
		{"alias inside group", `<?php use \Foo\Bar\{ A as X, B };`},
		{"nested namespace", `<?php use \A\B\C\D\{ E, F };`},

		// Form from the production PHP file that motivated the fix.
		{"production-style",
			`<?php use \NLS\ScanOutput\{ StringConverter, FormatSpecifierConverter };`},

		// Adjacent declarations with and without leading backslash, to
		// guard against parser-state bleeding between statements.
		{"mixed adjacent", `<?php
use \Foo\Bar\{ A, B };
use Baz\Qux\{ C };
use \Single\Name;
use Another\Single;
`},
	}
	for _, c := range cases {
		mustParseClean(t, c.name, c.src)
	}
}

// Confirm the parsed tree exposes the prefix names so downstream
// visitors can reconstruct the imported FQNs. Specifically, the
// namespace_name children inside the namespace_use_declaration must
// contain "Foo" and "Bar".
func TestLeadingBackslashGroupUse_TreeShape(t *testing.T) {
	src := `<?php use \Foo\Bar\{ A, B };`
	n, err := sitter.ParseCtx(context.Background(), []byte(src), php.GetLanguage())
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if n.HasError() {
		t.Fatalf("unexpected parse error\ntree: %s", n.String())
	}
	tree := n.String()
	for _, want := range []string{"namespace_use_declaration", "namespace_name", "namespace_use_group", "namespace_use_clause"} {
		if !strings.Contains(tree, want) {
			t.Errorf("expected node %q in tree, got: %s", want, tree)
		}
	}
}
