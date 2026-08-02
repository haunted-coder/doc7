# doc7 自然语言配置 Agent 实施方案

**状态：** 实施中
**目标：** 让普通用户通过 `doc7 chat` 用自然语言查看、发现、验证和修改 doc7 配置，同时保证配置写入经过确认、模型无法读取或保存明文凭据。
**实现方式：** 在现有 Chat Agent 中增加四个受限配置工具，由 CLI 负责配置白名单、真实探测、两阶段确认、持久化和会话内配置刷新；模型只负责理解用户意图和选择工具。

## 目标与边界

- 用户不需要记住 `base_url`、`credential_store` 等 Key，可以直接说「使用本地 LM Studio」「切换到某个模型」「把界面改成中文」「检查现在的配置能不能用」。
- 配置意图由模型通过标准 Tool Call 表达，不使用中文关键词、固定句式或本地自然语言解析规则。
- Agent 只能访问 doc7 已定义的配置项、已知本地模型服务和视觉能力验证接口，不提供 Shell、任意文件读写、文件搜索或命令执行工具。
- 所有持久化配置修改必须经过用户确认；模型发起工具调用不能被视为确认。
- API Key 不得作为 Tool Call 参数，不得回显，不得写入聊天历史；Agent 只引导用户使用安全入口设置凭据。
- 保留现有 `doc7 config`、`doc7 setup` 和命令行参数，Chat Agent 是更友好的配置入口，不替代稳定的显式命令。
- 中英文行为一致，界面语言继续默认跟随系统，并允许通过配置覆盖。

## 目标行为示例

```text
场景：用户进入 doc7 chat 后说「你能配置自己吗」
修改前：模型只有 convert_document，回答自己不能配置。
修改后：模型调用 get_configuration，说明当前配置、配置文件路径、可修改内容和安全限制。
不变项：回复语言跟随当前界面语言和用户语言。
```

```text
场景：用户说「使用本地 LM Studio，选 qwen3.5-4b」
第一步：Agent 调用 discover_local_models，读取 LM Studio 实际返回的模型列表。
第二步：Agent 调用 verify_model_configuration，对候选模型发送真实图片请求。
第三步：Agent 调用 set_configuration 提交变更提案，CLI 展示 endpoint、model 等安全摘要，但不写文件。
第四步：用户在下一条消息中自然语言确认后，模型带原提案 ID 再次调用 set_configuration，CLI 才写入配置。
边界：模型不存在、服务未启动或视觉验证失败时不得保存为可用配置。
```

```text
场景：用户执行 doc7 --yes chat "使用本地 LM Studio 的第一个可用视觉模型"
修改后：全局 --yes 作为本次命令的显式写入授权，发现和视觉验证成功后可以直接保存。
边界：--yes 不允许模型读取、接收或保存 API Key，也不绕过远程文档上传的安全边界。
```

```text
场景：用户说「我的 API Key 是……，帮我保存」
修改后：Agent 不调用任何包含密钥参数的工具，不复述密钥，提示在终端运行 doc7 setup config --api-key-stdin。
边界：get_configuration 只返回凭据来源，例如环境变量或系统钥匙串，不返回凭据内容。
```

```text
场景：用户确认切换模型后继续聊天
修改后：CLI 写入配置并重新加载 AppConfig；本轮以明确的配置完成结果结束，下一条消息使用新 endpoint 和 model。
边界：配置写入失败时保留原内存配置和原配置文件状态，不声称切换成功。
```

## 已验证现状

