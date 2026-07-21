package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"chat2api/app/chatgpt_backend"

	"github.com/aurorax-neo/tls_client_httpi"
	"github.com/gorilla/websocket"
)

type chatWebsocketURLResponse struct {
	WebsocketURL string `json:"websocket_url"`
}

var chatWebsocketIDCounter int64 = 4

func nextChatWebsocketID() int64 {
	return atomic.AddInt64(&chatWebsocketIDCounter, 1)
}

func handoffTopicFromPayload(payload string) (string, bool) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return "", false
	}
	eventType := strings.TrimSpace(responseStringValue(raw["type"], ""))
	if eventType == "stream_handoff" {
		if topic := handoffTopicFromEvent(raw); topic != "" {
			return topic, true
		}
		return "", true
	}
	if eventType == "server_ste_metadata" {
		if topic := handoffTopicFromMetadata(raw); topic != "" {
			return topic, true
		}
		return "", true
	}
	if eventType == "resume_conversation_token" {
		return "", true
	}
	return "", false
}

func handoffTopicFromEvent(raw map[string]interface{}) string {
	options, ok := raw["options"].([]interface{})
	if !ok {
		return ""
	}
	for _, optionValue := range options {
		option, ok := optionValue.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.TrimSpace(responseStringValue(option["type"], "")) != "subscribe_ws_topic" {
			continue
		}
		if topic := strings.TrimSpace(responseStringValue(option["topic_id"], "")); topic != "" {
			return topic
		}
	}
	return ""
}

func handoffTopicFromMetadata(raw map[string]interface{}) string {
	if turn := strings.TrimSpace(responseStringValue(raw["turn_exchange_id"], "")); turn != "" {
		return "conversation-turn-" + turn
	}
	if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
		if turn := strings.TrimSpace(responseStringValue(metadata["turn_exchange_id"], "")); turn != "" {
			return "conversation-turn-" + turn
		}
	}
	return ""
}

func openChatWebsocketHandoff(backend *chatgpt_backend.Client, topicID string) (io.ReadCloser, error) {
	if backend == nil || strings.TrimSpace(topicID) == "" {
		return nil, fmt.Errorf("websocket handoff unavailable")
	}
	conn, err := dialChatWebsocket(backend)
	if err != nil {
		return nil, err
	}
	return chatWebsocketStreamReader(conn, topicID)
}

