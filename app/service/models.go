package service

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type modelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type modelListResponse struct {
	Object string      `json:"object"`
	Data   []modelItem `json:"data"`
}

// Models 对齐 OpenAI /v1/models 列表接口，返回当前对话链路可直接使用的模型 slug。
func Models(c *gin.Context) {
	models := []string{
		"auto",
		"gpt-5-5-instant",
		"gpt-5-5-thinking",
		"gpt-5-5-pro",
		"gpt-5",
		"gpt-4o",
		"gpt-4o-mini",
		"o3",
		"o4-mini",
		"o4-mini-high",
	}
	data := make([]modelItem, 0, len(models))
	created := int64(1685474247)
	if created == 0 {
		created = time.Now().Unix()
	}
	for _, model := range models {
		data = append(data, modelItem{
			ID:      model,
			Object:  "model",
			Created: created,
			OwnedBy: "openai",
		})
	}
	c.JSON(http.StatusOK, modelListResponse{Object: "list", Data: data})
}
