package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/repository"
)

const (
	analyticsCacheTTL = 45 * time.Second
)

// AnalyticsQueryInput 数据分析查询输入（复用 Dashboard 时间窗逻辑）
type AnalyticsQueryInput = DashboardQueryInput

// AnalyticsService 数据分析服务
type AnalyticsService struct {
	repo repository.AnalyticsRepository
}

// NewAnalyticsService 创建数据分析服务
func NewAnalyticsService(repo repository.AnalyticsRepository) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

// resolveAnalyticsWindow 解析时间窗口，与 dashboard 保持一致的 90 天上限
func resolveAnalyticsWindow(input AnalyticsQueryInput, now time.Time) (dashboardWindow, error) {
	return resolveDashboardWindow(input, now)
}

// ---- 客户价值 ----

// CustomerLTVResponse LTV 分布响应
type CustomerLTVResponse struct {
	Levels []CustomerLTVLevel `json:"levels"`
}

// CustomerLTVLevel 等级 LTV 数据
type CustomerLTVLevel struct {
	MemberLevelID uint    `json:"member_level_id"`
	UserCount     int64   `json:"user_count"`
	AvgLTV        float64 `json:"avg_ltv"`
	MaxLTV        float64 `json:"max_ltv"`
	MedianLTV     float64 `json:"median_ltv"`
}

