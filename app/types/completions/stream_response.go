package completions

import (
	"encoding/json"
)

type StreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ApiRespStream struct {
	ID             string            `json:"id,omitempty"`
	Object         string            `json:"object,omitempty"`
	Created        int64             `json:"created,omitempty"`
	Model          string            `json:"model,omitempty"`
	ConversationId string            `json:"conversation_id,omitempty"`
	MessageId      string            `json:"message_id,omitempty"`
	Choices        []ApiStreamChoice `json:"choices,omitempty"`
	Usage          *StreamUsage      `json:"usage,omitempty"`
}

type ApiStreamChoice struct {
	Delta        ApiStreamDelta `json:"delta,omitempty"`
	Index        int            `json:"index,omitempty"`
	FinishReason interface{}    `json:"finish_reason,omitempty"`
}

type ApiStreamDelta struct {
	Content            *string    `json:"content,omitempty"`
	ReasoningContent   string     `json:"reasoning_content,omitempty"`
	Role               string     `json:"role,omitempty"`
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`
	IncludeNullContent bool       `json:"-"`
}

func (d ApiStreamDelta) MarshalJSON() ([]byte, error) {
	out := make(map[string]interface{})
	if d.Role != "" {
		out["role"] = d.Role
	}
	if d.Content != nil {
		out["content"] = *d.Content
	} else if d.IncludeNullContent {
		out["content"] = nil
	}
	if d.ReasoningContent != "" {
		out["reasoning_content"] = d.ReasoningContent
	}
	if len(d.ToolCalls) > 0 {
		out["tool_calls"] = d.ToolCalls
	}
	return json.Marshal(out)
}

func (ARS *ApiRespStream) String() string {
	resp, _ := json.Marshal(ARS)
	return string(resp)
}
