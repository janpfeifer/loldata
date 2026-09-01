package main

import (
	"testing"
)

func TestNormalizePlatform(t *testing.T) {
	tests := []struct {
		input        string
		wantPlatform string
		wantRegional string
	}{
		{"EUW", "euw1", "europe"},
		{"euw", "euw1", "europe"},
		{"euw1", "euw1", "europe"},
		{"europe", "euw1", "europe"},
		{"NA", "na1", "americas"},
		{"na1", "na1", "americas"},
		{"americas", "na1", "americas"},
		{"KR", "kr", "asia"},
		{"asia", "kr", "asia"},
		{"EUNE", "eun1", "europe"},
		{"oce", "oc1", "sea"},
		{"sea", "sg2", "sea"},
	}

	for _, tt := range tests {
		gotPlat, gotReg := NormalizePlatform(tt.input)
		if gotPlat != tt.wantPlatform || gotReg != tt.wantRegional {
			t.Errorf("NormalizePlatform(%q) = (%q, %q), want (%q, %q)",
				tt.input, gotPlat, gotReg, tt.wantPlatform, tt.wantRegional)
		}
	}
}
