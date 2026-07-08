package service

import (
	"regexp"
	"sort"
	"strings"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/models"
)

var settingSupportedLanguages = append([]string(nil), constants.SupportedLocales...)
var settingCurrencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

const (
	settingSiteScriptsMaxCount       = 20
	settingSiteScriptNameMaxRuneSize = 120
	settingSiteScriptCodeMaxRuneSize = 20000

	settingSiteFooterLinksMaxCount       = 20
	settingSiteFooterLinkNameMaxRuneSize = 120
	settingSiteFooterLinkURLMaxRuneSize  = 2000

	settingNavCustomItemsMaxCount        = 10
	settingNavCustomItemTitleMaxRuneSize = 120
	settingNavCustomItemURLMaxRuneSize   = 2000

	settingRegistrationEmailDomainMaxCount  = 100
	settingRegistrationEmailDomainMaxLength = 253
)

// normalizeSettingValueByKey 按设置键执行归一化，避免非法值入库。
func normalizeSettingValueByKey(key string, value map[string]interface{}) models.JSON {
	switch key {
	case constants.SettingKeyDashboardConfig:
		setting := dashboardSettingFromJSON(models.JSON(value), DashboardDefaultSetting())
		return DashboardSettingToMap(setting)
	case constants.SettingKeyOrderConfig:
		cfg := orderConfigFromJSON(models.JSON(value), DefaultOrderConfig())
		return OrderConfigToMap(cfg)
	case constants.SettingKeySiteConfig:
		return normalizeSiteSetting(value)
	case constants.SettingKeyTelegramAuthConfig:
		setting := telegramAuthSettingFromJSON(models.JSON(value), TelegramAuthDefaultSetting(config.TelegramAuthConfig{}))
		return TelegramAuthSettingToMap(setting)
	case constants.SettingKeyNotificationCenterConfig:
		setting := notificationCenterSettingFromJSON(models.JSON(value), NotificationCenterDefaultSetting())
		return NotificationCenterSettingToMap(setting)
	case constants.SettingKeyAffiliateConfig:
		return normalizeAffiliateSettingMap(value)
	case constants.SettingKeyTelegramBotConfig:
		return normalizeTelegramBotConfig(models.JSON(value))
	case constants.SettingKeyNavConfig:
		return normalizeNavConfig(value)
	case constants.SettingKeyRegistrationConfig:
		return normalizeRegistrationSetting(value)
	case constants.SettingKeyOrderRiskControlConfig:
		cfg := orderRiskControlConfigFromJSON(models.JSON(value), DefaultOrderRiskControlConfig())
		return OrderRiskControlConfigToMap(cfg)
	case constants.SettingKeyUpstreamSyncConfig:
		cfg := upstreamSyncConfigFromJSON(models.JSON(value), DefaultUpstreamSyncConfig())
		return UpstreamSyncConfigToMap(cfg)
	case constants.SettingKeyCallbackRoutesConfig:
		return normalizeCallbackRoutesSetting(value)
	case constants.SettingKeyHomeAnnouncement:
		return normalizeHomeAnnouncement(value)
	default:
		return models.JSON(value)
	}
}

// normalizeSiteSetting 归一化站点配置结构。
func normalizeSiteSetting(value map[string]interface{}) models.JSON {
	normalized := make(models.JSON, len(value)+8)
	for key, raw := range value {
		normalized[key] = raw
	}

	normalized["brand"] = normalizeSiteBrand(value["brand"])
	normalized["contact"] = normalizeSiteContact(value["contact"])
	normalized["seo"] = normalizeSiteLocalizedBlock(value["seo"], []string{"title", "keywords", "description"})
	normalized["legal"] = normalizeSiteLocalizedBlock(value["legal"], []string{"terms", "privacy"})
	normalized["about"] = normalizeSiteAbout(value["about"])
	normalized["scripts"] = normalizeSiteScripts(value["scripts"])
	normalized["footer_links"] = normalizeSiteFooterLinks(value["footer_links"])
	normalized[constants.SettingFieldSiteCurrency] = normalizeSiteCurrency(value[constants.SettingFieldSiteCurrency])
	normalized["template_mode"] = normalizeSiteTemplateMode(value["template_mode"])
	normalized[constants.SettingFieldStorefrontTemplate] = normalizeStorefrontTemplate(value[constants.SettingFieldStorefrontTemplate])

	if raw, ok := value["languages"]; ok {
		normalized["languages"] = normalizeSiteLanguages(raw)
	}

	return normalized
}

