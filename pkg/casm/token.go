package casm

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/Supercaly/coppervm/internal"
)

type tokenKind int

const (
	tokenKindNumLit tokenKind = iota
	tokenKindStringLit
	tokenKindCharLit
	tokenKindSymbol
	tokenKindPlus
	tokenKindMinus
	tokenKindAsterisk
	tokenKindSlash
	tokenKindPercent
	tokenKindComma
	tokenKindOpenParen
	tokenKindCloseParen
	tokenKindOpenBracket
	tokenKindCloseBracket
	tokenKindNewLine
	tokenKindColon
)

type token struct {
	kind     tokenKind
	text     string
	location FileLocation
}

type tokens []token

// Returns the first Token of the Tokens list.
func (t tokens) First() token {
	if len(t) == 0 {
		panic("trying to access the elements of an empty tokens list")
	}
	return t[0]
}

// Returns true if the Tokens list if empty, false otherwise.
func (t tokens) Empty() bool {
	return len(t) == 0
}

// Removes and returns the first element of the Tokens list.
func (t *tokens) Pop() (out token) {
	if len(*t) == 0 {
		panic("trying to pop the elements of an empty tokens list")
	}
	out = (*t)[0]
	*t = (*t)[1:]
	return out
}

// This method will panic if list of tokens is empty or the next
// token is not of given type.
func (t *tokens) expectTokenKind(kind tokenKind) {
	if t.Empty() {
		panic(fmt.Sprintf("expecting token '%s' but list is empty", kind))
	}
	if t.First().kind != kind {
		panic(fmt.Sprintf("%s: expecting token '%s' but got '%s'", t.First().location, kind, t.First().kind))
	}
}

// This method will panic with custom message if list of tokens is
// empty or the next token is not of given type.
func (t *tokens) expectTokenKindMsg(kind tokenKind, msg string) {
	if t.Empty() {
		panic(fmt.Sprintf("expecting token '%s' but list is empty", kind))
	}
	if t.First().kind != kind {
		panic(fmt.Sprintf("%s: %s", t.First().location, msg))
	}
}

// Tokenize a source string.
// Returns a list of tokens from a string.
// This method will panic when something went wrong.
func tokenize(source string, filePath string) (out tokens) {
	location := FileLocation{FileName: filePath}

	// Tokenize the whole source string
	for len(source) != 0 {
		switch source[0] {
		case ' ':
			source = source[1:]
			location.Col++
		case ';':
			var comment string
			comment, source = internal.SplitByDelim(source, '\n')
			location.Col += len(comment)
		case '\n':
			source = source[1:]
			out = append(out, token{kind: tokenKindNewLine, location: location})
			location.Row++
			location.Col = 0
		case '+':
			source = source[1:]
			out = append(out, token{kind: tokenKindPlus, location: location})
			location.Col++
		case '-':
			source = source[1:]
			out = append(out, token{kind: tokenKindMinus, location: location})
			location.Col++
		case '*':
			source = source[1:]
			out = append(out, token{kind: tokenKindAsterisk, location: location})
			location.Col++
		case '/':
			source = source[1:]
			out = append(out, token{kind: tokenKindSlash, location: location})
			location.Col++
		case '%':
			source = source[1:]
			out = append(out, token{kind: tokenKindPercent, location: location})
			location.Col++
		case ',':
			source = source[1:]
			out = append(out, token{kind: tokenKindComma, location: location})
			location.Col++
		case ':':
			source = source[1:]
			out = append(out, token{kind: tokenKindColon, location: location})
			location.Col++
		case '(':
			source = source[1:]
			out = append(out, token{kind: tokenKindOpenParen, location: location})
			location.Col++
		case ')':
			source = source[1:]
			out = append(out, token{kind: tokenKindCloseParen, location: location})
			location.Col++
		case '[':
			source = source[1:]
			out = append(out, token{kind: tokenKindOpenBracket, location: location})
			location.Col++
		case ']':
			source = source[1:]
			out = append(out, token{kind: tokenKindCloseBracket, location: location})
			location.Col++
		case '"':
			source = source[1:]
			if strings.Contains(source, "\"") {
				str, rest := internal.SplitByDelim(source, '"')
				source = rest[1:]
				unquotedStr, err := strconv.Unquote(`"` + str + `"`)
				if err != nil {
					panic(fmt.Sprintf("%s: error tokenizing literal string '%s'", location, str))
				}
				out = append(out, token{
					kind:     tokenKindStringLit,
					text:     unquotedStr,
					location: location,
				})
				// TODO: Location in not incremented correctly if there's a new line in the string
				location.Col += len(unquotedStr) + 2
			} else {
				panic(fmt.Sprintf("%s: could not find closing \"", location))
			}
		case '\'':
			source = source[1:]
			if strings.Contains(source, "'") {
				char, rest := internal.SplitByDelim(source, '\'')
				source = rest[1:]
				out = append(out, token{
					kind:     tokenKindCharLit,
					text:     char,
					location: location,
				})
				// TODO: Location in not incremented correctly if there's a new line in the char
				location.Col += len(char) + 2
			} else {
				panic(fmt.Sprintf("%s: could not find closing '", location))
			}
		default:
			if isDigit(rune(source[0])) {
				// Tokenize a number
				number, rest := internal.SplitWhile(source, isNumber)
				source = rest
				out = append(out, token{
					kind:     tokenKindNumLit,
					text:     number,
					location: location,
				})
				location.Col += len(number)
			} else if isAlpha(rune(source[0])) {
				// Tokenize a symbol
				symbol, rest := internal.SplitWhile(source, isAlpha)
				source = rest
				out = append(out, token{
					kind:     tokenKindSymbol,
					text:     symbol,
					location: location,
				})
				location.Col += len(symbol)
			} else {
				panic(fmt.Sprintf("%s: unknown token starting with '%s'", location, string(source[0])))
			}
		}
	}

	// Remove duplicate consecutive new lines
	var newOut []token
	var lastToken token
	for _, t := range out {
		if t.kind != tokenKindNewLine || lastToken.kind != tokenKindNewLine {
			newOut = append(newOut, t)
		}
		lastToken = t
	}
	out = newOut

	return out
}

func isNumber(r rune) bool {
	return isDigit(r) || isHex(r)
}

func isHex(r rune) bool {
	return unicode.In(r, unicode.Hex_Digit) || r == 'x' || r == 'X'
}

func isAlpha(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_'
}

func isDigit(r rune) bool {
	return unicode.IsNumber(r) || r == '.'
}

func (kind tokenKind) String() string {
	return [...]string{
		"TokenKindNumLit",
		"TokenKindStringLit",
		"TokenKindCharLit",
		"TokenKindSymbol",
		"TokenKindPlus",
		"TokenKindMinus",
		"TokenKindAsterisk",
		"TokenKindSlash",
		"TokenKindPercent",
		"TokenKindComma",
		"TokenKindOpenParen",
		"TokenKindCloseParen",
		"TokenKindOpenBracket",
		"TokenKindCloseBracket",
		"TokenKindNewLine",
		"TokenKindColon",
	}[kind]
}
