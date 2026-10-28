# Codex CLI 行为与指纹对齐：核心开发指南

整理日期：2026-10-26。代码基线：`e59fb52e2327aad880cbf2f2c983a161b0df4a3b`。

本文面向修改 OpenAI/Codex 转发链的开发者。重点回看 2026-09-29 至 2026-10-21 的提交，必要时追溯更早的身份实现；“近期”截至本仓库当前 HEAD，不代表已经追踪到今日官方 CLI 的全部变化。历史结论依据提交 diff 和调用链；本次已实施公开 `rust-v0.164.0` 的应用层对齐改动，并在本机隔离环境运行测试。2026-10-28 获用户授权后，已将 `eb6ac2192` 部署至 ccvps，完成启动、健康接口及控制台检查；没有进行真实账号请求，真实上游验收仍未完成。具体证据、备份与回滚方式、已修正的调度旧断言和适用边界见 [0.164.0 对齐记录](codex-cli-0.164.0-alignment.md)。

## 1. 当前完成到哪一层

2026-10-28 部署后另完成[两端自然流量的被动核对](codex-cli-0.164.0-passive-acceptance.md)。已关联 15 个响应 ID，核对 13 对网关接收/发送侧续聊引用，并看到实际缓存读取；未主动发起真实账号测试，也未补齐完整请求、上游握手或 CLI 直连官方对照。下文的离线验证边界仍适用。

这轮工作的核心，是让同一次请求的客户端声明、身份关系、正文、元数据和传输选择相互一致，并减少网关改写带来的额外差异。

| 层次 | 当前实现 | 证据边界 |
| --- | --- | --- |
| 客户端声明 | UA、originator、version 同源；推理、认证和模型清单按各自规则处理 | 生效版本受运行设置影响，不一定等于代码常量 |
| 身份与状态 | 调用方及凭据隔离、可选设备收敛、UUID 版本/时间、turn-state 来源、续接与连接寿命 | 有网关自己的派生与调度策略，不等于原生 CLI 的完整状态机 |
| 应用协议与字节格式 | HTTP/WS 头差异、turn metadata 专用编码、部分正文顺序保持、Lite/compaction、HTTP zstd | 字节往返测试只证明特定对象；并未覆盖完整网络会话 |
| 底层网络指纹 | 核心链使用 Go HTTP transport 与 `coder/websocket` | 未证明 TLS ClientHello、HTTP/2 SETTINGS、头部线序或压缩字节与官方 Rust 栈一致 |

因此可以说“对齐了这些可观察行为”，不能据此说“所有请求与官方逐字节不可区分”，也不能把降智检测结果当作客户端指纹验收。

本机 0.164.0 安装包另已完成隔离 WS 对照：预热与多轮续接、取消后重连、steer 中断的两种确认顺序、Retry-After，共五个场景直连 mock 与经过本地网关转发层均通过。实际观察到 `turn/interrupt` 断连，而 `turn/steer` 发出 `response.interrupt`；握手身份与压缩提议仍有差异。此前将 v7/v5 解释为源码与安装包冲突的结论已撤回：同一本机二进制默认增量模式使用历史项 UUIDv7，关闭该模式则使用临时前缀 UUIDv5，与公开源码两条路径相符。见[WS 采样结果](codex-cli-0.164.0-ws-sample.json)、[ID 对照](codex-cli-0.164.0-id-ab.json)及[对齐记录](codex-cli-0.164.0-alignment.md)。随后另完成完整后端的隔离验证：假账号的 HTTP/WS 鉴权、余额限制、选号、用量扣费、Lite 前缀稳定性以及基线/候选启动回退均通过。最终所选回归 5241 项通过、0 失败，1 项真实 API 测试跳过。这仍不覆盖真实账号缓存或底层网络指纹。

## 2. 近期具体做了什么

下表按提交实际内容归纳；混合提交只列与本主题有关的部分。用 `git show <提交> -- <文件>` 可复查原始改动。

