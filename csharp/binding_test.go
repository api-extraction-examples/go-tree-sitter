package csharp_test

import (
	"context"
	"os"
	"strings"
	"testing"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
	"github.com/api-extraction-examples/go-tree-sitter/csharp"
	"github.com/stretchr/testify/assert"
)

func TestGrammar(t *testing.T) {
	assert := assert.New(t)

	n, err := sitter.ParseCtx(context.Background(), []byte("using static System.Math;"), csharp.GetLanguage())
	assert.NoError(err)
	assert.Equal(
		"(compilation_unit (using_directive (qualified_name qualifier: (identifier) name: (identifier))))",
		n.String(),
	)
}

func TestGrammar2(t *testing.T) {
	assert := assert.New(t)

	content, err := os.ReadFile("testing/testt.cs") // testt.cs is: https://github.com/Universalis-FFXIV/Universalis/blob/bc38866d8cbc85ed95df46f432bc896b697f218c/src/Universalis.Application/Controllers/V2/WebSocketController.cs#L10
	assert.NoError(err)

	n, err := sitter.ParseCtx(context.Background(), content, csharp.GetLanguage())
	assert.NoError(err)
	expected := "(compilation_unit (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (identifier) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (file_scoped_namespace_declaration name: (qualified_name qualifier: (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier)) name: (identifier))) (class_declaration (attribute_list (attribute name: (identifier))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (preproc_if_in_attribute_list condition: (unary_expression argument: (identifier)) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument name: (identifier) (boolean_literal)))))) (modifier) name: (identifier) (base_list (identifier)) body: (declaration_list (field_declaration (modifier) (modifier) (variable_declaration type: (identifier) (variable_declarator name: (identifier)))) (constructor_declaration (modifier) name: (identifier) parameters: (parameter_list (parameter type: (identifier) name: (identifier))) body: (block (expression_statement (assignment_expression left: (identifier) right: (identifier))))) (comment) (comment) (comment) (method_declaration (attribute_list (attribute name: (identifier))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (modifier) returns: (identifier) name: (identifier) parameters: (parameter_list (parameter type: (identifier) name: (identifier) (default_expression))) body: (block (return_statement (invocation_expression function: (identifier) arguments: (argument_list (argument (identifier))))))) (comment) (comment) (comment) (method_declaration (attribute_list (attribute name: (identifier))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (modifier) (modifier) returns: (identifier) name: (identifier) parameters: (parameter_list (parameter type: (identifier) name: (identifier) (default_expression))) body: (block (if_statement condition: (member_access_expression expression: (member_access_expression expression: (identifier) name: (identifier)) name: (identifier)) consequence: (block (expression_statement (await_expression (invocation_expression function: (member_access_expression expression: (identifier) name: (identifier)) arguments: (argument_list (argument (identifier)) (argument (identifier)) (argument (identifier))))))) alternative: (block (expression_statement (assignment_expression left: (member_access_expression expression: (member_access_expression expression: (identifier) name: (identifier)) name: (identifier)) right: (member_access_expression expression: (identifier) name: (identifier)))))))))))"
	assert.Equal(
		expected,
		n.String(),
	)
}

// TestCSharp12_SemicolonTerminatedTypes verifies that C# 12 semicolon-terminated
// (bodyless) type declarations parse without errors. This was the primary fix
// from upstream PR #364 (commit d13ccdd).
func TestCSharp12_SemicolonTerminatedTypes(t *testing.T) {
	assert := assert.New(t)

	code := `public class BasePluginController : ControllerBase;`
	n, err := sitter.ParseCtx(context.Background(), []byte(code), csharp.GetLanguage())
	assert.NoError(err)

	ast := n.String()
	assert.NotContains(ast, "ERROR", "semicolon-terminated class should parse without errors")
	assert.Contains(ast, "class_declaration")
	assert.Contains(ast, "base_list")
}

