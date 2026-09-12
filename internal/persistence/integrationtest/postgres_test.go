//go:build integration
// +build integration

package integrationtest

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"

	"github.com/dujiao-next/internal/constants"
	productgormstore "github.com/dujiao-next/internal/modules/catalog/product/store/gormstore"
	contentcontract "github.com/dujiao-next/internal/modules/content/contract"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	contentgormstore "github.com/dujiao-next/internal/modules/content/infrastructure/gormstore"
	dashboardgormstore "github.com/dujiao-next/internal/modules/dashboard/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupPostgresIntegrationDB 初始化 PostgreSQL 集成测试数据库。
func setupPostgresIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("skip postgres integration test: TEST_POSTGRES_DSN is empty")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open postgres failed: %v", err)
	}

	cleanupModels := []interface{}{
		&orderdomain.OrderItem{},
		&paymentdomain.Payment{},
		&orderdomain.Order{},
		&contentdomain.PostProduct{},
		&productdomain.ProductSKU{},
		&productdomain.Product{},
		&categorydomain.Category{},
		&contentdomain.Banner{},
		&contentdomain.Post{},
		&contentdomain.PostCategory{},
	}
	_ = db.Migrator().DropTable(cleanupModels...)

	// product_skus：商品搜索条件里含有对 product_skus 的 EXISTS 子查询，缺表会让既有用例报错；
	// post_categories：文章分类筛选用例的依赖表。两者都必须随夹具一同迁移。
	if err := db.AutoMigrate(
		&categorydomain.Category{},
		&productdomain.Product{},
		&productdomain.ProductSKU{},
		&contentdomain.PostCategory{},
		&contentdomain.Post{},
		&contentdomain.PostProduct{},
		&contentdomain.Banner{},
		&orderdomain.Order{},
		&orderdomain.OrderItem{},
		&paymentdomain.Payment{},
	); err != nil {
		t.Fatalf("migrate postgres models failed: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Migrator().DropTable(cleanupModels...)
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestPostgresLocalizedJSONSearchRepositories(t *testing.T) {
	db := setupPostgresIntegrationDB(t)
	ctx := context.Background()

	category := &categorydomain.Category{
		Slug:     "pg-category",
		NameJSON: jsonmap.JSON{"zh-CN": "Postgres 分类"},
	}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	productRepo := productgormstore.NewProductStore(db)
	product := &productdomain.Product{
		CategoryID:       category.ID,
		Slug:             "pg-product-rocket",
		TitleJSON:        jsonmap.JSON{"zh-CN": "火箭会员"},
		DescriptionJSON:  jsonmap.JSON{"en-US": "rocket booster package"},
		PriceAmount:      money.FromDecimal(decimal.NewFromInt(99)),
		PurchaseType:     constants.ProductPurchaseMember,
		FulfillmentType:  constants.FulfillmentTypeManual,
		ManualStockTotal: 10,
		IsActive:         true,
	}
	if err := productRepo.Create(product); err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	productRows, productTotal, err := productRepo.List(productcontract.ListFilter{
		Page:   1,
		Search: "火箭",
	})
	if err != nil {
		t.Fatalf("product list search zh-CN failed: %v", err)
	}
	if productTotal != 1 || len(productRows) != 1 {
		t.Fatalf("product list search zh-CN want 1 got total=%d len=%d", productTotal, len(productRows))
	}

	productRows, productTotal, err = productRepo.List(productcontract.ListFilter{
		Page:   1,
		Search: "booster",
	})
	if err != nil {
		t.Fatalf("product list search en-US failed: %v", err)
	}
	if productTotal != 1 || len(productRows) != 1 {
		t.Fatalf("product list search en-US want 1 got total=%d len=%d", productTotal, len(productRows))
	}

	postStore := contentgormstore.NewPostStore(db)
	post := &contentdomain.Post{
		Slug:        "pg-post-release",
		Type:        "notice",
		TitleJSON:   jsonmap.JSON{"en-US": "Release Notes"},
		IsPublished: true,
	}
	if err := postStore.Create(ctx, post); err != nil {
		t.Fatalf("create post failed: %v", err)
	}

	postRows, postTotal, err := postStore.List(ctx, contentcontract.PostQuery{
		Page:   1,
		Search: "Release",
		Order:  contentcontract.PostOrderCreatedDesc,
	})
	if err != nil {
		t.Fatalf("post list search failed: %v", err)
	}
	if postTotal != 1 || len(postRows) != 1 {
		t.Fatalf("post list search want 1 got total=%d len=%d", postTotal, len(postRows))
	}

	bannerStore := contentgormstore.NewBannerStore(db)
	banner := &contentdomain.Banner{
		Name:      "pg-home-banner",
		Position:  "home",
		TitleJSON: jsonmap.JSON{"zh-CN": "春季大促"},
		Image:     "/banner.png",
		LinkType:  "none",
		IsActive:  true,
	}
	if err := bannerStore.Create(ctx, banner); err != nil {
		t.Fatalf("create banner failed: %v", err)
	}

	bannerRows, bannerTotal, err := bannerStore.List(ctx, contentcontract.BannerQuery{
		Page:   1,
		Search: "春季",
	})
	if err != nil {
		t.Fatalf("banner list search failed: %v", err)
	}
	if bannerTotal != 1 || len(bannerRows) != 1 {
		t.Fatalf("banner list search want 1 got total=%d len=%d", bannerTotal, len(bannerRows))
	}
}

func TestPostgresContentGormStoresPreserveQuerySemantics(t *testing.T) {
	db := setupPostgresIntegrationDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-time.Hour)

	posts := []contentdomain.Post{
		{Slug: "pg-content-older", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "模块化指南"}, IsPublished: true, PublishedAt: &older},
		{Slug: "pg-content-newer", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "模块化指南新版"}, IsPublished: true, PublishedAt: &newer},
		{Slug: "pg-content-draft", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "模块化指南草稿"}, IsPublished: false},
	}
	for index := range posts {
		if err := db.Create(&posts[index]).Error; err != nil {
			t.Fatalf("create postgres content post %q: %v", posts[index].Slug, err)
		}
	}

	postStore := contentgormstore.NewPostStore(db)
	modularPosts, modularTotal, err := postStore.List(ctx, contentcontract.PostQuery{
		Page:          1,
		PageSize:      20,
		Type:          constants.PostTypeBlog,
		Search:        "模块化",
		OnlyPublished: true,
		Order:         contentcontract.PostOrderPublishedDesc,
	})
	if err != nil {
		t.Fatalf("modular postgres post query: %v", err)
	}
	if modularTotal != 2 || len(modularPosts) != 2 {
		t.Fatalf("postgres post query should return two published rows, total=%d rows=%#v", modularTotal, modularPosts)
	}
	if modularPosts[0].Slug != "pg-content-newer" || modularPosts[1].Slug != "pg-content-older" {
		t.Fatalf("postgres post publication order mismatch: %#v", modularPosts)
	}

	rollbackPost := &contentdomain.Post{
		Slug:      "pg-content-rollback",
		Type:      constants.PostTypeBlog,
		TitleJSON: jsonmap.JSON{"zh-CN": "事务回滚"},
	}
	forcedRollback := errors.New("forced content transaction rollback")
	err = postStore.WithinPostWriteTransaction(ctx, func(posts contentcontract.PostStore, relations contentcontract.PostProductRelationStore) error {
		if createErr := posts.Create(ctx, rollbackPost); createErr != nil {
			return createErr
		}
		if relationErr := relations.SetRelatedProductIDs(ctx, rollbackPost.ID, []uint{42}); relationErr != nil {
			return relationErr
		}
		return forcedRollback
	})
	if !errors.Is(err, forcedRollback) {
		t.Fatalf("postgres content transaction should return callback error, got %v", err)
	}
	var rollbackPostCount int64
	if err := db.Model(&contentdomain.Post{}).Where("slug = ?", rollbackPost.Slug).Count(&rollbackPostCount).Error; err != nil {
		t.Fatalf("count rolled back postgres content post: %v", err)
	}
	if rollbackPostCount != 0 {
		t.Fatalf("postgres content transaction should roll back post, count=%d", rollbackPostCount)
	}

	banner := contentdomain.Banner{
		Name:      "pg-content-banner",
		Position:  constants.BannerPositionHomeHero,
		TitleJSON: jsonmap.JSON{"en-US": "Modular launch"},
		Image:     "/pg-content.png",
		LinkType:  constants.BannerLinkTypeNone,
		IsActive:  true,
	}
	if err := db.Create(&banner).Error; err != nil {
		t.Fatalf("create postgres content banner: %v", err)
	}
	bannerStore := contentgormstore.NewBannerStore(db)
	modularBanners, modularBannerTotal, err := bannerStore.List(ctx, contentcontract.BannerQuery{Page: 1, PageSize: 20, Search: "Modular"})
	if err != nil {
		t.Fatalf("modular postgres banner query: %v", err)
	}
	if modularBannerTotal != 1 || len(modularBanners) != 1 || modularBanners[0].ID != banner.ID {
		t.Fatalf("postgres banner localized search mismatch total=%d rows=%#v", modularBannerTotal, modularBanners)
	}

	inactiveBanner := &contentdomain.Banner{
		Name:     "pg-content-inactive-banner",
		Position: constants.BannerPositionHomeHero,
		Image:    "/pg-content-inactive.png",
		LinkType: constants.BannerLinkTypeNone,
		IsActive: false,
	}
	if err := bannerStore.Create(ctx, inactiveBanner); err != nil {
		t.Fatalf("create inactive postgres content banner: %v", err)
	}
	var reloadedInactiveBanner contentdomain.Banner
	if err := db.First(&reloadedInactiveBanner, inactiveBanner.ID).Error; err != nil {
		t.Fatalf("reload inactive postgres content banner: %v", err)
	}
	if inactiveBanner.IsActive || reloadedInactiveBanner.IsActive {
		t.Fatalf("explicit inactive postgres banner should stay inactive, returned=%t stored=%t", inactiveBanner.IsActive, reloadedInactiveBanner.IsActive)
	}
}

