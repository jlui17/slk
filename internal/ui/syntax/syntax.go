package syntax

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/gammons/slk/internal/ui/styles"
)

var languageRe = regexp.MustCompile(`^[A-Za-z0-9_+#.-]+$`)

// Known reports whether language can ride on a code fence: a name the
// highlighter has a lexer for, spelled so it cannot break the fence line.
func Known(language string) bool {
	return languageRe.MatchString(language) && lexers.Get(language) != nil
}

// Highlight colors code with foreground SGR sequences only: no resets and
// no attributes, so the caller's background and per-row framing stay in
// force. Unknown languages and lexer failures return code unchanged.
func Highlight(code, language string) string {
	lexer := lexers.Get(language)
	if lexer == nil {
		return code
	}
	tokens, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return code
	}
	style := themeStyle()
	var b strings.Builder
	current := ""
	for tok := tokens(); tok != chroma.EOF; tok = tokens() {
		if fg := fgSGR(style.Get(tok.Type).Colour); fg != current {
			b.WriteString(fg)
			current = fg
		}
		b.WriteString(tok.Value)
	}
	return b.String()
}

var cache struct {
	sync.Mutex
	version int64
	style   *chroma.Style
}

func themeStyle() *chroma.Style {
	cache.Lock()
	defer cache.Unlock()
	if cache.style == nil || cache.version != styles.Version() {
		cache.style = buildThemeStyle()
		cache.version = styles.Version()
	}
	return cache.style
}

func buildThemeStyle() *chroma.Style {
	return chroma.MustNewStyle("slk", chroma.StyleEntries{
		chroma.Background:        hex(styles.TextPrimary),
		chroma.Keyword:           hex(styles.Primary),
		chroma.NameBuiltin:       hex(styles.Primary),
		chroma.NameTag:           hex(styles.Primary),
		chroma.NameDecorator:     hex(styles.Primary),
		chroma.LiteralString:     hex(styles.Accent),
		chroma.LiteralNumber:     hex(styles.Warning),
		chroma.NameConstant:      hex(styles.Warning),
		chroma.NameAttribute:     hex(styles.Warning),
		chroma.Comment:           hex(styles.TextMuted),
		chroma.GenericDeleted:    hex(styles.Error),
		chroma.GenericInserted:   hex(styles.Accent),
		chroma.GenericHeading:    hex(styles.Primary),
		chroma.GenericSubheading: hex(styles.Primary),
	})
}

func hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func fgSGR(c chroma.Colour) string {
	if !c.IsSet() {
		return ""
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.Red(), c.Green(), c.Blue())
}
