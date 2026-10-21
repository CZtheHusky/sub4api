# OpenAI legacy 粘性会话 TTL 恢复任务书

日期：2026-10-21。状态：补丁完成，线上验证待执行。优先级：P2，优先撤销已确认的亲和时长回归。

本地核对基准：`baf55ebaf92b8ac81e4e90d271f37837881277c7`，起草时工作区干净。最近一次生产核验为 2026-10-21 09:40 PDT，ccvps 的 `sub2api.service` 运行 `a6c17dc1231b204b9823b5df775eaa8d7ba7dc0a`；09:42 完成 Redis TTL 快照。实施及发布前重新核对版本。

## 1. 任务目标与决策

撤销 `234ef4d60` 将 `openaiStickySessionTTL` 从 `time.Hour` 缩短为 `5 * time.Minute` 的变更，使 legacy 调度写入和刷新会话主键时恢复 1 小时亲和。

**本次选择恢复常量，不将 legacy 调度全面改为配置驱动。** 后者涉及既有配置语义的统一，可另开任务。本次交付应形成可独立应用、验证和发布的最小补丁。

成功标准：在账号仍健康、可调度、未被排除且容量充足的条件下，无有效 response-ID 绑定兜底的会话，间隔 6 分钟再次请求仍能命中原账号；创建和命中续期均写入 3600 秒，续期后按最后一次刷新计算过期时间。

本任务恢复的是 session→account 的缓存保留时间。它不保证账号始终可用，也不保证 prompt cache 必然命中。

## 2. 已确认事实及证据边界

| 事实 | 结论与限制 |
|---|---|
| `234ef4d60` 将公共常量由 1 小时改为 5 分钟 | 是本轮新增回归，修改点位于 [openai_gateway_service.go:35](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_service.go:35) |
| legacy 调度有 5 处直接使用常量写入或刷新绑定 | 无有效 response-ID 绑定等条件下，空闲超过 5 分钟可能重新选号、失去账号亲和 |
| `BindStickySession()` 和高级调度器优先读取配置 | 不能声称所有 HTTP 入口都忽略配置；legacy 直接用常量的行为在本轮之前已经存在 |
| response-ID→account 独立绑定默认仍为 1 小时 | session 绑定过期不等于携带有效 previous_response_id 的请求必然换号或返回 400 |
| 生产高级调度器关闭；API key 2 所属 group 2 的利润门关闭 | 具备进入受影响 legacy 路径的条件，不代表所有请求都经过该路径 |
| 09:42 的 Redis 快照：6 个 session 绑定剩余 2356–3565 秒；85 个 response-ID 绑定剩余 1516–3590 秒 | 当时实际存在大量超过 5 分钟的绑定；该快照不能证明所有路径正常，也不能证明历史故障由 TTL 导致 |

本任务依据是已确认的时长回归，不将“偶发 400 已定位”“上下文丢失已由 TTL 解释”作为前提。400 根因仍需单独关联请求、账号选择和绑定失效证据。

## 3. 修改范围与影响清单

### 3.1 生产代码

唯一计划中的生产逻辑改动：将 [openaiStickySessionTTL](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_service.go:35) 恢复为 `time.Hour`。保留现有命名和调用结构，不新增配置、接口、依赖或缓存类型。

实施者必须核对下列全部受影响位置。行号以本任务书基准为准，后续以符号定位。

| 位置 | 作用 | 恢复后的行为 |
|---|---|---|
| [openai_gateway_scheduling.go:951](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_scheduling.go:951) | 普通选号后首次绑定 | 主键 TTL 为 1 小时 |
| [openai_gateway_scheduling.go:1019](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_scheduling.go:1019) | 普通粘性命中后续期 | 主键从本次刷新起保留 1 小时 |
| [openai_gateway_scheduling.go:1244](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_scheduling.go:1244) | 负载感知路径命中后续期 | 同上 |
| [openai_gateway_scheduling.go:1411](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_scheduling.go:1411) | 负载数据正常时选号绑定 | 主键 TTL 为 1 小时 |
| [openai_gateway_scheduling.go:1450](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_scheduling.go:1450) | 负载查询失败后的选号绑定 | 主键 TTL 为 1 小时 |
| [BindStickySession](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_scheduling.go:234) | 配置化绑定，缺配置时用常量 | 有效显式配置继续优先；兜底恢复 1 小时 |
| [openAIWSSessionStickyTTL](/home/caozhe/workspace/sub4api/backend/internal/service/openai_account_scheduler.go:2598) | 配置化 TTL 获取 | 有效显式配置继续优先；兜底恢复 1 小时 |
| [openAIStickyLegacyTTL](/home/caozhe/workspace/sub4api/backend/internal/service/openai_sticky_compat.go:111) | 旧哈希兼容键保留时间 | 继续遵守既有 10 分钟上限 |

