// Command seeddata 生成会员等级/测试会员数据 或 随机化商品类型+生成卡密库存
// 用法: go run ./cmd/seeddata [mode]
//
//	mode=members  → 生成会员等级与测试会员（默认）
//	mode=randomize → 随机化所有商品 purchase_type/fulfillment_type，并为 auto 商品生成卡密库存
//	mode=orders → 生成 500 个测试订单
//	mode=analytics → 全链路（members → orders → fulfillments → refunds → login_logs）
//	mode=tags → 为商品填充产品标签
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dujiao-next/internal/constants"
	auditlogdomain "github.com/dujiao-next/internal/modules/auditlog/domain"
	cardsecretdomain "github.com/dujiao-next/internal/modules/cardsecret/domain"
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	memberleveldomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/jsonslice"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	sqliteDSN         = "db/dujiao.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	pgDSN             = "host=/Applications/ServBay/tmp user=zing password=ServBay.dev dbname=dujiao sslmode=disable"
	userCount         = 100
	seedEmail         = "seeduser%03d@example.com"
	rawPasswd         = "Password123!"
	cardSecretsPerSKU = 50
)

type levelDef struct {
	slug      string
	nameZH    string
	nameEN    string
	icon      string
	discount  int64
	recharge  int64
	spend     int64
	sort      int
	isDefault bool
}

var levels = []levelDef{
	{"bronze", "青铜会员", "Bronze", "🥉", 100, 0, 0, 0, true},
	{"silver", "白银会员", "Silver", "🥈", 98, 100, 100, 1, false},
	{"gold", "黄金会员", "Gold", "🥇", 95, 500, 500, 2, false},
	{"platinum", "铂金会员", "Platinum", "💎", 92, 2000, 2000, 3, false},
	{"diamond", "钻石会员", "Diamond", "👑", 88, 5000, 5000, 4, false},
}

func moneyAmt(v int64) money.Amount {
	return money.FromDecimal(decimal.NewFromInt(v))
}

func open(driver, dsn string) (*gorm.DB, error) {
	cfg := &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Warn),
		NowFunc: func() time.Time { return time.Now().UTC() },
	}
	if driver == "sqlite" {
		return gorm.Open(sqlite.Open(dsn), cfg)
	}
	return gorm.Open(postgres.Open(dsn), cfg)
}

// ---------- 会员等级/测试会员 ----------

func seedMembers(db *gorm.DB, label string) error {
	// 清理旧 seed 会员
	if err := db.Unscoped().Where("email LIKE ?", "seeduser%@example.com").Delete(&userdomain.User{}).Error; err != nil {
		return fmt.Errorf("清理旧会员失败: %w", err)
	}
	// 幂等写入等级
	levelIDs := make([]uint, 0, len(levels))
	for _, l := range levels {
		var existing memberleveldomain.MemberLevel
		err := db.Unscoped().Where("slug = ?", l.slug).First(&existing).Error
		ml := memberleveldomain.MemberLevel{
			NameJSON:          jsonmap.JSON{"zh-CN": l.nameZH, "en-US": l.nameEN, "zh-TW": l.nameZH},
			Slug:              l.slug,
			Icon:              l.icon,
			DiscountRate:      moneyAmt(l.discount),
			RechargeThreshold: moneyAmt(l.recharge),
			SpendThreshold:    moneyAmt(l.spend),
			IsDefault:         l.isDefault,
			SortOrder:         l.sort,
			IsActive:          true,
		}
		if err == nil {
			ml.ID = existing.ID
			ml.CreatedAt = existing.CreatedAt
			if err := db.Unscoped().Save(&ml).Error; err != nil {
				return fmt.Errorf("更新等级 %s 失败: %w", l.slug, err)
			}
		} else {
			if err := db.Create(&ml).Error; err != nil {
				return fmt.Errorf("创建等级 %s 失败: %w", l.slug, err)
			}
		}
		levelIDs = append(levelIDs, ml.ID)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(rawPasswd), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bcrypt 失败: %w", err)
	}
	now := time.Now().UTC()
	dist := make(map[uint]int)
	users := make([]userdomain.User, 0, userCount)
	for i := 1; i <= userCount; i++ {
		lid := levelIDs[mathrand.Intn(len(levelIDs))]
		dist[lid]++
		// 注册时间分散在 90 天内，保证增长曲线有数据
		regDaysAgo := mathrand.Intn(90)
		regTime := now.AddDate(0, 0, -regDaysAgo).Add(time.Duration(mathrand.Intn(86400)) * time.Second)
		users = append(users, userdomain.User{
			Email:           fmt.Sprintf(seedEmail, i),
			PasswordHash:    string(hash),
			DisplayName:     fmt.Sprintf("测试会员%03d", i),
			Locale:          "zh-CN",
			Status:          "active",
			MemberLevelID:   lid,
			EmailVerifiedAt: &regTime,
			CreatedAt:       regTime,
		})
	}
	if err := db.CreateInBatches(users, 50).Error; err != nil {
		return fmt.Errorf("批量创建会员失败: %w", err)
	}
	log.Printf("[%s] 等级=%d 会员=%d 分布=%v", label, len(levelIDs), len(users), dist)
	return nil
}

// ---------- 分类与商品 ----------

func seedCategories(db *gorm.DB, label string) ([]uint, error) {
	catDefs := []struct {
		slug   string
		nameZH string
		nameEN string
		icon   string
		sort   int
	}{
		{"game-topup", "游戏充值", "Game Top-up", "🎮", 1},
		{"software-key", "软件激活码", "Software Key", "💻", 2},
		{"digital-card", "数字卡券", "Digital Card", "🎫", 3},
		{"vip-service", "会员服务", "VIP Service", "👑", 4},
		{"virtual-goods", "虚拟商品", "Virtual Goods", "📦", 5},
	}

	categoryIDs := make([]uint, 0, len(catDefs))
	for _, cd := range catDefs {
		var existing categorydomain.Category
		err := db.Unscoped().Where("slug = ?", cd.slug).First(&existing).Error
		cat := categorydomain.Category{
			Slug:      cd.slug,
			NameJSON:  jsonmap.JSON{"zh-CN": cd.nameZH, "en-US": cd.nameEN},
			Icon:      cd.icon,
			SortOrder: cd.sort,
			IsActive:  true,
		}
		if err == nil {
			cat.ID = existing.ID
			cat.CreatedAt = existing.CreatedAt
			if err := db.Unscoped().Save(&cat).Error; err != nil {
				return nil, fmt.Errorf("更新分类 %s 失败: %w", cd.slug, err)
			}
		} else {
			if err := db.Create(&cat).Error; err != nil {
				return nil, fmt.Errorf("创建分类 %s 失败: %w", cd.slug, err)
			}
		}
		categoryIDs = append(categoryIDs, cat.ID)
	}
	log.Printf("[%s] 分类=%d", label, len(categoryIDs))
	return categoryIDs, nil
}

