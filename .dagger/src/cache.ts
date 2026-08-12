const TRUST_DOMAIN = /^(fork|internal|main|release|assets|local)$/

/** Resolve caller context to a cache namespace enforced by the host boundary. */
export function resolveCachePartition(value: string): string {
  if (!TRUST_DOMAIN.test(value)) {
    throw new Error(`unsafe trust domain: ${value}`)
  }
  return value === "fork" || value === "internal" ? "pr" : value
}
