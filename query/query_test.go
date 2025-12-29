package query

import (
	"testing"
)

func TestCompile_EmptyExpression(t *testing.T) {
	result := Compile("")
	if !result.Valid {
		t.Errorf("Expected valid result for empty expression")
	}
	if result.AST != nil {
		t.Errorf("Expected nil AST for empty expression")
	}
}

func TestCompile_SimpleTerm(t *testing.T) {
	result := Compile("error")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST == nil {
		t.Fatal("Expected non-nil AST")
	}
	if result.AST.Type != NodePattern {
		t.Errorf("Expected pattern node, got %s", result.AST.Type)
	}
	if result.AST.Pattern != "error" {
		t.Errorf("Expected pattern 'error', got '%s'", result.AST.Pattern)
	}
	if result.AST.Regex != "error" {
		t.Errorf("Expected regex 'error', got '%s'", result.AST.Regex)
	}
}

func TestCompile_TermWithSpaces(t *testing.T) {
	result := Compile("hello world")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Pattern != "hello world" {
		t.Errorf("Expected pattern 'hello world', got '%s'", result.AST.Pattern)
	}
}

func TestCompile_QuotedString(t *testing.T) {
	result := Compile(`"hello|world"`)
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Pattern != "hello|world" {
		t.Errorf("Expected pattern 'hello|world', got '%s'", result.AST.Pattern)
	}
	// | should be escaped in regex
	if result.AST.Regex != `hello\|world` {
		t.Errorf("Expected regex 'hello\\|world', got '%s'", result.AST.Regex)
	}
}

func TestCompile_OrExpression(t *testing.T) {
	result := Compile("error|warn")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeOr {
		t.Fatalf("Expected or node, got %s", result.AST.Type)
	}
	if len(result.AST.Children) != 2 {
		t.Fatalf("Expected 2 children, got %d", len(result.AST.Children))
	}
	if result.AST.Children[0].Pattern != "error" {
		t.Errorf("Expected first child 'error', got '%s'", result.AST.Children[0].Pattern)
	}
	if result.AST.Children[1].Pattern != "warn" {
		t.Errorf("Expected second child 'warn', got '%s'", result.AST.Children[1].Pattern)
	}
}

func TestCompile_AndExpression(t *testing.T) {
	result := Compile("error&fatal")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeAnd {
		t.Fatalf("Expected and node, got %s", result.AST.Type)
	}
	if len(result.AST.Children) != 2 {
		t.Fatalf("Expected 2 children, got %d", len(result.AST.Children))
	}
}

func TestCompile_NotExpression(t *testing.T) {
	result := Compile("!error")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeNot {
		t.Fatalf("Expected not node, got %s", result.AST.Type)
	}
	if result.AST.Child == nil {
		t.Fatal("Expected child node")
	}
	if result.AST.Child.Pattern != "error" {
		t.Errorf("Expected child pattern 'error', got '%s'", result.AST.Child.Pattern)
	}
}

func TestCompile_Precedence_AndBeforeOr(t *testing.T) {
	// A|B&C should parse as A|(B&C)
	result := Compile("A|B&C")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeOr {
		t.Fatalf("Expected or at top level, got %s", result.AST.Type)
	}
	if len(result.AST.Children) != 2 {
		t.Fatalf("Expected 2 children, got %d", len(result.AST.Children))
	}
	// First child should be A
	if result.AST.Children[0].Type != NodePattern || result.AST.Children[0].Pattern != "A" {
		t.Errorf("Expected first child to be pattern A")
	}
	// Second child should be B&C
	if result.AST.Children[1].Type != NodeAnd {
		t.Errorf("Expected second child to be and node, got %s", result.AST.Children[1].Type)
	}
}

func TestCompile_Precedence_NotBeforeAnd(t *testing.T) {
	// !A&B should parse as (!A)&B
	result := Compile("!A&B")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeAnd {
		t.Fatalf("Expected and at top level, got %s", result.AST.Type)
	}
	if len(result.AST.Children) != 2 {
		t.Fatalf("Expected 2 children, got %d", len(result.AST.Children))
	}
	// First child should be !A
	if result.AST.Children[0].Type != NodeNot {
		t.Errorf("Expected first child to be not node")
	}
}