func seedProducts(db *gorm.DB, categoryIDs []uint, label string) error {
	type productDef struct {
		slug            string
		nameZH          string
		nameEN          string
		price           int64
		purchaseType    string
		fulfillmentType string
		hasSKU          bool
	}
	productDefs := []productDef{
		// 游戏充值
		{"lol-100-points", "英雄联盟 100点券", "LoL 100 RP", 1000, constants.ProductPurchaseMember, constants.FulfillmentTypeAuto, false},
		{"pubg-uc-60", "PUBG 60 UC", "PUBG 60 UC", 600, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		{"genshin-6480", "原神 6480创世结晶", "Genshin 6480 Crystals", 64800, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		// 软件激活码
		{"win11-pro-key", "Windows 11 Pro 激活码", "Windows 11 Pro Key", 15800, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		{"office2024-key", "Office 2024 专业增强版", "Office 2024 Pro Plus", 29900, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		{"adobe-ps-annual", "Adobe Photoshop 年费", "Adobe Photoshop Annual", 88800, constants.ProductPurchaseMember, constants.FulfillmentTypeManual, false},
		// 数字卡券
		{"steam-gift-50", "Steam 礼品卡 $50", "Steam Gift Card $50", 35000, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		{"apple-gift-100", "Apple 充值卡 ¥100", "Apple Gift Card ¥100", 10000, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		{"netflix-month", "Netflix 月卡", "Netflix Monthly", 2500, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, false},
		// 会员服务
		{"vip-monthly", "VIP 月度会员", "VIP Monthly", 2990, constants.ProductPurchaseMember, constants.FulfillmentTypeManual, false},
		{"vip-yearly", "VIP 年度会员", "VIP Yearly", 29900, constants.ProductPurchaseMember, constants.FulfillmentTypeManual, false},
		{"svip-yearly", "SVIP 年度会员", "SVIP Yearly", 59900, constants.ProductPurchaseMember, constants.FulfillmentTypeManual, false},
		// 虚拟商品 (with SKUs)
		{"domain-com-cn", "域名注册 .com/.cn", "Domain Registration", 5500, constants.ProductPurchaseGuest, constants.FulfillmentTypeManual, true},
		{"cdn-traffic-100g", "CDN 流量包 100G", "CDN Traffic 100GB", 9900, constants.ProductPurchaseMember, constants.FulfillmentTypeAuto, true},
		{"sms-package-1000", "短信包 1000条", "SMS Package 1000", 5000, constants.ProductPurchaseGuest, constants.FulfillmentTypeAuto, true},
	}

	// 清理旧测试商品
	if err := db.Unscoped().Where("slug LIKE ?", "lol-%").Or("slug IN (?)", []string{
		"pubg-uc-60", "genshin-6480", "win11-pro-key", "office2024-key", "adobe-ps-annual",
		"steam-gift-50", "apple-gift-100", "netflix-month",
		"vip-monthly", "vip-yearly", "svip-yearly",
		"domain-com-cn", "cdn-traffic-100g", "sms-package-1000",
	}).Delete(&productdomain.Product{}).Error; err != nil {
		return fmt.Errorf("清理旧测试商品失败: %w", err)
	}

	// 清理旧测试 SKU
	if err := db.Unscoped().Where("product_id IN (SELECT id FROM products WHERE slug LIKE ?)", "lol-%").Or("product_id IN (SELECT id FROM products WHERE slug IN (?))", []string{
		"pubg-uc-60", "genshin-6480", "win11-pro-key", "office2024-key", "adobe-ps-annual",
		"steam-gift-50", "apple-gift-100", "netflix-month",
		"vip-monthly", "vip-yearly", "svip-yearly",
		"domain-com-cn", "cdn-traffic-100g", "sms-package-1000",
	}).Delete(&productdomain.ProductSKU{}).Error; err != nil {
		return fmt.Errorf("清理旧测试SKU失败: %w", err)
	}

	productCount := 0
	skuCount := 0
	for i, pd := range productDefs {
		catID := categoryIDs[i%len(categoryIDs)]
		prod := productdomain.Product{
			CategoryID:      catID,
			Slug:            pd.slug,
			TitleJSON:       jsonmap.JSON{"zh-CN": pd.nameZH, "en-US": pd.nameEN},
			DescriptionJSON: jsonmap.JSON{"zh-CN": pd.nameZH + " - 测试商品描述", "en-US": pd.nameEN + " - Test product description"},
			PriceAmount:     money.FromDecimal(decimal.NewFromInt(pd.price)),
			PurchaseType:    pd.purchaseType,
			FulfillmentType: pd.fulfillmentType,
			IsActive:        true,
			SortOrder:       i * 10,
		}
		if err := db.Create(&prod).Error; err != nil {
			return fmt.Errorf("创建商品 %s 失败: %w", pd.slug, err)
		}
		productCount++

		if pd.hasSKU {
			// 创建多规格 SKU
			skuDefs := []struct {
				nameZH string
				nameEN string
				price  int64
			}{
				{"标准版", "Standard", pd.price},
				{"高级版", "Premium", pd.price * 3 / 2},
				{"旗舰版", "Ultimate", pd.price * 2},
			}
			for j, sd := range skuDefs {
				sku := productdomain.ProductSKU{
					ProductID:        prod.ID,
					SKUCode:          fmt.Sprintf("%s-%d", pd.slug, j),
					SpecValuesJSON:   jsonmap.JSON{"zh-CN": sd.nameZH, "en-US": sd.nameEN},
					PriceAmount:      money.FromDecimal(decimal.NewFromInt(sd.price)),
					CostPriceAmount:  money.FromDecimal(decimal.NewFromInt(sd.price * 7 / 10)),
					ManualStockTotal: 100,
					IsActive:         true,
					SortOrder:        j,
				}
				if err := db.Create(&sku).Error; err != nil {
					return fmt.Errorf("创建SKU失败: %w", err)
				}
				skuCount++
			}
		} else {
			// 单规格，创建默认 SKU
			sku := productdomain.ProductSKU{
				ProductID:        prod.ID,
				SKUCode:          "DEFAULT",
				SpecValuesJSON:   jsonmap.JSON{"zh-CN": pd.nameZH, "en-US": pd.nameEN},
				PriceAmount:      money.FromDecimal(decimal.NewFromInt(pd.price)),
				CostPriceAmount:  money.FromDecimal(decimal.NewFromInt(pd.price * 7 / 10)),
				ManualStockTotal: -1, // 无限库存
				IsActive:         true,
				SortOrder:        0,
			}
			if err := db.Create(&sku).Error; err != nil {
				return fmt.Errorf("创建SKU失败: %w", err)
			}
			skuCount++
		}
	}
	log.Printf("[%s] 商品=%d, SKU=%d", label, productCount, skuCount)
	return nil
}

// ---------- 商品随机化 + 卡密 ----------

type productRow struct {
	ID              uint
	IsMapped        bool
	PurchaseType    string
	FulfillmentType string
}

type skuRow struct {
	ID        uint
	ProductID uint
}

func randomizeProducts(db *gorm.DB, label string) error {
	// 1. 读取所有未删除商品
	var products []productRow
	if err := db.Model(&productdomain.Product{}).Where("deleted_at IS NULL").Select("id, is_mapped, purchase_type, fulfillment_type").Find(&products).Error; err != nil {
		return fmt.Errorf("读取商品失败: %w", err)
	}
	if len(products) == 0 {
		log.Printf("[%s] 无商品，跳过随机化", label)
		return nil
	}

	// 2. 读取所有活跃 SKU
	var allSKUs []skuRow
	if err := db.Model(&productdomain.ProductSKU{}).Where("deleted_at IS NULL AND is_active = ?", true).Select("id, product_id").Find(&allSKUs).Error; err != nil {
		return fmt.Errorf("读取 SKU 失败: %w", err)
	}
	skuByProduct := make(map[uint][]skuRow)
	for _, s := range allSKUs {
		skuByProduct[s.ProductID] = append(skuByProduct[s.ProductID], s)
	}

	// 幂等：清理此前生成的测试卡密（TEST- 前缀）
	if err := db.Unscoped().Where("secret LIKE ?", "TEST-%").Delete(&cardsecretdomain.Secret{}).Error; err != nil {
		return fmt.Errorf("清理旧测试卡密失败: %w", err)
	}

	// 3. 随机分配 purchase_type 和 fulfillment_type，收集 auto 商品的 SKU
	type autoTarget struct{ productID, skuID uint }
	autoSKUs := make([]autoTarget, 0)
	autoProductCount := 0
	ptDist := map[string]int{}
	ftDist := map[string]int{}
	const (
		purchaseTypes    = 2 // guest, member
		fulfillmentTypes = 2 // manual, auto
	)
	for _, p := range products {
		newPt := []string{constants.ProductPurchaseGuest, constants.ProductPurchaseMember}[mathrand.Intn(purchaseTypes)]
		newFt := []string{constants.FulfillmentTypeManual, constants.FulfillmentTypeAuto}[mathrand.Intn(fulfillmentTypes)]
		if err := db.Model(&productdomain.Product{}).Where("id = ?", p.ID).Updates(map[string]interface{}{
			"purchase_type":    newPt,
			"fulfillment_type": newFt,
		}).Error; err != nil {
			return fmt.Errorf("更新商品 %d 失败: %w", p.ID, err)
		}
		ptDist[newPt]++
		ftDist[newFt]++
		if newFt == constants.FulfillmentTypeAuto {
			autoProductCount++
			skus := skuByProduct[p.ID]
			if len(skus) == 0 {
				// 无活跃 SKU（单规格），按 sku_id=0 生成卡密
				autoSKUs = append(autoSKUs, autoTarget{productID: p.ID, skuID: 0})
			} else {
				for _, s := range skus {
					autoSKUs = append(autoSKUs, autoTarget{productID: p.ID, skuID: s.ID})
				}
			}
		}
	}
	log.Printf("[%s] 随机化完成，商品=%d，购买资格=%v，交付方式=%v，auto商品=%d", label, len(products), ptDist, ftDist, autoProductCount)

	// 4. 为 auto 商品的每个 SKU 生成卡密库存
	if len(autoSKUs) > 0 {
		totalSecrets := len(autoSKUs) * cardSecretsPerSKU
		secrets := make([]cardsecretdomain.Secret, 0, totalSecrets)
		for _, target := range autoSKUs {
			for i := 0; i < cardSecretsPerSKU; i++ {
				secretText := genSecretString(target.productID, target.skuID, i)
				secrets = append(secrets, cardsecretdomain.Secret{
					ProductID: target.productID,
					SKUID:     target.skuID,
					Secret:    secretText,
					Status:    cardsecretdomain.StatusAvailable,
				})
			}
		}
		if err := db.CreateInBatches(secrets, 200).Error; err != nil {
			return fmt.Errorf("批量插入卡密失败: %w", err)
		}
		log.Printf("[%s] 卡密生成完成：%d 个 SKU × %d = %d 条", label, len(autoSKUs), cardSecretsPerSKU, len(secrets))
	}
	return nil
}

func genSecretString(productID, skuID uint, idx int) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)
	return fmt.Sprintf("TEST-%d-%d-%s-%04d", productID, skuID, suffix, idx)
}

// parseProductTitle 从数据库 title_json 字符串解析为 jsonmap.JSON
func parseProductTitle(raw string) jsonmap.JSON {
	if raw == "" {
		return jsonmap.JSON{"zh-CN": "未知商品", "en-US": "Unknown Product"}
	}
	var j jsonmap.JSON
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		s := raw
		if len(s) > 20 {
			s = s[:20]
		}
		return jsonmap.JSON{"zh-CN": "商品(" + s + ")", "en-US": "Product(" + s + ")"}
	}
	return j
}

// generateFakeIP 生成随机的假 IP
func fakeIP() string {
	return fmt.Sprintf("192.168.%d.%d", mathrand.Intn(256), mathrand.Intn(256))
}

// generateFakeUA 生成随机的 User-Agent
func fakeUA() string {
	uas := []string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/120.0.0.0",
	}
	return uas[mathrand.Intn(len(uas))]
}

// ---------- 订单与支付种子数据 ----------

func seedOrders(db *gorm.DB, label string) error {
	// 1. 读取用户
	var users []userdomain.User
	if err := db.Where("deleted_at IS NULL AND status = ?", "active").Find(&users).Error; err != nil {
		return fmt.Errorf("读取用户失败: %w", err)
	}
	if len(users) == 0 {
		return fmt.Errorf("无可用用户，请先运行 seeddata members")
	}

	// 2. 读取商品和 SKU
	type productInfo struct {
		ID              uint
		TitleJSON       string
		PurchaseType    string
		FulfillmentType string
	}
	var products []productInfo
	if err := db.Model(&productdomain.Product{}).Where("deleted_at IS NULL").Select("id, title_json, purchase_type, fulfillment_type").Find(&products).Error; err != nil {
		return fmt.Errorf("读取商品失败: %w", err)
	}
	if len(products) == 0 {
		return fmt.Errorf("无可用商品，请先在后台创建商品")
	}

	type skuInfo struct {
		ID        uint
		ProductID uint
		BasePrice money.Amount `gorm:"column:price_amount"`
	}
	var skus []skuInfo
	if err := db.Model(&productdomain.ProductSKU{}).Where("deleted_at IS NULL AND is_active = ?", true).Select("id, product_id, price_amount").Find(&skus).Error; err != nil {
		return fmt.Errorf("读取 SKU 失败: %w", err)
	}
	skusByProduct := make(map[uint][]skuInfo)
	for _, s := range skus {
		skusByProduct[s.ProductID] = append(skusByProduct[s.ProductID], s)
	}

	// 3. 读取支付通道
	var channels []paymentdomain.PaymentChannel
	if err := db.Where("deleted_at IS NULL AND is_active = ?", true).Find(&channels).Error; err != nil {
		return fmt.Errorf("读取支付通道失败: %w", err)
	}
	if len(channels) == 0 {
		// 创建一个默认通道
		ch := paymentdomain.PaymentChannel{
			Name:            "测试通道(支付宝)",
			ProviderType:    constants.PaymentProviderOfficial,
			ChannelType:     constants.PaymentChannelTypeAlipay,
			InteractionMode: constants.PaymentInteractionQR,
			IsActive:        true,
		}
		if err := db.Create(&ch).Error; err != nil {
			return fmt.Errorf("创建默认支付通道失败: %w", err)
		}
		channels = append(channels, ch)
	}

	// 4. 清理旧测试订单
	if err := db.Unscoped().Where("order_no LIKE ?", "SEED-%").Delete(&orderdomain.Order{}).Error; err != nil {
		return fmt.Errorf("清理旧订单失败: %w", err)
	}
	if err := db.Unscoped().Where("gateway_order_no LIKE ?", "SEED-%").Delete(&paymentdomain.Payment{}).Error; err != nil {
		return fmt.Errorf("清理旧支付记录失败: %w", err)
	}

	// 5. 生成订单
	const totalOrders = 500
	now := time.Now().UTC()

	dist := map[string]int{}
	dayDist := map[string]int{}
	itemCount := 0
	payCount := 0
	var orderCounter int64

	for i := 0; i < totalOrders; i++ {
		user := users[mathrand.Intn(len(users))]
		product := products[mathrand.Intn(len(products))]

		// 时间分布：60% 最近 7 天，25% 8-30 天前，10% 31-60，5% 61-90
		var daysAgo int
		roll := mathrand.Intn(100)
		switch {
		case roll < 60:
			daysAgo = mathrand.Intn(7)
		case roll < 85:
			daysAgo = 8 + mathrand.Intn(23)
		case roll < 95:
			daysAgo = 31 + mathrand.Intn(30)
		default:
			daysAgo = 61 + mathrand.Intn(30)
		}
		orderDate := now.AddDate(0, 0, -daysAgo).Add(
			time.Duration(mathrand.Intn(86400)) * time.Second,
		)
		dayKey := orderDate.Format("2006-01-02")
		dayDist[dayKey]++

		// 状态分布
		var status string
		roll = mathrand.Intn(100)
		switch {
		case roll < 50:
			status = constants.OrderStatusCompleted
		case roll < 70:
			status = constants.OrderStatusPaid
		case roll < 80:
			status = constants.OrderStatusRefunded
		case roll < 88:
			status = constants.OrderStatusDelivered
		case roll < 93:
			status = constants.OrderStatusFulfilling
		default:
			status = constants.OrderStatusPartiallyDelivered
		}
		dist[status]++

		no := atomic.AddInt64(&orderCounter, 1)
		orderNo := fmt.Sprintf("SEED-%s-%04d", orderDate.Format("20060102"), no)
		paidAt := orderDate.Add(time.Duration(mathrand.Intn(3600)) * time.Second)

		var memberLevelID *uint
		if user.MemberLevelID > 0 {
			lvl := user.MemberLevelID
			memberLevelID = &lvl
		}

		// 计算价格
		qty := mathrand.Intn(3) + 1
		var unitPrice money.Amount = money.FromDecimal(decimal.NewFromInt(int64(mathrand.Intn(49000) + 1000)))
		var priceSKUID uint = 0
		if pSkus, ok := skusByProduct[product.ID]; ok && len(pSkus) > 0 {
			s := pSkus[mathrand.Intn(len(pSkus))]
			priceSKUID = s.ID
			if s.BasePrice.IntPart() > 0 {
				unitPrice = s.BasePrice
			}
		}
		totalPrice := money.FromDecimal(unitPrice.Mul(decimal.NewFromInt(int64(qty))))
		discountRatio := float64(mathrand.Intn(15)) / 100.0
		discAmt := money.FromDecimal(totalPrice.Mul(decimal.NewFromFloat(discountRatio)).Round(2))
		paidAmt := money.FromDecimal(totalPrice.Sub(discAmt.Decimal))

		order := orderdomain.Order{
			OrderNo:          orderNo,
			UserID:           user.ID,
			Status:           status,
			Currency:         "CNY",
			TotalAmount:      paidAmt,
			OriginalAmount:   totalPrice,
			DiscountAmount:   discAmt,
			MemberLevelID:    memberLevelID,
			OnlinePaidAmount: paidAmt,
			ClientIP:         "127.0.0.1",
			CreatedAt:        orderDate,
			PaidAt:           &paidAt,
		}
		if err := db.Create(&order).Error; err != nil {
			return fmt.Errorf("创建订单 %s 失败: %w", orderNo, err)
		}

		// 订单项
		ni := mathrand.Intn(2) + 1
		for j := 0; j < ni; j++ {
			itemProd := product
			itemSKUID := priceSKUID
			itemPrice := unitPrice
			itemQty := qty
			if j > 0 {
				itemProd = products[mathrand.Intn(len(products))]
				itemQty = mathrand.Intn(3) + 1
				itemPrice = money.FromDecimal(decimal.NewFromInt(int64(mathrand.Intn(30000) + 500)))
				if pSkus, ok := skusByProduct[itemProd.ID]; ok && len(pSkus) > 0 {
					s := pSkus[mathrand.Intn(len(pSkus))]
					itemSKUID = s.ID
					if s.BasePrice.IntPart() > 0 {
						itemPrice = s.BasePrice
					}
				}
			}
			itemTotal := money.FromDecimal(itemPrice.Mul(decimal.NewFromInt(int64(itemQty))))
			oi := orderdomain.OrderItem{
				OrderID:         order.ID,
				ProductID:       itemProd.ID,
				SKUID:           itemSKUID,
				TitleJSON:       parseProductTitle(itemProd.TitleJSON),
				UnitPrice:       itemPrice,
				Quantity:        itemQty,
				TotalPrice:      itemTotal,
				FulfillmentType: itemProd.FulfillmentType,
				CreatedAt:       orderDate,
			}
			if err := db.Create(&oi).Error; err != nil {
				return fmt.Errorf("创建订单项失败: %w", err)
			}
			itemCount++
		}

		// 支付记录
		ch := channels[mathrand.Intn(len(channels))]
		payStatus := constants.PaymentStatusSuccess
		if mathrand.Intn(100) < 8 {
			payStatus = constants.PaymentStatusFailed
		}
		p := paymentdomain.Payment{
			OrderID:         order.ID,
			ChannelID:       ch.ID,
			ProviderType:    ch.ProviderType,
			ChannelType:     ch.ChannelType,
			InteractionMode: ch.InteractionMode,
			Amount:          paidAmt,
			Currency:        "CNY",
			Status:          payStatus,
			GatewayOrderNo:  orderNo,
			CreatedAt:       orderDate,
		}
		if payStatus == constants.PaymentStatusSuccess {
			p.PaidAt = &paidAt
		}
		if err := db.Create(&p).Error; err != nil {
			return fmt.Errorf("创建支付记录失败: %w", err)
		}
		payCount++
	}

	log.Printf("[%s] 写入完成: 订单=%d, 订单项=%d, 支付=%d", label, totalOrders, itemCount, payCount)
	log.Printf("[%s] 状态分布: %v", label, dist)
	log.Printf("[%s] 覆盖 %d 天", label, len(dayDist))
	return nil
}

func seedOrdersMain(driver, dsn, label string) error {
	db, err := open(driver, dsn)
	if err != nil {
		return fmt.Errorf("连接 %s 失败: %w", label, err)
	}
	return seedOrders(db, label)
}

// ---------- 交付记录 ----------

// seedFulfillments 为 auto 交付订单生成交付记录
func seedFulfillments(db *gorm.DB, label string) error {
	type orderInfo struct {
		ID              uint
		Status          string
		OrderNo         string
		UserID          uint
		FulfillmentType string
	}
	var autoOrders []orderInfo
	if err := db.Model(&orderdomain.Order{}).
		Select("orders.id, orders.status, orders.order_no, orders.user_id, oi.fulfillment_type").
		Joins("JOIN order_items oi ON oi.order_id = orders.id").
		Where("oi.fulfillment_type = ? AND orders.deleted_at IS NULL", constants.FulfillmentTypeAuto).
		Group("orders.id").
		Find(&autoOrders).Error; err != nil {
		return fmt.Errorf("查询 auto 订单失败: %w", err)
	}

	// 清理旧测试交付
	if err := db.Unscoped().Where("order_id IN (SELECT id FROM orders WHERE order_no LIKE ?)", "SEED-%").Delete(&fulfillmentdomain.Fulfillment{}).Error; err != nil {
		return fmt.Errorf("清理旧交付记录失败: %w", err)
	}

	deliveredCount := 0
	pendingCount := 0
	for _, o := range autoOrders {
		fulfillment := fulfillmentdomain.Fulfillment{
			OrderID: o.ID,
			Type:    constants.FulfillmentTypeAuto,
			Status:  constants.FulfillmentStatusPending,
			Payload: fmt.Sprintf("# %s 交付内容\n\n卡密: TEST-CARD-%d\n使用说明: 请登录对应平台激活", o.OrderNo, o.ID),
		}

		// 根据订单状态决定交付状态
		deliveredStatuses := map[string]bool{
			constants.OrderStatusFulfilling:         false,
			constants.OrderStatusPartiallyDelivered: true,
			constants.OrderStatusDelivered:          true,
			constants.OrderStatusCompleted:          true,
			constants.OrderStatusPartiallyRefunded:  true,
		}
		if delivered, ok := deliveredStatuses[o.Status]; ok && delivered {
			now := time.Now().UTC()
			fulfillment.Status = constants.FulfillmentStatusDelivered
			fulfillment.DeliveredAt = &now
			deliveredCount++
		} else {
			pendingCount++
		}

		if err := db.Create(&fulfillment).Error; err != nil {
			return fmt.Errorf("创建交付记录失败(order=%d): %w", o.ID, err)
		}
	}
	log.Printf("[%s] 交付记录: %d (delivered=%d pending=%d)", label, len(autoOrders), deliveredCount, pendingCount)
	return nil
}

// ---------- 退款记录 ----------

// seedRefundRecords 为退款订单生成退款记录
func seedRefundRecords(db *gorm.DB, label string) error {
	type refundOrder struct {
		ID          uint
		UserID      uint
		TotalAmount money.Amount
		Currency    string
		PaidAt      *time.Time
	}
	var orders []refundOrder
	if err := db.Model(&orderdomain.Order{}).
		Select("id, user_id, total_amount, currency, paid_at").
		Where("status = ? AND order_no LIKE ?", constants.OrderStatusRefunded, "SEED-%").
		Find(&orders).Error; err != nil {
		return fmt.Errorf("查询退款订单失败: %w", err)
	}

	// 清理旧测试退款记录
	if err := db.Unscoped().Where("order_id IN (SELECT id FROM orders WHERE order_no LIKE ?)", "SEED-%").Delete(&orderdomain.OrderRefundRecord{}).Error; err != nil {
		return fmt.Errorf("清理旧退款记录失败: %w", err)
	}

	refundTypes := []string{constants.OrderRefundTypeManual, constants.OrderRefundTypeWallet}
	for _, o := range orders {
		refundAmt := o.TotalAmount
		// 20% 部分退款
		if mathrand.Intn(100) < 20 {
			refundAmt = money.FromDecimal(o.TotalAmount.Mul(decimal.NewFromFloat(0.3 + float64(mathrand.Intn(50))/100.0)).Round(2))
		}

		rec := orderdomain.OrderRefundRecord{
			OrderID:   o.ID,
			UserID:    o.UserID,
			Type:      refundTypes[mathrand.Intn(len(refundTypes))],
			Amount:    refundAmt,
			Currency:  o.Currency,
			Remark:    fmt.Sprintf("测试退款 - 订单"),
			CreatedAt: time.Now().UTC(),
		}
		if o.PaidAt != nil {
			rec.CreatedAt = o.PaidAt.Add(time.Duration(mathrand.Intn(86400*7)) * time.Second)
		}
		if err := db.Create(&rec).Error; err != nil {
			return fmt.Errorf("创建退款记录失败(order=%d): %w", o.ID, err)
		}
	}
	log.Printf("[%s] 退款记录: %d", label, len(orders))
	return nil
}

// ---------- 登录日志 ----------

// seedUserLoginLogs 生成用户登录日志（丰富 DAU 数据）
func seedUserLoginLogs(db *gorm.DB, label string) error {
	var users []userdomain.User
	if err := db.Where("deleted_at IS NULL AND status = ?", "active").Find(&users).Error; err != nil {
		return fmt.Errorf("查询用户失败: %w", err)
	}
	if len(users) == 0 {
		return fmt.Errorf("无可用用户")
	}

	// 清理旧测试登录日志
	if err := db.Unscoped().Where("client_ip LIKE ?", "192.168.%").Delete(&auditlogdomain.UserLoginLog{}).Error; err != nil {
		return fmt.Errorf("清理旧登录日志失败: %w", err)
	}

	now := time.Now().UTC()
	totalLogs := 0
	// 每个用户在过去 90 天内有 15-60 次登录
	for _, u := range users {
		loginCount := 15 + mathrand.Intn(45)
		for i := 0; i < loginCount; i++ {
			daysAgo := mathrand.Intn(90)
			hourOffset := mathrand.Intn(86400)
			loginTime := now.AddDate(0, 0, -daysAgo).Add(time.Duration(hourOffset) * time.Second)

			log := auditlogdomain.UserLoginLog{
				UserID:      u.ID,
				Email:       u.Email,
				Status:      "success",
				ClientIP:    fakeIP(),
				UserAgent:   fakeUA(),
				LoginSource: "web",
				RequestID:   fmt.Sprintf("seed-login-%d-%d", u.ID, i),
				CreatedAt:   loginTime,
			}
			if err := db.Create(&log).Error; err != nil {
				return fmt.Errorf("创建登录日志失败: %w", err)
			}
			totalLogs++
		}
	}
	log.Printf("[%s] 登录日志: %d 条 (覆盖 %d 用户)", label, totalLogs, len(users))
	return nil
}

// seedProductTags 为所有 active 商品随机分配产品标签（products.tags 列，非 seo_meta）。
func seedProductTags(db *gorm.DB, label string) error {
	// 标签池（产品分类标签，区别于 SEO 标签 seo_meta）
	tagPool := []string{"热门", "推荐", "限时", "新上", "特价", "爆款", "新品", "精选", "折扣", "秒杀", "包邮", "正品", "独家", "首发", "预售"}

	var products []productdomain.Product
	if err := db.Where("is_active = ?", true).Find(&products).Error; err != nil {
		return fmt.Errorf("%s: 查询商品失败: %w", label, err)
	}
	if len(products) == 0 {
		log.Printf("[%s] 无 active 商品，跳过标签填充", label)
		return nil
	}

	rng := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
	updateCount := 0
	for i := range products {
		// 每个商品随机 2~4 个不重复标签
		n := 2 + rng.Intn(3)
		rng.Shuffle(len(tagPool), func(i, j int) { tagPool[i], tagPool[j] = tagPool[j], tagPool[i] })
		tags := jsonslice.Strings{}
		seen := map[string]bool{}
		for _, t := range tagPool {
			if len(tags) >= n {
				break
			}
			if seen[t] {
				continue
			}
			seen[t] = true
			tags = append(tags, t)
		}

		if err := db.Model(&products[i]).Update("tags", tags).Error; err != nil {
			return fmt.Errorf("%s: 更新商品 %d 标签失败: %w", label, products[i].ID, err)
		}
		updateCount++
	}
	log.Printf("[%s] 已为 %d 个商品填充产品标签", label, updateCount)
	return nil
}

// ---------- 站点设置 ----------

type settingRow struct {
	Key       string       `gorm:"primarykey"`
	ValueJSON jsonmap.JSON `gorm:"type:json"`
}

func (settingRow) TableName() string { return "settings" }

// seedSiteConfig 写入站点基本配置（品牌、SEO、联系方式等）。
func seedSiteConfig(db *gorm.DB, label string) error {
	siteConfig := jsonmap.JSON{
		"brand": jsonmap.JSON{
			"site_name": "角标数卡",
			"site_url":  "https://dujiao-next.com",
			"site_icon": "",
			"site_description": jsonmap.JSON{
				"zh-CN": "角标数卡 - 专业的数字商品交易平台，提供游戏充值、软件激活码、数字卡券等一站式服务。",
				"en-US": "Dujiao - Professional digital goods trading platform.",
			},
		},
		"contact": jsonmap.JSON{
			"telegram": "dujiao_support",
			"whatsapp": "",
		},
		"seo": jsonmap.JSON{
			"title": jsonmap.JSON{
				"zh-CN": "角标数卡 - 数字商品交易平台",
				"en-US": "Dujiao - Digital Goods Platform",
			},
			"keywords": jsonmap.JSON{
				"zh-CN": "游戏充值,软件激活码,数字卡券,Steam礼品卡,会员服务",
				"en-US": "game topup,software key,digital card,gift card,membership",
			},
			"description": jsonmap.JSON{
				"zh-CN": "角标数卡提供游戏充值、软件激活码、数字卡券、会员服务等数字商品，安全快捷，24小时自动发货。",
				"en-US": "Dujiao provides game top-up, software keys, digital cards, and membership services.",
			},
		},
		"legal": jsonmap.JSON{
			"terms": jsonmap.JSON{
				"zh-CN": "## 服务条款\n\n欢迎使用角标数卡。使用本平台即表示您同意以下条款...",
				"en-US": "## Terms of Service\n\nWelcome to Dujiao. By using this platform you agree...",
			},
			"privacy": jsonmap.JSON{
				"zh-CN": "## 隐私政策\n\n我们重视您的隐私...",
				"en-US": "## Privacy Policy\n\nWe value your privacy...",
			},
		},
		"about": jsonmap.JSON{
			"hero": jsonmap.JSON{
				"title": jsonmap.JSON{
					"zh-CN": "角标数卡",
					"en-US": "Dujiao",
				},
				"subtitle": jsonmap.JSON{
					"zh-CN": "专业数字商品交易平台",
					"en-US": "Professional Digital Goods Platform",
				},
			},
			"introduction": jsonmap.JSON{
				"zh-CN": "角标数卡成立于2024年，致力于为全球用户提供安全、便捷的数字商品交易服务。我们与多家知名游戏厂商和软件发行商建立了合作关系，确保每一笔交易的真实可靠。",
				"en-US": "Founded in 2024, Dujiao is dedicated to providing secure and convenient digital goods trading services worldwide. We partner with major game publishers and software vendors to ensure every transaction is authentic and reliable.",
			},
			"services": jsonmap.JSON{
				"title": jsonmap.JSON{
					"zh-CN": "我们的服务",
					"en-US": "Our Services",
				},
				"items": []interface{}{
					jsonmap.JSON{
						"zh-CN": "游戏充值 - 支持 LOL、PUBG、原神等热门游戏",
						"en-US": "Game Top-up - LOL, PUBG, Genshin and more",
					},
					jsonmap.JSON{
						"zh-CN": "软件激活码 - Windows、Office、Adobe 正版授权",
						"en-US": "Software Keys - Windows, Office, Adobe licenses",
					},
					jsonmap.JSON{
						"zh-CN": "数字卡券 - Steam、Apple、Netflix 礼品卡",
						"en-US": "Digital Cards - Steam, Apple, Netflix gift cards",
					},
					jsonmap.JSON{
						"zh-CN": "会员服务 - VIP/SVIP 多级会员体系",
						"en-US": "Membership - VIP/SVIP tier system",
					},
				},
			},
			"contact": jsonmap.JSON{
				"title": jsonmap.JSON{
					"zh-CN": "联系我们",
					"en-US": "Contact Us",
				},
				"text": jsonmap.JSON{
					"zh-CN": "如有任何问题或建议，欢迎通过 Telegram @dujiao_support 联系我们，我们将在24小时内回复。",
					"en-US": "For any questions or suggestions, reach us via Telegram @dujiao_support. We respond within 24 hours.",
				},
			},
		},
		"scripts":             []interface{}{},
		"footer_links":        []interface{}{},
		"currency":            "CNY",
		"template_mode":       "default",
		"storefront_template": "",
	}
	// 用 GORM upsert，确保 JSON 序列化正确
	var existing settingRow
	err := db.Where("key = ?", constants.SettingKeySiteConfig).First(&existing).Error
	if err == nil {
		existing.ValueJSON = siteConfig
		if err := db.Save(&existing).Error; err != nil {
			return fmt.Errorf("更新站点配置失败: %w", err)
		}
	} else {
		if err := db.Create(&settingRow{Key: constants.SettingKeySiteConfig, ValueJSON: siteConfig}).Error; err != nil {
			return fmt.Errorf("创建站点配置失败: %w", err)
		}
	}
	log.Printf("[%s] 站点配置已写入", label)
	return nil
}

// ---------- 文章分类 ----------

func seedPostCategories(db *gorm.DB, label string) ([]postCategorySeed, error) {
	catDefs := []struct {
		slug   string
		nameZH string
		nameEN string
		icon   string
		sort   int
	}{
		{"announcements", "网站公告", "Announcements", "📢", 1},
		{"help", "帮助中心", "Help Center", "❓", 2},
		{"news", "行业资讯", "Industry News", "📰", 3},
		{"tutorials", "使用教程", "Tutorials", "📖", 4},
		{"updates", "更新日志", "Changelog", "🔄", 5},
		{"faq", "常见问题", "FAQ", "💬", 6},
	}

	catSeeds := make([]postCategorySeed, 0, len(catDefs))
	for _, cd := range catDefs {
		var existing contentdomain.PostCategory
		err := db.Unscoped().Where("slug = ?", cd.slug).First(&existing).Error
		cat := contentdomain.PostCategory{
			Slug:      cd.slug,
			NameJSON:  jsonmap.JSON{"zh-CN": cd.nameZH, "en-US": cd.nameEN},
			Icon:      cd.icon,
			IsActive:  true,
			SortOrder: cd.sort,
		}
		if err == nil {
			cat.ID = existing.ID
			cat.CreatedAt = existing.CreatedAt
			if err := db.Unscoped().Save(&cat).Error; err != nil {
				return nil, fmt.Errorf("更新文章分类 %s 失败: %w", cd.slug, err)
			}
		} else {
			if err := db.Create(&cat).Error; err != nil {
				return nil, fmt.Errorf("创建文章分类 %s 失败: %w", cd.slug, err)
			}
		}
		catSeeds = append(catSeeds, postCategorySeed{ID: cat.ID, Slug: cat.Slug})
	}
	log.Printf("[%s] 文章分类=%d", label, len(catSeeds))
	return catSeeds, nil
}

// ---------- 文章 ----------

type postCategorySeed struct {
	ID   uint
	Slug string
}

type postDef struct {
	slug      string
	catIdx    int // 分类索引（-1 表示不设分类）
	titleZH   string
	titleEN   string
	summaryZH string
	summaryEN string
	contentZH string
	contentEN string
	published bool
}

func seedPosts(db *gorm.DB, catSeeds []postCategorySeed, label string) error {
	posts := []postDef{
		// 公告类
		{"welcome-2024", 0, "欢迎来到角标数卡！", "Welcome to Dujiao!",
			"角标数卡正式上线运营，感谢大家的支持。", "Dujiao is now officially launched. Thank you for your support.",
			"## 关于我们\n\n角标数卡是一个专业的数字商品交易平台，致力于为用户提供安全、便捷的数字商品购买体验。\n\n### 我们的服务\n\n- 🎮 **游戏充值** - 支持多款热门游戏点券充值\n- 💻 **软件激活码** - Windows、Office、Adobe 等正版激活\n- 🎫 **数字卡券** - Steam、Apple、Netflix 等礼品卡\n- 👑 **会员服务** - VIP/SVIP 多级会员体系\n\n### 联系我们\n\n如有任何问题，请联系客服 Telegram：@dujiao_support",
			"## About Us\n\nDujiao is a professional digital goods trading platform.\n\n### Our Services\n\n- 🎮 Game Top-up\n- 💻 Software Keys\n- 🎫 Digital Cards\n- 👑 VIP Membership",
			true},

		{"spring-festival-2025", 0, "2025春节活动公告", "2025 Spring Festival Announcement",
			"春节期间照常营业，部分商品限时特惠。", "We remain open during Spring Festival with special offers.",
			"## 春节活动\n\n春节期间（1月28日-2月4日）我们照常提供服务。\n\n### 限时特惠\n\n- Steam 礼品卡 9折\n- VIP 年度会员 8折\n- 新用户注册即送 50 元优惠券\n\n### 客服安排\n\n春节期间客服响应时间可能略有延迟，感谢理解。",
			"## Spring Festival\n\nWe remain open during Spring Festival (Jan 28 - Feb 4).\n\n### Special Offers\n\n- Steam Gift Card 10% off\n- VIP Yearly 20% off\n- New user 50 CNY coupon",
			true},

		// 帮助类
		{"how-to-buy", 1, "如何购买商品？", "How to Buy Products?",
			"详细的购买流程指南，帮助您快速上手。", "Detailed buying guide to help you get started quickly.",
			"## 购买流程\n\n### 第一步：选择商品\n\n浏览商品列表，点击感兴趣的商品进入详情页。\n\n### 第二步：确认规格\n\n选择商品规格（如有），确认购买数量。\n\n### 第三步：提交订单\n\n填写必要信息，选择支付方式，点击提交订单。\n\n### 第四步：完成支付\n\n根据选择的支付方式完成付款。支付成功后系统将自动处理交付。\n\n### 第五步：查收商品\n\n- **自动发货**：支付成功后卡密将自动显示在订单详情页\n- **人工发货**：客服将在工作时间尽快处理",
			"## How to Buy\n\n### Step 1: Choose Product\nBrowse products and click for details.\n\n### Step 2: Select Options\nChoose specs and quantity.\n\n### Step 3: Place Order\nFill in info and choose payment method.\n\n### Step 4: Pay\nComplete payment.\n\n### Step 5: Receive\nAuto-delivery or manual processing.",
			true},

		{"payment-methods", 1, "支持哪些支付方式？", "Supported Payment Methods",
			"了解角标数卡支持的各类支付方式。", "Learn about payment methods supported by Dujiao.",
			"## 支付方式\n\n### 当前支持的支付方式\n\n1. **支付宝** - 扫码支付，实时到账\n2. **微信支付** - 扫码支付，实时到账\n3. **USDT-TRC20** - 加密货币支付，需确认区块\n\n### 支付限额\n\n- 单笔最低：¥1.00\n- 单笔最高：¥50,000.00\n\n### 支付失败怎么办？\n\n如果支付失败，请检查：\n1. 账户余额是否充足\n2. 是否超过支付限额\n3. 网络连接是否正常\n\n仍然无法解决？联系客服获取帮助。",
			"## Payment Methods\n\n1. Alipay\n2. WeChat Pay\n3. USDT-TRC20\n\nContact support if you have payment issues.",
			true},

		{"refund-policy", 1, "退款政策说明", "Refund Policy",
			"了解我们的退款政策和申请流程。", "Learn about our refund policy and process.",
			"## 退款政策\n\n### 可退款情况\n\n1. 商品未交付或交付失败\n2. 商品与描述严重不符\n3. 重复支付\n\n### 不可退款情况\n\n1. 虚拟商品已交付且正常使用\n2. 因用户自身原因造成的购买错误\n\n### 退款流程\n\n1. 在订单详情页点击「申请退款」\n2. 填写退款原因并提交\n3. 客服将在 24 小时内审核\n4. 审核通过后，退款将在 3-7 个工作日内到账",
			"## Refund Policy\n\n### Eligible\n1. Product not delivered\n2. Product significantly different from description\n3. Duplicate payment\n\n### Process\nSubmit refund request → Review within 24h → Refund in 3-7 business days",
			true},

		// 资讯类
		{"digital-goods-market-2025", 2, "2025年数字商品市场趋势分析", "2025 Digital Goods Market Trends",
			"全球数字商品市场规模持续增长，2025年预计突破3000亿美元。", "Global digital goods market continues to grow, expected to exceed $300B in 2025.",
			"## 市场概况\n\n2025年，全球数字商品市场继续保持强劲增长态势。根据最新数据显示：\n\n### 关键数据\n\n- 市场规模：预计 $3,200 亿\n- 年增长率：18.5%\n- 移动端占比：67%\n\n### 热门品类\n\n1. **游戏虚拟商品** - 占比 35%\n2. **软件即服务(SaaS)** - 占比 28%\n3. **数字内容订阅** - 占比 22%\n4. **其他数字商品** - 占比 15%\n\n### 趋势展望\n\n- AI 驱动的个性化推荐将成为标配\n- 区块链技术在数字商品溯源中的应用\n- 跨境数字商品交易的便利化",
			"## Market Overview\n\n### Key Data\n- Market size: $320B\n- Annual growth: 18.5%\n- Mobile share: 67%\n\n### Hot Categories\n1. Gaming - 35%\n2. SaaS - 28%\n3. Digital Subscriptions - 22%\n4. Others - 15%",
			true},

		{"ai-impact-digital-trade", 2, "AI如何改变数字商品交易", "How AI is Changing Digital Goods Trading",
			"人工智能技术正在深刻改变数字商品交易的方式和体验。", "AI is transforming digital goods trading.",
			"## AI 在数字商品交易中的应用\n\n### 智能定价\n\n基于市场供需、用户行为等数据，AI 算法可以动态调整商品定价，实现收益最大化。\n\n### 风控检测\n\nAI 模型能够实时识别异常交易行为，有效防止欺诈和刷单。\n\n### 智能客服\n\n7×24 小时在线的 AI 客服可以处理大部分常见问题，提升服务效率。\n\n### 个性化推荐\n\n通过分析用户购买历史和行为偏好，AI 可以为每个用户推荐最合适的商品。",
			"## AI in Digital Trading\n\n- Smart pricing\n- Fraud detection\n- AI customer service\n- Personalized recommendations",
			true},

		// 教程类
		{"steam-gift-card-guide", 3, "Steam 礼品卡使用完整指南", "Complete Guide to Steam Gift Cards",
			"手把手教你购买和使用 Steam 礼品卡。", "Step-by-step guide to buying and using Steam gift cards.",
			"## Steam 礼品卡使用指南\n\n### 什么是 Steam 礼品卡？\n\nSteam 礼品卡是 Valve 公司发行的数字充值卡，可用于在 Steam 平台购买游戏、软件和社区市场物品。\n\n### 如何购买\n\n1. 在本站选择对应面额的 Steam 礼品卡\n2. 完成支付\n3. 在订单详情页获取卡密\n4. 登录 Steam 客户端进行充值\n\n### 充值步骤\n\n1. 打开 Steam 客户端\n2. 点击右上角用户名 → 账户明细\n3. 点击「+为您的 Steam 钱包充值」\n4. 选择「兑换 Steam 礼品卡或钱包充值码」\n5. 输入卡密完成充值",
			"## Steam Gift Card Guide\n\n### How to Buy\n1. Choose card\n2. Pay\n3. Get code\n4. Redeem on Steam",
			true},

		{"windows-activation-guide", 3, "Windows 激活码使用教程", "Windows Activation Guide",
			"详细说明如何使用购买的 Windows 激活码。", "How to activate Windows with your purchased key.",
			"## Windows 激活步骤\n\n### 准备工作\n\n确保你的 Windows 版本与购买的激活码版本一致（如 Windows 11 Pro）。\n\n### 激活方法\n\n#### 方法一：设置中激活\n\n1. 打开「设置」→「系统」→「激活」\n2. 点击「更改产品密钥」\n3. 输入购买的激活码\n4. 点击「下一步」完成激活\n\n#### 方法二：命令提示符\n\n```cmd\nslmgr /ipk XXXXX-XXXXX-XXXXX-XXXXX-XXXXX\nslmgr /ato\n```\n\n### 常见问题\n\n- **0xC004C008**：密钥已在其他设备上使用，请联系客服\n- **0x8007232B**：DNS 名称不存在，检查网络连接",
			"## Windows Activation\n\n### Steps\n1. Settings → System → Activation\n2. Click Change product key\n3. Enter your key\n4. Complete activation\n\n### Common errors\n- 0xC004C008: Key already used\n- 0x8007232B: Check network",
			true},

		// 更新日志
		{"changelog-v1-5", 4, "v1.5 更新日志", "v1.5 Changelog",
			"v1.5 版本新增会员等级系统、订单导出等功能。", "v1.5 adds membership levels, order export, and more.",
			"## v1.5 更新内容\n\n### 新增功能\n\n- 🏆 **会员等级系统**：青铜→钻石五级会员，享受不同折扣\n- 📊 **订单数据导出**：支持导出 CSV 格式订单报表\n- 🔍 **商品搜索优化**：支持多关键词模糊搜索\n- 📱 **移动端适配**：优化移动端购物体验\n\n### 优化改进\n\n- 支付流程优化，减少支付超时问题\n- 后台管理界面升级\n- 系统性能提升 30%\n\n### 问题修复\n\n- 修复偶发订单状态不同步问题\n- 修复部分商品图片加载失败\n- 修复 Safari 浏览器兼容性问题",
			"## v1.5 Changes\n\n### New Features\n- Membership level system\n- Order CSV export\n- Enhanced product search\n- Mobile optimization\n\n### Improvements\n- Payment flow optimization\n- Admin UI upgrade\n- 30% performance boost\n\n### Bug Fixes\n- Order sync issue\n- Image loading fix\n- Safari compatibility",
			true},

		{"changelog-v1-4", 4, "v1.4 更新日志", "v1.4 Changelog",
			"v1.4 版本新增分销系统、Telegram 通知等功能。", "v1.4 adds reseller system, Telegram notifications.",
			"## v1.4 更新内容\n\n### 新增功能\n\n- 🤝 **分销系统**：支持多级分销和佣金结算\n- 📢 **Telegram 通知**：新订单实时推送到 Telegram\n- 🛒 **购物车功能**：支持多商品同时下单\n- 💰 **钱包系统**：支持余额充值和消费\n\n### 优化改进\n\n- API 响应速度提升\n- 数据库查询优化\n\n### 问题修复\n\n- 修复批量发货偶发卡顿\n- 修复订单金额计算精度问题",
			"## v1.4 Changes\n\n### New\n- Reseller system\n- Telegram notifications\n- Shopping cart\n- Wallet system",
			true},

		// FAQ
		{"order-not-received", 5, "支付成功但未收到商品怎么办？", "Paid but Not Received?",
			"支付成功后未收到商品的常见原因和解决方法。", "Common reasons and solutions for not receiving products after payment.",
			"## 支付成功但未收到商品\n\n### 自动发货商品\n\n1. **等待 1-2 分钟**：自动发货系统可能需要短暂延迟\n2. **刷新订单页**：在订单详情页刷新查看\n3. **检查垃圾邮件**：部分邮箱可能将发货通知误判为垃圾邮件\n\n### 人工发货商品\n\n1. **查看订单状态**：确认订单是否处于「待处理」状态\n2. **注意工作时间**：人工发货仅在工作时间 9:00-21:00 处理\n3. **联系客服**：超过 2 小时未处理，请联系客服\n\n### 以上方法无效？\n\n联系 Telegram 客服 @dujiao_support，提供订单号以便快速处理。",
			"## Paid but Not Received?\n\n### Auto-delivery Products\n1. Wait 1-2 minutes\n2. Refresh order page\n3. Check spam folder\n\n### Manual Products\n1. Check order status\n2. Business hours: 9:00-21:00\n3. Contact support after 2 hours",
			true},

		{"account-security", 5, "账户安全建议", "Account Security Tips",
			"保护您的角标数卡账户安全的最佳实践。", "Best practices for keeping your Dujiao account secure.",
			"## 账户安全建议\n\n### 密码安全\n\n- 使用至少 8 位包含大小写字母、数字和特殊字符的密码\n- 不要使用与其他网站相同的密码\n- 定期更换密码（建议每 3 个月）\n\n### 开启两步验证\n\n我们强烈建议您开启 TOTP 两步验证，为账户增加额外保护层。\n\n### 防范钓鱼\n\n- 确保访问的是官方网址 https://dujiao-next.com\n- 不要点击可疑邮件中的链接\n- 客服不会索要您的密码或验证码\n\n### 设备安全\n\n- 不要在公共设备上勾选「记住我」\n- 使用完毕后及时退出登录",
			"## Account Security\n\n### Password\n- 8+ chars with mixed case, numbers, symbols\n- Don't reuse passwords\n- Change every 3 months\n\n### Enable 2FA\nEnable TOTP two-factor authentication.\n\n### Anti-phishing\n- Only visit https://dujiao-next.com\n- Don't click suspicious links",
			true},
	}

	// 清理旧测试文章
	if err := db.Unscoped().Where("slug LIKE ?", "welcome-%").Or("slug LIKE ?", "changelog-%").Or("slug IN (?)", []string{
		"spring-festival-2025", "how-to-buy", "payment-methods", "refund-policy",
		"digital-goods-market-2025", "ai-impact-digital-trade",
		"steam-gift-card-guide", "windows-activation-guide",
		"order-not-received", "account-security",
	}).Delete(&contentdomain.Post{}).Error; err != nil {
		return fmt.Errorf("清理旧测试文章失败: %w", err)
	}

	now := time.Now().UTC()
	postCount := 0
	for i, pd := range posts {
		var catID *uint
		catSlug := ""
		if pd.catIdx >= 0 && pd.catIdx < len(catSeeds) {
			cid := catSeeds[pd.catIdx].ID
			catID = &cid
			catSlug = catSeeds[pd.catIdx].Slug
		}

		// 发布时间分散在 60 天内
		daysAgo := mathrand.Intn(60)
		pubTime := now.AddDate(0, 0, -daysAgo).Add(time.Duration(mathrand.Intn(86400)) * time.Second)

		pt := "blog"
		if posts[i].catIdx == 0 {
			pt = "notice" // 公告分类用 notice 类型
		}
		if posts[i].catIdx == 4 {
			pt = "blog" // 更新日志也用 blog 类型（显式保持 blog）
		}

		post := contentdomain.Post{
			Slug:         pd.slug,
			Type:         pt,
			TitleJSON:    jsonmap.JSON{"zh-CN": pd.titleZH, "en-US": pd.titleEN},
			SummaryJSON:  jsonmap.JSON{"zh-CN": pd.summaryZH, "en-US": pd.summaryEN},
			ContentJSON:  jsonmap.JSON{"zh-CN": pd.contentZH, "en-US": pd.contentEN},
			CategoryID:   catID,
			CategorySlug: catSlug,
			IsPublished:  pd.published,
			PublishedAt:  &pubTime,
			CreatedAt:    pubTime,
		}
		if err := db.Create(&post).Error; err != nil {
			return fmt.Errorf("创建文章 %s 失败: %w", pd.slug, err)
		}
		postCount++

		// 部分文章添加缩略图（占位）
		if i < 8 {
			thumbnailURL := fmt.Sprintf("https://picsum.photos/seed/%d/800/400", i+100)
			db.Model(&post).Update("thumbnail", thumbnailURL)
		}
	}
	log.Printf("[%s] 文章=%d", label, postCount)
	return nil
}

// ---------- Banner ----------

func seedBanners(db *gorm.DB, label string) error {
	type bannerDef struct {
		name       string
		titleZH    string
		titleEN    string
		subtitleZH string
		subtitleEN string
		linkType   string
		linkValue  string
		sort       int
	}
	banners := []bannerDef{
		{"春节特惠", "🎉 春节特惠活动", "Spring Festival Sale",
			"全场商品限时折扣，低至8折", "Store-wide discounts up to 20% off",
			"none", "", 1},
		{"新用户福利", "🎁 新用户专享福利", "New User Benefits",
			"注册即送50元优惠券，首单再减10元", "Sign up for 50 CNY coupon + 10 CNY off first order",
			"none", "", 2},
		{"会员升级", "👑 升级会员享受更多", "Upgrade to VIP",
			"钻石会员享88折优惠，更有专属客服", "Diamond members enjoy 12% discount and VIP support",
			"none", "", 3},
		{"Steam礼品卡", "🎮 Steam 礼品卡热卖中", "Steam Gift Cards",
			"即买即用，自动发货", "Instant delivery, buy now",
			"product", "/products/steam-gift-50", 4},
		{"Windows激活码", "💻 正版 Windows 激活码", "Genuine Windows Keys",
			"Win11 Pro 仅需 ¥158，终身激活", "Win11 Pro only ¥158, lifetime activation",
			"product", "/products/win11-pro-key", 5},
	}

	// 清理旧测试 banner
	if err := db.Unscoped().Where("name IN (?)", []string{
		"春节特惠", "新用户福利", "会员升级", "Steam礼品卡", "Windows激活码",
	}).Delete(&contentdomain.Banner{}).Error; err != nil {
		return fmt.Errorf("清理旧测试Banner失败: %w", err)
	}

	now := time.Now().UTC()
	bannerCount := 0
	for _, bd := range banners {
		banner := contentdomain.Banner{
			Name:         bd.name,
			Position:     "home_hero",
			TitleJSON:    jsonmap.JSON{"zh-CN": bd.titleZH, "en-US": bd.titleEN},
			SubtitleJSON: jsonmap.JSON{"zh-CN": bd.subtitleZH, "en-US": bd.subtitleEN},
			Image:        fmt.Sprintf("https://picsum.photos/seed/banner%d/1200/400", bannerCount),
			MobileImage:  fmt.Sprintf("https://picsum.photos/seed/banner%d-m/750/600", bannerCount),
			LinkType:     bd.linkType,
			LinkValue:    bd.linkValue,
			OpenInNewTab: false,
			IsActive:     true,
			StartAt:      nil,
			EndAt:        nil,
			SortOrder:    bd.sort,
			CreatedAt:    now,
		}
		if err := db.Create(&banner).Error; err != nil {
			return fmt.Errorf("创建Banner %s 失败: %w", bd.name, err)
		}
		bannerCount++
	}
	log.Printf("[%s] Banner=%d", label, bannerCount)
	return nil
}

// ---------- 全部内容数据（设置+分类+文章+Banner） ----------

func seedContent(db *gorm.DB, label string) error {
	if err := seedSiteConfig(db, label); err != nil {
		return err
	}
	catIDs, err := seedPostCategories(db, label)
	if err != nil {
		return err
	}
	if err := seedPosts(db, catIDs, label); err != nil {
		return err
	}
	if err := seedBanners(db, label); err != nil {
		return err
	}
	return nil
}

func main() {
	mode := "members"
	if len(os.Args) > 1 {
		mode = strings.ToLower(os.Args[1])
	}

	switch mode {
	case "tags":
		sdb, err := open("sqlite", sqliteDSN)
		if err != nil {
			log.Fatalf("连接 SQLite 失败: %v", err)
		}
		if err := seedProductTags(sdb, "SQLite"); err != nil {
			log.Fatalf("SQLite 标签填充失败: %v", err)
		}
		pdb, err := open("postgres", pgDSN)
		if err != nil {
			log.Fatalf("连接 PostgreSQL 失败: %v", err)
		}
		if err := seedProductTags(pdb, "PostgreSQL"); err != nil {
			log.Fatalf("PostgreSQL 标签填充失败: %v", err)
		}
		log.Println("[DONE] 双库产品标签填充完成")

	case "randomize":
		sdb, err := open("sqlite", sqliteDSN)
		if err != nil {
			log.Fatalf("连接 SQLite 失败: %v", err)
		}
		if err := randomizeProducts(sdb, "SQLite"); err != nil {
			log.Fatalf("SQLite 随机化失败: %v", err)
		}
		pdb, err := open("postgres", pgDSN)
		if err != nil {
			log.Fatalf("连接 PostgreSQL 失败: %v", err)
		}
		if err := randomizeProducts(pdb, "PostgreSQL"); err != nil {
			log.Fatalf("PostgreSQL 随机化失败: %v", err)
		}
		log.Println("[DONE] 双库商品随机化 + 卡密生成完成")

	case "orders":
		if err := seedOrdersMain("sqlite", sqliteDSN, "SQLite"); err != nil {
			log.Fatalf("SQLite 订单填充失败: %v", err)
		}
		if err := seedOrdersMain("postgres", pgDSN, "PostgreSQL"); err != nil {
			log.Fatalf("PostgreSQL 订单填充失败: %v", err)
		}
		log.Println("[DONE] 双库订单填充完成：500 个订单覆盖 90 天 + 订单项 + 支付记录")

	case "analytics":
		sdb, err := open("sqlite", sqliteDSN)
		if err != nil {
			log.Fatalf("连接 SQLite 失败: %v", err)
		}
		catIDs, err := seedCategories(sdb, "SQLiteCat")
		if err != nil {
			log.Fatalf("SQLite 分类填充失败: %v", err)
		}
		if err := seedProducts(sdb, catIDs, "SQLiteProd"); err != nil {
			log.Fatalf("SQLite 商品填充失败: %v", err)
		}
		if err := seedMembers(sdb, "SQLiteMembers"); err != nil {
			log.Fatalf("SQLite 会员填充失败: %v", err)
		}
		if err := seedContent(sdb, "SQLiteContent"); err != nil {
			log.Fatalf("SQLite 内容填充失败: %v", err)
		}
		if err := seedOrders(sdb, "SQLiteOrders"); err != nil {
			log.Fatalf("SQLite 订单填充失败: %v", err)
		}
		if err := seedFulfillments(sdb, "SQLiteFulfill"); err != nil {
			log.Fatalf("SQLite 交付填充失败: %v", err)
		}
		if err := seedRefundRecords(sdb, "SQLiteRefund"); err != nil {
			log.Fatalf("SQLite 退款记录填充失败: %v", err)
		}
		if err := seedUserLoginLogs(sdb, "SQLiteLogin"); err != nil {
			log.Fatalf("SQLite 登录日志填充失败: %v", err)
		}
		log.Println("[DONE] SQLite 全量种子数据填充完成：分类+商品+会员+内容+订单+交付+退款+登录日志")
		// PostgreSQL 跳过（仅本地开发用 SQLite）

	case "content":
		sdb, err := open("sqlite", sqliteDSN)
		if err != nil {
			log.Fatalf("连接 SQLite 失败: %v", err)
		}
		if err := seedContent(sdb, "SQLite"); err != nil {
			log.Fatalf("SQLite 内容填充失败: %v", err)
		}
		log.Println("[DONE] SQLite 内容填充完成：站点配置+文章分类+文章+Banner")

	default: // members
		sdb, err := open("sqlite", sqliteDSN)
		if err != nil {
			log.Fatalf("连接 SQLite 失败: %v", err)
		}
		if err := seedMembers(sdb, "SQLite"); err != nil {
			log.Fatalf("SQLite 填充失败: %v", err)
		}
		log.Println("[DONE] SQLite 会员填充完成：5 个会员等级 + 100 个会员")
		// PostgreSQL 跳过（仅本地开发用 SQLite）
	}
}
