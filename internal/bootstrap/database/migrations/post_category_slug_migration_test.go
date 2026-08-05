package migrations

import (
	"testing"

	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	settingsstore "github.com/dujiao-next/internal/modules/settings/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"gorm.io/gorm"
)

func TestEnsurePostCategorySlugMigrationBackfillsLegacyPosts(t *testing.T) {
	db := setupSKUMigrationTestDB(t)

	if err := db.AutoMigrate(
		&contentdomain.PostCategory{},
		&contentdomain.Post{},
		&settingsstore.SettingRecord{},
	); err != nil {
		t.Fatalf("auto migrate post tables failed: %v", err)
	}

	category := contentdomain.PostCategory{
		Slug:     "docs",
		NameJSON: jsonmap.JSON{"zh-CN": "docs"},
		IsActive: true,
	}
	if err := db.Create(&category).Error; err != nil {
		t.Fatalf("create post category failed: %v", err)
	}

	legacy := contentdomain.Post{
		Slug:       "legacy-post",
		TitleJSON:  jsonmap.JSON{"zh-CN": "legacy-post"},
		CategoryID: &category.ID,
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy post failed: %v", err)
	}
	filled := contentdomain.Post{
		Slug:         "filled-post",
		TitleJSON:    jsonmap.JSON{"zh-CN": "filled-post"},
		CategoryID:   &category.ID,
		CategorySlug: "already-filled",
	}
	if err := db.Create(&filled).Error; err != nil {
		t.Fatalf("create already-filled post failed: %v", err)
	}
	uncategorized := contentdomain.Post{
		Slug:      "uncategorized-post",
		TitleJSON: jsonmap.JSON{"zh-CN": "uncategorized-post"},
	}
	if err := db.Create(&uncategorized).Error; err != nil {
		t.Fatalf("create uncategorized post failed: %v", err)
	}
	orphanCategoryID := uint(9999)
	orphan := contentdomain.Post{
		Slug:       "orphan-post",
		TitleJSON:  jsonmap.JSON{"zh-CN": "orphan-post"},
		CategoryID: &orphanCategoryID,
	}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("create orphan post failed: %v", err)
	}

	if err := ensurePostCategorySlugMigration(); err != nil {
		t.Fatalf("ensure post category slug migration failed: %v", err)
	}

	assertPostCategorySlug(t, db, "legacy-post", "docs")
	assertPostCategorySlug(t, db, "filled-post", "already-filled")
	assertPostCategorySlug(t, db, "uncategorized-post", "")
	assertPostCategorySlug(t, db, "orphan-post", "")

	if err := ensurePostCategorySlugMigration(); err != nil {
		t.Fatalf("second run should be idempotent: %v", err)
	}
}

func assertPostCategorySlug(t *testing.T, db *gorm.DB, slug, wantSlug string) {
	t.Helper()
	var post contentdomain.Post
	if err := db.Where("slug = ?", slug).First(&post).Error; err != nil {
		t.Fatalf("query post %q: %v", slug, err)
	}
	if post.CategorySlug != wantSlug {
		t.Fatalf("post %q category_slug want %q got %q", slug, wantSlug, post.CategorySlug)
	}
}
