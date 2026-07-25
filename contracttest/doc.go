// Package contracttest provides environment-neutral conformance suites for
// adapters that implement Manja's public application ports.
//
// Factories own adapter setup and cleanup. They may use testing.TB.TempDir and
// testing.TB.Cleanup, but the suites never provision containers, repositories,
// credentials, or network resources. Each factory call must return an isolated
// adapter safe for the lifecycle documented by its suite.
package contracttest
