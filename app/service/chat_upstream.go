package service

import (
	"bytes"
	"chat2api/app/browserfp"
	"chat2api/app/chatgpt_backend"
	"chat2api/app/common"
	"chat2api/app/types/chat"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func sendChatRequest(c *gin.Context, chatReq *chat.Request) (*http.Response, *chatgpt_backend.Client, error) {
	backend, err := chatgpt_backend.New(c.Request.Header.Get("Authorization"), chatgpt_backend.Retry())
	if err != nil {
		return nil, nil, err
	}
	if err := prepareChatVisionInputs(backend, chatReq); err != nil {
		return nil, backend, err
	}
	applyChatTargetDefaults(backend, chatReq)
	response, err := sendChatRequestWithBackend(backend, chatReq)
	if err != nil {
		return nil, backend, err
	}
	return response, backend, nil
}

func sendChatRequestWithBackend(backend *chatgpt_backend.Client, chatReq *chat.Request) (*http.Response, error) {
	if backend == nil {
		return nil, fmt.Errorf("backend client is nil")
	}
	upstreamURL := backend.ChatURL
	conduitToken := ""
	turnTraceID := uuid.New().String()
	if shouldUseFConversation(backend) {
		upstreamURL = backend.BaseURL + "/backend-api/f/conversation"
		applyFConversationPayloadDefaults(backend, chatReq)
		var prepareErr error
		conduitToken, prepareErr = prepareFConversation(backend, chatReq, turnTraceID)
		if prepareErr != nil {
			return nil, prepareErr
		}
	}
	body, err := common.Struct2BytesBuffer(chatReq)
	if err != nil {
		return nil, err
	}
	headers, cookies := backend.Headers(upstreamURL)
	headers.Set("accept", "text/event-stream")
	headers.Set("content-type", "application/json")
	if shouldUseFConversation(backend) {
		headers.Set("oai-echo-logs", "0,943,1,65876,0,68124,1,68930")
		headers.Set("oai-telemetry", "[1,null]")
	}
	applySentinelHeaders(headers, backend, turnTraceID)
	if conduitToken != "" {
		headers.Set("x-conduit-token", conduitToken)
	}
	response, err := backend.HTTP.Request(tls_client_httpi.POST, upstreamURL, headers, cookies, body)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	return response, nil
}

func maxContinueCount() int {
	v := strings.TrimSpace(os.Getenv("MAX_CONTINUE_COUNT"))
	if v == "" {
		return 3
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 3
	}
	return n
}

func applyContinueRequest(chatReq *chat.Request, conversationID, parentMessageID string) {
	if chatReq == nil {
		return
	}
	chatReq.Action = "continue"
	chatReq.Messages = nil
	chatReq.ConversationId = conversationID
	chatReq.ParentMessageId = parentMessageID
}

func applyChatTargetDefaults(backend *chatgpt_backend.Client, chatReq *chat.Request) {
	timezone, offset := backend.ChatTimezone()
	chatReq.Timezone = timezone
	chatReq.TimeZoneOffsetMin = offset
	if chatReq != nil && chatReq.ConversationId != "" {
		backend.NoteConversation(chatReq.ConversationId)
	}
	applyClientContextualInfo(backend, chatReq)
}

func applyClientContextualInfo(backend *chatgpt_backend.Client, chatReq *chat.Request) {
	if chatReq == nil {
		return
	}
	info := chat.ClientContextualInfo{
		IsDarkMode:      false,
		TimeSinceLoaded: 10,
		PageHeight:      1014,
		PageWidth:       1055,
		PixelRatio:      1,
		ScreenHeight:    1080,
		ScreenWidth:     1920,
		AppName:         "chatgpt.com",
	}
	if backend != nil {
		if sec := backend.TimeSinceLoadedSeconds(); sec > 0 {
			info.TimeSinceLoaded = sec
		}
	}
	if fp := browserfp.Get(); fp != nil {
		if fp.ScreenHeight > 0 {
			info.ScreenHeight = fp.ScreenHeight
		}
		if fp.ScreenWidth > 0 {
			info.ScreenWidth = fp.ScreenWidth
		}
		if fp.DevicePixelRatio > 0 {
			info.PixelRatio = fp.DevicePixelRatio
		}
		info.PageHeight = fp.ScreenHeight - 66
		if info.PageHeight < 600 {
			info.PageHeight = 600
		}
		info.PageWidth = fp.ScreenWidth - 196
		if info.PageWidth < 800 {
			info.PageWidth = 800
		}
	}
	chatReq.ClientContextualInfo = info
}

func shouldUseFConversation(backend *chatgpt_backend.Client) bool {
	return backend.AccAuth != ""
}

func messageHasAssetPointer(message chat.Message) bool {
	if message.Content.ContentType != "multimodal_text" {
		return false
	}
	for _, part := range message.Content.Parts {
		item, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(responseStringValue(item["content_type"], "")) == "image_asset_pointer" {
			return true
		}
		if pointer := strings.TrimSpace(responseStringValue(item["asset_pointer"], "")); strings.HasPrefix(pointer, "file-service://") || strings.HasPrefix(pointer, "sediment://") {
			return true
		}
	}
	return false
}

func applyFConversationPayloadDefaults(backend *chatgpt_backend.Client, chatReq *chat.Request) {
	chatReq.ClientPrepareState = string(fConversationPrepareStateSuccess)
	chatReq.EnableMessageFollowups = true
	chatReq.SupportsBuffering = true
	chatReq.SupportedEncodings = []string{"v1"}
	chatReq.HistoryAndTrainingDisabled = false
	chatReq.ParagenCotSummaryDisplayOverride = "allow"
	chatReq.ForceParallelSwitch = "auto"
	if strings.TrimSpace(chatReq.ThinkingEffort) == "" {
		chatReq.ThinkingEffort = "standard"
	}
	applyClientContextualInfo(backend, chatReq)
}

type fConversationPrepareState string

const (
	fConversationPrepareStateNone    fConversationPrepareState = "none"
	fConversationPrepareStateSent    fConversationPrepareState = "sent"
	fConversationPrepareStateSuccess fConversationPrepareState = "success"
)

func prepareFConversation(backend *chatgpt_backend.Client, chatReq *chat.Request, turnTraceID string) (string, error) {
	if backend.AccAuth == "" {
		return "", nil
	}
	conduitToken := ""
	for _, state := range []fConversationPrepareState{
		fConversationPrepareStateNone,
		fConversationPrepareStateSent,
		fConversationPrepareStateSuccess,
	} {
		nextToken, err := prepareFConversationState(backend, chatReq, turnTraceID, state, conduitToken)
		if err != nil {
			return "", err
		}
		conduitToken = nextToken
	}
	return conduitToken, nil
}

func prepareFConversationState(backend *chatgpt_backend.Client, chatReq *chat.Request, turnTraceID string, prepareState fConversationPrepareState, previousConduitToken string) (string, error) {
	path := "/backend-api/f/conversation/prepare"
	parentMessageID := chatReq.ParentMessageId
	if parentMessageID == "" {
		parentMessageID = "client-created-root"
	}
	payload := map[string]interface{}{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     parentMessageID,
		"model":                 conversationPrepareModel(chatReq.Model),
		"client_prepare_state":  string(prepareState),
		"timezone_offset_min":   chatReq.TimeZoneOffsetMin,
		"timezone":              chatReq.Timezone,
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"system_hints":          []string{},
		"supports_buffering":    true,
		"supported_encodings":   []string{"v1"},
		"client_contextual_info": map[string]interface{}{
			"app_name": "chatgpt.com",
		},
		"thinking_effort": "standard",
	}
	if prepareState == fConversationPrepareStateSent || prepareState == fConversationPrepareStateSuccess {
		payload["partial_query"] = partialQueryFromChatRequest(chatReq)
	}
	if chatReq.ConversationId != "" {
		payload["conversation_id"] = chatReq.ConversationId
	}
	if mimeTypes := attachmentMimeTypes(chatReq); len(mimeTypes) > 0 {
		payload["attachment_mime_types"] = mimeTypes
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	headers, cookies := backend.Headers(backend.BaseURL + path)
	headers.Set("accept", "*/*")
	headers.Set("content-type", "application/json")
	headers.Set("x-openai-target-path", path)
	headers.Set("x-openai-target-route", path)
	applySentinelHeaders(headers, backend, turnTraceID)
	if previousConduitToken != "" {
		headers.Set("x-conduit-token", previousConduitToken)
	}
	resp, err := backend.HTTP.Request(tls_client_httpi.POST, backend.BaseURL+path, headers, cookies, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("prepare conversation(%s) failed: %w", prepareState, err)
	}
	defer resp.Body.Close()
	if !isHTTPSuccess(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("prepare conversation(%s) failed: status=%d body=%s", prepareState, resp.StatusCode, string(body))
	}
	var result struct {
		ConduitToken string `json:"conduit_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return strings.TrimSpace(result.ConduitToken), nil
}

func conversationPrepareModel(model string) string {
	if model == "" {
		return "auto"
	}
	return model
}

func applySentinelHeaders(headers tls_client_httpi.Headers, backend *chatgpt_backend.Client, turnTraceID string) {
	headers.Set("openai-sentinel-chat-requirements-token", backend.Auth.Token)
	if backend.Auth.PrepareToken != "" {
		headers.Set("openai-sentinel-chat-requirements-prepare-token", backend.Auth.PrepareToken)
	}
	if backend.Auth.ProofWork.Ospt != "" {
		headers.Set("openai-sentinel-proof-token", backend.Auth.ProofWork.Ospt)
	}
	if backend.Auth.TurnstileToken != "" {
		headers.Set("openai-sentinel-turnstile-token", backend.Auth.TurnstileToken)
	}
	if soToken := backend.EnsureSOToken(); soToken != "" {
		headers.Set("openai-sentinel-so-token", soToken)
	}
	if turnTraceID != "" {
		headers.Set("x-oai-turn-trace-id", turnTraceID)
	}
}

func partialQueryFromChatRequest(chatReq *chat.Request) map[string]interface{} {
	message := latestUserMessage(chatReq.Messages)
	return map[string]interface{}{
		"id":      uuid.New().String(),
		"author":  map[string]interface{}{"role": "user"},
		"content": message.Content,
	}
}

func latestUserMessage(messages []chat.Message) chat.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Author.Role) == "user" {
			return messages[i]
		}
	}
	if len(messages) > 0 {
		return messages[len(messages)-1]
	}
	return chat.Message{Content: chat.Content{ContentType: "text", Parts: []interface{}{""}}}
}

func attachmentMimeTypes(chatReq *chat.Request) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, message := range chatReq.Messages {
		attachments, ok := message.Metadata["attachments"].([]chat.Attachment)
		if !ok {
			continue
		}
		for _, item := range attachments {
			mimeType := strings.TrimSpace(item.MimeType)
			if mimeType != "" && !seen[mimeType] {
				seen[mimeType] = true
				result = append(result, mimeType)
			}
		}
	}
	return result
}
