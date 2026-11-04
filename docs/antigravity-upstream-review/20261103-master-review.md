# Antigravity 上游移植：独立复核与验收

评审起始日期：2026-11-03（America/Los_Angeles）。本文件记录 Codex Master 的独立检查；候选逐项评审由 ZCode 记录在同目录 `20261103.md`。两者均独立于旧 `docs/upstream-review/` 批次台账。

最终状态：筛选的 6 个上游提交已最小适配并接收到主工作区，**本地独立验收通过**。其中 `412718edf` 与 `feab6d80d` 为部分采用；共享 apicompat 变动已按用户新增要求撤下。未提交、未推送、未发布；真实账号生成与全仓测试不在本次通过结论内。

## 固定输入与历史解释

| 项目 | 值 |
| --- | --- |
| 项目 | `CZtheHusky/sub4api` |
| 上游 | `Wei-Shaw/sub2api`，`main` |
| 本地起点 | `5cbb2bcfa` |
| 固定上游终点 | `4a4a83ee5ec86b19d3a2dd9b6471de5128963bec` |
| 实际 Git merge-base | `29b0e4a8a5c0b778583289c4b0fcc715fbd9cd9d` |
| 已有人工同步目标 | `3040209f205472038c1ba745a1bedd2edd9053b1` |
| 旧台账上游终点 | `a3298b46ebe239fb0720b068a1eb3383955bdac2` |

Master 实际执行 `git merge-base`、`git merge-base --is-ancestor`、`git rev-list --count` 并读取现有指南和台账。人工同步目标是上游祖先，但不是本地 HEAD 的祖先，不能沿用旧台账把它称为当前 Git 共同祖先。

实际 merge-base 到固定上游共有 3,793 个非 merge 提交及 1,927 个 merge 提交；人工同步点到上游有 264 个非 merge 及 153 个 merge。旧审阅终点之后新增 11 个非 merge、3 个 merge，非 merge 内容为管理 MCP、管理员密钥权限和相关依赖，未命中本轮 Antigravity 核心路径。

这些是历史范围计数，**不是未吸收功能数，也不是逐提交深审数量**。本地有大量人工吸收实现；本轮对照最终代码及直接调用者判断缺口，不能按 `HEAD..upstream` 整包合并。具体候选路径集合和覆盖界限见 ZCode 记录及 [候选索引](20261103-candidate-index.tsv)。

索引由 Master 直接根据 Git 对象生成：路径集合 B 自实际 merge-base 起 330 条，其中人工同步点后 37 条；另加 3 条路径集合之外的重点候选，共 333 条。11 条重点候选有本轮详审结论；其余行只记录路径命中、旧台账对应条目与“本轮未逐项复核”，不能当作 333 条深审通过。旧台账属于历史依据，不据此推断当前全部已吸收。

## Master 最终选择的接收范围

| 上游提交 | 选择 | 独立代码依据 |
| --- | --- | --- |
| `9c41b3269` | 采用最小修复 | Antigravity 非流式、流式用量更新、message_start，以及共享 `extractGeminiUsage` 共有四处直接执行 prompt 减 cached；缺 prompt 时会产生负 input_tokens。保留上游真实缓存命中值，仅将差值下限设为 0。 |
| `cfbbfd6b3` | 采用 | `modelRateLimitKeysForRequest` 和 `antigravityModelRateLimitKeys` 原来把图片模型并入 Gemini 文本家族。图片模型自身限流仍有效，图片与文本家族分离。 |
| `412718edf` | 仅采用 Antigravity 部分 | `ClaudeContentItem` 及 Antigravity 流式 thinking 起始块在空签名时省略字段；补齐字段。共享 `AnthropicContentBlock` 改动按用户新增约束撤下，保持 Codex/OpenAI 兼容链路原有序列化。 |
| `14a5fe305` | 采用 | 非流式转换与 Claude SSE 收集器未拒绝 `MALFORMED_FUNCTION_CALL`，包括终止错误帧没有 parts 的情况。错误不能包装成成功工具响应；不因此增加自动重试。 |
| `856d7ea90` | 采用 | 管理端 Antigravity 测试模型固定返回内置目录，未呈现账号的显式映射入口。展示请求侧 key，实际测试继续解析映射目标。 |
| `feab6d80d` | 部分采用并适配 | 保留 tool_result 图片/文档、原生 Gemini functionCall 签名、非流式聚合的完整 parts 和文本签名。复合组路由扩展不接收。补丁须保留本地 Claude/Gemini 签名边界，不能原样照搬。 |
| `38b72d734` | 本次不接收 | 写时复制有独立价值，但当前 `setAccountModelRateLimitSnapshot` 的调用者仅为 `openai_images_balance.go` 与 `ratelimit_service.go` 的 Anthropic Fable 路径，未找到 Antigravity 调用。撤下 worker 首轮额外移植，保留同文件的图片限流修复。 |
| `373d43533` / `9d3cc4507` | 暂不引入差异 | 不在本轮盲目扩充新模型目录、价格或改变裸模型显式映射行为；未验证该账号的新模型能力和定价。已有覆盖部分与后续修正须在候选记录中区分。 |
| `d5dbb5c36` / `53002f677` | 不引入 | 单账号判定性能改动和常驻调度缓存重构不属于本轮必须修复的协议正确性缺口。 |

