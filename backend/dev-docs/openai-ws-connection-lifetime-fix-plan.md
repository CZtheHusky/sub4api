# R5 WebSocket 连接寿命与恢复修复计划

日期：2026-10-21。状态：代码修复、定向验证和 ccvps 边界冒烟完成；全量检查未全部通过，尚未发布验收。

源码核对基准：`0036d3905640463366f28ae88935f43fb01283d8`。最近一次生产核验截至 2026-10-21 03:11 PDT，ccvps 当时运行 `a6c17dc1231b204b9823b5df775eaa8d7ba7dc0a`。实施前核对这两个版本之间及之后的相关变更。

本计划落实[缓存异常报告的 R5](/home/caozhe/workspace/sub4api/backend/dev-docs/cache-anomaly-assessment.md:111)：减少因复用临近寿命上限的 WS 连接而产生的请求失败，并让满足安全重放条件的请求有机会恢复。主要措施为连接年龄准入、原生 WS 轮次边界检查、受限恢复和重试预算调整。

## 一 修复依据

以下时间均为洛杉矶时间 PDT，来自对 ccvps 的只读日志核验。实际可用 journal 约 10.5 小时，不是完整 30 天。

| 事件 | 已核实事实 | 修复含义 |
|---|---|---|
| 10/20 22:15，requestID `c5d43ffa-3a7e-484b-b36f-7a708452802b` | 连接 `oa_ws_1_6` 在租借中关闭，`age_ms=3600408`；上游明确返回 60 分钟寿命错误；首尝试耗时 15915ms，重试预算 5000ms，最终 HTTP 502 | 年龄与预算问题已经共同造成客户端失败 |
| 10/21 00:34，requestID `3d6f2fd5-4c5a-4233-bc25-2f2716b31015` | 当前部署进程内再次出现同一寿命错误；转发返回 `openai ws error event`，HTTP 状态已为 200，流内结束为错误 | 需要发送前预防；已输出的请求不能靠透明重发挽回 |

第二个事件的 `can_fallback=true` 与 v2 返回分支相结合，说明该次没有进入“未输出时返回可恢复错误”的路径。单独的 `upstream_error_response_already_written=false` 不是“客户端未收到任何字节”的证据。

两次故障都发生在 HTTP 入口使用 WS v2 的路径。原生 WS ingress 的跨轮租约检查是同一连接寿命问题的必要覆盖项，目前没有将上述两次事件归因于 ingress。

## 二 目标与范围

交付必须满足以下行为：

1. 可换连的新请求不再被分配到已进入轮换区间的旧连接；排队后拿到连接时仍复查年龄。
2. 原生 WS 长会话在每轮发送前检查当前连接年龄，能够安全重建上下文时在轮次之间换连。
3. 到期错误在尚未向客户端输出、且重放条件成立时，可以使用新连接进行有限恢复。
4. 已输出、上下文不完整、工具调用无法配对、请求取消等场景不会重复输出或静默丢失上下文。
5. 重试不会增加重复结算，不改变 WS 失败后不自动回退 HTTP 的现有行为。

本次只修改 R5 所需的连接池、两个 WS 转发入口、重放资格判断、预算配置、关联日志与测试。不扩展为 R1/R2 租户隔离、R4 任意迟到帧治理或额度缓存改造。

五分钟轮换余量和三十秒重试窗口是本计划的初始设计值，不是已测得的最优值。五分钟余量不能保证一个持续超过剩余连接寿命的请求成功；也不能保证无法重建的连接内增量会话跨过上游硬上限。此类限制必须在验收与发布记录中保留。

## 三 现有路径和实现约束

| 位置 | 当前行为 | 本次要补的部分 |
|---|---|---|
| [连接池](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool.go:1137) | 后台和 acquire 均可清理；preferred、普通选择、容量等待有多条分支；已租借和 pin 的连接存在清理例外 | 统一年龄选择规则及拿到租约后的复查，覆盖所有分支 |
| [v2 转发](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_v2.go:158) | response/session 绑定影响 preferred conn；寿命错误已可分类，错误连接会 MarkBroken | 将连接依赖语义传给池；到期恢复明确取新连接；保护 previous_response_id |
| [HTTP 入口重试循环](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_forward.go:949) | 预算包含首轮耗时；已写客户端响应则停止重试 | 调整默认预算并明确口径，保留输出与取消边界 |
| [原生 ingress](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress.go:1711) | 租约跨轮持有；已有部分 ping 失败恢复，但带常驻 reader 的连接不走同一 ping 检查 | 年龄检查独立于 ping 能力；到期错误在允许恢复时先交给恢复逻辑 |
| [重放辅助函数](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_payload.go:785) | 构造 input 序列及工具配对；存在 input 不代表历史完整 | 明确“数据存在”与“覆盖原续接上下文”的区别 |

