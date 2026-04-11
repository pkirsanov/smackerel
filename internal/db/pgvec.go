package db

import (
	"strconv"
	"strings"
)

// FormatEmbedding converts a float32 slice to pgvector string format.
// Uses strconv.AppendFloat for faster formatting and shorter output
// (no trailing zeros) compared to fmt.Fprintf with %f.
func FormatEmbedding(vec []float32) string {
	if len(vec) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(vec) * 12) // pre-allocate ~12 bytes per float
	b.WriteByte('[')
	// Reusable scratch buffer for strconv.AppendFloat to avoid allocations.
	buf := make([]byte, 0, 24)
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		buf = strconv.AppendFloat(buf[:0], float64(v), 'f', -1, 32)
		b.Write(buf)
	}
	b.WriteByte(']')
	return b.String()
}