| 时间 / 提交 | 改动 | 开发时应保住的结果 |
| --- | --- | --- |
| 早期基础：`2eb24814f`、`fce41e318`、`6793d5ac8`、`bb6c3b4f6`、`a34123959`、`d493ce0bb` | 建立统一身份与版本同步；设备收敛改为 opt-in；凭据面身份同源；按上游凭据隔离 Codex IDs | 这些不是十月新功能。后续修改应复用现有策略，不另造身份来源 |
| 09-20 `763443abf`；09-29 `049c00f78` | 保留 metadata 非 ASCII 转义；修正 Lite 工具/指令转换；协调正文与头部元数据；引入 regular Responses zstd；按具体 turn-state token 溯源；补 OAuth 到期刷新及 401 恢复 | 工具续接不重造 turn；重试正文可重放；临时刷新失败不当作永久失效 |
| 10-03 `913817cd9` | 补缺失的 reasoning/tool_choice/parallel_tool_calls/include 与客户端元数据；OpenAI legacy compact 提升为 native compaction v2；补环境时区/日期 | 非原生客户端也有必要字段；保留已有合法字段及其他供应商的独立协议 |
| 10-13 `16db311ad` | turn metadata 按 serde 对象结构排序，停止 HTML 转义；生成 v7 会话/回合形状；补 window_number/context_window_id；删除非官方头；新加坡位置字段同步；Python 别名由 `python__sub2api` 改成 `python__custom` | 不因 Go map 重编码改乱嵌套元数据；最终出口不残留代理专属头和旧别名 |
| 10-13 `7565484fe`、`2a66c987c` | 加 metadata 字节往返测试；修复派生 v7 落入未来、账号隔离把 v7 改成 v4、无会话信号时所有对话合并的问题；补 history_initialization 的字段位置 | UUID 的版本、时间与 window 序号相互协调；不同对话不只按账号合成同一个 session |
| 10-14 `e6e20441f` | 改写普通 JSON 后恢复已有顶层字段相对顺序；普通正文关闭 HTML 转义；OpenAI transport 禁止 Go 自动声明 gzip | 保住原生请求的路由字段前缀；响应仍由现有代码解压 |
| 10-15 `12466aa7d`、`53abc8a4c` | Codex OAuth/SetupToken WS 固定 v2 Beta；保留条件握手头；头部 metadata 去掉完整工具清单，帧正文保留；增加真实本地 socket 测试 | HTTP 与 WS 使用不同头规则；WS 的 version、Beta、account 身份不能误删。`a6d5e59ff` 只增加两个测试断言，未另改生产实现 |
| 10-15 `234ef4d60`、`25b06c394` | 修正 Responses 流的保活、首输出、终止和错误处理；WS server error 接入临时停调策略 | 客户端看见的响应语义与重试边界保持正确；这些属于网关兼容性，不能算成官方出站指纹复刻 |
| 10-20 `7e6e0c17f`、`b19a15475`、`817714ef2` | 移除旧打票链；ModelTrace 改走进程内网关；HTTP effort probe 复用生产发送栈；增加跟随账号传输的独立 WS probe、观测字段和握手一致性测试 | 检测不能用另一套 transport/身份得到与生产无关的结论；不同检测仍有各自入口 |
| 10-20 `f2f98830b` | OAuth 类账号未显式配置时跟随默认 `ctx_pool`；前端创建/编辑默认值同步 | 显式 off、全局禁用和 force HTTP 仍有效；API Key 不被无条件改成 WS |
| 10-21 `0036d3905` | 删除无人调用的 harvest profile、专属 H1 不复用分支及测试 | 不重新引入打票专用 transport；保留诊断历史和敏感字段剥除 |
| 10-21 `baf55ebaf`、`e59fb52e2` | 完善 55/60 分钟 WS 寿命处理、续接保护和默认 30 秒重试预算；恢复 legacy session 默认一小时粘性 TTL | 降低换连接/换账号造成的状态断裂；寿命与粘性是网关策略，不是“官方客户端也这样实现”的证明 |

