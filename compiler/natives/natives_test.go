package natives

import (
	"math"
	"strings"
	"testing"
)

func TestRandomRange(t *testing.T) {
	tests := []struct {
		min, max int32
		r        uint32
		want     int32
	}{
		{1, 6, 0, 1},              // tirage minimal → borne basse
		{1, 6, math.MaxUint32, 6}, // tirage maximal → borne haute
		{1, 6, 1 << 31, 4},        // milieu
		{7, 7, 12345, 7},          // intervalle d'une seule valeur
		{-10, -1, 0, -10},         // négatifs
		{math.MinInt32, math.MaxInt32, 0, math.MinInt32},              // toute la plage
		{math.MinInt32, math.MaxInt32, math.MaxUint32, math.MaxInt32}, // toute la plage
	}
	for _, tt := range tests {
		got, err := RandomRange(tt.min, tt.max, tt.r)
		if err != nil || got != tt.want {
			t.Errorf("RandomRange(%d, %d, %d) = %d, %v ; attendu %d", tt.min, tt.max, tt.r, got, err, tt.want)
		}
	}
	if _, err := RandomRange(6, 1, 0); err == nil {
		t.Error("max < min accepté")
	}
}

func TestTableSignatures(t *testing.T) {
	for _, n := range Table {
		if len(n.Sig) != n.Arity || strings.Trim(n.Sig, "isv") != "" {
			t.Errorf("%s : signature %q incohérente avec l'arité %d", n.Name, n.Sig, n.Arity)
		}
	}
}

func TestStrNatives(t *testing.T) {
	if got := StrSub("bonjour", 3, 99); got != "jour" {
		t.Errorf("StrSub = %q", got)
	}
	if got := StrSub("bonjour", -5, 3); got != "bon" {
		t.Errorf("StrSub = %q", got)
	}
	if got := StrSub("abc", 2, 1); got != "" {
		t.Errorf("StrSub = %q", got)
	}
	if StrFind("bonjour", "jour") != 3 || StrFind("abc", "z") != -1 || StrFind("abc", "") != 0 {
		t.Error("StrFind")
	}
	for s, want := range map[string]int32{"42": 42, " -17 ": -17, "+3": 3, "": 0, "-": 0, "12a": 0, "4294967295": -1} {
		if got := StrInt(s); got != want {
			t.Errorf("StrInt(%q) = %d, attendu %d", s, got, want)
		}
	}
}
