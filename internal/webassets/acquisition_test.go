package webassets

import "testing"

func TestAcquisitionInventoryMatchesShippedPackages(t *testing.T) {
	want := map[string]string{
		"readme-httpsnippet": "11.1.0", "base64-js": "1.5.1", "buffer": "6.0.3",
		"call-bind-apply-helpers": "1.0.2", "call-bound": "1.0.4", "dunder-proto": "1.0.1",
		"es-define-property": "1.0.1", "es-errors": "1.3.0", "es-object-atoms": "1.1.2",
		"function-bind": "1.1.2", "get-intrinsic": "1.3.0",
		"get-own-enumerable-property-symbols": "3.0.2", "get-proto": "1.0.1", "gopd": "1.2.0",
		"has-symbols": "1.1.0", "hasown": "2.0.4", "highlight-js": "11.11.1",
		"ieee754": "1.2.1", "is-obj": "1.0.1", "is-regexp": "1.0.0",
		"math-intrinsics": "1.1.0", "object-inspect": "1.13.4", "punycode": "1.4.1",
		"qs": "6.15.2", "side-channel": "1.1.0", "side-channel-list": "1.0.1",
		"side-channel-map": "1.0.1", "side-channel-weakmap": "1.0.2",
		"stringify-object": "3.3.0", "url": "0.11.4", "openapi-sampler": "1.7.4",
	}
	got := MuambaResources()
	if len(got) != len(want) {
		t.Fatalf("resources = %d, want %d", len(got), len(want))
	}
	for _, resource := range got {
		if want[resource.Name] != resource.Version {
			t.Fatalf("%s version = %q", resource.Name, resource.Version)
		}
		if len(resource.Downloads) != 2 {
			t.Fatalf("%s downloads = %d, want archive+license", resource.Name, len(resource.Downloads))
		}
		for _, download := range resource.Downloads {
			if download.Integrity == "" || download.Hash == "" {
				t.Fatalf("unlocked %s/%s", resource.Name, download.Name)
			}
		}
	}
}
