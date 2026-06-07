---
name: Manja
description: Search-first OpenAPI documentation UI built with Goshtoso.
colors:
  surface: "#f5f5f5"
  surface-alt: "#e5e5e5"
  surface-dark: "#262626"
  surface-dark-alt: "#171717"
  on-surface: "#262626"
  on-surface-muted: "#525252"
  on-surface-strong: "#0a0a0a"
  on-surface-dark: "#d4d4d4"
  on-surface-dark-muted: "#a3a3a3"
  on-surface-dark-strong: "#f5f5f5"
  primary: "#a855f7"
  primary-dark: "#c084fc"
  on-primary: "#fafafa"
  on-primary-dark: "#0a0a0a"
  outline: "#d4d4d4"
  outline-strong: "#262626"
  outline-dark: "#404040"
  outline-dark-strong: "#d4d4d4"
typography:
  display:
    fontFamily: "Poppins, Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.875rem"
    fontWeight: 700
    lineHeight: 1.2
    letterSpacing: "normal"
  headline:
    fontFamily: "Poppins, Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.25
    letterSpacing: "normal"
  title:
    fontFamily: "Poppins, Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: "normal"
  body:
    fontFamily: "Instrument Sans, Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: 1.5
    letterSpacing: "normal"
  label:
    fontFamily: "Instrument Sans, Inter, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 500
    lineHeight: 1.25
    letterSpacing: "normal"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: "normal"
rounded:
  radius: "12px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  lg: "24px"
  xl: "32px"
components:
  search-trigger:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface-muted}"
    rounded: "{rounded.radius}"
    padding: "8px 12px"
    height: "40px"
  badge-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.radius}"
    padding: "4px 8px"
  operation-section:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.radius}"
    padding: "16px"
  schema-row:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.radius}"
    padding: "0 0 12px 0"
---

# Design System: Manja

## 1. Overview

**Creative North Star: "The Indexed Workbench"**

Manja is a task-first documentation surface for people who need to inspect an API quickly, not be sold a product. The interface should feel like a precise workbench: quiet surfaces, high signal hierarchy, predictable anchors, and search that becomes the main path through the document.

The system inherits Goshtoso's semantic token contract and should continue to compose Goshtoso primitives before inventing Manja-only UI. The current public docs route uses a restrained light surface by default, dense operation sections, method badges, schema rows, and a command-palette search trigger in the header.

It explicitly rejects generic OpenAPI renderer chrome, marketing heroes, decorative dashboards, and anything that fights Goshtoso tokens. Public docs are read-only in v1; there is no interactive API console and no server-side proxy for upstream API requests.

**Key Characteristics:**
- Search-first navigation with Ctrl+K or Cmd+K.
- Restrained product UI, centered on operations, schemas, and version context.
- Semantic Goshtoso colors: `surface`, `outline`, `primary`, `on-surface`, and dark variants.
- Section anchors and search result IDs must stay distinct so keyboard search lands on visible content.
- Markdown must render inside `.manja-markdown` and map back to Goshtoso tokens.

## 2. Colors

The palette is a restrained Goshtoso semantic palette: neutral surfaces carry the page, violet primary marks actions and method badges, and outlines create structure without decorative color.

### Primary
- **Command Violet** (`#a855f7`): Used for primary badges, active focus rings, and search/action emphasis. It should stay rare; the operation method badge earns this color because method scanning is a core task.
- **Dark Command Violet** (`#c084fc`): Dark-mode equivalent for the same focused interactive roles.

### Neutral
- **Workbench Surface** (`#f5f5f5`): Default light page and operation section surface through `--color-surface`.
- **Workbench Layer** (`#e5e5e5`): Secondary light layer for search controls, hover states, or Markdown/table alternation through `--color-surface-alt`.
- **Ink Text** (`#262626`): Primary readable body text through `--color-on-surface`.
- **Muted Ink** (`#525252`): Secondary metadata such as "OpenAPI docs", tags, and schema descriptions.
- **Strong Ink** (`#0a0a0a`): Headings and high emphasis text.
- **Night Surface** (`#262626`): Default dark surface.
- **Night Layer** (`#171717`): Secondary dark layer.
- **Night Text** (`#d4d4d4`): Primary dark-mode body text.
- **Night Strong Text** (`#f5f5f5`): Dark-mode headings.
- **Outline Grey** (`#d4d4d4`): Light borders, dividers, and section outlines.
- **Night Outline** (`#404040`): Dark borders and dividers.

### Named Rules
**The Semantic Token Rule.** Manja-specific CSS must target Goshtoso variables such as `var(--color-surface)` and `var(--color-primary)`, not hard-coded palette values.

**The Accent Rarity Rule.** Primary violet belongs on method badges, focus, and true active states. Do not use it as decoration, background wash, or arbitrary section color.

## 3. Typography

