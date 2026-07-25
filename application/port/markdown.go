package port

import (
	"context"

	"github.com/araihu/manja/domain"
)

type MarkdownRenderer interface {
	Render(context.Context, string) (domain.MarkdownResult, error)
}