- `internal/cli/chat.go` 只注册 `convert_document`，System Prompt 也明确要求只使用该工具，因此截图中的「不能主动配置自己」符合当前实现。
- `internal/cli/chat_tool.go` 只分发文档转换工具，工具执行结果用一个布尔值区分转换是否完成，尚不足以表达配置提案、等待确认和配置已应用等状态。
- `internal/cli/config_command.go` 已有可编辑 Key 列表、Key 规范化、值校验、配置路径展示和 `config.SetUserValue` 写入入口。
- `internal/config/config.go` 已有 `EffectivePath`、`SetUserValue`、`Load`、`ResolveCredentials` 和 `Validate`，配置文件以 `0600` 权限写入。
- `internal/discovery/local.go` 已能发现 LM Studio 与 Ollama 的真实模型列表，并通过合成图片发送真实视觉请求。
- `internal/cli/model_setup.go` 已有本地模型发现、视觉验证、远程 endpoint 确认和配置保存逻辑，但这些能力尚未作为 Agent 工具复用。
- 全局已有 `--yes`，当前用于非交互确认远程 endpoint，可以作为一次性 Chat 配置写入的显式授权。
- `doc7 config show` 已显示实际配置文件路径并隐藏凭据内容；中英文配置文案已有统一 i18n 入口。
- 当前仓库 `main` 与 `origin/main` 一致，最新正式版本是 `v0.3.1`。

## 已确认决策

- 配置能力必须由小型 Agent 和 Tool Calling 实现，不允许靠写死中文、英文关键词或固定句式判断。
- 实现必须通用，用户使用什么支持 Tool Calling 的模型，就由该模型理解自然语言配置意图。
- Agent 不获得 Shell、任意命令、任意配置文件编辑或秘密读取能力。
- API Key 不进入模型请求；继续使用 `doc7 setup config --api-key-stdin` 等本机安全入口。
- 完成后发布新的修订版本，计划版本为 `v0.3.2`。

## 技术设计

### 工具注册与分发

将当前单一的 `chatTools` 拆成按职责定义的类型化工具集合，并在 Chat Agent 中注册：

1. `get_configuration`
   - 无参数。
   - 返回配置文件实际路径、当前有效配置、endpoint 类型、凭据来源和可编辑 Key。
   - `base_url` 使用现有脱敏逻辑；不返回 `APIKey`、凭据文件内容或环境变量值。

2. `discover_local_models`
   - 可选参数 `runtime`，取值限制为 `any`、`lm_studio`、`ollama`。
   - 复用 `discovery.LocalModels` 返回真实服务名、endpoint 和模型 ID。
   - 只发现，不修改配置，不凭模型名称猜测视觉能力。

3. `verify_model_configuration`
   - 参数为 `base_url`、`model`，`provider` 固定使用当前支持的 OpenAI 兼容协议。
   - 复用 `discovery.VerifyVision` 发送真实图片请求，返回成功或可解释的失败原因。
   - 对 URL 做结构校验和脱敏展示，禁止 URL userinfo、fragment 和明显携带秘密的 query 参数。

4. `set_configuration`
   - 参数为类型化 `changes` 数组和可选 `proposal_id`。
   - 只允许白名单 Key，不接受任意 YAML、文件路径或自由结构对象。
   - 第一次调用创建待确认提案并返回安全摘要；模型随后必须调用 `ask_user`，由 CLI 读取用户的结构化选择后才能应用。
   - `--yes` 存在时可在同一条一次性命令中应用经过校验、且不含秘密的变更。

不在首轮增加 `reset_configuration`。删除整个配置文件是破坏性操作，继续保留显式命令 `doc7 config reset`，避免模型误触发。

### 配置白名单与类型

Agent 可修改以下非秘密配置：

| Key | Tool 参数类型 | 约束 |
| --- | --- | --- |
| `language` | string | `auto`、`en`、`zh-CN` |
| `provider` | string | 当前只允许 `openai-compatible` |
| `base_url` | string | 有效 HTTP/HTTPS URL，不允许内嵌凭据 |
| `model` | string | 非空模型 ID |
| `credential_store` | string | `auto`、`keychain`、`file`、`env` |
| `credential_account` | string | 非空账户标识，不包含秘密值 |
| `api_key_env` | string | 合法环境变量名，只保存变量名 |
| `ppt_renderer` | string | `auto`、`libreoffice`、`keynote` |
| `remote_confirmed` | boolean | 只表示用户确认远程文档上传 |
| `workers` | integer | 正整数 |
| `file_workers` | integer | 正整数 |
| `max_tokens` | integer | 正整数 |
| `timeout_seconds` | integer | 正整数 |

