# gocaptcha

本目录是验证码图形引擎，源自 [wenlng/go-captcha](https://github.com/wenlng/go-captcha)，现由本项目自行维护。

- 上游版本：v2.0.5（commit `de8cdfb00538c3cb1d04f7da5bcfc76faadd900e`，2026-03-07）
- 许可证：Apache License 2.0，原文见同目录 `LICENSE`；原作者署名保留在各文件头部
- 引入原因：上游自 v2.0.5 后不再更新，而出图清晰度、抗识别处理与校验规则都需要修改引擎内部

## 与上游的差异

引入时：

- 只引入 `base/`、`click/`、`rotate/`、`slide/` 四部分
- 未引入根目录的 `capts.go`（本项目不使用其中的便捷构造函数）
- 未引入 `tests/`：这些用例依赖上游仓库未提交的 `.cache/` 字体与图片，无法运行。生成流程由 `backend/tests` 中各题型的端到端测试覆盖
- 导入路径由 `github.com/wenlng/go-captcha/v2/...` 改为 `simple-captcha/internal/gocaptcha/...`，并在每个文件头部加注来源声明

此后的修改逐条记录在下方，便于日后与上游对照。

## 修改记录
