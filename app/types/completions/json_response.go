package completions

import "time"

type ApiRespJson struct {
	ID             string              `json:"id,omitempty"`
	Object         string              `json:"object,omitempty"`
	Created        int64               `json:"created,omitempty"`
	Model          string              `json:"model,omitempty"`
	ConversationId string              `json:"conversation_id,omitempty"`
	MessageId      string              `json:"message_id,omitempty"`
	Usage          ApiRespJsonUsage    `json:"usage,omitempty"`
	Choices        []ApiRespJsonChoice `json:"choices,omitempty"`
}

type ApiRespJsonMessage struct {
	Role             string     `json:"role,omitempty"`
	Content          *string    `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

type ApiRespJsonChoice struct {
	Delta        ApiRespJsonChoiceDelta `json:"delta,omitempty"`
	Message      ApiRespJsonMessage     `json:"message,omitempty"`
	FinishReason string                 `json:"finish_reason,omitempty"`
	Index        int                    `json:"index,omitempty"`
}

type ApiRespJsonChoiceDelta struct {
	Content string `json:"content,omitempty"`
	Role    string `json:"role,omitempty"`
}

type ApiRespJsonUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewApiRespJson(id string, model string, content string) *ApiRespJson {
	return NewApiRespJsonWithReasoning(id, model, content, "", "stop")
}

func NewApiRespJsonWithReasoning(id string, model string, content string, reasoning string, finishReason string) *ApiRespJson {
	if finishReason == "" {
		finishReason = "stop"
	}
	contentPtr := content
	return &ApiRespJson{
		ID:      id,
		Created: time.Now().Unix(),
		Object:  "chat.completion",
		Model:   model,
		Usage: ApiRespJsonUsage{
			PromptTokens:     0,
			CompletionTokens: 0,
			TotalTokens:      0,
		},
		Choices: []ApiRespJsonChoice{
			{
				Message: ApiRespJsonMessage{
					Role:             "assistant",
					Content:          &contentPtr,
					ReasoningContent: reasoning,
				},
				FinishReason: finishReason,
				Index:        0,
			},
		},
	}
}

func (r *ApiRespJson) WithUsage(promptTokens, completionTokens int) *ApiRespJson {
	if r == nil {
		return r
	}
	r.Usage = ApiRespJsonUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	return r
}

func NewToolCallsApiRespJson(id string, model string, content string, toolCalls []ToolCall) *ApiRespJson {
	resp := NewApiRespJson(id, model, content)
	resp.Choices[0].Message.ToolCalls = toolCalls
	resp.Choices[0].FinishReason = "tool_calls"
	resp.Choices[0].Message.Content = nil
	return resp
}