必须注意两个实现细节：

- `AgeBefore()` 是租借时快照，不能用于一个持有数十分钟的 ingress 租约。轮次检查应读取连接当前年龄，复用现有 `createdAt`/`age(now)` 数据。
- ingress 当前通过 `replayCollector.Items()` 保存上游重放项，主要覆盖工具调用；`AllItems()` 虽然存在，也不能在未检查采集完整性的情况下直接视为完整会话。首帧已带 previous_response_id 时，网关可能从未见过此前历史。

## 四 行为方案

### 4.1 连接年龄准入

保留 60 分钟最大年龄，增加一个内部轮换余量常量，初值 5 分钟；可换连请求的软轮换阈值为 55 分钟。本次不新增运行时开关或账号级配置。

| 连接与请求状态 | 行为 |
|---|---|
| 年龄小于 55 分钟，连接健康 | 沿用现有复用与亲和规则 |
| 年龄达到 55 分钟，请求可安全换连 | 不再选择该连接；使用较年轻的兼容连接或新拨号连接 |
| 排队前未到阈值，拿到租约时已到阈值 | 发送前重新判断，释放本次租约并重选；等待、重选均受原取消与获取超时约束 |
| 严格依赖原连接且无法重建历史，年龄在 55 至 60 分钟之间 | 保持原连接，不因软轮换阈值提前中断；记录未轮换原因，明确仍可能遇到硬上限 |
| 年龄达到 60 分钟或已收到寿命错误 | 不再在该连接上发送新请求；可恢复则换连，不可恢复则明确返回续接不可用 |
| 请求已经在飞或已经输出 | 不由后台因软轮换强行断流；若上游到期终止，按输出状态结束，不重放已输出部分 |

池只判断连接年龄与调用方声明的连接依赖，不解析业务 payload。优先复用 `ForcePreferredConn` 表达严格亲和；必须同步检查 v2 的调用方，因为当前 v2 并未为所有依赖连接的续接设置该字段，不能把默认 false 理解为可自由迁移。

普通选择、preferred 快路径、routing affinity、容量等待、强制 preferred 等路径必须使用一致规则。等待成功后和实际发送前需要重新判断，不能只在选候选连接时判断一次。池锁内不执行阻塞关闭，不抢占其他请求持有的租约，不突破账号连接上限；已有容量不足行为保持可取消且有界。

软轮换不删除其他会话的绑定。对本次已淘汰连接清理或更新相关连接指向时，必须确认仍指向该旧连接，避免覆盖另一轮刚写入的新绑定；如当前接口不能原子地做到这一点，先让旧指向自然走“连接不可用”，不要增加跨用户遍历清理。

### 4.2 安全换连与重放资格

到期恢复只依赖当前请求和网关已掌握的历史，不增加上游探测或补历史请求，不推断 `store=false` 等于无状态。

| 请求类型 | 到期后的处理 |
|---|---|
| 无 previous_response_id 的独立请求，工具结果均有可用配对上下文 | 保留 payload 语义，允许换连；错误后重放仍要求尚未输出 |
| 使用服务端存储续接的请求 | 新连接保留原 previous_response_id；若上游不接受该锚点，只有完整历史已获证明时才允许改为完整 create |
| store=false 增量续接，当前历史覆盖原链且工具配对完整 | 构造完整 create，再去掉 previous_response_id，允许换连 |
| store=false 增量续接，首帧从外部 responseID 开始或历史覆盖无法确认 | 不删锚点、不猜历史；软阈值保留严格亲和，硬到期后沿用明确的续接不可用处理 |
| 任一请求已向客户端输出本轮事件 | 不重发这一轮；保留已有错误呈现，避免追加重复终止事件 |

完整历史的证明至少包含：从已知 root 开始、后续锚点属于已采集链、必要的上一轮输出已保存、工具结果均能匹配调用、上下文窗口切换正确处理。input 非空、没有 function_call_output、模型相同，都不足以证明完整。

