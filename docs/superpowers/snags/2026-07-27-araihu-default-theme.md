# Canonical Arai Hû Default Theme

Date: 2026-07-27

Manja consumes the canonical organization theme as an application-owned static
stylesheet. Goshtoso v0.0.13's `head.Dependencies()` emits its compiled
`/assets/styles.css` first; Manja then loads `/manja-assets/araihu.css`, before
its existing app-specific `/manja-assets/manja.css`. No Goshtoso base theme or
dependency-module file is modified.

Provenance: [araihu/assets `themes/araihu.css` at commit
`a8a9647a6e803586c556859eb20f95ef9fcb20a1`](https://github.com/araihu/assets/blob/a8a9647a6e803586c556859eb20f95ef9fcb20a1/themes/araihu.css).
The canonical Git blob is `15d7d2df1fc13199b518abb7fdc1379931bf6858`; exact
decoded bytes have SHA-256
`9e7756cea751aa95bcf2f0b6545dc32ab9037a47a08a91287502dae52829d265`.

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
