package main

import (
	"net/http"
	"testing"
	"time"
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

func TestRiotClient_GetMatchIDsByPUUID_RankedType(t *testing.T) {
	var capturedType string
	client := newMockRiotClient(func(req *http.Request) (*http.Response, error) {
		capturedType = req.URL.Query().Get("type")
		return jsonResponse(http.StatusOK, []string{"NA1_123", "NA1_456"})
	})

	matchIDs, err := client.GetMatchIDsByPUUID(t.Context(), "test_puuid", time.Time{}, time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matchIDs) != 2 {
		t.Fatalf("expected 2 match IDs, got %d", len(matchIDs))
	}
	if capturedType != "ranked" {
		t.Errorf("expected query param type='ranked', got %q", capturedType)
	}
}
