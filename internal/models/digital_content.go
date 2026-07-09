package models

import (
	"time"

	"gorm.io/gorm"
)

// DigitalContent 数字内容本体（可复用、不消耗）
// 一条内容可绑定多个商品/SKU，交付后不改变状态、不消耗库存内容本身。
type DigitalContent struct {
	ID        uint           `gorm:"primarykey" json:"id"`                    // 主键
	Title     string         `gorm:"type:varchar(255);not null" json:"title"` // 管理识别用标题
	Content   string         `gorm:"type:text;not null" json:"content"`       // 交付给用户的文本
	IsActive  bool           `gorm:"not null;default:true" json:"is_active"`  // 是否启用
	CreatedAt time.Time      `json:"created_at"`                              // 创建时间
	UpdatedAt time.Time      `json:"updated_at"`                              // 更新时间
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`                          // 软删除时间

	Bindings []DigitalContentBinding `gorm:"foreignKey:DigitalContentID" json:"bindings,omitempty"` // 绑定关系
}

// TableName 指定表名
func (DigitalContent) TableName() string {
	return "digital_contents"
}

// DigitalContentBinding 数字内容与商品/SKU 的绑定（一条内容 → 多个商品/SKU）
// 采用硬删除（无 DeletedAt），避免软删行占用唯一索引导致重复绑定冲突。
type DigitalContentBinding struct {
	ID               uint      `gorm:"primarykey" json:"id"`                                                             // 主键
	DigitalContentID uint      `gorm:"not null;index" json:"digital_content_id"`                                         // 数字内容ID
	ProductID        uint      `gorm:"not null;uniqueIndex:uq_dc_bind_product_sku" json:"product_id"`                     // 商品ID
	SKUID            uint      `gorm:"column:sku_id;not null;default:0;uniqueIndex:uq_dc_bind_product_sku" json:"sku_id"` // 0=商品级，>0=SKU级
	CreatedAt        time.Time `json:"created_at"`                                                                       // 创建时间
	UpdatedAt        time.Time `json:"updated_at"`                                                                       // 更新时间
}

// TableName 指定表名
func (DigitalContentBinding) TableName() string {
	return "digital_content_bindings"
}