配置命令和 Agent 工具必须复用同一份字段元数据与校验入口，避免两套白名单和规则逐渐不一致。为此将 `editableConfigFields`、Key 规范化和配置值校验从 Cobra 命令细节中提取为 CLI 内部可复用组件，但不改变公开配置格式。

### ask_user 与两阶段确认状态

`chatAgent` 增加单个 `pendingConfigChange`：

```text
proposal_id
changes
safe_summary
created_after_user_message
```

- 每个 Agent 会话最多保留一个待确认提案，新提案替换旧提案并明确告知用户。
- 第一次 `set_configuration` 只生成随机提案 ID，不写配置。
- `ask_user` 展示模型提供的问题、选项标签和说明，CLI 只接受编号选择或取消，并把稳定选项 ID 返回模型。
- 应用时必须满足：提案 ID 相同、变更内容相同，并存在本次会话中真实完成且选择 ID 为 `confirm` 的 `ask_user` 交互；或者本次命令显式提供全局 `--yes`。
- 模型在同一轮重复调用、篡改变更内容、伪造不存在的 ID，CLI 均拒绝写入。
- 用户表达取消或改变目标时，由模型提出新提案；CLI 不通过固定自然语言判断「确认」「取消」。

### 原子写入与配置刷新

- 增加批量配置写入入口，一次校验并组装完整变更集合后再完成单次文件写入，避免连续 `SetUserValue` 造成部分成功，并保持 Windows 行为一致。
- 写入前基于当前配置应用全部候选值，并调用 `config.Validate` 验证最终结果。
- 写入成功后通过 `loadConfig` 重新加载配置和凭据来源，更新 `chatAgent.config`。
- 修改模型 endpoint 或 model 后，本轮不再向旧模型或新模型追加生成请求，CLI 直接输出已应用的结构化完成信息；下一条用户消息使用新配置。
- 配置文件写入失败时不改变 Agent 内存配置，并返回明确错误。

### 本地模型发现与验证

- 将 `discovery.LocalModels` 增加可选 runtime 过滤能力，保留 LM Studio 和 Ollama 的并发探测。
- 将视觉验证参数从仅接受 `discovery.Candidate` 调整为可复用候选结构，不复制探测图片和请求逻辑。
- Agent 只能把真实发现结果传给验证工具；手动提供的 endpoint 和 model 也可以验证，但不能伪称为本地发现结果。
- 验证成功只证明 endpoint 能处理 doc7 的视觉探测请求，不宣称模型质量、速度或所有文档格式都已验证。

### API Key 安全边界

- Tool schema 中不存在 `api_key`、`token`、`secret`、请求头或任意凭据值字段。
- Agent System Prompt 明确：发现用户在消息中粘贴疑似凭据时，不得复述、保存或传给工具，只提示安全命令。
- `get_configuration` 仅返回 `APIKeySource` 的安全描述。
- 远程 endpoint 无凭据时，Agent 可以配置 endpoint 和 model，但验证失败应提示用户在聊天外运行：

  ```bash
  doc7 setup config --api-key-stdin
  ```

- 远程 endpoint 的文档上传许可继续由 `remote_confirmed` 和现有确认逻辑控制；配置模型本身不等于同意上传文档。

### Agent 循环结果

将当前 `executeTool` 的 `(string, bool)` 改为明确的工具执行结果类型，至少区分：

- `continue_agent`：把工具结果交回模型继续组织回复。
- `document_completed`：文档转换已完成，沿用当前 CLI 完成提示。
- `configuration_applied`：配置已写入并刷新，本轮直接结束。
- `confirmation_required`：提案已创建，由模型向用户解释并等待下一条消息。

这样配置工具不会误用「转换完成」的结束逻辑，也避免配置切换后继续调用旧模型。

## 文件与职责