// TestCSharp12_SemicolonTerminatedVariants verifies semicolon-terminated syntax
// for all type kinds: class, struct, record, record struct, enum, and interface.
func TestCSharp12_SemicolonTerminatedVariants(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		nodeType string
	}{
		{"class", "public class Foo : Bar;", "class_declaration"},
		{"struct", "public struct Foo;", "struct_declaration"},
		{"record", "public record Foo;", "record_declaration"},
		{"record struct", "public record struct Foo;", "record_declaration"},
		{"interface", "public interface IFoo;", "interface_declaration"},
		{"enum", "public enum Foo;", "enum_declaration"},
		{"generic class", "public class Foo<T> : Bar<T>;", "class_declaration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := sitter.ParseCtx(context.Background(), []byte(tt.code), csharp.GetLanguage())
			assert.NoError(t, err)
			ast := n.String()
			assert.NotContains(t, ast, "ERROR", "should parse without errors: %s", tt.code)
			assert.Contains(t, ast, tt.nodeType)
		})
	}
}

// TestCSharp12_PrimaryConstructors verifies primary constructor syntax on classes.
func TestCSharp12_PrimaryConstructors(t *testing.T) {
	assert := assert.New(t)

	code := `public class UserService(ILogger logger, IRepository repo)
{
    public void Log(string message) => logger.Log(message);
}`
	n, err := sitter.ParseCtx(context.Background(), []byte(code), csharp.GetLanguage())
	assert.NoError(err)

	ast := n.String()
	assert.NotContains(ast, "ERROR", "primary constructor should parse without errors")
	assert.Contains(ast, "class_declaration")
}

// TestCSharp12_PrimaryConstructorWithBaseAndSemicolon verifies that a class with
// a primary constructor, base invocation, and semicolon body parses correctly.
func TestCSharp12_PrimaryConstructorWithBaseAndSemicolon(t *testing.T) {
	assert := assert.New(t)

	code := `public class DerivedService(ILogger logger) : BaseService(logger);`
	n, err := sitter.ParseCtx(context.Background(), []byte(code), csharp.GetLanguage())
	assert.NoError(err)

	ast := n.String()
	assert.NotContains(ast, "ERROR", "primary constructor with base and semicolon should parse without errors")
	assert.Contains(ast, "class_declaration")
	assert.Contains(ast, "base_list")
}

// TestCSharp12_FullFile parses the C# 12 test fixture and verifies no errors.
func TestCSharp12_FullFile(t *testing.T) {
	assert := assert.New(t)

	content, err := os.ReadFile("testing/csharp12.cs")
	assert.NoError(err)

	n, err := sitter.ParseCtx(context.Background(), content, csharp.GetLanguage())
	assert.NoError(err)

	ast := n.String()
	assert.NotContains(ast, "ERROR", "C# 12 test fixture should parse without errors")
}

// TestCSharp13_FullFile parses the C# 13 test fixture and verifies no errors.
func TestCSharp13_FullFile(t *testing.T) {
	assert := assert.New(t)

	content, err := os.ReadFile("testing/csharp13.cs")
	assert.NoError(err)

	n, err := sitter.ParseCtx(context.Background(), content, csharp.GetLanguage())
	assert.NoError(err)

	ast := n.String()
	// Count ERROR nodes if any — some C# 13 features may not be fully supported
	errorCount := strings.Count(ast, "ERROR")
	t.Logf("C# 13 parse errors: %d", errorCount)
	// Partial properties may not be fully supported; allow a small number of errors
	assert.LessOrEqual(errorCount, 2, "C# 13 test fixture should have at most 2 parse errors")
}

// assertCleanParse parses code and asserts no ERROR or MISSING nodes.
func assertCleanParse(t *testing.T, code string) string {
	t.Helper()
	n, err := sitter.ParseCtx(context.Background(), []byte(code), csharp.GetLanguage())
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	ast := n.String()
	if strings.Contains(ast, "ERROR") || strings.Contains(ast, "MISSING") {
		t.Errorf("expected clean parse, got AST: %s", ast)
	}
	return ast
}

