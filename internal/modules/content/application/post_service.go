package application

import (
	"context"
	"strconv"
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/content/contract"
	"github.com/dujiao-next/internal/modules/content/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// CreatePostInput 描述文章创建和更新所需字段。
type CreatePostInput struct {
	Slug        string
	Type        string
	TitleJSON   map[string]interface{}
	SummaryJSON map[string]interface{}
	ContentJSON map[string]interface{}
	Thumbnail   string
	IsPublished *bool
	ProductIDs  *[]uint
	CategoryID  *uint
}

// PublicPostQuery 描述公开文章列表查询。
type PublicPostQuery struct {
	Type       string
	Search     string
	CategoryID string
	Page       int
	PageSize   int
}

// AdminPostQuery 描述后台文章列表查询。
type AdminPostQuery struct {
	Type        string
	Search      string
	CategoryID  string
	IsPublished *bool
	Page        int
	PageSize    int
}

// PostService 实现文章用例。
type PostService struct {
	posts      contract.PostStore
	relations  contract.PostProductRelationStore
	categories contract.PostCategoryStore
	clock      contract.Clock
}

// NewPostService 创建文章用例服务。
func NewPostService(posts contract.PostStore, relations contract.PostProductRelationStore, categories contract.PostCategoryStore, clock contract.Clock) *PostService {
	if clock == nil {
		clock = SystemClock{}
	}
	return &PostService{
		posts:      posts,
		relations:  relations,
		categories: categories,
		clock:      clock,
	}
}

// ListPublic 获取公开文章列表。
func (s *PostService) ListPublic(ctx context.Context, query PublicPostQuery) ([]domain.Post, int64, error) {
	categoryIDs, err := s.expandPublicPostCategoryIDs(ctx, query.CategoryID)
	if err != nil {
		return nil, 0, err
	}
	return s.posts.List(ctx, contract.PostQuery{
		Page:          query.Page,
		PageSize:      query.PageSize,
		Type:          query.Type,
		Search:        query.Search,
		CategoryID:    strings.TrimSpace(query.CategoryID),
		CategoryIDs:   categoryIDs,
		OnlyPublished: true,
		Order:         contract.PostOrderPublishedDesc,
	})
}

// GetPublicBySlug 获取公开文章详情。
func (s *PostService) GetPublicBySlug(ctx context.Context, slug string) (*domain.Post, error) {
	post, err := s.posts.GetBySlug(ctx, slug, true)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, contract.ErrNotFound
	}
	return post, nil
}

// ListAdmin 获取后台文章列表。
func (s *PostService) ListAdmin(ctx context.Context, query AdminPostQuery) ([]domain.Post, int64, error) {
	return s.posts.List(ctx, contract.PostQuery{
		Page:        query.Page,
		PageSize:    query.PageSize,
		Type:        query.Type,
		Search:      query.Search,
		CategoryID:  strings.TrimSpace(query.CategoryID),
		IsPublished: query.IsPublished,
		Order:       contract.PostOrderCreatedDesc,
	})
}

// expandPublicPostCategoryIDs 展开公开文章列表的分类筛选条件，返回应命中的分类 ID。
//
// 语义与商品分类的 expandPublicCategoryIDs 保持一致：未传分类返回 nil（不筛选）；
// 一级分类展开为「自身 + 启用中的子分类」；二级分类只返回自身；
// 分类不存在时按原 ID 查询，分类已停用时返回空集。
func (s *PostService) expandPublicPostCategoryIDs(ctx context.Context, categoryID string) ([]uint, error) {
	normalizedCategoryID := strings.TrimSpace(categoryID)
	if normalizedCategoryID == "" {
		return nil, nil
	}

	parsedCategoryID, err := strconv.ParseUint(normalizedCategoryID, 10, 64)
	if err != nil || parsedCategoryID == 0 {
		return nil, nil
	}
	if s.categories == nil {
		return []uint{uint(parsedCategoryID)}, nil
	}

	category, err := s.categories.GetByID(ctx, uint(parsedCategoryID))
	if err != nil {
		return nil, err
	}
	if category == nil {
		return []uint{uint(parsedCategoryID)}, nil
	}
	if !category.IsActive {
		return []uint{}, nil
	}
	if category.ParentID != nil && *category.ParentID > 0 {
		return []uint{category.ID}, nil
	}

	categories, err := s.categories.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	categoryIDs := []uint{category.ID}
	for _, item := range categories {
		if item.ParentID != nil && *item.ParentID == category.ID {
			categoryIDs = append(categoryIDs, item.ID)
		}
	}
	return categoryIDs, nil
}