实现优先复用现有 input 合并和工具配对函数；如需补充完整性状态，只在当前会话内维护必要状态，不新建历史缓存或通用重放框架。若现有 collector 丢失了迁移所需的 assistant/reasoning 输出，要么在既有内存与载荷约束内补齐采集并验证，要么将该会话标为不可迁移；不得将工具项集合充当整段历史。

本次新增的寿命恢复必须阻止后续通用 previous_response_not_found 分支绕过完整性条件。不能先在到期分支保留锚点，再由下一次重试无条件删掉它。

### 4.3 两个入口分别接入

**HTTP 入口使用 WS v2。** 在发送 create 前应用年龄规则；寿命错误发生后，复用现有分类和 MarkBroken，在重放资格成立时明确要求新连接。沿用最多五次重试、共六次尝试的已有上限，不增加额外的“寿命专用重试次数”。取消、预算和输出状态共同限制是否进入下一次尝试。

**原生 WS ingress。** 在当前轮 payload、续接锚点与 replay 信息确定之后、发送 create 之前，独立执行年龄检查，不挂在 `SupportsIdlePingWithoutReader()` 的条件下。可迁移时先准备完整且可验证的重放请求，再释放旧租约、解除旧 pin、获取新连接并更新本会话绑定。

ingress 当前通用重试主要处理读写错误，严格亲和会跳过。需要将“未输出且可恢复的寿命 error 帧”在发给客户端之前交给轮次恢复；只增加明确寿命错误的处理，不放宽所有 error 帧。沿用每轮最多一次恢复重试的边界，确保寿命、读写和历史恢复组合不会为同一轮重复增加次数；已输出则结束。

轮换属于同一逻辑请求或同一轮，不能重复执行准入、BeforeTurn/AfterTurn 或结算。仅对成功完成的结果维持原计费流程；不为本次修复新增“失败不计费”的笼统规则。

### 4.4 重试预算

采用以下单一口径，避免两种预算模型混用：

- 保留从首次 WS 尝试开始计时的现有行为。
- 默认 `gateway.openai_ws.retry_total_budget_ms` 从 5000 改为 30000，示例配置与字段注释同步修改。
- 该值限制“是否继续启动下一次尝试”，计入首轮耗时和退避；不是强制截断已开始响应的总墙钟时长。已启动尝试仍受请求 context、拨号、获取、写入和读取超时约束。
- 不从首次失败重新起表，不因一次重试重置窗口，不创建三十秒到期就切断流的 context。
- 保留显式配置覆盖、0 关闭该预算限制、最大尝试次数和退避规则。部署配置若显式写着 5000，改默认值不会改变线上有效值，发布时必须核对。

三十秒为 15.9 秒首尝试后提供进入重试的机会，不承诺所有代理链路都能在三十秒内完成。ingress 沿用每轮的次数和 I/O 超时边界，本次不为整个长会话套用 v2 的请求级预算。

### 4.5 观测

在现有 WS 日志和重试统计中补充足以验证 R5 的信息，不记录 payload、凭据或模型输出：

- 路径（v2/ingress）、requestID 或客户端连接与 turn、账号、旧新 connID。
- 选中及发送前的连接年龄、软轮换阈值、是否复用、严格亲和状态。
- 本轮是否已输出、是否允许迁移、跳过原因、重试次数、累计耗时及预算。
- 轮换成功、硬到期且无法恢复、已输出后到期三种结果可区分。

connID 可能在进程重启后重复，排查时与进程 PID/启动时间及部署版本关联。原来的 `upstream_error_response_already_written` 继续表达其原有含义，不能拿来代替本轮输出状态。

## 五 实施任务