关于测试映射的判断曾在复核中更正：通配 key 会先经过账号映射，并不必然成为无效上游模型名，不能仅因含 `*` 就过滤。应按真实请求构造和映射契约测试。

## 新增约束：保持 Codex/sub2api 行为

用户明确要求不要引入针对 Codex/sub2api 的行为变动。Master 检查共享类型的直接调用者后发现，`AnthropicContentBlock.MarshalJSON` 同时经 `ResponsesToAnthropic` / `ResponsesAnthropicEventToSSE` 被 `openai_gateway_messages.go` 和 `openai_gateway_messages_chat_fallback.go` 使用。该改动虽来自所选上游提交，也会改变现有 OpenAI 兼容输出，故不接收以下 worker 差异：

- `backend/internal/pkg/apicompat/types.go`
- `backend/internal/pkg/apicompat/anthropic_responses_test.go`
- 新增 `backend/internal/pkg/apicompat/thinking_signature_wire_test.go`

本轮只保留 Antigravity 类型和输出的 thinking 修复，不另建全局开关或共享抽象。`extractGeminiUsage` 确由 Antigravity 原生流式/非流式路径调用，也供 Gemini 平台复用；其非负用量修复保留，未发现 OpenAI/Codex 供应商调用者。图片限流修复在 `PlatformAntigravity` 分支内；管理端模型列表同样仅修改 Antigravity 分支。

接收前后已用 `git diff --exit-code HEAD --` 核对 `pkg/apicompat`、`service/openai*`、`service/codex*`、`service/billing*`、`service/pricing*`、`service/account.go`、`handler/openai*` 无差异。Codex 相关定向回归和 OpenAI WS relay 测试均独立运行通过。保护范围是现有 Codex/OpenAI 专用链路；本任务选择的 Antigravity 路径自身缺陷修复仍按上表实施。

## 独立检查的合并冲突解决

Master 实际执行以下范围的 `git log --merges --remerge-diff --name-only`，并读取四个非空 remerge-diff：

```text
29b0e4a8a5c0b778583289c4b0fcc715fbd9cd9d..4a4a83ee5ec86b19d3a2dd9b6471de5128963bec
backend/internal/pkg/antigravity
backend/internal/service/antigravity*
backend/internal/pkg/apicompat
backend/internal/handler/gemini_v1beta_handler.go
backend/internal/service/gemini_messages_compat_service.go
backend/internal/service/model_rate_limit.go
```

| 合并提交 | 本地核对结论 |
| --- | --- |
| `afbe51abf8951a7df851fc897cf6a1b4cc33aa01` | 已覆盖：schema 的 const/enum 取交集与 prefixItems 清洗共存。 |
| `ef891643e976c4f27a30725ce7dbb33afb5d6040` | 已覆盖：始终带 toolConfig，同时保留混用函数工具时丢弃内置工具的策略。 |
| `9192426d2e650e805d6cd15d0491fdf523c73a67` | 已覆盖：Responses CreatedAt 与 ServiceTier 共存；本地 `chatcompletions_responses_bridge.go` 与固定上游内容相同。 |
| `d1c6456d0813d4cc119f52731880fbff3c7693ac` | 已覆盖：tool search 发现结果归一化及错误传播；本地 `responses_client_tools.go` 与固定上游内容相同，相关边界测试已存在。 |

此范围之外的合并提交不据此宣称逐项深审。

## Master 发现并要求返工的项目

1. **文本签名聚合遗漏**：首轮 worker 迁移了 Gemini collector，却漏了上游对 `mergeCollectedPartsToResponse` 的修改。原 helper 丢失普通文本的 thoughtSignature，并丢掉仅携带签名的空文本片段。Master 新增独立 Go overlay 回归，在原实现上实际复现两种失败。须将修复和回归纳入仓库，并验证 native Gemini/Claude 两个收集器。
2. **最终模型边界**：首轮在 `ForwardGemini` 无条件补 Gemini dummy signature，与本地兼容路径只对最终 Gemini 目标补签名的合同冲突。原生入口可映射到 Claude；须按最终 mappedModel 判断，用 mock 捕获实际发送内容，覆盖 Gemini、Claude 与已有真实签名。
3. **接收范围收窄**：撤下 COW 实现和本轮新增的 COW 测试，将其保留为独立候选，避免顺带修改其他平台。

