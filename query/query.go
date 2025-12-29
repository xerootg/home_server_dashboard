// Package query provides parsing and compilation of bang-and-pipe search expressions.
// The syntax supports:
//   - | (OR): A|B matches lines containing A or B
//   - & (AND): A&B matches lines containing both A and B
//   - ! (NOT): !A matches lines NOT containing A
//   - () (grouping): (A|B)&C
//   - "" (literals): "A|B" matches literal "A|B"
//
// Operator precedence (lowest to highest): OR, AND, NOT
// Example: !A&B|C means (!A AND B) OR C
package query

import (
	"encoding/json"
	"regexp"
	"strings"
)

// NodeType represents the type of AST node.
type NodeType string

const (
	NodePattern NodeType = "pattern"
	NodeOr      NodeType = "or"
	NodeAnd     NodeType = "and"
	NodeNot     NodeType = "not"
)

// Node represents a node in the AST.
type Node struct {
	Type     NodeType `json:"type"`
	Pattern  string   `json:"pattern,omitempty"`  // Original pattern text (for pattern nodes)
	Regex    string   `json:"regex,omitempty"`    // Escaped regex pattern (for pattern nodes)
	Children []*Node  `json:"children,omitempty"` // For or/and nodes
	Child    *Node    `json:"child,omitempty"`    // For not nodes
}

// ParseError represents a syntax error with position information.
type ParseError struct {
	Message  string `json:"message"`
	Position int    `json:"position"`
	Length   int    `json:"length"`
}

func (e *ParseError) Error() string {
	return e.Message
}

// CompileResult is the result of compiling an expression.
type CompileResult struct {
	Valid bool        `json:"valid"`
	AST   *Node       `json:"ast,omitempty"`
	Error *ParseError `json:"error,omitempty"`
}

// Compile parses and compiles a bang-and-pipe expression into an AST.
func Compile(expr string) *CompileResult {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return &CompileResult{
			Valid: true,
			AST:   nil,
		}
	}

	lexer := newLexer(expr)
	tokens, err := lexer.tokenize()
	if err != nil {
		return &CompileResult{
			Valid: false,
			Error: err,
		}
	}

	parser := newParser(tokens, expr)
	ast, err := parser.parse()
	if err != nil {
		return &CompileResult{
			Valid: false,
			Error: err,
		}
	}

	return &CompileResult{
		Valid: true,
		AST:   ast,
	}
}

// CompileToJSON compiles an expression and returns the result as JSON.
func CompileToJSON(expr string) ([]byte, error) {
	result := Compile(expr)
	return json.Marshal(result)
}

// escapeRegex escapes special regex characters in a string.
func escapeRegex(s string) string {
	special := regexp.MustCompile(`[.*+?^${}()|[\]\\]`)
	return special.ReplaceAllString(s, `\$0`)
}
