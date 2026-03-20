package storage

import (
	"math"
	"testing"
	"time"
)

func TestGetStatsCalculatesRecentRates(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer store.Close()

	records := []*RequestRecord{
		{
			Timestamp:    time.Now().Add(-2 * time.Minute),
			Provider:     "zhipu",
			Model:        "glm-4-flash",
			Method:       "POST",
			Path:         "/v1/chat/completions",
			StatusCode:   200,
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
			Success:      true,
		},
		{
			Timestamp:    time.Now().Add(-30 * time.Second),
			Provider:     "zhipu",
			Model:        "glm-4-flash",
			Method:       "POST",
			Path:         "/v1/chat/completions",
			StatusCode:   200,
			InputTokens:  40,
			OutputTokens: 10,
			TotalTokens:  50,
			Success:      true,
		},
		{
			Timestamp:    time.Now().Add(-10 * time.Minute),
			Provider:     "zhipu",
			Model:        "glm-4-flash",
			Method:       "POST",
			Path:         "/v1/chat/completions",
			StatusCode:   200,
			InputTokens:  20,
			OutputTokens: 5,
			TotalTokens:  25,
			Success:      true,
		},
	}

	for _, record := range records {
		if err := store.SaveRequest(record); err != nil {
			t.Fatalf("save request: %v", err)
		}
	}

	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if stats.TotalRequests != 3 {
		t.Fatalf("expected 3 total requests, got %d", stats.TotalRequests)
	}

	if math.Abs(stats.RequestsPerMin-0.4) > 0.001 {
		t.Fatalf("expected requests_per_min to be 0.4, got %f", stats.RequestsPerMin)
	}

	if math.Abs(stats.InputPerMin-28.0) > 0.001 {
		t.Fatalf("expected input_per_min to be 28, got %f", stats.InputPerMin)
	}

	if math.Abs(stats.OutputPerMin-12.0) > 0.001 {
		t.Fatalf("expected output_per_min to be 12, got %f", stats.OutputPerMin)
	}
}
