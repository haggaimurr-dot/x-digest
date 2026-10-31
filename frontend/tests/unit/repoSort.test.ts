import { describe, expect, it } from 'vitest'
import { isRepoSort, ownerAvatarUrl, sortRepos } from '~/utils/repoSort'
import type { Repo } from '~/composables/types'

function repo (full_name: string, o: Partial<Repo>): Repo {
  return { full_name, owner: full_name.split('/')[0], name: full_name.split('/')[1], stars: 0, forks: 0, html_url: '', sync_status: 'synced', ...o }
}
const list = [
  repo('a/one', { stars: 10, created_at: '2026-10-01 08:00:00', released_at: '2026-09-01 00:00:00' }),
  repo('a/two', { stars: 300, created_at: '2026-10-20 08:00:00', released_at: null }),
  repo('a/three', { stars: 10, created_at: '2026-10-10 08:00:00', released_at: '2026-10-25 00:00:00' })
]
const names = (key: Parameters<typeof sortRepos>[1]) => sortRepos(list, key).map(r => r.name)

describe('sortRepos', () => {
  it('stars both ways, ties by full name', () => {
    expect(names('stars_desc')).toEqual(['two', 'one', 'three'])
    expect(names('stars_asc')).toEqual(['one', 'three', 'two'])
  })
  it('入库时间 both ways', () => {
    expect(names('created_desc')).toEqual(['two', 'three', 'one'])
    expect(names('created_asc')).toEqual(['one', 'three', 'two'])
  })
  it('发布时间 both ways, repos without a release last', () => {
    expect(names('released_desc')).toEqual(['three', 'one', 'two'])
    expect(names('released_asc')).toEqual(['one', 'three', 'two'])
  })
  it('does not reorder the input', () => {
    sortRepos(list, 'stars_asc')
    expect(list.map(r => r.name)).toEqual(['one', 'two', 'three'])
  })
  it('validates stored sort keys and builds avatar URLs', () => {
    expect(isRepoSort('created_asc')).toBe(true)
    expect(isRepoSort('stars')).toBe(false)
    expect(ownerAvatarUrl('vuejs')).toBe('https://github.com/vuejs.png?size=64')
  })
})
