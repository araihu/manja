//go:build !manja_runtime

package config

import (
	"github.com/araihu/manja/domain"
	sourceadapter "github.com/araihu/manja/internal/adapters/source"
	"github.com/araihu/manja/renderer"
)

// Sources materializes configured source adapters. Runtime-only builds exclude
// this file and therefore cannot acquire or refresh source content.
func (file RendererFile) Sources() []renderer.CatalogSource {
	result := make([]renderer.CatalogSource, len(file.Catalogs))
	for index, configured := range file.Catalogs {
		manifest := sourceadapter.CatalogManifest{
			ID: configured.ID, Title: configured.Title,
			Branding: domain.DocsBranding{
				DisplayName: configured.Branding.DisplayName,
				Logo: domain.DocsBrandingLogo{
					Src: configured.Branding.Logo.Src, Alt: configured.Branding.Logo.Alt,
					HomeURL: configured.Branding.Logo.HomeURL,
				},
				Favicon: configured.Branding.Favicon,
			},
			DefaultDocumentKey: configured.DefaultDocumentKey,
			ProfileID:          domain.CompatibilityProfileID(configured.ProfileID),
			Includes:           append([]string(nil), configured.Source.Include...),
			DocumentKeys:       make([]sourceadapter.CatalogDocumentKey, len(configured.Source.Documents)),
		}
		for documentIndex, document := range configured.Source.Documents {
			manifest.DocumentKeys[documentIndex] = sourceadapter.CatalogDocumentKey{SourcePath: document.Path, Key: document.Key}
		}
		switch configured.Source.Kind {
		case RendererSourceFiles:
			result[index] = sourceadapter.FileCatalogSource{Root: file.resolve(configured.Source.Root), Manifest: manifest}
		case RendererSourceGit:
			result[index] = sourceadapter.GitCatalogSource{
				Repository: configured.Source.Repository, Ref: configured.Source.Ref, Manifest: manifest,
			}
		}
	}
	return result
}
