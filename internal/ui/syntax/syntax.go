package syntax

import (
	"image/color"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/gammons/slk/internal/ui/styles"
)

// Names and aliases only: chroma's own lookup falls back to filename-glob
// matching, which scans every lexer on a miss and would turn a first code
// line like "main.go" into a language tag.
var byName = func() map[string]chroma.Lexer {
	m := map[string]chroma.Lexer{}
	for _, l := range lexers.GlobalLexerRegistry.Lexers {
		cfg := l.Config()
		m[strings.ToLower(cfg.Name)] = l
		for _, alias := range cfg.Aliases {
			m[strings.ToLower(alias)] = l
		}
	}
	return m
}()

// Lexer resolves a language tag by lexer name or alias; nil when unknown.
func Lexer(language string) chroma.Lexer {
	return byName[strings.ToLower(language)]
}

type Span struct {
	Text  string
	Color color.Color
}

// Highlight splits code into runs colored from the active theme; the
// concatenated Text is code unchanged.
func Highlight(code string, lexer chroma.Lexer) []Span {
	tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return []Span{{Text: code, Color: styles.TextPrimary}}
	}
	var spans []Span
	n := 0
	for tok := tokens(); tok != chroma.EOF; tok = tokens() {
		spans = append(spans, Span{Text: tok.Value, Color: tokenColor(tok.Type)})
		n += len(tok.Value)
	}
	// Lexers configured with ensure_nl tokenise code plus a newline they add.
	if n > len(code) {
		last := &spans[len(spans)-1]
		last.Text = strings.TrimSuffix(last.Text, "\n")
		if last.Text == "" {
			spans = spans[:len(spans)-1]
		}
	}
	return spans
}

// Pointers, because styles.Apply reassigns the theme variables.
var palette = map[chroma.TokenType]*color.Color{
	chroma.Keyword:           &styles.Primary,
	chroma.NameBuiltin:       &styles.Primary,
	chroma.NameTag:           &styles.Primary,
	chroma.NameDecorator:     &styles.Primary,
	chroma.LiteralString:     &styles.Accent,
	chroma.LiteralNumber:     &styles.Warning,
	chroma.NameConstant:      &styles.Warning,
	chroma.NameAttribute:     &styles.Warning,
	chroma.Comment:           &styles.TextMuted,
	chroma.GenericDeleted:    &styles.Error,
	chroma.GenericInserted:   &styles.Accent,
	chroma.GenericHeading:    &styles.Primary,
	chroma.GenericSubheading: &styles.Primary,
}

func tokenColor(t chroma.TokenType) color.Color {
	for _, k := range []chroma.TokenType{t, t.SubCategory(), t.Category()} {
		if c, ok := palette[k]; ok {
			return *c
		}
	}
	return styles.TextPrimary
}