func normalizeSiteScripts(raw interface{}) []interface{} {
	listRaw, ok := raw.([]interface{})
	if !ok {
		return make([]interface{}, 0)
	}

	result := make([]interface{}, 0, len(listRaw))
	for _, itemRaw := range listRaw {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}

		code := normalizeSettingTextWithRuneLimit(itemMap["code"], settingSiteScriptCodeMaxRuneSize)
		if code == "" {
			continue
		}

		position := normalizeSettingText(itemMap["position"])
		if position != "head" && position != "body_end" {
			position = "head"
		}

		result = append(result, map[string]interface{}{
			"name":     normalizeSettingTextWithRuneLimit(itemMap["name"], settingSiteScriptNameMaxRuneSize),
			"enabled":  parseSettingBool(itemMap["enabled"]),
			"position": position,
			"code":     code,
		})

		if len(result) >= settingSiteScriptsMaxCount {
			break
		}
	}

	return result
}

func normalizeSiteFooterLinks(raw interface{}) []interface{} {
	listRaw, ok := raw.([]interface{})
	if !ok {
		return make([]interface{}, 0)
	}

	result := make([]interface{}, 0, len(listRaw))
	for _, itemRaw := range listRaw {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}

		name := normalizeSettingTextWithRuneLimit(itemMap["name"], settingSiteFooterLinkNameMaxRuneSize)
		if name == "" {
			continue
		}

		url := normalizeSettingTextWithRuneLimit(itemMap["url"], settingSiteFooterLinkURLMaxRuneSize)

		result = append(result, map[string]interface{}{
			"name": name,
			"url":  url,
		})

		if len(result) >= settingSiteFooterLinksMaxCount {
			break
		}
	}

	return result
}

func normalizeSiteContact(raw interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"telegram": "",
		"whatsapp": "",
	}
	contactMap, ok := raw.(map[string]interface{})
	if !ok {
		return result
	}
	result["telegram"] = normalizeSettingText(contactMap["telegram"])
	result["whatsapp"] = normalizeSettingText(contactMap["whatsapp"])
	return result
}

func normalizeSiteBrand(raw interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"site_name":        "",
		"site_url":         "",
		"site_icon":        "",
		"site_description": normalizeSiteLocalizedField(nil),
	}
	brandMap, ok := raw.(map[string]interface{})
	if !ok {
		return result
	}
	result["site_name"] = normalizeSettingText(brandMap["site_name"])
	result["site_url"] = strings.TrimRight(normalizeSettingText(brandMap["site_url"]), "/")
	result["site_icon"] = normalizeSettingText(brandMap["site_icon"])
	result["site_description"] = normalizeSiteLocalizedField(brandMap["site_description"])
	return result
}

func normalizeSiteLocalizedBlock(raw interface{}, fields []string) map[string]interface{} {
	result := make(map[string]interface{}, len(fields))
	blockMap, _ := raw.(map[string]interface{})

	for _, field := range fields {
		if blockMap == nil {
			result[field] = normalizeSiteLocalizedField(nil)
			continue
		}
		result[field] = normalizeSiteLocalizedField(blockMap[field])
	}

	return result
}