| 任务 | 内容 | 完成条件 | 依赖 |
|---|---|---|---|
| T1 复现与边界 | 固化两类生产故障，核对 root、增量、工具续接的可迁移条件 | 可控测试分别重现“首轮耗尽预算”和“已输出后寿命终止”；记录安全恢复与拒绝恢复场景 | 无 |
| T2 连接池年龄规则 | 候选选择、preferred、等待后复查、容量与释放处理 | 所有可换连 acquire 路径均不交付达到软阈值的连接；严格亲和例外及硬阈值明确，租约和 pin 无泄漏 | T1 |
| T3 ingress 轮次轮换 | 当前年龄检查、历史完整性、轮换与寿命错误恢复 | root 与完整历史能续接；不完整历史不会被删除锚点；reader-loop 连接同样覆盖；每轮恢复有界 | T1、T2 |
| T4 v2 恢复和预算 | 声明连接依赖、寿命错误新连、预算默认值及配置说明 | 15.9 秒首尝试后允许预算内安全重试；已输出不重试；0 和显式配置兼容；没有 HTTP 自动回退 | T1、T2 |
| T5 回归与观测 | 补日志并覆盖取消、并发池容量、工具链、结算 | 第六节全部适用场景通过；日志能区分预轮换、恢复和不可恢复；检查结果可复查 | T2—T4 |
| T6 发布验证 | 核对生产有效配置、版本、上线观察与回退 | 真实流量跨越连接寿命边界后，按第七节记录结果；无静默上下文缺失和重复输出 | T5 |

主要文件：[连接池](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool.go)、[v2](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_v2.go)、[HTTP 入口循环](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_forward.go)、[ingress](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress.go)、[ingress 错误分类](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder.go)、[payload 辅助函数](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_payload.go)、[配置](/home/caozhe/workspace/sub4api/backend/internal/config/config.go:1339)、[示例配置](/home/caozhe/workspace/sub4api/deploy/config.example.yaml:395)。只有历史完整性验证需要时，才修改[现有 collector](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_http_bridge.go:260)。

## 六 验收矩阵与检查

测试用受控连接建立时间和可编排的上游事件，不真实等待 55 或 60 分钟，不靠压力碰撞来制造时序。

| 场景 | 必须断言的结果 |
|---|---|
| 55 分钟阈值前、等于阈值、阈值后 | 可换连请求的选择准确；等于阈值进入轮换 |
| 普通、preferred、routing affinity、容量等待 | 都遵循同一规则，不能漏过 preferred 快路径 |
| 等待期间跨越阈值 | 获取租约后重新选择，不向旧连接写 create |
| 满池，旧连接忙或被其他会话 pin | 不强关在飞连接、不超额拨号；等待与失败遵循原容量和取消语义 |
| lease 已持有数十分钟，reader loop 开启 | 使用当前年龄，在下一轮发送前检查，不依赖租借年龄快照或 ping 分支 |
| 可重放 root、完整多轮历史 | 轮换后请求语义、assistant 历史和工具上下文不丢失 |
| 工具结果完整配对／孤立工具结果 | 前者可恢复；后者不被伪装为独立完整 create |
| 首帧自外部 previous_response_id 开始 | 即使 input 非空也不误判完整；软阈值保留严格依赖，硬到期明确失败 |
| 上下文窗口变化、上一轮输出不完整 | 不拼接错误历史，不宣称可无损迁移 |
| 首次耗时相当于 15.9 秒、预算相当于 5/30 秒 | 小预算停止；新默认窗口内可启动安全重试；不重置计时起点 |
| 服务端存储续接换连后锚点失效 | 不由通用恢复分支绕过完整历史判断 |
| 寿命错误前已发送客户端事件 | 上游不收到重复 create，客户端不收到重放片段或重复终止事件 |
| 重试后再次到期、读写错误与历史恢复交织 | 遵守各入口最大尝试次数，不出现嵌套倍增 |
| 退避、获取连接或恢复期间取消 | 及时结束，归还租约和 pin，不额外启动尝试 |
| 成功恢复后的计费与 hooks | 一次逻辑请求只按现有约定结算一次，准入与轮次回调不重复 |
| 默认值、显式 5000/30000、0 | 配置语义明确，示例与实现一致 |

重点扩展现有测试文件：[池测试](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool_test.go)、[reader-loop 测试](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool_reader_loop_test.go)、[v2 协议与重试](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_protocol_forward_test.go:876)、[预算与分类](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_fallback_test.go:250)、[ingress 会话](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress_session_test.go)、[ingress 重试](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress_retry_test.go)、[工具重放](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_http_bridge_resume_test.go)。

旧测试名 `WSv2ConnectionLimitReachedRetryThenFallbackHTTP` 与实际“不回退 HTTP”的断言不符；修改该测试时同步纠正名称，不能按名称恢复已取消的 HTTP 回退。

实施后的检查顺序，在 backend 目录执行：

