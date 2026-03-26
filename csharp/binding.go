package csharp

//#cgo CFLAGS: -Wno-trigraphs
//#include "tree_sitter/parser.h"
//TSLanguage *tree_sitter_c_sharp();
import "C"
import (
	"unsafe"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
	// Blank import to ensure go mod vendor copies the tree_sitter directory
	// which contains header files required by the C code.
	_ "github.com/api-extraction-examples/go-tree-sitter/csharp/tree_sitter"
)

// GetLanguage returns a grammar for C# language.
//
// Note: The parser is incomplete, it may return a partial or wrong AST! You were warned.
func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_c_sharp())
	return sitter.NewLanguage(ptr)
}
