# Chat 只读工具与安全凭据配置实施方案

**状态：** 已完成
**目标：** 让 `doc7 chat` 能在用户授权范围内通过受限只读工具定位模糊文件，并由 Agent 引导完成模型配置和 API Key 安全录入，同时保证模型永远无法读取密钥内容。
**实现方式：** 使用结构化的只读命令工具替代专用文件搜索封装；使用独立的隐藏输入工具接收敏感信息；文件访问、命令白名单、路径边界和凭据写入全部由 CLI 代码控制，模型只负责理解意图和组织 Tool Call。

## 目标与边界

- 用户可以说「把桌面上的会议纪要转成 Markdown」或「找一下最近的财务报告」，Agent 能先查看允许范围内的文件，再让用户选择目标文件并调用现有转换流程。
- 文件定位使用受限只读命令，不增加一个只针对文件名的高层搜索器，也不允许模型执行任意 Shell 字符串。
- 工具调用使用结构化命令名和参数，底层通过 `exec.CommandContext` 直接执行，不经过 Shell 解释器，因此不存在管道、重定向、命令替换或拼接执行。
- 默认搜索范围包含当前 Chat 进程的工作目录，以及存在的 Desktop、Documents、Downloads。用户需要访问其他目录时，Agent 必须先让用户在本地终端输入并确认目录；不能默认扫描整个用户目录或磁盘。
- `ls`、`find`、`file`、`stat`、`wc` 等工具只返回文件系统元数据和匹配结果。
- `head`、`tail` 是只读命令，但会把文件内容交给模型，因此作为独立的内容预览能力，必须限制文件类型、行数和字节数，并明确提示用户该操作会让模型看到预览内容。
- API Key 等秘密通过单独的隐藏输入工具录入。工具可以返回输入长度、存储位置和成功状态，但不返回密钥内容，也不让模型通过参数传入密钥。
- 保留现有 `convert_document`、`set_configuration`、`ask_user` 和显式 `doc7 setup config --api-key-stdin` 入口；新能力扩展 Chat，不改变非 Chat 命令的公开语义。
- 不提供写文件、删除文件、移动文件、修改权限、安装软件、联网请求、进程管理、环境变量读取、任意配置文件编辑或任意命令执行能力。

## 目标行为示例

```text
场景：用户说「把最近那个会议纪要转成 Markdown」
之前：Agent 要求用户提供完整路径；模型不能搜索文件。
之后：Agent 使用受限 find/stat 查看当前授权目录中的文件元数据，按名称和时间筛选候选，调用 ask_user 让用户选择，随后使用已确认的文件对象转换。
不变项：模型不能凭空扩展搜索范围；没有用户确认的候选文件不能作为转换输入。
边界：没有匹配结果时，Agent 请求用户提供另一个目录或更具体的文件名，不尝试扫描整个磁盘。
```

```text
场景：用户说「看看这个目录里有什么文件」
之前：Chat 没有目录浏览工具。
之后：Agent 使用 `ls` 或 `find` 查看授权目录的名称、类型、大小和修改时间，不读取文件正文。
不变项：命令只能访问当前授权范围；输出不会包含文件内容。
边界：用户指定目录之外的路径会被拒绝，符号链接解析后也不能越过授权根目录。
```

```text
场景：用户说「检查这个 Markdown 的开头」
之前：Chat 没有受控文本预览能力。
之后：Agent 只有在用户明确要求查看内容时使用 `head`，并限制为文本文件、最多固定行数和字节数；预览内容会进入模型上下文。
不变项：默认的文件发现和转换流程不读取正文。
边界：二进制文件、超出限制的请求和未授权路径直接拒绝。
```

```text
场景：用户说「帮我配置这个模型，我需要输入 API Key」
之前：Agent 只能提示用户在 Chat 外运行 `doc7 setup config --api-key-stdin`。
之后：Agent 先提出非敏感配置的 dry-run，用户确认后调用 `input_secret`；CLI 在终端关闭回显读取 Key，将它写入当前凭据存储，并只返回长度和存储来源。
不变项：模型不会收到 Key 内容，Tool Result、聊天历史、日志和错误信息中都不包含 Key。
边界：非交互终端、`credential_store=env` 或用户取消输入时，不写入秘密并返回可执行的下一步。
```

