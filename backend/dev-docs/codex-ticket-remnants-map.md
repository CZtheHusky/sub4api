# 任务书：Codex 打票残留清理

> **状态：已完成（2026-10-21）。** 已删除无生产入口的 harvest 传输残留链与两个专属测试，修正开发指南；定向回归及后端构建通过，无跳过用例。未执行数据库操作。
> **核对日期：** 2026-10-21。
> **目标：** 删除已无生产调用方的打票专用 HTTP 传输策略及其专属测试，修正开发指南中的过时诊断说明。
> **范围依据：** 原“Codex 打票残留地图”及当前代码。不把“名字含 ticket/harvest”视为删除理由，不扩展为全仓库功能裁剪。

## 1. 背景与当前结论

Codex 打票（ticket harvesting）已在 1.2.0 移除：后台收割、票据注入、票据中心、打票代理池、失效流水和相关工具不再使用。参见 [1.2.0 变更记录](../../changelog/1.2.0/CHANGELOG.md)及[迁移 251](../migrations/251_drop_codex_ticket.sql)。

打票时代的一部分命名已被降智检测复用。当前 `codex_ticket_attempts` 仍承担检测历史，[迁移 252](../migrations/252_codex_attempt_ws_probe.sql)已增加 WS effort probe 观测字段；本任务不得删除这些在用能力。

本次核对确认两项可实施工作：

1. **删除 harvest 传输策略整条残留链。** `HTTPUpstreamProfileOpenAIHarvest` / `openai_harvest` 未发现生产设置入口，只剩定义、识别分支、内部协议模式及两个专属测试。不能只删公开常量而留下内部 `openai_h1_noreuse` 分支。
2. **修正开发指南的一整条过时说明。** [DEV_GUIDE.md](../../DEV_GUIDE.md)仍描述 `RunCodexNativeDiagnostic`、原生直连、打票代理池和“不回环网关”；当前实际入口已是 `CodexGatewayDiagnosticRunner.RunGatewayDiagnostic`。修正范围是该诊断说明整条，不限于代理池一句，也不顺带重写邻近章节。

本结论仅针对本任务范围，不代表其他功能均有用或均已审计。实施前复核引用；如果发现新的有效调用方，暂停删除该链并报告用途，不能直接删除调用方以满足清理目标。

## 2. 删除清单

以下四个代码文件应在同一改动中完成，保留文件内的其他功能和测试：

| 位置 | 删除内容 | 保留内容 |
| --- | --- | --- |
| [service/http_upstream_profile.go](../internal/service/http_upstream_profile.go) | `HTTPUpstreamProfileOpenAIHarvest` 常量，以及 `HTTPUpstreamProfileFromContext` 中对应分支项 | Default、OpenAI、Grok、LongStream；上下文读写、重定向及公共地址限制 |
| [repository/http_upstream.go](../internal/repository/http_upstream.go) | `upstreamProtocolModeOpenAIH1NoReuse = "openai_h1_noreuse"`；`resolveProtocolMode` 中 harvest 选择分支；`isOpenAIUpstreamProtocolMode` 中该模式项；transport 构造中为它禁用 keep-alive 的专属 case 和注释 | 默认/OpenAI/Grok/LongStream 传输；H1/H2、代理兼容回退、连接池及指纹处理 |
| [service/http_upstream_profile_test.go](../internal/service/http_upstream_profile_test.go) | `TestWithHTTPUpstreamProfile_OpenAIHarvest` | 其他 profile、重定向及公共地址限制测试 |
| [repository/http_upstream_test.go](../internal/repository/http_upstream_test.go) | `HTTPUpstreamSuite.TestOpenAIHarvestProfileDisablesKeepAlives` | 其他 HTTPUpstream 用例，尤其连接复用、代理、H1/H2 回退和长流测试 |

边界要求：

- 删除打票专属的“每次新建 CONNECT”策略，不改变正常 OpenAI 请求的连接复用与代理出口。
- 不将 harvest 名称换成通用名称继续保留同一死分支，不新增兼容别名、配置项或替代机制。
- 不全仓删除 `DisableKeepAlives` 等通用能力，只删除本表标明的专属分支。
- LongStream 不属于本任务。即使实施时发现其调用情况值得另查，也不能借本次清理一并移除。

## 3. 必须保留的能力与数据