```bash
go test -tags=unit ./internal/service ./internal/config -run 'OpenAIWS|WSv2|ProxyResponsesWebSocket|RetryTotalBudget' -count=1
go test -race -tags=unit ./internal/service -run 'OpenAIWSConnPool|WSv2|ProxyResponsesWebSocket' -count=1
go test -tags=unit ./...
go test -tags=integration ./...
golangci-lint run ./...
```

新增测试名称必须被定向过滤器覆盖，并核对实际运行列表。按仓库 CI 要求使用 Go 1.27.0；集成依赖不可用时明确记录未执行项，不声称全部通过。耗时无法可靠预估或可能超过十分钟的检查使用持久监管器运行，优先 `systemd-run --user`，不可用再用 tmux 或 setsid/nohup，记录命令、单元、日志与输出位置；不使用 PTY 承载长任务。

## 七 发布与关闭条件

1. 发布前记录待发布 SHA、当前线上版本、实际生效的重试预算及现有延迟/连接创建数据。代码默认值和部署显式值一起核对。
2. 在测试环境验证新旧连接轮换和状态完整性后，再安排 ccvps 发布。单实例部署如需重启，应明确这是一次会中断现有 WS 的发布，重启本身不能当成修复验证。
3. 发布后至少观察一个完整连接寿命周期及后续轮次，建议 75 分钟以上；必须确实有连接接近阈值并继续承载请求。只看到“重启后暂时没有寿命错误”不算通过。
4. 按 requestID、进程和 connID 关联检查轮换、恢复、最终结果；对比首 token 延迟、连接创建、容量失败及寿命错误。任何重复输出、工具上下文丢失、错误完整重放或结算重复，立即停止推广并回退本次版本及相关预算配置。
5. 预算放大若使失败请求等待显著增加，先核对是否只有安全重试获得了新窗口；回退预算不能替代对年龄准入的验收。回退不清空共享 Redis，不更改账号传输模式。

R5 可关闭的条件是：可安全迁移的请求不会因已知年龄选择缺口被送入到期连接；输出前的寿命错误按规则有界恢复；输出后及历史不足的情况行为明确；矩阵、必要回归和上线观察有记录。无法迁移的增量会话及超过剩余寿命的在飞请求仍可能遇到上游硬限制，不能承诺所有 60 分钟相关错误归零。

原计划交付时尚未实施；本次实施和验证结果见下节。


## 八 实施与验证记录（2026-10-21，PDT）

源码基准 `0036d3905`。本次未提交、未发布；T6 和 R5 关闭条件尚未完成。工作区另外两份研究文档及模型目录的两份外部修改不属于本任务。

### 已实施

- 池的 ordinary、preferred、routing、容量等待、strict 及获取租约后复查统一应用 55/60 分钟准入规则；不因软轮换关闭其他会话 pin 或在飞连接，不突破容量。
- v2 声明严格续接依赖并在发送前复查年龄；寿命恢复明确使用新连接，保留无法证明完整的续接锚点，阻止随后通用恢复悄悄删锚点。
- ingress 持有跨轮租约时读取当前年龄，独立于 ping；保存完整 assistant/reasoning 输出并维护已知 root、锚点和窗口完整性。缺失终态 output、reasoning 密文、未知 hosted-tool 状态、工具未配对或历史清洗省略项时不宣称可完整迁移。
- 未输出的寿命错误参与每轮一次的恢复预算；输出后不重放。重试不重复准入和轮次回调，保持 WS 不自动退回 HTTP。
- 默认重试预算 30000ms，从首次尝试开始计入首轮和退避，保留 0 和显式覆盖。同步配置、示例和日志。
- 旧测试中“删除未知外部锚点后重放部分历史”与新要求冲突，改为保留锚点、不可用时明确失败。

### 本机验证

正式检查使用 Go 1.27.0、golangci-lint 2.13.2，全部通过用户级 systemd 托管，未使用 PTY。结果目录 `/tmp/sub4api-ws-lifetime-20261021`。

| 检查 | 实际结果 | 记录 |
|---|---|---|
| 修复前年龄缺口复现 | 确认 ordinary/preferred/routing 放过 55 分钟连接、strict 放过 60 分钟连接 | `reproduce.log`；该初次复现使用既有 Go 1.27.1 |
| 最终定向 unit | 693 项通过，无跳过 | `final-targeted.jsonl` |
| 最终定向 race | 238 项通过，无跳过 | `final-race.jsonl` |
| 全量 unit | 22557 项通过、4 个用例失败、17 个跳过；整体失败 | `unit.jsonl` |
| 全量 integration | 14133 项通过、1 个用例失败、16 个跳过；整体失败 | `integration.jsonl` |
| 构建全部后端包 | 通过 | `final-build.log` |
| 全量 lint | 57 项问题，整体失败；本次修改文件无 lint 报错 | `final-lint.log` |

