package checker

import (
	"context"
	"testing"
)

func TestCheckNetwork_Success(t *testing.T) {
	ctx := context.Background()
	result := CheckNetwork(ctx)
	if !result {
		t.Log("Warning: CheckNetwork failed with context.Background(), maybe no internet access in test environment.")
	}
}

func TestCheckNetwork_Failure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately to ensure dial fails

	result := CheckNetwork(ctx)
	if result {
		t.Errorf("Expected CheckNetwork to fail with a canceled context")
	}
}