- 修改 `internal/cli/chat.go`：注册配置工具、维护待确认状态、更新 System Prompt、支持 `--yes` 配置写入语义和配置应用后的会话结束。
- 修改 `internal/cli/chat_tool.go`：把工具分发改为类型化结果，并继续承载 `convert_document`。
- 新增 `internal/cli/chat_config_tool.go`：配置工具参数、执行器、安全结果和提案状态，避免继续扩大文档工具文件。
- 修改 `internal/cli/config_command.go`：复用统一配置字段元数据和校验，不再独占这些规则。
- 新增 `internal/cli/config_fields.go`：配置字段白名单、类型转换、显示元数据和共享校验。
- 修改 `internal/config/config.go`：增加经过完整校验的批量原子写入能力，保留现有单项写入公开行为。
- 修改 `internal/discovery/local.go`：支持 runtime 过滤和可复用视觉验证候选。
- 修改 `internal/cli/model_setup.go`：复用统一的模型配置写入与验证能力，消除连续三次单项写入。
- 修改 `internal/i18n/messages_en.go`、`internal/i18n/messages_zh_cn.go`：增加工具执行、发现、验证、提案、确认、安全凭据和应用结果文案。
- 修改 `README.md`、`README.zh-CN.md`：在快速开始与 Chat 章节展示自然语言配置示例、`--yes` 行为和 API Key 安全入口。
- 修改 `packaging/cli/README.txt`、`packaging/windows/README.txt`：同步下载包中的首次配置和自然语言配置说明。

## 影响范围

- `doc7 chat` 可调用的工具从一个增加到五个。
- 配置文件仍使用现有 YAML 路径和 Key，不引入迁移。
- `doc7 config set`、`doc7 setup config`、环境变量和命令行参数继续可用。
- `set_configuration` 的批量写入会成为模型自动配置和本地模型自动发现共用的持久化入口。
- 不支持 Tool Calling 的模型仍能普通聊天，但不能通过自然语言自动配置；显式 `doc7 config` 和 `doc7 setup` 不受影响。

## 不在范围内

- 通过聊天安装 LM Studio、Ollama、LibreOffice、MuPDF 或系统软件。
- 自动下载或删除模型。
- Shell、文件管理、联网搜索、插件安装、系统设置修改。
- 让模型读取、接收、迁移或管理 API Key 明文。
- 长期保存聊天记录或跨进程保留待确认提案。
- 通过自然语言删除整个配置文件。

## 实施任务

### 任务 1：建立统一配置字段和原子写入契约

**依赖：** 无

**文件：**
- 新增：`internal/cli/config_fields.go`
- 修改：`internal/cli/config_command.go`
- 修改：`internal/config/config.go`
- 修改：`internal/cli/model_setup.go`

**符号与契约：** 配置字段元数据、共享值校验、批量原子写入、`config.AppConfig`

**实施：**
- [ ] 将配置 Key、类型、说明、规范化和校验整理为唯一的 CLI 内部白名单。
- [ ] 让 `doc7 config set` 和 Agent 工具使用同一校验入口。
- [ ] 增加批量更新：先应用到候选配置并完整验证，再原子写入配置文件。
- [ ] 让本地模型自动配置复用批量写入，避免 provider、base_url、model 部分成功。

**交付物：** 显式命令和 Agent 可以共享的一套类型化配置修改能力。

**完成标准：** 任一批量变更失败时配置文件保持原状；现有配置命令的公开用法和输出保持可用。

### 任务 2：增加配置读取、发现和视觉验证工具

**依赖：** 任务 1

**文件：**
- 新增：`internal/cli/chat_config_tool.go`
- 修改：`internal/cli/chat.go`
- 修改：`internal/cli/chat_tool.go`
- 修改：`internal/discovery/local.go`

**符号与契约：** `get_configuration`、`discover_local_models`、`verify_model_configuration`、类型化工具执行结果

**实施：**
- [ ] 注册三个只读工具并定义严格 JSON Schema。
- [ ] 返回实际配置路径、脱敏配置和凭据来源，不返回秘密。
- [ ] 支持按 LM Studio、Ollama 或全部运行时发现真实模型。
- [ ] 对候选 endpoint 和 model 发送真实视觉验证请求，并返回准确的能力边界。
- [ ] 将工具执行结果从布尔值改为明确状态，保持文档转换结束语义不变。

**交付物：** Agent 可以回答「我现在怎么配置」「本地有什么模型」「这个模型能不能处理图片」。

