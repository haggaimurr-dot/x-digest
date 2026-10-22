# EasyMDE 2.20.0（v47）

来源：https://github.com/Ionaru/easy-markdown-editor

固定版本文件下载自 https://cdn.jsdelivr.net/npm/easymde@2.20.0/ （dist/easymde.min.js、dist/easymde.min.css、LICENSE）。运行时仅使用本地文件，不访问 CDN。MIT LICENSE 及打包依赖的许可文本已保留在本目录。

| 文件 | 原始字节 | gzip 字节（本地计算，非服务器传输保证） | SHA-256 |
| --- | ---: | ---: | --- |
| easymde.min.js | 326778 | 106488 | d684d83b1a2ed08b664e6e140984dbb2c26d7e6198a6537fcae3355e85cd47ba |
| easymde.min.css | 12923 | 3143 | e7a7feebd8e4a193c03f87dea9f782d40c0e50baa8b8b93cc552f92aa51a61c0 |

首次聚焦备注或打开格式菜单时加载，页面共享一份 JS/CSS；每个实际使用的备注创建独立实例。禁用拼写检查、字体图标下载与库自己的 autosave，沿用站点 note 字段和保存提示。格式命令和 Ctrl+B/I 由 EasyMDE 提供；编辑/预览切换沿用站点 marked + DOMPurify，禁用 EasyMDE 自带预览快捷键，避免绕过清洗。

## 选型比较

四款均为 MIT。以下是针对当前纯静态、多步骤短备注页面的取舍，未选库未做同条件打包或真机验证，不能把不同项目宣称的体积当成严格横向基准。

| 库 / 官方来源 | 体积与接入 | 手机适用性 | 本次取舍 |
| --- | --- | --- | --- |
| [EasyMDE](https://github.com/Ionaru/easy-markdown-editor) | 固定 2.20.0，JS+CSS 339701 字节，gzip 合计约 107 KiB；直接 script 接入 | CodeMirror 5 支持触屏输入；本次 390px 触屏模拟通过，未覆盖实体手机输入法 | 命令、选区、快捷键和撤销成熟，Markdown 源文直接兼容，选用 |
| [Toast UI Editor](https://github.com/nhn/tui.editor) | 完整 Markdown/WYSIWYG 双模式、预览及 UI，通常比本场景所需功能更多 | 可调整编辑模式和工具栏布局，窄屏需要配置 | 短备注不需要完整双模式 UI |
| [Milkdown](https://github.com/Milkdown/milkdown) | ProseMirror、remark 及插件体系，体积取决于插件，需要构建 | 可定制触屏 UI，仍需验证插件/输入法组合 | 当前无构建静态站接入成本较高 |
| [OverType](https://github.com/panphora/overtype) | 官方宣称约 134 KB minified（非本次实测）；textarea 叠加渲染、框架无关 | 原生 textarea 路线适合移动输入，仍需实测透明叠层选区与输入法 | 更轻，但本次优先 EasyMDE 成熟命令接口；未引入第二套编辑预览渲染 |

许可证来源：各官方仓库的 LICENSE。EasyMDE 打包包含 CodeMirror、marked、codemirror-spell-checker 与 Typo.js；对应许可保存在 third-party-licenses（Typo.js 为 BSD）。
