package admin

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/repository"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// digitalContentBindingBody 绑定入参
type digitalContentBindingBody struct {
	ProductID uint `json:"product_id" binding:"required"`
	SKUID     uint `json:"sku_id"`
}

// digitalContentBody 数字内容创建/更新请求体
type digitalContentBody struct {
	Title    string                      `json:"title" binding:"required"`
	Content  string                      `json:"content" binding:"required"`
	IsActive *bool                       `json:"is_active"`
	Bindings []digitalContentBindingBody `json:"bindings"`
}

func (b digitalContentBody) toInput() service.DigitalContentInput {
	active := true
	if b.IsActive != nil {
		active = *b.IsActive
	}
	bindings := make([]service.DigitalContentBindingInput, 0, len(b.Bindings))
	for _, x := range b.Bindings {
		bindings = append(bindings, service.DigitalContentBindingInput{ProductID: x.ProductID, SKUID: x.SKUID})
	}
	return service.DigitalContentInput{
		Title:    b.Title,
		Content:  b.Content,
		IsActive: active,
		Bindings: bindings,
	}
}

// GetDigitalContents 数字内容列表
func (h *Handler) GetDigitalContents(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)
	filter := repository.DigitalContentListFilter{
		Title:    strings.TrimSpace(c.Query("title")),
		Page:     page,
		PageSize: pageSize,
	}
	if pid, err := shared.ParseQueryUint(c.Query("product_id"), false); err == nil {
		filter.ProductID = pid
	}
	if raw := strings.TrimSpace(c.Query("is_active")); raw != "" {
		active := raw == "true" || raw == "1"
		filter.IsActive = &active
	}

	items, total, err := h.DigitalContentService.List(filter)
	if err != nil {
		respondDigitalContentError(c, err)
		return
	}
	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, items, pagination)
}

// GetDigitalContent 数字内容详情（含绑定）
func (h *Handler) GetDigitalContent(c *gin.Context) {
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondBindError(c, err)
		return
	}
	content, err := h.DigitalContentService.GetByID(id)
	if err != nil {
		respondDigitalContentError(c, err)
		return
	}
	response.Success(c, content)
}

// CreateDigitalContent 创建数字内容
func (h *Handler) CreateDigitalContent(c *gin.Context) {
	var body digitalContentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	content, err := h.DigitalContentService.Create(body.toInput())
	if err != nil {
		respondDigitalContentError(c, err)
		return
	}
	response.Success(c, content)
}

// UpdateDigitalContent 更新数字内容
func (h *Handler) UpdateDigitalContent(c *gin.Context) {
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondBindError(c, err)
		return
	}
	var body digitalContentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	content, err := h.DigitalContentService.Update(id, body.toInput())
	if err != nil {
		respondDigitalContentError(c, err)
		return
	}
	response.Success(c, content)
}

// DeleteDigitalContent 删除数字内容
func (h *Handler) DeleteDigitalContent(c *gin.Context) {
	id, err := shared.ParseParamUint(c, "id")
	if err != nil {
		shared.RespondBindError(c, err)
		return
	}
	if err := h.DigitalContentService.Delete(id); err != nil {
		respondDigitalContentError(c, err)
		return
	}
	response.Success(c, nil)
}

func respondDigitalContentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrDigitalContentNotFound):
		shared.RespondError(c, response.CodeNotFound, "error.digital_content_not_found", nil)
	case errors.Is(err, service.ErrProductNotFound):
		shared.RespondError(c, response.CodeBadRequest, "error.product_not_found", nil)
	case errors.Is(err, service.ErrDigitalContentInvalid):
		shared.RespondError(c, response.CodeBadRequest, "error.digital_content_invalid", nil)
	case errors.Is(err, service.ErrDigitalContentBindingConflict):
		shared.RespondError(c, response.CodeBadRequest, "error.digital_content_binding_conflict", nil)
	case errors.Is(err, service.ErrDigitalContentProductHasCardSecret):
		shared.RespondError(c, response.CodeBadRequest, "error.digital_content_product_has_card_secret", nil)
	default:
		shared.RespondError(c, response.CodeInternal, "error.digital_content_save_failed", err)
	}
}
