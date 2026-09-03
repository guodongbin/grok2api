package egress

import "testing"

func TestSupportsScopePreservesPrimaryAndResourceCompatibility(t *testing.T) {
	tests := []struct {
		name               string
		nodeScope, request Scope
		want               bool
	}{
		{name: "exact Console asset", nodeScope: ScopeConsoleAsset, request: ScopeConsoleAsset, want: true},
		{name: "Console serves Console asset", nodeScope: ScopeConsole, request: ScopeConsoleAsset, want: true},
		{name: "Web serves Console asset", nodeScope: ScopeWeb, request: ScopeConsoleAsset, want: true},
		{name: "Web serves Web asset", nodeScope: ScopeWeb, request: ScopeWebAsset, want: true},
		{name: "Console asset does not serve primary Console", nodeScope: ScopeConsoleAsset, request: ScopeConsole, want: false},
		{name: "Web asset does not serve primary Web", nodeScope: ScopeWebAsset, request: ScopeWeb, want: false},
		{name: "Build remains isolated", nodeScope: ScopeBuild, request: ScopeConsoleAsset, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SupportsScope(test.nodeScope, test.request); got != test.want {
				t.Fatalf("SupportsScope(%q, %q) = %v, want %v", test.nodeScope, test.request, got, test.want)
			}
		})
	}
}

func TestProbeStatusIsValid(t *testing.T) {
	valid := []ProbeStatus{ProbeStatusUnknown, ProbeStatusHealthy, ProbeStatusUnhealthy}
	for _, value := range valid {
		if !value.IsValid() {
			t.Fatalf("ProbeStatus(%q).IsValid() = false", value)
		}
	}
	for _, value := range []ProbeStatus{"up", "down", ""} {
		if value.IsValid() {
			t.Fatalf("ProbeStatus(%q).IsValid() = true", value)
		}
	}
}

func TestFallbackModeValidationAndNormalization(t *testing.T) {
	for _, value := range []FallbackMode{FallbackModeNone, FallbackModeDirect, FallbackModeFixed} {
		if !value.IsValid() {
			t.Fatalf("FallbackMode(%q).IsValid() = false", value)
		}
	}
	if FallbackMode("random").IsValid() {
		t.Fatalf("invalid FallbackMode reported valid")
	}
	tests := []struct {
		name  string
		value FallbackMode
		want  FallbackMode
	}{
		{name: "zero value maps to fail-closed none", value: "", want: FallbackModeNone},
		{name: "explicit none preserved", value: FallbackModeNone, want: FallbackModeNone},
		{name: "direct preserved", value: FallbackModeDirect, want: FallbackModeDirect},
		{name: "fixed preserved", value: FallbackModeFixed, want: FallbackModeFixed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.value.Normalized(); got != test.want {
				t.Fatalf("Normalized() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFallbackForAlwaysReturnsCanonicalSafeValue(t *testing.T) {
	config := OperationsConfig{Fallbacks: map[Scope]FallbackConfig{
		ScopeWeb:     {Mode: ""},
		ScopeBuild:   {Mode: FallbackModeFixed, NodeID: 7},
		ScopeConsole: {Mode: FallbackModeDirect, NodeID: 9},
	}}
	tests := []struct {
		name  string
		scope Scope
		want  FallbackConfig
	}{
		{name: "empty mode normalized to none", scope: ScopeWeb, want: FallbackConfig{Mode: FallbackModeNone}},
		{name: "fixed keeps node id", scope: ScopeBuild, want: FallbackConfig{Mode: FallbackModeFixed, NodeID: 7}},
		{name: "direct drops stale node id", scope: ScopeConsole, want: FallbackConfig{Mode: FallbackModeDirect}},
		{name: "missing scope fails closed", scope: ScopeWebAsset, want: FallbackConfig{Mode: FallbackModeNone}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := config.FallbackFor(test.scope); got != test.want {
				t.Fatalf("FallbackFor(%q) = %+v, want %+v", test.scope, got, test.want)
			}
		})
	}
}

func TestDefaultOperationsConfigIsConservative(t *testing.T) {
	config := DefaultOperationsConfig()
	if config.ProbeProvider != ProbeProviderCloudflare {
		t.Fatalf("ProbeProvider = %q, want cloudflare", config.ProbeProvider)
	}
	if config.ProbeIntervalSeconds != 900 || config.AssignmentIntervalSeconds != 300 {
		t.Fatalf("intervals = probe %d / assignment %d", config.ProbeIntervalSeconds, config.AssignmentIntervalSeconds)
	}
	for _, scope := range []Scope{ScopeBuild, ScopeWeb, ScopeConsole, ScopeWebAsset, ScopeConsoleAsset} {
		if fallback := config.FallbackFor(scope); fallback.Mode != FallbackModeNone || fallback.NodeID != 0 {
			t.Fatalf("FallbackFor(%q) = %+v, want fail-closed none", scope, fallback)
		}
	}
}

func TestProbeProviderValidationAndNormalization(t *testing.T) {
	if !ProbeProviderIPInfo.IsValid() || !ProbeProviderCloudflare.IsValid() {
		t.Fatalf("known providers reported invalid")
	}
	if ProbeProvider("vanta").IsValid() {
		t.Fatalf("unknown provider reported valid")
	}
	if got := ProbeProvider("").Normalized(); got != ProbeProviderCloudflare {
		t.Fatalf("empty provider normalized to %q, want cloudflare", got)
	}
	if got := ProbeProvider(ProbeProviderIPInfo).Normalized(); got != ProbeProviderIPInfo {
		t.Fatalf("ipinfo provider normalized to %q", got)
	}
}

func TestSupportsScope(t *testing.T) {
	tests := []struct {
		name  string
		node  Scope
		scope Scope
		want  bool
	}{
		{name: "build node serves build", node: ScopeBuild, scope: ScopeBuild, want: true},
		{name: "build node no cross use", node: ScopeBuild, scope: ScopeWeb, want: false},
		{name: "web node serves web", node: ScopeWeb, scope: ScopeWeb, want: true},
		{name: "web node serves web asset", node: ScopeWeb, scope: ScopeWebAsset, want: true},
		{name: "web node serves console", node: ScopeWeb, scope: ScopeConsole, want: true},
		{name: "web node serves console asset", node: ScopeWeb, scope: ScopeConsoleAsset, want: true},
		{name: "console node serves console", node: ScopeConsole, scope: ScopeConsole, want: true},
		{name: "console node serves console asset", node: ScopeConsole, scope: ScopeConsoleAsset, want: true},
		{name: "console node does not serve web asset", node: ScopeConsole, scope: ScopeWebAsset, want: false},
		{name: "resource node never serves provider scope", node: ScopeWebAsset, scope: ScopeWeb, want: false},
		{name: "unknown scope denied", node: ScopeWeb, scope: Scope("grok_unknown"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := SupportsScope(test.node, test.scope); got != test.want {
				t.Fatalf("SupportsScope(%q, %q) = %v, want %v", test.node, test.scope, got, test.want)
			}
		})
	}
}