这里的“legacy 调度”与“旧哈希兼容键”是两件事：前者的会话主键恢复 1 小时；双写的旧哈希键经过 `min(主键 TTL, 10 分钟)` 限制。本次主键从 5 分钟恢复为 1 小时后，该兼容键通常随之恢复为 10 分钟，不能误验收为 1 小时。

### 3.2 测试与配置

优先扩展现有测试及其局部 stub：

- [openai_gateway_service_test.go](/home/caozhe/workspace/sub4api/backend/internal/service/openai_gateway_service_test.go)：选号、粘性命中、负载查询失败。
- [openai_legacy_scheduler_decision_test.go](/home/caozhe/workspace/sub4api/backend/internal/service/openai_legacy_scheduler_decision_test.go)：真实 legacy 调度入口及 response-ID 优先级。
- [openai_sticky_compat_test.go](/home/caozhe/workspace/sub4api/backend/internal/service/openai_sticky_compat_test.go)：新旧哈希键 TTL。
- [openai_account_scheduler_test.go](/home/caozhe/workspace/sub4api/backend/internal/service/openai_account_scheduler_test.go)：兜底与显式配置兼容。

不要求每个文件都修改；按实际测试复用情况选最少文件。当前缓存 stub 的 `SetSessionAccountID` 忽略 TTL，`RefreshSessionTTL` 直接返回成功，不能验证此次回归。测试需捕获实际传入的 TTL，并对关键场景提供测试内可推进的过期判断。优先复用现有 stub，不增加生产时钟接口或通用缓存模拟框架。

[配置默认值](/home/caozhe/workspace/sub4api/backend/internal/config/config.go:2478)和[示例配置](/home/caozhe/workspace/sub4api/deploy/config.example.yaml:401)已为 3600，无需修改。不要为本次恢复新增参数或环境变量。

### 3.3 排除项

本任务不调整账号选择优先级、健康判断、容量溢出、利润门和排队策略；不改变 response-ID 亲和、续接恢复、用户并发限制或 `http_bridge` 解释方式。R5 的连接 55/60 分钟轮换、30 秒重试预算、Sol 500k 上下文改动均不属于本次热修复。

不迁移、批量续期或清空生产 Redis。既有键维持原剩余 TTL，新代码执行正常写入/刷新时才获得恢复后的 TTL；已经过期的绑定不会自动复活。

## 4. 实施任务

| 任务 | 工作内容 | 完成条件 |
|---|---|---|
| T1：确定补丁基线并复现 | 核实 HEAD、生产版本及全部常量引用；用 legacy 路径构造超过 5 分钟的会话间隔 | 修改前测试因绑定过早失效失败，而非因为账号健康、容量或随机选择失败 |
| T2：恢复时长 | 将公共常量恢复为 `time.Hour` | 生产代码仅有必要改动；五处调度操作和三个兜底/兼容使用处均已核对 |
| T3：回归与兼容 | 完成第 5 节矩阵，记录定向测试及仓库检查结果 | 核心用例在旧值下失败、新值下通过；显式配置与旧哈希上限均符合预期 |
| T4：交付可发布补丁 | 形成独立提交，列明测试结果、已知基线问题、最终发布基线 | 提交只含 TTL 恢复、必要测试和任务记录；可独立应用到选定生产基线 |
| T5：发布与验证 | 在明确发布安排后部署最终热修复制品，执行第 7 节验证 | 核实运行版本、新建/续期 TTL 和跨 5 分钟粘性命中；记录回退制品与观察结果 |

T1–T4 完成表示补丁可交付；T5 完成后才能写“线上恢复已验证”。本次起草任务书不代表已执行修复或发布。

## 5. 验收矩阵

测试前固定：高级调度器关闭、利润门关闭、两个账号均可用、可立即抢到槽、session 标识稳定。检验 session TTL 的用例不携带 previous_response_id，避免被独立的 1 小时 response 绑定掩盖。

