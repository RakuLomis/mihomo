package adapter

import (
	"strings"

	"github.com/metacubex/mihomo/adapter/outbound"
	"github.com/metacubex/mihomo/component/proxysemantics"
)

const semanticsImplementationVersion = 1

var supportedSSCiphers = stringSet(
	"none",
	"aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
	"chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
	"chacha8-ietf-poly1305", "xchacha8-ietf-poly1305", "rabbit128-poly1305",
	"aes-128-ccm", "aes-192-ccm", "aes-256-ccm",
	"aes-128-gcm-siv", "aes-256-gcm-siv", "aegis-128l", "aegis-256",
	"aez-384", "deoxys-ii-256-128", "lea-128-gcm", "lea-192-gcm", "lea-256-gcm",
	"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
	"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
	"rc4-md5", "chacha20-ietf", "xchacha20", "chacha20",
	"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm",
	"2022-blake3-chacha20-poly1305", "2022-blake3-chacha8-poly1305",
	"2022-blake3-aes-128-ccm", "2022-blake3-aes-256-ccm",
)

var supportedTrojanSecondaryCiphers = stringSet(
	"aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
	"chacha20-poly1305", "xchacha20-poly1305",
	"chacha8-poly1305", "xchacha8-poly1305",
	"aes-128-ccm", "aes-192-ccm", "aes-256-ccm",
	"rc4-md5", "aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
	"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
	"chacha20", "chacha20-ietf", "xchacha20",
)

var supportedVMessCiphers = stringSet("auto", "none", "aes-128-gcm", "chacha20-poly1305")
var supportedMuxProtocols = stringSet("h2mux", "smux", "yamux")
var supportedVLESSFlows = stringSet("none", "xtls-rprx-vision")
var supportedHysteria2Obfs = stringSet("none", "salamander")

func buildProxySemantics(proxyType string, mapping map[string]any, option any, mux *outbound.SingMuxOption) proxysemantics.Snapshot {
	return buildProxySemanticsWithGeneration(proxyType, mapping, option, mux, proxysemantics.NextConfigGeneration())
}

func NextProxyConfigGeneration() uint64 {
	return proxysemantics.NextConfigGeneration()
}

func buildProxySemanticsWithGeneration(proxyType string, mapping map[string]any, option any, mux *outbound.SingMuxOption, generation uint64) proxysemantics.Snapshot {
	builder := proxysemantics.NewBuilderWithGeneration(proxyType, semanticsImplementationVersion, generation)
	addBasicSemantics(builder, mapping, option)
	switch typed := option.(type) {
	case *outbound.ShadowSocksOption:
		addShadowSocksSemantics(builder, mapping, typed)
	case *outbound.VlessOption:
		addVLESSSemantics(builder, mapping, typed)
	case *outbound.Hysteria2Option:
		addHysteria2Semantics(builder, mapping, typed)
	case *outbound.TrojanOption:
		addTrojanSemantics(builder, mapping, typed)
	case *outbound.VmessOption:
		addVMessSemantics(builder, mapping, typed)
	case *outbound.AnyTLSOption:
		addAnyTLSSemantics(builder, mapping, typed)
	default:
		builder.Missing("effective", "protocol", "protocol semantics are not implemented")
	}
	addMuxSemantics(builder, mapping, mux)
	return builder.Build()
}

func configured(mapping map[string]any, key string, value any) proxysemantics.Field {
	if _, exists := mapping[key]; !exists {
		return proxysemantics.Unknown("not explicitly configured")
	}
	return proxysemantics.Known(value)
}

