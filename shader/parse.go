package shader

import (
	"fmt"
	"strings"
)

// Node is one statement of a shader file:
//
//	Key ( Args ) Name = Value
//	Key ( Args ) Name { Body and Items }
//	Key [[ Raw ]]
//
// with every part but Key optional. A block holds statements, and bare words
// or strings, which are its Items: Defines = { "TREE" "WIND" }.
type Node struct {
	Key   string
	Args  string
	Name  string
	Value string
	Items []string
	Body  []*Node

	// Raw is the text of a code block, or the body of a structure or a
	// constant buffer, which are declarations in the shader language rather
	// than statements.
	Raw string

	// Line is where the statement starts, counting from 1.
	Line int
}

// rawBodies are the statements whose braces hold declarations in the shader
// language, read as they are.
var rawBodies = map[string]bool{
	"VertexStruct":   true,
	"ConstantBuffer": true,
	"struct":         true,
}

// parseNodes reads the statements of a shader file.
func parseNodes(source string) ([]*Node, error) {
	// Editors on Windows like to start a file with a byte order mark.
	p := &parser{text: strings.TrimPrefix(source, "\xef\xbb\xbf"), line: 1}

	nodes, err := p.statements(false)
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

type parser struct {
	text string
	pos  int
	line int
}

func (p *parser) errorf(format string, args ...any) error {
	return fmt.Errorf("line %d: %s", p.line, fmt.Sprintf(format, args...))
}

// statements reads statements up to the end of the file, or of the block
// when inBlock is set, which also returns the bare items of the block.
func (p *parser) statements(inBlock bool) ([]*Node, error) {
	nodes, _, err := p.block(inBlock)

	return nodes, err
}

func (p *parser) block(inBlock bool) ([]*Node, []string, error) {
	var (
		nodes []*Node
		items []string
	)

	for {
		p.skip()

		if p.pos >= len(p.text) {
			if inBlock {
				return nil, nil, p.errorf("a block is never closed")
			}

			return nodes, items, nil
		}

		switch p.text[p.pos] {
		case '}':
			if !inBlock {
				return nil, nil, p.errorf("a closing brace closes nothing")
			}

			p.pos++

			return nodes, items, nil

		case ';':
			p.pos++

			continue

		case '"':
			value, err := p.quoted()
			if err != nil {
				return nil, nil, err
			}

			items = append(items, value)

			continue
		}

		word := p.word()
		if word == "" {
			return nil, nil, p.errorf("unexpected %q", p.text[p.pos])
		}

		if !p.startsStatement() {
			items = append(items, word)

			continue
		}

		node, err := p.statement(word)
		if err != nil {
			return nil, nil, err
		}

		nodes = append(nodes, node)
	}
}

// startsStatement reports whether the word just read begins a statement
// rather than being a bare item of a block: it is followed by what a
// statement has, or by a name and then a block.
func (p *parser) startsStatement() bool {
	start, line := p.pos, p.line
	defer func() { p.pos, p.line = start, line }()

	p.skip()

	if p.pos >= len(p.text) {
		return false
	}

	switch {
	case p.text[p.pos] == '=', p.text[p.pos] == '{', p.text[p.pos] == '(', strings.HasPrefix(p.text[p.pos:], "[["):
		return true
	}

	// A name, then a block: TextureSampler DiffuseMap { ... }
	if p.word() == "" {
		return false
	}

	p.skip()

	return p.pos < len(p.text) && p.text[p.pos] == '{'
}

func (p *parser) statement(key string) (*Node, error) {
	node := &Node{Key: key, Line: p.line}

	p.skip()

	if p.pos < len(p.text) && p.text[p.pos] == '(' {
		end := strings.IndexByte(p.text[p.pos:], ')')
		if end < 0 {
			return nil, p.errorf("%s: a parenthesis is never closed", key)
		}

		node.Args = strings.TrimSpace(p.text[p.pos+1 : p.pos+end])
		p.advance(end + 1)
		p.skip()
	}

	if p.pos < len(p.text) && (isWordByte(p.text[p.pos]) || p.text[p.pos] == '"') {
		name, err := p.value()
		if err != nil {
			return nil, err
		}

		node.Name = name
		p.skip()
	}

	if p.pos >= len(p.text) {
		return node, nil
	}

	switch {
	case strings.HasPrefix(p.text[p.pos:], "[["):
		raw, err := p.code()
		if err != nil {
			return nil, err
		}

		node.Raw = raw

	case p.text[p.pos] == '{' && rawBodies[key]:
		raw, err := p.braces()
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", key, node.Name, err)
		}

		node.Raw = raw

	case p.text[p.pos] == '{':
		p.pos++

		body, items, err := p.block(true)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", key, node.Name, err)
		}

		node.Body, node.Items = body, items

	case p.text[p.pos] == '=':
		p.pos++
		p.skip()

		// Code = [[ ... ]] means the same as Code [[ ... ]].
		if strings.HasPrefix(p.text[p.pos:], "[[") {
			raw, err := p.code()
			if err != nil {
				return nil, err
			}

			node.Raw = raw

			break
		}

		if p.pos < len(p.text) && p.text[p.pos] == '{' {
			p.pos++

			body, items, err := p.block(true)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}

			node.Body, node.Items = body, items

			break
		}

		value, err := p.value()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}

		node.Value = value
	}

	return node, nil
}

