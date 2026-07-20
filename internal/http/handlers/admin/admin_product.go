package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// GetAdminProducts 获取商品列表 (Admin)
func (h *Handler) GetAdminProducts(c *gin.Context) {
	page, pageSize := shared.ParsePagination(c)
	categoryID := c.Query("category_id")
	search := c.Query("search")
	fulfillmentType := strings.TrimSpace(c.Query("fulfillment_type"))
	stockStatus := c.Query("stock_status")
	if stockStatus == "" {
		stockStatus = c.Query("stock_staus")
	}
	hasWholesalePrices, err := parseWholesaleFilter(c.Query("wholesale"))
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	if hasWholesalePrices == nil {
		hasWholesalePrices, err = parseWholesaleFilter(c.Query("has_wholesale_prices"))
		if err != nil {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
			return
		}
	}

	lowStockThreshold := h.SettingService.GetDashboardLowStockThreshold()
	products, total, err := h.ProductService.ListAdmin(categoryID, search, fulfillmentType, stockStatus, hasWholesalePrices, lowStockThreshold, page, pageSize)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		return
	}

	if err := h.ProductService.ApplyAutoStockCounts(products); err != nil {
		shared.RespondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		return
	}

	h.applyUpstreamDisplayTypes(products)

	pagination := response.BuildPagination(page, pageSize, total)
	response.SuccessWithPage(c, products, pagination)
}

func parseWholesaleFilter(raw string) (*bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "all":
		return nil, nil
	case "1", "true", "yes", "on", "enabled", "has":
		parsed := true
		return &parsed, nil
	case "0", "false", "no", "off", "disabled", "none":
		parsed := false
		return &parsed, nil
	default:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, err
		}
		return &parsed, nil
	}
}

// GetAdminProduct 获取商品详情 (Admin)
func (h *Handler) GetAdminProduct(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	product, err := h.ProductService.GetAdminByID(id)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		return
	}

	temp := []models.Product{*product}
	if err := h.ProductService.ApplyAutoStockCounts(temp); err != nil {
		shared.RespondError(c, response.CodeInternal, "error.product_fetch_failed", err)
		return
	}
	*product = temp[0]

	h.applyUpstreamDisplayTypes(temp)
	*product = temp[0]

	response.Success(c, product)
}

// ====================  商品管理  ====================

type ProductSKURequest struct {
	ID               uint                   `json:"id"`
	SKUCode          string                 `json:"sku_code" binding:"required"`
	SpecValuesJSON   map[string]interface{} `json:"spec_values"`
	PriceAmount      float64                `json:"price_amount" binding:"required"`
	CostPriceAmount  float64                `json:"cost_price_amount"`
	ManualStockTotal int                    `json:"manual_stock_total"`
	IsActive         *bool                  `json:"is_active"`
	SortOrder        int                    `json:"sort_order"`
}

