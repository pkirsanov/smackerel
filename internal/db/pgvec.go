package db

import "fmt"

// FormatEmbedding converts a float32 slice to pgvector string format.
func FormatEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	s := "["
	for i, v := range vec {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf("%f", v)
	}
	s += "]"
	return s
}
