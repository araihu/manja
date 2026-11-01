#!/usr/bin/env python3
"""Extract the offline browser corpus from verified, pinned GitHub GHEC inputs."""
import argparse
import copy
import hashlib
import json
from pathlib import Path

COMMIT = "320307e220815d528224ca891cf8a9e3c0243678"
SOURCE_SHA256 = "65cbdfbd52b2f966681632e1e04cbdd96fa9ad149a09d6825c71be0161920a33"
LICENSE_SHA256 = "3243761cbac07e6d169a5a2f4e7c25cc544da85248e735df74c3672e055cc87b"
OPERATIONS = (
    "actions/get-actions-cache-usage-for-enterprise",
    "actions/add-custom-labels-to-self-hosted-runner-for-org",
    "apps/list-webhook-deliveries",
    "apps/redeliver-webhook-delivery",
    "actions/create-self-hosted-runner-group-for-org",
    "repos/add-user-access-restrictions",
    "enterprise-admin/create-registration-token-for-enterprise",
)
LEGACY_SHA256 = "dedfee9ad6a676c2f7186b8e2137d887d6449cad8b7af8253aecdaae24b27977"
LEGACY_OPERATION = "actions/create-self-hosted-runner-group-for-org"
EXTRA_REFS = ("#/components/schemas/actions-artifact-and-log-retention-response",)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def blob(data):
    return hashlib.sha1(b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()


def encode(value):
    return (json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()


def extract(source):
    result = {key: source[key] for key in ("openapi", "info", "servers", "externalDocs") if key in source}
    result["paths"] = {}
    found = set()
    for path, item in source["paths"].items():
        selected = {method: operation for method, operation in item.items()
                    if isinstance(operation, dict) and operation.get("operationId") in OPERATIONS}
        if selected:
            found.update(operation["operationId"] for operation in selected.values())
            # Path-level parameters apply to every selected method.
            selected.update({key: item[key] for key in ("parameters", "summary", "description", "servers") if key in item})
            result["paths"][path] = selected
    if found != set(OPERATIONS):
        raise ValueError(f"Missing operations: {set(OPERATIONS) - found}")
    result["components"] = {}
    visited = set()

    def include(reference):
        if reference in visited:
            return
        if not reference.startswith("#/components/"):
            raise ValueError(f"Non-local component reference: {reference}")
        visited.add(reference)
        segments = [segment.replace("~1", "/").replace("~0", "~") for segment in reference[2:].split("/")]
        original = source
        target = result
        for segment in segments[:-1]:
            original = original[segment]
            target = target.setdefault(segment, {})
        target[segments[-1]] = original[segments[-1]]
        visit(original[segments[-1]])

    def visit(value):
        if isinstance(value, dict):
            if "$ref" in value:
                include(value["$ref"])
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)

    visit(result["paths"])
    for reference in EXTRA_REFS:
        include(reference)
    return result


def include_legacy_markdown(source, legacy):
    # Retain the original inline-code regression: newer GitHub descriptions
    # replaced these backticks with prose. Namespace refs to avoid mixing schemas.
    def rewrite(value):
        if isinstance(value, dict):
            result = {key: rewrite(child) for key, child in value.items()}
            reference = result.get("$ref", "")
            if reference.startswith("#/components/"):
                parts = reference.split("/")
                parts[3] = "legacy-" + parts[3]
                result["$ref"] = "/".join(parts)
            return result
        if isinstance(value, list):
            return [rewrite(child) for child in value]
        return value

    result = copy.deepcopy(source)
    for path, item in legacy["paths"].items():
        for method, operation in item.items():
            if isinstance(operation, dict) and operation.get("operationId") == LEGACY_OPERATION:
                result["paths"][path][method] = rewrite(operation)
    for category, items in legacy["components"].items():
        result["components"].setdefault(category, {}).update({"legacy-" + key: rewrite(value) for key, value in items.items()})
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--license", required=True, type=Path)
    parser.add_argument("--legacy-source", type=Path,
                        default=Path(__file__).resolve().parents[3] / "adapters/openapi/testdata/github-v3-rest.json")
    args = parser.parse_args()
    raw = args.source.read_bytes()
    if digest(raw) != SOURCE_SHA256:
        raise ValueError("Upstream source SHA-256 differs from the pinned revision")
    license_bytes = args.license.read_bytes()
    if digest(license_bytes) != LICENSE_SHA256:
        raise ValueError("Upstream license SHA-256 differs from the pinned revision")
    output = Path(__file__).resolve().parent
    legacy_bytes = args.legacy_source.read_bytes()
    if digest(legacy_bytes) != LEGACY_SHA256:
        raise ValueError("Legacy Markdown source SHA-256 differs from its pinned revision")
    payload = encode(extract(include_legacy_markdown(json.loads(raw), json.loads(legacy_bytes))))
    (output / "github-v3-rest.json").write_bytes(payload)
    (output / "LICENSE.md").write_bytes(license_bytes)
    receipt = {
        "schemaVersion": 1,
        "repository": "https://github.com/github/rest-api-description",
        "commitSha": COMMIT,
        "source": {"path": "descriptions/ghec/ghec.json", "size": len(raw), "sha256": digest(raw), "gitBlobSha": blob(raw)},
        "operations": list(OPERATIONS),
        "legacyMarkdown": {
            "commitSha": "6948cb04f5304188569c4bb4ae2190c08e7cbdba",
            "path": "descriptions/ghes-3.0/ghes-3.0.json",
            "sha256": digest(legacy_bytes),
            "operationId": LEGACY_OPERATION,
            "componentPrefix": "legacy-",
        },
        "extraRefs": list(EXTRA_REFS),
        "fixture": {"path": "github-v3-rest.json", "size": len(payload), "sha256": digest(payload)},
        "license": {"path": "LICENSE.md", "size": len(license_bytes), "sha256": digest(license_bytes), "gitBlobSha": blob(license_bytes)},
    }
    (output / "provenance.json").write_bytes(encode(receipt))


if __name__ == "__main__":
    main()