| 保留项 | 当前用途与证据 |
| --- | --- |
| `codex_ticket_attempts`、分区、仓库和结构体 | 检测历史读写；见 [attempt repo](../internal/repository/codex_ticket_attempt_repo.go)、[attempt model](../internal/service/codex_ticket_attempt.go)。[gateway runner](../internal/service/codex_diagnostic_gateway.go)仍维护分区生命周期 |
| `openai_codex_ticket_prompt_template` 设置键 | 降智探测模板；见 [domain_constants.go](../internal/service/domain_constants.go) 的 `SettingKeyOpenAICodexTicketPromptTemplate` 和 [模板读取](../internal/service/setting_codex_probe_template.go) |
| ModelTrace、effort probe、WS 观测及自动降智检测 | 都是保留功能；不修改判据、模型白名单、账号封锁/恢复策略或计费语义 |
| diagnostic / fingerprint / attempt 命名的代码 | `codex_ticket_diagnostic.go` 当前提供上下文标记与判定；实际网关探测在 `codex_diagnostic_gateway.go`，不能按旧文件名判断已废弃 |
| `IsCodexTicketDiagnostic`、内部诊断认证及调用点 | 支撑内部合成身份认证、指定账号探测和不产生网关计费记录；见 [内部认证](../internal/service/codex_diagnostic_internal_auth.go)、[认证中间件](../internal/server/middleware/api_key_auth.go) |
| 管理接口与 [codexTickets.ts](../../frontend/src/api/admin/codexTickets.ts) | 指纹查询/刷新、ModelTrace 检测、effort 快速检测、历史查询/删除；接口路径和前端类型名称保持不变 |
| [codex_ticket_extra_redact.go](../internal/service/codex_ticket_extra_redact.go) | 仍由 [DTO 映射](../internal/handler/dto/mappers.go)和[账号导出](../internal/handler/admin/account_data.go)调用，剥除 `codex_turn_ticket:*` 与 `codex_harvest_proxy_url`；这是现有敏感信息边界 |
| 所有已发布迁移及其回归测试 | 保留 239、241、244、251、252 等数据库演进记录，不编辑旧 SQL，也不新建清理迁移 |
| 历史 CHANGELOG 和本地研究资料 | 保留历史事实，不要求历史文本中的 ticket/harvest 消失 |
| 前端构建产物 | `backend/internal/web/dist/` 下带 ticket 的文件名随正常构建维护，不手工删除、不为本任务重建前端 |

不重命名保留表、设置键、路由、DTO、文件或函数；不清空检测历史；不调整账号配置。敏感信息剥除代码及对应测试不得作为“只剩兼容用途”的残留删除。

背景研究可参考 [研究史](../../docs/openai-degradation-research-history.md)。`docs/` 是本地忽略目录，相关链接不保证在远端仓库可访问，不能代替上表中的代码证据。

## 4. 实施任务与交付物

| 编号 | 任务 | 完成条件 |
| --- | --- | --- |
| T1 | 复核删除链与当前工作区 | 列出两个 profile/mode 名称的全部代码引用，确认无生产设置入口；识别并保留其他任务的改动 |
| T2 | 一次性删除第 2 节代码和专属测试 | 常量、识别项、模式选择、transport 分支和两个专属用例均移除，无剩余死分支；其他传输行为不变 |
| T3 | 修正 DEV_GUIDE 的过时诊断条目 | 描述与当前 gateway runner 一致，不再声称存在打票代理池或旧原生直连入口 |
| T4 | 定向回归、构建及差异检查 | 第 5 节验收满足；记录通过、失败和未执行项；更新本任务书实施状态 |

T3 应明确写出当前事实：

- ModelTrace 探测通过 `CodexGatewayDiagnosticRunner.RunGatewayDiagnostic` 在进程内进入网关中间件链，使用内部诊断令牌、合成身份并钉住目标账号。
- 上游转发复用当前网关的账号传输与代理处理，不再存在打票专用代理池。
- 诊断不产生网关 usage/billing 记录、不消耗用户在网关内的额度；这不等于 ModelTrace 在上游零 token。
- 失败原因以当前 runner/handler 返回为准，不照抄旧直连实现的原因列表。
- 该说明专指 ModelTrace 网关诊断，不把 effort probe 描述成经过同一入口。

本期预期交付为四个 Go 文件中指定代码和测试的删除、DEV_GUIDE 中一个诊断条目的修订，以及本任务书的完成记录。四个文件本身保留；不新增依赖、数据库迁移、前端改动或功能开关。

## 5. 验收标准与验证方法

### 5.1 验收矩阵

