package completions

import (
	"log"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pkoukk/tiktoken-go"
)

var (
	tokenEncoderOnce sync.Once
	tokenEncoder     *tiktoken.Tiktoken
)

func getTokenEncoder() *tiktoken.Tiktoken {
	tokenEncoderOnce.Do(func() {
		// Align with aurora/util.CountToken: gpt-3.5-turbo encoding.
		enc, err := tiktoken.EncodingForModel("gpt-3.5-turbo")
		if err != nil {
			log.Printf("tiktoken.EncodingForModel error: %v", err)
			return
		}
		tokenEncoder = enc
	})
	return tokenEncoder
}

// CountTokens counts tokens with tiktoken (gpt-3.5-turbo encoding), aligned with aurora.
// Falls back to a rough rune-based estimate only when tokenizer init fails.
func CountTokens(text string) int {
	if text == "" {
		return 0
	}
	if enc := getTokenEncoder(); enc != nil {
		return len(enc.Encode(text, nil, nil))
	}
	return approximateTokens(text)
}

func approximateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	if runes <= 0 {
		return 0
	}
	tokens := (runes + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func CountMessagesTokens(messages []ApiMessage) int {
	// Keep OpenAI-style message overhead while using exact token counts per field.
	total := 0
	for _, message := range messages {
		total += 4 // per-message overhead
		total += CountTokens(contentToText(message.Content))
		if message.Name != "" {
			total += CountTokens(message.Name)
		}
		for _, call := range message.ToolCalls {
			total += CountTokens(call.Function.Name)
			total += CountTokens(call.Function.Arguments)
		}
	}
	if total == 0 {
		return 0
	}
	return total + 2
}