**Display Font:** Poppins with Inter/system fallback via `font-title`.
**Body Font:** Instrument Sans with Inter/system fallback via Goshtoso base styles.
**Label/Mono Font:** UI monospace stack for operation paths and schema names.

**Character:** Typography is compact and work-focused. Headings use enough weight to orient the page, while operation paths use monospace to preserve API shape and make route comparison easier.

### Hierarchy
- **Display** (700, `text-3xl`, 1.2): Public spec title in the page header.
- **Headline** (700, `text-2xl`, 1.25): Major subsections such as "Schemas".
- **Title** (600, `text-lg`, 1.35): Operation route headings and dense panel titles.
- **Body** (400, `text-base`, 1.5): Summaries and prose. Keep Markdown prose to 65-75ch where long reading appears.
- **Label** (500, `text-sm` / `text-xs`, normal tracking): Eyebrows, tags, keyboard hints, and badge text.
- **Mono** (600, `font-mono`, fixed rem sizes): Paths, schema names, code-like identifiers, and generated anchors.

### Named Rules
**The Route Shape Rule.** API paths, operation identifiers, schema names, and code-like values should use monospace; explanatory prose should not.

## 4. Elevation

Manja is flat by default. Depth is conveyed through borders, tonal layers, spacing, and focus outlines rather than ambient shadows. The Goshtoso search modal may use the component library's existing shadow vocabulary, but Manja docs content should not introduce new shadow styles unless a component is actively floating above the page.

### Named Rules
**The Flat Content Rule.** Operation sections and schema rows use `border border-outline` and stable padding, not drop shadows.

**The State-Only Motion Rule.** Motion belongs to Goshtoso controls such as the command palette open/close transition. Documentation content itself should not animate on load.

## 5. Components

### Buttons
- **Shape:** Goshtoso `rounded-radius` (`12px` in the current theme).
- **Primary:** `bg-primary text-on-primary border-primary`, usually handled by Goshtoso primitives.
- **Hover / Focus:** Use component-library opacity, outline, and `focus-visible:outline-primary`; do not invent Manja-only focus treatments.
- **Secondary / Ghost:** Prefer border and text treatments from Goshtoso; keep neutral until selected or active.

### Chips
- **Style:** Method badges use Goshtoso `badge.Primary`: primary background, on-primary text, compact `px-2 py-1`.
- **State:** Badges are informative labels, not filters. Do not add hover affordance unless the badge becomes actionable.

### Cards / Containers
- **Corner Style:** `rounded-radius` only. Operation sections are the only repeated card-like content in the current public docs surface.
- **Background:** `bg-surface` in light mode, `dark:bg-surface-dark` in dark mode.
- **Shadow Strategy:** No shadow at rest.
- **Border:** `border border-outline`, with `dark:border-outline-dark`.
- **Internal Padding:** `p-4` for operation sections, `pb-3` for schema rows.

### Inputs / Fields
- **Style:** Use Goshtoso search and input primitives. The command-palette trigger is a full-width rounded border control with neutral text.
- **Focus:** `focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary` and dark equivalent.
- **Error / Disabled:** Follow Goshtoso form vocabulary; disabled states reduce opacity and keep cursor semantics.

### Navigation
- **Style:** The public docs v1 has no sidebar yet. Search is the primary navigation affordance and sits in the header.
- **Responsive:** Header stacks on mobile, then aligns title/version and search horizontally at the medium breakpoint.
- **Active Targets:** Search item DOM IDs must be prefixed separately from content anchors. Search hrefs must resolve to visible `section` targets.

### Markdown
- **Wrapper:** All rendered Markdown lives under `.manja-markdown`.
- **Color:** Use `var(--color-on-surface)` and dark variants.
- **Links:** `var(--color-primary)` with underline, dark variant on dark surfaces.
- **Headings:** Use `var(--font-title)` and strong surface text tokens.

## 6. Do's and Don'ts

### Do:
- **Do** use Goshtoso semantic classes and variables (`bg-surface`, `text-on-surface`, `border-outline`, `rounded-radius`) as the first design language.
- **Do** keep operation and schema anchors stable, visible, and distinct from search result control IDs.
- **Do** reserve `primary` for method badges, focus, selected states, and real actions.
- **Do** keep public docs immediately useful: spec title, version, search, operations, and schemas should be visible without a marketing prelude.
- **Do** use borders and spacing to create hierarchy before adding shadows.
- **Do** keep Markdown safe and token-styled through `.manja-markdown`.

### Don't:
- **Don't** create a landing-page hero for public docs.
- **Don't** add a v1 "try it" console or any UI that implies Manja proxies upstream API calls.
- **Don't** use Tailwind Typography `prose` classes or arbitrary author classes for Markdown.
- **Don't** add decorative gradients, glassmorphism, side-stripe borders, or hero-metric patterns.
- **Don't** duplicate DOM IDs between search controls and content anchors.
- **Don't** hard-code colors in Manja CSS when a Goshtoso semantic token exists.
