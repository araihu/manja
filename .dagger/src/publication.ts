export type Publication = {
  buildVersion: string
  ociVersion: string
  tags: string[]
}

export function assertOCIPublicationGate(status: string): void {
  if (status !== "PASS") {
    throw new Error(
      "OCI publication is BLOCKED: OC-01 first-party authority and redistribution/trademark clearance are not PASS for the exact image digest",
    )
  }
}

export function resolvePublication(
  refType: string,
  refName: string,
  sourceSHA: string,
): Publication {
  if (refType === "branch" && refName === "main") {
    return {
      buildVersion: sourceSHA,
      ociVersion: "main",
      tags: ["main", sourceSHA],
    }
  }
  if (
    refType === "tag" &&
    /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(refName)
  ) {
    const [major, minor, patch] = refName.slice(1).split(".")
    return {
      buildVersion: refName,
      ociVersion: refName.slice(1),
      tags: [`${major}.${minor}.${patch}`, `${major}.${minor}`, major, "latest"],
    }
  }
  throw new Error("publish ref must be main or a stable SemVer tag")
}