// Create 创建文章。
func (s *PostService) Create(ctx context.Context, input CreatePostInput) (*domain.Post, error) {
	if !isAllowedPostType(input.Type) {
		return nil, contract.ErrInvalidPostType
	}
	categoryID := normalizePostCategoryID(input.CategoryID)
	if err := s.validateCategoryAssignment(ctx, input.Type, categoryID, nil); err != nil {
		return nil, err
	}

	count, err := s.posts.CountBySlug(ctx, input.Slug, nil)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, contract.ErrSlugExists
	}

	isPublished := false
	if input.IsPublished != nil {
		isPublished = *input.IsPublished
	}
	post := domain.Post{
		Slug:        input.Slug,
		Type:        input.Type,
		TitleJSON:   jsonmap.JSON(input.TitleJSON),
		SummaryJSON: jsonmap.JSON(input.SummaryJSON),
		ContentJSON: jsonmap.JSON(input.ContentJSON),
		Thumbnail:   input.Thumbnail,
		IsPublished: isPublished,
		CategoryID:  categoryID,
	}
	if isPublished {
		now := s.clock.Now()
		post.PublishedAt = &now
	}

	if err := s.posts.WithinPostWriteTransaction(ctx, func(posts contract.PostStore, relations contract.PostProductRelationStore) error {
		if err := posts.Create(ctx, &post); err != nil {
			return err
		}
		if input.ProductIDs == nil {
			return nil
		}
		return relations.SetRelatedProductIDs(ctx, post.ID, *input.ProductIDs)
	}); err != nil {
		return nil, err
	}
	return &post, nil
}

// Update 更新文章。
func (s *PostService) Update(ctx context.Context, id string, input CreatePostInput) (*domain.Post, error) {
	if !isAllowedPostType(input.Type) {
		return nil, contract.ErrInvalidPostType
	}

	post, err := s.posts.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if post == nil {
		return nil, contract.ErrNotFound
	}
	categoryID := normalizePostCategoryID(input.CategoryID)
	if err := s.validateCategoryAssignment(ctx, input.Type, categoryID, post.CategoryID); err != nil {
		return nil, err
	}

	count, err := s.posts.CountBySlug(ctx, input.Slug, &id)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, contract.ErrSlugExists
	}

	post.Slug = input.Slug
	post.Type = input.Type
	post.TitleJSON = jsonmap.JSON(input.TitleJSON)
	post.SummaryJSON = jsonmap.JSON(input.SummaryJSON)
	post.ContentJSON = jsonmap.JSON(input.ContentJSON)
	post.Thumbnail = input.Thumbnail
	post.CategoryID = categoryID
	if input.IsPublished != nil {
		wasPublished := post.IsPublished
		post.IsPublished = *input.IsPublished
		if *input.IsPublished && !wasPublished && post.PublishedAt == nil {
			now := s.clock.Now()
			post.PublishedAt = &now
		}
	}

	if err := s.posts.WithinPostWriteTransaction(ctx, func(posts contract.PostStore, relations contract.PostProductRelationStore) error {
		if err := posts.Update(ctx, post); err != nil {
			return err
		}
		if input.ProductIDs == nil {
			return nil
		}
		return relations.SetRelatedProductIDs(ctx, post.ID, *input.ProductIDs)
	}); err != nil {
		return nil, err
	}
	return post, nil
}

// GetRelatedProductIDs 获取文章关联商品 ID 列表。
func (s *PostService) GetRelatedProductIDs(ctx context.Context, postID uint) ([]uint, error) {
	return s.relations.GetRelatedProductIDs(ctx, postID)
}

// ListRelatedProducts 获取文章关联商品列表。
func (s *PostService) ListRelatedProducts(ctx context.Context, postID uint) ([]contract.RelatedProduct, error) {
	return s.relations.ListRelatedProducts(ctx, postID)
}

// ListPostsForProduct 获取与商品关联的已发布博客列表。
func (s *PostService) ListPostsForProduct(ctx context.Context, productID uint, limit int) ([]contract.RelatedPost, error) {
	return s.relations.ListPostsForProduct(ctx, productID, constants.PostTypeBlog, true, limit)
}

// Delete 删除文章。
func (s *PostService) Delete(ctx context.Context, id string) error {
	post, err := s.posts.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if post == nil {
		return contract.ErrNotFound
	}
	return s.posts.Delete(ctx, id)
}

func isAllowedPostType(postType string) bool {
	return postType == constants.PostTypeBlog || postType == constants.PostTypeNotice
}

func (s *PostService) validateCategoryAssignment(ctx context.Context, postType string, categoryID, currentCategoryID *uint) error {
	if postType == constants.PostTypeNotice {
		if categoryID != nil && *categoryID > 0 {
			return contract.ErrPostNoticeCategoryUnsupported
		}
		return nil
	}
	if categoryID == nil || *categoryID == 0 {
		return nil
	}
	if s.categories == nil {
		return contract.ErrPostCategoryInvalid
	}

	category, err := s.categories.GetByID(ctx, *categoryID)
	if err != nil {
		return err
	}
	if category == nil {
		return contract.ErrPostCategoryInvalid
	}
	if !category.IsActive && !sameOptionalUint(currentCategoryID, categoryID) {
		return contract.ErrPostCategoryInvalid
	}

	childCount, err := s.categories.CountChildren(ctx, *categoryID)
	if err != nil {
		return err
	}
	if childCount > 0 && !sameOptionalUint(currentCategoryID, categoryID) {
		return contract.ErrPostCategoryInvalid
	}
	return nil
}

func normalizePostCategoryID(categoryID *uint) *uint {
	if categoryID != nil && *categoryID == 0 {
		return nil
	}
	return categoryID
}

func sameOptionalUint(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
