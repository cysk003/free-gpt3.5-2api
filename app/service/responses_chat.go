package service

import (
	"chat2api/app/types/completions"

	"github.com/gin-gonic/gin"
)

func runResponsesTextChat(c *gin.Context, apiReq *completions.ApiReq, streamResponses bool) (*chatResult, error) {
	chatReq := completions.BuildChatRequest(apiReq)
	// Responses 路径复用 chat completions 的 continue/handoff 能力。
	// streamResponses=true 时走 responses 事件封装；false 时复用完整对话循环。
	if streamResponses {
		resp, backend, err := sendChatRequest(c, chatReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if handleResponseError(c, resp, backend.AccAuth) {
			return nil, nil
		}
		if completions.HasTools(apiReq) {
			return streamResponsesFunctionCallingEvents(c, apiReq, resp)
		}
		_, err = streamResponsesTextEvents(c, apiReq.Model, resp)
		return nil, err
	}
	return runChatCompletionConversation(c, apiReq, chatReq)
}
