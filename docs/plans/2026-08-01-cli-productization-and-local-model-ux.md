# doc7 CLI 产品化与本地模型交互方案

## 目标

让普通用户下载 doc7 后，只需要把文档交给命令行，就能完成「识别输入、找到本地模型、转换 Markdown、提示结果」的完整流程。

核心体验：

```text
doc7 report.pdf
doc7 screenshot.png
doc7 report.docx
doc7 "把 report.pdf 转成适合知识库检索的 Markdown"
```

用户不需要理解 PDF 渲染、页面提取、VLM、OCR、模型端点或内部流水线命令。

## 已确认的问题

### 命令语义混乱

当前 `convert` 是「转换为中间 PDF」，不是「转换为 Markdown」。

- 图片传给 `convert` 会被拒绝。
- PDF 传给 `convert` 会提示已经是 PDF。
- 真正的通用 Markdown 流程在 `read`，视觉提取核心在 `extract`。

这些内部阶段命令不应成为普通用户的主要入口。

### 未配置时行为不透明

当前 CLI 会从以下来源合并配置：

1. 命令行参数
2. 环境变量
3. 当前目录或父目录的 `.doc7.yaml`
4. 用户配置目录中的 `config.yaml`
5. 本机凭据存储

因此「没有输入配置」不等于「没有配置」。CLI 必须显示实际生效的模型、端点位置和凭据来源，不能让用户猜测。

### 图片能力没有进入主入口

底层已经支持图片视觉提取，但用户使用了只负责中间 PDF 转换的 `convert` 命令，导致图片能力看起来像不存在。

## 语言策略

### 默认自动检测

用户不需要配置语言。启动时按以下优先级解析：

1. `--lang`
2. `DOC7_LANG`
3. doc7 配置中的 `language`
4. 系统语言
5. 英文兜底

支持的初始语言：

- `zh-CN`：中文系统使用简体中文
- `en`：英文系统和未识别语言使用英文

语言检测来源：

- macOS：系统首选语言，环境变量作为降级来源
- Windows：用户界面语言，环境变量作为降级来源
- Linux：`LC_ALL`、`LC_MESSAGES`、`LANG`

### 配置覆盖

添加统一配置入口：

```bash
doc7 config set language en
doc7 config set language zh-CN
doc7 config set language auto
```

保留 `doc7 setup config` 作为首次配置兼容入口，但新的帮助和文档统一使用 `doc7 config`。

语言配置只影响 CLI 的帮助、进度、错误、诊断和安装提示，不影响：

- `--json` 的字段名和错误码
- 文件路径、模型名称、URL
- Markdown 输出内容
- 发给视觉模型的文档理解提示词

### 翻译架构

建立集中式消息目录，不在业务代码中散落中英文字符串：

```text
internal/i18n/
  locale.go
  detect_darwin.go
  detect_windows.go
  detect_unix.go
  messages_en.go
  messages_zh_cn.go
```

错误使用稳定错误码和结构化参数，最终在 CLI 输出边界渲染当前语言。这样可以在不修改业务流程的情况下增加新语言。

## 万能入口

### 普通命令

新增根命令的路径参数入口：

```bash
doc7 <input>
```

支持：

- 图片
- PDF
- DOC、DOCX、ODT、RTF
- PPT、PPTX、ODP
- 表格
- Markdown、TXT、CSV、JSON 等原生文本
- 目录批处理
- ZIP 文档包
- HTTP(S) 文档地址
- 标准输入

处理逻辑由输入探测器决定：

1. 原生文本格式直接转 Markdown。
2. 图片直接进入视觉理解流程。
3. PDF 先渲染页面，再逐页调用视觉模型。
4. Office 先使用可用渲染器生成页面，再进入视觉理解流程。
5. 目录、ZIP 和 URL 进入批处理流程。

`doc7 read <input>` 保留为明确的同义入口。

### 高级命令整理

把内部阶段从普通帮助中移出，避免和万能入口竞争：

- `convert` 改名为 `to-pdf`，明确表示只做中间 PDF 转换。
- `extract` 标记为高级视觉提取命令。
- `render`、`merge`、`refine`、`export-pptx` 放入 Advanced Commands 分组。

对外文档、README 和安装后的 `doc7 --help` 只突出万能入口。

### 输出体验

默认输出继续保留页面图片、manifest 和缓存，保证可恢复和可审计；但 CLI 必须明确打印最终 Markdown 路径：

```text
✓ Conversion complete
Markdown: ./report-doc7/report.md
Artifacts: ./report-doc7
Model: local · qwen3.5-9b
```

