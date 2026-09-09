"""Generate a deterministic, bounded static-export benchmark input."""
import argparse
import hashlib
import json
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("output", type=Path)
root = parser.parse_args().output.resolve()
root.mkdir(parents=True, exist_ok=True)
schemas = {
    "Address": {"type": "object", "properties": {
        "city": {"type": "string"}, "postcode": {"type": "string"}}},
    "Metadata": {"type": "object", "properties": {
        "createdAt": {"type": "string", "format": "date-time"},
        "address": {"$ref": "#/components/schemas/Address"}}},
}
for i in range(128):
    schemas[f"Entity{i}"] = {
        "type": "object", "description": f"Representative entity {i}",
        "properties": {
            **{f"field{j}": {"type": "string", "description": f"Field {j}"}
               for j in range(12)},
            "metadata": {"$ref": "#/components/schemas/Metadata"}},
        "required": ["field0"],
    }
paths = {}
for i in range(256):
    paths[f"/entities/{i}"] = {"get": {
        "operationId": f"getEntity{i}", "tags": [f"Group{i // 16}"],
        "summary": f"Get entity {i}", "responses": {"200": {
            "description": "Success", "content": {"application/json": {
                "schema": {"$ref": f"#/components/schemas/Entity{i % 128}"}}}}}}}
data = json.dumps({"openapi": "3.0.3", "info": {
    "title": "Representative export benchmark", "version": "1.0.0"},
    "paths": paths, "components": {"schemas": schemas}},
    sort_keys=True, separators=(",", ":")).encode() + b"\n"
(root / "representative.json").write_bytes(data)
(root / "renderer.yaml").write_text(f'''version: 1
catalogs:
  - id: representative
    mount: /representative
    title: Representative
    defaultDocument: representative
    profile: strict-v1
    source:
      kind: files
      root: {json.dumps(str(root))}
      include: [representative.json]
''')
print(json.dumps({"operations": 256, "schemas": 130, "groups": 16,
                  "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}, indent=2))
