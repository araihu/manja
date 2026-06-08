# Manja Goshtoso Brand Theme Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Manja-branded Goshtoso theme that is the default on Manja public docs while retaining the built-in Goshtoso themes in the picker.

**Architecture:** Manja already loads Goshtoso CSS before `internal/web/static/manja.css`, so the local stylesheet can extend the theme system with a `[data-theme=manja]` override. The public docs layout keeps using semantic Goshtoso classes; only the default theme string and picker options change.

**Tech Stack:** Go, templ, Goshtoso theme tokens, Alpine.js state attributes, Go unit tests.

---

## File Structure

- Modify `internal/web/public_test.go`: add failing assertions for Manja default theme markup and local theme CSS tokens.
- Modify `internal/web/static/manja.css`: add the local `[data-theme=manja]` token override.
- Modify `internal/web/templates/layout.templ`: change the default `data-theme` and Alpine fallback from `goshtoso` to `manja`.
- Modify `internal/web/templates/public.templ`: add `Manja` as the selected first theme option and keep `Goshtoso` plus all built-in themes.
- Regenerate `internal/web/templates/layout_templ.go` and `internal/web/templates/public_templ.go` with templ.

---

### Task 1: Lock Expected Theme Behavior With Tests

**Files:**
- Modify: `internal/web/public_test.go`

- [x] **Step 1: Update the render test to expect Manja as the default**

In `TestPublicDocsRenderSearchAndOperations`, replace the theme picker assertion block with:

```go
for _, want := range []string{
	`data-theme="manja"`,
	`id="manja-theme"`,
	`name="theme"`,
	`manja-theme-trigger`,
	`theme: localStorage.getItem('theme') || 'manja'`,
	`localStorage.getItem('theme') || 'manja'`,
	`theme = opt.value`,
} {
	if !strings.Contains(body, want) {
		t.Fatalf("header theme picker or default theme missing %q:\n%s", want, body)
	}
}
```

Then replace the theme option loop with:

