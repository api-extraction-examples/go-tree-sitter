package html

//#include "parser.h"
//TSLanguage *tree_sitter_html();
import "C"
import (
	"unsafe"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
)

func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_html())
	return sitter.NewLanguage(ptr)
}
