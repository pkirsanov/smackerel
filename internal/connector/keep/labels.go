package keep

import (
	"fmt"
	"strings"
)

// TopicMatch represents a resolved label-to-topic mapping.
type TopicMatch struct {
	LabelName string
	TopicID   string
	TopicName string
	MatchType string // "exact", "abbreviation", "fuzzy", "created"
}

// TopicMapper maps Keep labels to Smackerel topics.
type TopicMapper struct {
	abbreviations map[string]string
}

// NewTopicMapper creates a new TopicMapper.
func NewTopicMapper() *TopicMapper {
	return &TopicMapper{
		abbreviations: map[string]string{
			"ml":     "Machine Learning",
			"ai":     "Artificial Intelligence",
			"devops": "DevOps",
			"k8s":    "Kubernetes",
			"js":     "JavaScript",
			"ts":     "TypeScript",
			"py":     "Python",
			"db":     "Database",
			"ux":     "User Experience",
			"ui":     "User Interface",
			"api":    "Application Programming Interface",
			"ci":     "Continuous Integration",
			"cd":     "Continuous Deployment",
			"aws":    "Amazon Web Services",
			"gcp":    "Google Cloud Platform",
		},
	}
}

// MapLabels resolves a list of Keep labels to topic matches.
// Uses a 4-stage cascade: exact → abbreviation → fuzzy → create new.
// Without a DB connection, this performs local matching only (exact + abbreviation).
func (tm *TopicMapper) MapLabels(labels []string, existingTopics []string) []TopicMatch {
	var matches []TopicMatch

	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}

		match := tm.resolveLabel(label, existingTopics)
		matches = append(matches, match)
	}

	return matches
}

// resolveLabel resolves a single label through the cascade.
func (tm *TopicMapper) resolveLabel(label string, existingTopics []string) TopicMatch {
	// Stage 1: Exact match (case-insensitive)
	for _, topic := range existingTopics {
		if strings.EqualFold(label, topic) {
			return TopicMatch{
				LabelName: label,
				TopicID:   topicIDFromName(topic),
				TopicName: topic,
				MatchType: "exact",
			}
		}
	}

	// Stage 2: Abbreviation match
	labelLower := strings.ToLower(label)
	if expanded, ok := tm.abbreviations[labelLower]; ok {
		for _, topic := range existingTopics {
			if strings.EqualFold(expanded, topic) {
				return TopicMatch{
					LabelName: label,
					TopicID:   topicIDFromName(topic),
					TopicName: topic,
					MatchType: "abbreviation",
				}
			}
		}
	}
	// Reverse abbreviation: label is full name, topic stored as abbreviation
	for abbr, expanded := range tm.abbreviations {
		if strings.EqualFold(label, expanded) {
			for _, topic := range existingTopics {
				if strings.EqualFold(abbr, topic) {
					return TopicMatch{
						LabelName: label,
						TopicID:   topicIDFromName(topic),
						TopicName: topic,
						MatchType: "abbreviation",
					}
				}
			}
		}
	}

	// Stage 3: Fuzzy match (trigram similarity)
	bestMatch, bestSimilarity := tm.fuzzyMatch(label, existingTopics)
	if bestSimilarity >= 0.4 {
		return TopicMatch{
			LabelName: label,
			TopicID:   topicIDFromName(bestMatch),
			TopicName: bestMatch,
			MatchType: "fuzzy",
		}
	}

	// Stage 4: Create new topic
	return TopicMatch{
		LabelName: label,
		TopicID:   topicIDFromName(label),
		TopicName: label,
		MatchType: "created",
	}
}

// fuzzyMatch performs simple trigram similarity matching.
func (tm *TopicMapper) fuzzyMatch(label string, topics []string) (string, float64) {
	labelLower := strings.ToLower(label)
	labelTrigrams := trigrams(labelLower)

	var bestTopic string
	var bestSim float64

	for _, topic := range topics {
		topicLower := strings.ToLower(topic)
		topicTrigrams := trigrams(topicLower)
		sim := trigramSimilarity(labelTrigrams, topicTrigrams)
		if sim > bestSim {
			bestSim = sim
			bestTopic = topic
		}
	}

	return bestTopic, bestSim
}

// trigrams generates trigrams from a string.
func trigrams(s string) map[string]bool {
	padded := "  " + s + " "
	t := make(map[string]bool)
	for i := 0; i <= len(padded)-3; i++ {
		t[padded[i:i+3]] = true
	}
	return t
}

// trigramSimilarity calculates Jaccard similarity between two trigram sets.
func trigramSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// topicIDFromName generates a simple topic ID from a topic name.
func topicIDFromName(name string) string {
	return fmt.Sprintf("topic-%s", strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-")))
}

// DiffLabels compares current labels against previous labels to find added and removed.
func DiffLabels(current, previous []string) (added, removed []string) {
	currentSet := make(map[string]bool)
	for _, l := range current {
		currentSet[l] = true
	}
	previousSet := make(map[string]bool)
	for _, l := range previous {
		previousSet[l] = true
	}

	for _, l := range current {
		if !previousSet[l] {
			added = append(added, l)
		}
	}
	for _, l := range previous {
		if !currentSet[l] {
			removed = append(removed, l)
		}
	}
	return
}
