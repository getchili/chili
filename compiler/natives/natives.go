// Package natives est la table des fonctions natives fournies par la VM.
//
// L'ID de chaque native est son index dans Table : il est encodé dans le
// bytecode (CALL_NATIVE) et doit correspondre à la table côté VM C.
// N'ajouter qu'en fin de table pour ne pas casser les programmes compilés.
package natives

import (
	"fmt"
	"strconv"
	"strings"
)

type Native struct {
	Name  string // "delay" ou "gpio.write"
	Arity int
}

var Table = []Native{
	{"print", 1},
	{"delay", 1},
	{"millis", 0},
	{"gpio.mode", 2},
	{"gpio.write", 2},
	{"gpio.read", 1},
	{"random", 2},
}

// Lookup renvoie l'ID et la description d'une native par son nom complet.
func Lookup(name string) (id int, n Native, ok bool) {
	for i, n := range Table {
		if n.Name == name {
			return i, n, true
		}
	}
	return 0, Native{}, false
}

// IsModule indique si au moins une native est préfixée par module + ".".
func IsModule(module string) bool {
	for _, n := range Table {
		if strings.HasPrefix(n.Name, module+".") {
			return true
		}
	}
	return false
}

// Arity renvoie l'arité de la native d'ID id (format attendu par bytecode.Verify).
func Arity(id int) (int, bool) {
	if id < 0 || id >= len(Table) {
		return 0, false
	}
	return Table[id].Arity, true
}

// Name renvoie le nom de la native d'ID id, ou "#id" si elle est inconnue.
func Name(id int) string {
	if id < 0 || id >= len(Table) {
		return "#" + strconv.Itoa(id)
	}
	return Table[id].Name
}

// RandomRange ramène une valeur aléatoire 32 bits r dans [min, max] (bornes
// incluses), sans biais notable. La VM C applique exactement la même formule.
func RandomRange(min, max int32, r uint32) (int32, error) {
	if max < min {
		return 0, fmt.Errorf("random : le maximum (%d) est plus petit que le minimum (%d)", max, min)
	}
	span := uint64(int64(max)-int64(min)) + 1 // de 1 à 2^32
	return int32(int64(min) + int64((uint64(r)*span)>>32)), nil
}
