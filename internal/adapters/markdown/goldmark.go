package markdown

import (
	"bytes"
	"context"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	core "github.com/araihu/manja/domain"
)

type Renderer struct {
	md goldmark.Markdown
}

func NewRenderer() Renderer {
	return Renderer{md: goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)}
}

func (r Renderer) Render(ctx context.Context, input string) (core.MarkdownResult, error) {
	if err := ctx.Err(); err != nil {
		return core.MarkdownResult{}, err
	}

	sanitized := stripRawHTML(input)
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(sanitized), &buf); err != nil {
		return core.MarkdownResult{}, err
	}

	html := `<div class="manja-markdown">` + buf.String() + `</div>`
	return core.MarkdownResult{HTML: html, Plain: plainText(sanitized)}, nil
}

var rawHTML = regexp.MustCompile(`(?is)<[^>]+>`)

func stripRawHTML(input string) string {
	return rawHTML.ReplaceAllString(input, "")
}

func plainText(input string) string {
	text := stripRawHTML(input)
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "#", "")
	text = strings.ReplaceAll(text, "*", "")
	return strings.Join(strings.Fields(text), " ")
}