```text
场景：用户输入 API Key 后问「刚才的 Key 是什么」
之前：没有 Chat 内安全录入工具。
之后：Agent 只能回答已配置的凭据来源和长度等非敏感元数据，不能恢复、复述或验证密钥正文。
不变项：凭据读取接口不对 Agent 开放。
```

## 已验证当前状态

- `internal/cli/chat.go` 当前只注册 `convert_document` 和配置类工具，System Prompt 明确禁止 Shell、文件搜索和秘密输入。
- `internal/cli/chat_tool.go` 当前要求 `convert_document.input` 出现在用户消息中，并通过 `os.Stat` 验证路径存在；模型不能处理用户只提供模糊文件名的场景。
- `internal/cli/chat_user_tool.go` 当前的 `ask_user` 只支持编号选择，不支持用户输入目录路径或其他非敏感文本。
- `internal/cli/chat_config_tool.go` 当前的 `set_configuration` 拒绝 `api_key`、`secret` 和 `token` 字段；配置写入仍然只允许非敏感字段。
- `internal/cli/setup.go` 已支持从 stdin 读取 API Key，并调用 `credentials.Store` 写入凭据存储，但当前入口不是 Chat Tool。
- `internal/credentials/credentials.go` 已提供 `auto`、`env`、`keychain` 和 `file` 存储边界；`auto` 在不支持系统 Keychain 的平台回退到文件凭据。
- `internal/cli/config_command.go` 已提供实际配置路径、可编辑字段和脱敏凭据来源展示。
- `internal/cli/read.go` 的 `executeRead` 是统一文档转换入口，新增文件定位能力应在 Chat Agent 层完成后继续复用它。
- 当前仓库的项目规范要求 Agent 不暴露任意 Shell、文件搜索或秘密读取能力；本方案将该规则细化为“结构化只读工具”和“独立隐藏输入工具”，不是开放任意命令执行。

## 已确认决策

- 文件定位不做一个只服务于模糊文件名的高度封装工具，改为提供一组小而明确的只读工具。
- 只读工具通过结构化参数调用，禁止把完整 Shell 命令字符串交给模型。
- 允许 Agent 使用文件系统浏览、文件类型判断、元数据读取和受限文本预览能力，但这些能力分级暴露。
- API Key 输入由工具发起，工具在本地终端确认目标后关闭回显读取；模型可以看到输入长度等元数据，但不能看到输入内容。
- 需要配置的非敏感字段继续采用 dry-run 和 `ask_user` 确认；敏感输入不能被 `--yes` 绕过，也不能通过自然语言消息传入。
- `head`、`tail` 和 `input_secret` 的敏感交互由本地工具直接确认，避免依赖小模型在多轮 Tool Call 中重复传递确认 ID。

## 建议的只读工具设计

### 工具调用边界

不增加 `run_shell(command: string)`。新增工具接收固定命令名和类型化参数，例如：

```json
{
  "command": "find",
  "root": ".",
  "pattern": "*会议*",
  "file_type": "document",
  "max_results": 20
}
```

CLI 将命令名映射到固定实现，不接受额外 argv。每个命令分别校验路径、参数、输出大小和执行时限。

### 第一组：不读取正文的文件系统工具

| 命令 | 用途 | 返回内容 | 限制 |
| --- | --- | --- | --- |
| `pwd` | 查看当前工作目录 | 当前授权根目录 | 不接受路径参数 |
| `ls` | 列出目录内容 | 名称、类型、大小、修改时间 | 只允许授权目录；不递归 |
| `find` | 按文件名、扩展名和时间筛选 | 候选文件元数据 | 限制深度、结果数和总输出大小 |
| `file` | 判断文件类型 | MIME、格式和文本/二进制判断 | 单文件；不能读取完整内容 |
| `stat` | 查看单文件元数据 | 大小、时间、权限、类型 | 单文件；不能越过授权根目录 |
| `wc` | 查看文本或二进制大小 | 字节数、行数等计数 | 只返回计数，不返回内容 |
| `realpath` | 规范化路径 | 规范化后的路径 | 必须在授权根目录内 |

这些命令用于“找到文件”和“判断是否适合转换”，正常情况下不会把文档正文放进模型上下文。

### 第二组：明确读取正文的预览工具

| 命令 | 用途 | 返回内容 | 限制 |
| --- | --- | --- | --- |
| `head` | 查看文本开头 | 最多固定行数和字节数 | 仅文本类文件；用户明确要求后使用 |
| `tail` | 查看文本结尾 | 最多固定行数和字节数 | 仅文本类文件；用户明确要求后使用 |

