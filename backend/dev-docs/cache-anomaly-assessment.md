# 缓存异常风险分析报告

更新日期：2026-10-21。源码基准：`a6c17dc1231b204b9823b5df775eaa8d7ba7dc0a`。

本报告面向故障排查和修复排期，覆盖 OpenAI 会话状态、WS 连接复用、余额与额度缓存、认证缓存，以及相关异步持久化路径。按多用户共享上游账号、同一用户可能并发请求的实际使用方式分析。

当前最值得处理的是三类问题：**会话状态缺少可靠的归属或分支约束、旧快照覆盖缓存更新、异步更新或持久化中断**。这些路径能分别造成上下文串线、误放行或误拒绝、额度显示与数据库不一致。不能用 Redis 实例健康、单条 Lua 原子执行或 `store=false` 概括为“缓存体系没有问题”。

本报告依据源码及已有测试文本；没有执行新的并发复现，也没有重新查询生产日志、数据库或 Redis。原记录中的生产观测单列于第五节，不等同于本轮独立验证。

## 一 异常范围与证据口径

“缓存异常”需要先区分所处层次，避免把不同现象归为同一个根因。

| 层次 | 典型状态 | 用户可见现象 |
|---|---|---|
| 会话与续接状态 | responseID、会话绑定、turn-state、digest 分支 | 串上下文、沿用旧回答、工具续接失败 |
| 连接池 | 连接年龄、握手身份、待读帧、连接租约 | 偶发失败、错误重试、跨轮帧污染 |
| 准入与计费用量视图 | 余额、订阅、API key 用量、倍率 | 充值后仍拒绝、额度耗尽后继续放行、倍率变更延迟 |
| 缓存更新与持久化 | 失效 outbox、异步写队列、配额 flusher | 旧值重新出现、Redis 与 DB 偏差 |

上游 prompt cache 的命中率、缓存 token 统计与上述状态缓存有关联，但不是同一件事。命中率下降本身不能证明串号；账单中的缓存 token 比普通输入 token 多，也不能直接判定重复计费。

证据采用以下口径：

- **代码可确认**：已定位到实际调用链和具体交错顺序；不代表生产已发生。
- **条件性风险**：代码存在缺口，但结果取决于上游行为、连接复用条件或部署配置。
- **待核验观测**：来自原记录，尚缺请求级关联、精确部署版本或独立复现。
- **不成立或需收窄**：原归因与现有代码不符，或证据不足以支持原结论。

优先级表示下一步处理顺序：P1 优先验证租户边界或修复已能构造的错误结果；P2 纳入后续修复与验证；P3 先明确生效约定或确认部署条件。优先级不代表生产发生频率。

## 二 风险总览

| 编号 | 问题 | 证据 | 优先级 | 原条目 |
|---|---|---|---|---|
| R1 | WS 续接缺少下游用户归属校验 | 校验缺口可确认，跨用户上下文复用待复现 | P1 验证 | B4、C10、C11 |
| R2 | 内容种子被用于跨会话共享状态 | 共享键及读取路径可确认 | P1 验证 | B5 |
| R3 | digest 并发分支共用可变 responseID | 代码可确认 | P1 | B2 |
| R4 | WS 迟到帧跨轮或跨重试污染 | 条件性风险 | P1 验证 | B1、C12 |
| R5 | 临近寿命上限的连接复用及重试预算不足 | 代码可确认，原日志支持预算耗尽 | P2 | A1 |
| R6 | 余额和订阅旧快照覆盖失效或用量更新 | 代码可确认 | P1 | B3、C2 |
| R7 | API key 限额用量被旧回填覆盖或漏增 | 代码可确认 | P2 | C1、C3 |
| R8 | 认证旧快照回填与 RPM 失效覆盖缺口 | 代码可确认，持续时间取决于失效与配置 | P2 | C5、C6 的 RPM 部分 |
| R9 | 用户专属倍率变更依赖本地缓存过期 | 生效延迟可确认，是否违约取决于产品约定 | P3 | C6 的倍率部分 |
| R10 | 平台配额 flusher 丢失待刷写标记 | 启用 flusher 时的条件性风险 | P2 若启用 | C4 |

## 三 会话状态和连接复用

