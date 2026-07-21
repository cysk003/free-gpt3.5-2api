package completions

import (
	"encoding/json"
	"fmt"
)

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolChoice struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func (f *ToolCallFunction) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name      string      `json:"name"`
		Arguments interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Name = raw.Name
	f.Arguments = normalizeFunctionArguments(raw.Arguments)
	return nil
}

type ToolCall struct {
	Index    *int             `json:"index,omitempty"`
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func (f *FunctionCall) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name      string      `json:"name"`
		Arguments interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Name = raw.Name
	f.Arguments = normalizeFunctionArguments(raw.Arguments)
	return nil
}

func normalizeFunctionArguments(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

type ResponseFormat struct {
	Type       string                 `json:"type,omitempty"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type StopParam struct {
	Values []string
}

func (s *StopParam) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		s.Values = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(data, &multi); err == nil {
		s.Values = multi
		return nil
	}
	return fmt.Errorf("stop must be a string or array of strings")
}

type ApiReq struct {
	Messages            []ApiMessage      `json:"messages"`
	Model               string            `json:"model"`
	Stream              bool              `json:"stream"`
	Tools               []Tool            `json:"tools,omitempty"`
	ToolChoice          interface{}       `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool             `json:"parallel_tool_calls,omitempty"`
	Functions           []ToolFunction    `json:"functions,omitempty"`
	FunctionCall        interface{}       `json:"function_call,omitempty"`
	PluginIds           []string          `json:"plugin_ids,omitempty"`
	ConversationId      string            `json:"conversation_id,omitempty"`
	ParentMessageId     string            `json:"parent_message_id,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	N                   *int              `json:"n,omitempty"`
	Stop                *StopParam        `json:"stop,omitempty"`
	MaxTokens           *int              `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	PresencePenalty     *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64          `json:"frequency_penalty,omitempty"`
	LogitBias           map[int]int       `json:"logit_bias,omitempty"`
	Seed                *int              `json:"seed,omitempty"`
	ResponseFormat      *ResponseFormat   `json:"response_format,omitempty"`
	ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
	StreamOptions       *StreamOptions    `json:"stream_options,omitempty"`
	User                string            `json:"user,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Store               *bool             `json:"store,omitempty"`
	NewMessages         string            `json:"-"`
	HasToolResults      bool              `json:"-"`
}

type ApiMessage struct {
	Role         string        `json:"role"`
	Content      interface{}   `json:"content,omitempty"`
	Name         string        `json:"name,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
}
