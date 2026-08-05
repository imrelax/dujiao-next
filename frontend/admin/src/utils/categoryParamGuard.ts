/**
 * 分类参数合规校验
 *
 * 文章列表类接口仅允许以 category_slug 作为分类标识进行查询，
 * 携带 categoryid / category_id 形式的参数将被判定为不合规调用并拒绝，
 * 从调用源头彻底禁用 categoryid 形式的接口调用方式。
 */

/** 文章列表类接口路径前缀（后台端） */
export const POST_LIST_PATH_PREFIXES = ['/public/posts', '/admin/posts']

export type CategoryParamGuardResult =
  | { ok: true }
  | { ok: false; reason: 'category_id_blocked' }
  | { ok: false; reason: 'category_slug_required' }

/**
 * 校验请求参数是否符合分类标识规范。
 * @param path   接口路径
 * @param params 请求查询参数
 */
export function checkCategoryParamCompliance(
  path: string,
  params?: Record<string, any>,
): CategoryParamGuardResult {
  if (!params) {
    return { ok: true }
  }
  const isPostListPath = POST_LIST_PATH_PREFIXES.some((prefix) => path.startsWith(prefix))
  if (!isPostListPath) {
    return { ok: true }
  }

  for (const key of Object.keys(params)) {
    const normalized = key.toLowerCase()
    if (normalized === 'categoryid' || normalized === 'category_id') {
      return { ok: false, reason: 'category_id_blocked' }
    }
  }

  if ('category_slug' in params) {
    const value = params.category_slug
    if (value === undefined || value === null || String(value).trim() === '') {
      return { ok: false, reason: 'category_slug_required' }
    }
  }

  return { ok: true }
}