`baf55ebaf` 同时调整 Sol 模型默认 context window，但模型目录与定价维护不属于客户端指纹复刻，本文不展开。

## 3. 改代码前先定位入口

同一个逻辑请求可能走不同通道。只检查一个 header builder 或 HTTP handler 不足以覆盖最终出站行为。

| 修改主题 | 先读共享实现 | 必须检查的直接入口 / 使用处 |
| --- | --- | --- |
| UA / 版本 | [openai_codex_identity.go](../internal/service/openai_codex_identity.go)、[setting_gateway_runtime.go](../internal/service/setting_gateway_runtime.go)、[版本同步](../internal/service/openai_codex_version_sync_service.go) | 普通 HTTP、passthrough、WS、认证端点、[模型清单](../internal/service/openai_codex_models_service.go) |
| 账号身份 / 收敛 | [openai_codex_account_identity.go](../internal/service/openai_codex_account_identity.go)、[openai_codex_fingerprint.go](../internal/service/openai_codex_fingerprint.go) | 每次选号后的 prepare/stage、正文与头、shadow 账号、重试换号、账号创建/更新种子处理 |
| metadata / 正文 | [openai_codex_turn_metadata.go](../internal/service/openai_codex_turn_metadata.go)、[JSON 编码与顺序恢复](../internal/service/openai_gateway_upstream_errors.go) | [HTTP](../internal/service/openai_gateway_forward.go)、[HTTP 透传](../internal/service/openai_gateway_passthrough.go)、WS 首帧与后续帧 |
| WS 协议 / 头 | [openai_ws_forwarder_payload.go](../internal/service/openai_ws_forwarder_payload.go)、[协议选择](../internal/service/openai_ws_protocol_resolver.go) | [HTTP→WS](../internal/service/openai_ws_forwarder_v2.go)、[WS ingress](../internal/service/openai_ws_forwarder_ingress.go)、[WS passthrough](../internal/service/openai_ws_v2_passthrough_adapter.go) |
| 连接与续接 | [openai_ws_pool.go](../internal/service/openai_ws_pool.go)、[HTTP bridge](../internal/service/openai_ws_http_bridge.go)、[turn-state](../internal/service/openai_codex_turn_state.go) | acquire、排队/健康检查后复查、回放完整性、首输出前后、窗口切换、取消与释放 |
| HTTP 实际发送 | [openai_plugin_transport.go](../internal/service/openai_plugin_transport.go)、[http_upstream.go](../internal/repository/http_upstream.go) | 插件接管、每次发送副本、GetBody、ContentLength、解压、401 重放 |
| 诊断 | [ModelTrace runner](../internal/service/codex_diagnostic_gateway.go)、[effort probe](../internal/service/codex_effort_probe.go) | handler、历史 repo/DTO、前端观测字段、[探针测试](../internal/service/codex_effort_ws_probe_test.go) |

主要处理关系如下，具体函数顺序以各入口为准：

```text
选中账号并解析真实凭据来源
  → 正文协议转换、模型/档位确定
  → 调用方与凭据身份隔离、可选收敛（正文和头共享结果）
  → metadata 补齐与同步、身份三元组和 routing hint
  → 最后一次头部投影，检查最终正文
  → 核心 HTTP/WS 或插件发送
```

## 4. 必须保持的行为

### 4.1 一个客户端身份来源，按端点使用

- 复用 `resolveCodexOutboundIdentity`。管理员 UA 可贡献受支持的客户端名称、OS、架构和终端；版本由统一生效值重建，不能只改单独一个 `version` 头。
- 版本优先级为管理员设置 → 已同步值 → 内置 `0.164.0`。内置 UA 为 `codex-tui/0.164.0 (Windows 10.0.22621.1848; x86_64) dumb`。关闭自动同步不会清除旧同步值。
- `gateway.disable_codex_identity_enforcement` 默认 false。设 true 后改走配对/低版本校正，并停止最终非官方头清理；它与账号 `codex_fingerprint_mode` 是两个独立开关。
- 认证端点复用 UA/originator，不主动补 version；模型清单的 Version 与 `client_version` 有独立选择逻辑。不能把推理头集合复制到所有端点。
- 自动同步只是更新声明。升级版本前应核对对应官方 tag 的请求类型、调用方和功能默认值；单独追新版本不能补出新行为。