| 场景 | 必须验证的结果 |
|---|---|
| 普通选号首次绑定、负载感知正常选号、负载查询失败兜底 | 三类路径实际写入主键的 TTL 均为 `time.Hour`；不能只断言绑定值存在 |
| 普通粘性命中、负载感知粘性命中 | 两类路径实际续期 TTL 均为 `time.Hour` |
| 首次绑定后推进 6 分钟 | 同一 session 仍命中原账号；把另一账号设成无绑定时更优的候选，使错误地重新选号一定能被测试发现 |
| 命中后续期，推进到首次写入已超过 1 小时、但距最后刷新不足 1 小时 | 仍能命中原账号，证明刷新改变了到期点 |
| 距最后刷新超过 1 小时 | 绑定过期并按已有策略重新选号，不成为永久绑定 |
| 无有效显式 TTL 的兜底 | `BindStickySession` 与 `openAIWSSessionStickyTTL` 的兜底均为 1 小时；仅测试既有内部调用可到达的缺配置状态 |
| 配置化路径显式设为 1800 秒 | `BindStickySession` 实际写入及 `openAIWSSessionStickyTTL` 返回值仍为 1800 秒；本次不把 legacy 五处调用改为读取该配置 |
| 新旧哈希双写及刷新 | 主键为 3600 秒，旧哈希兼容键为 600 秒；显式主键 TTL 短于 600 秒时，旧键也不超过主键 |
| session 绑定缺失但有效 response-ID 绑定存在 | 沿用已有 response-ID 优先路由测试，仍选原 response 所属账号 |
| 原账号不可调度或被排除 | 沿用现有测试，仍允许清理/绕过旧绑定；恢复 TTL 不强迫请求使用坏账号 |

新增 TTL 用例名称包含 `StickyTTL`，便于确认实际被执行。测试预期使用业务要求 `time.Hour` 或 3600 秒，不能与 `openaiStickySessionTTL` 相互引用导致改坏常量后仍通过。用测试时间推进验证间隔，不真实等待 6 分钟或 1 小时。

## 6. 检查要求与记录

使用仓库 CI 指定的 Go 1.27.0。从 backend 目录执行，先核对实际匹配的测试列表，再运行：

```bash
go test -tags=unit ./internal/service -list 'StickyTTL|LegacySchedulerDecision|OpenAISelectAccount|StickySessionAccountID|SchedulerWrappersAndDefaults|ProfitControl.*Sticky'
go test -tags=unit ./internal/service -run 'StickyTTL|LegacySchedulerDecision|OpenAISelectAccount|StickySessionAccountID|SchedulerWrappersAndDefaults|ProfitControl.*Sticky' -count=1
go test -race -tags=unit ./internal/service -run 'StickyTTL|LegacySchedulerDecision|StickySessionAccountID' -count=1
go build ./cmd/server
```

如果实际测试名称未被过滤器覆盖，先修正过滤器。保留测试数量、退出状态和日志位置。核心 TTL 用例须有修改前失败、修改后通过的证据；可在隔离 checkout 或覆盖视图中验证旧值，避免扰动并行任务的工作区。

在最终待发布源码上执行仓库要求的 `make test-unit`、`make test-integration` 和 `golangci-lint run ./...`；这些结果可复用同一源码已经完成的有效 CI/检查，不因文档改字重复执行。源码或发布基线不同则不能直接沿用 `baf55ebaf` 的检查结论。

已有记录的三个基线用例失败、Prewarm timing 失败和 57 项 lint 问题不自动归因于本任务，也不能把失败结果写成通过。实际遇到时记录名称及基线依据；新出现或受本次改动影响的失败必须查明并处理。未执行项明确标注。

构建、全量检查等耗时不确定的任务通过 `systemd-run --user` 托管；用户级 systemd 不可用时才选 tmux 或 setsid/nohup。记录单元/会话标识、完整命令、日志及输出目录，验证监管关系，不以 PTY 或持续轮询维持任务。

## 7. 发布、观察与回退

### 7.1 发布基线

默认采用独立热修复：以发布前核实的生产提交为基线，只应用本任务的恢复和必要测试。最近生产基线是 `a6c17dc`。分支如需新建使用 `codex/` 前缀，并将同一补丁同步到当前开发主线，避免后续发布再次带回 5 分钟。

本地 `baf55ebaf` 已包含尚未部署的 R5 和 Sol 500k。直接构建该提交加 TTL 补丁，会同时发布这些变更；这需要单独确定合并发布范围并完成相应验收，不能作为本次独立热修复的默认制品。

无数据库迁移、配置变更或 Redis 清理需求。单实例服务重启会中断现有 WS，发布前必须明确窗口及回退版本。保留上一运行制品及版本记录。

### 7.2 定向验证

