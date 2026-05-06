//go:build stress

package readiness

import (
	"context"
	"testing"
	"time"
)

func TestStressReadinessCanary_Live(test *testing.T) {
	config, err := ConfigFromEnv()
	if err != nil {
		test.Fatalf("stress readiness env: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := Check(ctx, config); err != nil {
		test.Fatalf("stress readiness canary failed: %v", err)
	}
}
