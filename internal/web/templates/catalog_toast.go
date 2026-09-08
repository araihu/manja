package templates

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/a-h/templ"
	"github.com/araihu/goshtoso/components/toast"
)

// catalogStableToast keeps Goshtoso v0.2.6's process-global generated toast ID
// out of hash-verifiable static pages. Goshtoso's next API supports a caller-
// supplied ID; this adapter can disappear when Manja upgrades to that release.
func catalogStableToast(id string, cfg toast.Config) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, writer io.Writer) error {
		var rendered bytes.Buffer
		if err := toast.Toast(cfg).Render(ctx, &rendered); err != nil {
			return err
		}

		markup := rendered.Bytes()
		prefix := []byte(`id="server-toast-`)
		start := bytes.Index(markup, prefix)
		if start < 0 {
			return fmt.Errorf("goshtoso toast generated no server toast ID")
		}
		valueStart := start + len(`id="`)
		valueEndOffset := bytes.IndexByte(markup[valueStart:], '"')
		if valueEndOffset < 0 {
			return fmt.Errorf("goshtoso toast generated an unterminated server toast ID")
		}
		generatedID := markup[valueStart : valueStart+valueEndOffset]
		_, err := writer.Write(bytes.ReplaceAll(markup, generatedID, []byte(id)))
		return err
	})
}