第二轮逐项修正。Master 已检查实际 diff 和新增文件，并在主工作区独立验证：两种签名丢失复现均由红变绿；原生 Gemini/Claude collector、最终映射目标与真实签名保留的回归通过；COW 未接收。Master 另补 `TestExtractGeminiUsage` 的“仅报缓存命中”用例，覆盖共享 helper 第四处非负计费修复。

## 已执行的独立验证

日志与监督记录位于本机 `~/.cache/sub4api-antigravity-upstream-20261103/`，不上传 生产环境。

| 检查 | 实际结果 | 证据 |
| --- | --- | --- |
| 改动前 Antigravity、apicompat 包 unit 测试 | PASS；807 个测试及子测试事件，0 失败、0 跳过 | `baseline-1.jsonl`、`baseline-1.exit=0` |
| 改动前 service 定向 unit 测试 | PASS；355 个测试及子测试事件，0 失败、0 跳过 | `baseline-2.jsonl`、`baseline-2.exit=0` |
| 新增独立文本签名回归，运行在原 helper 上 | 按预期 FAIL；普通文本签名缺失，签名专用空文本 parts 长度为 0 | `signed-text-red.jsonl`、`signed-text-red.exit=1` |
| 接收后 Antigravity、apicompat 包 unit 测试 | PASS；813 个测试及子测试事件，0 失败、0 跳过 | `acceptance-packages.jsonl` |
| 接收后 service 定向 unit 测试 | PASS；1,055 个测试及子测试事件，0 失败、0 跳过 | `acceptance-service.jsonl`；范围扩大到 `Antigravity\|Gemini\|ModelRateLimit`，包含 `TestExtractGeminiUsage` |
| 接收后 admin 全包 unit 测试 | PASS；701 个测试及子测试事件，0 失败、0 跳过 | `acceptance-admin.jsonl` |
| 独立文本签名回归，在真实更新后的 helper 上复跑 | PASS；父测试及两子场景共 3 个事件；原普通文本与签名专用空文本均保留签名 | `acceptance-signed-text-green.jsonl`；overlay 仅附加测试，不替换实现 |
| 接收后 pkg/service 定向 race 测试 | PASS；1,422 个测试及子测试事件，0 失败、0 跳过，无 race 报告 | `acceptance-race.jsonl` |
| Codex 保护范围回归 | PASS；94 个测试及子测试事件，0 失败、0 跳过 | `acceptance-codex.jsonl`：context、turn metadata、WS headers/socket/retry/lifetime、透传与 Codex 模型限流 |
| OpenAI WS relay 全包 unit 测试 | PASS；70 个测试及子测试事件，0 失败、0 跳过 | `acceptance-openai-ws-relay.jsonl` |
| gofmt、`git diff --check`、保护路径 diff 检查 | PASS | 22 个本轮 Go 文件均已格式化；保护路径无差异；检查不替代运行测试 |

7 组接收后测试全部退出 0，合计 4,158 个通过事件（**含子测试与重复/race 运行，并非 4,158 个独立测试**），0 失败、0 跳过。2026-11-04 07:53:35–07:57:18 UTC 执行，总计约 3 分 43 秒。Go 源码在该次测试后未再修改；随后仅补齐记录和范围检查。

接收后实际命令（下表 `go` 均为 `~/.local/go1.27.1/bin/go`，工作目录 `backend`）：

```sh
go test -tags=unit -json ./internal/pkg/antigravity ./internal/pkg/apicompat -count=1
go test -tags=unit -json ./internal/service -run 'Antigravity|Gemini|ModelRateLimit' -count=1
go test -tags=unit -json ./internal/handler/admin -count=1
go test -tags=unit -overlay=~/.cache/sub4api-antigravity-upstream-20261103/signed-text-overlay.json -json ./internal/service -run '^TestAntigravitySignedTextAggregationMasterReview$' -count=1
go test -tags=unit -race -json ./internal/pkg/antigravity ./internal/pkg/apicompat ./internal/service -run 'Antigravity|Gemini|ModelRateLimit|Thinking|Malformed' -count=1
go test -tags=unit -json ./internal/service -run 'CodexContextWindow|CodexTurnMetadata|CodexWSCompatibilityMetadata|ApplyOpenAIWSRetryPayloadStrategy|OpenAIWSConnPool_Lifetime|OpenAIGatewayService_BuildOpenAIWSHeaders|Account_IsOpenAIPassthrough|RateLimitService_HandleUpstreamError_Codex' -count=1
go test -tags=unit -json ./internal/service/openai_ws_v2 -count=1
```

监督单元 `sub4api-antigravity-acceptance-20261103.service`：启动后独立存活已确认；终态 `inactive/dead`、`ExecMainStatus=0`。完整启动 argv、每组命令及输出目录见缓存 `acceptance-command.json`，阶段时间与计数见 `acceptance-results.json`；`acceptance.exit=0`。监督命令为：