// value reads a word or a quoted string.
func (p *parser) value() (string, error) {
	if p.pos < len(p.text) && p.text[p.pos] == '"' {
		return p.quoted()
	}

	word := p.word()
	if word == "" {
		return "", p.errorf("expected a value")
	}

	return word, nil
}

func (p *parser) quoted() (string, error) {
	end := strings.IndexByte(p.text[p.pos+1:], '"')
	if end < 0 {
		return "", p.errorf("a string is never closed")
	}

	value := p.text[p.pos+1 : p.pos+1+end]
	p.advance(end + 2)

	return value, nil
}

func (p *parser) word() string {
	start := p.pos
	for p.pos < len(p.text) && isWordByte(p.text[p.pos]) {
		p.pos++
	}

	return p.text[start:p.pos]
}

func isWordByte(b byte) bool {
	return b == '_' || b == '.' || b == '-' || b == '/' || b == '@' ||
		'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}

// code reads a block of code between [[ and ]].
func (p *parser) code() (string, error) {
	end := strings.Index(p.text[p.pos+2:], "]]")
	if end < 0 {
		return "", p.errorf("a code block is never closed")
	}

	raw := p.text[p.pos+2 : p.pos+2+end]
	p.advance(end + 4)

	return raw, nil
}

// braces reads the text between a brace and the brace that closes it.
func (p *parser) braces() (string, error) {
	depth := 0

	for index := p.pos; index < len(p.text); index++ {
		switch p.text[index] {
		case '{':
			depth++
		case '}':
			depth--

			if depth == 0 {
				raw := p.text[p.pos+1 : index]
				p.advance(index + 1 - p.pos)

				return raw, nil
			}
		}
	}

	return "", p.errorf("a block is never closed")
}

// advance moves on by a number of bytes, counting the lines passed.
func (p *parser) advance(count int) {
	p.line += strings.Count(p.text[p.pos:p.pos+count], "\n")
	p.pos += count
}

// skip passes white space and comments: # and // to the end of the line, and
// /* */.
func (p *parser) skip() {
	for p.pos < len(p.text) {
		switch b := p.text[p.pos]; {
		case b == '\n':
			p.line++
			p.pos++
		case b == ' ' || b == '\t' || b == '\r':
			p.pos++
		case b == '#' || strings.HasPrefix(p.text[p.pos:], "//"):
			for p.pos < len(p.text) && p.text[p.pos] != '\n' {
				p.pos++
			}
		case strings.HasPrefix(p.text[p.pos:], "/*"):
			end := strings.Index(p.text[p.pos+2:], "*/")
			if end < 0 {
				p.pos = len(p.text)

				return
			}

			p.advance(end + 4)
		default:
			return
		}
	}
}