type WholesalePriceRequest struct {
	SKUID       uint    `json:"sku_id"`
	SKUCode     string  `json:"sku_code"`
	MinQuantity int     `json:"min_quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

// CreateProductRequest 创建商品请求
type CreateProductRequest struct {
	CategoryID          uint                     `json:"category_id" binding:"required"`
	Slug                string                   `json:"slug" binding:"required"`
	SeoMetaJSON         map[string]interface{}   `json:"seo_meta"`
	TitleJSON           map[string]interface{}   `json:"title" binding:"required"`
	DescriptionJSON     map[string]interface{}   `json:"description"`
	ContentJSON         map[string]interface{}   `json:"content"`
	InstructionsJSON    map[string]interface{}   `json:"instructions"`
	ManualFormSchema    map[string]interface{}   `json:"manual_form_schema"`
	PriceAmount         float64                  `json:"price_amount" binding:"required"`
	CostPriceAmount     float64                  `json:"cost_price_amount"`
	WholesalePrices     *[]WholesalePriceRequest `json:"wholesale_prices"`
	Images              []string                 `json:"images"`
	Tags                []string                 `json:"tags"`
	PurchaseType        string                   `json:"purchase_type"`
	MinPurchaseQuantity *int                     `json:"min_purchase_quantity"`
	MaxPurchaseQuantity *int                     `json:"max_purchase_quantity"`
	StockDisplayMode    string                   `json:"stock_display_mode"`
	FulfillmentType     string                   `json:"fulfillment_type"`
	ManualStockTotal    *int                     `json:"manual_stock_total"`
	SKUs                []ProductSKURequest      `json:"skus"`
	PaymentChannelIDs   []uint                   `json:"payment_channel_ids"`
	IsAffiliateEnabled  *bool                    `json:"is_affiliate_enabled"`
	IsActive            *bool                    `json:"is_active"`
	SortOrder           int                      `json:"sort_order"`
}

// toWholesalePriceInputs 透传「是否提供」语义：请求未携带 wholesale_prices 时返回 nil
// （Update 保留原配置），携带（含空数组）时返回非 nil 指针以整体覆盖。
func toWholesalePriceInputs(items *[]WholesalePriceRequest) *[]service.WholesalePriceInput {
	if items == nil {
		return nil
	}
	result := make([]service.WholesalePriceInput, 0, len(*items))
	for _, item := range *items {
		result = append(result, service.WholesalePriceInput{
			SKUID:       item.SKUID,
			SKUCode:     strings.TrimSpace(item.SKUCode),
			MinQuantity: item.MinQuantity,
			UnitPrice:   decimal.NewFromFloat(item.UnitPrice),
		})
	}
	return &result
}

func toProductSKUInputs(items []ProductSKURequest) []service.ProductSKUInput {
	if len(items) == 0 {
		return nil
	}
	result := make([]service.ProductSKUInput, 0, len(items))
	for _, item := range items {
		result = append(result, service.ProductSKUInput{
			ID:               item.ID,
			SKUCode:          item.SKUCode,
			SpecValuesJSON:   item.SpecValuesJSON,
			PriceAmount:      decimal.NewFromFloat(item.PriceAmount),
			CostPriceAmount:  decimal.NewFromFloat(item.CostPriceAmount),
			ManualStockTotal: item.ManualStockTotal,
			IsActive:         item.IsActive,
			SortOrder:        item.SortOrder,
		})
	}
	return result
}

// CreateProduct 创建商品
func (h *Handler) CreateProduct(c *gin.Context) {
	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	product, err := h.ProductService.Create(service.CreateProductInput{
		CategoryID:           req.CategoryID,
		Slug:                 req.Slug,
		SeoMetaJSON:          req.SeoMetaJSON,
		TitleJSON:            req.TitleJSON,
		DescriptionJSON:      req.DescriptionJSON,
		ContentJSON:          req.ContentJSON,
		InstructionsJSON:     req.InstructionsJSON,
		ManualFormSchemaJSON: req.ManualFormSchema,
		PriceAmount:          decimal.NewFromFloat(req.PriceAmount),
		CostPriceAmount:      decimal.NewFromFloat(req.CostPriceAmount),
		WholesalePrices:      toWholesalePriceInputs(req.WholesalePrices),
		Images:               req.Images,
		Tags:                 req.Tags,
		PurchaseType:         req.PurchaseType,
		MinPurchaseQuantity:  req.MinPurchaseQuantity,
		MaxPurchaseQuantity:  req.MaxPurchaseQuantity,
		StockDisplayMode:     req.StockDisplayMode,
		FulfillmentType:      req.FulfillmentType,
		ManualStockTotal:     req.ManualStockTotal,
		SKUs:                 toProductSKUInputs(req.SKUs),
		PaymentChannelIDs:    req.PaymentChannelIDs,
		IsAffiliateEnabled:   req.IsAffiliateEnabled,
		IsActive:             req.IsActive,
		SortOrder:            req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, service.ErrSlugExists) {
			shared.RespondError(c, response.CodeBadRequest, "error.slug_exists", nil)
			return
		}
		if errors.Is(err, service.ErrProductPriceInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_price_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductPurchaseInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_purchase_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductCategoryInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_category_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrFulfillmentInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualFormSchemaInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.manual_form_schema_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualStockInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.manual_stock_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductPurchaseLimitInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_purchase_limit_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductStockDisplayInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		if errors.Is(err, service.ErrWholesalePriceInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.wholesale_price_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductSKUInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		if errors.Is(err, service.ErrProductSKUHasCardSecretStock) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_sku_has_card_secret_stock", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.product_create_failed", err)
		return
	}

	response.Success(c, product)
}

// UpdateProduct 更新商品
func (h *Handler) UpdateProduct(c *gin.Context) {
	id := c.Param("id")

	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	product, err := h.ProductService.Update(id, service.CreateProductInput{
		CategoryID:           req.CategoryID,
		Slug:                 req.Slug,
		SeoMetaJSON:          req.SeoMetaJSON,
		TitleJSON:            req.TitleJSON,
		DescriptionJSON:      req.DescriptionJSON,
		ContentJSON:          req.ContentJSON,
		InstructionsJSON:     req.InstructionsJSON,
		ManualFormSchemaJSON: req.ManualFormSchema,
		PriceAmount:          decimal.NewFromFloat(req.PriceAmount),
		CostPriceAmount:      decimal.NewFromFloat(req.CostPriceAmount),
		WholesalePrices:      toWholesalePriceInputs(req.WholesalePrices),
		Images:               req.Images,
		Tags:                 req.Tags,
		PurchaseType:         req.PurchaseType,
		MinPurchaseQuantity:  req.MinPurchaseQuantity,
		MaxPurchaseQuantity:  req.MaxPurchaseQuantity,
		StockDisplayMode:     req.StockDisplayMode,
		FulfillmentType:      req.FulfillmentType,
		ManualStockTotal:     req.ManualStockTotal,
		SKUs:                 toProductSKUInputs(req.SKUs),
		PaymentChannelIDs:    req.PaymentChannelIDs,
		IsAffiliateEnabled:   req.IsAffiliateEnabled,
		IsActive:             req.IsActive,
		SortOrder:            req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrSlugExists) {
			shared.RespondError(c, response.CodeBadRequest, "error.slug_used", nil)
			return
		}
		if errors.Is(err, service.ErrProductPriceInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_price_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductPurchaseInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_purchase_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductCategoryInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_category_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrFulfillmentInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.fulfillment_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualFormSchemaInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.manual_form_schema_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrManualStockInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.manual_stock_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductPurchaseLimitInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_purchase_limit_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductStockDisplayInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		if errors.Is(err, service.ErrWholesalePriceInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.wholesale_price_invalid", nil)
			return
		}
		if errors.Is(err, service.ErrProductSKUInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
			return
		}
		if errors.Is(err, service.ErrProductSKUHasCardSecretStock) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_sku_has_card_secret_stock", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.product_update_failed", err)
		return
	}

	response.Success(c, product)
}

// QuickUpdateProductRequest 快速更新商品请求
type QuickUpdateProductRequest struct {
	IsActive   *bool `json:"is_active"`
	SortOrder  *int  `json:"sort_order"`
	CategoryID *uint `json:"category_id"`
}

type UpdateWholesalePricesRequest struct {
	WholesalePrices *[]WholesalePriceRequest `json:"wholesale_prices" binding:"required"`
}

// UpdateProductWholesalePrices 更新商品批发价阶梯。
func (h *Handler) UpdateProductWholesalePrices(c *gin.Context) {
	id := c.Param("id")

	var req UpdateWholesalePricesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	inputs := toWholesalePriceInputs(req.WholesalePrices)
	if inputs == nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	product, err := h.ProductService.UpdateWholesalePrices(id, *inputs)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrWholesalePriceInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.wholesale_price_invalid", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.product_update_failed", err)
		return
	}

	response.Success(c, product)
}

// QuickUpdateProduct 快速更新商品（状态/排序/分类）
func (h *Handler) QuickUpdateProduct(c *gin.Context) {
	id := c.Param("id")

	var req QuickUpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	fields := make(map[string]interface{})
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}
	if req.SortOrder != nil {
		fields["sort_order"] = *req.SortOrder
	}
	if req.CategoryID != nil {
		fields["category_id"] = *req.CategoryID
	}
	if len(fields) == 0 {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}

	product, err := h.ProductService.QuickUpdate(id, fields)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrProductCategoryInvalid) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_category_invalid", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.product_update_failed", err)
		return
	}

	response.Success(c, product)
}

// applyUpstreamDisplayTypes 将 upstream 类型商品的 FulfillmentType 替换为上游的实际交付类型，并填充库存字段
func (h *Handler) applyUpstreamDisplayTypes(products []models.Product) {
	var upstreamIDs []uint
	idxMap := make(map[uint]int) // localProductID -> products slice index
	for i := range products {
		if products[i].FulfillmentType == constants.FulfillmentTypeUpstream {
			upstreamIDs = append(upstreamIDs, products[i].ID)
			idxMap[products[i].ID] = i
		}
	}
	if len(upstreamIDs) == 0 {
		return
	}

	mappings, err := h.ProductMappingRepo.ListByLocalProductIDs(upstreamIDs)
	if err != nil || len(mappings) == 0 {
		return
	}

	for _, mp := range mappings {
		idx, ok := idxMap[mp.LocalProductID]
		if !ok {
			continue
		}
		p := &products[idx]

		displayType := mp.UpstreamFulfillmentType
		if displayType != constants.FulfillmentTypeAuto {
			displayType = constants.FulfillmentTypeManual
		}
		p.FulfillmentType = displayType

		// 获取 SKU 映射以填充库存字段
		skuMappings, err := h.SKUMappingRepo.ListByProductMapping(mp.ID)
		if err != nil || len(skuMappings) == 0 {
			continue
		}

		skuMappingByLocal := make(map[uint]*models.SKUMapping, len(skuMappings))
		for i := range skuMappings {
			skuMappingByLocal[skuMappings[i].LocalSKUID] = &skuMappings[i]
		}

		var totalStock int64
		hasUnlimited := false

		for j := range p.SKUs {
			sku := &p.SKUs[j]
			sm, found := skuMappingByLocal[sku.ID]
			if !found || !sm.UpstreamIsActive {
				continue
			}

			if sm.UpstreamStock == -1 {
				hasUnlimited = true
			} else {
				totalStock += int64(sm.UpstreamStock)
			}

			if displayType == constants.FulfillmentTypeAuto {
				sku.AutoStockAvailable = int64(sm.UpstreamStock)
				if sm.UpstreamStock > 0 {
					sku.AutoStockTotal = int64(sm.UpstreamStock)
				}
			} else {
				sku.ManualStockTotal = sm.UpstreamStock
			}
		}

		// 填充商品级汇总库存
		if displayType == constants.FulfillmentTypeAuto {
			if hasUnlimited {
				p.AutoStockAvailable = -1
			} else {
				p.AutoStockAvailable = totalStock
				p.AutoStockTotal = totalStock
			}
		} else {
			if hasUnlimited {
				p.ManualStockTotal = constants.ManualStockUnlimited
			} else {
				p.ManualStockTotal = int(totalStock)
			}
		}
	}
}

// BatchProductActionRequest 商品批量操作请求
type BatchProductActionRequest struct {
	IDs []uint `json:"ids" binding:"required,min=1"`
}

// BatchProductStatusRequest 商品批量状态更新请求
type BatchProductStatusRequest struct {
	IDs      []uint `json:"ids" binding:"required,min=1"`
	IsActive bool   `json:"is_active"`
}

// BatchProductCategoryRequest 商品批量分类更新请求
type BatchProductCategoryRequest struct {
	IDs        []uint `json:"ids" binding:"required,min=1"`
	CategoryID uint   `json:"category_id"`
}

type batchProductFailureItem struct {
	ID        uint   `json:"id"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func productBatchFailureFromError(locale string, id uint, err error) batchProductFailureItem {
	errorCode := "product_update_failed"
	switch {
	case errors.Is(err, service.ErrProductCategoryInvalid):
		errorCode = "product_category_invalid"
	case errors.Is(err, service.ErrNotFound):
		errorCode = "product_not_found"
	}
	return batchProductFailureItem{
		ID:        id,
		ErrorCode: errorCode,
		Message:   i18n.T(locale, "error."+errorCode),
	}
}

// BatchUpdateProductStatus 批量上架/下架
func (h *Handler) BatchUpdateProductStatus(c *gin.Context) {
	var req BatchProductStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	locale := i18n.ResolveLocale(c)
	successCount := 0
	failedItems := make([]batchProductFailureItem, 0)
	for _, id := range req.IDs {
		_, err := h.ProductService.QuickUpdate(strconv.FormatUint(uint64(id), 10), map[string]interface{}{"is_active": req.IsActive})
		if err == nil {
			successCount++
		} else {
			failedItems = append(failedItems, productBatchFailureFromError(locale, id, err))
		}
	}
	response.Success(c, gin.H{"total": len(req.IDs), "success_count": successCount, "failed_items": failedItems})
}

// BatchUpdateProductCategory 批量修改分类
func (h *Handler) BatchUpdateProductCategory(c *gin.Context) {
	var req BatchProductCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	successCount := 0
	for _, id := range req.IDs {
		_, err := h.ProductService.QuickUpdate(strconv.FormatUint(uint64(id), 10), map[string]interface{}{"category_id": req.CategoryID})
		if err == nil {
			successCount++
		}
	}
	response.Success(c, gin.H{"total": len(req.IDs), "success_count": successCount})
}

// BatchDeleteProducts 批量删除商品
func (h *Handler) BatchDeleteProducts(c *gin.Context) {
	var req BatchProductActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}
	successCount := 0
	var failedIDs []uint
	for _, id := range req.IDs {
		if err := h.ProductService.Delete(strconv.FormatUint(uint64(id), 10)); err == nil {
			successCount++
		} else {
			failedIDs = append(failedIDs, id)
		}
	}
	response.Success(c, gin.H{"total": len(req.IDs), "success_count": successCount, "failed_ids": failedIDs})
}