后续可增加 `--stdout` 作为管道入口：

```bash
doc7 report.pdf --stdout > report.md
```

## 首次运行与本地模型发现

### 无配置时的处理顺序

当用户运行 `doc7 <input>` 且需要视觉模型时：

1. 读取合并后的配置。
2. 如果已有完整模型配置，显示脱敏后的生效信息并继续。
3. 如果没有完整配置，发现本机可用的 OpenAI 兼容模型服务。
4. 对候选端点调用 `/v1/models` 获取模型列表。
5. 对候选模型执行一次最小视觉探测，确认它接受图片输入。
6. 一个候选模型时进入确认并保存。
7. 多个候选模型时让用户选择。
8. 没有候选模型时给出启动 LM Studio 或执行 `doc7 setup` 的明确指引。

默认不自动调用未知的远程服务。

### 模型发现原则

- 只发现本机或用户明确配置的端点。
- 不从 GitHub、Release 或任何项目服务获取模型配置。
- 不把模型权重下载到 doc7。
- 不把 API Key、模型响应或文档内容上传到 GitHub。
- 模型名称从端点真实返回的 `/v1/models` 获取，不写死模型名。
- 如果端点需要 Key，先提示用户，再通过本机凭据存储保存。

### 首次配置示例

中文系统：

```text
尚未配置视觉模型。

正在查找本地模型服务……
✓ 找到 LM Studio
✓ 找到 2 个模型

请选择用于文档理解的模型：
  1. qwen3.5-9b
  2. qwen3.5-0.8b

请选择 [1-2]：
```

英文系统显示对应英文消息。选择结果写入用户本机配置，不进入仓库。

### 非交互环境

管道、CI 和脚本执行时不弹出选择框。没有配置时返回稳定错误码和可复制命令：

```text
No vision model is configured.
Run `doc7 setup` interactively, or provide `--base-url` and `--model`.
```

## 自然语言使用方式

### 入口设计

增加明确的自然语言命令，避免把普通文件路径和自然语言混在一起：

```bash
doc7 ask "把 report.pdf 转成适合知识库检索的 Markdown，保留表格、公式和图片描述"
```

自然语言请求可以包含：

- 输入文件路径
- 输出风格
- 是否保留表格
- 是否生成图片描述
- 是否面向知识库、论文、报告或网页
- 语言要求
- 是否保留原始结构

### 安全边界

自然语言只控制文档转换和 Markdown 输出，不允许模型：

- 执行任意 Shell 命令
- 读取输入文件之外的任意路径
- 修改系统文件
- 自动上传文档到第三方服务
- 修改 doc7 配置或凭据，除非用户明确执行配置命令

路径解析和输入检测由本地代码完成，模型只负责理解转换意图和处理文档内容。

### 模型调用方式

如果本地已有视觉模型，使用该模型完成：

1. 自然语言请求理解。
2. 文档页面视觉理解。
3. Markdown 结构化输出。

不额外下载第二个模型，不因为自然语言入口引入新的云端依赖。

## 配置与隐私

### 配置查看

```bash
doc7 config show
```

只显示：

- 当前语言来源和最终语言
- Provider
- 脱敏端点
- 模型名称
- 凭据来源类型
- 默认渲染器和并发配置

绝不显示 Key 内容。

### 凭据来源

优先使用平台原生安全存储：

- macOS Keychain
- Windows Credential Manager
- Linux Secret Service

环境变量只作为 CI 和明确配置场景的来源。普通配置文件只保存端点、模型和凭据引用，不保存明文 Key。

### 远程端点提示

如果生效端点不是本机地址，CLI 在首次任务开始前显示：

```text
Endpoint: remote · https://example.com/v1
Document content will be sent to this endpoint.
Continue? [y/N]
```

已明确配置并确认过的端点可以记录确认状态；`--yes` 只允许在用户明确指定时跳过交互。

## 帮助与错误设计

首页帮助只保留普通用户真正需要的内容：

```text
Usage:
  doc7 <file|directory|url>       Convert to Markdown
  doc7 ask "request"              Convert with natural-language instructions
  doc7 setup                      Configure a local or private model
  doc7 config                     View or change configuration
  doc7 doctor                     Check the local environment
```

每条配置错误都包含：

1. 发生了什么。
2. 为什么需要模型或依赖。
3. 下一步可以直接复制执行的命令。

示例：

```text
This PDF needs a vision model, but no model is configured.

Start LM Studio with a vision model, then run:
  doc7 setup
```

## 实施阶段

