// Package diag définit les erreurs de compilation, partagées par le parser et
// l'analyse sémantique.
package diag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getchili/chili/compiler/token"
)

type Error struct {
	Pos token.Pos
	Msg string
}

func (e Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

// ErrorList regroupe plusieurs erreurs.
type ErrorList []Error

func (l ErrorList) Error() string {
	msgs := make([]string, len(l))
	for i, e := range l {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n")
}

// Sort trie les erreurs par position (stable).
func (l ErrorList) Sort() {
	sort.SliceStable(l, func(i, j int) bool {
		a, b := l[i].Pos, l[j].Pos
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
}
