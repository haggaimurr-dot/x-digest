import type { Repo } from '~/composables/types'

// 仓库列表排序（2026-10-29）：星标、入库时间、发布时间（最新 Release）各有
// 升序 / 降序，另保留原来的「最近更新」（push 时间，降序）。没有该字段的
// 仓库（如还没有 Release）不论升降序都排在最后；相同时按 full_name 排，顺序稳定。
export const REPO_SORTS = ['stars_desc', 'stars_asc', 'created_desc', 'created_asc', 'released_desc', 'released_asc', 'updated'] as const
export type RepoSort = typeof REPO_SORTS[number]

function timeOf (s?: string | null): number | null {
  if (!s) return null
  // 服务端返回上海时间 "2006-01-02 15:04:05"（无时区）；按 +08:00 解析。
  const t = Date.parse(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(s) ? s.replace(' ', 'T') + '+08:00' : s)
  return Number.isNaN(t) ? null : t
}

function valueOf (r: Repo, key: RepoSort): number | null {
  switch (key) {
    case 'stars_desc': case 'stars_asc': return r.stars ?? null
    case 'created_desc': case 'created_asc': return timeOf(r.created_at)
    case 'released_desc': case 'released_asc': return timeOf(r.released_at)
    default: return timeOf(r.pushed_at || r.updated_at)
  }
}

export function sortRepos (list: Repo[], key: RepoSort): Repo[] {
  const asc = key.endsWith('_asc')
  return [...list].sort((a, b) => {
    const va = valueOf(a, key)
    const vb = valueOf(b, key)
    if (va === null || vb === null) {
      if (va !== vb) return va === null ? 1 : -1
    } else if (va !== vb) {
      return asc ? va - vb : vb - va
    }
    return a.full_name.localeCompare(b.full_name)
  })
}

export function isRepoSort (v: unknown): v is RepoSort {
  return typeof v === 'string' && (REPO_SORTS as readonly string[]).includes(v)
}

// GitHub 账号头像（无需 API 调用）；加载失败时页面显示首字母。
export function ownerAvatarUrl (owner: string): string {
  return `https://github.com/${encodeURIComponent(owner)}.png?size=64`
}