```sh
systemd-run --user --unit=sub4api-antigravity-acceptance-20261103 --property=WorkingDirectory=~/workspace/sub4api/backend --property=StandardOutput=append:~/.cache/sub4api-antigravity-upstream-20261103/acceptance-supervisor.log --property=StandardError=append:~/.cache/sub4api-antigravity-upstream-20261103/acceptance-supervisor.log -- /usr/bin/python3 ~/.cache/sub4api-antigravity-upstream-20261103/run-acceptance.py
```

**未执行或未完成的范围**：worker 第一轮发起的全量 service suite 随 Bridge 超时中断，未得到完整结果，不能声称全量 service 或全仓检查通过；本轮没有前端、数据库迁移、真实账号生成和部署后冒烟。全部实际测试在本机，生产环境 仅做下面的只读配置核实。

实际基线命令（工作目录 `backend`）：

```sh
~/.local/go1.27.1/bin/go test -tags=unit -json ./internal/pkg/antigravity ./internal/pkg/apicompat -count=1
~/.local/go1.27.1/bin/go test -tags=unit -json ./internal/service -run 'Antigravity|GeminiUsage|ModelRateLimit' -count=1
```

基线由 `sub4api-antigravity-baseline-20261103.service` 持久监督；完整命令和输出目录保存在 `baseline-command.json`。签名复现由 `sub4api-antigravity-signed-text-red-20261103.service` 监督，命令在 `signed-text-red-command.json`，测试通过 Go overlay 放在缓存目录，没有改写主工作区或 worker 的测试源文件。

## 生产环境 只读范围

2026-11-04 07:21:30 UTC，Master 在 生产环境 使用 PostgreSQL `BEGIN READ ONLY` 查询数量和分组平台，随后回滚：

```json
{"antigravity_accounts":1,"active_antigravity_accounts":1,"associated_group_platforms":["antigravity"]}
```

只读取聚合数量和 `groups.platform`，未选择凭据。这给“不扩展复合组路由”提供了实际配置依据。它不是新补丁的真实上游验收；本轮尚未发起真实账号生成测试、上传测试材料或替换服务。

## 执行与接收状态

- Kimi `antigravity-upstream-20261103` 两次因 `Kimi visible output stream offset mismatch` 退出；未形成代码交付，改用用户允许的 ZCode。
- ZCode `antigravity-upstream-zcode-20261103`：实际模型 `account:bigmodel-individual-coding-plan/GLM-5.3`，runtime 档位 `max`、执行模式 yolo；首轮因一小时上限超时，进程清理已确认，实际 diff 保留；第二轮同会话续作，于 07:46:47 UTC 以 Bridge `completed` 结束，清理已确认。
- Bridge 执行完成状态不等于 Master 验收。Master 随后完成实际代码审阅、范围收窄、接收和以上独立测试。本地验收通过；代码已接收到 `~/workspace/sub4api`，未提交、未推送、未发布。
- 本轮最终改动 25 个文件：10 个源文件、12 个测试文件和 3 个审阅/索引文件。接收前 `git apply --check` 通过；精确清单在本机缓存 `receipt-manifest.json` 与 `owned-paths.json`。新文件原被本地 exclude 隐藏，已只对这些文件设置 intent-to-add 以便后续审阅与提交；未改变本地 exclude，也未把本轮内容正式暂存提交。
- 任务前已有的 `DEV_GUIDE.md`、`frontend/src/components/account/__tests__/EditAccountModal.spec.ts` 修改与已暂存的 `backend/dev-docs/upstream-error-proxy-attribution.md` 删除保留，不属于本轮。
- 独立的 Bridge 实现修复/插件安装调查不属于本任务的接收范围，其旁支记录不在本轮复制或清理。

耗时与 token（原始报告保存于本机缓存 `bridge-execution-summary.json`）：ZCode 首轮执行 3,604.263 秒，第二轮 999.529 秒，共约 1 时 16 分 44 秒；独立接收后测试约 3 分 43 秒，人工审阅耗时未单独计量。第二轮 Bridge 的 `queued_ms=3848704` 横跨上一轮，不能当作本次实际排队，故排队时长记未知。Kimi 两次失败执行约 2 分 52 秒。

ZCode runtime 报告输入 **19,006,348**、输出 **23,547**、总量 **19,029,895** token，`finality=reported`、来源 `zcode_runtime_turn`、scope 为同一 session；缓存输入未报告，不据此估算费用或推断跨轮用量。首轮超时及 Kimi 两次均无最终 usage。Codex 主会话 token：**未取得：当前宿主未提供本次调用统计**。

验收责任方：Codex Master。结论仅覆盖上述已接收补丁和本地实测范围；未验证事项已明确列出，无需待用户决定的代码接收项。
