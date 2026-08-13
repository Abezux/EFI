package main

import (
	"math"
	"testing"
)

func TestEvaluateSourceAudit_FoundingPost(t *testing.T) {
	vec := []float32{1.0, 0.0, 0.0}
	centSim, fndSim, status := EvaluateSourceAudit(42, 42, vec, vec, vec, 0.82, 0.65)

	if status != StatusFoundingPost {
		t.Fatalf("expected StatusFoundingPost for founding post ID match, got %v", status)
	}
	if centSim != 1.0 || fndSim != 1.0 {
		t.Fatalf("expected 1.0 similarities for founding post, got centSim=%f fndSim=%f", centSim, fndSim)
	}
}

func TestEvaluateSourceAudit_HighConfidencePass(t *testing.T) {
	// Source vector very close to both founding and centroid
	sourceVec := []float32{0.95, 0.31, 0.0}
	foundingVec := []float32{1.0, 0.0, 0.0}
	centroidVec := []float32{0.98, 0.19, 0.0}

	centSim, fndSim, status := EvaluateSourceAudit(101, 42, sourceVec, foundingVec, centroidVec, 0.82, 0.65)

	if status != StatusPass {
		t.Fatalf("expected StatusPass for high confidence match, got %v (centSim=%f, fndSim=%f)", status, centSim, fndSim)
	}
	if centSim < 0.82 || fndSim < 0.82 {
		t.Fatalf("expected similarities >= 0.82, got centSim=%f, fndSim=%f", centSim, fndSim)
	}
}

func TestEvaluateSourceAudit_DriftFlagged(t *testing.T) {
	// Source vector close to drifted centroid (centSim = ~0.98), but far from founding (fndSim = ~0.10)
	sourceVec := []float32{0.10, 0.995, 0.0}
	foundingVec := []float32{1.0, 0.0, 0.0}
	driftedCentroid := []float32{0.30, 0.954, 0.0}

	centSim, fndSim, status := EvaluateSourceAudit(102, 42, sourceVec, foundingVec, driftedCentroid, 0.82, 0.65)

	if status != StatusFlaggedDrift {
		t.Fatalf("DRIFT FLAG FAILED: expected StatusFlaggedDrift for drifted source, got %v (centSim=%f, fndSim=%f)", status, centSim, fndSim)
	}
	if centSim < 0.82 {
		t.Fatalf("setup expectation: centSim %f should be >= 0.82", centSim)
	}
	if fndSim >= 0.65 {
		t.Fatalf("setup expectation: fndSim %f should be < 0.65", fndSim)
	}
}

func TestEvaluateSourceAudit_ReviewBand(t *testing.T) {
	// Source vector with moderate similarity to both
	sourceVec := []float32{0.70, 0.714, 0.0}
	foundingVec := []float32{1.0, 0.0, 0.0}
	centroidVec := []float32{0.90, 0.435, 0.0}

	centSim, fndSim, status := EvaluateSourceAudit(103, 42, sourceVec, foundingVec, centroidVec, 0.82, 0.65)

	if status != StatusReview {
		t.Fatalf("expected StatusReview for review band match, got %v (centSim=%f, fndSim=%f)", status, centSim, fndSim)
	}
	if fndSim < 0.65 || fndSim >= 0.82 {
		t.Fatalf("expected fndSim between 0.65 and 0.82, got %f", fndSim)
	}
}

func TestParsePgVector(t *testing.T) {
	t.Run("valid vector string", func(t *testing.T) {
		str := "[0.123, -0.456, 0.789]"
		vec, err := ParsePgVector(str)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vec) != 3 {
			t.Fatalf("expected 3 elements, got %d", len(vec))
		}
		if math.Abs(float64(vec[0]-0.123)) > 1e-4 || math.Abs(float64(vec[1]+0.456)) > 1e-4 || math.Abs(float64(vec[2]-0.789)) > 1e-4 {
			t.Fatalf("unexpected values: %v", vec)
		}
	})

	t.Run("empty vector brackets", func(t *testing.T) {
		vec, err := ParsePgVector("[]")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vec) != 0 {
			t.Fatalf("expected empty slice, got %v", vec)
		}
	})

	t.Run("invalid vector formats", func(t *testing.T) {
		if _, err := ParsePgVector("invalid"); err == nil {
			t.Fatal("expected error for non-bracket string")
		}
		if _, err := ParsePgVector("[0.1, not_a_number]"); err == nil {
			t.Fatal("expected error for non-float element")
		}
	})
}

func TestCosineSimilarity(t *testing.T) {
	vecA := []float32{1.0, 0.0, 0.0}
	vecB := []float32{0.0, 1.0, 0.0}
	vecC := []float32{1.0, 0.0, 0.0}

	if sim := CosineSimilarity(vecA, vecB); math.Abs(sim) > 1e-6 {
		t.Fatalf("expected 0 similarity for orthogonal vectors, got %f", sim)
	}
	if sim := CosineSimilarity(vecA, vecC); math.Abs(sim-1.0) > 1e-6 {
		t.Fatalf("expected 1.0 similarity for identical vectors, got %f", sim)
	}
	if sim := CosineSimilarity([]float32{}, vecA); sim != 0.0 {
		t.Fatalf("expected 0.0 for empty vector, got %f", sim)
	}
	if sim := CosineSimilarity(vecA, []float32{1.0, 0.0}); sim != 0.0 {
		t.Fatalf("expected 0.0 for mismatched lengths, got %f", sim)
	}
}
