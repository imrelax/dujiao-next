import { describe, expect, it } from 'vitest'
import { checkCategoryParamCompliance } from './categoryParamGuard'

describe('checkCategoryParamCompliance', () => {
  it('文章列表接口允许 category_slug 参数', () => {
    expect(checkCategoryParamCompliance('/admin/posts', { category_slug: 'docs' })).toEqual({ ok: true })
  })

  it('拦截携带 categoryid 的文章列表请求', () => {
    expect(checkCategoryParamCompliance('/admin/posts', { categoryid: 1 })).toEqual({
      ok: false,
      reason: 'category_id_blocked',
    })
  })

  it('拦截携带 category_id 的文章列表请求', () => {
    expect(checkCategoryParamCompliance('/admin/posts', { category_id: 1 })).toEqual({
      ok: false,
      reason: 'category_id_blocked',
    })
  })

  it('拦截大小写变体的 id 形式参数', () => {
    expect(checkCategoryParamCompliance('/admin/posts', { CategoryId: 1 })).toEqual({
      ok: false,
      reason: 'category_id_blocked',
    })
    expect(checkCategoryParamCompliance('/admin/posts', { CATEGORY_ID: 2 })).toEqual({
      ok: false,
      reason: 'category_id_blocked',
    })
  })

  it('category_slug 为空时拒绝请求', () => {
    expect(checkCategoryParamCompliance('/admin/posts', { category_slug: '' })).toEqual({
      ok: false,
      reason: 'category_slug_required',
    })
    expect(checkCategoryParamCompliance('/admin/posts', { category_slug: '   ' })).toEqual({
      ok: false,
      reason: 'category_slug_required',
    })
  })

  it('非文章列表接口不受拦截规则影响', () => {
    expect(checkCategoryParamCompliance('/admin/products', { category_id: 1 })).toEqual({ ok: true })
    expect(checkCategoryParamCompliance('/public/categories', { category_slug: '' })).toEqual({ ok: true })
  })

  it('文章详情等路径不误伤无分类参数请求', () => {
    expect(checkCategoryParamCompliance('/admin/posts/123')).toEqual({ ok: true })
    expect(checkCategoryParamCompliance('/admin/posts', { page: 1, page_size: 20, type: 'blog' })).toEqual({
      ok: true,
    })
  })
})
