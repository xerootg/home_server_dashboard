package query

// parser implements a recursive descent parser for bang-and-pipe expressions.
//
// Grammar:
//   expr     → or_expr
//   or_expr  → and_expr ('|' and_expr)*
//   and_expr → unary ('&' unary)*
//   unary    → '!' unary | primary
//   primary  → '(' expr ')' | quoted | term
type parser struct {
	tokens []Token
	pos    int
	input  string
}

// newParser creates a new parser for the given tokens.
func newParser(tokens []Token, input string) *parser {
	return &parser{
		tokens: tokens,
		pos:    0,
		input:  input,
	}
}

// current returns the current token.
func (p *parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

// advance moves to the next token and returns the previous one.
func (p *parser) advance() Token {
	token := p.current()
	p.pos++
	return token
}

// expect consumes a token of the expected type or returns an error.
func (p *parser) expect(tokenType TokenType) (Token, *ParseError) {
	token := p.current()
	if token.Type != tokenType {
		return Token{}, &ParseError{
			Message:  "Unexpected token: " + p.tokenTypeName(token.Type) + ", expected " + p.tokenTypeName(tokenType),
			Position: token.Position,
			Length:   max(token.Length, 1),
		}
	}
	return p.advance(), nil
}

// tokenTypeName returns a human-readable name for a token type.
func (p *parser) tokenTypeName(t TokenType) string {
	switch t {
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenOr:
		return "|"
	case TokenAnd:
		return "&"
	case TokenNot:
		return "!"
	case TokenQuoted:
		return "quoted string"
	case TokenTerm:
		return "term"
	case TokenEOF:
		return "end of expression"
	default:
		return "unknown"
	}
}

// parse parses the tokens into an AST.
func (p *parser) parse() (*Node, *ParseError) {
	if p.current().Type == TokenEOF {
		return nil, nil
	}

	node, err := p.parseOrExpr()
	if err != nil {
		return nil, err
	}

	if p.current().Type != TokenEOF {
		token := p.current()
		return nil, &ParseError{
			Message:  "Unexpected token: " + p.tokenTypeName(token.Type),
			Position: token.Position,
			Length:   max(token.Length, 1),
		}
	}

	return node, nil
}

// parseOrExpr parses: and_expr ('|' and_expr)*
func (p *parser) parseOrExpr() (*Node, *ParseError) {
	left, err := p.parseAndExpr()
	if err != nil {
		return nil, err
	}

	if p.current().Type == TokenOr {
		children := []*Node{left}
		for p.current().Type == TokenOr {
			p.advance() // consume |
			right, err := p.parseAndExpr()
			if err != nil {
				return nil, err
			}
			children = append(children, right)
		}
		return &Node{Type: NodeOr, Children: children}, nil
	}

	return left, nil
}

// parseAndExpr parses: unary ('&' unary)*
func (p *parser) parseAndExpr() (*Node, *ParseError) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}

	if p.current().Type == TokenAnd {
		children := []*Node{left}
		for p.current().Type == TokenAnd {
			p.advance() // consume &
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			children = append(children, right)
		}
		return &Node{Type: NodeAnd, Children: children}, nil
	}

	return left, nil
}

// parseUnary parses: '!' unary | primary
func (p *parser) parseUnary() (*Node, *ParseError) {
	if p.current().Type == TokenNot {
		p.advance() // consume !
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &Node{Type: NodeNot, Child: child}, nil
	}

	return p.parsePrimary()
}

// parsePrimary parses: '(' expr ')' | quoted | term
func (p *parser) parsePrimary() (*Node, *ParseError) {
	token := p.current()

	switch token.Type {
	case TokenLParen:
		p.advance() // consume (
		node, err := p.parseOrExpr()
		if err != nil {
			return nil, err
		}
		_, err = p.expect(TokenRParen)
		if err != nil {
			return nil, err
		}
		return node, nil

	case TokenQuoted:
		p.advance()
		return &Node{
			Type:    NodePattern,
			Pattern: token.Value,
			Regex:   escapeRegex(token.Value),
		}, nil

	case TokenTerm:
		p.advance()
		if token.Value == "" {
			return nil, &ParseError{
				Message:  "Empty term",
				Position: token.Position,
				Length:   1,
			}
		}
		return &Node{
			Type:    NodePattern,
			Pattern: token.Value,
			Regex:   escapeRegex(token.Value),
		}, nil

	case TokenEOF:
		return nil, &ParseError{
			Message:  "Unexpected end of expression",
			Position: token.Position,
			Length:   1,
		}

	default:
		return nil, &ParseError{
			Message:  "Unexpected token: " + p.tokenTypeName(token.Type),
			Position: token.Position,
			Length:   max(token.Length, 1),
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
