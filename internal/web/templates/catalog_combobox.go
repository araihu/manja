package templates

import (
	"github.com/a-h/templ"
	"github.com/araihu/goshtoso/components/combobox"
)

const (
	CatalogDocumentComboboxID             = "catalog-document"
	CatalogDocumentComboboxPathPrefix     = "/_manja/catalog/document-combobox/"
	CatalogDocumentComboboxOptionsPath    = CatalogDocumentComboboxPathPrefix + "options"
	CatalogDocumentComboboxTogglePath     = CatalogDocumentComboboxPathPrefix + "toggle"
	CatalogDocumentComboboxClearPath      = CatalogDocumentComboboxPathPrefix + "clear"
	CatalogDocumentComboboxMountFieldName = "catalog-mount"
)

func CatalogDocumentComboboxConfig() combobox.Config {
	return combobox.Config{
		ID:              CatalogDocumentComboboxID,
		Name:            CatalogDocumentComboboxID,
		Placeholder:     "Catalog overview",
		Mode:            combobox.ModeSingle,
		Source:          combobox.Source{LazyEndpoint: CatalogDocumentComboboxOptionsPath},
		EnableSearch:    true,
		DependsOn:       []string{CatalogDocumentComboboxMountFieldName},
		ToggleEndpoint:  CatalogDocumentComboboxTogglePath,
		OptionsEndpoint: CatalogDocumentComboboxOptionsPath,
		ClearEndpoint:   CatalogDocumentComboboxClearPath,
		RootClass:       "w-72 max-w-[40vw]",
		TriggerAttrs: templ.Attributes{
			"aria-label": "OpenAPI document",
			"hx-get":     CatalogDocumentComboboxOptionsPath,
			"hx-trigger": "click once",
			"hx-target":  "#" + CatalogDocumentComboboxID + "-options",
			"hx-swap":    "outerHTML",
			"hx-include": "[name=" + CatalogDocumentComboboxMountFieldName + "]",
		},
	}
}

func catalogDocumentComboboxState(options []CatalogDocumentOption) combobox.State {
	state := combobox.State{Options: make([]combobox.Option, 0, 1)}
	for _, option := range options {
		if !option.Selected {
			continue
		}
		state.Options = append(state.Options, combobox.Option{Value: option.Href, Label: option.Label})
		state.Selected = []string{option.Href}
		break
	}
	return state
}