### R1 WS 续接缺少用户归属校验

**触发条件。** 用户 B 已知用户 A 的 responseID，二者处于可路由到同一上游账号的分组；目标连接仍存在且允许复用。responseID 是否可被另一用户获得、上游是否接受该续接，需要独立验证，不能假定 ID 可猜。

**机制与影响。** `responseID → connID` 绑定不含下游用户维度，ingress 按客户端提交的 `previous_response_id` 查找连接。HTTP 续接有用户归属校验，WS 路径未使用相同校验。指纹模式 off 时，握手兼容性主要依据 beta features；device 模式也没有完整会话身份约束。会话抢占清除 session 绑定，不等于撤销所有 response 绑定。

`store=false` 不能证明安全：代码本身把 `store=false + previous_response_id` 作为需要连接亲和性的续接场景。若同一连接保留可续接状态，潜在影响就是跨用户使用上下文，而非仅丢上下文；实际读取到何种内容仍需状态化上游复现。开启存储恢复也不能替代下游授权。

**处理方向。** 先用两个独立认证用户验证边界，再复用现有 response owner 机制，为 WS 首轮和后续每轮绑定、校验归属。遵循现有 HTTP 的“同用户不同 key 可互通”语义，不应未经确认改成所有 key 之间绝对隔离。握手兼容性与连接绑定不能充当授权检查。

**验收。** A 正常结束后释放连接；B 提交 A 的 responseID，必须在发往上游前拒绝。同时验证同用户允许的续接、off/device 指纹模式、同一客户端连接中途换 responseID，以及原连接仍在池内的情况。

代码依据：[绑定存储](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_state_store.go:289)、[ingress 查找连接](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress.go:585)、[HTTP 归属规则](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_response_handling.go:1448)、[握手兼容性](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool.go:2372)。

### R2 内容种子被用于共享会话状态

**触发条件。** 两个用户属于同一分组，未提供显式 thread/session 身份，且模型、工具、指令和首条用户消息等内容种子相同。仅“首条消息相同”不是充分条件。

**机制与影响。** 显式 execution scope 包含 API key 维度，但身份缺失时返回空值；ingress 随后沿用纯内容生成的 sessionHash。内容种子适合做账号亲和，却被用于读取 session turn-state、preferred connection 和 invalid-encrypted 标记。

非 OAuth 路径可能继承另一会话的 turn-state；共享连接绑定可能指向另一会话的连接；共享失效密文标记可能导致匹配项被剥离。共享键可由代码确认，是否进一步产生回答泄漏或上下文丢失依赖对应上游行为。HTTP bridge 明确跳过原生 turn-state 与连接绑定继承，但这不意味着所有使用 sessionHash 的状态都已隔离。

**处理方向。** 将账号亲和与私有会话状态的使用范围分开。没有可靠会话身份时，不跨客户端连接恢复私有 turn-state、连接续接状态或失效密文记录；需要恢复的场景复用现有可信身份。仅添加用户维度仍不能区分同一用户的两个独立会话。

**验收。** 同组不同用户使用完全相同种子、同用户建立两个无显式身份的独立会话，都不得继承对方状态；同一显式线程的正常重连仍符合现有约定。

代码依据：[身份作用域](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_execution_scope.go:145)、[内容回退与状态读取](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress.go:565)、[内容种子](/home/caozhe/workspace/sub4api/backend/internal/service/openai_content_session_seed.go)。

### R3 digest 并发分支覆盖续接锚点

**触发条件。** Anthropic 兼容入口使用支持自动续接的模型和 API key 类型上游账号；没有更高优先级的显式 prompt key、metadata session 或 cache-control 锚点，进入 digest 回退；同一下游 key 的两条分支并发命中同一历史前缀。

**机制与影响。** digest 前缀映射到 promptCacheKey，而 promptCacheKey 再映射到“最近完成的 responseID”。分支归属在第二次映射中丢失：

1. A、B 同时从公共前缀取得 K，分别发起请求。
2. A 完成，将 K 绑定到 RA；B 随后完成，将 K 改绑到 RB。
3. A 下一轮仍取得 K，读到 RB，并将 input 裁到最新一轮。
4. 上游沿 B 的上下文回答 A 的问题。

