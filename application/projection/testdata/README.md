# Projection version 1 fixtures

Version-1 fixtures are independent canonical-wire oracles. Ordinary tests must
never rewrite accepted `.json` or `.sha256` files. Vector A and B bytes come
from the reviewed design; Vector C requires a separately reviewed candidate and
literal manifest before acceptance.

## Vector C manifest

| Source branch | Canonical output |
| --- | --- |
| Identity | `payments`, `rev-0001`, title `Payments <API>\u2028`, API version `2026-07` |
| Branding | display `Payments`; `/logo.svg`; alt `Payments`; home `/`; `/favicon.svg` |
| Overview | `Payments\nAPI`, `/terms`, API-team contact, MIT license |
| Servers | ordinal 0 `https://api.example` with `region` and enum `us`,`eu`; ordinal 1 `https://sandbox.example` |
| Operations | ordinal 0 `operation-create-pet`; ordinal 1 `operation-list-pets` |
| Create tags | `Pets` ordinal 0 and `Admin` ordinal 1; repeated `Pets` excluded |
| Create parameters | `trace` header ordinal 0; `dryRun` query ordinal 1 |
| Request media | `application/json`; embedded JSON `{"a":0.001,"z":1000}`; provided empty example retained |
| Responses | `500` ordinal 0; `201` ordinal 1 |
| Security | `oauth`; scopes `write` ordinal 0 and `read` ordinal 1 |
| Code samples | cURL/shell ordinal 0; JavaScript/javascript ordinal 1 |
| Schemas | ordinal 0 `schema-error`; ordinal 1 `schema-pet` |
| Error schema | object with one `items` record containing string schema |
| Pet schema | object; `id` property; embedded JSON `{"n":0,"script":"\u003cscript\u003e"}` |
| Pet example | schema JSON `{"shape":1}` and distinct primary text `__EXPLICIT_PET__` |
| Search | ordinal 0 `operation-list`; ordinal 1 `overview`; ordinal 2 `schema-pet` |
| Public routes | four records: root, create, list, and Pet, sorted by path then title |
| Sidebar | two hashed operation-tag sections followed by `schemas`; three total |
| Exclusions | `__MANJA_SPEC_DOWNLOAD_SENTINEL_7d67d7e4__` and `__MANJA_EXAMPLE_SPEC_SENTINEL_12eb9dc1__` absent from bytes |

All collections are explicit. All IDs and ordinals are asserted through the
decoded DTO plus the accepted bytes. Canonical examples are strings, not
untyped JSON fields.
