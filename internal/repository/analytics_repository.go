package repository

import (
	"fmt"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
)

// AnalyticsRepository 数据分析聚合查询接口
type AnalyticsRepository interface {
	// 客户价值
	GetCustomerLTV(startAt, endAt time.Time) ([]AnalyticsLTVRow, error)
	GetARPUSeries(startAt, endAt time.Time, loc *time.Location, refTime time.Time) ([]AnalyticsARPURow, error)
	GetRepurchaseRate(startAt, endAt time.Time) (*AnalyticsRepurchaseRow, error)
	GetChurnRiskUsers(cutoffTime time.Time, minLTV float64, limit int) ([]AnalyticsChurnRiskRow, error)

	// 商品分析
	GetProductFunnel(startAt, endAt time.Time) ([]AnalyticsProductFunnelRow, error)
	GetRefundRanking(startAt, endAt time.Time, limit int) ([]AnalyticsRefundRankingRow, error)
	GetCrossSell(startAt, endAt time.Time, limit int) ([]AnalyticsCrossSellRow, error)
	GetPriceBandDistribution(startAt, endAt time.Time) ([]AnalyticsPriceBandRow, error)

	// 营收结构
	GetRevenueByChannel(startAt, endAt time.Time) ([]AnalyticsChannelRevenueRow, error)
	GetRevenueByMemberLevel(startAt, endAt time.Time) ([]AnalyticsMemberLevelRevenueRow, error)
	GetRevenueByCategory(startAt, endAt time.Time) ([]AnalyticsCategoryRevenueRow, error)
	GetRevenueHeatmap(startAt, endAt time.Time, loc *time.Location) ([]AnalyticsHeatmapRow, error)

	// 用户增长
	GetUserGrowth(startAt, endAt time.Time, loc *time.Location, refTime time.Time) ([]AnalyticsUserGrowthRow, error)
	GetActivationCohort(startAt, endAt time.Time) ([]AnalyticsActivationCohortRow, error)
	GetDAUTrend(startAt, endAt time.Time, loc *time.Location, refTime time.Time) ([]AnalyticsDAURow, error)
}

// ---- 客户价值 ----

// AnalyticsLTVRow 客户 LTV 分布原始行
type AnalyticsLTVRow struct {
	MemberLevelID uint    `gorm:"column:member_level_id"`
	UserCount     int64   `gorm:"column:user_count"`
	AvgLTV        float64 `gorm:"column:avg_ltv"`
	MaxLTV        float64 `gorm:"column:max_ltv"`
	// LTV 原始值列表由 service 层读取并计算中位数
}

// AnalyticsARPURow ARPU 月趋势
type AnalyticsARPURow struct {
	Month       string  `gorm:"column:month"`
	Revenue     float64 `gorm:"column:revenue"`
	PayingUsers int64   `gorm:"column:paying_users"`
	ARPU        float64 `gorm:"column:arpu"`
}

// AnalyticsRepurchaseRow 复购率
type AnalyticsRepurchaseRow struct {
	Once           int64   `gorm:"column:once"`
	Twice          int64   `gorm:"column:twice"`
	ThreePlus      int64   `gorm:"column:three_plus"`
	TotalUsers     int64   `gorm:"column:total_users"`
	RepurchaseRate float64 `gorm:"column:repurchase_rate"`
}

// AnalyticsChurnRiskRow 流失预警用户
// 注意：SQLite 子查询返回 datetime 为字符串
type AnalyticsChurnRiskRow struct {
	UserID        uint    `gorm:"column:user_id"`
	Email         string  `gorm:"column:email"`
	MemberLevelID uint    `gorm:"column:member_level_id"`
	LastOrderAt   string  `gorm:"column:last_order_at"`
	LifetimeValue float64 `gorm:"column:lifetime_value"`
	TotalOrders   int64   `gorm:"column:total_orders"`
}

// ---- 商品分析 ----

// AnalyticsProductFunnelRow 商品转化漏斗
type AnalyticsProductFunnelRow struct {
	ProductID        uint  `gorm:"column:product_id"`
	OrdersCreated    int64 `gorm:"column:orders_created"`
	OrdersPaid       int64 `gorm:"column:orders_paid"`
	OrdersCompleted  int64 `gorm:"column:orders_completed"`
}

