package natives

import (
	"math"
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