func normalizeSiteLocalizedField(raw interface{}) map[string]interface{} {
	fieldResult := make(map[string]interface{}, len(settingSupportedLanguages))
	for _, language := range settingSupportedLanguages {
		fieldResult[language] = ""
	}

	fieldRaw, ok := raw.(map[string]interface{})
	if !ok {
		return fieldResult
	}

	for _, language := range settingSupportedLanguages {
		fieldResult[language] = normalizeSettingText(fieldRaw[language])
	}

	return fieldResult
}

func normalizeSiteLocalizedList(raw interface{}, maxItems int) []interface{} {
	listRaw, ok := raw.([]interface{})
	if !ok {
		return make([]interface{}, 0)
	}

	result := make([]interface{}, 0, len(listRaw))
	for _, item := range listRaw {
		normalized := normalizeSiteLocalizedField(item)
		hasText := false
		for _, language := range settingSupportedLanguages {
			text, _ := normalized[language].(string)
			if text != "" {
				hasText = true
				break
			}
		}
		if !hasText {
			continue
		}

		result = append(result, normalized)
		if maxItems > 0 && len(result) >= maxItems {
			break
		}
	}

	return result
}

func normalizeSiteAbout(raw interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"hero":         normalizeSiteLocalizedBlock(nil, []string{"title", "subtitle"}),
		"introduction": normalizeSiteLocalizedField(nil),
		"services": map[string]interface{}{
			"title": normalizeSiteLocalizedField(nil),
			"items": make([]interface{}, 0),
		},
		"contact": map[string]interface{}{
			"title": normalizeSiteLocalizedField(nil),
			"text":  normalizeSiteLocalizedField(nil),
		},
	}

	aboutMap, ok := raw.(map[string]interface{})
	if !ok {
		return result
	}

	result["hero"] = normalizeSiteLocalizedBlock(aboutMap["hero"], []string{"title", "subtitle"})
	result["introduction"] = normalizeSiteLocalizedField(aboutMap["introduction"])

	services := map[string]interface{}{
		"title": normalizeSiteLocalizedField(nil),
		"items": make([]interface{}, 0),
	}
	if servicesRaw, ok := aboutMap["services"].(map[string]interface{}); ok {
		services["title"] = normalizeSiteLocalizedField(servicesRaw["title"])
		services["items"] = normalizeSiteLocalizedList(servicesRaw["items"], 12)
	}
	result["services"] = services

	contact := map[string]interface{}{
		"title": normalizeSiteLocalizedField(nil),
		"text":  normalizeSiteLocalizedField(nil),
	}
	if contactRaw, ok := aboutMap["contact"].(map[string]interface{}); ok {
		contact["title"] = normalizeSiteLocalizedField(contactRaw["title"])
		contact["text"] = normalizeSiteLocalizedField(contactRaw["text"])
	}
	result["contact"] = contact

	return result
}

