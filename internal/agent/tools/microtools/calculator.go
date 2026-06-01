// Spec 065 SCOPE-3 — calculator micro-tool.
//
// calculator evaluates pure arithmetic expressions using a dedicated
// numeric parser with a strictly bounded grammar:
//
//   expr    := term (("+"|"-") term)*
//   term    := factor (("*"|"/"|"%") factor)*
//   factor  := unary ("**" factor)?     // right-associative exponentiation
//   unary   := ("+"|"-")? primary
//   primary := number | "(" expr ")"
//   number  := digits ("." digits)? ([eE] ("+"|"-")? digits)?
//
// The parser REJECTS identifiers, function calls, names, dotted
// member access, brackets, assignment, comparisons, bitwise ops,
// shell metacharacters, semicolons, and any character outside the
// digit/operator/whitespace alphabet. There is no eval, no
// interpolation, no string concatenation, no host function table.
//
// Non-finite results (NaN, +/-Inf) are rejected as failed. Division
// or modulo by zero is rejected as failed.

package microtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/smackerel/smackerel/internal/agent"
)

// CalculatorToolName is the canonical tool name registered through
// the spec 037 agent registry.
const CalculatorToolName = "calculator"

// CalculatorServices holds the runtime dependencies.
type CalculatorServices struct {
	// MaxExpressionChars caps input length. Required, >= 1.
	MaxExpressionChars int
}

var (
	calcSvcMu sync.RWMutex
	calcSvc   *CalculatorServices
)

// SetCalculatorServices wires the production calculator runtime.
// Pass nil to clear (test-only).
func SetCalculatorServices(s *CalculatorServices) {
	calcSvcMu.Lock()
	defer calcSvcMu.Unlock()
	calcSvc = s
}

// ResetCalculatorServicesForTest clears the wired services. Test-only.
func ResetCalculatorServicesForTest() {
	calcSvcMu.Lock()
	defer calcSvcMu.Unlock()
	calcSvc = nil
}

func loadCalculatorServices() (*CalculatorServices, error) {
	calcSvcMu.RLock()
	defer calcSvcMu.RUnlock()
	if calcSvc == nil {
		return nil, errors.New("calculator_not_configured")
	}
	if calcSvc.MaxExpressionChars < 1 {
		return nil, fmt.Errorf("calculator_max_expression_chars_invalid: %d", calcSvc.MaxExpressionChars)
	}
	return calcSvc, nil
}

// -------------------- schemas --------------------

var calculatorInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["expression"],
  "properties": {
    "expression": {"type": "string", "minLength": 1}
  }
}`)

var calculatorOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": true,
  "required": ["schema_version", "status", "source"],
  "properties": {
    "schema_version": {"type": "string"},
    "status":         {"type": "string", "enum": ["resolved", "ambiguous", "failed"]},
    "source":         {"type": "object"}
  }
}`)

// -------------------- registration --------------------

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             CalculatorToolName,
		Description:      "Evaluate a pure arithmetic expression over numeric literals, parentheses, unary signs, and the operators + - * / % **. Identifiers, function calls, and non-finite results are rejected.",
		InputSchema:      calculatorInputSchema,
		OutputSchema:     calculatorOutputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/agent/tools/microtools",
		PerCallTimeoutMs: 250,
		Handler:          handleCalculator,
	})
}

// -------------------- handler --------------------

type calculatorInput struct {
	Expression string `json:"expression"`
}

func handleCalculator(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadCalculatorServices()
	if err != nil {
		return nil, err
	}
	var in calculatorInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("calculator_bad_input: %w", err)
	}
	expr := strings.TrimSpace(in.Expression)
	if expr == "" {
		return nil, errors.New("calculator_empty_expression")
	}
	if len(expr) > svc.MaxExpressionChars {
		return marshalCalcEnvelope(calcFailed("expression_too_long",
			fmt.Sprintf("expression length %d exceeds limit %d", len(expr), svc.MaxExpressionChars)))
	}
	tokens, err := tokenize(expr)
	if err != nil {
		return marshalCalcEnvelope(calcFailed("invalid_expression", err.Error()))
	}
	p := &calcParser{tokens: tokens}
	value, err := p.parseExpr()
	if err != nil {
		return marshalCalcEnvelope(calcFailed("invalid_expression", err.Error()))
	}
	if !p.atEnd() {
		return marshalCalcEnvelope(calcFailed("invalid_expression", "unexpected trailing input"))
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return marshalCalcEnvelope(calcFailed("non_finite_result", "expression evaluated to a non-finite value"))
	}
	return marshalCalcEnvelope(calcResolved(value))
}

func calcResolved(value float64) Envelope {
	const sigDigits = 6
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusResolved,
		Value: map[string]any{
			"value":     roundToSignificant(value, sigDigits),
			"precision": sigDigits,
		},
		Source: Source{
			Provider:    "calculator",
			Kind:        SourceKindLocalCompute,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Compute: calculator",
		},
	}
}

func calcFailed(code, msg string) Envelope {
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusFailed,
		Source: Source{
			Provider:    "calculator",
			Kind:        SourceKindLocalCompute,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Compute: calculator",
		},
		Error: &Error{Code: code, Message: msg},
	}
}

func marshalCalcEnvelope(env Envelope) (json.RawMessage, error) {
	if err := ValidateEnvelope(env); err != nil {
		return nil, fmt.Errorf("calculator_envelope_invalid: %w", err)
	}
	return json.Marshal(env)
}

// -------------------- tokenizer --------------------

type tokKind int

