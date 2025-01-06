package protobuf

//#include "parser.h"
//TSLanguage *tree_sitter_proto();
import "C"
import (
	"unsafe"

	sitter "github.com/api-extraction-examples/go-tree-sitter"
)

func GetLanguage() *sitter.Language {
	ptr := unsafe.Pointer(C.tree_sitter_proto())
	return sitter.NewLanguage(ptr)
}
