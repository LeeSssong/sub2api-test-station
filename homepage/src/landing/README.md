# 星桥AI Link 首页 v24

`homepage/index.html` 是 Vite 正式首页入口，引用本目录的 `app.js`、`styles.css` 和品牌资源。旧 React 页面源码保留，当前首页不再加载其入口。

本版保留已确认的五模型左侧纵排、五条独立连接、星桥向模型发射、Logo 下方入口、品牌配色和文案。动效及样式直接复用已验证的 v24：

- app.js SHA256：`fb1ff9b3ee7125e02717df99ec6e40ef7d5e0584dd7790242b30bfe647ca9428`
- styles.css SHA256：`d51f1b17707a11ae67ba7cece8215150990cbe3e1bd81688540813b7639b5970`

测试站“开始使用”指向同站 `/login`，教程指向 `/docs/`。这只调整前端入口，不修改注册设置。图片、SVG mask 和脚本样式均由 Vite 处理，发布文件使用内容哈希。

验证：`npm run test:run -- src/landing/entry.test.ts`、`npm run build`。动效循环为7.2秒；减少动态、交互、暂停恢复验证沿用相同源文件的 v24 证据，集成后另核对构建产物及测试站公网结果。
