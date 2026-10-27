import { test } from 'node:test'
import assert from 'node:assert/strict'
import { canReplicatePost, rawAvatar } from './x-post.mjs'
test('praise and thanks remain ineligible despite stale model metadata', () => {
  for (const text of ['卧槽，做的好看啊！！怎么这么懂我！！', '@johnzhou123 @EianLu 好看！！感谢🙏', 'Claude 效果太棒了，感谢指导！', '感谢分享教程，太棒了！']) {
    assert.equal(canReplicatePost({ text, title: 'Claude 安装教程', show_flowchart: true, steps: [{ text: '安装' }, { text: '运行' }] }), false)
  }
})
test('tutorials and technical sharing qualify', () => {
  for (const text of ['手把手部署 Claude Code 教程', 'Python 脚本示例：通过 API 实现批量处理', '先安装 Docker，然后配置 API 接口']) assert.equal(canReplicatePost({ text }), true)
})
test('API avatar shapes survive normalization', () => {
  assert.equal(rawAvatar({ author: { profile_image_url_https: 'https://example.com/avatar.jpg' } }), 'https://example.com/avatar.jpg')
  assert.equal(rawAvatar({ author: 'miles_mazy' }), '')
})
