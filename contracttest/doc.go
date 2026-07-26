// Package contracttest provides environment-neutral conformance suites for
// adapters that implement Manja's public application ports.
//
// Factories own adapter setup and cleanup. They may use testing.TB.TempDir and
// testing.TB.Cleanup, but the suites never provision containers, repositories,
// credentials, or network resources. Each factory call must return an isolated
// adapter safe for the lifecycle documented by its suite.
//
// UnitOfWork reports the release-transition concurrency and isolation cases as
// skipped when the adapter also implements ReleaseAuthorizationWriter, because
// authenticated state requires canonical blobs, reviews, syncs, and effects.
// Such adapters must additionally run ReleaseAuthorityUnitOfWork, which tests
// those behaviors with a complete authenticated evidence fixture.
package contracttest
