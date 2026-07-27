# Manja Management State And Action Ledger

Date: 2026-07-27
Status: Required executable contract for the application-structure slice

This ledger freezes current management behavior. The UI refactor may improve
presentation and recovery but may not broaden authorization or side effects.
Tests must replace fixture labels below with exact server state and assert the
effect counts exposed by fake stores/actions.

| Route/state | Action/request | Allowed? | Expected response | Context preserved | Focus/destination | Effect count |
| --- | --- | ---: | --- | --- | --- | ---: |
| contract list / loaded | Select contract | yes | selected workspace fragment plus canonical URL | query, theme, mode | selected workspace heading | 0 mutations |
| contract list / filtered empty | Clear filter | yes | populated list or true empty state | theme, mode | filter control | 0 mutations |
| contract detail / healthy candidate | Sync ref | yes | refreshed authoritative workspace | selected contract, ref, theme, mode | explicit result or workspace heading | 1 sync |
| contract detail / unavailable candidate list | Forged Sync ref | no | in-shell policy/validation error | selected contract, entered ref | error summary | 0 syncs |
| contract detail / transport failure | Sync ref | retryable | visible retry state | selected contract, ref | retry control | 0 completed syncs |
| contract detail / healthy revision | Publish current revision | yes | PRG or authoritative success fragment | selected contract, public path, theme, mode | same contract receipt/workspace | 1 publication write |
| contract detail / missing contract or revision | Forged Publish | no | in-shell not-found/validation recovery | known list/filter/theme/mode | recovery heading | 0 writes |
| contract detail / persistence failure | Publish | no | visible retryable server error | selected contract and valid form values | error/retry target | 0 committed writes |
| contract detail / request in flight | Repeat submit | no duplicate | loading copy and disabled submitter | all form values | initiating control remains owned | still 1 eventual effect |
| contract detail / completed request | Replay identical request | idempotent where service supports it; otherwise explicit validation | same authoritative state | selected contract | same receipt/workspace | no duplicate effect |
| unknown management route | GET | no mutation | complete in-shell recovery document | theme and mode | recovery heading | 0 mutations |
| any unauthorized state introduced later | forged mutation | no | 403 in-shell recovery | route context | permission heading | 0 effects |

Release-track promotion, preview, review, and policy actions are absent by
design. Their implementing slice must append rows before exposing controls.

## Executable response policy

- Expected validation, sync, persistence, and HTMX not-found outcomes render
  the authoritative workspace with HTTP 200 plus
  `X-Manja-Application-Status`; this is the explicit swap contract, not a
  successful domain effect. Direct missing-resource loads remain HTTP 404.
- Malformed requests, missing server configuration, and unexpected server
  failures remain non-2xx transport/server responses. The persistent
  management transport region owns retry for those failures without replacing
  the selected workspace.
- Management responses send `Cache-Control: no-store`. Successful native POSTs
  use 303 PRG; HTMX POSTs return the authoritative fragment and canonical
  history instruction. A different canonical destination uses `HX-Push-Url`;
  a mutation that stays on the current canonical URL uses `HX-Replace-Url`.
  Management shells set `hx-history="false"` so Back and Forward reload
  server-authoritative state instead of restoring a stale HTMX snapshot.
- Mutation forms carry a deterministic request ID, Goshtoso loading text, and
  `hx-disabled-elt`. The handler stores one bounded request token plus a
  canonical submitted-payload fingerprint per contract and action slot. An
  immediate replay is idempotent only when both match; reusing a token with
  different ref, publish, revision, visibility, or path values fails closed
  before any effect.