**完成标准：** 截图中的问题不再得到泛化的「不能配置」回答；工具输出不包含 API Key、认证头或凭据内容。

### 任务 3：实现两阶段自然语言配置写入

**依赖：** 任务 2

**文件：**
- 修改：`internal/cli/chat.go`
- 修改：`internal/cli/chat_config_tool.go`

**符号与契约：** `set_configuration`、`pendingConfigChange`、`proposal_id`、`configuration_applied`

**实施：**
- [ ] 第一次调用只创建单个待确认提案并返回安全摘要。
- [ ] 仅允许下一条或更晚的用户消息触发同一提案，拒绝同轮确认、未知 ID 和内容变化。
- [ ] 支持全局 `--yes` 对本次非秘密变更提供一次性明确授权。
- [ ] 写入成功后重新加载 Agent 配置，并让本轮直接以配置完成结果结束。
- [ ] 更新 System Prompt，使模型正确选择工具、遵守确认流程和 API Key 安全边界。

**交付物：** 用户可以用任意自然语言完成受控配置，不需要记忆 Key。

**完成标准：** 未确认时配置文件不变；确认后配置与会话内状态一致；切换模型后下一条消息使用新模型。

### 任务 4：同步中英文用户说明

**依赖：** 任务 3

**文件：**
- 修改：`internal/i18n/messages_en.go`
- 修改：`internal/i18n/messages_zh_cn.go`
- 修改：`README.md`
- 修改：`README.zh-CN.md`
- 修改：`packaging/cli/README.txt`
- 修改：`packaging/windows/README.txt`

**符号与契约：** Chat 配置提示、确认摘要、安全凭据提示、快速开始示例

**实施：**
- [ ] 增加中英文工具状态和错误文案，不把语言判断写进工具逻辑。
- [ ] 在快速开始中展示自然语言发现、切换和验证本地模型。
- [ ] 说明普通交互需要下一条消息确认，一次性自动写入需要显式 `--yes`。
- [ ] 明确 API Key 只能通过安全命令输入，不能粘贴给 Chat Agent。

**交付物：** GitHub README 与发行包说明能够指导首次使用者完成模型配置。

**完成标准：** 中文和英文文档都覆盖相同行为，命令与实际 CLI 一致。

### 任务 5：验证并发布 v0.3.2

**依赖：** 任务 4

**文件：**
- 不新增版本文件；继续使用现有构建时版本注入和 Tag 发布流程。

**实施：**
- [ ] 使用真实 LM Studio 运行「查看配置、发现模型、验证模型、提案、拒绝未确认写入、确认写入、切换后继续聊天」完整流程。
- [ ] 检查 Tool Call、错误和普通输出中没有 API Key、私有 endpoint 或凭据内容。
- [ ] 运行仓库现有格式、构建、公开源代码、体积、benchmark、Go 测试、vet 和漏洞检查。
- [ ] 使用 `esowt <esowt@qq.com>` 作为提交作者，推送 `main` 并发布 `v0.3.2`，等待 Release、Windows Portable 和 GHCR 流水线完成。

**交付物：** 包含自然语言配置 Agent 的 `v0.3.2` 正式版本。

**完成标准：** GitHub Release 包、`portable-latest` 和多架构 GHCR 镜像均由同一提交构建完成；线上安装或 `doc7 update` 可以取得 `v0.3.2`。

## 不兼容或破坏性影响

- 没有配置格式迁移，也不删除现有命令。
- `doc7 chat` 的工具集合和 System Prompt 会改变，支持 Tool Calling 的模型将能够提出配置操作；所有写入仍由 CLI 白名单和确认状态约束。
- 配置写入从逐项覆盖改为批量原子更新。若用户当前配置文件包含重复 Key，原子重写时需要采用与现有读取规则一致的单一有效值并消除重复项；这会整理配置文本，但不改变有效配置语义。

## 待用户确认的决策

- 按本方案首轮不提供自然语言 `reset_configuration`，删除配置仍使用 `doc7 config reset`。
- 版本按修订版本发布为 `v0.3.2`。
