package domain

import "testing"

func TestValidateProjectRejectsInvalidUTF8WithoutNormalizingDisplayText(t *testing.T) {
	valid := Project{
		ID: "payments", Name: "Payments", Slug: "payments",
		SEO: ProjectSEO{
			TitleTemplate: "Payments %s", Description: "Docs",
			CanonicalBase: "https://docs.example.test",
			SocialImage:   "https://docs.example.test/social.png",
			Robots:        "index,follow",
		},
		Theme:     ThemeSettings{Theme: "system", DarkMode: "auto"},
		SourceIDs: []string{"payments-git"},
	}
	for _, test := range []struct {
		name   string
		mutate func(*Project)
	}{
		{name: "name", mutate: func(project *Project) { project.Name = "bad-\xff" }},
		{name: "SEO title", mutate: func(project *Project) { project.SEO.TitleTemplate = "bad-\xff" }},
		{name: "SEO description", mutate: func(project *Project) { project.SEO.Description = "bad-\xff" }},
		{name: "SEO canonical", mutate: func(project *Project) { project.SEO.CanonicalBase = "bad-\xff" }},
		{name: "SEO social image", mutate: func(project *Project) { project.SEO.SocialImage = "bad-\xff" }},
		{name: "SEO robots", mutate: func(project *Project) { project.SEO.Robots = "bad-\xff" }},
		{name: "theme", mutate: func(project *Project) { project.Theme.Theme = "bad-\xff" }},
		{name: "dark mode", mutate: func(project *Project) { project.Theme.DarkMode = "bad-\xff" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			project := valid
			test.mutate(&project)
			if err := ValidateProject(project); err == nil {
				t.Fatal("ValidateProject accepted invalid UTF-8")
			}
		})
	}

	valid.Name = "  Pagamentos 日本語  \n"
	valid.SEO.Description = "linha 1\nlinha 2"
	if err := ValidateProject(valid); err != nil {
		t.Fatalf("ValidateProject rejected valid display text: %v", err)
	}
}
