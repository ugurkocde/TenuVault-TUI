package jsonutil

import (
	"encoding/json"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"Win/OIB: Policy": "Win_OIB_ Policy",
		"":                "unnamed",
		"  spaced  ":      "spaced",
	}
	for in, want := range cases {
		if got := SanitizeFilename(in); got != want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripKeys(t *testing.T) {
	raw := []byte(`{"id":"1","displayName":"x","nested":{"id":"2","keep":true},"arr":[{"id":"3"}]}`)
	got, err := Normalize(raw, map[string]bool{"id": true})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(got)
	if string(out) == "" {
		t.Fatal("empty")
	}
	m := got.(map[string]any)
	if _, ok := m["id"]; ok {
		t.Error("top-level id not stripped")
	}
	if _, ok := m["nested"].(map[string]any)["id"]; ok {
		t.Error("nested id not stripped")
	}
	if m["nested"].(map[string]any)["keep"] != true {
		t.Error("keep dropped")
	}
}

func TestDisplayName(t *testing.T) {
	if got := DisplayName([]byte(`{"name":"settings cat"}`), "name"); got != "settings cat" {
		t.Errorf("got %q", got)
	}
	if got := DisplayName([]byte(`{"displayName":"dc"}`), "name"); got != "dc" {
		t.Errorf("fallback failed, got %q", got)
	}
}

func TestIsODataAnnotation(t *testing.T) {
	drop := []string{"@odata.context", "authenticationStrength@odata.context", "x@odata.navigationLink"}
	for _, k := range drop {
		if !IsODataAnnotation(k) {
			t.Errorf("%q should be dropped", k)
		}
	}
	if IsODataAnnotation("@odata.type") {
		t.Error("@odata.type must be preserved")
	}
	if IsODataAnnotation("displayName") {
		t.Error("displayName must be kept")
	}
}
