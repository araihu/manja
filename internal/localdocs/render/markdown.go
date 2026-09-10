package render

import (
	"context"
	"github.com/a-h/templ"
	"io"
)

type descriptionRendererKey struct{}

// WithDescriptionRenderer supplies a host-owned component factory.
// The portable renderer does not import the Markdown compiler or its assets.
func WithDescriptionRenderer(ctx context.Context, renderer func(string, string) templ.Component) context.Context {
	return context.WithValue(ctx, descriptionRendererKey{}, renderer)
}

// OperationDescription is shared by static fragments and hosted operation views.
func OperationDescription(input, scope string) templ.Component {
	return Description(input, "operation-"+scope)
}

// Description renders through the host's component factory, with a plain-text
// fallback for portable renderers without a Markdown integration.
func Description(input, scope string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if renderer, ok := ctx.Value(descriptionRendererKey{}).(func(string, string) templ.Component); ok && renderer != nil {
			return renderer(input, scope).Render(ctx, w)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := io.WriteString(w, `<p class="whitespace-pre-wrap">`+templ.EscapeString(input)+`</p>`)
		return err
	})
}