RB 可以一直有效，因此不能依赖 `previous_response_not_found` 自动恢复。digest 本身不包含 model/tools，也需要验证切换这些参数时续接是否符合预期。此问题不能称为“HTTP 唯一会错答的路径”。

**处理方向。** 续接锚点应属于具体历史分支，不能仅依赖共享 promptCacheKey 的最新值。最小止损方案是在无法证明分支一致时走现有完整重放，避免错误裁剪；不要只在最后写入处加锁，因为串行完成仍会覆盖另一分支。

**验收。** 固定 A/B 同时命中前缀，再分别控制两种完成顺序；A、B 后续请求各自使用正确父响应或完整重放。再覆盖模型和工具变更。

代码依据：[digest 前缀复用](/home/caozhe/workspace/sub4api/backend/internal/service/openai_messages_digest_session.go:47)、[共享 responseID](/home/caozhe/workspace/sub4api/backend/internal/service/openai_messages_continuation.go:163)、[输入裁剪](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_messages.go:179)、[完成后绑定](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_messages.go:537)。

### R4 迟到帧污染下一轮或字段修复重试

**触发条件。** 上游在终止事件之后仍发送数据，或字段拒绝事件后仍有后续帧；ingress 保留连接并开始下一轮或修复后的重试。

**机制与影响。** 连接池在重新租借时检查已经排队的脏帧，但 ingress 跨轮持有租约，不经过该检查。字段修复重试也直接继续使用原连接。旧 completed 帧可能被采纳为新一轮终止响应，旧 error 帧可能使新尝试误失败。

v2 的 `pendingJSONDocuments` 检查只覆盖同一条 WS message 内终止 JSON 后的尾部 JSON，不能证明任意稍后到达的独立帧都安全。单次检查“当前队列为空”也不能排除仍在飞行中的旧帧。

**处理方向。** 分别处理同消息多 JSON、轮次间迟到帧和拒绝后重试。能够识别归属的事件应归属到正确轮次；不能确认连接干净时，不在该连接上冒险重发。任何换连策略都要保留或明确处理 store=false 的续接上下文，不能用无条件换连掩盖丢上下文。

**验收。** 可控上游分别发送同消息尾部 JSON、跨消息延迟 completed/error、400 拒绝后的延迟帧。下一轮不得转发旧终止结果、绑定旧 responseID，或将旧错误算作本轮失败。

代码依据：[租借检查](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool.go:414)、[字段拒绝返回](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress.go:1092)、[原连接重试](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_ingress.go:1855)、[v2 检查范围](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_forwarder_v2.go:823)。

### R5 连接年龄与重试预算组合导致偶发失败

**机制。** 代码将最大连接年龄设为 60 分钟。年龄清理既在后台执行，也会在 acquire 中按间隔触发；已租借或被 pin 的连接存在跳过清理的情况。临近最大年龄的连接仍可能被用于新请求，跨轮长期持有的连接也不能仅靠 acquire 检查覆盖。

HTTP 入口走 WS v2 时，重试总预算从第一次尝试之前开始计算。首次尝试耗时超过默认 5 秒后，即使错误被分类为可重试，也可能直接耗尽预算。该预算路径与原生 WS ingress 的轮次重试应分别分析，不能混为一套机制。

**影响与边界。** 可表现为使用池化连接时偶发失败。原日志支持预算不足，不能单凭错误码证明连接刚好达到 60 分钟。是否可透明重放，必须检查客户端是否已收到输出、payload 是否完整，以及是否依赖连接内续接状态；不能从 `store=false` 推导“上下文全在 payload”。

**处理方向。** 分别验证临近到期连接的入场策略、长期持有连接的轮次边界处理、可重放请求的重试预算。预算调整应受请求总 deadline 约束，避免把一个偶发错误变成长时间挂起。年龄余量和预算调整不应写成“任选其一即可解决全部场景”。

**验收。** 覆盖临近到期的新租借、持有租约期间跨越年龄上限、首尝试超过预算、完整 payload 重放、仅增量续接、已有客户端输出及取消场景。