```go
for _, theme := range []string{
	`value:&#39;manja&#39;`,
	`value:&#39;goshtoso&#39;`,
	`value:&#39;arctic&#39;`,
	`value:&#39;minimal&#39;`,
	`value:&#39;modern&#39;`,
	`value:&#39;high-contrast&#39;`,
	`value:&#39;neo-brutalism&#39;`,
	`value:&#39;halloween&#39;`,
	`value:&#39;zombie&#39;`,
	`value:&#39;pastel&#39;`,
	`value:&#39;90s&#39;`,
	`value:&#39;christmas&#39;`,
	`value:&#39;prototype&#39;`,
	`value:&#39;news&#39;`,
	`value:&#39;industrial&#39;`,
	`value:&#39;dracula&#39;`,
} {
	if !strings.Contains(body, theme) {
		t.Fatalf("theme picker missing theme option %q:\n%s", theme, body)
	}
}
if !regexp.MustCompile(`value:&#39;manja&#39;,\s*label:&#39;Manja&#39;,\s*selected:true`).MatchString(body) {
	t.Fatalf("Manja theme option should be selected by default:\n%s", body)
}
if strings.Contains(body, `data-theme="goshtoso"`) || strings.Contains(body, `|| 'goshtoso'`) {
	t.Fatalf("public docs should default to the Manja theme, not Goshtoso:\n%s", body)
}
```

- [x] **Step 2: Add a focused CSS token test**

Add this test after `TestPublicDocsRequestSampleHighlightCSSUsesThemeTokens`:

```go
func TestPublicDocsManjaThemeCSSDefinesBrandTokens(t *testing.T) {
	css, err := os.ReadFile("static/manja.css")
	if err != nil {
		t.Fatal(err)
	}
	body := string(css)
	for _, want := range []string{
		`[data-theme=manja]`,
		`--color-surface: #f7f4ec;`,
		`--color-surface-alt: #fffdf8;`,
		`--color-primary: #0d8f73;`,
		`--color-secondary: #18d6a7;`,
		`--color-surface-dark: #101513;`,
		`--color-primary-dark: #68f0c8;`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("Manja theme CSS missing %q:\n%s", want, body)
		}
	}
}
```

- [x] **Step 3: Run focused tests and confirm RED**

Run:

```bash
go test ./internal/web -run 'TestPublicDocsRenderSearchAndOperations|TestPublicDocsManjaThemeCSSDefinesBrandTokens' -count=1
```

Expected: FAIL. The failures should mention missing `data-theme="manja"` or missing `[data-theme=manja]`.

---

### Task 2: Add The Local Manja Theme Tokens

**Files:**
- Modify: `internal/web/static/manja.css`

- [x] **Step 1: Add the theme override at the top of `manja.css`**

Insert before `.manja-markdown`:

```css
@layer base {
  [data-theme=manja] {
    --font-body: 'Inter', sans-serif;
    --font-title: 'Poppins', sans-serif;
    --color-surface: #f7f4ec;
    --color-surface-alt: #fffdf8;
    --color-on-surface: #101920;
    --color-on-surface-strong: #0b1116;
    --color-on-surface-muted: #626b64;
    --color-primary: #0d8f73;
    --color-on-primary: #fffdf8;
    --color-secondary: #18d6a7;
    --color-on-secondary: #07120f;
    --color-outline: rgba(16, 25, 32, 0.16);
    --color-outline-strong: rgba(16, 25, 32, 0.28);
    --color-surface-dark: #101513;
    --color-surface-dark-alt: #151b1a;
    --color-on-surface-dark: #f7f4ec;
    --color-on-surface-dark-strong: #fffdf8;
    --color-on-surface-dark-muted: #aeb8b0;
    --color-primary-dark: #68f0c8;
    --color-on-primary-dark: #07120f;
    --color-secondary-dark: #18d6a7;
    --color-on-secondary-dark: #07120f;
    --color-outline-dark: rgba(247, 244, 236, 0.14);
    --color-outline-dark-strong: rgba(247, 244, 236, 0.26);
    --color-info: #326ce5;
    --color-on-info: #fffdf8;
    --color-success: #0d8f73;
    --color-on-success: #fffdf8;
    --color-warning: #bf7a10;
    --color-on-warning: #07120f;
    --color-danger: #b01f3a;
    --color-on-danger: #fffdf8;
    --radius-radius: var(--radius-lg);
  }
}
```

- [x] **Step 2: Run focused CSS test and confirm partial GREEN**

Run:

```bash
go test ./internal/web -run TestPublicDocsManjaThemeCSSDefinesBrandTokens -count=1
```

Expected: PASS.

---

### Task 3: Make Manja The Default Theme In The Docs Shell

**Files:**
- Modify: `internal/web/templates/layout.templ`
- Modify: `internal/web/templates/public.templ`
- Regenerate: `internal/web/templates/layout_templ.go`
- Regenerate: `internal/web/templates/public_templ.go`

- [x] **Step 1: Change the layout default theme**

In `internal/web/templates/layout.templ`, change all three default theme locations:

```templ
data-theme="manja"
```

```js
theme: localStorage.getItem('theme') || 'manja',
```

```js
document.documentElement.setAttribute('data-theme', localStorage.getItem('theme') || 'manja');
```

- [x] **Step 2: Add Manja to the picker and keep Goshtoso built in**

In `themeOptions()` in `internal/web/templates/public.templ`, make the first two options:

```go
{Value: "manja", Label: "Manja", Selected: true},
{Value: "goshtoso", Label: "Goshtoso"},
```

Leave the rest of the built-in theme options in place.

- [x] **Step 3: Regenerate templ output**

Run:

```bash
go run github.com/a-h/templ/cmd/templ generate
```

Expected: generated output updates `layout_templ.go` and `public_templ.go`.

- [x] **Step 4: Run focused theme tests and confirm GREEN**

Run:

```bash
go test ./internal/web -run 'TestPublicDocsRenderSearchAndOperations|TestPublicDocsManjaThemeCSSDefinesBrandTokens' -count=1
```

Expected: PASS.

---

### Task 4: Verify Dropdown And Full Test Surface

**Files:**
- Test: `internal/web/e2e/public_docs_test.go`
- Test: all Go packages

- [x] **Step 1: Run the existing theme dropdown E2E regression**

Run:

```bash
go test ./internal/web/e2e -run TestPublicDocsThemeSelectDropdownOverlaysContent -count=1
```

Expected: PASS. This proves the picker remains visible and unclipped.

- [x] **Step 2: Run all Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [x] **Step 3: Inspect the final diff**

Run:

```bash
git diff --check
git diff --stat
git diff -- internal/web/static/manja.css internal/web/templates/layout.templ internal/web/templates/public.templ internal/web/public_test.go
```

Expected: no whitespace errors, and the diff is limited to theme tokens, default theme strings, picker options, tests, generated templ files, and this plan.

- [x] **Step 4: Commit the implementation**

Run:

```bash
git add internal/web/public_test.go internal/web/static/manja.css internal/web/templates/layout.templ internal/web/templates/public.templ internal/web/templates/layout_templ.go internal/web/templates/public_templ.go docs/superpowers/plans/2026-06-08-manja-goshtoso-brand-theme.md
git commit -m "feat: add Manja Goshtoso brand theme"
```