`head` 和 `tail` 虽然本身是只读命令，但不能标记为“内容安全”。Tool Result 必须包含 `content_visible_to_model: true`，CLI 在执行前显示本地提示，避免用户误以为模型只看到了文件名。

### 授权范围与路径安全

- Chat 会话建立一个 `authorizedRoots` 集合，初始只包含进程工作目录。
- 用户要求访问其他目录时，`authorize_directory` 在本地终端请求目录路径；CLI 规范化、解析符号链接并要求目标仍在用户明确提交的路径范围内。
- 目录授权只在当前 Chat 进程中有效，不写入配置文件，不跨进程保存。
- 工具执行前统一拒绝空路径、根目录、路径遍历、越界符号链接和不符合当前命令参数约束的路径。
- `find` 默认只搜索授权根目录，限制最大深度、结果数、单次执行时间和输出字节数。
- 工具不读取环境变量、不访问凭据目录、不接受 URL 作为本地文件路径；HTTP URL 仍由显式 `convert_document` 和现有远程确认流程处理。
- 文件候选返回稳定的会话内 `document_id`。模型后续调用转换工具时优先提交 `document_id`，CLI 从会话状态解析真实路径，不信任模型重新填写的路径。

### 文件选择与转换流程

1. 用户提出模糊文件描述。
2. Agent 使用 `find`、`stat`、`file` 和必要的 `wc` 获得候选元数据。
3. 有多个候选时，Agent 调用 `ask_user` 展示候选项；超过选项上限时分批展示，选项 ID 使用会话内稳定值。
4. 用户选择后，CLI 将 `document_id` 标记为已确认输入。
5. Agent 调用 `convert_document`，参数使用 `document_id` 和用户的转换要求。
6. CLI 解析会话状态中的真实路径，复用 `executeRead` 完成转换。

## 安全凭据工具设计

### 工具接口

新增 `input_secret`，首期只支持 `api_key`：

```json
{
  "kind": "api_key",
  "label": "当前模型服务 API Key",
  "credential_account": "default"
}
```

Schema 不包含 `value`、`api_key`、`token`、`secret`、Authorization Header 或任意自由凭据字段。凭据存储目标从当前已验证配置读取；Chat 不能传入任意凭据文件路径。

### 输入和结果契约

- 工具要求交互式终端；非 TTY 直接失败并提示使用 `doc7 setup config --api-key-stdin` 或显式环境变量。
- CLI 使用跨平台的无回显终端输入，不把字符写入普通 stdin 日志、命令输出或模型消息。
- 录入前显示凭据账户和存储方式，并让用户在本地确认是否开始输入；该确认不是 API Key 内容。
- 写入成功后返回：`stored`、凭据来源、配置路径、`length_bytes` 和是否为空；不返回内容、摘要、哈希或前缀。
- `length_bytes` 可以进入模型上下文，便于 Agent 判断用户是否确实输入了内容；该字段不能用于推断或恢复 Key。
- 失败时只返回分类错误，例如存储不可用、输入取消、空值或权限失败；不得包含原始 Key、完整凭据文件内容或底层请求头。
- 输入后不得调用模型验证“Key 是否正确”。验证只允许由 CLI 使用当前凭据对已配置 endpoint 发起真实请求，模型只能看到成功、鉴权失败或不可达等分类结果。

### 存储和配置流程

1. Agent 通过 `get_configuration` 读取当前 endpoint、model、credential store 和凭据来源。
2. Agent 用 `set_configuration` 创建非敏感配置 dry-run。
3. 用户通过 `ask_user` 确认配置变更，并可选择随后安全录入 API Key。
4. CLI 应用已确认的非敏感配置。
5. Agent 调用 `input_secret`，CLI 使用当前配置的 `credentials.Store` 保存 API Key。
6. CLI 使用脱敏配置重新加载并执行一次分类验证；验证结果不包含密钥内容。
7. 当前 Chat 轮次结束，下一条用户消息使用重新加载后的配置。

如果非敏感配置应用成功但凭据写入失败，结果必须明确区分两者：不声称整个配置完成，并告诉用户当前配置文件路径和下一步安全命令。凭据写入失败不能回滚用户已经确认的普通配置，也不能删除用户原有凭据。

`credential_store=env` 不由 Chat 工具写入，因为子进程不能安全修改父 Shell 的环境变量。当前配置为 `env` 时，`input_secret` 必须拒绝写入并提示用户在外部设置指定环境变量；`auto`、`keychain` 和 `file` 按现有凭据存储实现处理。