// DeleteProduct 删除商品
func (h *Handler) DeleteProduct(c *gin.Context) {
	id := c.Param("id")

	if err := h.ProductService.Delete(id); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			shared.RespondError(c, response.CodeNotFound, "error.product_not_found", nil)
			return
		}
		if errors.Is(err, service.ErrProductHasStock) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_has_stock", nil)
			return
		}
		if errors.Is(err, service.ErrProductHasOrderRecord) {
			shared.RespondError(c, response.CodeBadRequest, "error.product_has_order_record", nil)
			return
		}
		shared.RespondError(c, response.CodeInternal, "error.product_delete_failed", err)
		return
	}

	response.Success(c, nil)
}

// AIProductSEORequest AI SEO 生成请求。
type AIProductSEORequest struct {
	Fields                  []string               `json:"fields"`
	Languages               []string               `json:"languages"`
	CategoryName            string                 `json:"category_name"`
	CurrentTitle            map[string]interface{} `json:"current_title"`
	CurrentSlug             string                 `json:"current_slug"`
	CurrentMetaKeywords     map[string]interface{} `json:"current_meta_keywords"`
	CurrentMetaDescription  map[string]interface{} `json:"current_meta_description"`
	CurrentDescription      map[string]interface{} `json:"current_description"`
}

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatRequest struct {
	Model    string          `json:"model"`
	Messages []aiChatMessage `json:"messages"`
}

type aiChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

const defaultAISystemPrompt = "你是一个专业的电商 SEO 优化专家，请根据用户提供的商品信息，为指定的字段和语言生成优化后的内容。"

// truncateMetaDescription 截断 meta_description 到 160 字符以内（作为兜底）。
func truncateMetaDescription(result map[string]interface{}, languages []string) {
	descRaw, ok := result["meta_description"]
	if !ok {
		return
	}
	descMap, ok := descRaw.(map[string]interface{})
	if !ok {
		return
	}
	for _, lang := range languages {
		val, ok := descMap[lang]
		if !ok {
			continue
		}
		str, ok := val.(string)
		if !ok {
			continue
		}
		runes := []rune(str)
		if len(runes) > 160 {
			descMap[lang] = string(runes[:160])
		}
	}
}

// AIProductSEO 调用 AI 接口为商品生成 SEO 内容。
func (h *Handler) AIProductSEO(c *gin.Context) {
	var req AIProductSEORequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shared.RespondBindError(c, err)
		return
	}

	cfg, err := h.SettingService.GetAIConfig()
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.ai_config_not_found", err)
		return
	}

	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultAISystemPrompt
	}

	userPrompt := buildAIProductSEOPrompt(req)
	aiReqBody := aiChatRequest{
		Model: cfg.ModelID,
		Messages: []aiChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
	}
	reqBytes, err := json.Marshal(aiReqBody)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_parse_failed", err)
		return
	}

	httpReq, err := http.NewRequest(http.MethodPost, cfg.APIUrl, bytes.NewReader(reqBytes))
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_connection_failed", err)
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_connection_failed", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_api_error", fmt.Errorf("AI API returned status %d", resp.StatusCode))
		return
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_connection_failed", err)
		return
	}

	var aiResp aiChatResponse
	if err := json.Unmarshal(respBytes, &aiResp); err != nil {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_parse_failed", err)
		return
	}
	if len(aiResp.Choices) == 0 {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_parse_failed", fmt.Errorf("empty AI response"))
		return
	}

	content := strings.TrimSpace(aiResp.Choices[0].Message.Content)

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		shared.RespondError(c, response.CodeInternal, "error.ai_seo_parse_failed", err)
		return
	}

	// 对 meta_description 做 160 字符兜底截断
	truncateMetaDescription(result, req.Languages)

	// 仅返回请求的字段和语言
	filtered := filterAIProductSEOResult(result, req.Fields, req.Languages)
	response.Success(c, filtered)
}