func normalizeSiteLanguages(raw interface{}) []string {
	list := make([]string, 0)
	switch value := raw.(type) {
	case []string:
		list = append(list, value...)
	case []interface{}:
		for _, item := range value {
			list = append(list, normalizeSettingText(item))
		}
	default:
		return append([]string(nil), settingSupportedLanguages...)
	}

	result := make([]string, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, item := range list {
		lang := strings.TrimSpace(item)
		if lang == "" {
			continue
		}
		if _, exists := seen[lang]; exists {
			continue
		}
		seen[lang] = struct{}{}
		result = append(result, lang)
	}
	if len(result) == 0 {
		return append([]string(nil), settingSupportedLanguages...)
	}
	return result
}

// normalizeSiteTemplateMode 归一化站点模板模式，允许 "card" 或 "list"，默认 "card"。
func normalizeSiteTemplateMode(raw interface{}) string {
	mode := normalizeSettingText(raw)
	if mode == "list" {
		return "list"
	}
	return "card"
}

// normalizeStorefrontTemplate 归一化店面模板，允许 "classic" 或 "vault"，默认 "classic"。
func normalizeStorefrontTemplate(raw interface{}) string {
	if normalizeSettingText(raw) == constants.StorefrontTemplateVault {
		return constants.StorefrontTemplateVault
	}
	return constants.StorefrontTemplateDefault
}

// navBuiltinKeys 是内置导航项的白名单与默认顺序（不可删除）。
var navBuiltinKeys = []string{"blog", "notice", "about"}

// navBuiltinKeyAllowed 判断给定 key 是否属于内置导航白名单。
func navBuiltinKeyAllowed(key string) bool {
	for _, allowed := range navBuiltinKeys {
		if allowed == key {
			return true
		}
	}
	return false
}

// navOrderedEntry 是导航项在全局排序时的中间表示。
type navOrderedEntry struct {
	order   int
	payload map[string]interface{}
}

// navEntryOrder 读取导航条目的 sort_order，缺失或非法时回退 0。
func navEntryOrder(item map[string]interface{}) int {
	if v, err := parseSettingInt(item["sort_order"]); err == nil {
		return v
	}
	return 0
}

// assignGlobalNavOrder 合并内置项与自定义项，按 sort_order 稳定升序排序后
// 规整为连续的全局 sort_order（0..N），并写回各自条目。
// 合并时内置项在前，故同 sort_order 时内置优先（确定性兜底）。
func assignGlobalNavOrder(builtin, custom []map[string]interface{}) {
	entries := make([]navOrderedEntry, 0, len(builtin)+len(custom))
	for _, item := range builtin {
		entries = append(entries, navOrderedEntry{order: navEntryOrder(item), payload: item})
	}
	for _, item := range custom {
		entries = append(entries, navOrderedEntry{order: navEntryOrder(item), payload: item})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].order < entries[j].order
	})

	for index := range entries {
		entries[index].payload["sort_order"] = index
	}
}

// offsetNavCustomOrder 将自定义项整体排到内置项之后（用于旧布尔字典迁移场景）：
// 先按自定义项各自 sort_order 稳定排序，再顺延内置项之后赋全局序号。
func offsetNavCustomOrder(builtin, custom []map[string]interface{}) {
	sort.SliceStable(custom, func(i, j int) bool {
		return navEntryOrder(custom[i]) < navEntryOrder(custom[j])
	})
	base := len(builtin)
	for index, item := range custom {
		item["sort_order"] = base + index
	}
}

// toInterfaceSlice 将 []map[string]interface{} 转为 []interface{}，供 models.JSON 存储。
func toInterfaceSlice(items []map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	return result
}

// normalizeNavBuiltinItems 将 builtin 段（有序数组或旧布尔字典）归一化为规范有序数组
// [{key, enabled, sort_order}]。始终保留全部白名单 key（不可删除），非法 key 丢弃、
// 重复去重、缺失补齐、enabled 缺省 true。当入参为数组时 fromArray=true，
// 供上游决定自定义项的排序策略（数组=全局穿插，字典=内置在前自定义顺延）。
func normalizeNavBuiltinItems(raw interface{}) (items []map[string]interface{}, fromArray bool) {
	enabledMap := make(map[string]bool, len(navBuiltinKeys))
	orderMap := make(map[string]int, len(navBuiltinKeys))
	hasOrder := make(map[string]bool, len(navBuiltinKeys))
	for _, key := range navBuiltinKeys {
		enabledMap[key] = true
	}

	switch typed := raw.(type) {
	case []interface{}:
		fromArray = true
		for index, entryRaw := range typed {
			entryMap, ok := entryRaw.(map[string]interface{})
			if !ok {
				continue
			}
			key := normalizeSettingText(entryMap["key"])
			if !navBuiltinKeyAllowed(key) {
				continue
			}
			if hasOrder[key] {
				continue // 去重：保留首次出现
			}
			enabled := true
			if v, exists := entryMap["enabled"]; exists {
				enabled = parseSettingBool(v)
			}
			enabledMap[key] = enabled
			order := index
			if v, err := parseSettingInt(entryMap["sort_order"]); err == nil {
				order = v
			}
			orderMap[key] = order
			hasOrder[key] = true
		}
	case map[string]interface{}:
		// 旧布尔字典 {blog:true, notice:true, about:true}
		for _, key := range navBuiltinKeys {
			if v, exists := typed[key]; exists {
				enabledMap[key] = parseSettingBool(v)
			}
		}
	}

	items = make([]map[string]interface{}, 0, len(navBuiltinKeys))
	for defaultIndex, key := range navBuiltinKeys {
		order := defaultIndex
		if hasOrder[key] {
			order = orderMap[key]
		}
		items = append(items, map[string]interface{}{
			"key":        key,
			"enabled":    enabledMap[key],
			"sort_order": order,
		})
	}
	return items, fromArray
}