## Chat Agent 状态和提示词调整

- 工具集合增加只读文件工具、目录授权工具和 `input_secret`，但不增加任意 Shell 工具。
- System Prompt 必须说明：先用元数据工具定位文件；只有用户明确要求检查正文时才使用 `head` 或 `tail`；不得把模型推断路径当作用户授权。
- 文件工具结果需要区分 `metadata_only` 和 `content_visible_to_model`，让 Agent 能正确向用户解释已经暴露了哪些信息。
- 配置流程必须说明：非敏感配置先 dry-run，API Key 只能通过隐藏输入工具录入，长度可见但内容不可见。
- Agent 不得因为用户在普通消息里粘贴 API Key 就调用工具；应停止复述并引导用户重新通过隐藏输入入口输入。
- 工具调用结果状态需要支持：继续对话、等待用户选择、文档完成、配置完成和安全验证失败；敏感工具的本地确认不进入模型多轮状态。

## 文件与职责

- 修改 `internal/cli/chat.go`：注册新的 Chat 工具、更新 System Prompt、维护授权根目录、候选文件和工具状态，并处理“配置完成后本轮结束”的状态。
- 修改 `internal/cli/chat_tool.go`：将 `convert_document` 从直接接受模型路径扩展为优先接受会话内 `document_id`，保留显式用户路径和 HTTP URL 的现有入口。
- 修改 `internal/cli/chat_user_tool.go`：为候选文件选项增加结构化 `document_id`，避免依赖模型把文件 ID 写入选项 ID；秘密输入不复用该工具。
- 新增 `internal/cli/chat_fs_tool.go`：定义结构化只读命令 Schema、命令白名单、参数校验、授权根目录检查、输出限制和元数据结果。
- 新增 `internal/cli/chat_secret_tool.go`：定义 `input_secret`、跨平台无回显输入、长度统计、凭据存储调用和脱敏结果。
- 修改 `internal/credentials/credentials.go` 及平台实现：补充 Chat 所需的安全存储结果分类和无秘密错误边界，不改变已有 `Store` 公开行为。
- 修改 `internal/cli/chat_config_tool.go`、`internal/discovery/local.go`：让视觉验证复用当前和待应用配置解析出的凭据，但不向模型返回凭据。
- 修改 `internal/i18n/messages_en.go`、`internal/i18n/messages_zh_cn.go`：增加命令提示、目录授权、候选文件、内容可见性、隐藏输入和错误文案。
- 修改 `AGENTS.md`：将“禁止 Shell 和文件搜索”更新为“仅允许结构化只读工具，禁止任意 Shell 和写入工具”，并固化秘密输入工具的边界。
- 修改 `README.md`、`README.zh-CN.md`：说明模糊文件定位、目录授权、内容预览会暴露正文、Chat 配置和 API Key 安全录入流程。

## 影响范围

- Chat Agent 的工具数量和工具循环状态增加；普通聊天、显式 `doc7 read`/`extract`、Go API、HTTP 和 MCP 的行为不变。
- Chat 可以访问当前进程工作目录及用户在会话中明确授权的附加目录，但授权不写入配置，不跨进程保留。
- 模型可能看到文件元数据；只有用户明确要求且调用 `head`/`tail` 时才看到限定的文本预览。
- API Key 可以由 Chat 发起录入，但模型永远只能看到存储状态、长度和分类错误，不能读取凭据。
- Windows、macOS 和 Linux 都需要实现一致的无回显输入和路径边界检查；macOS Keychain 与非 macOS 文件凭据继续复用现有存储策略。

## 不在范围内

- 任意 Shell 字符串执行、管道、重定向、命令替换和脚本运行。
- `rm`、`mv`、`cp`、`mkdir`、`chmod`、`chown`、`sudo`、包管理器、系统设置、进程管理和网络命令。
- 默认扫描整个用户目录、根目录、挂载盘、网络共享或凭据目录。
- 让模型读取 API Key、环境变量、Keychain、凭据文件或完整认证错误响应。
- 通过 Chat 安装 LibreOffice、MuPDF、Chrome、LM Studio、Ollama 或下载模型。
- 将秘密输入通过 `ask_user` 普通文本模式实现；普通文本模式只能处理非敏感目录路径等输入。
- 长期保存目录授权、文件候选、秘密输入结果或跨进程聊天历史。

