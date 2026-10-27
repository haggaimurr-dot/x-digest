// Classify original content, never model-generated summaries or steps.
export function canReplicatePost (post) {
  const text = String(post.text || post.title || '')
  const tutorial = /教程|手把手|保姆级|技术分享|实现原理|源码解析|配置清单|操作步骤|tutorial|step.by.step|how to|walkthrough/i
  const action = /安装|配置|部署|执行|运行|创建|接入|导入|npm |pip |git clone|docker /gi
  const technical = /代码|源码|脚本|接口|API|SDK|MCP|SQL|Python|JavaScript|TypeScript|Node\.?js|Docker|ffmpeg|算法|架构|数据库|开源/i
  if (text.length < 140 && /感谢|谢谢|好看|太棒|惊艳|amazing|thank/i.test(text) && (text.match(action) || []).length < 2 && !/原理|源码|代码示例|实现细节/.test(text)) return false
  return tutorial.test(text) || (technical.test(text) && (text.match(action) || []).length >= 2) ||
    (technical.test(text) && /实现|原理|示例|用法|设计|性能优化|implementation|architecture/i.test(text))
}
export function rawAvatar (post) {
  if (!post || typeof post !== 'object') return ''
  for (const key of ['author_avatar', 'authorAvatar', 'avatar', 'avatar_url', 'profile_image', 'profile_image_url_https', 'profile_image_url', 'avatarRemote']) {
    if (typeof post[key] === 'string' && post[key].trim()) return post[key].trim()
  }
  for (const key of ['author', 'user', 'legacy']) {
    const value = rawAvatar(post[key])
    if (value) return value
  }
  return ''
}