func TestCompile_Parentheses(t *testing.T) {
	// (A|B)&C should parse as (A|B)&C
	result := Compile("(A|B)&C")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeAnd {
		t.Fatalf("Expected and at top level, got %s", result.AST.Type)
	}
	// First child should be OR
	if result.AST.Children[0].Type != NodeOr {
		t.Errorf("Expected first child to be or node")
	}
}

func TestCompile_NestedExpression(t *testing.T) {
	// ((A|B)|C)&D
	result := Compile("((A|B)|C)&D")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeAnd {
		t.Fatalf("Expected and at top level, got %s", result.AST.Type)
	}
}

func TestCompile_MultipleOr(t *testing.T) {
	result := Compile("A|B|C")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeOr {
		t.Fatalf("Expected or node, got %s", result.AST.Type)
	}
	if len(result.AST.Children) != 3 {
		t.Errorf("Expected 3 children, got %d", len(result.AST.Children))
	}
}

func TestCompile_DoubleNot(t *testing.T) {
	result := Compile("!!A")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeNot {
		t.Fatalf("Expected not node, got %s", result.AST.Type)
	}
	if result.AST.Child.Type != NodeNot {
		t.Errorf("Expected nested not node")
	}
}

func TestCompile_QuotedWithOperators(t *testing.T) {
	// "!A|B" should be a single pattern, not parsed as operators
	result := Compile(`"!A|B"`)
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodePattern {
		t.Fatalf("Expected pattern node, got %s", result.AST.Type)
	}
	if result.AST.Pattern != "!A|B" {
		t.Errorf("Expected pattern '!A|B', got '%s'", result.AST.Pattern)
	}
}

func TestCompile_EscapedQuoteInQuoted(t *testing.T) {
	result := Compile(`"hello\"world"`)
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Pattern != `hello"world` {
		t.Errorf("Expected pattern 'hello\"world', got '%s'", result.AST.Pattern)
	}
}

func TestCompile_Error_UnmatchedParen(t *testing.T) {
	result := Compile("(A|B")
	if result.Valid {
		t.Errorf("Expected invalid result for unmatched paren")
	}
	if result.Error == nil {
		t.Fatal("Expected error")
	}
	if result.Error.Position < 0 {
		t.Errorf("Expected valid error position")
	}
}

func TestCompile_Error_UnterminatedQuote(t *testing.T) {
	result := Compile(`"hello`)
	if result.Valid {
		t.Errorf("Expected invalid result for unterminated quote")
	}
	if result.Error == nil {
		t.Fatal("Expected error")
	}
}

func TestCompile_Error_EmptyParens(t *testing.T) {
	result := Compile("()")
	if result.Valid {
		t.Errorf("Expected invalid result for empty parens")
	}
}

func TestCompile_Error_TrailingOperator(t *testing.T) {
	result := Compile("A|")
	if result.Valid {
		t.Errorf("Expected invalid result for trailing operator")
	}
}

func TestCompile_Error_LeadingOperator(t *testing.T) {
	result := Compile("|A")
	if result.Valid {
		t.Errorf("Expected invalid result for leading operator")
	}
}

func TestCompile_RegexEscaping(t *testing.T) {
	// Special regex chars should be escaped
	result := Compile("hello.world")
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Regex != `hello\.world` {
		t.Errorf("Expected regex 'hello\\.world', got '%s'", result.AST.Regex)
	}
}

func TestCompile_ComplexExpression(t *testing.T) {
	// Complex real-world example
	result := Compile(`(error|warn)&!health&!"DEBUG"`)
	if !result.Valid {
		t.Fatalf("Expected valid result, got error: %v", result.Error)
	}
	if result.AST.Type != NodeAnd {
		t.Fatalf("Expected and at top level, got %s", result.AST.Type)
	}
	if len(result.AST.Children) != 3 {
		t.Errorf("Expected 3 children in and, got %d", len(result.AST.Children))
	}
}

func TestCompileToJSON(t *testing.T) {
	jsonBytes, err := CompileToJSON("A|B")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(jsonBytes) == 0 {
		t.Error("Expected non-empty JSON")
	}
	// Basic sanity check
	jsonStr := string(jsonBytes)
	if jsonStr == "" {
		t.Error("Expected non-empty JSON string")
	}
}
