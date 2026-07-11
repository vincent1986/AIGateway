package main

import "testing"

func TestSetTomlTopLevel(t *testing.T) {
	in := "model = \"a\"\nmodel_provider = \"p\"\n\n[model_providers.x]\nname = \"x\"\n"
	out := setTomlTopLevelString(in, "model", "b")
	if got := readTomlTopLevelString(out, "model"); got != "b" {
		t.Fatalf("model=%q", got)
	}
	if got := readTomlTopLevelString(out, "model_provider"); got != "p" {
		t.Fatalf("provider=%q", got)
	}
	cands := parseCodexModels("[[models]]\nname = \"N\"\nprovider = \"p1\"\nmodel = \"m1\"\n")
	if len(cands) != 1 || cands[0].ID != "m1" || cands[0].Provider != "p1" {
		t.Fatalf("%+v", cands)
	}
}