代码依据：[连接年龄与 acquire 清理](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool.go:1161)、[跳过清理条件](/home/caozhe/workspace/sub4api/backend/internal/service/openai_ws_pool.go:1645)、[v2 重试预算](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_forward.go:949)。

## 四 额度与认证缓存

### R6 余额和订阅旧快照重新进入缓存

**机制。** 余额、订阅 cache miss 后读 DB，再异步 SET/HSET。以下交错可将已经失效的数据重新写回：

1. 请求读取旧余额或旧订阅快照，回填尚未执行。
2. 管理操作提交充值、调额、撤销或窗口重置，并删除缓存。
3. 旧回填执行，覆盖删除结果。

旧快照还可能覆盖已经执行的缓存扣减或订阅用量增量。订阅缓存虽然保存 Version 字段，但当前 HSET 没有比较版本，命中路径也未用它阻止旧快照。

**影响。** 余额偏低造成充值后误拒绝；余额偏高造成错误放行。订阅撤销或窗口重置后，缓存检查可能仍使用旧状态、旧累计量。订阅缓存没有窗口起点，无法独立识别重置前的用量；最终请求是否放行，还取决于其他订阅检查。

**现有缓解与限制。** 队列满时余额扣减、订阅用量更新有同步回退；DB 余额低于准入阈值后也会主动失效缓存。这些缓解不消除迟到的旧回填。余额和订阅更新均续期 TTL，因此“五分钟 TTL”不是所有陈旧状态的持续时间上界。DB 扣款原子性保证的是扣账操作，余额不足时仍存在负余额扣款路径，不能据此宣称透支金额有固定上界。

**处理方向。** 让失效、回填和增量更新之间能够识别过期快照。单纯改同步 SET、给 SET 加锁、或只在 Lua 中比较当前缓存值，都不能自动处理 DB 与 Redis 之间的交错。先以实际失败顺序写测试，再选择能覆盖跨进程写入的最小方案；不要默认增加一套版本系统。

**验收。** 固定“旧 DB 读取 → 修改并失效 → 旧回填”和“旧读取 → 已提交用量更新 → 旧回填”的顺序。分别验证充值、降额、撤销、窗口重置，并在持续消费续期 TTL 的情况下检查恢复行为。

代码依据：[余额回填](/home/caozhe/workspace/sub4api/backend/internal/service/billing_cache_service.go:311)、[订阅回填](/home/caozhe/workspace/sub4api/backend/internal/service/billing_cache_service.go:415)、[订阅 HSET](/home/caozhe/workspace/sub4api/backend/internal/repository/billing_cache.go:223)、[更新续期 TTL](/home/caozhe/workspace/sub4api/backend/internal/repository/billing_cache.go:76)、[扣费后失效](/home/caozhe/workspace/sub4api/backend/internal/service/gateway_usage_billing.go:463)。

### R7 API key 限额缓存漏增或被旧回填覆盖

**机制。** 有三个独立入口：

- 多个 cache miss 读到旧 DB 快照；一个回填并执行了用量增量后，另一个全量 HSET 将其覆盖。
- 旧 DB 读取之后产生新的用量，但增量执行时缓存尚不存在，Lua 直接返回；随后旧快照回填，缺失这次用量。
- `QueueUpdateAPIKeyRateLimitUsage` 在队列满或关闭时未处理入队失败。存在节流告警和丢弃计数，不能称为完全静默，但没有补偿动作。

**影响。** 标准模式的缓存准入可能低估 5h/1d/7d 用量，允许超过额度继续请求。DB 已提交的用量不因缓存漏增而自动丢失；错误准入也不会因为 DB 正确而自动消失。窗口重置、失效和重新装载可能修复偏差，不能笼统写成“一定持续七天”。simple mode 的 DB 准入口径应单独验证。

**处理方向。** 将快照回填与增量更新放到同一一致性分析中；队列拒绝入队时，采用可验证的更新或失效处理。Redis 执行后响应丢失与明确未入队不同，不能把所有错误统一重试增量，避免重复累加。

**验收。** 精确安排两次 miss、一次已提交用量及回填先后顺序；注入队列满和关闭，检查下次准入不会继续读取偏低快照，并验证增量不重复。

