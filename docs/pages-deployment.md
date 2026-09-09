# Deploy to Pages

These examples publish the complete output of `manja export` from a repository
containing `renderer.yaml` and its OpenAPI sources. Start with the
[static export guide](static-export.md) for a minimal configuration and host
requirements. The site runs without a Manja server after deployment.

The examples install revision `6ee0252301ff6fd060d206abf5fb574de8b211b2`, which
includes incremental HTML export, with Go 1.27.0. Update this pin deliberately
when adopting a newer version; older releases may have different export
behavior. The documentation repository does not need a Go module of its own.
To install the same version locally into `./bin`:

```bash
GOBIN="$PWD/bin" go install github.com/araihu/manja/cmd/manja@6ee0252301ff6fd060d206abf5fb574de8b211b2
```

Both examples build a fresh export. To enable reuse across runs, add storage
for the previous verified output following
[incremental rebuilds](static-export.md#incremental-rebuilds). Do not put
`.manja/data`, source credentials, or build tools in the deployment artifact.

## GitHub Pages

Set the repository's Pages publishing source to **GitHub Actions**. Add this
workflow as `.github/workflows/pages.yml` in your documentation repository.
Set `MANJA_BASE_PATH` to the final URL prefix: `/api-docs/` for a repository
published at `https://example.github.io/api-docs/`, or `/` for a root/custom
domain site. Change the branch trigger if your default branch is not `main`.

```yaml
name: Publish API docs
on:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read

concurrency:
  group: manja-pages
  cancel-in-progress: false

env:
  MANJA_BASE_PATH: /api-docs/
  MANJA_VERSION: 6ee0252301ff6fd060d206abf5fb574de8b211b2

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v7
        with:
          go-version: '1.27.0'
          cache: false
      - uses: actions/configure-pages@v5
      - name: Install Manja
        run: GOBIN="$PWD/bin" go install "github.com/araihu/manja/cmd/manja@$MANJA_VERSION"
      - name: Export and verify
        run: |
          ./bin/manja export \
            --renderer-config ./renderer.yaml \
            --data-dir ./.manja/data \
            --output ./public \
            --base-path "$MANJA_BASE_PATH" \
            --sidebar-chunk-size 12 \
            --fragment-workers 4
          ./bin/manja export verify --output ./public
      - uses: actions/upload-pages-artifact@v4
        with:
          path: public

  deploy:
    needs: build
    runs-on: ubuntu-latest
    permissions:
      pages: write
      id-token: write
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    steps:
      - name: Deploy
        id: deployment
        uses: actions/deploy-pages@v4
```

This uses a custom workflow, so the generated files are uploaded directly
without a Jekyll build. Keep the export directory complete, including `_manja`
and sidecars. The build and deployment are separate jobs: allow enough build
time for large specs independently of the provider's deployment timeout.

See GitHub's [custom workflow documentation](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages)
and [Pages limits](https://docs.github.com/en/pages/getting-started-with-github-pages/github-pages-limits).
As checked on 2026-09-08, the published site limit is 1 GB and deployments have
a 10-minute timeout. A small compressed upload does not by itself establish
that the published site fits the limit.

## GitLab Pages

For GitLab 17.10 or later, add this `.gitlab-ci.yml` to your documentation
repository. Set `MANJA_BASE_PATH` from the actual Pages URL shown by GitLab.
A unique or custom domain normally serves at `/`; a project/subgroup URL may
have a prefix. Do not assume the prefix equals the full repository namespace.

```yaml
image: golang:1.27.0

stages:
  - deploy

variables:
  MANJA_BASE_PATH: "/"
  MANJA_VERSION: "6ee0252301ff6fd060d206abf5fb574de8b211b2"

publish-docs:
  stage: deploy
  resource_group: manja-pages
  rules:
    - if: '$CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH'
  script:
    - GOBIN="$PWD/bin" go install "github.com/araihu/manja/cmd/manja@$MANJA_VERSION"
    - |
      ./bin/manja export \
        --renderer-config ./renderer.yaml \
        --data-dir ./.manja/data \
        --output ./public \
        --base-path "$MANJA_BASE_PATH" \
        --sidebar-chunk-size 12 \
        --fragment-workers 4
    - ./bin/manja export verify --output ./public
  pages:
    publish: public
```

GitLab 17.10+ adds `pages.publish` to artifact paths automatically. The job
publishes `public`, not the source checkout or the build data directory. See
the [Pages CI reference](https://docs.gitlab.com/ci/yaml/#pagespublish).

As checked on 2026-09-08, GitLab.com documents a 1 GB maximum Pages site size,
linked to its artifact limit, and 200,000 filesystem entries per site.
Self-Managed instance limits can differ; the documented default unpacked Pages
limit is 100 MB. Check your instance before publishing a large catalog set.
See [GitLab.com settings](https://docs.gitlab.com/user/gitlab_com/#gitlab-pages),
[entry limits](https://docs.gitlab.com/administration/instance_limits/#gitlab-pages-limits),
and [Self-Managed size settings](https://docs.gitlab.com/administration/pages/source/#set-maximum-pages-size).

## Verify the published site

Run the [deployment checks](static-export.md#deployment-checks-and-troubleshooting)
against the actual public URL. Root and subpath builds are covered by Manja's
generic static-server browser tests; provider-specific CI permissions, quotas,
and domain configuration still need validation in your project.