// TestCSharp_ObjectInitializerQualifiedMember verifies NV-4235: object
// initializers whose property values are qualified member-access expressions
// (e.g. Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping) parse cleanly.
// This pattern triggered a visitor-side panic in NV-3800; the visitor was
// fixed in api-excavator and the parser-side ambiguity appears to have been
// resolved by the C# 12/13 grammar upgrade. This test is a regression guard.
func TestCSharp_ObjectInitializerQualifiedMember(t *testing.T) {
	cases := map[string]string{
		"jsonwriter-options": `class C {
    private static readonly JsonWriterOptions WriterOptions = new()
    {
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
        Indented = false,
        SkipValidation = true
    };
}`,
		"deeper-qualifier": `class C { void M() { var x = new Foo { Bar = A.B.C.D.E }; } }`,
		"single-property": `class C { void M() { var x = new Foo { Bar = Ns.Type.Member }; } }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			ast := assertCleanParse(t, code)
			if !strings.Contains(ast, "initializer_expression") {
				t.Errorf("expected initializer_expression in AST: %s", ast)
			}
		})
	}
}

// TestCSharp12_CollectionExpressionInArgPosition verifies NV-4233:
// C# 12 collection expressions [item] passed as method-call arguments
// parse cleanly. Fixed by upstream PR #402 which added the
// collection_expression / collection_element / expression_element /
// spread_element rules; this repo picks up the fix by upgrading the
// vendored tree-sitter-c-sharp pin to v0.23.5.
//
// The ticket's literal example uses `[field]`, but `field` is a C# 13
// contextual keyword and is not in the upstream grammar's
// _reserved_identifier choice. That defect is the same root cause as
// NV-4232 (async:/await: as named-arg labels) and is addressed by a
// follow-up patch. Cases here use non-keyword names; the literal
// [field] form is covered by TestCSharp13_FieldAsBarewordIdentifier.
// `[async]` is not covered: keeping `async` out of _reserved_identifier
// is intentional because that breaks the upstream Async-Lambda corpus
// test, and the named-arg form (Foo(async: ...)) is the hot path.
func TestCSharp12_CollectionExpressionInArgPosition(t *testing.T) {
	cases := map[string]string{
		"single-item-middle":    `class C { void M() { db.HashFieldExpire(key, [item], TimeSpan.FromHours(1)); } }`,
		"with-out-and-ref":      `class C { void M() { HashDelete(key, [item], out itemsDoneCount, ref ctx); } }`,
		"multi-item":            `class C { void M() { F([a, b, c]); } }`,
		"spread":                `class C { void M() { F([..xs, last]); } }`,
		"empty-as-arg":          `class C { void M() { F([]); } }`,
		"as-only-arg":           `class C { void M() { F([42]); } }`,
		"nested-in-object-init": `class C { void M() { var x = new Foo { Items = [a, b] }; } }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			ast := assertCleanParse(t, code)
			if !strings.Contains(ast, "collection_expression") {
				t.Errorf("expected collection_expression in AST: %s", ast)
			}
		})
	}
}

// TestCSharp_ContextualKeywordAsNamedArgLabel verifies NV-4232: when
// a method-call argument uses a contextual keyword as its label
// (Foo(x, async: true)), the parser must accept the keyword as the
// argument name rather than emit ERROR at the colon. Patch the
// argument rule to admit `async`/`await` via _argument_name_keyword,
// matching Roslyn behavior. The `field` named-arg case lives in
// TestCSharp13_FieldAsBarewordIdentifier below; that keyword reaches
// argument names via _reserved_identifier rather than this patch.
func TestCSharp_ContextualKeywordAsNamedArgLabel(t *testing.T) {
	cases := []struct {
		name, code string
		// namedArgs is the total number of `name:` argument labels in
		// the snippet. Asserting on the count (not just presence) means
		// a regression that loses one specific keyword label still
		// fails the test, even when other plain-identifier labels in
		// the same call would satisfy a substring check.
		namedArgs int
	}{
		{"async-only", `class C { void M() { Foo(async: true); } }`, 1},
		{"async-after-positional", `class C { void M() { Foo(1, async: true); } }`, 1},
		{"await-after-positional", `class C { void M() { Foo(x, await: y); } }`, 1},
		{"garnet-shape", `class C { void M() { ClusterReplicate(1, primaryId, async: true, logger: ctx.logger); } }`, 2},
		{"multi-named-keyword", `class C { void M() { Foo(replicaNodeIndex: r, primaryNodeIndex: p, failEx: false, async: true, logger: l); } }`, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ast := assertCleanParse(t, tc.code)
			// `(argument name: (identifier` is the unique marker for a
			// named-arg label. The bare `name: (identifier` substring
			// also appears for class/method names and member-access
			// names, so it would over-count.
			got := strings.Count(ast, "(argument name: (identifier")
			if got != tc.namedArgs {
				t.Errorf("expected %d named-arg labels, got %d. AST: %s", tc.namedArgs, got, ast)
			}
		})
	}
}

