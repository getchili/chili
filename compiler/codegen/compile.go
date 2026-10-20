package codegen

import (
	"github.com/getchili/chili/compiler/bytecode"
	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/parser"
	"github.com/getchili/chili/compiler/sema"
)

// Compile enchaîne parsing, analyse sémantique, génération et vérification.
// En cas d'erreur de source, err est une diag.ErrorList.
func Compile(src []byte) (*bytecode.Program, error) {
	prog, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	info, err := sema.Check(prog)
	if err != nil {
		return nil, err
	}
	bc, err := Generate(info)
	if err != nil {
		return nil, err
	}
	// Garde-fou : le compilateur ne doit jamais produire de bytecode invalide.
	if err := bytecode.Verify(bc, natives.Arity); err != nil {
		panic("codegen: bytecode invalide : " + err.Error())
	}
	return bc, nil
}
