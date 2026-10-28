# Codex CLI 0.159.3 之后的协议变化与模拟兼容性

调研日期：2026-10-21。当前仓库基线：`baf55ebaf92b8ac81e4e90d271f37837881277c7`。

## 结论与范围

存在值得处理的兼容差异，不能认为把 User-Agent 与 `version` 自动更新到最新版本，就完成了新版 CLI 的行为模拟。其中最明确的是 **0.163.0 默认即时中断在 Lite WS 场景下与当前 ctx_pool / HTTP bridge 入口不兼容**；其次是 **0.162.0 指令字段迁移与网关补齐逻辑冲突**，以及 **0.161.0–0.162.0 服务端重试建议未被网关容量重试链完整采用**。

这些结论来自官方稳定 tag、发布说明、相关 PR 和本仓库调用链。这里的“失效”分为三种：控制功能不能正常工作、请求结构与声明版本不一致、错误恢复行为不同。后两种不等于上游一定拒绝请求，更不能直接推导出上游会识别、封禁或按旧版本降载。

覆盖 OpenAI Codex/ChatGPT 后端相关的 HTTP Responses、Responses Lite 和 WebSocket 路径；区分真实 CLI 经过网关与其他客户端由网关合成 Codex 请求。不评估所有 TUI、MCP、沙箱和工具执行功能，也不把第三方 provider 的兼容性结论套用到官方后端。

本文是源码审查，未向生产上游发送请求，未复现运行中的故障。配套背景见[当前仓库客户端模拟实现](codex-client-fingerprint-simulation.md)。

## 稳定版本清单

截至调研时，官方最新稳定版为 `0.163.0`。`0.159.3`（2026-09-30）之后有以下六个稳定版本；不包含 alpha。

