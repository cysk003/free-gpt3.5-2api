package chatgpt_backend

import (
	"strings"
	"testing"

	"chat2api/app/common"
	"chat2api/app/fingerprint"
)

func TestFixedUAAndTLS(t *testing.T) {
	if !strings.Contains(common.GetUa(), "Chrome/148") {
		t.Fatalf("unexpected ua: %s", common.GetUa())
	}
	if common.ClientBuildNumber != "7823760" {
		t.Fatalf("unexpected build number: %s", common.ClientBuildNumber)
	}
}

func TestLegacyRequirementsTokenShape(t *testing.T) {
	tok := LegacyRequirementsToken(common.GetUa(), "device-1")
	if !strings.HasPrefix(tok, "gAAAAAC") || !strings.HasSuffix(tok, "~S") {
		t.Fatalf("unexpected requirements token shape: %s", tok)
	}
}

func TestRequirementsTokenNonce2(t *testing.T) {
	tok := RequirementsTokenNonce2(common.GetUa(), "device-2")
	if !strings.HasPrefix(tok, "gAAAAAC") {
		t.Fatalf("unexpected req token: %s", tok)
	}
}

func TestIdentityReuse(t *testing.T) {
	d1, s1 := resolveIdentity("Bearer abc.def")
	d2, s2 := resolveIdentity("Bearer abc.def")
	if d1 != d2 || s1 != s2 {
		t.Fatalf("identity not reused: %s/%s vs %s/%s", d1, s1, d2, s2)
	}
	d3, _ := resolveIdentity("Bearer other.token")
	if d3 == d1 {
		t.Fatalf("different auth should not share device id")
	}
}

func TestBuild25Length(t *testing.T) {
	cfg := fingerprint.Build25(fingerprint.DefaultOptions())
	if len(cfg) != 25 {
		t.Fatalf("expected 25 elements, got %d", len(cfg))
	}
}