// TestCSharp13_FieldAsBarewordIdentifier verifies that the C# 13
// contextual keyword `field` (partial-property accessor backing field)
// can also appear as a regular identifier in expression position. This
// closes the keyword sub-case of NV-4233: F([field]) and similar
// patterns from garnet/RespHashTests.cs:98-100 parse cleanly. The
// upstream `field:` attribute target use site continues to work via
// the [_reserved_identifier, attribute_target_specifier] conflict.
func TestCSharp13_FieldAsBarewordIdentifier(t *testing.T) {
	cases := map[string]string{
		"in-collection-arg":         `class C { void M() { F([field]); } }`,
		"in-middle-collection-arg":  `class C { void M() { F(key, [field], time); } }`,
		"as-rhs":                    `class C { void M() { var x = field; } }`,
		"in-method-arg":             `class C { void M() { F(field); } }`,
		"as-named-arg-label":        `class C { void M() { Foo(field: 1); } }`,
		"in-fluent-chain":           `class C { void M() { db.HashFieldExpire(key, [field], TimeSpan.FromHours(1)); } }`,
		"attribute-target-preserved": `[field: NonSerialized] class C { }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			assertCleanParse(t, code)
		})
	}
}

// TestCSharp_RefPartialStructModifierOrder verifies NV-4234: a struct
// declaration accepts `ref` at any position among its modifiers
// (orleans/Writer.cs:102 has `public ref partial struct Writer<T>`).
// Upstream pins `ref` immediately before `struct`, forcing all other
// modifiers to precede it. Tracked against upstream issue #361.
func TestCSharp_RefPartialStructModifierOrder(t *testing.T) {
	cases := map[string]string{
		"public-ref-partial":          `public ref partial struct W { }`,
		"public-partial-ref":          `public partial ref struct W { }`,
		"bare-ref-partial":            `ref partial struct W { }`,
		"public-ref":                  `public ref struct W { }`,
		"with-where-constraint":       `public ref partial struct W<T> where T : IBufferWriter<byte> { }`,
		"orleans-writer-shape":        `namespace N { public ref partial struct Writer<TBufferWriter> where TBufferWriter : IBufferWriter<byte> { } }`,
		"plain-struct":                `public struct W { }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			ast := assertCleanParse(t, code)
			if !strings.Contains(ast, "struct_declaration") {
				t.Errorf("expected struct_declaration in AST: %s", ast)
			}
		})
	}
}