const (
	tkNumber tokKind = iota
	tkPlus
	tkMinus
	tkStar
	tkSlash
	tkPercent
	tkPow // **
	tkLParen
	tkRParen
)

type token struct {
	kind tokKind
	num  float64
	pos  int
}

func tokenize(src string) ([]token, error) {
	var out []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '+':
			out = append(out, token{kind: tkPlus, pos: i})
			i++
		case c == '-':
			out = append(out, token{kind: tkMinus, pos: i})
			i++
		case c == '*':
			if i+1 < len(src) && src[i+1] == '*' {
				out = append(out, token{kind: tkPow, pos: i})
				i += 2
			} else {
				out = append(out, token{kind: tkStar, pos: i})
				i++
			}
		case c == '/':
			out = append(out, token{kind: tkSlash, pos: i})
			i++
		case c == '%':
			out = append(out, token{kind: tkPercent, pos: i})
			i++
		case c == '(':
			out = append(out, token{kind: tkLParen, pos: i})
			i++
		case c == ')':
			out = append(out, token{kind: tkRParen, pos: i})
			i++
		case c >= '0' && c <= '9' || c == '.':
			n, consumed, err := readNumber(src, i)
			if err != nil {
				return nil, err
			}
			out = append(out, token{kind: tkNumber, num: n, pos: i})
			i += consumed
		default:
			r := rune(c)
			if unicode.IsLetter(r) || r == '_' {
				return nil, fmt.Errorf("identifiers not allowed at position %d (saw %q)", i, string(r))
			}
			return nil, fmt.Errorf("unexpected character %q at position %d", string(r), i)
		}
	}
	return out, nil
}

// readNumber parses a numeric literal: digits, optional dot, digits,
// optional [eE] [+-]? digits. Anything else (a trailing letter, an
// underscore, a second dot) is rejected.
func readNumber(src string, start int) (float64, int, error) {
	j := start
	sawDigit := false
	sawDot := false
	// integer part
	for j < len(src) && src[j] >= '0' && src[j] <= '9' {
		sawDigit = true
		j++
	}
	if j < len(src) && src[j] == '.' {
		sawDot = true
		j++
		for j < len(src) && src[j] >= '0' && src[j] <= '9' {
			sawDigit = true
			j++
		}
	}
	if j < len(src) && (src[j] == 'e' || src[j] == 'E') {
		j++
		if j < len(src) && (src[j] == '+' || src[j] == '-') {
			j++
		}
		expStart := j
		for j < len(src) && src[j] >= '0' && src[j] <= '9' {
			j++
		}
		if j == expStart {
			return 0, 0, fmt.Errorf("malformed exponent at position %d", expStart)
		}
	}
	if !sawDigit {
		return 0, 0, fmt.Errorf("malformed number at position %d", start)
	}
	// reject a trailing identifier char (e.g. "1.5abc")
	if j < len(src) {
		r := rune(src[j])
		if unicode.IsLetter(r) || r == '_' {
			return 0, 0, fmt.Errorf("identifiers not allowed at position %d (saw %q)", j, string(r))
		}
	}
	_ = sawDot
	var f float64
	_, err := fmt.Sscanf(src[start:j], "%g", &f)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed number %q at position %d: %w", src[start:j], start, err)
	}
	return f, j - start, nil
}

// -------------------- parser --------------------

type calcParser struct {
	tokens []token
	i      int
}

func (p *calcParser) atEnd() bool { return p.i >= len(p.tokens) }

func (p *calcParser) peek() (token, bool) {
	if p.atEnd() {
		return token{}, false
	}
	return p.tokens[p.i], true
}

func (p *calcParser) advance() token {
	t := p.tokens[p.i]
	p.i++
	return t
}

func (p *calcParser) parseExpr() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		t, ok := p.peek()
		if !ok || (t.kind != tkPlus && t.kind != tkMinus) {
			return left, nil
		}
		p.advance()
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if t.kind == tkPlus {
			left += right
		} else {
			left -= right
		}
	}
}

func (p *calcParser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		t, ok := p.peek()
		if !ok || (t.kind != tkStar && t.kind != tkSlash && t.kind != tkPercent) {
			return left, nil
		}
		p.advance()
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		switch t.kind {
		case tkStar:
			left *= right
		case tkSlash:
			if right == 0 {
				return 0, errors.New("division by zero")
			}
			left /= right
		case tkPercent:
			if right == 0 {
				return 0, errors.New("modulo by zero")
			}
			left = math.Mod(left, right)
		}
	}
}

func (p *calcParser) parseFactor() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	if t, ok := p.peek(); ok && t.kind == tkPow {
		p.advance()
		right, err := p.parseFactor() // right-associative
		if err != nil {
			return 0, err
		}
		return math.Pow(left, right), nil
	}
	return left, nil
}

func (p *calcParser) parseUnary() (float64, error) {
	if t, ok := p.peek(); ok && (t.kind == tkPlus || t.kind == tkMinus) {
		p.advance()
		v, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		if t.kind == tkMinus {
			return -v, nil
		}
		return v, nil
	}
	return p.parsePrimary()
}

func (p *calcParser) parsePrimary() (float64, error) {
	t, ok := p.peek()
	if !ok {
		return 0, errors.New("unexpected end of expression")
	}
	switch t.kind {
	case tkNumber:
		p.advance()
		return t.num, nil
	case tkLParen:
		p.advance()
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		nt, ok := p.peek()
		if !ok || nt.kind != tkRParen {
			return 0, errors.New("missing closing parenthesis")
		}
		p.advance()
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected token at position %d", t.pos)
	}
}
