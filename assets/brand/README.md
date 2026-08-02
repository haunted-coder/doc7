# doc7 Brand Assets

此目录保存 doc7 已批准、可长期复用的品牌参考资产。`dist/` 和仓库外审图目录只保存生成过程，不是品牌来源。

## Canonical References

| 文件 | 用途 | 约束 |
| --- | --- | --- |
| `references/open-source-pop.webp` | 唯一批准的品牌母版；所有品牌图和 README 宣传图必须使用 | 保持完整 `doc7` 字标、蓝色 `doc`、红色 `7`、白色编辑场和开源开发者工具气质 |

## Visual System

- 主字标：小写 `doc7`，`doc` 为 cobalt blue，`7` 为 signal red。
- 辅助色：terminal green、black ink、white editorial field。
- 品牌气质：开放、直接、现代、开发者友好，避免企业软件模板感和过度科幻效果。
- 字标必须完整清晰，尤其不能裁掉 `d` 左侧、`7` 下部或字符间的关键结构。

## Generation Rules

1. 使用 Lovart 或其他图像模型时，必须通过真实参考图参数传入 `references/open-source-pop.webp`，不能只在提示词中描述品牌。
2. 未经用户明确批准，不得把 Logo 探索稿、终端字形探索稿或其他生成图加入 canonical 品牌目录或作为参考图。
3. 不得从旧图裁取 Logo 后粘贴到新图，不得用本地脚本重绘或修补字标。
4. 除论文 Showcase 这类真实输入与真实 Markdown 对照图外，README 宣传图必须由模型直接生成完整画面。
5. 生成结果统一保存到项目内 `.review/pending/`，该目录被 Git 忽略。用户明确批准后，才允许转换为 WebP 并写入 `assets/readme/`。
6. 每轮生成记录模型、质量、尺寸、提示词、参考图、时间和用途；失败稿不得进入项目。

## Approved Source

- `references/open-source-pop.webp`
  - SHA-256：`fc0833fa88fc54e85aa2d4cd6e377e0deedbd4f8c8554b394303750ab5f6be10`

这些哈希用于确认 canonical 文件没有被生成流程意外覆盖。品牌方案变更必须由用户明确批准，并同步更新本文件。