// AnalyticsRefundRankingRow 退款率排行
type AnalyticsRefundRankingRow struct {
	ProductID  uint    `gorm:"column:product_id"`
	Refunded   int64   `gorm:"column:refunded"`
	Total      int64   `gorm:"column:total"`
	RefundRate float64 `gorm:"column:refund_rate"`
}

// AnalyticsCrossSellRow 关联购买
type AnalyticsCrossSellRow struct {
	ProductA      uint  `gorm:"column:product_a"`
	ProductB      uint  `gorm:"column:product_b"`
	CoOccurrence  int64 `gorm:"column:co_occurrence"`
}

// AnalyticsPriceBandRow 价格带销量
type AnalyticsPriceBandRow struct {
	PriceBand     string  `gorm:"column:price_band"`
	OrderCount    int64   `gorm:"column:order_count"`
	TotalQuantity int64   `gorm:"column:total_quantity"`
	TotalRevenue  float64 `gorm:"column:total_revenue"`
}

// ---- 营收结构 ----

// AnalyticsChannelRevenueRow 支付通道营收
type AnalyticsChannelRevenueRow struct {
	ChannelName  string  `gorm:"column:channel_name"`
	PaymentCount int64   `gorm:"column:payment_count"`
	TotalAmount  float64 `gorm:"column:total_amount"`
}

// AnalyticsMemberLevelRevenueRow 会员等级消费贡献
type AnalyticsMemberLevelRevenueRow struct {
	LevelID      uint    `gorm:"column:level_id"`
	LevelName    string  `gorm:"column:level_name"`
	OrderCount   int64   `gorm:"column:order_count"`
	UserCount    int64   `gorm:"column:user_count"`
	TotalRevenue float64 `gorm:"column:total_revenue"`
	AvgOrderVal  float64 `gorm:"column:avg_order_value"`
}

// AnalyticsCategoryRevenueRow 分类营收
type AnalyticsCategoryRevenueRow struct {
	CategoryID   uint    `gorm:"column:category_id"`
	CategoryName string  `gorm:"column:category_name"`
	OrderCount   int64   `gorm:"column:order_count"`
	Revenue      float64 `gorm:"column:revenue"`
}

// AnalyticsHeatmapRow 营收时段热力图
type AnalyticsHeatmapRow struct {
	DayOfWeek  int64   `gorm:"column:day_of_week"`
	HourOfDay  int64   `gorm:"column:hour_of_day"`
	OrderCount int64   `gorm:"column:order_count"`
	Revenue    float64 `gorm:"column:revenue"`
}

// ---- 用户增长 ----

// AnalyticsUserGrowthRow 新用户注册趋势
type AnalyticsUserGrowthRow struct {
	Day      string `gorm:"column:day"`
	NewUsers int64  `gorm:"column:new_users"`
}

// AnalyticsActivationCohortRow 激活转化队列原始行
// 返回 user_id + 注册时间 + 首购时间，Go 端计算天数差
// 注意：SQLite 子查询返回 datetime 为字符串，使用 string 扫描后 Go 端解析
type AnalyticsActivationCohortRow struct {
	UserID        uint   `gorm:"column:user_id"`
	RegisteredAt  string `gorm:"column:registered_at"`
	FirstOrderAt  *string `gorm:"column:first_order_at"`
}

// AnalyticsDAURow 日活跃用户
type AnalyticsDAURow struct {
	Day string `gorm:"column:day"`
	DAU int64  `gorm:"column:dau"`
}

// ---- GORM 实现 ----

// GormAnalyticsRepository GORM 数据分析聚合实现
type GormAnalyticsRepository struct {
	db *gorm.DB
}

// NewAnalyticsRepository 创建数据分析仓库
func NewAnalyticsRepository(db *gorm.DB) *GormAnalyticsRepository {
	return &GormAnalyticsRepository{db: db}
}

// paidIn 返回已支付状态 IN 子句常量
func paidIn() string {
	return quotedStatusList([]string{
		constants.OrderStatusPaid,
		constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered,
		constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered,
		constants.OrderStatusCompleted,
	})
}

// ---- 客户价值实现 ----

