# Canonical Arai Hû Default Theme

Date: 2026-07-27

Manja consumes the canonical organization theme as an application-owned static
stylesheet. Goshtoso v0.0.13's `head.Dependencies()` emits its compiled
`/assets/styles.css` first; Manja then loads `/manja-assets/araihu.css`, before
its existing app-specific `/manja-assets/manja.css`. No Goshtoso base theme or
dependency-module file is modified.

Provenance: [araihu/assets `themes/araihu.css` at commit
`f841fe90b967b16ab2ad9efaee5aa636468e1afd`](https://github.com/araihu/assets/blob/f841fe90b967b16ab2ad9efaee5aa636468e1afd/themes/araihu.css).
The canonical Git blob is `f511bd11c0f70733784d09335b9d1b986c9806f4`; exact
decoded bytes have SHA-256
`c0bf105f332bca41af1dbd6ccb867ec9fda9c6c688beb609723c7186842044a4`.

Source-dive note: the public Goshtoso head contract documents `Dependencies()`
as the owner of the compiled theme-token stylesheet and requires consumers to
mount `assets.Handler()` at `/assets/`. The needed consumer extension point is
ordinary stylesheet ordering, so Manja needs no missing Goshtoso API or CSS
escape hatch.

## Deferred Assurance Receipt

Receipt: `QDR-2026-07-27-araihu-extra-platforms`.

The durable session has only its local Darwin/arm64 runtime; no separate Linux
or Windows runner is provisioned within this task's authority. Cross-platform
execution is therefore deferred, not assumed. Scope is limited to the
application-owned static CSS and server-rendered head markup. Exit condition:
run the committed root suite and `internal/web/e2e` Arai Hû theme/mode test on
at least one non-Darwin runner before declaring cross-platform assurance.