1. 上线后核实实际运行二进制包含本补丁，记录版本、PID、启动时间和服务状态。
2. 使用允许的低成本请求或已获授权的正常测试会话：HTTP Responses 入口、稳定 session 标识、不携带 previous_response_id、legacy 调度、可立即获得账号槽。记录请求时间、所选账号和路由层；不记录凭据或正文。
3. 对照源码识别该 session 的当前格式主键、旧哈希兼容键及 response-ID 键。只读检查新写入/续期的主键 TTL 接近 3600 秒，误差由采样延迟解释；旧键上限为 600 秒。不能用 response-ID 键的一小时 TTL 充当 session 修复证据。
4. 同一 session 空闲至少 6 分钟后再请求，确认原账号在此期间仍可用且未满载，路由仍命中 session；随后确认主键重新获得接近 3600 秒的 TTL。若所走路径被排队后重新绑定等逻辑覆盖，标记该样本不证明目标分支，重新选择合适样本。
5. 遇到 400、换号或流错误时保留 requestID 与账号路由证据，单独归因。未看到错误不等于已证明 TTL 正确；账号变更若由限额、健康或容量触发，也不能直接判本次修复失败。

新版本不追溯延长未被访问的旧键。观察期间旧键仍可能以原剩余 TTL 过期；已有续接上下文不会因恢复常量自动补回。

这是一项 session TTL 验收，不要求真实等待 60 分钟验证过期；完整过期和续期边界由受控时间测试覆盖。R5 的 75 分钟连接寿命观察仍属于 R5 自己的发布验收。

### 7.3 回退与容量影响

出现可归因于本补丁的新增故障时，回退上一制品并记录触发条件。恢复为 1 小时会增加空闲 session 绑定驻留；同等新增会话速率下，这部分键的稳态数量可能接近原 5 分钟时的 12 倍，但不代表 Redis 总内存增长 12 倍。利用已有 Redis 内存、键数量和淘汰指标观察，不新增监控系统。

回退二进制不会立即缩短所有已经写入的 1 小时键；它们会自然过期，或被回退版本后续访问刷新。不得用清空 Redis 或批量删除会话作为默认回退步骤。

## 8. 交付与完成判定

交付物：独立修复提交、必要回归测试、修改前后复现记录、最终发布基线的检查结果、发布及回退说明。若尚未部署，明确标记“补丁完成，线上验证待执行”。

只有以下事项全部完成，才关闭本任务：

- 五处 legacy 写入/续期恢复 1 小时，配置化路径和旧哈希兼容键通过矩阵验证。
- 核心测试能识别旧 5 分钟行为；没有未解释的新失败或跳过项。
- 同一最小补丁已落到需要发布的分支及开发主线，未意外夹带其他功能发布。
- 生产运行版本及超过 5 分钟的定向 session 亲和验证有记录；风险与回退步骤已交付。

任务书起草时尚未实施。2026-10-21 的实施、检查与交付状态记录如下。

## 9. 实施与验证记录（2026-10-21，PDT）

### 基线与修改

- 09:53 重新核实 ccvps：`sub2api.service` 为 active，PID `1493590`，启动时间仍为 10/20 23:24:25 PDT；版本为 `a6c17dc1231b204b9823b5df775eaa8d7ba7dc0a`，可用空间约 2.2 GB。
- 独立工作区 `/home/caozhe/.codex/worktrees/openai-legacy-sticky-ttl/sub4api`，分支 `codex/openai-legacy-sticky-ttl-restore`，基于上述生产提交。热修复不包含开发主线的 R5、Sol 500k 或旧打票传输清理。
- 唯一生产逻辑改动是 `openaiStickySessionTTL = time.Hour`。已逐一核对五处 legacy 写入/刷新、`BindStickySession`、`openAIWSSessionStickyTTL` 及旧哈希 TTL 上限；配置默认值和示例保持 3600。
- 在既有缓存 stub 中记录实际 TTL 和到期点，测试内推进时间；没有增加生产时钟接口。覆盖普通、负载感知和负载查询失败兜底，六分钟亲和、按最后刷新计时与最终过期，显式 1800 秒和 300 秒配置，以及兼容键最多 600 秒。
- 原账号不可调度或被排除、response-ID 独立路由和利润门行为，沿用并执行已有定向用例。

### 修改前复现

在仍为 5 分钟的生产代码上，三个 legacy 子用例均于推进六分钟后从账号 `38101` 改选 `38102` 而失败。两账号健康、立即获取槽，另一账号已被设为无绑定时确定更优；没有携带 response-ID，故失败直接反映绑定过早过期。缺配置和部分配置的兜底用例也分别返回 5 分钟而失败；显式配置用例通过。

