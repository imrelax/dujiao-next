package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	contentapp "github.com/dujiao-next/internal/modules/content/application"
	contentdomain "github.com/dujiao-next/internal/modules/content/domain"
	localfilestore "github.com/dujiao-next/internal/modules/content/infrastructure/filestore/local"
	"github.com/dujiao-next/internal/modules/content/infrastructure/gormstore"
	contenttransport "github.com/dujiao-next/internal/modules/content/transport/http"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAdminContentHandlerTest(t *testing.T) (*contenttransport.AdminHandler, *contentapp.PostService, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:admin_content_handler_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&contentdomain.PostCategory{},
		&contentdomain.Post{},
		&contentdomain.PostProduct{},
		&contentdomain.Banner{},
		&contentdomain.Media{},
	); err != nil {
		t.Fatalf("auto migrate admin content tables: %v", err)
	}

	postStore := gormstore.NewPostStore(db)
	postService := contentapp.NewPostService(
		postStore,
		postStore,
		gormstore.NewPostCategoryStore(db),
		contentapp.SystemClock{},
	)
	handler := contenttransport.NewAdminHandler(
		postService,
		contentapp.NewPostCategoryService(gormstore.NewPostCategoryStore(db)),
		contentapp.NewBannerService(gormstore.NewBannerStore(db), contentapp.SystemClock{}),
		contentapp.NewMediaService(gormstore.NewMediaStore(db), localfilestore.New(), nil),
	)
	return handler, postService, db
}

func adminContentTestRouter(handler *contenttransport.AdminHandler) *gin.Engine {
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	contenttransport.RegisterAdminRoutes(admin, handler)
	return router
}

func requestAdminContent(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 for %s %s, got %d body=%s", method, path, recorder.Code, recorder.Body.String())
	}
	return recorder
}

func decodeAdminContentResponse(t *testing.T, recorder *httptest.ResponseRecorder) response.Response {
	t.Helper()
	var got response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode admin content response: %v body=%s", err, recorder.Body.String())
	}
	return got
}

