package query

import "strings"

// TokenType represents the type of lexical token.
type TokenType int

const (
	TokenLParen TokenType = iota
	TokenRParen
	TokenOr
	TokenAnd
	TokenNot
	TokenQuoted
	TokenTerm
	TokenEOF
)

// Token represents a lexical token.
type Token struct {
	Type     TokenType
	Value    string
	Position int
	Length   int
}

// lexer tokenizes input strings.
type lexer struct {
	input string
	pos   int
}

// newLexer creates a new lexer for the given input.
func newLexer(input string) *lexer {
	return &lexer{
		input: input,
		pos:   0,
	}
}

// tokenize converts the input string into a slice of tokens.
func (l *lexer) tokenize() ([]Token, *ParseError) {
	var tokens []Token

	for l.pos < len(l.input) {
		ch := l.input[l.pos]

		switch ch {
		case '(':
			tokens = append(tokens, Token{Type: TokenLParen, Value: "(", Position: l.pos, Length: 1})
			l.pos++
		case ')':
			tokens = append(tokens, Token{Type: TokenRParen, Value: ")", Position: l.pos, Length: 1})
			l.pos++
		case '|':
			tokens = append(tokens, Token{Type: TokenOr, Value: "|", Position: l.pos, Length: 1})
			l.pos++
		case '&':
			tokens = append(tokens, Token{Type: TokenAnd, Value: "&", Position: l.pos, Length: 1})
			l.pos++
		case '!':
			tokens = append(tokens, Token{Type: TokenNot, Value: "!", Position: l.pos, Length: 1})
			l.pos++
		case '"':
			token, err := l.readQuoted()
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
		case ' ', '\t', '\n', '\r':
			// Skip whitespace between tokens (but not inside terms)
			l.pos++
		default:
			token := l.readTerm()
			if token.Value != "" {
				tokens = append(tokens, token)
			}
		}
	}

	tokens = append(tokens, Token{Type: TokenEOF, Value: "", Position: l.pos, Length: 0})
	return tokens, nil
}

// readQuoted reads a quoted string (content between "").
func (l *lexer) readQuoted() (Token, *ParseError) {
	startPos := l.pos
	l.pos++ // skip opening quote

	var value strings.Builder
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '"' {
			l.pos++ // skip closing quote
			return Token{
				Type:     TokenQuoted,
				Value:    value.String(),
				Position: startPos,
				Length:   l.pos - startPos,
			}, nil
		}
		if ch == '\\' && l.pos+1 < len(l.input) {
			// Handle escape sequences
			nextCh := l.input[l.pos+1]
			if nextCh == '"' || nextCh == '\\' {
				value.WriteByte(nextCh)
				l.pos += 2
				continue
			}
		}
		value.WriteByte(ch)
		l.pos++
	}

	return Token{}, &ParseError{
		Message:  "Unterminated quoted string",
		Position: startPos,
		Length:   l.pos - startPos,
	}
}

// readTerm reads a term (sequence of non-operator, non-special characters).
func (l *lexer) readTerm() Token {
	startPos := l.pos
	var value strings.Builder

	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		// Stop at operators, parens, quotes
		if ch == '|' || ch == '&' || ch == '!' || ch == '(' || ch == ')' || ch == '"' {
			break
		}
		value.WriteByte(ch)
		l.pos++
	}

	// Trim trailing whitespace from term
	termValue := strings.TrimRight(value.String(), " \t\n\r")
	
	return Token{
		Type:     TokenTerm,
		Value:    termValue,
		Position: startPos,
		Length:   len(termValue),
	}
}
