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

- 删除上游标记为 Deprecated 的 API，调用方改用其等价替代：`click.CheckPoint`、`slide.CheckPoint`、`rotate.CheckAngle` → 各包的 `Validate`；`click.Builder.MakeWithShape` → `MakeShape`；`slide.Builder.MakeWithRegion` → `MakeDragDrop`（上游注释误写为 `MakeDrag`）；`slide.Block` 的 `TileX/TileY` → `DX/DY`；`rotate.Block` 的 `ParentWidth/ParentHeight` 字段删除
- `base/random`：池化生成器由“时间 + math/rand”播种改为以 crypto/rand 为种子的 math/rand/v2 ChaCha8，答案相关的位置、角度与排列不再可从已解题目反推；删除未使用且同样以时间播种的全局生成器 `rnd/getRnd`
- `click.Validate`：修正容差偏移。上游以 `max(dx, dx-padding)` 计算左上角，结果恒为 `dx`，有效区间变成 `[dx, dx+w+2p]`；现改为四边各放宽 `padding`，即 `[dx-p, dx+w+p]`（纵向同理）
- 清理 staticcheck 告警：导出的错误变量按 Go 惯例由 `XxxErr` 改名为 `ErrXxx`（如 `ImageTypeErr` → `ErrImageType`）；删除未使用的 `click.wrapError` 与 click/rotate/slide 中从未读写的 `version` 字段
- 删除本项目不使用的工具函数：`canvas.MakeAreaRect/MakePositionRect`（对应类型保留）、`codec.DecodeByteToJpeg/DecodeByteToPng`、`helper.RgbToHex/HexToRgb/PathExists` 及仅供 `RgbToHex` 使用的 `t2x`