// normalizeNavCustomItemsStrict 严格归一化自定义导航项（管理端写入路径）：
// 跳过全空标题项、截断长度、字段白名单、最多 settingNavCustomItemsMaxCount 项。
func normalizeNavCustomItemsStrict(raw interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	itemsRaw, ok := raw.([]interface{})
	if !ok {
		return result
	}
	for _, itemRaw := range itemsRaw {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}

		title := normalizeSiteLocalizedField(itemMap["title"])
		// 跳过全空标题项
		allEmpty := true
		for _, lang := range settingSupportedLanguages {
			if s, _ := title[lang].(string); s != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}
		// 截断标题长度
		for _, lang := range settingSupportedLanguages {
			if s, _ := title[lang].(string); s != "" {
				title[lang] = normalizeSettingTextWithRuneLimit(s, settingNavCustomItemTitleMaxRuneSize)
			}
		}

		linkType := normalizeSettingText(itemMap["link_type"])
		if linkType != "internal" && linkType != "external" {
			linkType = "internal"
		}

		target := normalizeSettingText(itemMap["target"])
		if target != "_self" && target != "_blank" {
			target = "_self"
		}

		url := normalizeSettingTextWithRuneLimit(itemMap["url"], settingNavCustomItemURLMaxRuneSize)

		sortOrder := 0
		if v, err := parseSettingInt(itemMap["sort_order"]); err == nil {
			sortOrder = v
		}

		icon := normalizeSettingText(itemMap["icon"])
		if icon == "" {
			icon = "link"
		}

		// 保留前端生成的 id
		id := float64(0)
		if v, ok := itemMap["id"].(float64); ok {
			id = v
		}

		result = append(result, map[string]interface{}{
			"id":         id,
			"title":      title,
			"link_type":  linkType,
			"url":        url,
			"target":     target,
			"sort_order": sortOrder,
			"enabled":    parseSettingBool(itemMap["enabled"]),
			"icon":       icon,
		})

		if len(result) >= settingNavCustomItemsMaxCount {
			break
		}
	}
	return result
}

// normalizeNavConfig 归一化导航配置（管理端写入路径，严格）。
// builtin 归一化为有序数组 [{key,enabled,sort_order}]（始终保留全部白名单 key，不可删除）；
// custom_items 严格归一化；builtin 与 custom_items 共享同一全局 sort_order 空间并规整为连续 0..N。
func normalizeNavConfig(value map[string]interface{}) models.JSON {
	builtin, fromArray := normalizeNavBuiltinItems(value["builtin"])
	custom := normalizeNavCustomItemsStrict(value["custom_items"])
	if !fromArray {
		// 旧布尔字典：内置默认序在前，自定义整体顺延其后（等价升级前视觉）
		offsetNavCustomOrder(builtin, custom)
	}
	assignGlobalNavOrder(builtin, custom)
	return models.JSON{
		"builtin":      toInterfaceSlice(builtin),
		"custom_items": toInterfaceSlice(custom),
	}
}