代码依据：[准入与同步回填](/home/caozhe/workspace/sub4api/backend/internal/service/billing_cache_service.go:578)、[整量 HSET](/home/caozhe/workspace/sub4api/backend/internal/repository/billing_cache.go:330)、[缺 key 跳过增量](/home/caozhe/workspace/sub4api/backend/internal/repository/billing_cache.go:106)、[未处理入队失败](/home/caozhe/workspace/sub4api/backend/internal/service/billing_cache_service.go:691)、[丢弃告警](/home/caozhe/workspace/sub4api/backend/internal/service/billing_cache_service.go:267)。

### R8 认证旧快照回填与 RPM 变更失效缺口

**机制一。** DB 读取与缓存回填之间、L2 读取与 L1 回填之间，均可能穿插失效。outbox 和约 30 秒后二次删除能缓解，但没有使“读旧快照 → 迟到回填”不可能发生。尾部延迟包括 Redis、快照组装和调度等环节，不能限定为“只有 DB 查询超过 30 秒才漏”。快照常量版本 24 是结构版本，不是每次数据修改的版本。

**机制二。** `user_group_rates.rpm_override` 被写入认证快照。管理接口先提交配置，再调用应用层认证失效；现有迁移中未找到该表的失效触发器。如果进程在提交后、失效前终止，就没有该表对应的事务 outbox 事件补偿。

**影响。** 用户权限或 RPM 变更后可能暂时使用旧规则。持续时间取决于失效投递、缓存配置、TTL 抖动及回填时序，不能统一承诺 30 秒或 300 秒。

**处理方向。** 保留现有 outbox、二次删除和跨实例失效广播，优先补齐真实配置修改路径的事务性通知。对迟到回填，用受控延迟验证现有缓解的边界，再决定是否需要更强的写入条件。RPM 与计费倍率是两个读取路径，不能合并视为同一个认证快照字段。

**验收。** 暂停旧 DB/L2 读取结果，在第一次及第二次失效之后释放回填；记录最后一次失效到实际拒绝旧规则的时延。对 RPM 修改，在提交后、应用层失效前模拟中断，验证恢复后的规则。

代码依据：[认证读取和回填](/home/caozhe/workspace/sub4api/backend/internal/service/api_key_auth_cache_impl.go:201)、[二次失效](/home/caozhe/workspace/sub4api/backend/internal/service/auth_cache_invalidation_outbox.go:178)、[RPM 更新后失效](/home/caozhe/workspace/sub4api/backend/internal/service/admin_group.go:1296)、[现有事务触发器](/home/caozhe/workspace/sub4api/backend/migrations/184_auth_cache_invalidation_outbox.sql)。

### R9 用户专属倍率变更存在缓存生效延迟

**机制与影响。** 倍率由独立的 `userGroupRateResolver` 按 user/group 保存在进程内，默认 TTL 为 30 秒；管理接口批量更新倍率后直接返回，所核对路径没有向这些 resolver 广播失效。已有缓存会在过期前继续提供旧倍率，多实例还可能在不同时间生效。这会影响按倍率计算的费用，而不只是管理台展示。

**处理方向。** 先明确倍率变更采用“允许短暂延迟”还是“提交后新计费立即生效”的约定。若允许延迟，应写明缓存配置和请求计费时点；若要求立即生效，需覆盖所有计费读取实例，单删认证缓存不足以解决。不要把该行为误写成认证 L2 的五分钟窗口。

**验收。** 两个独立 resolver 预热旧倍率，提交变更，再分别计费；验证实际生效时点符合选定约定，并覆盖没有用户专属倍率、使用分组默认倍率的情况。

代码依据：[倍率读取缓存](/home/caozhe/workspace/sub4api/backend/internal/service/user_group_rate_resolver.go:44)、[批量修改倍率](/home/caozhe/workspace/sub4api/backend/internal/service/admin_group.go:1264)、[默认 TTL](/home/caozhe/workspace/sub4api/backend/internal/service/gateway_service.go:60)。

### R10 平台配额 flusher 的待持久化状态可能丢失

**触发条件。** `database.user_platform_quota_flusher_enabled=true`。默认关闭；关闭时走异步 DB 增量路径，不适用同一条 SPOP 归因。

