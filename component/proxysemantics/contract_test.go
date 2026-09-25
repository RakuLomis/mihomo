package proxysemantics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKnownFalseSurvivesJSONAndClone(t *testing.T) {
	builder := NewBuilder("ss", 1)
	builder.Configured("enabled", Known(false))
	builder.Effective("enabled", Known(false))
	snapshot := builder.Build()
	cloned := Clone(snapshot)
	field := cloned.Evidence.Effective["enabled"]
	if field.State != StateKnown || field.Value != false {
		t.Fatalf("known false did not survive clone: %#v", field)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"value":false`) {
		t.Fatalf("known false missing from JSON: %s", encoded)
	}
}

func TestBuildIdentityHashesRunningExecutable(t *testing.T) {
	identity := CurrentBuildIdentity()
	if identity.Product == "" || identity.GoVersion == "" {
		t.Fatalf("incomplete build identity: %+v", identity)
	}
	if identity.ExecutableHashStatus != "verified_self" {
		t.Fatalf("executable hash status = %q", identity.ExecutableHashStatus)
	}
	if !strings.HasPrefix(identity.ExecutableSHA256, "sha256:") {
		t.Fatalf("executable digest = %q", identity.ExecutableSHA256)
	}
	if !strings.HasPrefix(identity.DependencyManifestSHA256, "sha256:") {
		t.Fatalf("dependency manifest digest = %q", identity.DependencyManifestSHA256)
	}
}

func TestCloneDoesNotShareEvidenceMaps(t *testing.T) {
	builder := NewBuilder("vless", 1)
	builder.Effective("tls", Known(true))
	original := builder.Build()
	cloned := Clone(original)
	cloned.Evidence.Effective["tls"] = Known(false)
	if original.Evidence.Effective["tls"].Value != true {
		t.Fatal("clone mutated original evidence")
	}
}
