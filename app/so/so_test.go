package so

import "testing"

func TestBuildToken(t *testing.T) {
	tok, err := BuildToken("so-result", "chat-token", "device", "flow")
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("expected non-empty token")
	}
	empty, err := BuildToken("", "c", "id", "flow")
	if err != nil {
		t.Fatal(err)
	}
	if empty != "" {
		t.Fatalf("expected empty token for empty so result, got %q", empty)
	}
}
