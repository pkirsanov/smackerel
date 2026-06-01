package tools

// calculator is a small recursive-descent arithmetic evaluator. It does
// NOT use go/parser, go/ast, or any text/template-style "eval" — those
// surfaces accept arbitrary Go expressions (variable refs, package
// access, struct literals) and would defeat sandboxing. The parser
// below only accepts numeric literals, the operators +,-,*,/, unary
// minus, parentheses, and an explicit allowlist of pure math functions
// (sqrt, abs, min, max, pow). Recursion is depth-capped to keep
// adversarial nested input bounded.

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

// Typed sentinel errors returned by calculator.
var (
	ErrParseFailure  = &ok.ToolError{Code: "parse_failure", Message: "expression could not be parsed"}
	ErrUnknownFunc   = &ok.ToolError{Code: "unknown_function", Message: "function is not in the allowlist"}
	ErrDivideByZero  = &ok.ToolError{Code: "divide_by_zero", Message: "division by zero"}
	ErrResultNaN     = &ok.ToolError{Code: "result_nan", Message: "expression produced NaN or Inf"}
	ErrCalcMalformed = &ok.ToolError{Code: "malformed_params", Message: "params do not match schema"}
	ErrDepthExceeded = &ok.ToolError{Code: "parse_failure", Message: "expression nests deeper than the safety cap"}
)

const calculatorSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["expression"],
  "properties": {
    "expression": {"type": "string"}
  }
}`

const calcMaxDepth = 512

type calculatorParams struct {
	Expression *string `json:"expression"`
}

type calculatorOutput struct {
	Result float64 `json:"result"`
}

// Calculator is the registry-facing handle.
type Calculator struct{}

// NewCalculator returns a value usable as openknowledge.Tool.
func NewCalculator() *Calculator { return &Calculator{} }

// Name reports the registry key.
func (Calculator) Name() string { return "calculator" }

// Description summarises the tool for the planner prompt.
func (Calculator) Description() string {
	return "Evaluate an arithmetic expression with +, -, *, /, parentheses, unary minus, and allowlisted math functions (sqrt, abs, min, max, pow)."
}

// ParamsSchema returns the JSONSchema for Execute params.
func (Calculator) ParamsSchema() json.RawMessage { return json.RawMessage(calculatorSchema) }

// Execute parses and evaluates the expression.
func (c Calculator) Execute(_ context.Context, params json.RawMessage) (*ok.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p calculatorParams
	if err := dec.Decode(&p); err != nil {
		return &ok.ToolResult{Error: ErrCalcMalformed}, nil
	}
	if p.Expression == nil {
		return &ok.ToolResult{Error: ErrCalcMalformed}, nil
	}
	expr := *p.Expression

	tokens, err := tokenize(expr)
	if err != nil {
		return &ok.ToolResult{Error: err}, nil
	}
	parser := &calcParser{tokens: tokens}
	result, perr := parser.parseExpression(0)
	if perr != nil {
		return &ok.ToolResult{Error: perr}, nil
	}
	if parser.pos != len(tokens) {
		return &ok.ToolResult{Error: ErrParseFailure}, nil
	}
	if math.IsNaN(result) {
		return &ok.ToolResult{Error: ErrResultNaN}, nil
	}
	if math.IsInf(result, 0) {
		return &ok.ToolResult{Error: ErrResultNaN}, nil
	}

	inJSON, _ := json.Marshal(map[string]any{"expression": expr})
	outJSON, mErr := json.Marshal(calculatorOutput{Result: result})
	if mErr != nil {
		return &ok.ToolResult{Error: &ok.ToolError{Code: "encode_failure", Message: mErr.Error()}}, nil
	}

	return &ok.ToolResult{
		Sources: []ok.Source{{
			Kind: ok.SourceToolComputation,
			Computation: &ok.ComputationSource{
				Tool:   c.Name(),
				Input:  inJSON,
				Output: outJSON,
			},
		}},
		Computation: &ok.Computation{
			Tool:   c.Name(),
			Input:  inJSON,
			Output: outJSON,
		},
		Snippets: nil,
		Error:    nil,
	}, nil
}

// --- Lexer ---

type tokKind int

const (
	tNum tokKind = iota + 1
	tIdent
	tPlus
	tMinus
	tStar
	tSlash
	tLParen
	tRParen
	tComma
)

type token struct {
	kind tokKind
	num  float64
	text string
}

func tokenize(s string) ([]token, *ok.ToolError) {
	out := make([]token, 0, len(s))
	i := 0
	for i < len(s) {
		r := rune(s[i])
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '+':
			out = append(out, token{kind: tPlus})
			i++
		case r == '-':
			out = append(out, token{kind: tMinus})
			i++
		case r == '*':
			out = append(out, token{kind: tStar})
			i++
		case r == '/':
			out = append(out, token{kind: tSlash})
			i++
		case r == '(':
			out = append(out, token{kind: tLParen})
			i++
		case r == ')':
			out = append(out, token{kind: tRParen})
			i++
		case r == ',':
			out = append(out, token{kind: tComma})
			i++
		case r >= '0' && r <= '9' || r == '.':
			j := i
			seenDot := false
			for j < len(s) {
				c := s[j]
				if c >= '0' && c <= '9' {
					j++
					continue
				}
				if c == '.' && !seenDot {
					seenDot = true
					j++
					continue
				}
				break
			}
			v, err := strconv.ParseFloat(s[i:j], 64)
			if err != nil {
				return nil, ErrParseFailure
			}
			out = append(out, token{kind: tNum, num: v})
			i = j
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(s) {
				c := rune(s[j])
				if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' {
					j++
					continue
				}
				break
			}
			out = append(out, token{kind: tIdent, text: s[i:j]})
			i = j
		default:
			return nil, ErrParseFailure
		}
	}
	return out, nil
}

// --- Parser ---

type calcParser struct {
	tokens []token
	pos    int
}

func (p *calcParser) peek() (token, bool) {
	if p.pos >= len(p.tokens) {
		return token{}, false
	}
	return p.tokens[p.pos], true
}

func (p *calcParser) advance() token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// parseExpression: term (('+'|'-') term)*
func (p *calcParser) parseExpression(depth int) (float64, *ok.ToolError) {
	if depth > calcMaxDepth {
		return 0, ErrDepthExceeded
	}
	left, err := p.parseTerm(depth + 1)
	if err != nil {
		return 0, err
	}
	for {
		t, ok1 := p.peek()
		if !ok1 || (t.kind != tPlus && t.kind != tMinus) {
			return left, nil
		}
		p.advance()
		right, rerr := p.parseTerm(depth + 1)
		if rerr != nil {
			return 0, rerr
		}
		if t.kind == tPlus {
			left += right
		} else {
			left -= right
		}
	}
}

// parseTerm: unary (('*'|'/') unary)*
func (p *calcParser) parseTerm(depth int) (float64, *ok.ToolError) {
	if depth > calcMaxDepth {
		return 0, ErrDepthExceeded
	}
	left, err := p.parseUnary(depth + 1)
	if err != nil {
		return 0, err
	}
	for {
		t, ok1 := p.peek()
		if !ok1 || (t.kind != tStar && t.kind != tSlash) {
			return left, nil
		}
		p.advance()
		right, rerr := p.parseUnary(depth + 1)
		if rerr != nil {
			return 0, rerr
		}
		if t.kind == tStar {
			left *= right
		} else {
			if right == 0 {
				return 0, ErrDivideByZero
			}
			left /= right
		}
	}
}

// parseUnary: '-' unary | '+' unary | primary
func (p *calcParser) parseUnary(depth int) (float64, *ok.ToolError) {
	if depth > calcMaxDepth {
		return 0, ErrDepthExceeded
	}
	t, ok1 := p.peek()
	if !ok1 {
		return 0, ErrParseFailure
	}
	if t.kind == tMinus {
		p.advance()
		v, err := p.parseUnary(depth + 1)
		if err != nil {
			return 0, err
		}
		return -v, nil
	}
	if t.kind == tPlus {
		p.advance()
		return p.parseUnary(depth + 1)
	}
	return p.parsePrimary(depth + 1)
}

// parsePrimary: number | ident '(' args ')' | '(' expression ')'
func (p *calcParser) parsePrimary(depth int) (float64, *ok.ToolError) {
	if depth > calcMaxDepth {
		return 0, ErrDepthExceeded
	}
	t, ok1 := p.peek()
	if !ok1 {
		return 0, ErrParseFailure
	}
	switch t.kind {
	case tNum:
		p.advance()
		return t.num, nil
	case tLParen:
		p.advance()
		v, err := p.parseExpression(depth + 1)
		if err != nil {
			return 0, err
		}
		next, ok2 := p.peek()
		if !ok2 || next.kind != tRParen {
			return 0, ErrParseFailure
		}
		p.advance()
		return v, nil
	case tIdent:
		p.advance()
		next, ok2 := p.peek()
		if !ok2 || next.kind != tLParen {
			// Bare identifiers (variables) are not allowed.
			return 0, ErrUnknownFunc
		}
		p.advance() // consume '('
		args := []float64{}
		// support zero-arg or N-arg lists
		if peek, ok3 := p.peek(); ok3 && peek.kind != tRParen {
			for {
				v, err := p.parseExpression(depth + 1)
				if err != nil {
					return 0, err
				}
				args = append(args, v)
				peek2, ok4 := p.peek()
				if !ok4 {
					return 0, ErrParseFailure
				}
				if peek2.kind == tComma {
					p.advance()
					continue
				}
				break
			}
		}
		closing, ok5 := p.peek()
		if !ok5 || closing.kind != tRParen {
			return 0, ErrParseFailure
		}
		p.advance()
		return applyFunc(t.text, args)
	}
	return 0, ErrParseFailure
}

func applyFunc(name string, args []float64) (float64, *ok.ToolError) {
	switch name {
	case "sqrt":
		if len(args) != 1 {
			return 0, ErrParseFailure
		}
		r := math.Sqrt(args[0])
		if math.IsNaN(r) {
			return 0, ErrResultNaN
		}
		return r, nil
	case "abs":
		if len(args) != 1 {
			return 0, ErrParseFailure
		}
		return math.Abs(args[0]), nil
	case "min":
		if len(args) < 2 {
			return 0, ErrParseFailure
		}
		r := args[0]
		for _, v := range args[1:] {
			r = math.Min(r, v)
		}
		return r, nil
	case "max":
		if len(args) < 2 {
			return 0, ErrParseFailure
		}
		r := args[0]
		for _, v := range args[1:] {
			r = math.Max(r, v)
		}
		return r, nil
	case "pow":
		if len(args) != 2 {
			return 0, ErrParseFailure
		}
		r := math.Pow(args[0], args[1])
		if math.IsNaN(r) {
			return 0, ErrResultNaN
		}
		return r, nil
	}
	return 0, ErrUnknownFunc
}