func buildAIProductSEOPrompt(req AIProductSEORequest) string {
	var sb strings.Builder
	sb.WriteString("请为以下商品优化 SEO 内容。\n\n")

	sb.WriteString(fmt.Sprintf("需要优化的字段: %s\n", strings.Join(req.Fields, ", ")))
	sb.WriteString(fmt.Sprintf("目标语言: %s\n\n", strings.Join(req.Languages, ", ")))

	if req.CategoryName != "" {
		sb.WriteString(fmt.Sprintf("商品分类: %s\n", req.CategoryName))
	}

	sb.WriteString("当前内容:\n")
	for _, field := range req.Fields {
		switch field {
		case "title":
			sb.WriteString(fmt.Sprintf("- 标题: %v\n", req.CurrentTitle))
		case "slug":
			sb.WriteString(fmt.Sprintf("- Slug: %s\n", req.CurrentSlug))
		case "meta_keywords":
			sb.WriteString(fmt.Sprintf("- Meta Keywords: %v\n", req.CurrentMetaKeywords))
		case "meta_description":
			sb.WriteString(fmt.Sprintf("- Meta Description: %v\n", req.CurrentMetaDescription))
			sb.WriteString("生成的 Meta Description 请严格控制在 160 个字符以内\n")
		case "description":
			sb.WriteString(fmt.Sprintf("- 描述: %v\n", req.CurrentDescription))
		}
	}

	sb.WriteString("\n请为每个目标语言生成对应字段的优化内容，以 JSON 格式返回。")

	return sb.String()
}

func filterAIProductSEOResult(result map[string]interface{}, fields, languages []string) map[string]interface{} {
	filtered := make(map[string]interface{})
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[f] = true
	}
	langSet := make(map[string]bool, len(languages))
	for _, l := range languages {
		langSet[l] = true
	}
	for _, field := range fields {
		raw, ok := result[field]
		if !ok {
			continue
		}
		if field == "slug" {
			if slug, ok := raw.(string); ok {
				filtered[field] = slug
			}
			continue
		}
		fieldMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		langResult := make(map[string]interface{})
		for _, lang := range languages {
			if val, ok := fieldMap[lang]; ok {
				langResult[lang] = val
			}
		}
		if len(langResult) > 0 {
			filtered[field] = langResult
		}
	}
	return filtered
}
