# 复刻页浏览器回归

依赖：Python 3 的 python-docx、python-pptx、Pillow；LibreOffice；Node.js、Playwright、Chrome。
不需要往站点根目录安装 npm 依赖。

```bash
python3 tests/generate_fuke_fixtures.py
libreoffice -env:UserInstallation=file:///tmp/fuke-lo --headless --convert-to doc --outdir /tmp/fuke-fixtures /tmp/fuke-fixtures/sample.docx
libreoffice -env:UserInstallation=file:///tmp/fuke-lo --headless --convert-to ppt --outdir /tmp/fuke-fixtures /tmp/fuke-fixtures/sample.pptx
npm install --prefix /tmp/fuke-deps playwright
python3 -m http.server 8765 --bind 127.0.0.1
# 另一终端：
NODE_PATH=/tmp/fuke-deps/node_modules node tests/fuke.playwright.cjs
```

可通过 CHROME_PATH 指定 Chrome。测试在独立浏览器上下文写入一条复刻种子数据，
所有附件经页面 file input 上传，不直接向 IndexedDB 注入附件。
桌面 1440×1000、移动 390×844；截图位于 /tmp/fuke-{desktop,mobile}-*.png。
验证真实 Office 内容、三页 PPTX 翻页、内嵌图片、Markdown 清洗、七种工具栏、
Ctrl+B/I、原位置自动保存及刷新、状态圆点、下载删除、10MB 边界和禁止类型。
还检查首屏不加载 vendor、第三方库只从本站加载、无 pageerror、页面无横向溢出。