全量 unit 的三个失败已在未修改的 HEAD 覆盖视图单独复现：

- `TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_NoAvailableAccount`（integration 中也是这一项失败）。
- `TestResponsesSearchBuiltinTranslation`。
- `TestResponsesSearchAdditionalToolsAndStringNamesAvoidCollisions`。

第四项 `TestOpenAIGatewayService_PrewarmReadHonorsParentContext` 在并行编译时超过 180ms 时限，单独连续 10 次均通过（约 40ms/次），保留原失败，不把全量结果写为通过。17/16 个跳过项主要为环境开关控制的实时账号、TLS 捕获和审计集成等；完整名单在 `checks-summary.json`，未执行的检查不计为通过。

命令和监管记录：`validate.sh` / `resumed-validation.log`；最终串行检查 `final-scoped.sh` / `final-scoped.log`，单元 `sub4api-ws-lifetime-final-scoped-20261021`；覆盖视图和基线结果 `baseline/overlay.json`、`baseline.log`、`baseline-more.log`；取消时限重复验证 `prewarm-repeat.log`。编译整理期间出现过未使用变量和重复函数声明，已修复；失败编译不计作有效验证。

### ccvps 真实账号验证

05:13 核实线上仍为 `a6c17dc1231b204b9823b5df775eaa8d7ba7dc0a`，可用空间约 2.2 GB；原测试目录不存在。未重启线上服务、未修改生产配置、Redis 或账号模式。

仅上传编译程序，数据库连接使用 `default_transaction_read_only=on`，测试单元限制 192 MB、50% CPU、240 秒。源码、依赖、凭据未上传。验证结束后已取回脱敏记录并清理临时目录 `/var/tmp/sub4api-ws-lifetime-20261021`；旧目录 `/var/tmp/sub4api-ws-probe-20261020` 也不存在。

- 账号 8 首次返回 `usage_limit_reached`，未对其持续重试；后续使用可用账号 1。
- HTTP v2 已以实际账号验证：两次独立请求、两条上游连接；受控地将首连接建立时间设为 55 分钟前，第二次没有复用。
- 原生 WS 最初的标记断言失败已定位为测试载荷缺少稳定窗口元数据；日志明确记录 `context_window_changed` 和按已有规则删除锚点。只固定握手头仍不足，归一化读取请求体的 metadata。前三轮诊断失败均保留，没有计为通过；最终改为复用现有 `buildCodexReplayRequest` 同时构造请求体和握手标识。
- 最终真实验证单元 `sub4api-ws-lifetime-real-protocol-20261021` 返回 `Result=success`、退出码 0。账号 1 原生 WS 三轮、三条连接，在受控 55/60 分钟边界换连后，两次都在流和终态中找到原随机上下文标记；HTTP v2 两轮、两条连接也通过。整组约 11.48 秒。
- 这是模拟连接建立时间的实际账号边界冒烟，不是等待 55/60 分钟或 75 分钟线上观察。未把“换连”或“重启后暂时无错误”当作上线验收。
- 脱敏结果保存于本机 `/tmp/sub4api-ws-lifetime-20261021/real-ccvps-results.json`（包含前述失败和最终成功）；源测试覆盖视图 `real-overlay.json` / `real_account_test.go`，编译与上传完整命令分别在 `build-real.sh`、`upload-real-protocol.sh`，本机上传监管单元 `sub4api-ws-lifetime-upload-real-protocol-20261021`。
- 清理后 ccvps 可用空间约 2.2 GB；线上 `sub2api` 仍为 active，PID `1493590`，启动时间仍为 10/20 23:24:25 PDT，未被本任务重启。

### 剩余发布验收

T6 未执行。需要先处理或明确接受全量检查的既有失败，再确定发布版本和单实例重启窗口；重启会中断现有 WS。发布后仍须有实际连接经历阈值并继续请求的至少 75 分钟观察。本次不能据重启、模拟建立时间或短时无错误关闭 R5。
