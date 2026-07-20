// Package jssource provides lexical helpers for tests that inspect committed
// or served JavaScript without executing it.
package jssource

import "strings"

// WithoutComments removes JavaScript line and block comments while preserving
// source length, line boundaries, and quoted string/template content. Keeping
// non-comment bytes in place lets policy tests apply stable token/regexp checks
// without mistaking a security comment for executable source or corrupting a
// URL containing "//".
func WithoutComments(source string) string {
	const (
		stateCode = iota
		stateSingleQuote
		stateDoubleQuote
		stateTemplate
		stateLineComment
		stateBlockComment
	)

	var output strings.Builder
	output.Grow(len(source))
	state := stateCode

	for index := 0; index < len(source); index++ {
		current := source[index]
		next := byte(0)
		if index+1 < len(source) {
			next = source[index+1]
		}

		switch state {
		case stateCode:
			switch {
			case current == '/' && next == '/':
				output.WriteString("  ")
				index++
				state = stateLineComment
			case current == '/' && next == '*':
				output.WriteString("  ")
				index++
				state = stateBlockComment
			default:
				output.WriteByte(current)
				switch current {
				case '\'':
					state = stateSingleQuote
				case '"':
					state = stateDoubleQuote
				case '`':
					state = stateTemplate
				}
			}
		case stateSingleQuote, stateDoubleQuote, stateTemplate:
			output.WriteByte(current)
			if current == '\\' && index+1 < len(source) {
				index++
				output.WriteByte(source[index])
				continue
			}
			if (state == stateSingleQuote && current == '\'') ||
				(state == stateDoubleQuote && current == '"') ||
				(state == stateTemplate && current == '`') {
				state = stateCode
			}
		case stateLineComment:
			if current == '\n' || current == '\r' {
				output.WriteByte(current)
				state = stateCode
			} else {
				output.WriteByte(' ')
			}
		case stateBlockComment:
			switch {
			case current == '*' && next == '/':
				output.WriteString("  ")
				index++
				state = stateCode
			case current == '\n' || current == '\r':
				output.WriteByte(current)
			default:
				output.WriteByte(' ')
			}
		}
	}

	return output.String()
}
