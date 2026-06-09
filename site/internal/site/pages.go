package site

import (
	"html/template"
	"io"
)

type PageKey string

const (
	Home PageKey = "home"
	Docs PageKey = "docs"
)

type page struct {
	Title       string
	Description string
	Path        string
	Body        template.HTML
}

// Render writes one of the public product-site pages.
func Render(w io.Writer, key PageKey) error {
	return pageTemplate.Execute(w, pageFor(key))
}

func pageFor(key PageKey) page {
	switch key {
	case Docs:
		return page{
			Title:       "Manja Docs",
			Description: "Setup notes for pointing Manja at an OpenAPI specification source and publishing a stable version.",
			Path:        "/docs",
			Body:        docsBody,
		}
	default:
		return page{
			Title:       "Manja",
			Description: "Source-connected OpenAPI publishing that renders stable public API documentation from the specs teams already keep in source control.",
			Path:        "/",
			Body:        homeBody,
		}
	}
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <meta name="description" content="{{.Description}}">
  <link rel="icon" href="/static/favicon.svg" type="image/svg+xml">
  <link rel="stylesheet" href="/static/site.css">
  <script>
    (function () {
      try {
        var stored = localStorage.getItem("darkMode");
        var on = stored !== null ? stored === "true" : window.matchMedia("(prefers-color-scheme: dark)").matches;
        document.documentElement.classList.toggle("dark", on);
      } catch (error) {}
    })();
  </script>
</head>
<body>
  <a class="skip-link" href="#main">Skip to content</a>
  <header class="site-header">
    <nav class="site-nav" aria-label="Main">
      <a class="brand" href="/" aria-label="Manja home">
        <img class="brand-mark" src="/static/manja-mark.svg" alt="" width="40" height="40">
        <span>Manja</span>
      </a>
      <div class="nav-actions">
        <a href="/demo" target="_blank" rel="noopener"{{if eq .Path "/demo"}} aria-current="page"{{end}}>Demo</a>
        <a href="/docs"{{if eq .Path "/docs"}} aria-current="page"{{end}}>Docs</a>
        <a href="https://github.com/araihu/manja" rel="noopener">GitHub</a>
        <button class="theme-toggle" type="button" aria-label="Toggle color mode" aria-pressed="false" data-theme-toggle>
          <svg class="theme-icon theme-icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
            <circle cx="12" cy="12" r="4"></circle>
            <path stroke-linecap="round" d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"></path>
          </svg>
          <svg class="theme-icon theme-icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
            <path stroke-linecap="round" stroke-linejoin="round" d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79Z"></path>
          </svg>
        </button>
      </div>
    </nav>
  </header>
  <main id="main">
    {{.Body}}
  </main>
  <script src="/static/theme.js"></script>
</body>
</html>`))

var homeBody = template.HTML(`
<section class="hero shell">
  <div class="hero-copy">
    <p class="eyebrow">Source-connected OpenAPI publishing</p>
    <h1>Point Manja at your spec.</h1>
    <p class="lead">Manja tracks the source, understands revisions, renders the OpenAPI reference, and keeps the public version stable. Say where the spec lives; the rest should fall into place without asking developers to change their workflow.</p>
    <div class="actions">
      <a class="button button-primary" href="/demo" target="_blank" rel="noopener">View live demo</a>
      <a class="button button-secondary" href="/docs">Read setup docs</a>
    </div>
  </div>
</section>

<section class="workflow shell" aria-label="Workflow">
  <div>
    <h2>Connect source</h2>
    <p>Use the repo, ref, credentials, and spec path developers already maintain.</p>
  </div>
  <div>
    <h2>Discover versions</h2>
    <p>Branches, tags, commits, and singleton file sources become selectable revisions.</p>
  </div>
  <div>
    <h2>Index OpenAPI</h2>
    <p>Operations, schemas, routes, snippets, and examples are derived from the spec.</p>
  </div>
  <div>
    <h2>Publish deliberately</h2>
    <p>Public readers see the chosen version, even if a later sync or parse fails.</p>
  </div>
</section>

<section class="principles">
  <div class="shell principles-grid">
    <article>
      <h2>Fits existing tools</h2>
      <p>No parallel authoring surface. The source of truth stays where teams already work.</p>
    </article>
    <article>
      <h2>Versions stay close to source</h2>
      <p>Branches, tags, commits, and singleton spec files become publication candidates without moving ownership out of source control.</p>
    </article>
  </div>
</section>
`)

var docsBody = template.HTML(`
<section class="page-hero shell">
  <p class="eyebrow">Docs</p>
  <h1>Setup docs</h1>
  <p class="lead">Manja starts from the spec file and source revision developers already maintain. Connect the source, choose the revision to publish, and keep the public docs pinned to a stable known-good version.</p>
</section>

<section class="docs-layout shell">
  <aside class="docs-nav" aria-label="Docs sections">
    <a href="#run-locally">Run locally</a>
    <a href="#run-with-docker">Run with Docker</a>
    <a href="#source">Point at source</a>
    <a href="#versions">Handle versions</a>
    <a href="#publication">Publish deliberately</a>
  </aside>

  <div class="docs-content">
    <section id="run-locally">
      <h2>Run locally</h2>
      <p>The current vertical slice renders a read-only OpenAPI reference from a spec file and stores publication metadata in a local data directory.</p>
      <div class="code-panel">
        <div class="code-head"><span>Command</span><span>shell</span></div>
        <pre><code>go run ./cmd/manja \
  -spec internal/adapters/openapi/testdata/github-v3-rest.json \
  -data-dir .manja/data</code></pre>
      </div>
    </section>

    <section id="run-with-docker">
      <h2>Run with Docker</h2>
      <p>Published images are available from GitHub Container Registry as <code>ghcr.io/araihu/manja</code>. The <code>main</code> tag follows the latest successful build from <code>main</code>; release builds also publish semver tags.</p>
      <div class="code-panel">
        <div class="code-head"><span>Command</span><span>shell</span></div>
        <pre><code>docker run --rm \
  -p 8080:8080 \
  -v manja-data:/var/lib/manja \
  ghcr.io/araihu/manja:main</code></pre>
      </div>
      <p>The image starts with the bundled GitHub REST API fixture. To render your own spec, mount it into the container and pass the same Manja flags used by the local binary.</p>
      <div class="code-panel">
        <div class="code-head"><span>Custom spec</span><span>shell</span></div>
        <pre><code>docker run --rm \
  -p 8080:8080 \
  -v "$PWD/openapi.yaml:/spec/openapi.yaml:ro" \
  -v manja-data:/var/lib/manja \
  ghcr.io/araihu/manja:main \
  -addr :8080 \
  -spec /spec/openapi.yaml \
  -data-dir /var/lib/manja</code></pre>
      </div>
    </section>

    <section id="source">
      <h2>Point at source</h2>
      <p>Keep the OpenAPI file in the repository that already owns it. Manja needs the repository, credentials when required, a spec path, and the revision or singleton file source to index.</p>
      <dl class="field-list wide">
        <div><dt>Repository</dt><dd>Remote or local source that owns the spec.</dd></div>
        <div><dt>Spec path</dt><dd>Path such as <code>docs/openapi.yaml</code> or <code>api/openapi.yaml</code>.</dd></div>
        <div><dt>Revision</dt><dd>Branch, tag, commit, or file source used as the publication candidate.</dd></div>
      </dl>
    </section>

    <section id="versions">
      <h2>Handle versions</h2>
      <p>Branches, tags, commits, and singleton spec files stay close to source control. Manja can index each candidate and expose the selected version without asking maintainers to author docs in a second place.</p>
    </section>

    <section id="publication">
      <h2>Publish deliberately</h2>
      <p>Public docs should show the chosen version until a later sync is healthy enough to replace it. That last-known-good behavior keeps readers on stable docs even if source access or parsing fails later.</p>
      <p>Generated examples and request snippets are useful for readers, but the public surface remains read-only and does not proxy upstream APIs.</p>
    </section>
  </div>
</section>
`)
