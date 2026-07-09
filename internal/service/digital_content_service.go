package service

import (
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/models"
	"github.com/dujiao-next/internal/repository"
)

// DigitalContentBindingInput 绑定入参
type DigitalContentBindingInput struct {
	ProductID uint
	SKUID     uint
}

// DigitalContentInput 数字内容创建/更新入参
type DigitalContentInput struct {
	Title    string
	Content  string
	IsActive bool
	Bindings []DigitalContentBindingInput
}

// DigitalContentService 数字内容业务服务
type DigitalContentService struct {
	repo           repository.DigitalContentRepository
	cardSecretRepo repository.CardSecretRepository
	productRepo    repository.ProductRepository
}

// NewDigitalContentService 创建数字内容服务
func NewDigitalContentService(repo repository.DigitalContentRepository, cardSecretRepo repository.CardSecretRepository, productRepo repository.ProductRepository) *DigitalContentService {
	return &DigitalContentService{repo: repo, cardSecretRepo: cardSecretRepo, productRepo: productRepo}
}

// List 数字内容列表
func (s *DigitalContentService) List(filter repository.DigitalContentListFilter) ([]models.DigitalContent, int64, error) {
	return s.repo.List(filter)
}

// GetByID 数字内容详情（含绑定）
func (s *DigitalContentService) GetByID(id uint) (*models.DigitalContent, error) {
	content, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, ErrDigitalContentNotFound
	}
	return content, nil
}

// Create 创建数字内容
func (s *DigitalContentService) Create(input DigitalContentInput) (*models.DigitalContent, error) {
	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	if title == "" || content == "" {
		return nil, ErrDigitalContentInvalid
	}
	bindings, err := s.normalizeBindings(input.Bindings)
	if err != nil {
		return nil, err
	}
	if err := s.checkProductsExist(bindings); err != nil {
		return nil, err
	}
	if err := s.checkCardSecretConflicts(bindings); err != nil {
		return nil, err
	}
	if err := s.checkBindingConflicts(bindings, 0); err != nil {
		return nil, err
	}

	dc := &models.DigitalContent{Title: title, Content: content, IsActive: input.IsActive}
	if err := s.repo.Create(dc); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceBindings(dc.ID, bindings); err != nil {
		return nil, err
	}
	return s.GetByID(dc.ID)
}

// Update 更新数字内容
func (s *DigitalContentService) Update(id uint, input DigitalContentInput) (*models.DigitalContent, error) {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrDigitalContentNotFound
	}
	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	if title == "" || content == "" {
		return nil, ErrDigitalContentInvalid
	}
	bindings, err := s.normalizeBindings(input.Bindings)
	if err != nil {
		return nil, err
	}
	if err := s.checkProductsExist(bindings); err != nil {
		return nil, err
	}
	if err := s.checkCardSecretConflicts(bindings); err != nil {
		return nil, err
	}
	if err := s.checkBindingConflicts(bindings, id); err != nil {
		return nil, err
	}

	existing.Title = title
	existing.Content = content
	existing.IsActive = input.IsActive
	if err := s.repo.Update(existing); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceBindings(id, bindings); err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

// Delete 删除数字内容
func (s *DigitalContentService) Delete(id uint) error {
	existing, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrDigitalContentNotFound
	}
	return s.repo.Delete(id)
}

// normalizeBindings 去重并转换绑定入参
func (s *DigitalContentService) normalizeBindings(inputs []DigitalContentBindingInput) ([]models.DigitalContentBinding, error) {
	seen := make(map[[2]uint]struct{}, len(inputs))
	out := make([]models.DigitalContentBinding, 0, len(inputs))
	for _, in := range inputs {
		if in.ProductID == 0 {
			return nil, ErrDigitalContentInvalid
		}
		key := [2]uint{in.ProductID, in.SKUID}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, models.DigitalContentBinding{ProductID: in.ProductID, SKUID: in.SKUID})
	}
	return out, nil
}

// checkProductsExist 校验绑定的商品均存在
func (s *DigitalContentService) checkProductsExist(bindings []models.DigitalContentBinding) error {
	if s.productRepo == nil {
		return nil
	}
	seen := make(map[uint]struct{}, len(bindings))
	for _, b := range bindings {
		if _, ok := seen[b.ProductID]; ok {
			continue
		}
		seen[b.ProductID] = struct{}{}
		product, err := s.productRepo.GetByID(strconv.FormatUint(uint64(b.ProductID), 10))
		if err != nil {
			return err
		}
		if product == nil {
			return ErrProductNotFound
		}
	}
	return nil
}

// checkBindingConflicts 校验绑定未被其它内容占用
func (s *DigitalContentService) checkBindingConflicts(bindings []models.DigitalContentBinding, excludeContentID uint) error {
	for _, b := range bindings {
		conflictID, err := s.repo.FindConflictContentID(b.ProductID, b.SKUID, excludeContentID)
		if err != nil {
			return err
		}
		if conflictID > 0 {
			return ErrDigitalContentBindingConflict
		}
	}
	return nil
}

// checkCardSecretConflicts 校验绑定商品未配置卡密（商品级互斥）
func (s *DigitalContentService) checkCardSecretConflicts(bindings []models.DigitalContentBinding) error {
	if s.cardSecretRepo == nil {
		return nil
	}
	seen := make(map[uint]struct{}, len(bindings))
	for _, b := range bindings {
		if _, ok := seen[b.ProductID]; ok {
			continue
		}
		seen[b.ProductID] = struct{}{}
		total, _, _, err := s.cardSecretRepo.CountByProduct(b.ProductID, 0)
		if err != nil {
			return err
		}
		if total > 0 {
			return ErrDigitalContentProductHasCardSecret
		}
	}
	return nil
}
