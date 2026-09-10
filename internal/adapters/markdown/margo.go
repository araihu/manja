package markdown

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/a-h/templ"
	"github.com/araihu/manja/application/port"
	"github.com/araihu/margo"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var descriptionCompiler = margo.New()

// RenderDescription renders a host-owned fragment, never a Margo page shell.
// The caller supplies the identity of the containing operation so headings and
// footnotes cannot collide when independently generated fragments are composed.
func RenderDescription(ctx context.Context, input, scope string) (port.MarkdownResult, error) {
	if err := ctx.Err(); err != nil {
		return port.MarkdownResult{}, err
	}
	document, err := descriptionCompiler.Compile(ctx, margo.Source{Name: "description.md", Content: []byte(input)})
	if err != nil {
		return port.MarkdownResult{}, err
	}
	rendered, err := descriptionCompiler.Render(ctx, document)
	if err != nil {
		return port.MarkdownResult{}, err
	}
	result, err := margo.RenderHTML(rendered)
	if err != nil {
		return port.MarkdownResult{}, err
	}
	// Core styles are installed once per shell. Copy controls are adapted to
	// the existing host runtime below; prose tables intentionally stay static.
	for _, requirement := range result.Requirements().List() {
		switch requirement.ID {
		case "goshtoso.styles", "margo.document.styles", "margo.table-sort", "margo.code-copy":
		default:
			return port.MarkdownResult{}, fmt.Errorf("unsupported description dependency %q", requirement.ID)
		}
	}
	var content bytes.Buffer
	if err := result.Fragment().Render(ctx, &content); err != nil {
		return port.MarkdownResult{}, err
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(&content, root)
	if err != nil {
		return port.MarkdownResult{}, err
	}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	prefix := fmt.Sprintf("manja-md-%x-", sha256.Sum256([]byte(scope)))
	ids := map[string]string{}
	unsupportedImage := false
	codeOrdinal := 0
	walkDescription(root, func(node *html.Node) {
		if node.DataAtom == atom.Img {
			unsupportedImage = true
		}
		for _, attr := range node.Attr {
			if attr.Key != "data-code-block" {
				continue
			}
			codeOrdinal++
			// Repeated identical code blocks can share a Goshtoso content ID.
			// Give each occurrence its own target before applying operation scope.
			target := fmt.Sprintf("markdown-code-%d", codeOrdinal)
			walkDescription(node, func(child *html.Node) {
				for i := range child.Attr {
					switch child.Attr[i].Key {
					case "id", "data-code-block-target", "aria-controls":
						child.Attr[i].Val = target
					}
				}
			})
		}
		// Manja already owns Goshtoso's delegated copy runtime. Restore its
		// semantic hooks instead of loading Margo's standalone copy runtime,
		// which only initializes blocks present at page load.
		isCopy := false
		for i := range node.Attr {
			switch node.Attr[i].Key {
			case "data-margo-code-copy-button":
				node.Attr[i].Key = "data-code-block-copy"
				isCopy = true
			case "data-margo-code-copy-label":
				node.Attr[i].Key = "data-code-block-copy-status"
			case "data-margo-table-sort":
				// Operation prose tables are static in this first integration.
				node.Attr[i].Key = "data-manja-markdown-table"
			}
		}
		if isCopy {
			attrs := node.Attr[:0]
			for _, attr := range node.Attr {
				if attr.Key != "hidden" {
					attrs = append(attrs, attr)
				}
			}
			node.Attr = attrs
		}
		for i := range node.Attr {
			if node.Attr[i].Key == "id" {
				old := node.Attr[i].Val
				ids[old] = prefix + old
				node.Attr[i].Val = ids[old]
			}
		}
	})
	if unsupportedImage {
		return port.MarkdownResult{}, fmt.Errorf("description images need an explicit asset policy")
	}
	walkDescription(root, func(node *html.Node) {
		for i := range node.Attr {
			attr := &node.Attr[i]
			if attr.Key == "aria-labelledby" || attr.Key == "aria-describedby" || attr.Key == "aria-controls" || attr.Key == "data-code-block-target" {
				parts := strings.Fields(attr.Val)
				for j, part := range parts {
					if replacement, ok := ids[part]; ok {
						parts[j] = replacement
					}
				}
				attr.Val = strings.Join(parts, " ")
			}
			if node.DataAtom != atom.A || attr.Key != "href" {
				continue
			}
			href := attr.Val
			if strings.HasPrefix(href, "#") {
				if replacement, ok := ids[strings.TrimPrefix(href, "#")]; ok {
					attr.Val = "#" + replacement
					continue
				}
			}
			parsed, err := url.Parse(href)
			if err == nil && ((parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil) || parsed.Scheme == "mailto") {
				continue
			}
			// No upstream documentation base is available yet. Preserve the
			// destination visibly, but do not turn it into a local export route.
			attr.Key, attr.Val = "data-manja-unresolved-link", href
			node.Data, node.DataAtom = "span", atom.Span
			if href != "" {
				node.AppendChild(&html.Node{Type: html.TextNode, Data: " (" + href + ")"})
			}
		}
	})
	content.Reset()
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&content, child); err != nil {
			return port.MarkdownResult{}, err
		}
	}
	return port.MarkdownResult{HTML: content.String(), Plain: result.PlainText()}, nil
}

func walkDescription(node *html.Node, visit func(*html.Node)) {
	visit(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkDescription(child, visit)
	}
}

// DescriptionComponent keeps unsupported author content readable without
// broadening Margo's host policy. The marker makes fallback testable/observable.
func DescriptionComponent(input, scope string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		result, err := RenderDescription(ctx, input, scope)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			_, err = w.Write([]byte(`<p data-manja-markdown-fallback="plain-text" class="whitespace-pre-wrap">` + templ.EscapeString(input) + `</p>`))
			return err
		}
		_, err = w.Write([]byte(result.HTML))
		return err
	})
}
