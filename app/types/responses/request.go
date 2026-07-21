package responses

type ResponseFormat struct {
	Type       string                 `json:"type,omitempty"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
}

type ResponseText struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

type ApiReq struct {
	Model           string            `json:"model"`
	Input           interface{}       `json:"input"`
	Instructions    string            `json:"instructions"`
	Stream          bool              `json:"stream"`
	Tools           []Tool            `json:"tools"`
	ToolChoice      interface{}       `json:"tool_choice"`
	Temperature     *float64          `json:"temperature,omitempty"`
	TopP            *float64          `json:"top_p,omitempty"`
	MaxOutputTokens *int              `json:"max_output_tokens,omitempty"`
	Text            *ResponseText     `json:"text,omitempty"`
	Reasoning       *ReasoningConfig  `json:"reasoning,omitempty"`
	User            string            `json:"user,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Store           *bool             `json:"store,omitempty"`
}

type Tool struct {
	Type         string                 `json:"type"`
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
	Strict       *bool                  `json:"strict,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Action       string                 `json:"action,omitempty"`
	Size         string                 `json:"size,omitempty"`
	Quality      string                 `json:"quality,omitempty"`
	OutputFormat string                 `json:"output_format,omitempty"`
}