**机制与影响。** flusher 先从 Redis dirty set 执行 SPOP，再读取快照、写 DB。进程若在弹出后写库前退出，标记已经离开队列。正常返回的读写错误大多会重新加入，但进程崩溃无法执行回填；快照缺失或窗口字段缺失的项也会跳过。

只要 Redis entry 仍在，准入仍可使用当前值；该 key 再次产生用量会重新入 dirty set，可能修复 DB 镜像。若没有后续流量且缓存先过期，DB 可能长期缺少最后一段用量，重新装载后导致配额视图偏低。这是平台配额持久化风险，不等于余额扣费或 usage 日志必然丢失。

**处理方向。** 先确认线上是否启用，再验证“弹出后中断”和“无新流量直到缓存过期”。若启用部署需要崩溃后仍保证落库，应在现有写入路径内补充可恢复的待处理状态或其他已持久化来源的对账恢复；不应只靠更频繁 flush 声称解决。源码已注明的管理员重置与 flusher 覆盖竞态属于现有取舍，另行核对业务要求，不在本轮直接扩大改造。

**验收。** 弹出后模拟进程重启，禁止后续请求重新打脏标记，检查恢复后能否补齐 DB；再覆盖 snapshot miss、可重试 DB 错误、回填 dirty set 失败以及关闭 flusher 的原路径。

代码依据：[弹出与快照处理](/home/caozhe/workspace/sub4api/backend/internal/service/user_platform_quota_flusher.go:117)、[SPOP](/home/caozhe/workspace/sub4api/backend/internal/repository/billing_cache.go:577)、[配置分流](/home/caozhe/workspace/sub4api/backend/internal/service/gateway_usage_billing.go:425)、[默认关闭](/home/caozhe/workspace/sub4api/backend/internal/config/config.go:2221)。

## 五 生产观测及核验要求

原记录描述环境为 ccvps / huskyc，部署 sub2api、Redis、PostgreSQL 16，并提到 30 天日志窗口及“2733d1106 之后的构建”。这个版本范围不足以证明当前源码与当时行为一致。以下保留为排查线索，不作为本轮实证结果。

| 观测 | 当前能支持的判断 | 尚需补齐 |
|---|---|---|
| 重连预算日志：attempts=1、elapsed_ms=15915、budget_ms=5000、reason=ws_connection_limit_reached | 首次尝试后预算已经耗尽，与 R5 路径一致 | 精确部署 SHA、requestID/connID、连接创建时间、上游错误原文、是否已输出及可重放内容 |
| 原记录称 30 天内 2 次 forward_failed、0 次 reconnect_retry | 值得核对的发生频次 | 查询语句、时间区间及时区、日志完整性、相关请求和重试路径 |
| 2026-10-20 23:05 重启窗口出现 task_dropped 和 mandatory_sync_fallback，所查时段 usage_logs 无行 | 工作池拒绝任务并进入同步回退，存在待核验的落库疑点 | 请求级记录、同步回退的完成或错误、DB 关闭顺序、账务变更；原时间未注明时区 |
| ingress 日志中 previous_response_id 与上轮 ID 一致 | 记录内部的续接 ID 自洽 | 用户、客户端会话、connID、轮次及响应内容关联，才能检查真实错链 |
| 原记录称 Redis 无淘汰且哨兵键长期存在 | 当时实例状态及容量线索 | 采样命令、时间、键类型、所属活跃桶与数量增长趋势 |

关机疑点不能直接定为“丢失一条并少收费”：工作池的丢弃日志有限频，handler 还会使用独立超时同步执行任务。usage 日志是否存在与实际是否扣款，应分别按请求核对。

对应实现：[同步回退](/home/caozhe/workspace/sub4api/backend/internal/handler/openai_gateway_handler.go:3468)、[丢弃日志限频](/home/caozhe/workspace/sub4api/backend/internal/service/usage_record_worker_pool.go:343)。

## 六 不应继续沿用的归因