### 4.2 区分身份隔离、设备收敛和真实会话状态

账号隔离使用下游 API Key 与真实上游凭据命名空间。命名空间优先来自 ChatGPT account/user ID，再使用持久化种子；SetupToken 还有 token 摘要兜底。shadow 先解析凭据来源，每次 failover 覆盖暂存结果，不能继续使用上一账号的身份。

`codex_fingerprint_mode` 缺省为 off，只表示不做额外设备/会话收敛：规范身份、账号隔离和缺失 metadata 补齐仍存在。device 收敛 installation；session/full 当前共用分支，收敛 session 并保留线程区分，不表示所有线程与 turn 合成一个值。非 off 模式复用系统管理的种子，正文和头使用同一份预计算结果。

约束：installation 按 v4 形状；改写 session/thread/turn 时按 v7 并保留已有 v7 时间位；window 保持 `thread:ordinal`。已有 turn_id 与开始时间不能因工具续接或普通重试而重建。没有会话信号时，现有兜底尝试用真实响应链或首条用户内容区分对话，但不能把这种启发式说成完整会话追踪。

探针没有入站请求时，可复用 `prepareCodexAccountIdentitySource(ctx, nil, account)` 与现有纯派生函数，使用其既有调用方作用域。不要复制一套“探针身份算法”，也不要对已隔离值再次施加账号命名空间。

### 4.3 头部投影必须在最后一次 metadata 写入之后

`projectCodexOutboundHeaders` 是显式删除清单和 session/thread 配对，不控制 Go 最终发送头的顺序、大小写或 HTTP/2 编码。

| 字段 | regular HTTP Responses | Codex WS 握手 |
| --- | --- | --- |
| UA / originator / version | 同源、保留 | 同源、保留 |
| `session-id` / `thread-id` / `x-client-request-id` | 保留并维护关系 | 同左 |
| `session_id` / `conversation_id` | 最终删除 | 最终删除 |
| `x-codex-installation-id`、`accept-language` | 最终删除；installation 留在正文 metadata | 同左 |
| `OpenAI-Beta` | 最终删除 | 保留 `responses_websockets=2026-02-06` |
| `x-codex-beta-features`、turn-state、routing hint | 按能力、来源和最终路由处理 | 按各自规则处理 |
| metadata 中的 `tool_namespaces_info` | 头部投影删除；正文保留 | 握手头删除；create 帧正文保留 |

metadata 补齐会重新写入部分旧头，因此 WS builder 里的清理不是最后一步：v2 forwarder、ingress 和 passthrough 在归一化后还需执行最终投影。不要根据旧注释把 `version` 当成应删除字段，实际删除清单和 socket 测试才是当前行为。

条件 WS 头包括 parent-thread、subagent、memgen、timing metrics，不能一律删除。普通 Responses WS 不转发客户端 `x-oai-attestation`。legacy compact、messages bridge、公共 API Key 和兼容供应商分别有分支，不能机械套用此表。

### 4.4 JSON 语义正确之外，还要保留已验证的格式

turn metadata 必须经过 `decodeCodexTurnMetadata` / `marshalCodexTurnMetadata`：已知字段按声明顺序，未知字段排在末尾并按字典序；workspace、namespace、function、compaction 子对象各有顺序。保持 `json.Number` 的整数精度、非 ASCII 转义、非 BMP 代理对，以及 `& < >` 不做 HTML 转义的行为。

正文平铺字段、内嵌 `x-codex-turn-metadata` 字符串和头部必须共享相关身份值，但完整正文元数据与删去工具清单后的头部字符串本来就不同，不能要求两段字符串完全相等。已有 history_initialization 应保留，不要在无法判断恢复/分叉来源时一律补 new。

