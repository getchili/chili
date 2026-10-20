package codegen

import (
	"regexp"
	"strings"
)

var mainDecl = regexp.MustCompile(`(?m)^(\s*)fn main\(\)`)

// legacy convertit les programmes de test écrits avant les classes :
// « fn » devient « function », et la fonction main() devient la méthode
// main() d'une classe Main (via une fonction start()). Un programme qui
// définit déjà class Main est laissé tel quel.
func legacy(src string) string {
	if strings.Contains(src, "class Main") {
		return src
	}
	src = mainDecl.ReplaceAllString(src, "${1}fn start()")
	src = regexp.MustCompile(`\bfn `).ReplaceAllString(src, "function ")
	return src + "\nclass Main { function main() { start() } }"
}