func TestAdminContentHandlersSuccessContracts(t *testing.T) {
	handler, _, db := setupAdminContentHandlerTest(t)
	router := adminContentTestRouter(handler)

	postRecorder := requestAdminContent(t, router, http.MethodPost, "/api/v1/admin/posts", `{
		"slug":"admin-post",
		"type":"blog",
		"title":{"zh-CN":"Admin post"},
		"is_published":true
	}`)
	if got := decodeAdminContentResponse(t, postRecorder); got.StatusCode != response.CodeOK {
		t.Fatalf("create post should succeed: %#v", got)
	}
	listRecorder := requestAdminContent(t, router, http.MethodGet, "/api/v1/admin/posts?type=blog", "")
	var postList struct {
		StatusCode int `json:"status_code"`
		Data       []struct {
			Slug string `json:"slug"`
		} `json:"data"`
		Pagination response.Pagination `json:"pagination"`
	}
	if err := json.Unmarshal(listRecorder.Body.Bytes(), &postList); err != nil {
		t.Fatalf("decode admin post list: %v", err)
	}
	if postList.StatusCode != response.CodeOK || len(postList.Data) != 1 || postList.Data[0].Slug != "admin-post" || postList.Pagination.Total != 1 {
		t.Fatalf("admin post list mismatch: %#v", postList)
	}

	categoryRecorder := requestAdminContent(t, router, http.MethodPost, "/api/v1/admin/post-categories", `{
		"name":{"zh-CN":"Docs"},
		"slug":"docs"
	}`)
	var categoryCreated struct {
		StatusCode int `json:"status_code"`
		Data       struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(categoryRecorder.Body.Bytes(), &categoryCreated); err != nil {
		t.Fatalf("decode created category: %v", err)
	}
	if categoryCreated.StatusCode != response.CodeOK || categoryCreated.Data.ID == 0 {
		t.Fatalf("create category should return ID: %#v", categoryCreated)
	}
	statusPath := fmt.Sprintf("/api/v1/admin/post-categories/%d/status", categoryCreated.Data.ID)
	if got := decodeAdminContentResponse(t, requestAdminContent(t, router, http.MethodPatch, statusPath, `{"is_active":false}`)); got.StatusCode != response.CodeOK {
		t.Fatalf("disable category should succeed: %#v", got)
	}
	var category contentdomain.PostCategory
	if err := db.First(&category, categoryCreated.Data.ID).Error; err != nil {
		t.Fatalf("reload disabled category: %v", err)
	}
	if category.IsActive {
		t.Fatalf("category status patch should persist false: %#v", category)
	}

	bannerRecorder := requestAdminContent(t, router, http.MethodPost, "/api/v1/admin/banners", `{
		"name":"Home",
		"image":"/uploads/home.png",
		"link_type":"none",
		"is_active":false
	}`)
	if got := decodeAdminContentResponse(t, bannerRecorder); got.StatusCode != response.CodeOK {
		t.Fatalf("create banner should succeed: %#v", got)
	}
	var inactiveBanner contentdomain.Banner
	if err := db.Where("name = ?", "Home").First(&inactiveBanner).Error; err != nil {
		t.Fatalf("reload inactive banner: %v", err)
	}
	if inactiveBanner.IsActive {
		t.Fatalf("admin banner create should persist explicit is_active=false: %#v", inactiveBanner)
	}

	media := contentdomain.Media{
		Name:     "before",
		Filename: "asset.png",
		Path:     "/uploads/common/asset.png",
		MimeType: "image/png",
		Size:     5,
		Scene:    "common",
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	mediaPath := fmt.Sprintf("/api/v1/admin/media/%d", media.ID)
	if got := decodeAdminContentResponse(t, requestAdminContent(t, router, http.MethodPut, mediaPath, `{"name":"after"}`)); got.StatusCode != response.CodeOK {
		t.Fatalf("rename media should succeed: %#v", got)
	}
	if err := db.First(&media, media.ID).Error; err != nil {
		t.Fatalf("reload media: %v", err)
	}
	if media.Name != "after" {
		t.Fatalf("media rename mismatch: %#v", media)
	}
}

func TestAdminContentHandlersValidationAndDomainErrorContracts(t *testing.T) {
	handler, posts, _ := setupAdminContentHandlerTest(t)
	router := adminContentTestRouter(handler)

	if _, err := posts.Create(context.Background(), contentapp.CreatePostInput{
		Slug:      "duplicate",
		Type:      "blog",
		TitleJSON: map[string]interface{}{"zh-CN": "duplicate"},
	}); err != nil {
		t.Fatalf("seed duplicate post: %v", err)
	}

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{
			name:     "post bind validation",
			method:   http.MethodPost,
			path:     "/api/v1/admin/posts",
			body:     `{"slug":"missing-title","type":"blog"}`,
			wantCode: response.CodeBadRequest,
		},
		{
			name:     "duplicate post slug",
			method:   http.MethodPost,
			path:     "/api/v1/admin/posts",
			body:     `{"slug":"duplicate","type":"blog","title":{"zh-CN":"duplicate"}}`,
			wantCode: response.CodeBadRequest,
		},
		{
			name:     "missing post update",
			method:   http.MethodPut,
			path:     "/api/v1/admin/posts/9999",
			body:     `{"slug":"missing","type":"blog","title":{"zh-CN":"missing"}}`,
			wantCode: response.CodeNotFound,
		},
		{
			name:     "invalid category parent",
			method:   http.MethodPost,
			path:     "/api/v1/admin/post-categories",
			body:     `{"name":{"zh-CN":"invalid"},"slug":"invalid","parent_id":9999}`,
			wantCode: response.CodeBadRequest,
		},
		{
			name:     "banner invalid time window",
			method:   http.MethodPost,
			path:     "/api/v1/admin/banners",
			body:     `{"name":"invalid","image":"/invalid.png","start_at":"2026-07-21T00:00:00Z","end_at":"2026-07-20T00:00:00Z"}`,
			wantCode: response.CodeBadRequest,
		},
		{
			name:     "missing banner",
			method:   http.MethodGet,
			path:     "/api/v1/admin/banners/9999",
			wantCode: response.CodeNotFound,
		},
		{
			name:     "invalid media ID",
			method:   http.MethodDelete,
			path:     "/api/v1/admin/media/not-a-number",
			wantCode: response.CodeBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := requestAdminContent(t, router, test.method, test.path, test.body)
			got := decodeAdminContentResponse(t, recorder)
			if got.StatusCode != test.wantCode {
				t.Fatalf("business status want %d got %d body=%s", test.wantCode, got.StatusCode, recorder.Body.String())
			}
		})
	}
}

