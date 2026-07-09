package admin

import (
	"errors"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/http/handlers/shared"
	"github.com/dujiao-next/internal/http/response"
	"github.com/dujiao-next/internal/service"

	"github.com/gin-gonic/gin"
)

// ---- 客户价值 ----

// GetCustomerLTV 获取客户 LTV 分布
func (h *Handler) GetCustomerLTV(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetCustomerLTV(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetARPUSeries 获取 ARPU 月趋势
func (h *Handler) GetARPUSeries(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetARPUSeries(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetRepurchaseRate 获取复购率
func (h *Handler) GetRepurchaseRate(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetRepurchaseRate(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetChurnRiskUsers 获取流失预警用户
func (h *Handler) GetChurnRiskUsers(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	minLTV, _ := strconv.ParseFloat(strings.TrimSpace(c.DefaultQuery("min_ltv", "100")), 64)
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "20")))

	data, err := h.AnalyticsService.GetChurnRiskUsers(c.Request.Context(), input, minLTV, limit)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// ---- 商品分析 ----

// GetProductFunnel 获取商品转化漏斗
func (h *Handler) GetProductFunnel(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetProductFunnel(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetRefundRanking 获取退款率排行
func (h *Handler) GetRefundRanking(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "20")))

	data, err := h.AnalyticsService.GetRefundRanking(c.Request.Context(), input, limit)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetCrossSell 获取关联购买
func (h *Handler) GetCrossSell(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "20")))

	data, err := h.AnalyticsService.GetCrossSell(c.Request.Context(), input, limit)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetPriceBandDistribution 获取价格带销量分布
func (h *Handler) GetPriceBandDistribution(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetPriceBand(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// ---- 营收结构 ----

// GetRevenueByChannel 获取按支付通道的营收拆解
func (h *Handler) GetRevenueByChannel(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetRevenueByChannel(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetRevenueByMemberLevel 获取按会员等级的消费贡献
func (h *Handler) GetRevenueByMemberLevel(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetRevenueByMemberLevel(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetRevenueByCategory 获取按分类的营收拆解
func (h *Handler) GetRevenueByCategory(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetRevenueByCategory(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetRevenueHeatmap 获取营收时段热力图
func (h *Handler) GetRevenueHeatmap(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetRevenueHeatmap(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// ---- 用户增长 ----

// GetUserGrowth 获取新用户注册趋势
func (h *Handler) GetUserGrowth(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetUserGrowth(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetActivationFunnel 获取注册→首购转化漏斗
func (h *Handler) GetActivationFunnel(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetActivationFunnel(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// GetDAUTrend 获取 DAU 趋势
func (h *Handler) GetDAUTrend(c *gin.Context) {
	input, err := parseAnalyticsQuery(c)
	if err != nil {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	data, err := h.AnalyticsService.GetDAUTrend(c.Request.Context(), input)
	if err != nil {
		handleAnalyticsError(c, err)
		return
	}

	response.Success(c, data)
}

// ---- 工具 ----

// parseAnalyticsQuery 解析分析查询参数（复用 Dashboard 时间窗解析逻辑）
func parseAnalyticsQuery(c *gin.Context) (service.AnalyticsQueryInput, error) {
	rangeRaw := strings.TrimSpace(c.DefaultQuery("range", "7d"))
	fromRaw := strings.TrimSpace(c.Query("from"))
	toRaw := strings.TrimSpace(c.Query("to"))
	timezone := strings.TrimSpace(c.Query("tz"))

	from, err := shared.ParseTimeNullable(fromRaw)
	if err != nil {
		return service.AnalyticsQueryInput{}, err
	}
	to, err := shared.ParseTimeNullable(toRaw)
	if err != nil {
		return service.AnalyticsQueryInput{}, err
	}

	return service.AnalyticsQueryInput{
		Range:    rangeRaw,
		From:     from,
		To:       to,
		Timezone: timezone,
	}, nil
}

// handleAnalyticsError 处理分析查询错误
func handleAnalyticsError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrDashboardRangeInvalid) {
		shared.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	shared.RespondError(c, response.CodeInternal, "error.analytics_fetch_failed", err)
}