## 实施任务

### 任务 1：建立会话级文件授权和结构化只读命令契约

**依赖：** 无

**文件：**
- 新增：`internal/cli/chat_fs_tool.go`
- 修改：`internal/cli/chat.go`
- 修改：`internal/cli/chat_user_tool.go`

**符号与契约：** 只读命令工具 Schema、`authorizedRoots`、会话文件候选、`document_id`

**实施：**
- [x] 定义固定命令枚举和参数结构，不接受任意命令字符串或任意 argv。
- [x] 实现 `pwd`、`ls`、`find`、`file`、`stat`、`wc`、`realpath`，统一执行超时、输出字节数、结果数量和路径边界检查。
- [x] 为 Chat Agent 初始化当前工作目录授权根，并增加非敏感目录文本输入与确认流程。
- [x] 解析符号链接后的真实路径，阻止越过授权根目录；拒绝凭据目录和系统敏感目录作为搜索根。
- [x] 将候选文件注册为会话内 `document_id`，记录真实路径和来源工具，不把路径授权交给模型自行维护。
- [x] 实现候选分页或限制结果集，确保 `ask_user` 的选项数量不超过现有交互上限。

**交付物：** Agent 能在授权范围内查找和比较文件，但不能写入文件或执行任意命令。

**完成标准：** 模型提交未知命令、Shell 元字符、越界路径或写入类命令时均被拒绝；合法的文件发现结果只包含元数据。

### 任务 2：增加受控文本预览并接入文档转换

**依赖：** 任务 1

**文件：**
- 修改：`internal/cli/chat_fs_tool.go`
- 修改：`internal/cli/chat_tool.go`
- 修改：`internal/cli/chat.go`

**符号与契约：** `head`、`tail`、`content_visible_to_model`、`convert_document.document_id`

**实施：**
- [x] 增加 `head` 和 `tail` 的独立工具路径，仅允许文本类文件和固定最大行数、字节数。
- [x] 对二进制文件、超限请求、不可读文件和未授权路径返回分类错误。
- [x] 在 Tool Result 中明确标注正文已进入模型上下文，并在 CLI 中先显示本地提示。
- [x] 让 `convert_document` 接受会话内已确认的 `document_id`，由 CLI 解析真实路径；保留用户明确提供的文件路径和 HTTP URL 入口。
- [x] 转换仍然调用 `executeRead`，不在 Chat 工具中复制解析、渲染或视觉理解流程。

**交付物：** Agent 可以先浏览文件，再选择文件并转换；只有明确正文预览时才读取有限内容。

**完成标准：** 模型无法用 `head`/`tail` 越界读取或扩大输出；未确认的候选 ID 不能转换；转换输出仍使用现有 Markdown 和产物路径。

### 任务 3：实现隐藏输入的 API Key 工具

**依赖：** 无

**文件：**
- 新增：`internal/cli/chat_secret_tool.go`
- 修改：`internal/credentials/credentials.go`
- 修改：`internal/credentials/keychain_darwin.go`
- 修改：`internal/credentials/keychain_other.go`
- 修改：`internal/cli/chat.go`

**符号与契约：** `input_secret`、`length_bytes`、`credentials.Store`

**实施：**
- [x] 定义不包含秘密值字段的 `input_secret` Tool Schema，首期只允许 `api_key`。
- [x] 使用跨平台无回显终端输入；非 TTY、取消、空值和存储失败分别返回分类状态。
- [x] 复用当前 `credentials.Store`，禁止 Chat 传入任意凭据文件路径或覆盖存储目标。
- [x] 返回 `stored`、凭据来源、长度和脱敏配置路径，不返回 Key、哈希、前缀或原始错误响应。
- [x] 对 `credential_store=env` 明确拒绝 Chat 写入，提示用户在外部设置环境变量；`auto`、`keychain` 和 `file` 使用当前平台策略。
- [x] 让 CLI 使用当前凭据对 endpoint 做分类验证，但不把认证头或完整响应交给模型。

**交付物：** 用户可以在 Chat 中由 Agent 发起 API Key 安全录入，模型能知道录入状态和长度，但无法读取内容。

**完成标准：** 全仓库日志、Tool Result、Agent 消息和错误路径中不存在 API Key；输入长度可见，输入内容不可见。

### 任务 4：整合自然语言配置流程和安全状态

**依赖：** 任务 3

