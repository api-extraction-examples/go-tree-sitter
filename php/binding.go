package php

//#include "tree_sitter/parser.h"
//TSLanguage *tree_sitter_php();
import "C"
import (
	"unsafe"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
	// Blank import to ensure go mod vendor copies the tree_sitter directory
	// which contains header files required by the C code.
	_ "github.com/api-extraction-examples/go-tree-sitter/php/tree_sitter"
)

func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_php())
	return sitter.NewLanguage(ptr)
}