| 检查项 | 验收标准 |
| --- | --- |
| 残留链 | Go 源码和测试不再引用 harvest profile 或 no-reuse 内部模式；文档与历史记录可以保留其名称 |
| 正常 HTTP 传输 | 原有 OpenAI H1/H2、代理兼容回退、连接复用、Grok 和 LongStream 用例继续通过 |
| 探测行为 | 既有 effort 分类、生产传输复用及 gateway diagnostic 定向用例继续通过 |
| 敏感信息保护 | DTO 与导出继续剥除门票材料、旧代理凭据，不修改源对象 |
| 保留边界 | 迁移、表/设置键、诊断认证、路由、前端及构建产物无改动 |
| 开发指南 | 当前条目准确描述进程内网关诊断，不再引用已不存在的原生直连与打票代理池 |
| 构建 | 后端构建成功，无删除后留下的引用或未使用导入 |

删除任务优先运行现有有效行为的回归测试，不新增只断言“旧符号不存在”的测试，也不为删除后的功能补造保活测试。

### 5.2 引用复核

在仓库根目录执行，实施前用于确定删除范围，实施后应无匹配：

```sh
rg -n 'HTTPUpstreamProfileOpenAIHarvest|openai_harvest|upstreamProtocolModeOpenAIH1NoReuse|openai_h1_noreuse' backend --glob '*.go'
```

`rg` 无匹配时退出码为 1，代表这里预期的结果。不要把文档中的保留说明或旧迁移里的其他 harvest 键计为失败。

实施前另核对 `WithHTTPUpstreamProfile` 的生产调用及动态参数来源，避免只搜常量名漏掉通过字符串构造的入口；本轮核对未发现 harvest 入口。

### 5.3 定向测试与构建

在 `backend/` 下运行：

```sh
go test -tags=unit ./internal/service ./internal/repository -run 'HTTPUpstream|EffortProbe|CodexDiagnostic|RunGatewayDiagnostic' -count=1
go test -tags=unit ./internal/handler/admin ./internal/handler/dto -run 'ExportDataExcludesCodexTicketMaterial|AccountFromServiceShallow_RedactsCodexTurnTicketState' -count=1
go build ./...
```

第一条包含 `TestHTTPUpstreamSuite`，应运行套件内的存续用例；不能只过滤已经删除的两个测试而得到“无测试可运行”。确认各目标包实际运行了相应用例；如测试名称变更，调整过滤器并记录。

不要求执行真实账号探针、线上部署、数据库迁移或前端重建。若定向用例暴露相关回归，修复后重跑受影响检查；无关既有失败单列，不扩大清理范围。

预计超过 10 分钟或耗时无法可靠预估的测试、构建，按项目约定使用持久监管：优先 `systemd-run --user`，不可用再使用规定的替代方式；记录监管标识、日志路径、输出目录和完整命令，不在 PTY 会话中承载。

### 5.4 交付检查

- 核对差异只包含第 2 节删除、DEV_GUIDE 修订及本任务书状态更新，未夹带其他清理。
- 保留现有迁移及数据，不需要新增迁移或执行 SQL。
- 回报实际运行的测试、构建及结果；跳过的检查明确写出，不能称为通过。
- 如发现新的生产调用方或无法解释的兼容依赖，单列具体调用链和影响，暂停相应删除；不要用重命名、fallback 或新增包装隐藏问题。

## 6. 状态记录

| 项目 | 状态 |
| --- | --- |
| 任务书整理与当前代码静态核对 | 已完成（2026-10-21） |
| T1 实施前复核 | 已完成；完整引用链及动态参数来源均复核，无生产入口 |
| T2 代码清理 | 已完成；四个 Go 文件中的专属链与两个测试已移除 |
| T3 DEV_GUIDE 修订 | 已完成；已改为当前 ModelTrace 进程内网关诊断说明 |
| T4 测试、构建与最终验收 | 已完成；定向回归与后端构建通过，无失败或跳过 |

本任务完成的含义是“删除已证实无入口的打票专用传输策略，修正相关说明，同时保留在用检测及数据边界”，不以全仓库 ticket/harvest 字样清零为目标。


## 7. 实施记录（2026-10-21）

### T1：引用与边界复核

实施前，指定搜索在 Go 源码/测试中共命中 12 行，仅分布于以下四个文件（行号为修改前版本）：

- `internal/service/http_upstream_profile.go:12,42`：harvest profile 常量与上下文识别项。
- `internal/service/http_upstream_profile_test.go:24,25,26`：该 profile 的专属上下文测试。
- `internal/repository/http_upstream.go:106,1127,1128,1428,1475`：no-reuse 模式常量、harvest 选择分支、OpenAI 模式识别及 transport 分支。
- `internal/repository/http_upstream_test.go:703,705`：harvest 专属 transport 测试。

所有 `WithHTTPUpstreamProfile` 生产调用均已复核；唯一动态 profile 参数位于 `internal/service/upstream_billing_probe.go:666-670`，仅在 Default 与 OpenAI 间选择，无字符串构造或配置注入 harvest 的入口。未发现新的有效调用方。

