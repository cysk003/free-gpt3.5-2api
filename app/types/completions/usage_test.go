package completions

import "testing"

func TestCountTokensUsesTiktoken(t *testing.T) {
	n := CountTokens("hello world")
	if n <= 0 {
		t.Fatalf("expected positive token count, got %d", n)
	}
	// gpt-3.5-turbo usually encodes "hello world" as 2 tokens.
	if n > 8 {
		t.Fatalf("unexpectedly large token count for short text: %d", n)
	}
}