| 版本及官方发布说明 | 发布日期（UTC） | 与本次审查相关的变化 | 判断 |
| --- | --- | --- | --- |
| [0.160.0](https://github.com/openai/codex/releases/tag/rust-v0.160.0) | 2026-10-01 | 主要是 TUI、会话恢复、配置、权限；另有内容过滤后的恢复指导和 h2 依赖更新 | 未在已查推理链中找到该版引入的明确阻断项 |
| [0.160.1](https://github.com/openai/codex/releases/tag/rust-v0.160.1) | 2026-10-05 | 修复远程 stdio MCP 启动时的 Windows 环境变量 | 没有发现直接改变推理请求协议的补丁 |
| [0.161.0](https://github.com/openai/codex/releases/tag/rust-v0.161.0) | 2026-10-07 | 请求路由字段前置；重试与 WS→HTTP 回退遵守服务端等待建议；默认模型与 Daybreak 路由变化 | 请求顺序和恢复时序需要对齐 |
| [0.162.0](https://github.com/openai/codex/releases/tag/rust-v0.162.0) | 2026-10-08 | 顶层 instructions 移入 input；SSE/WS 解析 Retry-After；可选增量工具目录 | 本次最主要的请求体变化 |
| [0.162.1](https://github.com/openai/codex/releases/tag/rust-v0.162.1) | 2026-10-09 | 修复多行异步问题显示及后台服务器 feature 配置兼容检查 | 没有发现直接改变推理请求协议的补丁 |
| [0.163.0](https://github.com/openai/codex/releases/tag/rust-v0.163.0) | 2026-10-15 | 默认即时中断；新增 history_initialization；可选输出密文回放；incomplete 终止语义；符合资格时默认 Daybreak | WS 中断是最明确的功能兼容风险 |

发布说明中的比较链接有时从前一主版本而非最新 hotfix 开始；本次关键请求类型和客户端代码直接比较 `rust-v0.159.3` 与 `rust-v0.163.0`，再用 PR 和版本 changelog 定位变化，避免把分支关系当作简单线性升级。

## 1. 优先处理：即时中断帧无法穿过 ctx_pool / HTTP 桥接

### 官方变化

`0.163.0` 将 `instant_interrupt` 的默认值从 false 改为 true。用户在生成期间发送新的输入，会尝试提前结束当前响应以处理后续输入。[官方 PR #51812](https://github.com/openai/codex/pull/51812)

需要限定：普通采样的 WS 中断发送分支要求模型 `use_responses_lite=true`；标准非 Lite 不能直接套用这一结论。对应 [turn.rs](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/core/src/session/turn.rs#L2739) 发出 Drain 信号，WS 层随后发送：

```json
{"type":"response.interrupt","response_id":"…","mode":"discard_partial_items"}
```

该控制帧在 `0.159.3` 已存在，本次变化是默认开启相应用户交互，使原有网关缺口更容易触发。`0.163.0` 另为取消的 Guardian 分类请求增加确认、限时排空和连接复用规则；不能将 Guardian 的确认规则误说成每次普通 steering 都必须执行。[PR #53698](https://github.com/openai/codex/pull/53698)、[WS 发送与确认处理](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/codex-api/src/endpoint/responses_websocket.rs#L736)

### 当前仓库

- 配置默认 ingress 为 `ctx_pool`；账号配置和模式路由开关可以改变实际路径，本文未读取部署数据库。[配置默认值](../internal/config/config.go#L2440)、[入口模式选择](../internal/service/openai_ws_forwarder_ingress.go#L123)
- `parseClientPayload` 只接受空 type（补为 create）与 `response.create`，其他类型返回 PolicyViolation；`response.interrupt` 会命中拒绝分支。[解析入口](../internal/service/openai_ws_forwarder_ingress.go#L232)
- `ctx_pool` 与 HTTP bridge 都是在本轮处理完成后读取下一条客户端业务消息，没有在生成期间把中断帧转给上游。[ctx_pool 读取位置](../internal/service/openai_ws_forwarder_ingress.go#L1950)、[HTTP bridge 读取位置](../internal/service/openai_ws_forwarder_ingress.go#L782)
- passthrough 有双向转发及非 create 帧转发路径，不落入上述 create-only 解析器，不能认定它也有相同阻断。[透传控制帧处理](../internal/service/openai_ws_v2_passthrough_adapter.go#L1196)

**源码能确认的失败路径：** 新 CLI → Lite WS → ctx_pool/HTTP bridge → 生成中发送 interrupt。网关不能及时转发，随后读取该帧时可能以不支持的事件类型关闭连接。客户端也可能在此之前超时或重连；具体用户表现需要运行验证。

这是应优先复现的功能问题。只修改版本字符串不会修复；修复也不能只给 switch 增加一个允许值，还需要处理中途读取、转发、响应收尾和连接复用。

## 2. 请求体确定存在偏差：instructions 已移入 input

### 官方变化

`0.162.0` 把标准 Responses 的基础指令从顶层 `instructions` 移入 `input` 前缀的 `developer` 消息，与 Lite 的表达方式统一。HTTP 与 WS 请求类型都删除了该字段，并为指令消息生成与线程、文本关联的稳定 ID；指令变化会使 WS 使用完整请求而非沿用 `previous_response_id`。[PR #51156](https://github.com/openai/codex/pull/51156)

概念上的变化（省略其他字段）：

```jsonc
// 旧标准 Responses
{"instructions":"基础指令","input":[{"role":"user","content":"问题"}]}

// 新标准 Responses
{"input":[{"type":"message","role":"developer","id":"msg_…","content":[{"type":"input_text","text":"基础指令"}]},{"role":"user","content":"问题"}]}
```

### 当前仓库

- 普通 Codex 转发的非 Lite 分支仍在顶层 instructions 缺失、非法或为空时补入默认指令，没有先检查 input 是否已有新版 developer 指令。[补齐位置](../internal/service/openai_gateway_forward.go#L437)
- HTTP passthrough 的非 Lite、模型名包含 codex 分支，在 instructions 缺失时也会补回默认值。**缺失本身不会触发此处 403**；明确的空值或错误类型才会进入相应拒绝检查。[透传处理](../internal/service/openai_gateway_passthrough.go#L171)、[判定函数](../internal/service/openai_gateway_request_body.go#L1678)
- Lite 转换器会迁移工具与指令，但迁移后把顶层 instructions 置为空字符串，而不是删除；自行插入的 developer 消息也没有官方稳定 ID。[Lite 转换](../internal/service/openai_responses_lite_tools.go#L61)

**影响：** 新 CLI 的标准请求可能被网关重新加上旧字段和额外默认提示，造成请求形状不一致，并可能改变提示语义。合成 Lite 请求同样不等同于原生序列化。不能因此断言服务器已禁止 instructions 或必然返回 400；本次没有这样的实测证据。

Lite 指令消息及稳定 ID 在基线版本已有，不应把当前 Lite 转换器缺失 ID 全算作 0.162.0 新增回归。新版最关键的增量是：标准 Responses 也迁移到 input，顶层字段从官方两种请求类型删除。

## 3. 过载恢复不同：网关没有完整遵守新版 Retry-After 语义

官方连续做了三步：

1. `0.161.0`：ServerOverloaded/RetryLimit 在存在服务端建议且预算允许时可重试；WS 握手失败后的 HTTP 回退不能早于服务端指定时间；额度、使用限制及策略错误仍终止。[PR #49441](https://github.com/openai/codex/pull/49441)
2. `0.162.0`：从 `response.failed` 的 `response.error.headers` 读取 Retry-After。[PR #50418](https://github.com/openai/codex/pull/50418)
3. `0.162.0`：WS error 优先使用错误内部 headers，再读取事件外层 headers。[PR #51440](https://github.com/openai/codex/pull/51440)

本仓库已有 HTTP 响应头透传、OAuth 429 等待逻辑，不能说完全不支持 Retry-After。但通用容量错误构造函数只是标为同账号可重试，没有由流内 Retry-After 设置等待时间；该调用链设置 `SameAccountRetryDelay` 的分支是 OAuth 429。容量错误通常继续使用网关自己的 500 ms 起步指数退避，上限 8 s。[错误构造](../internal/service/openai_gateway_upstream_errors.go#L324)、[429 特例](../internal/service/openai_gateway_upstream_errors.go#L376)、[等待规则](../internal/handler/failover_loop.go#L58)

此外，网关仍把 `server_is_overloaded` / `slow_down` 改写为 `server_error`，理由是旧 CLI 将前者判成致命错误。自 0.161.0 起，“始终致命”这个前提已不完整：是否有服务端重试建议也参与决策。当前改写只改错误码，并不必然删除嵌套 headers，不能据此说新版 CLI 收不到任何等待建议。[改写实现及注释](../internal/service/openai_gateway_passthrough.go#L1386)

**影响：** 服务端要求较长等待时，网关可能提前重试、较快用尽预算，且与真正新版 CLI 的错误恢复轨迹不同。它能解释部分容量压力下的行为差异，但不能证明“旧版本优先被降载”。

## 4. 已部分支持：路由字段的 JSON 顺序

`0.161.0` 把 `model`、`stream`、可选 `service_tier` 放在大 input 之前，HTTP 与 WS 请求类型同时修改。官方解释是网关可以在正文尚未读完时先解析路由字段；并没有声称顺序是封禁或客户端认证规则。[PR #49675](https://github.com/openai/codex/pull/49675)

本仓库已有官方字段顺序表，但 `restoreOpenAIUpstreamJSONTopLevelOrder` 首先保留原始请求已有字段的顺序，只对新增字段使用顺序表。因此能保留原生 CLI 的顺序，却不会把任意客户端的 `input → model` 排列统一改成官方顺序。[顺序恢复实现](../internal/service/openai_gateway_upstream_errors.go#L491)

HTTP 请求由网关构造 WS 帧的路径使用 map，经 `WriteJSONWithContextTimeout` 到 `wsjson.Write`，没有使用上述顺序恢复器。Go 的 map JSON 编码按键排序，不能与官方 struct 字段顺序等同。此结论针对该 map 构造路径，不包括保留原始字节的 WS 入口。[帧构造](../internal/service/openai_ws_forwarder_payload.go#L239)、[发送](../internal/service/openai_ws_forwarder_v2.go#L394)、[实际编码](../internal/service/openai_ws_client.go#L305)

**影响：** 严格请求字节模拟仍有差异；对增量路由的性能或兼容影响需要测量，没有证据说明普通 JSON 接口会因顺序不同必然拒绝。

## 5. 新元数据：能保留，但合成时未补齐

`0.163.0` 在 `x-codex-turn-metadata` 增加 `history_initialization`，取值为 `new`、`cleared`、`cold_resume`、`warm_fork`、`cold_fork`、`supplied_history`。它描述运行时的历史初始化来源，跨 warm resume 和 compaction 保持；不是一个每轮随意重新猜测的字段。[PR #52325](https://github.com/openai/codex/pull/52325)

本仓库字段顺序表已经包含它，也保留客户端提供的值；但 `ensureCodexClientMetadataDefaults` 不生成该值。因此原生 CLI 透传通常不会在此丢字段，其他客户端合成请求则可能缺失。[顺序表](../internal/service/openai_codex_turn_metadata.go#L39)、[默认合成](../internal/service/openai_codex_turn_metadata.go#L412)

**影响：** 可确认的元数据完整度差异。没有发现上游强制要求此字段的公开证据，也不宜为所有恢复/分叉会话一律硬填 new。

## 6. 不宜误判为默认阻断的变化

| 变化 | 官方条件 | 当前审查结论 |
| --- | --- | --- |
| 0.162.0 增量工具目录 | `incremental_tools`；0.163.0 源码默认 false；主要作用于 Lite | 开启后工具新增、修改以历史中的 additional_tools 增量表达，删除通过通知表达。本仓库保留后续历史工具声明，但不维护同等的会话工具目录状态；需单独测试显式开启的客户端，不能说所有新版都已改用它。 |
| 0.163.0 输出密文回放 | `output_token_replay` 默认 false | 开启时新增 include=output.encrypted_content，并在消息/工具调用中保存及回放密文。当前 map 转换与 include 补齐路径并非字段白名单，未发现必然删除所有这些字段的证据；切账号、重放、错误恢复仍需实测。 |
| 0.163.0 异步压缩基础设施 | `async_compaction` 默认 false | 不能把基础设施 PR 直接视为默认会并行发出新的压缩请求；本次未验证显式启用后的完整调度路径。 |
| 0.163.0 response.incomplete | 输出上限等导致 incomplete 时应终止，保留原因，不继续按流错误重试 | 本仓库 WS 已将其视为终止事件，相关 retry 判定测试也已有用例；没有发现“完全不认识此事件”的缺口。HTTP 与所有转换路径的 usage 一致性未运行验证。 |
| 0.161.0 / 0.163.0 Daybreak | 从 opt-in 变为符合资格时默认；还受账户、模型及服务端能力限制 | 可能改变 access_programs 的使用频率，但该字段在 0.159.3 已存在。不能给全部账号硬加，也不能算成新的强制身份签名。 |

对应证据：[增量目录 #50540](https://github.com/openai/codex/pull/50540)、[命名空间更新 #51119](https://github.com/openai/codex/pull/51119)、[删除语义 #51202](https://github.com/openai/codex/pull/51202)、[输出回放 #52742](https://github.com/openai/codex/pull/52742)、[异步压缩 #53614](https://github.com/openai/codex/pull/53614)、[incomplete #53451](https://github.com/openai/codex/pull/53451)、[Daybreak #53701](https://github.com/openai/codex/pull/53701)。默认值直接核对了 [0.163.0 features 源码](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/features/src/lib.rs)。

本仓库对应：[历史 additional_tools 保留](../internal/service/openai_responses_lite_tools.go#L245)、[include 合并](../internal/service/openai_codex_transform.go#L1480)、[input 字段复制](../internal/service/openai_codex_transform.go#L1650)、[WS 终止事件](../internal/service/openai_ws_forwarder_support.go#L199)、[已有 incomplete 测试](../internal/service/openai_ws_forwarder_test.go#L127)。这些测试仅阅读，未执行。

## 已核对而未发现协议换代的部分

- 基线与 0.163.0 的 `login/src/auth/default_client.rs` 内容相同；已查身份构造代码未显示新增强制客户端签名。这个范围不代表整个鉴权系统完全没有变化。[0.159.3](https://github.com/openai/codex/blob/rust-v0.159.3/codex-rs/login/src/auth/default_client.rs)、[0.163.0](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/login/src/auth/default_client.rs)
- `codex-api/src/endpoint/responses.rs` 内容相同；请求类型和调用方仍发生了本文列出的变化，不能因 endpoint 文件相同就说 HTTP 行为完全相同。[0.163.0 HTTP endpoint](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/codex-api/src/endpoint/responses.rs)
- 两个 tag 的 Responses WS beta 都是 `responses_websockets=2026-02-06`；已查主请求压缩仍按 OpenAI provider、Codex 后端鉴权及压缩开关选择 zstd。[0.163.0 客户端](https://github.com/openai/codex/blob/rust-v0.163.0/codex-rs/core/src/client.rs)
- 官方依赖升级可能影响底层传输，不能仅凭 h2/hyper/quinn 的版本升级断言产生了新的强制指纹。本仓库 Go 传输与官方 Rust 传输的差异本来就存在，本次没有抓包比较 TLS/HTTP2。

## 建议验证顺序

1. 用相同 Lite 模型分别走 ctx_pool、HTTP bridge、passthrough，生成中发送跟进输入，记录 interrupt 是否及时到达上游、终止帧与连接是否正确处理。这是最具体的功能验收项。
2. 对相同线程和提示采集脱敏后的 0.159.3、0.162.0、0.163.0 请求，核对标准/Lite 的 developer 前缀、稳定 ID、顶层 instructions 和 WS continuation；先确认语义没有被重复注入。
3. 用本地 mock 返回带 Retry-After 的 SSE/WS 容量错误，测量下一次请求的实际时间，覆盖 WS 握手失败后的 HTTP fallback，避免用真实容量波动推断策略。
4. 再核对序列化顺序和 history_initialization；增量工具与密文回放按显式开启的场景补验收，不为默认关闭功能先构建完整模拟状态机。

不要简单把版本号长期固定或一路追最新当作兼容性修复。版本声明与已验证的协议能力需要一起维护；此次没有发现“0.159.3 之后一律不能用”的证据，但已经足以否定“自动改版本号即可完整模拟新版 CLI”。

## 验证边界

此次只新增调研文档，未改生产代码、版本设置或自动同步开关。完成了发布信息、关键 tag 源码、PR 补丁及本仓库直接调用链核对；未逐条审计所有提交。未运行 Go 测试（当前环境未找到 Go 工具链），未执行端到端请求或传输抓包，未读取线上实际功能开关和账号模式。上文严重程度是依据静态失败路径排序，不是线上故障率测量结果。
