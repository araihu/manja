# Product

## Register

product

## Users

Manja serves API maintainers who publish and manage OpenAPI documentation, and API readers who need to navigate published docs quickly. Maintainers work in source, credential, sync, publishing, theme, and SEO flows. Readers arrive at public documentation to answer concrete questions about operations, schemas, versions, and source freshness.

## Product Purpose

Manja is a hosted OpenAPI renderer and publisher built with Goshtoso. It connects to spec sources, indexes revisions, stores metadata and files behind ports, and renders intentional public publications. Success means a maintainer can publish a selected version deliberately, and a reader can find the right operation or schema through fast search without leaving a Goshtoso-native, read-only docs surface.

## Brand Personality

Precise. Manja should feel like a disciplined documentation workbench: exact in its labels, calm in its layout, and trustworthy in how it presents published API state. It should avoid spectacle and make technical truth easy to locate.

## Anti-references

Manja should not look like a marketing landing page, a generic embedded OpenAPI renderer, a decorative SaaS dashboard, or a try-it console. Avoid hero sections, hero metrics, purple-blue gradients, glassmorphism, arbitrary Tailwind Typography prose, duplicated search/content anchors, and any UI that implies the server proxies upstream API calls in v1.

## Design Principles

- Search is the primary route through the product.
- Use Goshtoso before inventing Manja-only UI.
- Public publishing is intentional and visibly distinct from private management state.
- Preserve last-known-good confidence when sources or parsing fail.
- Keep API-first REST work narrow and avoid parallel surfaces unless integrations need them.

## Accessibility & Inclusion

Keyboard navigation is a first-class requirement, especially global Ctrl+K/Cmd+K search and visible focus targets. Public docs should keep semantic sections, stable anchors, sufficient contrast through Goshtoso tokens, reduced decorative motion, and safe readable Markdown. Default target is WCAG AA-minded behavior unless a future requirement raises the bar.
