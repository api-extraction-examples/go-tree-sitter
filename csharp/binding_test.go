package csharp_test

import (
	"context"
	"os"
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
	expected := "(compilation_unit (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (identifier) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (using_directive (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier))) (file_scoped_namespace_declaration name: (qualified_name qualifier: (qualified_name qualifier: (qualified_name qualifier: (identifier) name: (identifier)) name: (identifier)) name: (identifier))) (class_declaration (attribute_list (attribute name: (identifier))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (preproc_if_in_attribute_list condition: (unary_expression argument: (identifier)) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (assignment_expression left: (identifier) right: (boolean_literal))))))) (modifier) name: (identifier) (base_list (identifier)) body: (declaration_list (field_declaration (modifier) (modifier) (variable_declaration type: (identifier) (variable_declarator name: (identifier)))) (constructor_declaration (modifier) name: (identifier) parameters: (parameter_list (parameter type: (identifier) name: (identifier))) body: (block (expression_statement (assignment_expression left: (identifier) right: (identifier))))) (comment) (comment) (comment) (method_declaration (attribute_list (attribute name: (identifier))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (modifier) returns: (identifier) name: (identifier) parameters: (parameter_list (parameter type: (identifier) name: (identifier) (default_expression))) body: (block (return_statement (invocation_expression function: (identifier) arguments: (argument_list (argument (identifier))))))) (comment) (comment) (comment) (method_declaration (attribute_list (attribute name: (identifier))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (attribute_list (attribute name: (identifier) (attribute_argument_list (attribute_argument (string_literal (string_literal_content)))))) (modifier) (modifier) returns: (identifier) name: (identifier) parameters: (parameter_list (parameter type: (identifier) name: (identifier) (default_expression))) body: (block (if_statement condition: (member_access_expression expression: (member_access_expression expression: (identifier) name: (identifier)) name: (identifier)) consequence: (block (expression_statement (await_expression (invocation_expression function: (member_access_expression expression: (identifier) name: (identifier)) arguments: (argument_list (argument (identifier)) (argument (identifier)) (argument (identifier))))))) alternative: (block (expression_statement (assignment_expression left: (member_access_expression expression: (member_access_expression expression: (identifier) name: (identifier)) name: (identifier)) right: (member_access_expression expression: (identifier) name: (identifier)))))))))))"
	assert.Equal(
		expected,
		n.String(),
	)
}
