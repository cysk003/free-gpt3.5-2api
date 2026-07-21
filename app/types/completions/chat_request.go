package completions

import (
	"fmt"
	"strings"

	"chat2api/app/types/chat"

	"github.com/google/uuid"
)

func BuildChatRequest(apiReq *ApiReq) *chat.Request {
	messages := make([]chat.Message, 0, len(apiReq.Messages)+4)
	systemHints := make([]string, 0)
	if hint := responseFormatHint(apiReq.ResponseFormat); hint != "" {
		systemHints = append(systemHints, hint)
	}
	if hint := stopHint(apiReq.Stop); hint != "" {
		systemHints = append(systemHints, hint)
	}
	if hint := maxTokenHint(apiReq); hint != "" {
		systemHints = append(systemHints, hint)
	}
	if len(systemHints) > 0 {
		messages = append(messages, chat.Message{
			Id:      uuid.New().String(),
			Author:  chat.Author{Role: "system"},
			Content: chat.Content{ContentType: "text", Parts: []interface{}{strings.Join(systemHints, "\n\n")}},
		})
	}
	for _, apiMessage := range apiReq.Messages {
		content := chatContentFromOpenAI(apiMessage.Content)
		messages = append(messages, chat.Message{
			Id: uuid.New().String(),
			Author: chat.Author{
				Role: apiMessage.Role,
			},
			Content: content,
		})
	}
	parentMessageId := strings.TrimSpace(apiReq.ParentMessageId)
	if parentMessageId == "" {
		parentMessageId = uuid.New().String()
	}

	return &chat.Request{
		Action:                     "next",
		Messages:                   messages,
		ConversationId:             strings.TrimSpace(apiReq.ConversationId),
		ParentMessageId:            parentMessageId,
		Model:                      normalizeModel(apiReq.Model),
		Timezone:                   "Asia/Shanghai",
		TimeZoneOffsetMin:          -480,
		Suggestions:                make([]string, 0),
		SupportedEncodings:         make([]string, 0),
		SystemHints:                make([]string, 0),
		HistoryAndTrainingDisabled: true,
		ForceUseSse:                true,
		FaceUseSse:                 false,
		ForceParagen:               false,
		ForceParagenModelSlug:      "",
		ForceRateLimit:             false,
		ResetRateLimits:            false,
		VariantPurpose:             "comparison_implicit",
		ConversationMode: chat.ConversationMode{
			Kind: "primary_assistant",
		},
		WebsocketRequestId: uuid.New().String(),
		ClientContextualInfo: chat.ClientContextualInfo{
			IsDarkMode:      false,
			TimeSinceLoaded: 120,
			PageHeight:      900,
			PageWidth:       1400,
			PixelRatio:      2,
			ScreenHeight:    1440,
			ScreenWidth:     2560,
		},
		ThinkingEffort: normalizeThinkingEffort(apiReq.ReasoningEffort),
	}
}

func responseFormatHint(format *ResponseFormat) string {
	if format == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(format.Type)) {
	case "json_object":
		return "You must respond in valid JSON format only. Do not include any text outside the JSON object."
	case "json_schema":
		return "You must respond in valid JSON format only, following the specified schema."
	default:
		return ""
	}
}

func stopHint(stop *StopParam) string {
	if stop == nil || len(stop.Values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(stop.Values))
	for _, value := range stop.Values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%q", value))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Stop generating when you reach any of these sequences: " + strings.Join(parts, " or ") + "."
}

func maxTokenHint(apiReq *ApiReq) string {
	if apiReq.MaxCompletionTokens != nil && *apiReq.MaxCompletionTokens > 0 {
		return fmt.Sprintf("Limit your response to at most %d tokens.", *apiReq.MaxCompletionTokens)
	}
	if apiReq.MaxTokens != nil && *apiReq.MaxTokens > 0 {
		return fmt.Sprintf("Limit your response to at most %d tokens.", *apiReq.MaxTokens)
	}
	return ""
}

func normalizeThinkingEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low"
	case "high":
		return "high"
	case "medium", "standard", "":
		return "standard"
	default:
		return "standard"
	}
}

func chatContentFromOpenAI(content interface{}) chat.Content {
	textParts := make([]string, 0)
	imageParts := make([]interface{}, 0)
	collectOpenAIContent(content, &textParts, &imageParts)
	text := strings.TrimSpace(strings.Join(textParts, ""))
	if len(imageParts) == 0 {
		return chat.Content{ContentType: "text", Parts: []interface{}{text}}
	}
	parts := make([]interface{}, 0, len(imageParts)+1)
	parts = append(parts, imageParts...)
	if text != "" {
		parts = append(parts, text)
	}
	return chat.Content{ContentType: "multimodal_text", Parts: parts}
}

func collectOpenAIContent(value interface{}, textParts *[]string, imageParts *[]interface{}) {
	switch v := value.(type) {
	case string:
		*textParts = append(*textParts, v)
	case []interface{}:
		for _, item := range v {
			collectOpenAIContent(item, textParts, imageParts)
		}
	case map[string]interface{}:
		partType := strings.TrimSpace(stringValue(v["type"]))
		switch partType {
		case "text", "input_text", "output_text":
			*textParts = append(*textParts, stringValue(v["text"]))
		case "image_url", "input_image", "image":
			if image := imageValue(v); image != "" {
				*imageParts = append(*imageParts, map[string]interface{}{"type": "input_image", "image_url": image})
			}
		default:
			if content, ok := v["content"]; ok {
				collectOpenAIContent(content, textParts, imageParts)
			}
		}
	}
}

func stringValue(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func imageValue(item map[string]interface{}) string {
	for _, key := range []string{"image_url", "url", "base64", "b64_json"} {
		value, ok := item[key]
		if !ok {
			continue
		}
		if text := stringValue(value); text != "" {
			return strings.TrimSpace(text)
		}
		if obj, ok := value.(map[string]interface{}); ok {
			for _, nested := range []string{"url", "image_url", "base64", "b64_json"} {
				if text := stringValue(obj[nested]); text != "" {
					return strings.TrimSpace(text)
				}
			}
		}
	}
	if source, ok := item["source"].(map[string]interface{}); ok && stringValue(source["type"]) == "base64" {
		return strings.TrimSpace(stringValue(source["data"]))
	}
	return ""
}

func normalizeModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	return model
}