// 后台文章列表的 category_id 查询参数：等值匹配（与商品后台一致，不展开子分类），
// 并与 type 按 AND 组合。
func TestAdminContentPostsListFilterByCategoryID(t *testing.T) {
	handler, _, db := setupAdminContentHandlerTest(t)
	router := adminContentTestRouter(handler)

	root := contentdomain.PostCategory{Slug: "admin-filter-root", NameJSON: jsonmap.JSON{"zh-CN": "后台筛选根"}, IsActive: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root category failed: %v", err)
	}
	rootID := root.ID
	child := contentdomain.PostCategory{Slug: "admin-filter-child", NameJSON: jsonmap.JSON{"zh-CN": "后台筛选子"}, ParentID: &rootID, IsActive: true}
	if err := db.Create(&child).Error; err != nil {
		t.Fatalf("create child category failed: %v", err)
	}
	childID := child.ID

	fixtures := []contentdomain.Post{
		{Slug: "admin-filter-post-child", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "后台子分类"}, CategoryID: &childID},
		{Slug: "admin-filter-post-draft", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "后台子分类草稿"}, CategoryID: &childID},
		{Slug: "admin-filter-post-notice", Type: constants.PostTypeNotice, TitleJSON: jsonmap.JSON{"zh-CN": "后台子分类公告"}, CategoryID: &childID},
		{Slug: "admin-filter-post-none", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "后台未分类"}},
	}
	for i := range fixtures {
		if err := db.Create(&fixtures[i]).Error; err != nil {
			t.Fatalf("create post fixture %q failed: %v", fixtures[i].Slug, err)
		}
	}

	slugs := func(path string) map[string]bool {
		t.Helper()
		recorder := requestAdminContent(t, router, http.MethodGet, path, "")
		payload := decodeAdminContentResponse(t, recorder)

		raw, err := json.Marshal(payload.Data)
		if err != nil {
			t.Fatalf("re-marshal %s failed: %v", path, err)
		}
		var rows []contentdomain.Post
		if err := json.Unmarshal(raw, &rows); err != nil {
			t.Fatalf("decode %s failed: %v body=%s", path, err, recorder.Body.String())
		}

		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.Slug] = true
		}
		return got
	}

	got := slugs(fmt.Sprintf("/api/v1/admin/posts?category_id=%d", childID))
	if !got["admin-filter-post-child"] || !got["admin-filter-post-draft"] || !got["admin-filter-post-notice"] || got["admin-filter-post-none"] || len(got) != 3 {
		t.Fatalf("admin category filter mismatch: %+v", got)
	}

	got = slugs(fmt.Sprintf("/api/v1/admin/posts?category_id=%d&type=%s", childID, constants.PostTypeBlog))
	if !got["admin-filter-post-child"] || !got["admin-filter-post-draft"] || len(got) != 2 {
		t.Fatalf("admin category + type mismatch: %+v", got)
	}

	got = slugs("/api/v1/admin/posts")
	if len(got) != 4 {
		t.Fatalf("admin without category filter should return all posts: %+v", got)
	}
}

// 后台文章列表的 is_published 查询参数：三态筛选，并与 type 按 AND 组合。
func TestAdminContentPostsListFilterByPublishedState(t *testing.T) {
	handler, _, db := setupAdminContentHandlerTest(t)
	router := adminContentTestRouter(handler)

	fixtures := []contentdomain.Post{
		{Slug: "admin-state-published", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "后台已发布"}, IsPublished: true},
		{Slug: "admin-state-draft", Type: constants.PostTypeBlog, TitleJSON: jsonmap.JSON{"zh-CN": "后台草稿"}, IsPublished: false},
		{Slug: "admin-state-notice-draft", Type: constants.PostTypeNotice, TitleJSON: jsonmap.JSON{"zh-CN": "后台公告草稿"}, IsPublished: false},
	}
	for i := range fixtures {
		if err := db.Create(&fixtures[i]).Error; err != nil {
			t.Fatalf("create post fixture %q failed: %v", fixtures[i].Slug, err)
		}
	}

	slugs := func(path string) map[string]bool {
		t.Helper()
		recorder := requestAdminContent(t, router, http.MethodGet, path, "")
		payload := decodeAdminContentResponse(t, recorder)

		raw, err := json.Marshal(payload.Data)
		if err != nil {
			t.Fatalf("re-marshal %s failed: %v", path, err)
		}
		var rows []contentdomain.Post
		if err := json.Unmarshal(raw, &rows); err != nil {
			t.Fatalf("decode %s failed: %v body=%s", path, err, recorder.Body.String())
		}

		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.Slug] = true
		}
		return got
	}

	got := slugs("/api/v1/admin/posts?is_published=1")
	if !got["admin-state-published"] || got["admin-state-draft"] || got["admin-state-notice-draft"] || len(got) != 1 {
		t.Fatalf("is_published=1 should return only published rows: %+v", got)
	}

	got = slugs("/api/v1/admin/posts?is_published=0")
	if !got["admin-state-draft"] || !got["admin-state-notice-draft"] || got["admin-state-published"] || len(got) != 2 {
		t.Fatalf("is_published=0 should return only drafts: %+v", got)
	}

	got = slugs("/api/v1/admin/posts?is_published=all")
	if len(got) != 3 {
		t.Fatalf("is_published=all should return all rows: %+v", got)
	}

	got = slugs(fmt.Sprintf("/api/v1/admin/posts?is_published=0&type=%s", constants.PostTypeNotice))
	if !got["admin-state-notice-draft"] || len(got) != 1 {
		t.Fatalf("is_published=0 + type mismatch: %+v", got)
	}
}