证据：`/tmp/sub4api-sticky-ttl-20261021/before.jsonl`；启动及退出状态见 `reproduce.sh` / `reproduce.log`。

### 最终生产基线检查

使用 Go 1.27.0、golangci-lint 2.13.2；结果目录 `/tmp/sub4api-sticky-ttl-20261021`。

| 检查 | 结果 | 记录 |
|---|---|---|
| 实际匹配列表 | 已核对，两组新增 TTL 用例均匹配 | `test-list.log` |
| 定向 unit | 57 项通过，无跳过，退出 0 | `hotfix-targeted.jsonl` |
| 定向 race | 23 项通过，无跳过，退出 0 | `hotfix-race.jsonl` |
| `go build ./cmd/server` | 通过，退出 0 | `hotfix-build.log`，输出 `server-check` |
| `make test-unit` | 22510 项通过、3 项失败、17 项跳过；make 退出 2，整体失败 | `hotfix-unit.log` |
| `make test-integration` | 14079 项通过、1 项失败、16 项跳过；make 退出 2，整体失败 | `hotfix-integration.log` |
| `golangci-lint run --timeout=30m ./...` | 57 项范围外问题，退出 1；本次修改文件无报错 | `hotfix-lint.log` |
| 前端发布构建 | 3 项 i18n 测试、vue-tsc 与 Vite 构建通过；未执行全套前端测试，无前端源码修改 | `build-frontend.sh` / `frontend-build.log` |

三个单测失败为 `TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_NoAvailableAccount`、`TestResponsesSearchBuiltinTranslation`、`TestResponsesSearchAdditionalToolsAndStringNamesAvoidCollisions`。把本次四份 Go 文件替换为 `git show a6c17dc:<path>` 得到的原始文件后，三项同样失败；`TestOpenAIGatewayService_PrewarmReadHonorsParentContext` 在对照和本轮全量中均通过。覆盖视图为 `baseline/overlay.json`，结果 `baseline-tests.jsonl` / `baseline.log`。

集成测试失败的同样是上述账号调度用例。所有计数包含 Go 详细日志的父用例及子用例；失败、跳过名单见 `checks-summary.json`。本次 unit/integration 跳过名单相对上一轮检查没有新增项，包含实时账号、TLS 捕获和审计环境等已有条件跳过；它们没有计为通过。静态检查为 35 项 errcheck、3 项 gofmt、3 项 ineffassign、15 项 staticcheck 和 1 项 unused。本轮完整结果单独保存，不以同样的总数认定与上一轮逐项一致；例如本轮还报告了 `codex_probe_template.go` 和 `openai_bps_alignment_test.go` 中未检查的类型断言。这两份源文件与生产基线、开发主线均一致，不属于本补丁改动，未扩大范围处理。

首次全量启动曾把 `-json` 放入 `GOFLAGS`，被 ent 内部的 `go list -f` 继承，造成 schema 测试启动错误。已停止该次检查，结果保留为 `invalid-unit-go-flags-json.jsonl`，不计为有效全量结果。改用仓库 make 目标和普通详细日志后，schema 用例通过（`schema-runner-check.log`），重新执行全量检查。

全部构建与检查由用户级 systemd 托管，未用 PTY。单元包括 `sub4api-sticky-ttl-reproduce-20261021`、`sub4api-sticky-ttl-hotfix-checks-20261021`、`sub4api-sticky-ttl-full-hotfix-20261021`、`sub4api-sticky-ttl-baseline-20261021`、`sub4api-sticky-ttl-frontend-20261021`。完整命令保存在同目录脚本，启动后已核实 MainPID、ControlGroup 和 ExecStart。初次定向、race、后端构建记录在 `validate-hotfix.sh` / `hotfix-checks.log`，正式全量在 `validate-full-hotfix.sh` / `full-hotfix.log`。

### 交付与发布状态

独立修复提交落在 `codex/openai-legacy-sticky-ttl-restore`，同一补丁同步到开发主线；提交号与含内嵌前端的发布制品标识由交付记录列出。本轮尚未上传文件、变更生产配置或 Redis、重启服务，也未执行真实账号请求。

发布需要确定单实例重启窗口。发布时保留当前 `a6c17dc` 制品用于回退；不清空 Redis，不批量迁移或续期旧键。上线后仍须执行第 7.2 节真实 session 的新建/续期 TTL 与至少六分钟亲和验证，因此目前不能写“线上恢复已验证”或关闭本任务。
