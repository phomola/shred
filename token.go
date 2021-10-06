package shred

import (
	"fmt"
	"io"
	"strings"
	"text/scanner"
)

// Kind is a token's type.
type Kind byte

const (
	KindIdent Kind = iota
	KindInt
	KindFloat
	KindString
	KindRawString
	KindChar
	KindEOF
	KindOther
	KindMatch
)

// Token is a text token.
type Token interface {
	fmt.Stringer
	Text() string
	Kind() Kind
	IsEOF() bool
	IsIdent() bool
	IsInt() bool
	IsFloat() bool
	IsString() bool
	IsRawString() bool
	IsChar() bool
	Line() int
	Column() int
}

func isQuoted(t Token) bool { return t.IsString() || t.IsRawString() || t.IsChar() }

type goToken struct {
	tok  rune
	text string
	pos  scanner.Position
}

func (t *goToken) String() string {
	return fmt.Sprintf("%s[%s:%d:%d]", scanner.TokenString(t.tok), t.Text(), t.Line(), t.Column())
}

func (t *goToken) Text() string {
	if isQuoted(t) {
		return t.text[1 : len(t.text)-1]
	}
	return t.text
}

func (t *goToken) Kind() Kind {
	switch {
	case t.IsIdent():
		return KindIdent
	case t.IsInt():
		return KindInt
	case t.IsFloat():
		return KindFloat
	case t.IsString():
		return KindString
	case t.IsRawString():
		return KindRawString
	case t.IsChar():
		return KindChar
	case t.IsEOF():
		return KindEOF
	default:
		return KindOther
	}
}

func (t *goToken) IsEOF() bool { return t.tok == scanner.EOF }

func (t *goToken) IsIdent() bool { return t.tok == scanner.Ident }

func (t *goToken) IsInt() bool { return t.tok == scanner.Int }

func (t *goToken) IsFloat() bool { return t.tok == scanner.Float }

func (t *goToken) IsString() bool { return t.tok == scanner.String }

func (t *goToken) IsRawString() bool { return t.tok == scanner.RawString }

func (t *goToken) IsChar() bool { return t.tok == scanner.Char }

func (t *goToken) Line() int { return t.pos.Line }

func (t *goToken) Column() int { return t.pos.Column }

// TokeniseString tokenises a string.
func TokeniseString(s string) []Token {
	return Tokenise(strings.NewReader(s))
}

// Tokenise tokenises the contents of a reader.
func Tokenise(r io.Reader) []Token {
	var tokens []Token
	var s scanner.Scanner
	s.Init(r)
	for {
		tok := s.Scan()
		tokens = append(tokens, &goToken{tok, s.TokenText(), s.Position})
		if tok == scanner.EOF {
			break
		}
	}
	return tokens
}
