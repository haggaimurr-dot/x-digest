package xpost

import "testing"

func TestParseModel(t *testing.T) {
	var m modelOut
	if err := parseModel("```json\n{\"title\":\"t\",\"summary\":\"s\",\"tags\":[\"#a\",\"a\",\" b \"],\"steps\":[]}\n```", &m); err != nil {
		t.Fatal(err)
	}
	if got := CleanTags(m.Tags); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("tags %v", got)
	}
	if err := parseModel(`{"title":"","summary":"s"}`, &m); err == nil {
		t.Fatal("empty title should fail")
	}
}

func TestKeepResourcesOnlyRealLinks(t *testing.T) {
	got := KeepResources([]Res{
		{URL: "https://github.com/a/b", Name: "GitHub 仓库 a/b", Note: "源码"},
		{URL: "https://made.up/x", Name: "编的"},
		{URL: "https://github.com/a/b", Name: "重复"},
		{URL: "https://t.co/z", Name: ""},
	}, []string{"https://github.com/a/b", "https://t.co/z"})
	if len(got) != 1 || got[0].Name != "GitHub 仓库 a/b" {
		t.Fatalf("%+v", got)
	}
}

func TestParseStatusURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://x.com/aiwarts/status/2115071600865480927?s=20":     "2115071600865480927",
		"https://twitter.com/i/web/status/2115071600865480927":      "2115071600865480927",
		"mobile.twitter.com/a_b/status/2115071600865480927/photo/1": "2115071600865480927",
		" 2115071600865480927 ":                                     "2115071600865480927",
	} {
		if got, err := ParseStatusURL(in); err != nil || got != want {
			t.Errorf("%q → %q %v", in, got, err)
		}
	}
	if _, err := ParseStatusURL("https://x.com/aiwarts"); err == nil {
		t.Error("profile link should fail")
	}
}

func TestParseIDList(t *testing.T) {
	got := parseIDList("Here:\n```json\n[\"2115071600865480927\", \"2114721604173541727\", \"2115071600865480927\"]\n```")
	if len(got) != 2 || got[1] != "2114721604173541727" {
		t.Fatalf("%v", got)
	}
}