**文件：**
- 修改：`internal/cli/chat_config_tool.go`
- 修改：`internal/cli/chat.go`
- 修改：`internal/cli/chat_user_tool.go`
- 修改：`internal/cli/config_command.go`
- 修改：`internal/config/config.go`

**符号与契约：** `get_configuration`、`set_configuration`、`ask_user`、`input_secret`、Chat Tool 状态

**实施：**
- [x] 扩展 System Prompt，让 Agent 能按“查看配置、发现/验证模型、dry-run、用户确认、写入非敏感配置、隐藏录入 Key、分类验证”的顺序工作。
- [x] 增加“等待目录输入”“等待文件选择”“等待隐藏输入”“安全验证失败”等明确状态，避免工具执行后 Agent 提前结束或继续使用旧配置。
- [x] 非敏感配置仍必须验证完整变更集合后一次写入；凭据写入失败时准确报告部分完成状态，不删除旧凭据、不声称全部成功。
- [x] `ask_user` 继续只处理文件选择和普通配置确认；目录路径由 `authorize_directory` 在本地终端读取，密钥只能由 `input_secret` 读取。
- [x] 配置 endpoint 或 model 后重新加载配置；修改模型配置后本轮结束，下一条消息才使用新模型。
- [x] 让 Chat 配置流程可以在用户一句自然语言请求中完成，但不通过中文、英文关键词列表或固定句式解析意图。

**交付物：** 一个可由小模型执行的通用配置 Agent，不依赖硬编码语言规则。

**完成标准：** 用户可以自然语言完成本地或远程模型的非敏感配置，并在需要时通过隐藏输入录入凭据；模型不会看到秘密正文，工具不会绕过确认。

### 任务 5：同步项目规范和用户说明

**依赖：** 任务 4

**文件：**
- 修改：`AGENTS.md`
- 修改：`README.md`
- 修改：`README.zh-CN.md`
- 修改：`packaging/cli/README.txt`
- 修改：`packaging/windows/README.txt`
- 修改：`internal/i18n/messages_en.go`
- 修改：`internal/i18n/messages_zh_cn.go`

**实施：**
- [x] 将 Agent 工具安全边界写成可公开的项目规范：结构化只读工具、会话目录授权、内容预览显式标记、秘密独立隐藏输入。
- [x] 在中英文文档中说明模糊文件名、目录授权、候选选择、正文预览可见性和 Chat 配置流程。
- [x] 说明 API Key 长度等元数据可供 Agent 判断录入状态，但 API Key 内容不会进入模型上下文。
- [x] 同步首次配置、远程 endpoint、无鉴权本地模型和环境变量存储的边界。
- [x] 删除与新工具边界冲突的旧说明，不宣传任意 Shell 或全盘文件搜索。

**交付物：** 代码、项目规范、README 和发行包说明对工具边界保持一致。

**完成标准：** 中英文文档都能准确描述可用工具、授权范围、内容暴露边界和凭据流程，且不包含私有 endpoint、凭据或内部竞争话术。

## 不兼容或破坏性影响

- Chat Agent 将获得当前没有的本地文件元数据访问能力；这会扩大 Chat 进程可观察的文件范围，但仍限制在用户授权根目录内。
- 用户明确调用 `head` 或 `tail` 后，部分文件正文会进入模型上下文；这是有意的行为变化，必须在 CLI 和文档中明确提示。
- `convert_document` 后续优先使用会话内 `document_id`，模型自行填写的未经发现或确认的路径不能再作为模糊文件定位的替代方案。
- Chat 可以发起 API Key 输入，但 `credential_store=env` 仍需用户在外部设置环境变量，Chat 不能写入父 Shell 的环境。
- 不增加任意 Shell 兼容层；任何依赖 `doc7 chat` 执行自定义命令的做法都不受支持。

## 执行结果

- 初始授权根采用当前工作目录和存在的 Desktop、Documents、Downloads；其他目录由 `authorize_directory` 在本地终端授权。
- `head`、`tail` 和 `input_secret` 使用工具内本地确认，不依赖模型重复传递确认 ID。
- API Key 输入长度会返回给模型，Key 内容不会返回；使用文件凭据和 macOS Keychain 时均不把 Key 放入模型请求或进程参数。
- 本地 LM Studio 的 Qwen 9B 已真实验证文件元数据搜索和候选选择转换；Qwen 4B 已真实验证受限文本预览和隐藏 API Key 输入流程。
