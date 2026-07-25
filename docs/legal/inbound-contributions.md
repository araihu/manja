# Inbound Contribution Policy

Date: 2026-07-25

Selected policy: **Developer Certificate of Origin 1.1 (DCO)**

Activation status: **BLOCKED** until `docs/legal/provenance.md` is `PASS` and a
verified project license is present. External contributions must not be
accepted under an unstated or assumed license in the meantime.

## Policy

Once activated, every contribution must include a Git `Signed-off-by` trailer
certifying the Developer Certificate of Origin 1.1 published at
<https://developercertificate.org/>. The sign-off records that the contributor
has the right to submit the contribution under the open-source license stated
in the repository and that the public contribution record may be retained and
redistributed consistently with that license.

The project will enforce sign-off in pull-request checks and document the
remediation command for contributors:

```bash
git commit --signoff
```

Maintainers must not waive missing sign-off by silently adding another person's
certification. A contributor may amend and re-sign their own commits.

## Why DCO

DCO provides an explicit provenance assertion without a separate copyright
assignment or broad relicensing grant. This checkpoint does not introduce a
CLA. A CLA or additional relicensing permission would require a separately
approved, current dual-licensing need and legal review; no such requirement is
in scope.

## Activation Checklist

- `docs/legal/provenance.md` is `PASS` with the actual rights holder and
  authority evidence.
- The verified project license is committed and named in contribution guidance.
- DCO 1.1 sign-off enforcement is enabled for pull requests.
- Existing history is covered by the authority decision; DCO is not treated as
  retroactive evidence for earlier commits.
- Contribution documentation explains public retention of sign-off metadata.
