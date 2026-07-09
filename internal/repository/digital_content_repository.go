package repository

import (
	"errors"
	"strings"

	"github.com/dujiao-next/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DigitalContentListFilter 数字内容列表筛选条件
type DigitalContentListFilter struct {
	Title     string
	ProductID uint
	IsActive  *bool
	Page      int
	PageSize  int
}

// DigitalContentRepository 数字内容数据访问接口
type DigitalContentRepository interface {
	Create(content *models.DigitalContent) error
	Update(content *models.DigitalContent) error
	GetByID(id uint) (*models.DigitalContent, error)
	List(filter DigitalContentListFilter) ([]models.DigitalContent, int64, error)
	Delete(id uint) error
	// ReplaceBindings 全量替换某条内容的绑定（硬删旧 + 插新）。
	ReplaceBindings(contentID uint, bindings []models.DigitalContentBinding) error
	// FindContentByProductSKU 按 (product_id, sku_id) 读取可复用内容：SKU 级优先，回退商品级(sku_id=0)。
	FindContentByProductSKU(productID, skuID uint) (string, error)
	// CountBindingsByProduct 统计某商品(可选 SKU)已绑定的数字内容数量，用于互斥校验。
	CountBindingsByProduct(productID, skuID uint) (int64, error)
	// FindConflictContentID 返回已占用 (product_id, sku_id) 的其它内容 ID（0=无冲突）。
	FindConflictContentID(productID, skuID, excludeContentID uint) (uint, error)
}

// GormDigitalContentRepository 数字内容仓库 GORM 实现
type GormDigitalContentRepository struct {
	BaseRepository
}

// NewDigitalContentRepository 创建数字内容仓库
func NewDigitalContentRepository(db *gorm.DB) *GormDigitalContentRepository {
	return &GormDigitalContentRepository{BaseRepository: BaseRepository{db: db}}
}

// Create 创建数字内容（不含绑定，绑定由 ReplaceBindings 维护）
func (r *GormDigitalContentRepository) Create(content *models.DigitalContent) error {
	return r.db.Omit(clause.Associations).Create(content).Error
}

// Update 更新数字内容基础字段
func (r *GormDigitalContentRepository) Update(content *models.DigitalContent) error {
	return r.db.Model(&models.DigitalContent{}).Where("id = ?", content.ID).
		Updates(map[string]interface{}{
			"title":     content.Title,
			"content":   content.Content,
			"is_active": content.IsActive,
		}).Error
}

// GetByID 按 ID 查询（含绑定），不存在返回 nil。
func (r *GormDigitalContentRepository) GetByID(id uint) (*models.DigitalContent, error) {
	var content models.DigitalContent
	if err := r.db.Preload("Bindings").First(&content, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &content, nil
}

// List 查询数字内容列表（含绑定）
func (r *GormDigitalContentRepository) List(filter DigitalContentListFilter) ([]models.DigitalContent, int64, error) {
	query := r.db.Model(&models.DigitalContent{})
	if title := strings.TrimSpace(filter.Title); title != "" {
		query = query.Where("LOWER(title) LIKE LOWER(?)", "%"+title+"%")
	}
	if filter.IsActive != nil {
		query = query.Where("is_active = ?", *filter.IsActive)
	}
	if filter.ProductID > 0 {
		sub := r.db.Model(&models.DigitalContentBinding{}).
			Select("digital_content_id").
			Where("product_id = ?", filter.ProductID)
		query = query.Where("id IN (?)", sub)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	var contents []models.DigitalContent
	if err := applyPagination(query.Preload("Bindings").Order("id DESC"), filter.Page, pageSize).
		Find(&contents).Error; err != nil {
		return nil, 0, err
	}
	return contents, total, nil
}

// Delete 软删除内容并硬删除其绑定
func (r *GormDigitalContentRepository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("digital_content_id = ?", id).
			Delete(&models.DigitalContentBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.DigitalContent{}, id).Error
	})
}

// ReplaceBindings 全量替换某条内容的绑定
func (r *GormDigitalContentRepository) ReplaceBindings(contentID uint, bindings []models.DigitalContentBinding) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("digital_content_id = ?", contentID).
			Delete(&models.DigitalContentBinding{}).Error; err != nil {
			return err
		}
		if len(bindings) == 0 {
			return nil
		}
		for i := range bindings {
			bindings[i].ID = 0
			bindings[i].DigitalContentID = contentID
		}
		return tx.Create(&bindings).Error
	})
}

// FindContentByProductSKU 读取可复用内容：SKU 级优先，回退商品级(sku_id=0)。
func (r *GormDigitalContentRepository) FindContentByProductSKU(productID, skuID uint) (string, error) {
	if skuID > 0 {
		content, err := r.findActiveContent(productID, skuID)
		if err != nil {
			return "", err
		}
		if content != "" {
			return content, nil
		}
	}
	return r.findActiveContent(productID, 0)
}

func (r *GormDigitalContentRepository) findActiveContent(productID, skuID uint) (string, error) {
	var content string
	err := r.db.Model(&models.DigitalContentBinding{}).
		Select("digital_contents.content").
		Joins("JOIN digital_contents ON digital_contents.id = digital_content_bindings.digital_content_id AND digital_contents.deleted_at IS NULL").
		Where("digital_content_bindings.product_id = ? AND digital_content_bindings.sku_id = ?", productID, skuID).
		Where("digital_contents.is_active = ?", true).
		Limit(1).
		Scan(&content).Error
	if err != nil {
		return "", err
	}
	return content, nil
}

// CountBindingsByProduct 统计某商品(可选 SKU)已绑定的数字内容数量
func (r *GormDigitalContentRepository) CountBindingsByProduct(productID, skuID uint) (int64, error) {
	if productID == 0 {
		return 0, errors.New("invalid product id")
	}
	query := r.db.Model(&models.DigitalContentBinding{}).Where("product_id = ?", productID)
	if skuID > 0 {
		query = query.Where("sku_id = ?", skuID)
	}
	var count int64
	err := query.Count(&count).Error
	return count, err
}

// FindConflictContentID 返回已占用 (product_id, sku_id) 的其它内容 ID（0=无冲突）
func (r *GormDigitalContentRepository) FindConflictContentID(productID, skuID, excludeContentID uint) (uint, error) {
	var id uint
	query := r.db.Model(&models.DigitalContentBinding{}).
		Select("digital_content_id").
		Where("product_id = ? AND sku_id = ?", productID, skuID)
	if excludeContentID > 0 {
		query = query.Where("digital_content_id <> ?", excludeContentID)
	}
	if err := query.Limit(1).Scan(&id).Error; err != nil {
		return 0, err
	}
	return id, nil
}