func TestPostgresDashboardQueries(t *testing.T) {
	db := setupPostgresIntegrationDB(t)
	repo := dashboardgormstore.New(db)
	now := time.Now().UTC().Truncate(time.Second)

	category := &categorydomain.Category{
		Slug:     "pg-dashboard-category",
		NameJSON: jsonmap.JSON{"zh-CN": "仪表盘分类"},
	}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category failed: %v", err)
	}

	product := &productdomain.Product{
		CategoryID:      category.ID,
		Slug:            "pg-dashboard-product",
		TitleJSON:       jsonmap.JSON{"zh-CN": "仪表盘商品"},
		PriceAmount:     money.FromDecimal(decimal.NewFromInt(120)),
		PurchaseType:    constants.ProductPurchaseMember,
		FulfillmentType: constants.FulfillmentTypeManual,
		IsActive:        true,
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create product failed: %v", err)
	}

	order := &orderdomain.Order{
		OrderNo:        "PG-ORDER-001",
		UserID:         1,
		Status:         constants.OrderStatusPaid,
		Currency:       "USD",
		OriginalAmount: money.FromDecimal(decimal.NewFromInt(120)),
		DiscountAmount: money.FromDecimal(decimal.Zero),
		TotalAmount:    money.FromDecimal(decimal.NewFromInt(120)),
		CreatedAt:      now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	orderItem := &orderdomain.OrderItem{
		OrderID:           order.ID,
		ProductID:         product.ID,
		TitleJSON:         jsonmap.JSON{"zh-CN": "仪表盘商品"},
		UnitPrice:         money.FromDecimal(decimal.NewFromInt(120)),
		Quantity:          2,
		TotalPrice:        money.FromDecimal(decimal.NewFromInt(240)),
		CouponDiscount:    money.FromDecimal(decimal.NewFromInt(20)),
		PromotionDiscount: money.FromDecimal(decimal.NewFromInt(10)),
		FulfillmentType:   constants.FulfillmentTypeManual,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := db.Create(orderItem).Error; err != nil {
		t.Fatalf("create order item failed: %v", err)
	}

	payment := &paymentdomain.Payment{
		OrderID:         order.ID,
		ChannelID:       1,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          money.FromDecimal(decimal.NewFromInt(120)),
		FeeRate:         money.FromDecimal(decimal.Zero),
		FeeAmount:       money.FromDecimal(decimal.Zero),
		Currency:        "USD",
		Status:          constants.PaymentStatusSuccess,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	startAt := now.Add(-time.Hour)
	endAt := now.Add(time.Hour)

	topProducts, err := repo.GetTopProducts(startAt, endAt, 5)
	if err != nil {
		t.Fatalf("get top products failed: %v", err)
	}
	if len(topProducts) != 1 {
		t.Fatalf("top products len want 1 got %d", len(topProducts))
	}
	if topProducts[0].Title != "仪表盘商品" {
		t.Fatalf("top product title want 仪表盘商品 got %s", topProducts[0].Title)
	}

	orderTrends, err := repo.GetOrderTrends(startAt, endAt)
	if err != nil {
		t.Fatalf("get order trends failed: %v", err)
	}
	if len(orderTrends) == 0 {
		t.Fatalf("order trends should not be empty")
	}
	if strings.TrimSpace(orderTrends[0].Day) == "" {
		t.Fatalf("order trend day should not be empty")
	}

	paymentTrends, err := repo.GetPaymentTrends(startAt, endAt)
	if err != nil {
		t.Fatalf("get payment trends failed: %v", err)
	}
	if len(paymentTrends) == 0 {
		t.Fatalf("payment trends should not be empty")
	}
	if strings.TrimSpace(paymentTrends[0].Day) == "" {
		t.Fatalf("payment trend day should not be empty")
	}
}

// 文章列表的分类筛选在 PostgreSQL 上需与 SQLite 语义一致：
// CategoryIDs 走 IN、CategoryID 走等值，并与发布态按 AND 组合。
func TestPostgresPostListFiltersByCategory(t *testing.T) {
	db := setupPostgresIntegrationDB(t)
	ctx := context.Background()
	publishedAt := time.Now().UTC()

	categoryStore := contentgormstore.NewPostCategoryStore(db)
	root := &contentdomain.PostCategory{Slug: "pg-post-category-root", NameJSON: jsonmap.JSON{"zh-CN": "PG 文章分类"}, IsActive: true}
	if err := categoryStore.Create(ctx, root); err != nil {
		t.Fatalf("create postgres post category failed: %v", err)
	}
	rootID := root.ID
	child := &contentdomain.PostCategory{Slug: "pg-post-category-child", NameJSON: jsonmap.JSON{"zh-CN": "PG 文章子分类"}, ParentID: &rootID, IsActive: true}
	if err := categoryStore.Create(ctx, child); err != nil {
		t.Fatalf("create postgres post child category failed: %v", err)
	}
	childID := child.ID

	posts := []contentdomain.Post{
		{Slug: "pg-post-root", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "PG 根分类文章"}, CategoryID: &rootID, IsPublished: true, PublishedAt: &publishedAt},
		{Slug: "pg-post-child", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "PG 子分类文章"}, CategoryID: &childID, IsPublished: true, PublishedAt: &publishedAt},
		{Slug: "pg-post-none", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "PG 未分类文章"}, IsPublished: true, PublishedAt: &publishedAt},
		{Slug: "pg-post-draft", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "PG 子分类草稿"}, CategoryID: &childID, IsPublished: false},
	}
	for index := range posts {
		if err := db.Create(&posts[index]).Error; err != nil {
			t.Fatalf("create postgres post %q failed: %v", posts[index].Slug, err)
		}
	}

	store := contentgormstore.NewPostStore(db)
	listSlugs := func(query contentcontract.PostQuery) map[string]bool {
		t.Helper()
		query.Page = 1
		query.PageSize = 50
		rows, _, err := store.List(ctx, query)
		if err != nil {
			t.Fatalf("postgres post list failed: %v", err)
		}
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.Slug] = true
		}
		return got
	}

	got := listSlugs(contentcontract.PostQuery{CategoryID: strconv.FormatUint(uint64(childID), 10)})
	if !got["pg-post-child"] || !got["pg-post-draft"] || len(got) != 2 {
		t.Fatalf("postgres CategoryID equality mismatch: %+v", got)
	}

	got = listSlugs(contentcontract.PostQuery{CategoryIDs: []uint{rootID, childID}, OnlyPublished: true})
	if !got["pg-post-root"] || !got["pg-post-child"] || got["pg-post-draft"] || got["pg-post-none"] || len(got) != 2 {
		t.Fatalf("postgres CategoryIDs IN + OnlyPublished mismatch: %+v", got)
	}

	got = listSlugs(contentcontract.PostQuery{})
	if len(got) != 4 {
		t.Fatalf("postgres without category filter should return all posts: %+v", got)
	}
}