func nestedConfigured(mapping map[string]any, parent, key string, value any) proxysemantics.Field {
	nested, ok := mapping[parent].(map[string]any)
	if !ok {
		return proxysemantics.Unknown("parent object not explicitly configured")
	}
	if _, exists := nested[key]; !exists {
		return proxysemantics.Unknown("not explicitly configured")
	}
	return proxysemantics.Known(value)
}

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func allowlistedString(value string, allowed map[string]struct{}, unsupportedReason string) proxysemantics.Field {
	normalized := strings.ToLower(value)
	if _, ok := allowed[normalized]; ok {
		return proxysemantics.Known(normalized)
	}
	return proxysemantics.Unsupported(unsupportedReason)
}

func configuredAllowlistedString(mapping map[string]any, key, value string, allowed map[string]struct{}, unsupportedReason string) proxysemantics.Field {
	if _, exists := mapping[key]; !exists {
		return proxysemantics.Unknown("not explicitly configured")
	}
	return allowlistedString(value, allowed, unsupportedReason)
}

func nestedConfiguredAllowlistedString(mapping map[string]any, parent, key, value string, allowed map[string]struct{}, unsupportedReason string) proxysemantics.Field {
	nested, ok := mapping[parent].(map[string]any)
	if !ok {
		return proxysemantics.Unknown("parent object not explicitly configured")
	}
	if _, exists := nested[key]; !exists {
		return proxysemantics.Unknown("not explicitly configured")
	}
	return allowlistedString(value, allowed, unsupportedReason)
}

func recordUnsupported(builder *proxysemantics.Builder, layer, field string, value proxysemantics.Field) {
	if value.State == proxysemantics.StateUnsupported {
		builder.Missing(layer, field, value.Reason)
	}
}

func addBasicSemantics(builder *proxysemantics.Builder, mapping map[string]any, option any) {
	var basic outbound.BasicOption
	switch typed := option.(type) {
	case *outbound.ShadowSocksOption:
		basic = typed.BasicOption
	case *outbound.VlessOption:
		basic = typed.BasicOption
	case *outbound.Hysteria2Option:
		basic = typed.BasicOption
	case *outbound.TrojanOption:
		basic = typed.BasicOption
	case *outbound.VmessOption:
		basic = typed.BasicOption
	case *outbound.AnyTLSOption:
		basic = typed.BasicOption
	default:
		return
	}
	builder.Configured("tcp_fast_open", configured(mapping, "tfo", basic.TFO))
	builder.Configured("multipath_tcp", configured(mapping, "mptcp", basic.MPTCP))
	builder.Configured("ip_version", configured(mapping, "ip-version", normalizedIPVersion(basic.IPVersion)))
	builder.Configured("dialer_proxy_enabled", configured(mapping, "dialer-proxy", basic.DialerProxy != ""))
	builder.Effective("tcp_fast_open", proxysemantics.Known(basic.TFO))
	builder.Effective("multipath_tcp", proxysemantics.Known(basic.MPTCP))
	builder.Effective("ip_version", proxysemantics.Known(normalizedIPVersion(basic.IPVersion)))
	builder.Effective("dialer_proxy_enabled", proxysemantics.Known(basic.DialerProxy != ""))
}

func normalizedIPVersion(value string) string {
	switch strings.ToLower(value) {
	case "ipv4", "ipv6", "ipv4-prefer", "ipv6-prefer":
		return strings.ToLower(value)
	default:
		return "dual"
	}
}

func addShadowSocksSemantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.ShadowSocksOption) {
	plugin := strings.ToLower(option.Plugin)
	cipherReason := "cipher is outside the supported semantics schema"
	configuredCipher := configuredAllowlistedString(mapping, "cipher", option.Cipher, supportedSSCiphers, cipherReason)
	builder.Configured("cipher", configuredCipher)
	recordUnsupported(builder, "configured", "cipher", configuredCipher)
	if isSupportedSSPlugin(plugin) {
		builder.Configured("plugin", configured(mapping, "plugin", plugin))
	} else {
		builder.Configured("plugin", proxysemantics.Unsupported("plugin is outside the supported semantics schema"))
		builder.Missing("configured", "plugin", "unsupported plugin")
	}
	builder.Configured("udp", configured(mapping, "udp", option.UDP))
	builder.Configured("udp_over_tcp", configured(mapping, "udp-over-tcp", option.UDPOverTCP))
	builder.Configured("udp_over_tcp_version", configured(mapping, "udp-over-tcp-version", option.UDPOverTCPVersion))
	effectiveCipher := allowlistedString(option.Cipher, supportedSSCiphers, cipherReason)
	builder.Effective("cipher", effectiveCipher)
	recordUnsupported(builder, "effective", "cipher", effectiveCipher)
	builder.Effective("plugin", proxysemantics.Known(pluginOrNone(plugin)))
	builder.Effective("udp", proxysemantics.Known(option.UDP))
	builder.Effective("udp_over_tcp", proxysemantics.Known(option.UDPOverTCP))
	uotVersion := option.UDPOverTCPVersion
	if uotVersion == 0 {
		uotVersion = 1
	}
	builder.Effective("udp_over_tcp_version", proxysemantics.Known(uotVersion))

	pluginOpts, _ := mapping["plugin-opts"].(map[string]any)
	switch plugin {
	case "":
		builder.Effective("plugin_mode", proxysemantics.NotApplicable("no plugin"))
		builder.Effective("plugin_tls", proxysemantics.NotApplicable("no plugin"))
		builder.Effective("plugin_mux", proxysemantics.NotApplicable("no plugin"))
	case "obfs":
		mode, _ := pluginOpts["mode"].(string)
		allowed := stringSet("http", "tls")
		configuredMode := nestedConfiguredAllowlistedString(mapping, "plugin-opts", "mode", mode, allowed, "plugin mode is unsupported")
		effectiveMode := allowlistedString(mode, allowed, "plugin mode is unsupported")
		builder.Configured("plugin_mode", configuredMode)
		builder.Effective("plugin_mode", effectiveMode)
		recordUnsupported(builder, "configured", "plugin_mode", configuredMode)
		recordUnsupported(builder, "effective", "plugin_mode", effectiveMode)
		builder.Effective("plugin_tls", proxysemantics.NotApplicable("simple-obfs mode"))
		builder.Effective("plugin_mux", proxysemantics.NotApplicable("simple-obfs mode"))
	case "v2ray-plugin", "gost-plugin":
		mode, _ := pluginOpts["mode"].(string)
		allowed := stringSet("websocket")
		tlsEnabled, _ := pluginOpts["tls"].(bool)
		muxEnabled := true
		if value, ok := pluginOpts["mux"].(bool); ok {
			muxEnabled = value
		}
		configuredMode := nestedConfiguredAllowlistedString(mapping, "plugin-opts", "mode", mode, allowed, "plugin mode is unsupported")
		effectiveMode := allowlistedString(mode, allowed, "plugin mode is unsupported")
		builder.Configured("plugin_mode", configuredMode)
		builder.Configured("plugin_tls", nestedConfigured(mapping, "plugin-opts", "tls", tlsEnabled))
		builder.Configured("plugin_mux", nestedConfigured(mapping, "plugin-opts", "mux", muxEnabled))
		builder.Effective("plugin_mode", effectiveMode)
		recordUnsupported(builder, "configured", "plugin_mode", configuredMode)
		recordUnsupported(builder, "effective", "plugin_mode", effectiveMode)
		builder.Effective("plugin_tls", proxysemantics.Known(tlsEnabled))
		builder.Effective("plugin_mux", proxysemantics.Known(muxEnabled))
	case "shadow-tls":
		version := 2
		if value, ok := numberAsInt(pluginOpts["version"]); ok {
			version = value
		}
		builder.Configured("plugin_version", nestedConfigured(mapping, "plugin-opts", "version", version))
		builder.Effective("plugin_version", proxysemantics.Known(version))
		builder.Effective("plugin_tls", proxysemantics.Known(true))
		builder.Effective("plugin_mux", proxysemantics.NotApplicable("shadow-tls plugin"))
	case "restls":
		builder.Effective("plugin_tls", proxysemantics.Known(true))
		builder.Effective("plugin_mux", proxysemantics.NotApplicable("restls plugin"))
	default:
		builder.Effective("plugin", proxysemantics.Unsupported("plugin is outside the supported semantics schema"))
		builder.Missing("effective", "plugin", "unsupported plugin")
	}
}

func pluginOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func isSupportedSSPlugin(value string) bool {
	return value == "" || value == "obfs" || value == "v2ray-plugin" || value == "gost-plugin" ||
		value == "shadow-tls" || value == "restls"
}

func addVLESSSemantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.VlessOption) {
	network := normalizedTransport(option.Network)
	flow := flowOrNone(strings.ToLower(option.Flow))
	reality := option.RealityOpts.PublicKey != ""
	builder.Configured("transport", configured(mapping, "network", network))
	builder.Configured("tls", configured(mapping, "tls", option.TLS))
	builder.Configured("reality", configured(mapping, "reality-opts", reality))
	configuredFlow := configuredAllowlistedString(mapping, "flow", flow, supportedVLESSFlows, "flow is outside the supported semantics schema")
	effectiveFlow := allowlistedString(flow, supportedVLESSFlows, "flow is outside the supported semantics schema")
	builder.Configured("flow", configuredFlow)
	builder.Configured("udp", configured(mapping, "udp", option.UDP))
	builder.Configured("packet_encoding", configured(mapping, "packet-encoding", packetEncoding(option)))
	builder.Effective("transport", proxysemantics.Known(network))
	builder.Effective("tls", proxysemantics.Known(option.TLS))
	builder.Effective("reality", proxysemantics.Known(reality))
	builder.Effective("flow", effectiveFlow)
	recordUnsupported(builder, "configured", "flow", configuredFlow)
	recordUnsupported(builder, "effective", "flow", effectiveFlow)
	builder.Effective("vision", proxysemantics.Known(flow == "xtls-rprx-vision"))
	builder.Effective("udp", proxysemantics.Known(option.UDP))
	builder.Effective("packet_encoding", proxysemantics.Known(packetEncoding(option)))
}

func addHysteria2Semantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.Hysteria2Option) {
	obfs := pluginOrNone(strings.ToLower(option.Obfs))
	configuredObfs := configuredAllowlistedString(mapping, "obfs", obfs, supportedHysteria2Obfs, "obfs is outside the supported semantics schema")
	effectiveObfs := allowlistedString(obfs, supportedHysteria2Obfs, "obfs is outside the supported semantics schema")
	builder.Configured("obfs", configuredObfs)
	builder.Configured("port_hopping", configured(mapping, "ports", option.Ports != ""))
	builder.Configured("hop_interval_seconds", configured(mapping, "hop-interval", option.HopInterval))
	builder.Configured("udp_mtu", configured(mapping, "udp-mtu", option.UdpMTU))
	if alpn, ok := sanitizedALPN(option.ALPN); ok {
		builder.Configured("alpn", configured(mapping, "alpn", alpn))
		builder.Effective("alpn", proxysemantics.Known(alpn))
	} else {
		builder.Configured("alpn", proxysemantics.Unsupported("ALPN contains an unsupported identifier"))
		builder.Effective("alpn", proxysemantics.Unsupported("ALPN contains an unsupported identifier"))
		builder.Missing("effective", "alpn", "unsupported ALPN identifier")
	}
	builder.Effective("transport", proxysemantics.Known("quic"))
	builder.Effective("tls", proxysemantics.Known(true))
	builder.Effective("obfs", effectiveObfs)
	recordUnsupported(builder, "configured", "obfs", configuredObfs)
	recordUnsupported(builder, "effective", "obfs", effectiveObfs)
	builder.Effective("port_hopping", proxysemantics.Known(option.Ports != ""))
	hopInterval := option.HopInterval
	if option.Ports != "" && hopInterval == 0 {
		hopInterval = 30
	}
	builder.Effective("hop_interval_seconds", proxysemantics.Known(hopInterval))
	udpMTU := option.UdpMTU
	if udpMTU == 0 {
		udpMTU = 1197
	}
	builder.Effective("udp_mtu", proxysemantics.Known(udpMTU))
}

func addTrojanSemantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.TrojanOption) {
	network := normalizedTransport(option.Network)
	reality := option.RealityOpts.PublicKey != ""
	builder.Configured("transport", configured(mapping, "network", network))
	builder.Configured("tls", proxysemantics.Known(true))
	builder.Configured("reality", configured(mapping, "reality-opts", reality))
	builder.Configured("udp", configured(mapping, "udp", option.UDP))
	builder.Configured("secondary_shadowsocks", configured(mapping, "ss-opts", option.SSOpts.Enabled))
	builder.Effective("transport", proxysemantics.Known(network))
	builder.Effective("tls", proxysemantics.Known(true))
	builder.Effective("reality", proxysemantics.Known(reality))
	builder.Effective("udp", proxysemantics.Known(option.UDP))
	builder.Effective("secondary_shadowsocks", proxysemantics.Known(option.SSOpts.Enabled))
	if option.SSOpts.Enabled {
		secondaryCipher := allowlistedString(option.SSOpts.Method, supportedTrojanSecondaryCiphers, "secondary cipher is outside the supported semantics schema")
		builder.Effective("secondary_cipher", secondaryCipher)
		recordUnsupported(builder, "effective", "secondary_cipher", secondaryCipher)
	} else {
		builder.Effective("secondary_cipher", proxysemantics.NotApplicable("secondary Shadowsocks disabled"))
	}
}

func addVMessSemantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.VmessOption) {
	network := normalizedTransport(option.Network)
	reality := option.RealityOpts.PublicKey != ""
	configuredCipher := configuredAllowlistedString(mapping, "cipher", option.Cipher, supportedVMessCiphers, "cipher is outside the supported semantics schema")
	effectiveCipher := allowlistedString(option.Cipher, supportedVMessCiphers, "cipher is outside the supported semantics schema")
	builder.Configured("cipher", configuredCipher)
	builder.Configured("transport", configured(mapping, "network", network))
	builder.Configured("tls", configured(mapping, "tls", option.TLS))
	builder.Configured("reality", configured(mapping, "reality-opts", reality))
	builder.Configured("udp", configured(mapping, "udp", option.UDP))
	builder.Effective("cipher", effectiveCipher)
	recordUnsupported(builder, "configured", "cipher", configuredCipher)
	recordUnsupported(builder, "effective", "cipher", effectiveCipher)
	builder.Effective("transport", proxysemantics.Known(network))
	builder.Effective("tls", proxysemantics.Known(option.TLS))
	builder.Effective("reality", proxysemantics.Known(reality))
	builder.Effective("udp", proxysemantics.Known(option.UDP))
	builder.Effective("global_padding", proxysemantics.Known(option.GlobalPadding))
	builder.Effective("authenticated_length", proxysemantics.Known(option.AuthenticatedLength))
}

func addAnyTLSSemantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.AnyTLSOption) {
	builder.Configured("tls", proxysemantics.Known(true))
	builder.Configured("udp", configured(mapping, "udp", option.UDP))
	builder.Configured("idle_session_check_interval_seconds", configured(mapping, "idle-session-check-interval", option.IdleSessionCheckInterval))
	builder.Configured("idle_session_timeout_seconds", configured(mapping, "idle-session-timeout", option.IdleSessionTimeout))
	builder.Configured("minimum_idle_sessions", configured(mapping, "min-idle-session", option.MinIdleSession))
	builder.Effective("transport", proxysemantics.Known("tls"))
	builder.Effective("tls", proxysemantics.Known(true))
	builder.Effective("udp", proxysemantics.Known(option.UDP))
	builder.Effective("idle_session_check_interval_seconds", proxysemantics.Known(option.IdleSessionCheckInterval))
	builder.Effective("idle_session_timeout_seconds", proxysemantics.Known(option.IdleSessionTimeout))
	builder.Effective("minimum_idle_sessions", proxysemantics.Known(option.MinIdleSession))
}

