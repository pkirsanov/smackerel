package db

import (
	"fmt"
	"strings"
)

// FormatEmbedding converts a float32 slice to pgvector string format.
func FormatEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(vec) * 12) // pre-allocate ~12 bytes per float
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%f", v)
	}
	b.WriteByte(']')
	return b.String()
}