// normalizeNavCustomItemsLenient 宽松归一化自定义导航项（下发层 shim）：
// 透传既有字段、不因缺 title 丢弃（兼容经销商 {name,url} 形态），仅补齐 sort_order。
func normalizeNavCustomItemsLenient(raw interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	itemsRaw, ok := raw.([]interface{})
	if !ok {
		return result
	}
	for _, itemRaw := range itemsRaw {
		itemMap, ok := itemRaw.(map[string]interface{})
		if !ok {
			continue
		}
		item := make(map[string]interface{}, len(itemMap)+1)
		for k, v := range itemMap {
			item[k] = v
		}
		item["sort_order"] = navEntryOrder(itemMap)
		result = append(result, item)
	}
	return result
}

// asStringMap 将任意 JSON 对象值（map[string]interface{} 或 models.JSON）转为 map[string]interface{}。
func asStringMap(raw interface{}) map[string]interface{} {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return typed
	case models.JSON:
		return map[string]interface{}(typed)
	default:
		return map[string]interface{}{}
	}
}

// NormalizeNavConfigForPublic 归一化下发的导航配置（下发层 shim，宽松）。
// builtin 字典/数组均转为规范有序数组；custom_items 透传不丢弃（兼容经销商 {name,url} 形态）、
// 补齐 sort_order；builtin 与 custom_items 共享全局 sort_order 并规整为连续 0..N。
// 无输入时产出规范默认（3 个启用内置项 + 空自定义列表）。
func NormalizeNavConfigForPublic(raw interface{}) models.JSON {
	value := asStringMap(raw)
	builtin, fromArray := normalizeNavBuiltinItems(value["builtin"])
	custom := normalizeNavCustomItemsLenient(value["custom_items"])
	if !fromArray {
		offsetNavCustomOrder(builtin, custom)
	}
	assignGlobalNavOrder(builtin, custom)
	return models.JSON{
		"builtin":      toInterfaceSlice(builtin),
		"custom_items": toInterfaceSlice(custom),
	}
}

func normalizeRegistrationEmailDomains(raw interface{}) []string {
	var candidates []string
	switch value := raw.(type) {
	case string:
		candidates = strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		})
	case []string:
		candidates = append(candidates, value...)
	case []interface{}:
		for _, item := range value {
			candidates = append(candidates, normalizeSettingText(item))
		}
	default:
		return []string{}
	}

	seen := make(map[string]struct{}, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		domain := strings.ToLower(strings.TrimSpace(candidate))
		domain = strings.TrimPrefix(domain, "@")
		if len(domain) == 0 || len(domain) > settingRegistrationEmailDomainMaxLength {
			continue
		}
		if !isValidRegistrationEmailDomain(domain) {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
		if len(result) >= settingRegistrationEmailDomainMaxCount {
			break
		}
	}
	return result
}

func isValidRegistrationEmailDomain(domain string) bool {
	if strings.Contains(domain, "..") || !strings.Contains(domain, ".") {
		return false
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

// normalizeRegistrationSetting 归一化注册配置。
func normalizeRegistrationSetting(value map[string]interface{}) models.JSON {
	normalized := make(models.JSON, 4)
	registrationEnabled := true
	if raw, ok := value[constants.SettingFieldRegistrationEnabled]; ok {
		registrationEnabled = parseSettingBool(raw)
	}
	normalized[constants.SettingFieldRegistrationEnabled] = registrationEnabled

	emailVerificationEnabled := true
	if raw, ok := value[constants.SettingFieldEmailVerificationEnabled]; ok {
		emailVerificationEnabled = parseSettingBool(raw)
	}
	normalized[constants.SettingFieldEmailVerificationEnabled] = emailVerificationEnabled

	emailDomainAllowlistEnabled := false
	if raw, ok := value[constants.SettingFieldEmailDomainAllowlistEnabled]; ok {
		emailDomainAllowlistEnabled = parseSettingBool(raw)
	}
	normalized[constants.SettingFieldEmailDomainAllowlistEnabled] = emailDomainAllowlistEnabled
	normalized[constants.SettingFieldAllowedEmailDomains] = normalizeRegistrationEmailDomains(value[constants.SettingFieldAllowedEmailDomains])

	return normalized
}
