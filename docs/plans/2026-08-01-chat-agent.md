# doc7 Chat Agent 实施方案

**状态：** 已完成
**目标：** 将 `doc7 chat` 从文档路径解析器改为真正调用已配置模型的轻量 Agent，同时保留受控的文档转 Markdown 能力。
**实现方式：** 在现有 OpenAI 兼容客户端中增加标准消息与 Tool Call 支持，由 CLI 维护短会话循环，并只暴露 `convert_document` 文档工具。

## 目标与边界

- 普通消息直接发送给已配置的本地或私有模型，不要求包含文件。
- `doc7 chat` 进入持续交互；`doc7 chat "你好"` 执行一次对话。
- 模型判断是否需要调用 `convert_document`，不使用中文关键词、固定句式或本地意图规则。
- 文档转换继续复用 `executeRead`，不复制渲染、视觉理解或输出逻辑。
- Agent 不提供 Shell、任意命令执行、文件搜索或目录遍历工具。
- 文档工具只允许读取用户在会话消息中明确给出的文件、目录或 HTTP URL。

## 目标行为示例

```text
场景：doc7 chat "你好"
修改前：本地解析找不到文件，直接返回 ConfigError。
修改后：调用当前模型并输出正常回复。
不变项：没有配置模型时仍引导用户运行 doc7 setup。
```

```text
场景：doc7 chat "把 report.pdf 转成适合知识库的 Markdown"
修改前：本地代码从句子中寻找路径并直接转换。
修改后：模型选择 convert_document，CLI 校验 report.pdf 确实由用户明确提供后复用主转换流程。
边界：模型编造或自行扩展的路径不得访问。
```

```text
场景：doc7 chat
修改后：进入短会话；普通问题直接回答，用户后续提供文档路径时可调用转换工具，输入 exit 或 quit 结束。
```

## 已验证现状

- `internal/cli/chat.go` 当前在请求模型前调用 `chatInput`，找不到已有路径或 URL 就返回错误。
- `internal/vlm/text.go` 已能调用 OpenAI 兼容 `/chat/completions`，但只支持单条用户消息，不支持历史与 Tool Call。
- `internal/cli/read.go` 的 `executeRead` 是统一文档转换入口，已经负责配置、模型发现、渲染、提取和输出。
- `internal/cli/model_setup.go` 已有本地模型发现、视觉验证、远程接口确认与配置保存逻辑。

## 已确认决策

- 不保留当前 `chat` 的本地路径意图解析语义。
- 不为旧行为增加兼容命令或兼容层。
- 使用模型和受限工具组成轻量 Agent，不按语言或固定句式判断意图。
- 完成后重新发布修订版本。

## 技术设计

### 模型消息接口

在 `internal/vlm` 增加面向 Agent 的消息、工具定义、工具调用与响应类型，并通过现有 endpoint、鉴权、超时和错误分类发送 Chat Completions 请求。响应允许纯文本或 `tool_calls`，不得要求两者同时存在。

### CLI Agent 循环

`internal/cli/chat.go` 维护 system、user、assistant、tool 消息。每轮最多执行有限次数的工具调用，防止模型无限循环。一次性模式输出最终回复后退出；交互模式保留当前进程内的短历史。

### 文档工具

只注册 `convert_document`：参数为 `input` 和可选 `instruction`。执行前检查 `input` 是否在用户消息中明确出现，并确认它是已有文件、目录或 HTTP URL。转换要求通过现有自定义 prompt 文件传给 `executeRead`。

### 模型配置

把现有模型发现流程提取为可复用函数。视觉文档入口和 Chat Agent 都使用同一套本地模型发现、视觉能力验证、远程确认和配置保存逻辑。

## 文件与职责

- 新增 `internal/vlm/chat.go`：Agent Chat Completions 请求与响应类型。
- 修改 `internal/cli/chat.go`：会话循环、工具注册、工具执行和输入授权。
- 修改 `internal/cli/model_setup.go`：复用模型配置发现流程。
- 修改 `internal/i18n/messages_en.go`、`internal/i18n/messages_zh_cn.go`：聊天提示、退出、工具和错误文案。
- 修改 `README.md`、`README.zh-CN.md`、`packaging/cli/README.txt`、`packaging/windows/README.txt`：展示普通对话、交互对话和文档工具用法。

## 实施任务

### 任务 1：扩展 OpenAI 兼容聊天客户端

**依赖：** 无

**实施：**
- [x] 定义类型化消息、工具和 Tool Call 数据结构。
- [x] 复用现有 endpoint、鉴权、超时、额外请求字段和错误分类。
- [x] 同时支持纯文本回复和工具调用回复。

**完成标准：** CLI 可以发送多轮消息和工具 schema，并读取模型返回的文本或 Tool Call。

### 任务 2：实现 CLI Chat Agent

**依赖：** 任务 1

**实施：**
- [x] 删除 `chatInput` 前置路径要求。
- [x] 支持一次性消息和持续交互模式。
- [x] 注册并执行受限 `convert_document` 工具。
- [x] 限制工具循环次数和会话历史长度。
- [x] 复用模型发现与远程确认流程。

**完成标准：** `doc7 chat "你好"` 请求模型；文档任务由模型调用工具；模型编造路径不能被读取。

### 任务 3：同步用户说明并发布

**依赖：** 任务 2

**实施：**
- [x] 同步中英文 README 和发行包说明。
- [x] 执行现有格式、构建、公开源代码、体积和 benchmark 检查。
- [x] 使用 `esowt <esowt@qq.com>` 提交并发布修订版本。

**完成标准：** 正式 Release 包含新 Chat Agent，CLI、Windows Portable 和容器流水线完成。

## 不兼容影响

- `doc7 chat` 不再由本地代码直接从句子中提取路径；文档动作由模型 Tool Call 决定。
- 不支持 Tool Calling 的模型仍可完成普通聊天，但无法通过 Agent 自动调用文档转换；用户仍可使用稳定入口 `doc7 <文件>`。

## 不在范围内

- 长期会话持久化、联网搜索、Shell、插件市场、多 Agent、任意文件搜索。
- 改动 `doc7 <文件>`、Go SDK、MCP 或 HTTP 服务的文档转换语义。