普通正文使用 `marshalOpenAIUpstreamJSON`，重编码路径按需调用 `restoreOpenAIUpstreamJSONTopLevelOrder`。它保留已有字段相对顺序，只给新增字段安排顺序，不递归恢复全部嵌套字节。合成 WS create map 在正式 writer 中使用统一编码并按 type、model、stream 等官方字段顺序输出；原生 RawMessage 保留已有顺序。不要把顶层顺序或 metadata 的字节一致性测试外推到整段网络会话。

### 4.5 协议转换应保住输入语义

- regular OAuth Responses 使用 `store=false`、`stream=true`，通过既有 transform 补默认字段、处理不支持的参数和 reasoning include。不能让局部兼容补丁丢掉已有工具声明、结果或密文。
- 普通 Codex 推理的顶层 instructions 在最终 metadata 归一化时迁入 developer input；已有指令不重复注入默认内容。合成前缀 ID 按公开 0.164.0 源码规则稳定派生，原生 ID 保留；legacy compact 与公共 API Key 走各自分支。精确字段拒绝的 input 下标必须对应最终发送正文。
- Lite 使用[现有转换器](../internal/service/openai_responses_lite_tools.go)：工具和指令进入 input 前缀，历史工具仍保留；图片 detail 只在图片项处理。保留 schema、显式 tool_choice 与工具调用/结果配对。
- OpenAI legacy compact 通过[入口提升逻辑](../internal/service/openai_compact_body_signal.go)进入 native Responses；`compaction_trigger` 位于 input 末尾，remote_compaction_v2 与真实能力一致。不要给其他供应商强行改端点。
- [routing hint](../internal/service/openai_routing_hint.go)取最终上游模型与生效 service tier。改模型/档位时同时检查头、正文和 WS 池路由，避免彼此矛盾。
- 保留工具名映射的双向关系；`python__custom` 是本仓为绕开保留名做的兼容别名，不是官方固定工具名。

### 4.6 环境与传输的适用条件不能扩大

[环境处理](../internal/service/openai_request_locale.go)只改匹配结构的 OpenAI user 环境块：替换 timezone，缺失时补字段；**已有 current_date 保留**。默认 `Asia/Singapore`。web_search 的已有 timezone 会同步，新加坡映射只替换已有 country/region/city 字符串，其他时区不推测城市。这里未实现整套 OS、路径、sandbox 或遥测模拟。

[zstd 压缩](../internal/repository/http_upstream.go)仅作用于可重放的 HTTPS `POST https://chatgpt.com/backend-api/codex/responses` JSON 请求，且不能覆盖其他有效 Content-Encoding。压缩只改每次发送副本，Body、GetBody、ContentLength 与实际字节同步，清理旧长度/Transfer-Encoding。公共 API、其他域名、未提升的 legacy compact、WS 帧不适用。

OpenAI HTTP transport 关闭 Go 自动添加 gzip，响应继续按 Content-Encoding 解压；WS 使用自身压缩协商。插件接管后最终网络行为由插件决定。仓库 [uTLS 账号开关](../internal/service/account.go)只适用于 Anthropic OAuth/SetupToken，不能用“仓库已有 TLS 指纹功能”来证明 Codex 已有同等覆盖。

### 4.7 连接变化不能静默破坏续接

turn-state 是上游签发的不透明值：只在响应真正提交给客户端时记录具体 token 来源；已知来自另一凭据的值在出站前剥离，同源或来源未知的值沿用现有处理。不能只记“这个 session 最后一次 token”，否则并行回合会互相污染。

当前池策略：可迁移连接满 55 分钟后不再接新 turn；严格续接可使用剩余窗口，但满 60 分钟也不能接新 turn。排队及健康检查后再次检查年龄；不为轮换强杀正在使用的连接。

换连接前必须证明回放历史完整、自包含，才能去掉 previous_response_id。item_reference、缺少对应调用的工具结果、历史采集不完整等不能当作完整历史。已输出内容后不能透明重放；当前寿命恢复采用有界尝试，并保留无法证明可重放的 continuation。默认 30 秒预算从第一次 WS 尝试开始计算，约束下一次尝试的启动，不截断已开始的流。