func dialChatWebsocket(backend *chatgpt_backend.Client) (*websocket.Conn, error) {
	wsURL, err := getChatWebsocketURL(backend)
	if err != nil {
		return nil, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	header := http.Header{}
	header.Set("User-Agent", backend.UserAgent)
	header.Set("Origin", backend.BaseURL)
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, err
	}
	initMsg := []map[string]interface{}{
		{"id": 1, "command": map[string]interface{}{
			"type":     "connect",
			"presence": map[string]string{"type": "presence", "state": "background"},
		}},
		{"id": 2, "command": map[string]interface{}{"type": "subscribe", "topic_id": "calpico-chatgpt"}},
		{"id": 3, "command": map[string]interface{}{"type": "subscribe", "topic_id": "conversations"}},
		{"id": 4, "command": map[string]interface{}{"type": "subscribe", "topic_id": "app_notifications"}},
	}
	if err := conn.WriteJSON(initMsg); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func getChatWebsocketURL(backend *chatgpt_backend.Client) (string, error) {
	path := "/backend-api/celsius/ws/user"
	if backend.AccAuth == "" {
		path = "/backend-anon/celsius/ws/user"
	}
	apiURL := backend.BaseURL + path
	headers, cookies := backend.Headers(apiURL)
	headers.Set("accept", "*/*")
	headers.Set("x-openai-target-path", path)
	headers.Set("x-openai-target-route", path)
	resp, err := backend.HTTP.Request(tls_client_httpi.GET, apiURL, headers, cookies, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("celsius ws user failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	var result chatWebsocketURLResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.WebsocketURL) == "" {
		return "", fmt.Errorf("celsius ws user missing websocket_url")
	}
	return result.WebsocketURL, nil
}

func chatWebsocketStreamReader(conn *websocket.Conn, topicID string) (io.ReadCloser, error) {
	reader, writer := io.Pipe()
	subMsg := []map[string]interface{}{
		{"id": nextChatWebsocketID(), "command": map[string]interface{}{
			"type":     "subscribe",
			"topic_id": topicID,
			"offset":   "0",
		}},
	}
	if err := conn.WriteJSON(subMsg); err != nil {
		_ = reader.Close()
		_ = conn.Close()
		return nil, err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	})
	go func() {
		defer writer.Close()
		defer conn.Close()
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		done := make(chan error, 1)
		go func() {
			for {
				conn.SetReadDeadline(time.Now().Add(120 * time.Second))
				_, raw, err := conn.ReadMessage()
				if err != nil {
					done <- err
					return
				}
				for _, frame := range parseChatWebsocketFrames(raw) {
					frameType := strings.TrimSpace(responseStringValue(frame["type"], ""))
					if frameType == "reply" {
						reply, _ := frame["reply"].(map[string]interface{})
						replyTopicID := strings.TrimSpace(responseStringValue(reply["topic_id"], ""))
						if replyTopicID != topicID {
							continue
						}
						catchups, _ := reply["catchups"].([]interface{})
						for _, catchup := range catchups {
							catchupFrame, _ := catchup.(map[string]interface{})
							for _, item := range chatWebsocketSSEItems(catchupFrame, topicID) {
								if chatWebsocketWriteEncodedItem(writer, item) {
									done <- nil
									return
								}
							}
						}
						continue
					}
					if frameType != "message" {
						continue
					}
					for _, item := range chatWebsocketSSEItems(frame, topicID) {
						if chatWebsocketWriteEncodedItem(writer, item) {
							done <- nil
							return
						}
					}
				}
			}
		}()
		for {
			select {
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			case err := <-done:
				if err != nil {
					_ = writer.CloseWithError(err)
				}
				return
			}
		}
	}()
	return reader, nil
}

func parseChatWebsocketFrames(raw []byte) []map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}
	if raw[0] == '[' {
		var frames []map[string]interface{}
		if err := json.Unmarshal(raw, &frames); err != nil {
			return nil
		}
		return frames
	}
	var frame map[string]interface{}
	if err := json.Unmarshal(raw, &frame); err != nil {
		return nil
	}
	return []map[string]interface{}{frame}
}

func chatWebsocketSSEItems(frame map[string]interface{}, topicID string) []string {
	if encoded := chatWebsocketEncodedItem(frame, topicID); encoded != "" {
		return []string{encoded}
	}
	if update := chatWebsocketConversationUpdateItem(frame, topicID); update != "" {
		return []string{update}
	}
	return nil
}

func chatWebsocketEncodedItem(frame map[string]interface{}, topicID string) string {
	if frame == nil {
		return ""
	}
	if frameTopicID := strings.TrimSpace(responseStringValue(frame["topic_id"], "")); frameTopicID != "" && frameTopicID != topicID {
		return ""
	}
	payload, ok := frame["payload"].(map[string]interface{})
	if !ok {
		return ""
	}
	nested, ok := payload["payload"].(map[string]interface{})
	if !ok {
		return ""
	}
	return strings.TrimSpace(responseStringValue(nested["encoded_item"], ""))
}

func chatWebsocketConversationUpdateItem(frame map[string]interface{}, topicID string) string {
	if frame == nil {
		return ""
	}
	frameTopicID := strings.TrimSpace(responseStringValue(frame["topic_id"], ""))
	if frameTopicID != "" && frameTopicID != topicID && frameTopicID != "conversations" {
		return ""
	}
	payload, ok := frame["payload"].(map[string]interface{})
	if !ok {
		return ""
	}
	payloadType := strings.TrimSpace(responseStringValue(payload["type"], ""))
	if payloadType != "conversation-update" {
		if nested, ok := payload["payload"].(map[string]interface{}); ok {
			if nestedType := strings.TrimSpace(responseStringValue(nested["type"], "")); nestedType == "conversation-update" {
				payload = nested
				payloadType = nestedType
			}
		}
	}
	if payloadType != "conversation-update" {
		return ""
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return "data: " + string(body) + "\n"
}

func chatWebsocketWriteEncodedItem(writer *io.PipeWriter, encoded string) bool {
	if encoded == "" {
		return false
	}
	if !strings.HasSuffix(encoded, "\n") {
		encoded += "\n"
	}
	_, _ = writer.Write([]byte(encoded))
	return strings.Contains(encoded, "data: [DONE]") || strings.Contains(encoded, "data:[DONE]")
}
