package annotation

import (
	"testing"
)

func TestParse_RatingOnly(t *testing.T) {
	p := Parse("4/5")
	if p.Rating == nil || *p.Rating != 4 {
		t.Fatalf("expected rating 4, got %v", p.Rating)
	}
	if p.InteractionType != "" {
		t.Fatalf("expected no interaction, got %s", p.InteractionType)
	}
}

func TestParse_RatingAndNote(t *testing.T) {
	p := Parse("3/5 needs more salt")
	if p.Rating == nil || *p.Rating != 3 {
		t.Fatalf("expected rating 3, got %v", p.Rating)
	}
	if p.Note != "needs more salt" {
		t.Fatalf("expected note 'needs more salt', got %q", p.Note)
	}
}

func TestParse_InteractionOnly(t *testing.T) {
	p := Parse("made it")
	if p.InteractionType != InteractionMadeIt {
		t.Fatalf("expected made_it, got %s", p.InteractionType)
	}
}

func TestParse_FullAnnotation(t *testing.T) {
	p := Parse("4/5 made it #weeknight needs more garlic")
	if p.Rating == nil || *p.Rating != 4 {
		t.Fatalf("expected rating 4, got %v", p.Rating)
	}
	if p.InteractionType != InteractionMadeIt {
		t.Fatalf("expected made_it, got %s", p.InteractionType)
	}
	if len(p.Tags) != 1 || p.Tags[0] != "weeknight" {
		t.Fatalf("expected tags [weeknight], got %v", p.Tags)
	}
	if p.Note != "needs more garlic" {
		t.Fatalf("expected note 'needs more garlic', got %q", p.Note)
	}
}

func TestParse_MultipleTags(t *testing.T) {
	p := Parse("#finance #long-term #revisit")
	if len(p.Tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(p.Tags), p.Tags)
	}
	expected := []string{"finance", "long-term", "revisit"}
	for i, tag := range expected {
		if p.Tags[i] != tag {
			t.Fatalf("tag %d: expected %s, got %s", i, tag, p.Tags[i])
		}
	}
}

func TestParse_TagRemoval(t *testing.T) {
	p := Parse("#remove-quick")
	if len(p.RemovedTags) != 1 || p.RemovedTags[0] != "quick" {
		t.Fatalf("expected removed tag 'quick', got %v", p.RemovedTags)
	}
	// Should NOT appear in regular tags
	if len(p.Tags) != 0 {
		t.Fatalf("expected no regular tags, got %v", p.Tags)
	}
}

func TestParse_BoughtIt(t *testing.T) {
	p := Parse("bought it 5/5 #electronics great deal")
	if p.InteractionType != InteractionBoughtIt {
		t.Fatalf("expected bought_it, got %s", p.InteractionType)
	}
	if p.Rating == nil || *p.Rating != 5 {
		t.Fatalf("expected rating 5, got %v", p.Rating)
	}
	if len(p.Tags) != 1 || p.Tags[0] != "electronics" {
		t.Fatalf("expected tag electronics, got %v", p.Tags)
	}
	if p.Note != "great deal" {
		t.Fatalf("expected note 'great deal', got %q", p.Note)
	}
}

func TestParse_ReadIt(t *testing.T) {
	p := Parse("read it")
	if p.InteractionType != InteractionReadIt {
		t.Fatalf("expected read_it, got %s", p.InteractionType)
	}
}

func TestParse_EmptyString(t *testing.T) {
	p := Parse("")
	if p.Rating != nil || p.InteractionType != "" || len(p.Tags) != 0 || p.Note != "" {
		t.Fatal("expected empty parsed annotation")
	}
}

func TestParse_NoteOnly(t *testing.T) {
	p := Parse("this was really interesting and useful")
	if p.Note != "this was really interesting and useful" {
		t.Fatalf("expected full note, got %q", p.Note)
	}
	if p.Rating != nil {
		t.Fatal("expected no rating")
	}
}

func TestParse_InvalidRating(t *testing.T) {
	// "7/5" shouldn't match the rating regex
	p := Parse("7/5 stars")
	if p.Rating != nil {
		t.Fatalf("expected no rating for 7/5, got %v", p.Rating)
	}
}

func TestParse_CaseSensitiveInteraction(t *testing.T) {
	p := Parse("Made It")
	if p.InteractionType != InteractionMadeIt {
		t.Fatalf("expected made_it for 'Made It', got %s", p.InteractionType)
	}
}

func TestParse_TagsCaseNormalized(t *testing.T) {
	p := Parse("#WeekNight #QUICK")
	if len(p.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(p.Tags))
	}
	if p.Tags[0] != "weeknight" || p.Tags[1] != "quick" {
		t.Fatalf("expected lowercase tags, got %v", p.Tags)
	}
}