| 原判断 | 修正后的结论 |
|---|---|
| 没有逐请求失效认证缓存，所以 API key 总配额耗尽后正常放行约 300 秒（C7） | [扣费达到阈值时主动失效](/home/caozhe/workspace/sub4api/backend/internal/service/gateway_usage_billing.go:382)，状态变更另有 outbox。剩余问题应按失效失败、旧值回填或在途并发分析，不能据此增加逐请求失效。 |
| cache_read_tokens 大于 input_tokens 就是缓存统计错误 | [记录时将普通输入、缓存读取、缓存写入拆成互斥桶](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_usage.go:183)。应核对上游总输入与各桶、适用价格及成本计算，不能只比较两列大小；也不能据此断言所有生产账单正确。 |
| prompt_cache_key 相同就等于跨用户读到了缓存回答（C8） | 相同路由标识本身不能证明内容泄漏。需要追踪它是否又被用于续接锚点、私有状态或输入裁剪；这些风险分别见 R1—R3。 |
| cache TTL override 造成的 1h 归类就是缓存异常（C9） | [override 是明确的计费归类逻辑](/home/caozhe/workspace/sub4api/backend/internal/service/gateway_upstream_response.go:1294)。应核对账号配置与计价约定，不能作为随机缓存污染处理。 |
| 长期存在的 sched:ver 键就是泄漏（C13） | [版本键参与快照版本分配](/home/caozhe/workspace/sub4api/backend/internal/repository/scheduler_cache.go:467)。TTL=-1 不能单独证明泄漏；先确认废弃桶与增长趋势，不直接删除已有版本或隔离措施。 |
| Redis key 都有 groupID，所以没有跨租户问题 | 余额按 userID、限额按 keyID、订阅按 user/group 等不同作用域设计；是否隔离必须逐个追踪读取和授权，不能从某一种前缀概括全部状态。 |
| 日志未看到错链，就可以关闭上下文串线排查 | ID 自洽不等于归属正确；错误响应一旦被采纳为上一轮，后续日志仍可能自洽。 |
| 有 Lua、outbox、快照版本，就已消除所有竞态 | 这些机制只覆盖各自边界。Lua 不包含先前 DB 读取；outbox 不覆盖所有表；结构版本不阻止旧数据回填。 |

Gemini/OAuth token、管理台查询缓存、effort probe 等其他缓存不在本轮风险结论覆盖范围内；原记录中的“全部验证无问题”不再作为排除依据。

## 七 验证顺序与交付要求

先用受控测试确认错误链路，再对照生产请求归因。不要先清空全部 Redis 或批量切换连接模式，这会破坏现场，也不能证明根因。

| 顺序 | 验证工作 | 通过条件 |
|---|---|---|
| 1 | R1、R2 的两个用户及无身份会话隔离 | 不跨用户授权续接，不跨独立会话继承私有状态；允许的同用户续接保持可用 |
| 2 | R3 分叉、R4 迟到帧及拒绝重试 | 完成顺序与帧时序变化都不能改变响应归属，不能错误裁剪输入 |
| 3 | R6、R7 的 DB 读取、失效、回填、增量交错 | 已提交变更不被旧快照恢复；明确未入队的用量得到处理且不重复累加 |
| 4 | R5 的连接年龄、deadline 和续接重放 | 临近到期与长持有场景分别处理；不重复客户端输出、不静默丢上下文 |
| 5 | R8 的迟到回填、RPM 提交后中断 | 规则最终按约定生效，失效恢复时延可测量，未覆盖的路径有明确处理 |
| 6 | R9 倍率生效与 R10 配额持久化 | 多实例计价遵循生效约定；启用 flusher 时，中断后的配额能恢复或准确报告缺口 |
| 7 | 重启用量疑点与原生产计数 | 绑定精确版本、时区和请求 ID，分别核实日志落库与账务结果 |

现有连接池脏帧、execution scope、HTTP response owner、认证 outbox、flusher 错误回填测试可作为起点，但不能代替上表的跨轮、跨用户、并发分叉和中断场景。测试应使用可控制的时序和故障点，避免靠偶发竞争或长时间等待来判断结果。

每项修复交付应包含：可失败的最小复现、根因所在共享路径的修改、关键行为验收结果、受影响部署条件，以及剩余限制。仅调整 TTL、放大预算或增加告警，不能自动视为关闭对应问题。

本轮交付仅为报告重整与源码核对；未修改运行代码，未执行测试或生产操作。