// GetCustomerLTV 获取客户 LTV 分布
func (s *AnalyticsService) GetCustomerLTV(ctx context.Context, input AnalyticsQueryInput) (*CustomerLTVResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:ltv:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached CustomerLTVResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetCustomerLTV(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	// 查询 LTV 原始值用于计算中位数
	type ltvRepoWithValues interface {
		GetLTVValuesByLevel(startAt, endAt time.Time) (map[uint][]float64, error)
	}
	medianMap := make(map[uint]float64)
	if r, ok := s.repo.(ltvRepoWithValues); ok {
		if ltvValues, err := r.GetLTVValuesByLevel(window.startAt, window.endAt); err == nil {
			for levelID, values := range ltvValues {
				medianMap[levelID] = median(values)
			}
		}
	}

	levels := make([]CustomerLTVLevel, 0, len(rows))
	for _, row := range rows {
		levels = append(levels, CustomerLTVLevel{
			MemberLevelID: row.MemberLevelID,
			UserCount:     row.UserCount,
			AvgLTV:        row.AvgLTV,
			MaxLTV:        row.MaxLTV,
			MedianLTV:     medianMap[row.MemberLevelID],
		})
	}

	resp := &CustomerLTVResponse{Levels: levels}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ARPUSeriesResponse ARPU 月趋势响应
type ARPUSeriesResponse struct {
	Points []ARPUSeriesPoint `json:"points"`
}

// ARPUSeriesPoint ARPU 趋势点
type ARPUSeriesPoint struct {
	Month       string  `json:"month"`
	Revenue     float64 `json:"revenue"`
	PayingUsers int64   `json:"paying_users"`
	ARPU        float64 `json:"arpu"`
}

// GetARPUSeries 获取 ARPU 月趋势
func (s *AnalyticsService) GetARPUSeries(ctx context.Context, input AnalyticsQueryInput) (*ARPUSeriesResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:arpu:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached ARPUSeriesResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	loc := parseAnalyticsLocation(window.timezone)
	refTime := time.Now()
	rows, err := s.repo.GetARPUSeries(window.startAt, window.endAt, loc, refTime)
	if err != nil {
		return nil, err
	}

	points := make([]ARPUSeriesPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, ARPUSeriesPoint{
			Month:       row.Month,
			Revenue:     row.Revenue,
			PayingUsers: row.PayingUsers,
			ARPU:        row.ARPU,
		})
	}

	resp := &ARPUSeriesResponse{Points: points}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// RepurchaseRateResponse 复购率响应
type RepurchaseRateResponse struct {
	Once           int64   `json:"once"`
	Twice          int64   `json:"twice"`
	ThreePlus      int64   `json:"three_plus"`
	TotalUsers     int64   `json:"total_users"`
	RepurchaseRate float64 `json:"repurchase_rate"`
}

// GetRepurchaseRate 获取复购率
func (s *AnalyticsService) GetRepurchaseRate(ctx context.Context, input AnalyticsQueryInput) (*RepurchaseRateResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:repurchase:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached RepurchaseRateResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	row, err := s.repo.GetRepurchaseRate(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	resp := &RepurchaseRateResponse{
		Once:           row.Once,
		Twice:          row.Twice,
		ThreePlus:      row.ThreePlus,
		TotalUsers:     row.TotalUsers,
		RepurchaseRate: row.RepurchaseRate,
	}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ChurnRiskResponse 流失预警响应
type ChurnRiskResponse struct {
	Users []ChurnRiskUser `json:"users"`
}

// ChurnRiskUser 流失预警用户
type ChurnRiskUser struct {
	UserID        uint    `json:"user_id"`
	Email         string  `json:"email"`
	MemberLevelID uint    `json:"member_level_id"`
	LastOrderAt   string  `json:"last_order_at"`
	LifetimeValue float64 `json:"lifetime_value"`
	TotalOrders   int64   `json:"total_orders"`
}

// GetChurnRiskUsers 获取流失预警用户
func (s *AnalyticsService) GetChurnRiskUsers(ctx context.Context, input AnalyticsQueryInput, minLTV float64, limit int) (*ChurnRiskResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if minLTV <= 0 {
		minLTV = 100
	}

	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:churn:%s:%d:%d:%.0f:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix(), minLTV, limit)
	var cached ChurnRiskResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	cutoffTime := time.Now().AddDate(0, 0, -30)
	rows, err := s.repo.GetChurnRiskUsers(cutoffTime, minLTV, limit)
	if err != nil {
		return nil, err
	}

	users := make([]ChurnRiskUser, 0, len(rows))
	for _, row := range rows {
		users = append(users, ChurnRiskUser{
			UserID:        row.UserID,
			Email:         row.Email,
			MemberLevelID: row.MemberLevelID,
			LastOrderAt:   row.LastOrderAt,
			LifetimeValue: row.LifetimeValue,
			TotalOrders:   row.TotalOrders,
		})
	}

	resp := &ChurnRiskResponse{Users: users}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ---- 商品分析 ----

// ProductFunnelResponse 商品转化漏斗响应
type ProductFunnelResponse struct {
	Products []ProductFunnelItem `json:"products"`
}

// ProductFunnelItem 商品漏斗项
type ProductFunnelItem struct {
	ProductID       uint  `json:"product_id"`
	OrdersCreated   int64 `json:"orders_created"`
	OrdersPaid      int64 `json:"orders_paid"`
	OrdersCompleted int64 `json:"orders_completed"`
}

// GetProductFunnel 获取商品转化漏斗
func (s *AnalyticsService) GetProductFunnel(ctx context.Context, input AnalyticsQueryInput) (*ProductFunnelResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:funnel:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached ProductFunnelResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetProductFunnel(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	products := make([]ProductFunnelItem, 0, len(rows))
	for _, row := range rows {
		products = append(products, ProductFunnelItem{
			ProductID:       row.ProductID,
			OrdersCreated:   row.OrdersCreated,
			OrdersPaid:      row.OrdersPaid,
			OrdersCompleted: row.OrdersCompleted,
		})
	}

	resp := &ProductFunnelResponse{Products: products}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// RefundRankingResponse 退款率排行响应
type RefundRankingResponse struct {
	Products []RefundRankingItem `json:"products"`
}

// RefundRankingItem 退款率排行项
type RefundRankingItem struct {
	ProductID  uint    `json:"product_id"`
	Refunded   int64   `json:"refunded"`
	Total      int64   `json:"total"`
	RefundRate float64 `json:"refund_rate"`
}

// GetRefundRanking 获取退款率排行
func (s *AnalyticsService) GetRefundRanking(ctx context.Context, input AnalyticsQueryInput, limit int) (*RefundRankingResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:refund:%s:%d:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix(), limit)
	var cached RefundRankingResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetRefundRanking(window.startAt, window.endAt, limit)
	if err != nil {
		return nil, err
	}

	products := make([]RefundRankingItem, 0, len(rows))
	for _, row := range rows {
		products = append(products, RefundRankingItem{
			ProductID:  row.ProductID,
			Refunded:   row.Refunded,
			Total:      row.Total,
			RefundRate: row.RefundRate,
		})
	}

	resp := &RefundRankingResponse{Products: products}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// CrossSellResponse 关联购买响应
type CrossSellResponse struct {
	Pairs []CrossSellPair `json:"pairs"`
}

// CrossSellPair 关联购买对
type CrossSellPair struct {
	ProductA     uint  `json:"product_a"`
	ProductB     uint  `json:"product_b"`
	CoOccurrence int64 `json:"co_occurrence"`
}

// GetCrossSell 获取关联购买
func (s *AnalyticsService) GetCrossSell(ctx context.Context, input AnalyticsQueryInput, limit int) (*CrossSellResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:crosssell:%s:%d:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix(), limit)
	var cached CrossSellResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetCrossSell(window.startAt, window.endAt, limit)
	if err != nil {
		return nil, err
	}

	pairs := make([]CrossSellPair, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, CrossSellPair{
			ProductA:     row.ProductA,
			ProductB:     row.ProductB,
			CoOccurrence: row.CoOccurrence,
		})
	}

	resp := &CrossSellResponse{Pairs: pairs}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// PriceBandResponse 价格带销量响应
type PriceBandResponse struct {
	Bands []PriceBandItem `json:"bands"`
}

// PriceBandItem 价格带项
type PriceBandItem struct {
	PriceBand     string  `json:"price_band"`
	OrderCount    int64   `json:"order_count"`
	TotalQuantity int64   `json:"total_quantity"`
	TotalRevenue  float64 `json:"total_revenue"`
}

// GetPriceBand 获取价格带销量分布
func (s *AnalyticsService) GetPriceBand(ctx context.Context, input AnalyticsQueryInput) (*PriceBandResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:priceband:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached PriceBandResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetPriceBandDistribution(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	bands := make([]PriceBandItem, 0, len(rows))
	for _, row := range rows {
		bands = append(bands, PriceBandItem{
			PriceBand:     row.PriceBand,
			OrderCount:    row.OrderCount,
			TotalQuantity: row.TotalQuantity,
			TotalRevenue:  row.TotalRevenue,
		})
	}

	resp := &PriceBandResponse{Bands: bands}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ---- 营收结构 ----

// ChannelRevenueResponse 支付通道营收响应
type ChannelRevenueResponse struct {
	Channels []ChannelRevenueItem `json:"channels"`
}

// ChannelRevenueItem 支付通道营收项
type ChannelRevenueItem struct {
	ChannelName  string  `json:"channel_name"`
	PaymentCount int64   `json:"payment_count"`
	TotalAmount  float64 `json:"total_amount"`
	SharePct     float64 `json:"share_pct"`
}

// GetRevenueByChannel 获取按支付通道的营收拆解
func (s *AnalyticsService) GetRevenueByChannel(ctx context.Context, input AnalyticsQueryInput) (*ChannelRevenueResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:channel:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached ChannelRevenueResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetRevenueByChannel(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	// Go 端计算占比（避免 SQL OVER() 兼容问题）
	var totalAmount float64
	for _, row := range rows {
		totalAmount += row.TotalAmount
	}

	channels := make([]ChannelRevenueItem, 0, len(rows))
	for _, row := range rows {
		share := 0.0
		if totalAmount > 0 {
			share = math.Round(row.TotalAmount/totalAmount*10000) / 100
		}
		channels = append(channels, ChannelRevenueItem{
			ChannelName:  row.ChannelName,
			PaymentCount: row.PaymentCount,
			TotalAmount:  row.TotalAmount,
			SharePct:     share,
		})
	}

	resp := &ChannelRevenueResponse{Channels: channels}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// MemberLevelRevenueResponse 会员等级消费贡献响应
type MemberLevelRevenueResponse struct {
	Levels []MemberLevelRevenueItem `json:"levels"`
}

// MemberLevelRevenueItem 会员等级消费贡献项
type MemberLevelRevenueItem struct {
	LevelID      uint    `json:"level_id"`
	LevelName    string  `json:"level_name"`
	OrderCount   int64   `json:"order_count"`
	UserCount    int64   `json:"user_count"`
	TotalRevenue float64 `json:"total_revenue"`
	AvgOrderVal  float64 `json:"avg_order_value"`
}

// GetRevenueByMemberLevel 获取按会员等级的消费贡献
func (s *AnalyticsService) GetRevenueByMemberLevel(ctx context.Context, input AnalyticsQueryInput) (*MemberLevelRevenueResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:level:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached MemberLevelRevenueResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetRevenueByMemberLevel(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	levels := make([]MemberLevelRevenueItem, 0, len(rows))
	for _, row := range rows {
		levels = append(levels, MemberLevelRevenueItem{
			LevelID:      row.LevelID,
			LevelName:    row.LevelName,
			OrderCount:   row.OrderCount,
			UserCount:    row.UserCount,
			TotalRevenue: row.TotalRevenue,
			AvgOrderVal:  row.AvgOrderVal,
		})
	}

	resp := &MemberLevelRevenueResponse{Levels: levels}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// CategoryRevenueResponse 分类营收响应
type CategoryRevenueResponse struct {
	Categories []CategoryRevenueItem `json:"categories"`
}

// CategoryRevenueItem 分类营收项
type CategoryRevenueItem struct {
	CategoryID   uint    `json:"category_id"`
	CategoryName string  `json:"category_name"`
	OrderCount   int64   `json:"order_count"`
	Revenue      float64 `json:"revenue"`
}

// GetRevenueByCategory 获取按分类的营收拆解
func (s *AnalyticsService) GetRevenueByCategory(ctx context.Context, input AnalyticsQueryInput) (*CategoryRevenueResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:category:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached CategoryRevenueResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetRevenueByCategory(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	categories := make([]CategoryRevenueItem, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, CategoryRevenueItem{
			CategoryID:   row.CategoryID,
			CategoryName: row.CategoryName,
			OrderCount:   row.OrderCount,
			Revenue:      row.Revenue,
		})
	}

	resp := &CategoryRevenueResponse{Categories: categories}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// RevenueHeatmapResponse 营收热力图响应
type RevenueHeatmapResponse struct {
	Cells []RevenueHeatmapCell `json:"cells"`
}

// RevenueHeatmapCell 热力图单元格
type RevenueHeatmapCell struct {
	DayOfWeek  int64   `json:"day_of_week"`
	HourOfDay  int64   `json:"hour_of_day"`
	OrderCount int64   `json:"order_count"`
	Revenue    float64 `json:"revenue"`
}

// GetRevenueHeatmap 获取营收时段热力图
func (s *AnalyticsService) GetRevenueHeatmap(ctx context.Context, input AnalyticsQueryInput) (*RevenueHeatmapResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:heatmap:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached RevenueHeatmapResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	loc := parseAnalyticsLocation(window.timezone)
	rows, err := s.repo.GetRevenueHeatmap(window.startAt, window.endAt, loc)
	if err != nil {
		return nil, err
	}

	cells := make([]RevenueHeatmapCell, 0, len(rows))
	for _, row := range rows {
		cells = append(cells, RevenueHeatmapCell{
			DayOfWeek:  row.DayOfWeek,
			HourOfDay:  row.HourOfDay,
			OrderCount: row.OrderCount,
			Revenue:    row.Revenue,
		})
	}

	resp := &RevenueHeatmapResponse{Cells: cells}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ---- 用户增长 ----

// UserGrowthResponse 用户增长响应
type UserGrowthResponse struct {
	Points []UserGrowthPoint `json:"points"`
}

// UserGrowthPoint 用户增长趋势点
type UserGrowthPoint struct {
	Day      string `json:"day"`
	NewUsers int64  `json:"new_users"`
}

// GetUserGrowth 获取新用户注册趋势
func (s *AnalyticsService) GetUserGrowth(ctx context.Context, input AnalyticsQueryInput) (*UserGrowthResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:growth:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached UserGrowthResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	loc := parseAnalyticsLocation(window.timezone)
	refTime := time.Now()
	rows, err := s.repo.GetUserGrowth(window.startAt, window.endAt, loc, refTime)
	if err != nil {
		return nil, err
	}

	points := make([]UserGrowthPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, UserGrowthPoint{
			Day:      row.Day,
			NewUsers: row.NewUsers,
		})
	}

	resp := &UserGrowthResponse{Points: points}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ActivationFunnelResponse 激活漏斗响应
type ActivationFunnelResponse struct {
	CohortSize   int64   `json:"cohort_size"`
	Activated1D  int64   `json:"activated_1d"`
	Activated7D  int64   `json:"activated_7d"`
	Activated30D int64   `json:"activated_30d"`
	NotActivated int64   `json:"not_activated"`
	Rate1D       float64 `json:"rate_1d"`
	Rate7D       float64 `json:"rate_7d"`
	Rate30D      float64 `json:"rate_30d"`
}

// GetActivationFunnel 获取注册→首购转化漏斗
func (s *AnalyticsService) GetActivationFunnel(ctx context.Context, input AnalyticsQueryInput) (*ActivationFunnelResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:activation:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached ActivationFunnelResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	rows, err := s.repo.GetActivationCohort(window.startAt, window.endAt)
	if err != nil {
		return nil, err
	}

	// Go 端计算天数差并分类统计（SQLite 子查询返回字符串，需解析）
	var cohortSize, activated1D, activated7D, activated30D int64
	cohortSize = int64(len(rows))
	for _, row := range rows {
		registeredAt, err := time.Parse("2006-01-02 15:04:05", parseSQLiteTime(row.RegisteredAt))
		if err != nil {
			continue
		}
		if row.FirstOrderAt == nil || *row.FirstOrderAt == "" {
			continue
		}
		firstOrderAt, err := time.Parse("2006-01-02 15:04:05", parseSQLiteTime(*row.FirstOrderAt))
		if err != nil {
			continue
		}
		daysDiff := firstOrderAt.Sub(registeredAt).Hours() / 24
		if daysDiff <= 1 {
			activated1D++
		}
		if daysDiff <= 7 {
			activated7D++
		}
		if daysDiff <= 30 {
			activated30D++
		}
	}

	resp := &ActivationFunnelResponse{
		CohortSize:   cohortSize,
		Activated1D:  activated1D,
		Activated7D:  activated7D,
		Activated30D: activated30D,
		NotActivated: cohortSize - activated30D,
		Rate1D:       safePercent(activated1D, cohortSize),
		Rate7D:       safePercent(activated7D, cohortSize),
		Rate30D:      safePercent(activated30D, cohortSize),
	}

	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// DAUTrendResponse DAU 趋势响应
type DAUTrendResponse struct {
	Points []DAUPoint `json:"points"`
}

// DAUPoint DAU 趋势点
type DAUPoint struct {
	Day string `json:"day"`
	DAU int64  `json:"dau"`
}

// GetDAUTrend 获取 DAU 趋势
func (s *AnalyticsService) GetDAUTrend(ctx context.Context, input AnalyticsQueryInput) (*DAUTrendResponse, error) {
	window, err := resolveAnalyticsWindow(input, time.Now())
	if err != nil {
		return nil, err
	}

	cacheKey := fmt.Sprintf("analytics:dau:%s:%d:%d", window.rangeKey, window.startAt.Unix(), window.endAt.Unix())
	var cached DAUTrendResponse
	if hit, _ := cache.GetJSON(ctx, cacheKey, &cached); hit {
		return &cached, nil
	}

	loc := parseAnalyticsLocation(window.timezone)
	refTime := time.Now()
	rows, err := s.repo.GetDAUTrend(window.startAt, window.endAt, loc, refTime)
	if err != nil {
		return nil, err
	}

	points := make([]DAUPoint, 0, len(rows))
	for _, row := range rows {
		points = append(points, DAUPoint{
			Day: row.Day,
			DAU: row.DAU,
		})
	}

	resp := &DAUTrendResponse{Points: points}
	_ = cache.SetJSON(ctx, cacheKey, resp, analyticsCacheTTL)
	return resp, nil
}

// ---- 工具函数 ----

// median 计算中位数
func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}

// safePercent 安全百分比计算
func safePercent(part, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(part)/float64(total)*10000) / 100
}

// parseAnalyticsLocation 解析时区
func parseAnalyticsLocation(tz string) *time.Location {
	if tz == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Local
	}
	return loc
}

// parseSQLiteTime 规范化 SQLite 子查询返回的 datetime 字符串
// SQLite 可能返回 "2026-07-09 16:48:48" 或 "2026-07-09T16:48:48Z" 等格式
func parseSQLiteTime(s string) string {
	// 截取前 19 个字符（标准 datetime 格式）
	if len(s) >= 19 {
		s = s[:19]
	}
	// 将 T 替换为空格
	for i, c := range s {
		if c == 'T' {
			s = s[:i] + " " + s[i+1:]
			break
		}
	}
	return s
}