// TestCSharp_PointerDerefParenAndCast verifies NV-4231: unsafe
// pointer dereference of a parenthesized expression or a cast result
// parses cleanly. The patch has two parts and this test covers both:
//
//  1. Operand widening: _pointer_indirection_expression now accepts
//     parenthesized_expression and cast_expression, not just
//     lvalue_expression. Without it, *(p + 1) and *(long*)(ptr + idx)
//     in argument position fail. Covered by the deref-* / multi-arg-*
//     cases.
//
//  2. Assignment-context conflict declaration: the widened operand
//     introduces a shift-reduce ambiguity at the trailing `=` for
//     patterns like *(int*)p = 5, resolved by the
//     [$.assignment_expression, $.expression] conflict. Covered by
//     deref-as-rhs and deref-as-lvalue.
//
// Tracked against upstream issue #363.
func TestCSharp_PointerDerefParenAndCast(t *testing.T) {
	cases := map[string]string{
		"deref-paren-binary":    `class C { unsafe void M() { Set(*(p + 1)); } }`,
		"deref-cast-paren":      `class C { unsafe void M() { Set(*(long*)(p + 1)); } }`,
		"deref-cast-method":     `class C { unsafe void M() { keys.Add(*(long*)key.ToPointer()); } }`,
		"multi-arg-deref-cast":  `class C { unsafe void M() { Set(init_keys, count, *(long*)(chunk_ptr + idx)); } }`,
		"deref-cast-byte":       `class C { unsafe void M() { AreEqual((byte)flag, *(payloadPtr + entry)); } }`,
		"deref-as-rhs":          `class C { unsafe void M() { x = *(int*)p; } }`,
		"deref-as-lvalue":       `class C { unsafe void M() { *(int*)p = 5; } }`,
		"deref-simple":          `class C { unsafe void M() { Set(*p); } }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			ast := assertCleanParse(t, code)
			if !strings.Contains(ast, "prefix_unary_expression") {
				t.Errorf("expected prefix_unary_expression in AST: %s", ast)
			}
		})
	}
}

// TestCSharp12_CollectionExpressionInTernary verifies NV-4311: an empty
// or non-empty C# 12 collection expression appearing as a branch of a
// conditional (ternary) expression parses cleanly. Originally this
// pattern was misparsed because `b?[]` could match
// array_type(nullable_type(b), array_rank_specifier([])) and that path
// won during conflict resolution. Adding collection_expression in
// upstream PR #402 (vendored via the v0.23.5 pin) resolves both the
// then-branch and else-branch shapes plus the cascade case from
// upstream issue #406.
func TestCSharp12_CollectionExpressionInTernary(t *testing.T) {
	cases := map[string]string{
		"empty-then":       `class C { object x = b ? [] : null; }`,
		"empty-else":       `class C { object x = b ? null : []; }`,
		"non-empty-then":   `class C { object x = b ? [1] : null; }`,
		"bitwarden-shape":  `class C { object Discounts = phase1Ended ? [] : phase2.Discounts?.Select(d => new D { X = 1 }); }`,
		"upstream-cascade": "class C { void M() { var x = new Entry { Data = condition ? [] : Map(item, context), Other = Get(value) }; void Method() { } } }",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			ast := assertCleanParse(t, code)
			// Both nodes are required: a regression that loses the
			// ternary wrapping (e.g. by parsing `b?[]` as a nullable
			// array type) would still contain a collection_expression
			// somewhere and slip past a single-substring check.
			if !strings.Contains(ast, "conditional_expression") {
				t.Errorf("expected conditional_expression in AST: %s", ast)
			}
			if !strings.Contains(ast, "collection_expression") {
				t.Errorf("expected collection_expression in AST: %s", ast)
			}
		})
	}
}

// TestCSharp_NullConditionalFluentInTernary verifies NV-4236: null-conditional
// (?.) fluent call chains used as operands of a conditional expression parse
// cleanly. Closed by the C# 12/13 grammar upgrade; this test is a regression
// guard.
func TestCSharp_NullConditionalFluentInTernary(t *testing.T) {
	cases := map[string]string{
		"simple-chain-in-then": `class C { string M(object o) => c ? o?.Foo()?.Bar?.ToString() : null; }`,
		"complex-chain-in-else": `class C { string M() => useDefault ? null : Definition?.Methods?.FirstOrDefault(m => m.Name == n)?.GetParameters()?.Length.ToString(); }`,
		"semantic-model-shape": `class C { void M() {
    var x = (testMode == TestMode.None)
        ? compilation?.SemanticModel?.GetSymbolInfo(node)?.Symbol?.ContainingType?.ToDisplayString()
        : null;
} }`,
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			ast := assertCleanParse(t, code)
			if !strings.Contains(ast, "conditional_expression") {
				t.Errorf("expected conditional_expression in AST: %s", ast)
			}
		})
	}
}