// GetCustomerLTV 获取按会员等级的 LTV 分布
func (r *GormAnalyticsRepository) GetCustomerLTV(startAt, endAt time.Time) ([]AnalyticsLTVRow, error) {
	rows := make([]AnalyticsLTVRow, 0)

	// 子查询：每个用户的 LTV
	subQuery := r.db.Model(&models.Order{}).
		Select("user_id, COALESCE(SUM(total_amount), 0) AS ltv").
		Where("parent_id IS NULL AND status IN ?", []string{
			constants.OrderStatusPaid, constants.OrderStatusFulfilling,
			constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
			constants.OrderStatusDelivered, constants.OrderStatusCompleted,
		}).
		Group("user_id")

	// 按 member_level_id 分组聚合
	if err := r.db.Table("(?) AS ltv_sub", subQuery).
		Select(`u.member_level_id, COUNT(*) AS user_count, ROUND(AVG(ltv_sub.ltv), 2) AS avg_ltv, ROUND(MAX(ltv_sub.ltv), 2) AS max_ltv`).
		Joins("JOIN users u ON u.id = ltv_sub.user_id").
		Where("u.deleted_at IS NULL").
		Group("u.member_level_id").
		Order("avg_ltv DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	// 查询每个等级的用户 LTV 原始值（用于 Go 端计算中位数）
	// 注意：此查询只返回 row 结构，中位数由 service 层二次查询计算
	return rows, nil
}

// GetLTVValuesByLevel 按等级获取所有用户的 LTV 原始值（service 层计算中位数）
func (r *GormAnalyticsRepository) GetLTVValuesByLevel(startAt, endAt time.Time) (map[uint][]float64, error) {
	type ltvRaw struct {
		MemberLevelID uint    `gorm:"column:member_level_id"`
		LTV           float64 `gorm:"column:ltv"`
	}
	rawRows := make([]ltvRaw, 0)

	subQuery := r.db.Model(&models.Order{}).
		Select("user_id, COALESCE(SUM(total_amount), 0) AS ltv").
		Where("parent_id IS NULL AND status IN ?", []string{
			constants.OrderStatusPaid, constants.OrderStatusFulfilling,
			constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
			constants.OrderStatusDelivered, constants.OrderStatusCompleted,
		}).
		Group("user_id")

	if err := r.db.Table("(?) AS ltv_sub", subQuery).
		Select("u.member_level_id, ltv_sub.ltv").
		Joins("JOIN users u ON u.id = ltv_sub.user_id").
		Where("u.deleted_at IS NULL").
		Scan(&rawRows).Error; err != nil {
		return nil, err
	}

	result := make(map[uint][]float64)
	for _, item := range rawRows {
		result[item.MemberLevelID] = append(result[item.MemberLevelID], item.LTV)
	}
	return result, nil
}

// GetARPUSeries 获取 ARPU 月趋势
func (r *GormAnalyticsRepository) GetARPUSeries(startAt, endAt time.Time, loc *time.Location, refTime time.Time) ([]AnalyticsARPURow, error) {
	monthExpr := monthGroupExpr(r.db, "created_at", loc, refTime)
	rows := make([]AnalyticsARPURow, 0)

	selectSQL := fmt.Sprintf(`
		%s AS month,
		ROUND(SUM(total_amount), 2) AS revenue,
		COUNT(DISTINCT user_id) AS paying_users,
		ROUND(SUM(total_amount) / NULLIF(COUNT(DISTINCT user_id), 0), 2) AS arpu
	`, monthExpr)

	if err := r.db.Model(&models.Order{}).
		Select(selectSQL).
		Where("parent_id IS NULL AND status IN ? AND created_at >= ? AND created_at < ?",
			[]string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
				constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
				constants.OrderStatusDelivered, constants.OrderStatusCompleted},
			startAt, endAt).
		Group(monthExpr).
		Order("month ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRepurchaseRate 获取复购率
func (r *GormAnalyticsRepository) GetRepurchaseRate(startAt, endAt time.Time) (*AnalyticsRepurchaseRow, error) {
	result := &AnalyticsRepurchaseRow{}

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	// SQLite/Postgres 兼容：使用 SUM(CASE WHEN ...) 而非 FILTER
	selectSQL := fmt.Sprintf(`
		COALESCE(SUM(CASE WHEN oc.order_count = 1 THEN 1 ELSE 0 END), 0) AS once,
		COALESCE(SUM(CASE WHEN oc.order_count = 2 THEN 1 ELSE 0 END), 0) AS twice,
		COALESCE(SUM(CASE WHEN oc.order_count >= 3 THEN 1 ELSE 0 END), 0) AS three_plus,
		COUNT(*) AS total_users,
		ROUND(100.0 * COALESCE(SUM(CASE WHEN oc.order_count >= 2 THEN 1 ELSE 0 END), 0) / NULLIF(COUNT(*), 0), 2) AS repurchase_rate
	`)
	subQuery := r.db.Model(&models.Order{}).
		Select("user_id, COUNT(*) AS order_count").
		Where("parent_id IS NULL AND status IN ? AND created_at >= ? AND created_at < ?",
			paidStatuses, startAt, endAt).
		Group("user_id")

	if err := r.db.Table("(?) AS oc", subQuery).
		Select(selectSQL).
		Scan(result).Error; err != nil {
		return nil, err
	}
	return result, nil
}

// GetChurnRiskUsers 获取流失预警用户
func (r *GormAnalyticsRepository) GetChurnRiskUsers(cutoffTime time.Time, minLTV float64, limit int) ([]AnalyticsChurnRiskRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows := make([]AnalyticsChurnRiskRow, 0)

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	// 使用 Raw SQL 避免 SQLite 子查询 datetime 扫描问题
	sql := `
		SELECT u.id AS user_id, u.email, u.member_level_id, oa.last_order_at, oa.lifetime_value, oa.total_orders
		FROM users u
		JOIN (
			SELECT user_id, MAX(created_at) AS last_order_at, SUM(total_amount) AS lifetime_value, COUNT(id) AS total_orders
			FROM orders
			WHERE parent_id IS NULL AND status IN (?,?,?,?,?,?) AND deleted_at IS NULL
			GROUP BY user_id
			HAVING SUM(total_amount) > ? AND MAX(created_at) < ?
		) oa ON oa.user_id = u.id
		WHERE u.deleted_at IS NULL
		ORDER BY oa.lifetime_value DESC
		LIMIT ?
	`

	if err := r.db.Raw(sql,
		paidStatuses[0], paidStatuses[1], paidStatuses[2], paidStatuses[3],
		paidStatuses[4], paidStatuses[5],
		minLTV, cutoffTime, limit,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---- 商品分析实现 ----

// GetProductFunnel 获取商品转化漏斗
func (r *GormAnalyticsRepository) GetProductFunnel(startAt, endAt time.Time) ([]AnalyticsProductFunnelRow, error) {
	rows := make([]AnalyticsProductFunnelRow, 0)

	// FILTER → SUM(CASE WHEN ...)
	selectSQL := fmt.Sprintf(`
		order_items.product_id,
		COUNT(DISTINCT o.id) AS orders_created,
		COALESCE(SUM(CASE WHEN o.status IN (%s) THEN 1 ELSE 0 END), 0) AS orders_paid,
		COALESCE(SUM(CASE WHEN o.status = '%s' THEN 1 ELSE 0 END), 0) AS orders_completed
	`, paidIn(), constants.OrderStatusCompleted)

	if err := r.db.Model(&models.OrderItem{}).
		Select(selectSQL).
		Joins("JOIN orders o ON o.id = order_items.order_id AND o.parent_id IS NULL").
		Where("o.created_at >= ? AND o.created_at < ?", startAt, endAt).
		Group("order_items.product_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRefundRanking 获取退款率排行
func (r *GormAnalyticsRepository) GetRefundRanking(startAt, endAt time.Time, limit int) ([]AnalyticsRefundRankingRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows := make([]AnalyticsRefundRankingRow, 0)

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted, constants.OrderStatusRefunded}

	refundedExpr := fmt.Sprintf("COALESCE(SUM(CASE WHEN o.status = '%s' THEN 1 ELSE 0 END), 0)", constants.OrderStatusRefunded)

	selectSQL := fmt.Sprintf(`
		order_items.product_id,
		%s AS refunded,
		COUNT(DISTINCT o.id) AS total,
		ROUND(100.0 * %s / NULLIF(COUNT(DISTINCT o.id), 0), 2) AS refund_rate
	`, refundedExpr, refundedExpr)

	if err := r.db.Model(&models.OrderItem{}).
		Select(selectSQL).
		Joins("JOIN orders o ON o.id = order_items.order_id AND o.parent_id IS NULL").
		Where("o.created_at >= ? AND o.created_at < ? AND o.status IN ?", startAt, endAt, paidStatuses).
		Group("order_items.product_id").
		Having("COUNT(DISTINCT o.id) >= 10"). // 最小样本量
		Order("refund_rate DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetCrossSell 获取关联购买（商品共现对）
func (r *GormAnalyticsRepository) GetCrossSell(startAt, endAt time.Time, limit int) ([]AnalyticsCrossSellRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows := make([]AnalyticsCrossSellRow, 0)

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	selectSQL := `
		a.product_id AS product_a,
		b.product_id AS product_b,
		COUNT(*) AS co_occurrence
	`

	if err := r.db.Model(&models.OrderItem{}).
		Table("order_items a").
		Select(selectSQL).
		Joins("JOIN order_items b ON a.order_id = b.order_id AND a.product_id < b.product_id").
		Joins("JOIN orders o ON o.id = a.order_id").
		Where("o.status IN ? AND o.created_at >= ? AND o.created_at < ?", paidStatuses, startAt, endAt).
		Group("a.product_id, b.product_id").
		Order("co_occurrence DESC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetPriceBandDistribution 获取价格带销量分布
func (r *GormAnalyticsRepository) GetPriceBandDistribution(startAt, endAt time.Time) ([]AnalyticsPriceBandRow, error) {
	rows := make([]AnalyticsPriceBandRow, 0)

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	// CASE WHEN 在 SQLite/Postgres 均可用，无需方言
	selectSQL := `
		CASE
			WHEN order_items.unit_price < 10 THEN '0-10'
			WHEN order_items.unit_price < 50 THEN '10-50'
			WHEN order_items.unit_price < 100 THEN '50-100'
			WHEN order_items.unit_price < 500 THEN '100-500'
			ELSE '500+'
		END AS price_band,
		COUNT(*) AS order_count,
		COALESCE(SUM(order_items.quantity), 0) AS total_quantity,
		ROUND(COALESCE(SUM(order_items.total_price), 0), 2) AS total_revenue
	`

	if err := r.db.Model(&models.OrderItem{}).
		Select(selectSQL).
		Joins("JOIN orders o ON o.id = order_items.order_id").
		Where("o.status IN ? AND o.created_at >= ? AND o.created_at < ?", paidStatuses, startAt, endAt).
		Group("price_band").
		Order("MIN(order_items.unit_price) ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---- 营收结构实现 ----

// GetRevenueByChannel 获取按支付通道的营收拆解
func (r *GormAnalyticsRepository) GetRevenueByChannel(startAt, endAt time.Time) ([]AnalyticsChannelRevenueRow, error) {
	rows := make([]AnalyticsChannelRevenueRow, 0)

	if err := r.db.Model(&models.Payment{}).
		Select(`COALESCE(payment_channels.name, '') AS channel_name,
			COUNT(payments.id) AS payment_count,
			ROUND(COALESCE(SUM(payments.amount), 0), 2) AS total_amount`).
		Joins("JOIN payment_channels ON payment_channels.id = payments.channel_id").
		Where("payments.status = ? AND payments.created_at >= ? AND payments.created_at < ?",
			constants.PaymentStatusSuccess, startAt, endAt).
		Group("payment_channels.name").
		Order("total_amount DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRevenueByMemberLevel 获取按会员等级的消费贡献
func (r *GormAnalyticsRepository) GetRevenueByMemberLevel(startAt, endAt time.Time) ([]AnalyticsMemberLevelRevenueRow, error) {
	rows := make([]AnalyticsMemberLevelRevenueRow, 0)

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	if err := r.db.Model(&models.Order{}).
		Select(`COALESCE(ml.id, 0) AS level_id,
			ml.name_json AS level_name,
			COUNT(DISTINCT orders.id) AS order_count,
			COUNT(DISTINCT orders.user_id) AS user_count,
			ROUND(COALESCE(SUM(orders.total_amount), 0), 2) AS total_revenue,
			ROUND(COALESCE(AVG(orders.total_amount), 0), 2) AS avg_order_value`).
		Joins("JOIN users u ON u.id = orders.user_id").
		Joins("LEFT JOIN member_levels ml ON ml.id = u.member_level_id").
		Where("orders.parent_id IS NULL AND orders.status IN ? AND orders.created_at >= ? AND orders.created_at < ?",
			paidStatuses, startAt, endAt).
		Group("ml.id, ml.name_json").
		Order("total_revenue DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRevenueByCategory 获取按分类的营收拆解
func (r *GormAnalyticsRepository) GetRevenueByCategory(startAt, endAt time.Time) ([]AnalyticsCategoryRevenueRow, error) {
	rows := make([]AnalyticsCategoryRevenueRow, 0)

	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	if err := r.db.Model(&models.OrderItem{}).
		Select(`c.id AS category_id,
			c.name_json AS category_name,
			COUNT(DISTINCT orders.id) AS order_count,
			ROUND(COALESCE(SUM(order_items.total_price - order_items.coupon_discount), 0), 2) AS revenue`).
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Joins("JOIN products p ON p.id = order_items.product_id").
		Joins("JOIN categories c ON c.id = p.category_id").
		Where("orders.status IN ? AND orders.created_at >= ? AND orders.created_at < ?",
			paidStatuses, startAt, endAt).
		Group("c.id, c.name_json").
		Order("revenue DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetRevenueHeatmap 获取周/日时段营收热力图
func (r *GormAnalyticsRepository) GetRevenueHeatmap(startAt, endAt time.Time, loc *time.Location) ([]AnalyticsHeatmapRow, error) {
	rows := make([]AnalyticsHeatmapRow, 0)

	dowExpr := dayOfWeekExpr(r.db, "created_at", loc)
	hourExpr := hourExpr(r.db, "created_at", loc)
	paidStatuses := []string{constants.OrderStatusPaid, constants.OrderStatusFulfilling,
		constants.OrderStatusPartiallyDelivered, constants.OrderStatusPartiallyRefunded,
		constants.OrderStatusDelivered, constants.OrderStatusCompleted}

	selectSQL := fmt.Sprintf(`
		%s AS day_of_week,
		%s AS hour_of_day,
		COUNT(*) AS order_count,
		ROUND(COALESCE(SUM(total_amount), 0), 2) AS revenue
	`, dowExpr, hourExpr)

	if err := r.db.Model(&models.Order{}).
		Select(selectSQL).
		Where("parent_id IS NULL AND status IN ? AND created_at >= ? AND created_at < ?",
			paidStatuses, startAt, endAt).
		Group("day_of_week, hour_of_day").
		Order("day_of_week ASC, hour_of_day ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---- 用户增长实现 ----

// GetUserGrowth 获取新用户注册趋势
func (r *GormAnalyticsRepository) GetUserGrowth(startAt, endAt time.Time, loc *time.Location, refTime time.Time) ([]AnalyticsUserGrowthRow, error) {
	dayExpr := dateGroupExpr(r.db, "created_at", loc, refTime)
	rows := make([]AnalyticsUserGrowthRow, 0)

	selectSQL := fmt.Sprintf(`
		%s AS day,
		COUNT(*) AS new_users
	`, dayExpr)

	if err := r.db.Model(&models.User{}).
		Select(selectSQL).
		Where("created_at >= ? AND created_at < ?", startAt, endAt).
		Group(dayExpr).
		Order("day ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetActivationCohort 获取激活转化队列原始数据
// 返回 user_id + 注册时间 + 首购时间，service 层计算天数差和分类统计
// 注意：使用 Raw SQL 避免 GORM Model+子查询+Joins 在双数据库下的列别名冲突
func (r *GormAnalyticsRepository) GetActivationCohort(startAt, endAt time.Time) ([]AnalyticsActivationCohortRow, error) {
	rows := make([]AnalyticsActivationCohortRow, 0)

	sql := `
		SELECT u.id AS user_id, u.created_at AS registered_at, fo.first_order_at
		FROM users u
		LEFT JOIN (
			SELECT user_id, MIN(created_at) AS first_order_at
			FROM orders
			WHERE parent_id IS NULL
			  AND status IN ('paid','fulfilling','partially_delivered','partially_refunded','delivered','completed')
			  AND deleted_at IS NULL
			GROUP BY user_id
		) fo ON fo.user_id = u.id
		WHERE u.created_at >= ? AND u.created_at < ? AND u.deleted_at IS NULL
	`

	if err := r.db.Raw(sql, startAt, endAt).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// GetDAUTrend 获取 DAU 趋势
func (r *GormAnalyticsRepository) GetDAUTrend(startAt, endAt time.Time, loc *time.Location, refTime time.Time) ([]AnalyticsDAURow, error) {
	dayExpr := dateGroupExpr(r.db, "created_at", loc, refTime)
	rows := make([]AnalyticsDAURow, 0)

	selectSQL := fmt.Sprintf(`
		%s AS day,
		COUNT(DISTINCT user_id) AS dau
	`, dayExpr)

	if err := r.db.Model(&models.Order{}).
		Select(selectSQL).
		Where("parent_id IS NULL AND created_at >= ? AND created_at < ?", startAt, endAt).
		Group(dayExpr).
		Order("day ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
