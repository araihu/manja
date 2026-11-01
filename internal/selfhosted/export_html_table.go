//go:build !manja_runtime

package selfhosted

import (
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Preserve the Goshtoso table, replacing its server request with a local sort
// control. Only Manja's complete, unpaginated document inventory is eligible.
func prepareStaticDocumentSort(node *html.Node) {
	if node.DataAtom != atom.Th || !hasHTMLAttribute(node, "data-table-sort-request", "") {
		return
	}
	var table, head *html.Node
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.DataAtom == atom.Thead {
			head = parent
		}
		if parent.DataAtom == atom.Table {
			table = parent
			break
		}
	}
	if table == nil || !hasHTMLAttribute(table, "id", "catalog-documents-table") {
		return
	}
	column := 0
	for sibling := node.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
		if sibling.Type == html.ElementNode {
			column++
		}
	}
	keys := []string{"name", "version", "operations", "schemas"}
	if column >= len(keys) {
		return
	}
	key := keys[column]
	setHTMLAttribute(node, "data-manja-static-sort", key)
	setHTMLAttribute(node, "aria-sort", "none")
	button := &html.Node{Type: html.ElementNode, Data: "button", DataAtom: atom.Button, Attr: []html.Attribute{{Key: "type", Val: "button"}, {Key: "class", Val: "flex min-h-11 w-full items-center text-left focus-visible:outline-2 focus-visible:outline-offset-2"}}}
	// Keep the component label and replace its server-owned sort icon with state
	// that the browser can update without rendering another table header.
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		node.RemoveChild(child)
		button.AppendChild(child)
		child = next
	}
	var replaceIcon func(*html.Node)
	replaceIcon = func(parent *html.Node) {
		for child := parent.FirstChild; child != nil; {
			next := child.NextSibling
			if child.DataAtom == atom.Svg {
				indicator := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span, Attr: []html.Attribute{{Key: "data-manja-static-sort-direction", Val: ""}, {Key: "aria-hidden", Val: "true"}}}
				label := "↕"
				if head != nil && hasHTMLAttribute(head, "data-table-sort-by", key) {
					if hasHTMLAttribute(head, "data-table-sort-dir", "desc") {
						label = "↓"
						setHTMLAttribute(node, "aria-sort", "descending")
					} else {
						label = "↑"
						setHTMLAttribute(node, "aria-sort", "ascending")
					}
				}
				indicator.AppendChild(&html.Node{Type: html.TextNode, Data: label})
				parent.InsertBefore(indicator, child)
				parent.RemoveChild(child)
			} else {
				replaceIcon(child)
			}
			child = next
		}
	}
	replaceIcon(button)
	node.AppendChild(button)
}

func runtimeRequestAttribute(name string) bool {
	switch name {
	case "hx-get", "hx-post", "hx-put", "hx-patch", "hx-delete", "hx-boost",
		"data-hx-get", "data-hx-post", "data-hx-put", "data-hx-patch", "data-hx-delete", "data-hx-boost":
		return true
	}
	return false
}
