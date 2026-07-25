package port

import (
	"context"
)

type MarkdownResult struct {
	HTML  string
	Plain string
}

type MarkdownRenderer interface {
	Render(context.Context, string) (MarkdownResult, error)
}
