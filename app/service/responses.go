package service

import (
	"chat2api/app/common"
	"chat2api/app/types/completions"
	"chat2api/app/types/responses"
	"chat2api/pkg/logx"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func Responses(c *gin.Context) {
	apiReq := &responses.ApiReq{}
	if err := c.BindJSON(apiReq); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, "Invalid parameter", nil)
		return
	}
	if hasResponsesImageGenerationTool(apiReq) {
		if err := runCodexImageResponses(c, apiReq); err != nil {
			logx.WithContext(c.Request.Context()).Error(err.Error())
			common.ErrorResponse(c, http.StatusBadGateway, "codex responses request failed", err.Error())
		}
		return
	}
	compReq := &completions.ApiReq{
		Model:               responses.NormalizeModel(apiReq.Model),
		Stream:              apiReq.Stream,
		Messages:            completionMessagesFromResponse(apiReq),
		Tools:               completionToolsFromResponses(apiReq.Tools),
		ToolChoice:          completionToolChoiceFromResponses(apiReq.ToolChoice),
		Temperature:         apiReq.Temperature,
		TopP:                apiReq.TopP,
		MaxTokens:           apiReq.MaxOutputTokens,
		MaxCompletionTokens: apiReq.MaxOutputTokens,
		User:                apiReq.User,
		Metadata:            apiReq.Metadata,
		Store:               apiReq.Store,
	}
	if apiReq.Text != nil && apiReq.Text.Format != nil {
		compReq.ResponseFormat = &completions.ResponseFormat{
			Type:       apiReq.Text.Format.Type,
			JSONSchema: apiReq.Text.Format.JSONSchema,
		}
	}
	if apiReq.Reasoning != nil {
		compReq.ReasoningEffort = apiReq.Reasoning.Effort
	}
	if len(compReq.Messages) == 0 {
		common.ErrorResponse(c, http.StatusBadRequest, "input text is required", nil)
		return
	}
	if err := prepareFunctionCallingRequest(compReq); err != nil {
		common.ErrorResponse(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	result, err := runResponsesTextChat(c, compReq, apiReq.Stream)
	if err != nil {
		logx.WithContext(c.Request.Context()).Error(err.Error())
		common.ErrorResponse(c, http.StatusBadGateway, "", err.Error())
		return
	}
	if result == nil {
		return
	}
	if len(result.ToolCalls) > 0 {
		items := make([]responses.OutputItem, 0, len(result.ToolCalls))
		if result.ToolContent != "" {
			items = append(items, responses.TextOutputItem(responses.MessageID(), result.ToolContent, "completed"))
		}
		for _, toolCall := range result.ToolCalls {
			items = append(items, responses.FunctionCallOutputItem(
				responses.MessageID(),
				toolCall.ID,
				toolCall.Function.Name,
				toolCall.Function.Arguments,
				"completed",
			))
		}
		c.JSON(http.StatusOK, responses.CompletedEvent(responses.ResponseID(), compReq.Model, time.Now().Unix(), items).Response)
		return
	}
	item := responses.TextOutputItem(responses.MessageID(), result.Content, "completed")
	c.JSON(http.StatusOK, responses.CompletedEvent(responses.ResponseID(), compReq.Model, time.Now().Unix(), []responses.OutputItem{item}).Response)
}
