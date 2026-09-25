package adapter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/proxysemantics"
)

func marshalSnapshot(t *testing.T, snapshot proxysemantics.Snapshot) string {
	t.Helper()
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func assertRedacted(t *testing.T, encoded string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(encoded, secret) {
			t.Fatalf("semantic snapshot leaked %q: %s", secret, encoded)
		}
	}
}

func TestShadowSocksSemanticsAreAllowlistedAndStableAcrossIdentityChanges(t *testing.T) {
	mapping := map[string]any{
		"type": "ss", "name": "secret-node-a", "server": "203.0.113.10", "port": 443,
		"password": "secret-password-a", "cipher": "aes-128-gcm", "plugin": "v2ray-plugin",
		"plugin-opts": map[string]any{
			"mode": "websocket", "host": "secret.example", "path": "/secret-path",
			"headers": map[string]string{"Authorization": "secret-header"}, "tls": true, "mux": false,
		},
	}
	option := &outbound.ShadowSocksOption{
		Name: "secret-node-a", Server: "203.0.113.10", Port: 443, Password: "secret-password-a",
		Cipher: "aes-128-gcm", Plugin: "v2ray-plugin", PluginOpts: mapping["plugin-opts"].(map[string]any),
	}
	first := buildProxySemantics("ss", mapping, option, nil)
	mapping["name"] = "secret-node-b"
	mapping["server"] = "198.51.100.20"
	mapping["password"] = "secret-password-b"
	option.Name = "secret-node-b"
	option.Server = "198.51.100.20"
	option.Password = "secret-password-b"
	second := buildProxySemantics("ss", mapping, option, nil)

	if first.BehaviorFingerprint != second.BehaviorFingerprint {
		t.Fatalf("identity-only changes altered behavior fingerprint: %q != %q", first.BehaviorFingerprint, second.BehaviorFingerprint)
	}
	if first.SnapshotID == second.SnapshotID || first.AdapterInstanceID == second.AdapterInstanceID {
		t.Fatal("runtime instances must have distinct snapshot and adapter IDs")
	}
	if second.ConfigGeneration <= first.ConfigGeneration {
		t.Fatalf("config generation did not increase: %d then %d", first.ConfigGeneration, second.ConfigGeneration)
	}
	encoded := marshalSnapshot(t, first)
	assertRedacted(t, encoded, "secret-node-a", "203.0.113.10", "secret-password-a", "secret.example", "/secret-path", "secret-header")
	if got := first.Evidence.Effective["plugin_mux"]; got.State != proxysemantics.StateKnown || got.Value != false {
		t.Fatalf("plugin_mux = %#v, want known false", got)
	}
	if first.Coverage.Status != "partial" {
		t.Fatalf("coverage = %q, want partial until connection evidence is attached", first.Coverage.Status)
	}
}

func TestVLESSSemanticsExposeBehaviorWithoutRealitySecrets(t *testing.T) {
	mapping := map[string]any{
		"type": "vless", "name": "private-vless", "server": "vless.private.example",
		"uuid": "secret-uuid", "network": "grpc", "tls": true, "flow": "xtls-rprx-vision",
		"servername":   "cover.private.example",
		"reality-opts": map[string]any{"public-key": "secret-public-key", "short-id": "secret-short-id"},
		"grpc-opts":    map[string]any{"grpc-service-name": "secret-service"},
		"smux":         map[string]any{"enabled": true, "protocol": "h2mux", "max-connections": 2},
	}
	option := &outbound.VlessOption{
		Name: "private-vless", Server: "vless.private.example", UUID: "secret-uuid",
		Network: "grpc", TLS: true, Flow: "xtls-rprx-vision",
		RealityOpts: outbound.RealityOptions{PublicKey: "secret-public-key", ShortID: "secret-short-id"},
	}
	mux := &outbound.SingMuxOption{Enabled: true, Protocol: "h2mux", MaxConnections: 2}
	snapshot := buildProxySemantics("vless", mapping, option, mux)
	encoded := marshalSnapshot(t, snapshot)
	assertRedacted(t, encoded, "private-vless", "vless.private.example", "secret-uuid", "cover.private.example", "secret-public-key", "secret-short-id", "secret-service")
	for field, want := range map[string]any{
		"transport": "grpc", "tls": true, "reality": true, "vision": true, "smux_enabled": true,
	} {
		if got := snapshot.Evidence.Effective[field]; got.State != proxysemantics.StateKnown || got.Value != want {
			t.Errorf("%s = %#v, want known %#v", field, got, want)
		}
	}
}

func TestHysteria2SemanticsRejectUnrecognizedALPNAndRedactObfsIdentity(t *testing.T) {
	mapping := map[string]any{
		"type": "hysteria2", "name": "private-hy2", "server": "hy2.private.example",
		"password": "secret-password", "obfs": "salamander", "obfs-password": "secret-obfs",
		"sni": "cover.private.example", "alpn": []string{"secret-protocol"},
	}
	option := &outbound.Hysteria2Option{
		Name: "private-hy2", Server: "hy2.private.example", Password: "secret-password",
		Obfs: "salamander", ObfsPassword: "secret-obfs", SNI: "cover.private.example",
		ALPN: []string{"secret-protocol"},
	}
	snapshot := buildProxySemantics("hysteria2", mapping, option, nil)
	encoded := marshalSnapshot(t, snapshot)
	assertRedacted(t, encoded, "private-hy2", "hy2.private.example", "secret-password", "secret-obfs", "cover.private.example", "secret-protocol")
	if got := snapshot.Evidence.Effective["obfs"]; got.Value != "salamander" {
		t.Fatalf("obfs = %#v, want salamander", got)
	}
	if got := snapshot.Evidence.Effective["alpn"]; got.State != proxysemantics.StateUnsupported {
		t.Fatalf("alpn = %#v, want unsupported", got)
	}
}