legacy sticky TTL 已恢复一小时，配置覆盖和旧 hash 键的十分钟上限继续有效。它是账号亲和 TTL，与 WS socket 寿命不是一个计时器。OAuth 到期刷新、握手/HTTP 401 的一次原账号恢复也复用既有实现，不在新入口另写无限重试。

### 4.8 探针用于观察生产路径，但不等于生产请求

ModelTrace 使用进程内网关入口，钉住目标账号，走中间件和正式转发；effort probe 保留用于触发校验拒绝的特制 payload，不进入会改写 effort 的业务流程。HTTP probe 走 `doOpenAIUpstream`，WS probe 跟随 resolver、使用独立连接，不占用业务连接池；插件接管及 HTTP 账号保持其通道。

现有 `TestWSProbeProductionHandshakeIdentity` 比较授权、UA/originator/version、Beta、account-id、session/thread 和能力头，覆盖身份强制开关、账号 UA 与 shadow。它不是所有指纹模式、所有握手字段和线上配置的完全一致证明。

error 与 metadata 到达顺序不能假定。当前 WS probe 对 400 终止结果最多再读 8 帧/250 ms，仍在原 15 秒预算内；读取失败保留已有判定。回退 HTTP 时清除 WS 观测，每模型分别记 transport；缺失 sb 不能写成 false，也不为补字段再建请求。

effort probe 的判据限于已开放的 astra 范围；sb/faster-model/turn-state 长度是观测值。ModelTrace 模型识别、effort 判读和客户端指纹对齐是三件事。打票链已删除，不能因历史 ticket 命名重新恢复收割或注入。

## 5. 当前仍存的差异与维护风险

2026-10-21 的[新版兼容性审查](codex-cli-post-0.159.3-compatibility-review.md)讨论到 0.163.0；下面状态已按本机 0.164.0 工作区更新。完整差异和验证见[本次记录](codex-cli-0.164.0-alignment.md)。

| 主题 | 当前状态 | 仍需注意 |
| --- | --- | --- |
| Lite WS 即时中断 | ctx_pool 生成期间转发 interrupt，处理确认与完成的两种顺序，保留结算及续接 | HTTP bridge 只能及时报告 unsupported_operation 并读完响应，不具备真正中断能力 |
| 新版 instructions | 普通推理迁为 developer input；Lite 不保留空 instructions 或空工具 carrier；原生 ID 保留 | 官方默认增量模式使用历史项 v7，非增量模式的临时前缀用 v5；网关缺失 ID 的 v5 合成仅是兼容策略，不是默认历史分配的完整复刻 |
| Retry-After | SSE/WS/HTTP 建议接入同账号及普通 WS 重连等待，使用绝对时刻并受原预算约束 | 不扩展重试次数；无建议容量错误仍有旧客户端兼容改写 |
| 请求字节与元数据 | 合成 WS 顶层顺序已修正；未知 metadata 与原生历史来源保留 | 工具嵌套字节、TLS、HTTP2 和完整 CLI 状态机未复刻 |
| 运行配置 | 内置版本 0.164.0；手工和同步值继续优先 | 自动版本仍可能高于已验证行为；没有读取线上设置或实际账号抓包 |

源码注释中“旧客户端被优先降载”“某个字段会触发识别”等表述，不自动构成上游因果证据。改动可以减少格式差异；是否影响降智、缓存或封禁，需要独立实验，不能靠成功一次、sb 单字段或本地单测下结论。

## 6. 如何验证后续改动

选择与改动相关的现有测试，先覆盖真实失败路径，再补最小回归。不要为了文档总结重跑整个仓库，也不要只测试 helper 而漏掉最后的发送结果。