已确认保留诊断内部认证/合成身份、ModelTrace gateway runner、effort probe、检测历史与分区维护、模板设置键及 DTO/账号导出的敏感信息剥除调用链。工作区另外两份未跟踪文档 `cache-anomaly-assessment.md`、`codex-client-fingerprint-simulation.md` 不属于本任务，保持原样。

### T2/T3：已完成的改动

仅删除第 2 节所列常量、分支和两个专属测试；四个 Go 文件均保留。Default、OpenAI、Grok、LongStream、H1/H2、代理回退、重定向与公共地址限制逻辑未改。没有新增替代策略或测试。

`DEV_GUIDE.md` 仅替换一条过时诊断说明：当前 ModelTrace 经进程内网关中间件链、内部令牌与合成身份钉住账号，复用网关传输/代理处理，不产生网关 usage/billing 记录但不承诺上游零 token，错误以当前 runner/handler 为准；说明不适用于 effort probe。

删除后指定 `rg` 搜索无匹配（预期退出码 1），格式与差异检查通过。迁移、前端源码、构建产物及第 3 节保留代码均无改动。

### T4：验证记录

在本机使用 Go 1.27.1 执行以下命令，工作目录为 `backend/`，环境设置 `PATH=/home/caozhe/toolchains/go/bin:$PATH`、`GOMAXPROCS=3`：

```sh
go test -tags=unit ./internal/service ./internal/repository -run 'HTTPUpstream|EffortProbe|CodexDiagnostic|RunGatewayDiagnostic' -count=1 -json
go test -tags=unit ./internal/handler/admin ./internal/handler/dto -run 'ExportDataExcludesCodexTicketMaterial|AccountFromServiceShallow_RedactsCodexTurnTicketState' -count=1 -json
go build ./...
```

| 检查 | 实际结果 |
| --- | --- |
| service 定向回归 | 16 个顶层用例、20 个子用例通过；无失败/跳过 |
| repository 定向回归 | 16 个顶层用例、32 个子用例通过；包含 `TestHTTPUpstreamSuite` 的存续用例，无失败/跳过 |
| admin 账号导出敏感信息剥除 | `TestExportDataExcludesCodexTicketMaterial` 实际执行并通过 |
| DTO 敏感信息剥除 | `TestAccountFromServiceShallow_RedactsCodexTurnTicketState` 实际执行并通过 |
| 后端构建 | 退出码 0，通过 |
| 残留引用搜索 | 第 5.2 节命令无匹配，退出码 1，符合预期 |
| 差异及格式检查 | `git diff --check` 通过；仅四个 Go 文件、DEV_GUIDE 一个条目及本任务书变更 |

相关存续测试实际覆盖 OpenAI 默认 H2、禁用 H2 后的 H1、代理兼容回退、普通超时不触发回退、H2 单流关闭不影响其他流、连接池隔离与复用、Grok、LongStream keep-alive、重定向限制与公共地址校验、effort 分类及生产传输复用、ModelTrace gateway diagnostic、内部诊断认证和合成身份。未新增仅检查删除符号的测试。

任务通过用户级 systemd 独立监管：

- 单元：`sub4api-ticket-remnants-20261021.service`；启动后已核对 MainPID 与 ControlGroup，最终 `ExecMainStatus=0`、`ActiveState=inactive`。
- 输出目录：`/tmp/sub4api-ticket-remnants-20261021/`。
- 完整执行脚本：`/tmp/sub4api-ticket-remnants-20261021/validate.sh`，包含上述全部测试与构建命令，每步单独记录退出状态。
- 日志：`validation.log`（三个阶段退出码均为 0）、`transport-and-diagnostic.jsonl`、`redaction.jsonl`、`build.log`；`summary.json` 由测试 JSON 输出汇总实际用例数，`environment.txt` 记录 Go 版本。
- 启动命令：

```sh
systemd-run --user --unit=sub4api-ticket-remnants-20261021 \
  --property=StandardOutput=append:/tmp/sub4api-ticket-remnants-20261021/validation.log \
  --property=StandardError=append:/tmp/sub4api-ticket-remnants-20261021/validation.log \
  /tmp/sub4api-ticket-remnants-20261021/validate.sh
```

本次未运行真实账号探针、线上部署、数据库迁移或其他 SQL、前端测试/重建；任务不要求这些操作。未修改检测历史、设置键、诊断认证、判据/计费/停调策略、已发布迁移、前端与构建产物，也未处理另两份未跟踪研究文档。