### 第一阶段：语言与消息体系

- 添加系统语言检测。
- 添加 `--lang`、`DOC7_LANG` 和配置项 `language`。
- 集中 CLI 消息。
- 保证 JSON 字段和错误码稳定。

### 第二阶段：万能入口

- 根命令支持直接接收输入路径。
- 图片进入主流程。
- 重新组织命令分组和帮助文本。
- 明确输出 Markdown、产物目录和模型信息。

### 第三阶段：首次配置与模型发现

- 添加本地端点发现。
- 获取真实模型列表。
- 执行最小视觉能力探测。
- 交互选择并保存本地配置。
- 增加 `config show` 和 `config reset`。

### 第四阶段：自然语言入口

- 添加 `doc7 ask`。
- 本地解析输入路径和请求边界。
- 使用已配置模型理解转换意图。
- 禁止自然语言触发系统命令和越权文件访问。

### 第五阶段：发布与文档

- 更新中英文 README。
- 以 `doc7 <file>` 作为首屏示例。
- 添加中文和英文终端示例。
- 删除普通用户文档中的 `convert`、`extract` 主流程示例。
- 发布新的 CLI 版本。

## 验收标准

- 中文系统默认显示中文，英文系统默认显示英文。
- `doc7 config set language ...` 可以覆盖系统语言。
- 没有模型配置时，不出现难以理解的底层错误，而是给出下一步命令。
- 本地 LM Studio 无 Key 时可以正常运行。
- 远程接口缺少 Key 时明确提示凭据问题。
- `doc7 image.png`、`doc7 file.pdf`、`doc7 file.docx` 都进入 Markdown 主流程。
- `doc7 ask` 可以用自然语言指定转换目标，但不能执行任意系统操作。
- Key、文档内容和模型响应不进入 GitHub、Release 或普通日志。
- `doc7 --help` 首屏不再把内部中间步骤当成普通用户入口。
# v0.3.0 CLI 体验优化补充

## 用户问题

当前 `config` 输出使用本地化标签，但没有同时显示稳定的英文配置 Key，用户无法把显示项对应到 `config set <key> <value>`。配置文件路径也没有展示，`config set` 缺少参数时只返回 Cobra 的参数错误。自然语言入口仍叫 `ask`，与用户输入一句话的直觉不一致；发行包也缺少可验证、可跨平台的 `update` 命令。

## 目标

- `doc7 config` 默认展示配置，不要求用户记住 `show`。
- 配置项同时显示本地化名称和稳定英文 Key。
- 显示实际读取和写入的配置文件路径。
- `config set` 支持帮助列表、单 Key 交互编辑和 Key/Value 直接编辑。
- 用 `doc7 chat` 替代 `doc7 ask`，支持一次性自然语言任务和交互式会话入口。
- 增加 `doc7 update`，从 GitHub Release 下载当前平台包、校验 SHA-256，并安全替换当前 CLI。
- README、Windows Portable、安装脚本和 Release 流程全部同步。

## 行为设计

```text
doc7 config
doc7 config set
doc7 config set model
doc7 config set model qwen3.5
doc7 config reset
doc7 chat "把 report.pdf 转成适合知识库的 Markdown"
doc7 update
```

配置显示保留机器可读 Key：`模型（model）`、`模型接口（base_url）`。凭据只显示来源，不显示秘密。`update` 使用公开仓库 `magicrew/doc7` 的最新正式 Release，不下载预发布版本；用户可以用 `--check` 只检查更新，用 `--yes` 跳过确认。

## 影响范围

- `internal/config`：提供有效配置路径发现和写入路径统一逻辑。
- `internal/cli`：重构 config 入口，新增 chat 和 update 命令。
- `internal/i18n`：补充中英文配置、更新和聊天文案。
- `README.md`、`README.zh-CN.md`、`packaging/*`、`scripts/*`：更新用户流程和升级说明。
- Release：发布 `v0.3.0`，保留跨平台 CLI、Windows Portable、Docker 多架构和 GHCR 发布链路。

## 验收标准

- `doc7 config` 能显示有效配置文件路径和中英文 Key。
- `doc7 config set` 不带参数时不再报参数数量错误。
- `doc7 config set model` 在终端进入值输入流程，非交互环境给出明确用法。
- `doc7 chat` 替代 `ask` 出现在帮助、README 和发行包说明中。
- `doc7 update --check` 能读取最新 Release 并报告版本状态。
- `doc7 update` 校验校验和后更新当前平台二进制。
- `v0.3.0` 的 Build、Release、Portable、Container 流水线全部成功。