func TestSupportedProtocolBuildersUseOneContract(t *testing.T) {
	cases := []struct {
		protocol string
		option   any
	}{
		{"ss", &outbound.ShadowSocksOption{}},
		{"vless", &outbound.VlessOption{}},
		{"hysteria2", &outbound.Hysteria2Option{}},
		{"trojan", &outbound.TrojanOption{}},
		{"vmess", &outbound.VmessOption{}},
		{"anytls", &outbound.AnyTLSOption{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.protocol, func(t *testing.T) {
			snapshot := buildProxySemantics(testCase.protocol, map[string]any{"type": testCase.protocol}, testCase.option, nil)
			if snapshot.SchemaVersion != proxysemantics.SchemaVersion || snapshot.Protocol != testCase.protocol ||
				snapshot.SnapshotID == "" || snapshot.AdapterInstanceID == "" || snapshot.BehaviorFingerprint == "" {
				t.Fatalf("incomplete snapshot: %+v", snapshot)
			}
		})
	}
}

func TestUnsupportedPluginNameIsNotReflected(t *testing.T) {
	const secretPlugin = "private-plugin-secret"
	snapshot := buildProxySemantics(
		"ss",
		map[string]any{"type": "ss", "plugin": secretPlugin},
		&outbound.ShadowSocksOption{Plugin: secretPlugin},
		nil,
	)
	encoded := marshalSnapshot(t, snapshot)
	assertRedacted(t, encoded, secretPlugin)
	if got := snapshot.Evidence.Configured["plugin"]; got.State != proxysemantics.StateUnsupported {
		t.Fatalf("configured plugin = %#v, want unsupported", got)
	}
}

func TestArbitrarySemanticEnumValuesAreNeverReflected(t *testing.T) {
	const secret = "private-semantic-secret"
	cases := []struct {
		name     string
		protocol string
		mapping  map[string]any
		option   any
		mux      *outbound.SingMuxOption
		field    string
	}{
		{"ss cipher", "ss", map[string]any{"type": "ss", "cipher": secret}, &outbound.ShadowSocksOption{Cipher: secret}, nil, "cipher"},
		{"ss plugin mode", "ss", map[string]any{"type": "ss", "cipher": "none", "plugin": "obfs", "plugin-opts": map[string]any{"mode": secret}}, &outbound.ShadowSocksOption{Cipher: "none", Plugin: "obfs", PluginOpts: map[string]any{"mode": secret}}, nil, "plugin_mode"},
		{"vless flow", "vless", map[string]any{"type": "vless", "flow": secret}, &outbound.VlessOption{Flow: secret}, nil, "flow"},
		{"hysteria2 obfs", "hysteria2", map[string]any{"type": "hysteria2", "obfs": secret}, &outbound.Hysteria2Option{Obfs: secret}, nil, "obfs"},
		{"vmess cipher", "vmess", map[string]any{"type": "vmess", "cipher": secret}, &outbound.VmessOption{Cipher: secret}, nil, "cipher"},
		{"smux protocol", "vless", map[string]any{"type": "vless", "smux": map[string]any{"enabled": true, "protocol": secret}}, &outbound.VlessOption{}, &outbound.SingMuxOption{Enabled: true, Protocol: secret}, "smux_protocol"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			snapshot := buildProxySemantics(testCase.protocol, testCase.mapping, testCase.option, testCase.mux)
			encoded := marshalSnapshot(t, snapshot)
			assertRedacted(t, encoded, secret)
			if got := snapshot.Evidence.Effective[testCase.field]; got.State != proxysemantics.StateUnsupported {
				t.Fatalf("effective %s = %#v, want unsupported", testCase.field, got)
			}
		})
	}
}

func TestExplicitConfigGenerationIsSharedAcrossRuntimeAdapters(t *testing.T) {
	generation := NextProxyConfigGeneration()
	first := buildProxySemanticsWithGeneration(
		"ss", map[string]any{"type": "ss"},
		&outbound.ShadowSocksOption{}, nil, generation,
	)
	second := buildProxySemanticsWithGeneration(
		"vless", map[string]any{"type": "vless"},
		&outbound.VlessOption{}, nil, generation,
	)
	if first.ConfigGeneration != generation || second.ConfigGeneration != generation {
		t.Fatalf(
			"generation mismatch: first=%d second=%d want=%d",
			first.ConfigGeneration, second.ConfigGeneration, generation,
		)
	}
	if first.AdapterInstanceID == second.AdapterInstanceID {
		t.Fatal("adapters in one config generation still require distinct instance IDs")
	}
}

func TestFutureProtocolUsesExplicitPartialEvidenceWithoutReflection(t *testing.T) {
	const secret = "future-protocol-secret"
	snapshot := buildProxySemanticsWithGeneration(
		"tuic",
		map[string]any{
			"type": "tuic", "name": secret, "server": secret,
			"token": secret, "arbitrary-nested": map[string]any{"secret": secret},
		},
		nil, nil, NextProxyConfigGeneration(),
	)
	encoded := marshalSnapshot(t, snapshot)
	assertRedacted(t, encoded, secret)
	if snapshot.Protocol != "tuic" || snapshot.Coverage.Status != "partial" {
		t.Fatalf("unexpected generic semantics snapshot: %+v", snapshot)
	}
	if len(snapshot.Coverage.Missing) == 0 {
		t.Fatal("future protocol must explain unsupported effective semantics")
	}
}
