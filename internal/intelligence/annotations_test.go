package intelligence

import (
	"math"
	"testing"

	"github.com/smackerel/smackerel/internal/annotation"
)

func intPtr(v int) *int { return &v }

func TestAnnotationRelevanceDelta_Rating5(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeRating,
		Rating:         intPtr(5),
	}
	delta := annotationRelevanceDelta(ann)
	expected := 0.15
	if math.Abs(delta-expected) > 0.001 {
		t.Errorf("delta = %f, want %f", delta, expected)
	}
}

func TestAnnotationRelevanceDelta_Rating4(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeRating,
		Rating:         intPtr(4),
	}
	delta := annotationRelevanceDelta(ann)
	expected := 0.09
	if math.Abs(delta-expected) > 0.001 {
		t.Errorf("delta = %f, want %f", delta, expected)
	}
}

func TestAnnotationRelevanceDelta_Rating3(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeRating,
		Rating:         intPtr(3),
	}
	delta := annotationRelevanceDelta(ann)
	expected := 0.03
	if math.Abs(delta-expected) > 0.001 {
		t.Errorf("delta = %f, want %f", delta, expected)
	}
}

func TestAnnotationRelevanceDelta_Rating1(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeRating,
		Rating:         intPtr(1),
	}
	delta := annotationRelevanceDelta(ann)
	expected := -0.09
	if math.Abs(delta-expected) > 0.001 {
		t.Errorf("delta = %f, want %f", delta, expected)
	}
}

func TestAnnotationRelevanceDelta_Interaction(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType:  annotation.TypeInteraction,
		InteractionType: annotation.InteractionMadeIt,
	}
	delta := annotationRelevanceDelta(ann)
	if math.Abs(delta-0.10) > 0.001 {
		t.Errorf("delta = %f, want 0.10", delta)
	}
}

func TestAnnotationRelevanceDelta_TagAdd(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeTagAdd,
		Tag:            "weeknight",
	}
	delta := annotationRelevanceDelta(ann)
	if math.Abs(delta-0.02) > 0.001 {
		t.Errorf("delta = %f, want 0.02", delta)
	}
}

func TestAnnotationRelevanceDelta_Note(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeNote,
		Note:           "great dish",
	}
	delta := annotationRelevanceDelta(ann)
	if math.Abs(delta-0.03) > 0.001 {
		t.Errorf("delta = %f, want 0.03", delta)
	}
}

func TestAnnotationRelevanceDelta_NilRating(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeRating,
		Rating:         nil,
	}
	delta := annotationRelevanceDelta(ann)
	if delta != 0 {
		t.Errorf("delta = %f, want 0 for nil rating", delta)
	}
}

func TestAnnotationRelevanceDelta_TagRemove(t *testing.T) {
	ann := &annotation.Annotation{
		AnnotationType: annotation.TypeTagRemove,
	}
	delta := annotationRelevanceDelta(ann)
	if delta != 0 {
		t.Errorf("delta = %f, want 0 for tag_remove", delta)
	}
}

func TestClampFloat64_Overflow(t *testing.T) {
	// 0.98 + 0.15 = 1.13 → clamped to 1.0
	result := clampFloat64(0.98+0.15, 0, 1)
	if result != 1.0 {
		t.Errorf("clamp = %f, want 1.0", result)
	}
}

func TestClampFloat64_Underflow(t *testing.T) {
	// 0.02 - 0.09 = -0.07 → clamped to 0.0
	result := clampFloat64(0.02-0.09, 0, 1)
	if result != 0.0 {
		t.Errorf("clamp = %f, want 0.0", result)
	}
}

func TestClampFloat64_InRange(t *testing.T) {
	result := clampFloat64(0.5, 0, 1)
	if result != 0.5 {
		t.Errorf("clamp = %f, want 0.5", result)
	}
}

func TestAnnotationRelevanceDelta_AllRatings(t *testing.T) {
	// Verify the formula: (rating - 2.5) * 0.06
	tests := []struct {
		rating   int
		expected float64
	}{
		{1, -0.09},
		{2, -0.03},
		{3, 0.03},
		{4, 0.09},
		{5, 0.15},
	}
	for _, tt := range tests {
		ann := &annotation.Annotation{
			AnnotationType: annotation.TypeRating,
			Rating:         intPtr(tt.rating),
		}
		delta := annotationRelevanceDelta(ann)
		if math.Abs(delta-tt.expected) > 0.001 {
			t.Errorf("rating %d: delta = %f, want %f", tt.rating, delta, tt.expected)
		}
	}
}