func addMuxSemantics(builder *proxysemantics.Builder, mapping map[string]any, option *outbound.SingMuxOption) {
	if option == nil {
		builder.Configured("smux_enabled", proxysemantics.Unknown("not explicitly configured"))
		builder.Effective("smux_enabled", proxysemantics.Known(false))
		builder.Effective("smux_protocol", proxysemantics.NotApplicable("smux disabled"))
		return
	}
	builder.Configured("smux_enabled", nestedConfigured(mapping, "smux", "enabled", option.Enabled))
	configuredProtocol := nestedConfiguredAllowlistedString(mapping, "smux", "protocol", muxProtocolOrDefault(option.Protocol), supportedMuxProtocols, "mux protocol is outside the supported semantics schema")
	builder.Configured("smux_protocol", configuredProtocol)
	recordUnsupported(builder, "configured", "smux_protocol", configuredProtocol)
	builder.Configured("smux_max_connections", nestedConfigured(mapping, "smux", "max-connections", option.MaxConnections))
	builder.Configured("smux_min_streams", nestedConfigured(mapping, "smux", "min-streams", option.MinStreams))
	builder.Configured("smux_max_streams", nestedConfigured(mapping, "smux", "max-streams", option.MaxStreams))
	builder.Configured("smux_padding", nestedConfigured(mapping, "smux", "padding", option.Padding))
	builder.Configured("smux_only_tcp", nestedConfigured(mapping, "smux", "only-tcp", option.OnlyTcp))
	builder.Effective("smux_enabled", proxysemantics.Known(option.Enabled))
	if option.Enabled {
		effectiveProtocol := allowlistedString(muxProtocolOrDefault(option.Protocol), supportedMuxProtocols, "mux protocol is outside the supported semantics schema")
		builder.Effective("smux_protocol", effectiveProtocol)
		recordUnsupported(builder, "effective", "smux_protocol", effectiveProtocol)
		builder.Effective("smux_max_connections", proxysemantics.Known(option.MaxConnections))
		builder.Effective("smux_min_streams", proxysemantics.Known(option.MinStreams))
		builder.Effective("smux_max_streams", proxysemantics.Known(option.MaxStreams))
		builder.Effective("smux_padding", proxysemantics.Known(option.Padding))
		builder.Effective("smux_only_tcp", proxysemantics.Known(option.OnlyTcp))
	} else {
		builder.Effective("smux_protocol", proxysemantics.NotApplicable("smux disabled"))
	}
}

func muxProtocolOrDefault(value string) string {
	if value == "" {
		return "h2mux"
	}
	return strings.ToLower(value)
}

func normalizedTransport(value string) string {
	switch strings.ToLower(value) {
	case "", "tcp":
		return "tcp"
	case "ws", "http", "h2", "grpc":
		return strings.ToLower(value)
	default:
		return "tcp"
	}
}

func sanitizedALPN(values []string) ([]string, bool) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(value)
		switch normalized {
		case "h2", "http/1.1", "h3":
			result = append(result, normalized)
		default:
			return nil, false
		}
	}
	return result, true
}
func flowOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func packetEncoding(option *outbound.VlessOption) string {
	switch strings.ToLower(option.PacketEncoding) {
	case "packetaddr", "packet":
		return "packetaddr"
	default:
		if option.PacketAddr {
			return "packetaddr"
		}
		return "xudp"
	}
}

func numberAsInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}
