package domain

import (
	"strings"
	"testing"
)

func TestCatalogPublicationValidationAndSemver(t *testing.T) {
	valid := CatalogPublication{API: "internal", Audience: "internal", Channel: "release", Version: "0.2.0", Latest: true, Ref: "refs/tags/openapi/internal/v0.2.0", Commit: strings.Repeat("a", 40), SourcePath: "api/internal.yaml", RepositoryURL: "https://github.com/example/project"}
	for _, test := range []struct {
		name   string
		mutate func(*CatalogPublication)
	}{
		{"missing API", func(p *CatalogPublication) { p.API = "" }},
		{"short commit", func(p *CatalogPublication) { p.Commit = "aaaa" }},
		{"source escape", func(p *CatalogPublication) { p.SourcePath = "../private.yaml" }},
		{"credentials", func(p *CatalogPublication) { p.RepositoryURL = "https://token@example.com/project" }},
		{"latest prerelease", func(p *CatalogPublication) { p.Version = "0.2.0-rc.1" }},
		{"branch release", func(p *CatalogPublication) { p.Ref = "refs/heads/main" }},
		{"leading zero prerelease", func(p *CatalogPublication) { p.Version = "0.2.0-01"; p.Latest = false }},
		{"latest next", func(p *CatalogPublication) { p.Channel = "next"; p.Version = ""; p.Ref = "refs/heads/main" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := valid
			test.mutate(&p)
			if ValidateCatalogPublication(&p) == nil {
				t.Fatal("invalid publication accepted")
			}
		})
	}
	if err := ValidateCatalogPublication(&valid); err != nil {
		t.Fatal(err)
	}
	next := valid
	next.Channel = "next"
	next.Version = ""
	next.Latest = false
	next.Ref = "refs/heads/main"
	if err := ValidateCatalogPublicationSet([]*CatalogPublication{&valid, &next}); err != nil {
		t.Fatal(err)
	}
	duplicate := valid
	if ValidateCatalogPublicationSet([]*CatalogPublication{&valid, &duplicate}) == nil {
		t.Fatal("duplicate publication accepted")
	}
	higher := valid
	higher.Version = "0.10.0"
	higher.Ref = "refs/tags/openapi/internal/v0.10.0"
	higher.Latest = false
	if ValidateCatalogPublicationSet([]*CatalogPublication{&valid, &higher}) == nil {
		t.Fatal("incorrect latest accepted")
	}
	for _, pair := range [][2]string{{"1.10.0", "1.9.0"}, {"1.2.0", "1.2.0-rc.10"}, {"1.2.0-rc.10", "1.2.0-rc.2"}, {"99999999999999999999.0.0", "9.0.0"}} {
		if ComparePublicationVersions(pair[0], pair[1]) <= 0 || ComparePublicationVersions(pair[1], pair[0]) >= 0 {
			t.Fatalf("incorrect SemVer ordering %v", pair)
		}
	}
	if ComparePublicationVersions("1.0.0+one", "1.0.0+two") != 0 {
		t.Fatal("build metadata changed precedence")
	}
}

func TestCatalogBranchPublicationNames(t *testing.T) {
	base := CatalogPublication{API: "public", Channel: "branch", Ref: "refs/heads/main", Commit: strings.Repeat("a", 40), SourcePath: "api/public.yaml"}
	for _, test := range []struct{ channel, ref, name, want string }{
		{"branch", "refs/heads/main", "", "main · API 1.0.0"},
		{"branch", "refs/heads/master", "", "master · API 1.0.0"},
		{"branch", "refs/heads/release/v2", "", "release/v2 · API 1.0.0"},
		{"branch", "refs/heads/main", "preview", "preview · API 1.0.0"},
		{"next", "refs/heads/master", "", "master · API 1.0.0"},
	} {
		p := base
		p.Channel, p.Ref, p.ChannelName = test.channel, test.ref, test.name
		if err := ValidateCatalogPublication(&p); err != nil {
			t.Fatal(err)
		}
		if got := p.Label("1.0.0"); got != test.want {
			t.Errorf("publication label = %q, want %q", got, test.want)
		}
	}
	for _, name := range []string{" preview", "preview\nmain", strings.Repeat("a", 129)} {
		p := base
		p.ChannelName = name
		if ValidateCatalogPublication(&p) == nil {
			t.Errorf("invalid channelName accepted: %q", name)
		}
	}
	legacy := base
	legacy.Channel = "next"
	if ValidateCatalogPublicationSet([]*CatalogPublication{&base, &legacy}) == nil {
		t.Fatal("legacy and branch channels duplicated the same API publication")
	}
	release := base
	release.Channel, release.Version, release.Ref, release.ChannelName = "release", "1.0.0", "refs/tags/v1.0.0", "preview"
	if ValidateCatalogPublication(&release) == nil {
		t.Fatal("release accepted a branch channelName")
	}
}