| 验证意图 | 现有测试入口 |
| --- | --- |
| 身份来源一致、开关/UA 覆盖、切号不残留 | [identity](../internal/service/openai_codex_identity_test.go)、[account identity](../internal/service/openai_codex_account_identity_test.go)、[fingerprint](../internal/service/openai_codex_fingerprint_test.go) |
| v7 版本/时间、fallback 对话区分 | [identity drift](../internal/service/openai_codex_identity_drift_test.go) |
| 字段顺序、转义、精度和 metadata 往返 | [turn metadata](../internal/service/openai_codex_turn_metadata_test.go)、[roundtrip](../internal/service/openai_codex_turn_metadata_roundtrip_test.go)、[body order](../internal/service/openai_codex_body_key_order_test.go) |
| 最终 WS 握手与 create 帧，而非中间 map | [WS socket](../internal/service/openai_codex_ws_socket_test.go) |
| Lite/compact 输入语义、实际 zstd 字节与重放 | [Lite](../internal/service/openai_responses_lite_tools_test.go)、[request contract](../internal/service/openai_codex_request_contract_test.go)、[compression wire](../internal/repository/openai_request_compression_test.go) |
| 已提交 turn-state 来源、流输出/终止边界 | [turn-state](../internal/service/openai_codex_turn_state_test.go)、[preoutput wire](../internal/service/openai_responses_preoutput_wire_test.go) |
| 生命周期、回放完整性、粘性续期 | [WS lifetime](../internal/service/openai_ws_lifetime_test.go)、[legacy scheduler](../internal/service/openai_legacy_scheduler_decision_test.go)、[sticky compatibility](../internal/service/openai_sticky_compat_test.go) |
| 探针与生产握手、metadata 顺序、混合通道不污染 | [WS probe](../internal/service/codex_effort_ws_probe_test.go) |

例如，仅修改 metadata/身份/序列化时，可在 `backend` 目录选择：

```bash
go test -tags=unit ./internal/service -run 'Codex|RestoreOpenAIUpstreamJSONTopLevelOrder' -count=1
go test -tags=unit ./internal/repository -run 'CodexResponsesCompressionWireContract|HTTPUpstreamSuite' -count=1
```

涉及 WS 寿命、探针或流收尾时，另选对应测试；这组不会代替账号 TTL、OAuth 刷新或前端用例：

```bash
go test -tags=unit ./internal/service -run 'Lifetime|RetryTotalBudgetIncludesFirstAttempt|WSProbe|OpenAIResponsesPreoutputWire' -count=1
```

确认日志中目标用例确实执行，`no tests to run` 不算通过。涉及配置、handler、DTO、数据库或前端时，沿调用链补各层已有测试；并发池改动再跑相关 race 用例。未知耗时或预计超过十分钟的构建/测试按项目规则使用持久进程监管，不放在 PTY 会话中。

若交付声称“与某个官方版本一致”，还应记录具体官方 tag/commit、模式与开关，针对同一场景比较脱敏后的最终请求。保留字段关系和序列化证据，不把 token、原始 turn-state 或完整用户会话写入文档。最终发送 mock 能验证应用格式；TLS/HTTP2 一致性须另有网络层测量。

## 7. 验证范围

本指南最初只做代码分析，之后已在本机继续实施 0.164.0 对齐。验证使用缓存 Go 1.27.0、无外网命名空间和只读源码，真实账号目录不可见；包含本地 HTTP/WS mock 与已安装 CLI 的假凭据请求。测试结果、已确认的基线失败、监管单元和命令以 [0.164.0 对齐记录](codex-cli-0.164.0-alignment.md)为准。

没有部署 ccvps、修改线上开关、使用真实账号或验证上游识别效果。测试通过只能支持所覆盖的格式和控制流，不能推导为全指纹等价。

深入阅读：[客户端指纹实现说明](codex-client-fingerprint-simulation.md)、[WS 连接寿命任务与记录](openai-ws-connection-lifetime-fix-plan.md)、[legacy sticky TTL 任务与记录](openai-legacy-sticky-ttl-restore-task.md)、[打票残留清理记录](codex-ticket-remnants-map.md)。这些材料分别描述实现或历史工作；维护行为以当前调用链和相应回归为准。
