// Package searchv2 defines provider-neutral inputs and deterministic cached-run
// contracts for Manja's deployment-wide search index.
//
// It is intentionally independent from application/catalog's active v1 search
// artifacts and runtime. A later build slice can consume these canonical runs
// without changing the public search surface prematurely.
package searchv2
