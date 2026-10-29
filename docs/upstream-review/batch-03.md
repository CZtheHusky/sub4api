# 上游逐提交评审：171–253

基线：本地 HEAD `c67360f44`；上游冻结 `a3298b46ebe239fb0720b068a1eb3383955bdac2`；merge-base `3040209f205472038c1ba745a1bedd2edd9053b1`。范围为 batch-03.txt 的83条，编号遵循该清单，不按作者日期重排。

评审方法：每条均读取真实 git show diff，核对相关HEAD实现、共享函数/直接调用点和测试意图；判断基于代码语义，不把patch-id不等价当作未实现。超大变更聚焦手写授权、事务、缓存、计费和UI状态逻辑；Ent/Wire生成文件、lock/sum和宣传资产未逐行/逐像素审查。下述“验证/依赖”是移植后的验收要求，**本次只静态评审，未执行单测、集成、race、完整构建、漏洞扫描或真实性能基准，未调用真实账号/远程服务**。业务价格、模型下线和漏洞公告只引用冻结提交主张，未在线背书。

保留本地 CLI 0.164/`eb6ac2192` 对齐、UUIDv5合成前缀与原生v7、Lite.additional_tools、HTTP/WS metadata、interrupt、Retry-After、WSctx_pool和sticky。196身份策略禁止整包覆盖；188共享缓存须先审本地嵌套数据写入，不能以性能名义移除安全/失效措施。

结论统计：建议引入 32；需适配 40；已覆盖 2；不引入 7；待核实 2。这不是已移植数量。

优先候选：174图片下载SSRF；179/180/194/197/241依赖与工具链安全升级；175/176/181/195/203/231/234计费；182分组生图权限；236/237分页边界；238上传截断；250/251/253非流OAuth协议。232真实价卡和190供应商图片字段冲突待核实。200是新增受限管理员功能，211–228是目录诊断功能链，均需完整适配，不宜逐条盲目cherry-pick。

### 171 `bdfe215cbb06f5089112873b14c34c538fc37118`

原始标题：fix(responses): 网关送出的 Responses 对象一律带 created_at，修复 grok-build 反序列化失败

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_responses_created_at.go` 的 `responsesCreatedAtStamper.Stamp/Observe` 为生命周期对象补时间；`buildOpenAIResponseFailedSSE`、WS HTTP bridge 的失败合成同时补 sequence_number。HEAD 没有 stamper 文件，`handleNonStreamingResponse`、`handleSSEToJSON` 还直接返回恢复后的字节，合成失败帧也没有此 contract。
- **收益/冲突**：修复严格 Responses 反序列化失败。按流固定一个时间且保留上游已有值；透传只观察时间、不能无差别重写全部事件。合并时保留本地 namespace、工具恢复和 CLI 0.164 重试语义。
- **验证/依赖**：移植 created_at 单测及 HTTP/WS 失败帧断言；覆盖已有值逐字保留、null、非 response 对象、失败序号和同流一致性；实际新增测试例：`TestResponsesCreatedAtStamper_StampsMissingAndKeepsStreamStable`、`TestResponsesCreatedAtStamper_PreservesUpstreamValue`。
- **定位**：`backend/internal/service/openai_gateway_passthrough.go`、`backend/internal/service/openai_gateway_response_handling.go`、`backend/internal/service/openai_responses_created_at.go`、`backend/internal/service/openai_ws_http_bridge.go`。

### 172 `71e6b6a9717064b61826d7e5bc480a5f4b50d9cf`

原始标题：test(responses): 模型改写转发测试夹具补齐上游 created_at

- **结论：需适配**。
- **实际代码与HEAD证据**：`openai_response_model_rewrite_test.go:TestMappedResponseModelForwarding` 只给 Responses 夹具增加 `created_at:1700000000`；断言仍是只改 model 的原始字节比较。HEAD 夹具仍缺该字段。
- **收益/冲突**：这是171的测试依赖，单独引入没有运行收益；与166是重复分支上的同一夹具修复，不能重复落地。应随171补齐，避免时间戳正常补全使模型改写测试误报。
- **验证/依赖**：依赖171；运行映射响应转发用例并保留原始模型替换断言。
- **定位**：`backend/internal/service/openai_response_model_rewrite_test.go`。

### 173 `b0a02a3304a0a78cdd003ddda1dd23ba02c03084`

原始标题：chore: sync VERSION to 0.2.17 [skip ci]

- **结论：不引入**。
- **实际代码与HEAD证据**：仅 `backend/cmd/server/VERSION` 从0.2.16改0.2.17，没有函数或测试。HEAD 是sub4api的独立版本线。
- **收益/冲突**：上游发布标识不能代表本地实际移植范围；不改本地版本号。
- **验证/依赖**：无需业务测试；发布时使用本仓库发布流程。
- **定位**：`backend/cmd/server/VERSION`。

### 174 `ab71e12ec46c532839023c013daeca48e9f3e099`

原始标题：fix(images): 异步生图转存的图片下载与同步回填策略对齐

- **结论：建议引入**。
- **实际代码与HEAD证据**：`image_storage.go:defaultImageDownloadHTTPClient/download` 新增共享连接池、逐跳目标检查、直连 socket IP 检查和代理分支DNS检查，并拒绝声明/嗅探均非图片的响应。HEAD 默认客户端仅60秒Timeout，`download`直接Do上游URL；非图片还会落到 `detectImageContentType` 的兜底。
- **收益/冲突**：实际非受控输入是上游图片URL，可导致内网/元数据请求及任意正文转存。安全价值高。代理代解析时本地DNS检查不能保证代理实际连接IP，需明确保留此限制，不能声称完整消除代理DNS重绑定。
- **验证/依赖**：带入重定向到私网、IPv6 zone、代理解析、socket目标、非图片拒绝和异步任务失败断言；核实同步下载策略与已有私网地址工具，不能放松已有边界；实际新增测试例：`TestImageResultUploaderDefaultClientRejectsPrivateURL`、`TestImageTaskServiceCompletePrivateURLMarksFailed`。
- **定位**：`backend/internal/service/image_storage.go`。

### 175 `2b1f129385f4475ffddf8a91b44feceef5101fb6`

原始标题：fix(gemini): 流式中途客户端断开时按已收集用量计费

- **结论：建议引入**。
- **实际代码与HEAD证据**：`gemini_messages_compat_service.go:Forward/ForwardNative/handleStreamingResponse/handleNativeStreamingResponse` 与 chat兼容路径返回clientDisconnect/finished；`geminiStreamAbortedWithBillableOutput`只在取消且有已观测token/图片时保留结果。HEAD `geminiStreamResult`只有usage/firstTokenMs，读错误提前返回错误，Forward无法拿到部分计费结果。
- **收益/冲突**：防客户端中断丢失已发生用量；兼容流不输出图片时中途断开计0图，原生按已交付图片数，结束后断开保留完整用量。不能把活跃客户端的上游读错一律当成功。
- **验证/依赖**：回归断开前后finishReason、无usage取消、真实上游读错、残缺尾行、已交付图片；核对handler只记录一次，和183/184的排空上游策略是不同协议路径；实际新增测试例：`TestGeminiForwardNative_ClientDisconnectAfterFinalChunkBillsCollectedUsage`、`TestGeminiForwardNative_ClientDisconnectWithoutUsageStaysUnbilled`。
- **定位**：`backend/internal/service/gemini_chat_completions_compat_service.go`、`backend/internal/service/gemini_messages_compat_service.go`、`backend/internal/service/gemini_stream_client_abort.go`。

### 176 `9c41b32695c4aa5d2673f228097762cba14e64f0`

原始标题：fix(gemini): prompt 被拦截只报缓存命中时 input_tokens 不再为负

- **结论：建议引入**。
- **实际代码与HEAD证据**：`antigravity/response_transformer.go:buildResponse`、`stream_transformer.go:ProcessLine/emitMessageStart`及`extractGeminiUsage`把prompt-cached下限设0。HEAD四处仍直接减法，例如 `usage.InputTokens = PromptTokenCount - cached` 与 `InputTokens: prompt-cached`。
- **收益/冲突**：上游prompt被拦截可只报缓存命中，当前会形成负输入和错误账单；真实供应商响应边界，不是任意坏值防御。
- **验证/依赖**：带入usage_contract与原生promptBlocked仅缓存token用例；覆盖流/非流并确认缓存读取仍完整保留；实际新增测试例：`TestGeminiUsageMapping_MissingPromptTokenCountKeepsInputNonNegative`、`TestGeminiForwardNative_StreamPromptBlockedWithOnlyCachedTokensKeepsInputNonNegative`。
- **定位**：`backend/internal/pkg/antigravity/response_transformer.go`、`backend/internal/pkg/antigravity/stream_transformer.go`、`backend/internal/service/gemini_messages_compat_service.go`。

### 177 `e068c3d02ff8664c1142d5f7c51a05a82f75ad3d`

原始标题：fix(openai): preserve overload retries after queued and metadata events

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_gateway_passthrough.go:openAIStreamEventIsMetadata`增加response.queued与response.metadata；HEAD switch只有created/in_progress/keepalive。该分类被HTTP透传及WS bridge的首输出/失败转移判断调用。
- **收益/冲突**：排队和审核元数据不能锁死重试或解除首输出超时；否则随后overload无法安全换号。改动小，保留本地真实工具输出边界。
- **验证/依赖**：带入capacity_shed和WS bridge前导metadata后overload用例；与196空delta判断一并验证。
- **定位**：`backend/internal/service/openai_gateway_passthrough.go`。

### 178 `7fcaa32db4940ff9ca42b961b00c453b496d8d25`

原始标题：fix(accounts): persist platform and status filters

- **结论：建议引入**。
- **实际代码与HEAD证据**：`AccountsView.vue:loadInitialAccountFilterValues/saveAccountFilterValues`将platform/status保存在localStorage，接入useTableLoader.initialParams和debouncedReload。HEAD这两个初始参数仍为空，也没有account-filter-values键。
- **收益/冲突**：改善刷新后筛选丢失；沿用CONCRETE_PLATFORM_OPTIONS验证合法平台，不保存请求之外的筛选。
- **验证/依赖**：提交未增加专门测试；用组件或浏览器验证恢复、失效平台/状态回退、禁用storage时筛选仍可用。
- **定位**：`frontend/src/views/admin/AccountsView.vue`。

### 179 `cacc80b8620e11e9655ab307471f71eb9ee31a0e`

原始标题：fix(security): clear dependency security scan failures

- **结论：需适配**。
- **实际代码与HEAD证据**：`backend/go.mod`升Go1.27.3，`security-scan.yml`精确检查同步；`frontend/package.json`+锁新增follow-redirects>=1.16.1。HEAD Go1.27.0、锁follow-redirects1.16.0，真实axios依赖仍用该版本。
- **收益/冲突**：安全更新有价值，但HEAD模块名是github.com/MACOS-DO/sub4api，不能复制上游go.mod；与180/194/241组成一致升级，DOMPurify选197最终版。
- **验证/依赖**：离线静态核验版本缺口，未执行govulncheck、包审计或确认公告可达性；落地需依赖解析、前端构建及安全扫描。
- **定位**：`.github/workflows/security-scan.yml`、`backend/go.mod`、`frontend/package.json`。

### 180 `4c940c3cb7069d23cd304fc378f29ffae1ceb80f`

原始标题：fix(ci): synchronize Go patch version checks

- **结论：需适配**。
- **实际代码与HEAD证据**：`backend-ci.yml`单元/lint和`release.yml`验证Go1.27.3；HEAD backend-ci检查1.27.0。这里仅版本grep，没有业务代码。
- **收益/冲突**：仅在本地go.mod、Docker构建工具链同步升级时纳入，不能先改检查让现有CI必失败。
- **验证/依赖**：依赖179/194；同时检查本仓库所有工具链来源和发布工作流，不凭上游绿色状态宣称本地通过。
- **定位**：`.github/workflows/backend-ci.yml`、`.github/workflows/release.yml`。

### 181 `d6120b5d8e05fe4edf54f1575c4dd6e5c7beb856`

原始标题：feat(account): Vertex 账号支持配额上限

- **结论：需适配**。
- **实际代码与HEAD证据**：`account.go:AccountTypeSupportsQuota/SupportsAccountQuota`把Vertex service_account加入配额，调度、DTO、统一计费仓库与legacy账单、提醒和前端accountQuota.ts联动。HEAD `IsSchedulable`、`shouldUpdateAccountQuota`、`applyUsageBillingEffects`仍只认APIKey/Bedrock。
- **收益/冲突**：有实际漏计/不停调度价值，但必须分离配额类型和池模式类型，不能把IsAPIKeyOrBedrock整体扩宽。还修复账号删除时配额缺行不阻断用户扣费，需保留事务语义。
- **验证/依赖**：带入service_account配额累加、超限调度、DTO/UI、无上限不显示及删除账号仍扣余额断言；检查本地Vertex类型、配额outbox刷新和通知一次性；实际新增测试例：`TestAccountFromServiceShallow_ServiceAccountQuota`、`TestAccountFromServiceShallow_APIKeyWithoutLimitOmitsQuotaUsed`。
- **定位**：`backend/internal/handler/dto/mappers.go`、`backend/internal/repository/usage_billing_repo.go`、`backend/internal/service/account.go`、`backend/internal/service/gateway_scheduling.go`、`backend/internal/service/gateway_usage_billing.go` 等（实际 18 个变更文件）。

### 182 `47cb84b8b047ff883d3ab6214151c96ff8b93584`

原始标题：fix(gemini): 分组「允许当前分组生图」开关对 Gemini 系入口生效

- **结论：需适配**。
- **实际代码与HEAD证据**：新增handler/service `gemini_image_generation_gate.go`，入口和映射后按模型名/responseModalities判定；HEAD没有两文件，Gemini ForwardNative及Antigravity ForwardGemini映射后无分组生图拒绝。
- **收益/冲突**：当前UI开关对Gemini路径缺强制约束。需覆盖messages/chat/responses/native及映射成图片模型。迁移246会把所有Gemini/Antigravity关闭开关改true；这会改变运营者设置，不能无审查照搬。
- **验证/依赖**：核对分组默认值、已有显式关闭意图与迁移编号；带入选择账号前拒绝、映射后拒绝、IMAGE字符串/整数/重复键及countTokens豁免测试；实际新增测试例：`TestGatewayHandlers_GeminiImageGenerationDisabledGroupRejectsBeforeSelection`、`TestGatewayHandlers_GeminiImageGenerationGateSkipsGroupsWithoutTheSwitch`。
- **定位**：`backend/internal/handler/gateway_handler.go`、`backend/internal/handler/gateway_handler_chat_completions.go`、`backend/internal/handler/gateway_handler_responses.go`、`backend/internal/handler/gemini_image_generation_gate.go`、`backend/internal/handler/gemini_v1beta_handler.go` 等（实际 22 个变更文件）。

### 183 `94c7d6b025a4074efe34b444e7ea218c85409dd6`

原始标题：fix(gateway): Anthropic 账号 chat/responses 兼容流式客户端断开后读完上游再计费

- **结论：建议引入**。
- **实际代码与HEAD证据**：`gateway_forward_as_chat_completions.go:handleCCStreamingFromAnthropic`和`gateway_forward_as_responses.go:handleResponsesStreamingResponse`写失败后标记断开，继续经line pump读上游并合并message_delta用量。HEAD写Chunk失败仍提前退出，没有兼容流独立排空/ClientDisconnect结果。
- **收益/冲突**：避免Anthropic账号兼容出口因客户端断开漏收最终usage。必须有间隔超时，防排空永远等待；和184一起选最终状态。
- **验证/依赖**：带入两协议断开后最终usage、idle上游时间上限回归；检查detachUpstreamContext及handler提交计费链；实际新增测试例：`TestAnthropicCompatStreaming_IdleUpstreamEndsWithinInterval`、`TestHandleCCStreamingFromAnthropic_ClientDisconnectDrainsUpstreamUsage`。
- **定位**：`backend/internal/service/gateway_forward_as_chat_completions.go`、`backend/internal/service/gateway_forward_as_responses.go`。

### 184 `4355236e00230de65226ad6ba710cffd1a512473`

原始标题：fix(gateway): 兼容流上游未正常结束时报错并对齐原生收尾

- **结论：需适配**。
- **实际代码与HEAD证据**：新增 `gateway_forward_compat_stream.go` 统一错误/terminal处理，兼容流缺message_stop、读错/超时报错；handler chat/responses错误结果非nil也提交usage；复用 `sseStreamErrorEventFailover`。HEAD原生gateway_forward已经返回部分usage；Responses handler的submitResponsesUsage(result)已覆盖错误路径计费，不能重复加；ChatCompletions handler仍在error分支直接返回，兼容流helper/terminal检查缺失。
- **收益/冲突**：修复假成功和部分漏计，范围跨handler/service/pump。必须把无输出overload换号(result=nil)与有输出中断(result+error)区分，不能重复扣费。
- **验证/依赖**：随183带入EOF缺terminal、DONE、超时、error在输出前后、错误路径已观测usage且仅一次记录；保留本地原生终止及WS行为；实际新增测试例：`TestGatewayCompatibilityHandlers_StreamUsageRecordedOnceWithTerminalEvent`、`TestGatewayCompatibilityHandlers_IdleBeforeAnyEventWritesStreamError`。
- **定位**：`backend/internal/handler/gateway_handler_chat_completions.go`、`backend/internal/handler/gateway_handler_responses.go`、`backend/internal/service/gateway_forward.go`、`backend/internal/service/gateway_forward_as_chat_completions.go`、`backend/internal/service/gateway_forward_as_responses.go` 等（实际 12 个变更文件）。

### 185 `d5dbb5c36be3e2b76e7f0197e6ef7f3f214df6fc`

原始标题：perf(gateway): 单 antigravity 账号分组判定改为按需求值

- **结论：建议引入**。
- **实际代码与HEAD证据**：`gateway_scheduling.go:WithSingleAntigravityAccountGroupProbe`挂延迟闭包，`request_metadata.go`用sync.Once记结果，`isSingleAccountRetry`显式标记优先。HEAD handler还在每次请求前调用IsSingleAntigravityAccountGroup列账号。
- **收益/冲突**：免去非Antigravity路径额外调度快照查询；只在503或预检查需要时判一次。注意metadata更新不能丢失probe，groupID要挂载时复制。
- **验证/依赖**：上游测试覆盖取消context、显式false覆盖、同请求一次及组ID变化；落地核对本地metadata bridge、单账号503退避及sticky路径；实际新增测试例：`TestGeminiGroupRequestsDoNotReadAntigravityBucket`、`TestMessagesAnthropicGroupDoesNotReadAntigravityBucket`。
- **定位**：`backend/internal/handler/gateway_handler.go`、`backend/internal/handler/gemini_v1beta_handler.go`、`backend/internal/service/antigravity_gateway_retry.go`、`backend/internal/service/gateway_scheduling.go`、`backend/internal/service/request_metadata.go`。

### 186 `89654873ff4b8d586e333a1dd21d194aa82facf0`

原始标题：perf(account): 模型映射签名改为与遍历顺序无关、零分配

- **结论：需适配**。
- **实际代码与HEAD证据**：`account.go:modelMappingSignature`由排序键+fnv.New64a改成逐条FNV-1a混合后求和。HEAD仍make keys/sort.Strings，已是遍历顺序无关，但每次有分配。
- **收益/冲突**：这里只是零分配性能收益，标题中的顺序无关不是本地缺陷。已有签名不得顺手删除；在实际调度热路径测量后再选，避免为性能引入未测碰撞/复杂度。
- **验证/依赖**：上游新增顺序、内容交换、非string标记和零分配断言；需本地benchmark验证分配与吞吐，静态评审未测量；实际新增测试例：`TestModelMappingSignature_IndependentOfInsertionOrder`、`TestModelMappingSignature_DetectsContentChanges`。
- **定位**：`backend/internal/service/account.go`。

### 187 `38b72d734e0aa92ce7547a5ea8618abb7290cf98`

原始标题：fix(ratelimit): 模型限流快照写入改为写时复制

- **结论：建议引入**。
- **实际代码与HEAD证据**：`model_rate_limit.go:setAccountModelRateLimitSnapshot`改为复制Extra及model_rate_limits再替换。HEAD仍就地写`limits[scope] = payload`，且浅复制Account的map可共享。
- **收益/冲突**：防限流写入污染其它请求快照/并发map写；188共享只读母本的必要前置，独立也降低浅复制别名风险。
- **验证/依赖**：带入COW、旧map不变、并发副本race测试；核对所有调用方只变更本次账号快照，不能丢现有scope；实际新增测试例：`TestSetAccountModelRateLimitSnapshot_CopyOnWrite`、`TestSetAccountModelRateLimitSnapshot_InitializesMissingOrInvalidLimits`。
- **定位**：`backend/internal/service/model_rate_limit.go`。

### 188 `53002f677c3d8e182ba11858e00b301079107f6e`

原始标题：perf(scheduler): 调度快照解码结果常驻进程内存

- **结论：需适配**。
- **实际代码与HEAD证据**：`repository/scheduler_cache.go:GetSnapshot`加入bucket常驻已解码母本、ready/active/全局/平台revision校验、最长复用/淘汰、每次更新last_used；`Account.PrepareSharedSnapshot`绕过重复签名。HEAD没有snapshotEntry/PrepareSharedSnapshot，仍从Redis读并解码精简记录。
- **收益/冲突**：可减少解码/分配，但这是缓存一致性大改；写端SetAccount/DeleteAccount/批量writeAccountIDs、平台变更和部分失败全局revision必须同时接入。保留本地bucket epoch/tombstone、sticky与WSctx_pool行为，不单搬读缓存。 本地BPS、审计等扩展也须追踪嵌套map/slice写入，只有已证实发布后只读才可共享；未完成全仓mutation审计前不可直接引入。
- **验证/依赖**：依赖187。审过手写revision/复制/失效逻辑，未逐行跑性能或并发测试；需Redis集成/race及跨bucket平台变更、退休重开、失败pipeline、last_used新鲜度和真实benchmark；实际新增测试例：`TestSchedulerCacheSnapshotMemoSecondReadOnlyFetchesTagsAndLastUsed`、`TestSchedulerCacheSnapshotMemoLastUsedIsFreshAndCopiesAreIndependent`。
- **定位**：`backend/internal/repository/scheduler_cache.go`、`backend/internal/service/account.go`、`backend/internal/service/scheduler_cache.go`。

### 189 `67dca9687eddcd4a21d44b354c2afe2d921942a6`

原始标题：perf(billing): 账号配额累加按已设置的周期维度选择语句

- **结论：需适配**。
- **实际代码与HEAD证据**：`usage_billing_repo.go:incrementUsageBillingAccountQuota`按daily/weekly提示选4条SQL，省略维度必须在数据库WHERE确认未启用，不匹配退完整语句；`buildUsageBillingCommand`新增两提示。HEAD仍一条完整UPDATE，事务内累加所有维度并发布额度outbox。
- **收益/冲突**：纯性能优化不修本地准确性缺口，SQL复杂度明显增加；必须保留配置并发变化时数据库为准和deleted账号处理，提示不得进入幂等指纹。
- **验证/依赖**：结合181而非孤立复制签名；需SQL四变体与完整语句等价、错提示fallback、金额幂等和额度crossing集成测试，未测收益；实际新增测试例：`TestIncrementUsageBillingAccountQuota_StatementVariantsMatchFullStatement`、`TestAccountQuotaIncrementSQLs_GuardOmittedDimensions`。
- **定位**：`backend/internal/repository/usage_billing_repo.go`、`backend/internal/service/gateway_usage_billing.go`、`backend/internal/service/usage_billing.go`。

### 190 `afbef7c8cca571660d6ca66a211d36ad83ea46ff`

原始标题：fix(deepseek): avoid duplicate image payloads

- **结论：待核实**。
- **实际代码与HEAD证据**：`openai_gateway_request_body.go:aliasDeepSeekResponsesImagePart/extractDeepSeekResponsesImageURL`从image_url+url双写改只image_url，并优先image_url。HEAD仍双写url，注释明确记录线上serde曾要求url（422 missing field url）。
- **收益/冲突**：这是互斥协议假设，无法把“官方新形状”与当前兼容路径混合。需离线已有真实响应/兼容工件证明确认应更新哪一种；没有证据不移除url。
- **验证/依赖**：上游改了输入图片测试但不能证明当前部署供应商版本；只检查mock不能关闭该疑点。保留file_id、嵌套形态和重试不污染原body的断言。
- **定位**：`backend/internal/service/openai_gateway_request_body.go`。

### 191 `c7bfcd93aa206065238b67757d5d8fae81cf697b`

原始标题：fix(tools): normalize parameter constants and annotations

- **结论：建议引入**。
- **实际代码与HEAD证据**：`gemini_messages_compat_service.go:cleanToolSchemaNode`区分schema节点和数据节点，字符串const与enum取交集，移除examples但保留名叫examples/const的属性和default数据。HEAD `cleanToolSchema`对所有map盲目递归删关键字，没有schemaNode分派。
- **收益/冲突**：修复工具参数约束丢失/合法属性被删。必须保留非字符串常量类型，不把常量字符串化；这是受支持schema结构分派。
- **验证/依赖**：带入schema关键字命名属性、嵌套default、const/enum相交及oneOf/各schema子节点回归；核对调用方工具转换；实际新增测试例：`TestCleanToolSchema_PreservesSchemaKeywordNamedProperties`、`TestCleanToolSchema_NormalizesConstAndExamplesAtNestedSchemaNodes`。
- **定位**：`backend/internal/service/gemini_messages_compat_service.go`。

### 192 `e0fa4b94f9b962949da039c09e63968358324e26`

原始标题：fix(ratelimit): avoid penalizing Cloudflare 1010 blocks

- **结论：建议引入**。
- **实际代码与HEAD证据**：`ratelimit_service.go:handle403`将Cloudflare1010豁免上移所有平台，并用JSON结构识别cloudflare_error+code/error_code或browser_signature_banned。HEAD豁免仍在handleOpenAI403，且isCloudflareBotBlockResponse只找HTML字串。
- **收益/冲突**：防Anthropic/Antigravity等边缘拦截永久禁号；必须确保普通JSON code1010没有Cloudflare标记不获豁免，真实403仍按策略罚。
- **验证/依赖**：带入跨平台HTML/嵌套JSON及真实403反例测试，检查状态/strike/冷却没有被误修改；实际新增测试例：`TestHandleUpstreamError_Cloudflare1010DoesNotPenalizeAccount`、`TestHandleUpstreamError_Cloudflare1010JSONDoesNotPenalizeAccount`。
- **定位**：`backend/internal/service/ratelimit_service.go`。

### 193 `7da09cab4e875463baa79fc150adde700c5438df`

原始标题：chore: update sponsors

- **结论：不引入**。
- **实际代码与HEAD证据**：仅README三语赞助商内容和chenju_ai.png，未改网关/配置/测试。
- **收益/冲突**：上游商业展示与本地功能无关，不复制赞助信息。
- **验证/依赖**：文案/二进制图片未作视觉审核；无业务测试需求。
- **定位**：`README.md`、`README_CN.md`、`README_JA.md`。

### 194 `59100d324e9fafa6776182b3d3ea2b36e3edaf14`

原始标题：fix(build): 对齐上游 Go 1.27.3 镜像版本

- **结论：需适配**。
- **实际代码与HEAD证据**：`Dockerfile:GOLANG_IMAGE`从1.27.2-alpine升1.27.3-alpine；HEAD仍1.27.0-alpine。
- **收益/冲突**：应与179/180的go.mod/CI工具链一致升级，单独升级或复制全部Dockerfile都无必要。
- **验证/依赖**：只改所需ARG后验证镜像可用与构建；本次未下载/构建镜像。
- **定位**：`Dockerfile`。

### 195 `1cdecf4a88f016115392033bdbe0662d7ff92e81`

原始标题：fix(billing): Grok 4.7 Fast 按官方 Fast 价卡计费 (#8119)

- **结论：建议引入**。
- **实际代码与HEAD证据**：`billing_service.go:initFallbackPricing/getFallbackPricing`单列grok-4.7-fast与grok-4.7-build-fast，输入/缓存/输出4/1/12美元每百万，200k以上1.5倍；嵌入JSON同步。HEAD没有这两Fast专属分支，会落未知Grok家族价卡。
- **收益/冲突**：修复Fast账单低估，fallback和目录必须一致；模型response审计/别名不能把Fast退回普通4.7。
- **验证/依赖**：带入199999/200000/更长上下文和缓存成本测试，确认分组/渠道自定价仍优先。未在线核对官方价卡，金额按冻结diff评审；实际新增测试例：`TestGetModelPricing_Grok47FastOfficialFallback`、`TestCalculateCost_Grok47FastUsesFastCard`。
- **定位**：`backend/internal/service/billing_service.go`、`backend/resources/model-pricing/model_prices_and_context_window.json`。

### 196 `c591412c42ab195dacdede9eb19f9cbfee221144`

原始标题：fix(openai): 合并 Codex overload 身份与空流安全重试修复

- **结论：需适配**。
- **实际代码与HEAD证据**：`NormalizeCodexClientIdentityToCLI`强制TUI originator+UA首段为CLI，`resolveCodexOutboundIdentity`接入；另扩展空delta/done不算可见输出和request-scoped Retry-After delay。HEAD仍默认codex-tui，但已对齐0.164，`openai_retry_after.go`支持嵌套/顶层事件头且优先事件建议，capacity sanitization保留带Retry-After原始code。
- **收益/冲突**：不能整包覆盖本地CLI0.164/UUIDv5合成前缀保留原生v7/Lite.additional_tools/HTTP与WS metadata/interrupt。该diff上下文0.146.0不是0.164；改默认身份是产品策略变化，风险分层论据来自旧上游问题，需单独验收。可独立吸收空字符串delta/done安全重试；毫秒头补充不能覆盖本地更完整的优先级和deadline。
- **验证/依赖**：带入空内容后overload可重试、真实工具完成不重放用例；与本地openai_codex_164_test、Retry-After、WS resume和identity配对测试联合验证。身份归一化待用户策略/当前协议证据，禁止降版本或删除本地重试实现；实际新增测试例：`TestNormalizeCodexClientIdentityToCLI`、`TestNormalizeCodexClientIdentityToCLIIsPairedAndIdempotent`。
- **定位**：`backend/internal/pkg/openai/request.go`、`backend/internal/service/openai_codex_identity.go`、`backend/internal/service/openai_gateway_passthrough.go`、`backend/internal/service/openai_gateway_service.go`、`backend/internal/service/openai_gateway_upstream_errors.go` 等（实际 16 个变更文件）。

### 197 `8286268047cbbca4e7f5fbb63ff0890c1ea37cf5`

原始标题：fix(deps): bump dompurify to 3.4.17 for GHSA-cg2g-xh64-v4q7 / GHSA-mrv2-3pqr-j5jr

- **结论：建议引入**。
- **实际代码与HEAD证据**：`frontend/package.json`及pnpm锁把DOMPurify从3.4.14升3.4.17并收紧override。HEAD锁仍3.4.14；`frontend/src/utils/sanitize.ts:sanitizeSvg`真实调用DOMPurify，安全更新有实际消费方。
- **收益/冲突**：比179中的旧最低版本优先。按当前锁重新解析最小依赖更新，保留本地其它override，不直接覆盖上游lock。
- **验证/依赖**：未联网复核GHSA细节/跑包审计；需安全扫描、前端构建及已有SVG/Markdown净化调用验证，不声称仅升级就覆盖全部XSS。
- **定位**：`frontend/package.json`。

### 198 `ac1b8f1271be0d674fa6f074403c1410f30a3a81`

原始标题：chore: update sponsors

- **结论：不引入**。
- **实际代码与HEAD证据**：仅README三语赞助展示与ooko.ai.png，未改可执行代码。
- **收益/冲突**：不关联本地功能或安全边界，不复制。
- **验证/依赖**：未逐像素审核宣传资产；无运行测试。
- **定位**：`README.md`、`README_CN.md`、`README_JA.md`。

### 199 `5c185c5c5d4f5e409e2139806e0964ae944e03e8`

原始标题：chore: sync VERSION to 0.2.18 [skip ci]

- **结论：不引入**。
- **实际代码与HEAD证据**：仅`backend/cmd/server/VERSION`由0.2.17升0.2.18。
- **收益/冲突**：本地独立发布线，不因选择性移植冒用上游版本。
- **验证/依赖**：无需业务测试。
- **定位**：`backend/cmd/server/VERSION`。

### 200 `e445cc3e2d9c9f330f2af069d6693bfdeb2245ad`

原始标题：新增按操作与资源范围授权的管理员 API 密钥

- **结论：需适配**。
- **实际代码与HEAD证据**：新增`adminaccess.Policy`按同一rule匹配操作/分组/平台/账号类型；`admin_auth.go`识别adm_sc_；service生成随机密钥仅存SHA256摘要、有效期/IP/CIDR、撤销/轮换CAS；repository受限SQL投影和事务锁内重验范围；`AfterAccountCommit`延后缓存/外部副作用。HEAD没有adminaccess/custom_admin_keys，AdminAuth只有传统key/JWT路径。
- **收益/冲突**：是完整受限管理员API新功能，非现有边界小补丁。审过Policy.Matches/AllowsGroup、JWT-only生命周期+二次验证、scope SQL、账户行锁/关联差量、usage DTO白名单与导入权限。不得只加中间件而遗漏每个route/handler读写、代理、导入、审计和响应脱敏；本地新增端点需逐条拒绝默认。无明确使用需求可暂缓，不将扩大管理入口当紧急安全修复。
- **验证/依赖**：约1万行diff，重点展开手写授权/事务/密钥/投影，wire生成文件未逐行审。依赖迁移247、DI、限流、审计、前端设置；需权限组合不可拼接、跨组越权、JWT-only key管理、原子导入/回滚/并发轮换、secret redaction及Postgres集成。静态评审不能证明无越权；实际新增测试例：`TestScopedAdminAccountDTORedactsSecretsAndForeignGroups`、`TestScopedAdminAccountErrorsAreClassifiedAndRedacted`。
- **定位**：`.gitignore`、`backend/cmd/server/wire_gen.go`、`backend/internal/handler/admin/account_handler.go`、`backend/internal/handler/admin/announcement_handler.go`、`backend/internal/handler/admin/custom_admin_key_accounts.go` 等（实际 92 个变更文件）。

### 201 `3f1c8f36ce8c2cd378015830a90e89970b9a9ea7`

原始标题：chore: update Sponsors

- **结论：不引入**。
- **实际代码与HEAD证据**：仅三语README、赞助商logo与wandapi广告SVG，未改服务实现。
- **收益/冲突**：本地无需继承上游商业资产。
- **验证/依赖**：未视觉审核SVG/logo；无业务测试。
- **定位**：`README.md`、`README_CN.md`、`README_JA.md`。

### 202 `fd998129ec8375c6623e061c7e5e391752dc954a`

原始标题：feat(billing): support per-image billing for multi-image requests

- **结论：需适配**。
- **实际代码与HEAD证据**：`Group.ImageCountBilling`从schema/迁移248贯通DTO、仓库、auth缓存、分组复制/UI；图片入口调用 `resolveBillableImageCount(reported,requested)`，账单开关优先于渠道token价。HEAD没有字段、helper和迁移，仍原来的渠道token优先策略。
- **收益/冲突**：按张计费是新收费策略，不能只移植helper或默认切换现有账单。此初版把请求n当兜底，随后203/205已修正，应只选最终组合。
- **验证/依赖**：审手写schema/handler/repo/cache/billing/UI，Ent生成文件未逐行审核。与203–206整体验证迁移、缓存往返/旧快照默认false、API contract、跨协议入口、0/1/多图及自定义价优先级；实际新增测试例：`TestResolveBillableImageCount`。
- **定位**：`backend/internal/handler/admin/group_handler.go`、`backend/internal/handler/dto/mappers.go`、`backend/internal/handler/dto/types.go`、`backend/internal/repository/api_key_repo.go`、`backend/internal/repository/group_repo.go` 等（实际 33 个变更文件）。

### 203 `1487fa511bffbb26ebcebb82a717264c84ffb3a0`

原始标题：fix(billing): count only images returned in upstream response

- **结论：需适配**。
- **实际代码与HEAD证据**：`image_billing_count.go:resolveBillableImageCount`撤销请求n兜底，`forwardOpenAIImagesAPIKey/OAuth`以响应data/output的实际数优先。HEAD APIKey `imageCount := parsed.N`，仅当nonStreamCount>0覆盖；helper缺失。
- **收益/冲突**：修复请求3张实返1张却按3张计费的风险。若不引入202开关，也可独立最小修正实际计数，但必须明确失败/无图成功的兜底策略，避免把所有未知都收费。
- **验证/依赖**：上游OfficialMultiImageResponse测试检查data数组实际3图。依赖最终205规则决定未知数是否允许按请求n回退；覆盖n>实返、流/非流/OAuth、空data及工具输出；实际新增测试例：`TestCountOpenAIImageOutputs_OfficialMultiImageResponse`。
- **定位**：`backend/internal/service/image_billing_count.go`、`backend/internal/service/openai_images.go`、`backend/internal/service/openai_images_responses.go`。

### 204 `cb2959245f7ab452e152a2e77bf811f9be66cbfa`

原始标题：test(api): include image_count_billing in available groups contract

- **结论：需适配**。
- **实际代码与HEAD证据**：`server/api_contract_test.go`仅将image_count_billing加入available-groups响应期望。HEAD公开分组DTO无字段，故不能先改测试宣称API已具备功能。
- **收益/冲突**：属于202新字段的契约补齐，只有完整字段链实现后有价值。
- **验证/依赖**：依赖202/205；执行可用分组API contract与auth缓存旧快照用例。
- **定位**：`backend/internal/server/api_contract_test.go`。

### 205 `aa1a1b11c864a009ddd83e1f9010b0f92bd1da0b`

原始标题：fix(billing): gate per-image billing by endpoint and pricing mode

- **结论：需适配**。
- **实际代码与HEAD证据**：新增 `ImageEndpoint` 标志及 `imageCountBillingApplies` 将强制按张限于OpenAI/composite、已允许生图、图片入口；mixed chat/responses token请求不强制。`resolveBillableImageCount`按开关/endpoint处理未知计数。HEAD只有ImageCount，没有这些选择字段/函数。
- **收益/冲突**：修正202把带图片的混合请求误转按张收费；扩散到ForwardResult/OpenAIForwardResult、各图片入口、两个计费service和分组清理规则。不能只移开关。
- **验证/依赖**：与202/203最终状态一起取；上游switch用例覆盖group/channel token价、混合请求、平台与许可归一化；保留本地分组与渠道优先级和api缓存版本；实际新增测试例：`TestImageCountBillingApplies`、`TestCalculateRecordUsageCost_ImageCountBillingGroupTokenPricing`。
- **定位**：`backend/internal/service/antigravity_gateway_gemini.go`、`backend/internal/service/api_key_auth_cache_impl.go`、`backend/internal/service/gateway_service.go`、`backend/internal/service/gateway_usage_billing.go`、`backend/internal/service/gemini_chat_completions_compat_service.go` 等（实际 17 个变更文件）。

### 206 `3ce09c8aff085ce42538d539d515742d8650e6f0`

原始标题：fix(billing): keep tokens for channel per-request image tiers

- **结论：已覆盖**。
- **实际代码与HEAD证据**：`gateway_usage_billing.go:calculateImageCost`上游把UsageTokens提到channel模式switch前并传给按次CostInput。HEAD已直接为渠道来源统一构建tokens并传入`Tokens: tokens`，包含Input/Output/ImageOutput及RequestCount/SizeTier。
- **收益/冲突**：按次定价缺尺寸档时用上下文档匹配的关键语义已在本地，不能因新增文本不相同就重复移植重排。若采202–205重构，该能力应保留。
- **验证/依赖**：核对调用CalculateCostUnified和tier选择；可复用上游ChannelPerRequestContextTierUsesTokens回归，当前评审未运行；实际新增测试例：`TestCalculateRecordUsageCost_ChannelPerRequestContextTierUsesTokens`。
- **定位**：`backend/internal/service/gateway_usage_billing.go`。

### 207 `34236674223877d265bb5dbc071277d04ded0d34`

原始标题：feat(openai): support GPT-6.1 Terra

- **结论：需适配**。
- **实际代码与HEAD证据**：`IsGPT61TerraModelSpelling`、`RequiresResponsesForToolCalls`新增Terra别名/effort识别、模型目录、Codex manifest能力、fallback价/JSON与UI默认模型；HEAD只有6.1-sol支持，未含6.1-terra完整链。
- **收益/冲突**：新模型支持不是只加名称：工具调用需走Responses，none与max能力、priority/cache/长上下文定价必须一起确认。保留本地0.164传输和additional_tools支持。
- **验证/依赖**：带入Terra别名/effort、真实manifest能力不被fallback覆盖、Chat工具拒绝及定价测试；金额/模型可用性未在线查证，落地前按当前实际目录确认；实际新增测试例：`TestGPT61TerraIdentityAndEffort`、`TestGPT61TerraAccountDiscoveryPreservesCatalog`。
- **定位**：`backend/internal/pkg/apicompat/anthropic_to_responses.go`、`backend/internal/pkg/openai/constants.go`、`backend/internal/service/billing_service.go`、`backend/internal/service/openai_codex_models_service.go`、`backend/internal/service/openai_codex_transform.go` 等（实际 18 个变更文件）。

### 208 `014cc016f9a01b0b5ee61493cbb0ff282e81d7ac`

原始标题：fix(grok): 补全原生 Responses 响应模型审计

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_gateway_grok.go:forwardGrokResponses`将observedUpstreamResponseModel及Conflict写入OpenAIForwardResult。HEAD同函数构造结果只有请求model/upstreamModel，虽共享响应handler观测了实际model，结果未携带。
- **收益/冲突**：修复原生Grok Responses审计和按响应模型计费的证据链，不能把请求映射型号当供应商实际声明。
- **验证/依赖**：带入native JSON/SSE/WS、模型冲突、失败尝试隔离与未知更高型号不误计用例；核对RecordUsage审计字段消费者；实际新增测试例：`TestGrokResponseAuditNativeJSON`、`TestGrokResponseAuditNativeProtocols`。
- **定位**：`backend/internal/service/openai_gateway_grok.go`。

### 209 `7131b997ca0bb5fb68459ced150a1428d3107935`

原始标题：fix(grok): 补齐聚合路径跨事件模型冲突

- **结论：需适配**。
- **实际代码与HEAD证据**：`openai_gateway_messages.go:readOpenAICompatBufferedTerminal`收集非terminal事件model，两个buffered兼容出口在最终model前应用；HEAD函数只返回最终response/usage/acc/error，中间声明被丢弃。
- **收益/冲突**：补Grok跨事件模型冲突证据。但此初版按事件累加slice，210马上改有界状态，应直接选210最终实现。
- **验证/依赖**：依赖208审计链，连210；聚合中early=A/final=B标冲突、同型号别名、非Grok保持terminal-only，不能只测最后一帧；实际新增测试例：`TestGrokResponseAuditConvertedAggregateConflict`、`TestGrokResponseAuditConvertedAggregateModelBoundaries`。
- **定位**：`backend/internal/service/openai_gateway_chat_completions.go`、`backend/internal/service/openai_gateway_messages.go`。

### 210 `d01f2eb42cf4dccc22690acc849be2d6ce54f118`

原始标题：fix(grok): 固定聚合前序模型审计并跳过非 Grok

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openAICompatPriorModelAudit`只对Grok启用内部observer，保存首个有上限的model及冲突位；reader新增account参数，chat/messages调用同步更新。HEAD没有这个状态，当前buffered读取会遗漏早期模型。
- **收益/冲突**：替代209无界models slice；模型声明再多也固定内存，并保持非Grok审计行为不扩散。
- **验证/依赖**：和208一起适配reader所有直接调用方/测试，带入大量前序model有界性、非Grok终止模型及buffered失败路径回归；实际新增测试例：`TestGrokResponseAuditAggregatePriorStateStaysBounded`。
- **定位**：`backend/internal/service/openai_gateway_chat_completions.go`、`backend/internal/service/openai_gateway_messages.go`。

### 211 `6202da0f53b0ed41d889a66c03a61d2854aea0f9`

原始标题：fix(grok): 统一模型目录解析与观测状态

- **结论：需适配**。
- **实际代码与HEAD证据**：新增`grok_model_catalog.go:parseGrokCatalogModelIDs/NewGrokCatalogScope/MergeGrokCatalogObservation`，统一手动/后台解析及带source scope、成功开始时间/最后尝试的合并。HEAD `grok_observed_models.go`快照只有Models/FetchedAt/Source，extract用gjson字符串兜底，手动upstream_models另一路解析。
- **收益/冲突**：可避免失败覆盖成功、乱序观察回退和换源借用旧目录。但包含endpoint+账号身份的SHA256 fingerprint及较多验证/clone helper；按用户规则14/15不能仅因上游存在就照搬：需证明普通结构字段比较/事务/测试不足，优先保留实际一致性需求的最小表达。
- **验证/依赖**：需与212/214/221成组适配，解析裸数组/data/models、空/混合坏元素、重复ID、乱序成功/失败、不改变输入；旧快照兼容。当前未批准引入新增hash设计；实际新增测试例：`TestGrokCatalogParserParity`、`TestGrokCatalogParserRejectsBadInput`。
- **定位**：`backend/internal/service/grok_model_catalog.go`、`backend/internal/service/grok_observed_models.go`、`backend/internal/service/upstream_models.go`。

### 212 `87266a7b181024f1e48dcdb3ff5c53018496c30a`

原始标题：fix(grok): 原子保存模型目录观测快照

- **结论：需适配**。
- **实际代码与HEAD证据**：`account_repo_grok_models.go:CompareAndSwapGrokObservedModels`只原子改extra.grok_observed_models，WHERE匹配expected JSON且账号未删/platform=grok；`PersistGrokCatalogObservation`冲突重读合并最多3次。HEAD后台直接UpdateExtra单键，无此CAS能力和固定观察重试。
- **收益/冲突**：实际并发手动/后台同步避免旧结果覆盖新结果；外层事务必须使用transaction client，未提交不刷新缓存，不能回退无锁写。保留现有事务机制，避免借机新增泛型repository层。
- **验证/依赖**：依赖211；带入Postgres JSON对象键序/缺失null等价、并发成功/失败、取消及外层事务回滚，用213补充断言；实际新增测试例：`TestGrokCatalogCASPreservesSingleKeyAndRejectsStaleExpected`、`TestGrokCatalogCASTreatsNullMissingAndKeyOrderAsSameValue`。
- **定位**：`backend/internal/repository/account_repo_grok_models.go`、`backend/internal/service/grok_model_catalog_store.go`。

### 213 `b1cb2eb1f30d5f098fe509964b0d793c3ceb40ea`

原始标题：fix(grok): 补齐取消与调用方事务的回归断言

- **结论：需适配**。
- **实际代码与HEAD证据**：仅 `account_repo_grok_models_test.go`加强调用方事务SQL验证及 `grok_model_catalog_store_test.go`取消断言。HEAD没有212引入的CAS接口/文件。
- **收益/冲突**：不能单独修本地生产问题；作为212验收必需，证明没有绕过调用方事务，也没有在取消后写库。
- **验证/依赖**：依赖212；核查SQL期望实际落在tx client、取消时repository未被调用；本次未运行；实际新增测试例：`TestGrokCatalogCASAppliesOnCallerTransactionNotBaseClient`。
- **定位**：`backend/internal/repository/account_repo_grok_models_test.go`、`backend/internal/service/grok_model_catalog_store_test.go`。

### 214 `41a6ff8ac1209da6057f46e1ae49b92852d3baa2`

原始标题：fix(grok): 对齐手动同步与后台目录观察

- **结论：需适配**。
- **实际代码与HEAD证据**：`syncGrokObservedModels`把构建请求/抓取/保存观察分开；手动 `fetchUpstreamModelListWithObservation`返回同源GrokObservation，`discovery_source`标上游或configured_fallback。HEAD后台TTL只看FetchedAt，HTTP>=400静默返回，成功直接UpdateExtra；手动同步没有此观察持久化。
- **收益/冲突**：失败保留历史成功、改变源后不借用TTL、fallback不能伪装上游观测；实际目录成功与metadata补全失败分开保存。不要把所有provider套入Grok特有状态。
- **验证/依赖**：依赖211/212；需带入大体积/无效/空目录、非2xx、保存失败、manual忽略TTL、preview不持久化、取消、URI查询源区分、时间固定跨CAS重试；实际新增测试例：`TestGrokCatalogSyncSavesIDOnlyModelsWhenMetadataRegistryFails`、`TestGrokCatalogSyncConfiguredFallbackDoesNotRefreshSuccess`。
- **定位**：`backend/internal/service/grok_observed_models.go`、`backend/internal/service/upstream_models.go`。

### 215 `635b8353508eb0b8994afcc5b1ef73c5b270d1b1`

原始标题：fix(grok): 让旧后台观察测试走 CAS 持久化

- **结论：需适配**。
- **实际代码与HEAD证据**：`grok_quota_service_test.go`将旧后台观察fake从UpdateExtra改为GetByID+CompareAndSwapGrokObservedModels，并按CAS快照断言。HEAD后台仍旧UpdateExtra且无CAS。
- **收益/冲突**：是214实现改变后的测试适配，单独移植会错误要求不存在的能力。
- **验证/依赖**：依赖211–214；和旧quota刷新测试一起跑，避免目录保存改动破坏原配额探测。
- **定位**：`backend/internal/service/grok_quota_service_test.go`。

### 216 `d4136cea5e6c3c8fb40487f3f4692ab72919e8b1`

原始标题：fix(grok): 提供账号模型目录差异诊断

- **结论：需适配**。
- **实际代码与HEAD证据**：`grok_model_diagnostics.go:EvaluateGrokModelDiagnostics/GetGrokModelDiagnostics`纯读取比较已保存映射与fresh同源目录，handler详情/同步返回diagnostics；HEAD没有该文件/DTO字段，列表/详情不提供诊断。
- **收益/冲突**：有管理排障收益，但属于目录诊断新功能；仅fresh才报告not_listed，过期/换源是历史，音视频out_of_scope，不应直接参与路由或改映射。详情构造不得取token/发HTTP。
- **验证/依赖**：依赖211–214并与217纠偏一起取；验证详情只读无请求、列表DTO省略、source_changed/stale/unknown、通配符/文本别名及敏感URL脱敏；实际新增测试例：`TestAccountHandlerGrokModelDiagnosticsGetByID`、`TestAccountHandlerGrokModelDiagnosticsOmitsNonGrokAndMissingSnapshot`。
- **定位**：`backend/internal/handler/admin/account_handler.go`、`backend/internal/handler/dto/types.go`、`backend/internal/service/grok_model_diagnostics.go`、`backend/internal/service/upstream_models.go`。

### 217 `bb11e83c44abe9786c20cffca4445190bf2b63fb`

原始标题：fix(grok): 裸别名无法核验且自定义 ID 保留大小写

- **结论：需适配**。
- **实际代码与HEAD证据**：`grokDiagnosticObservedID/grokDiagnosticBareAlias`不把裸grok/grok-latest硬推默认代际，自定义ID保留大小写，build后缀不借运行时计费等价。HEAD诊断模块缺失，但`xai`运行时别名机制已存在，不能复用为目录事实。
- **收益/冲突**：修正216把运行时路由选择误当目录核验事实；任何引入216方案都应保留此界限。
- **验证/依赖**：依赖216；带入裸别名unverifiable、已知文本别名、未知大小写自定义ID和build不等价断言；实际新增测试例：`TestGrokModelDiagnosticsBareAliasDoesNotInventDefaultGeneration`、`TestGrokModelDiagnosticsCustomIDKeepsCase`。
- **定位**：`backend/internal/service/grok_model_diagnostics.go`。

### 218 `60931bce3114eb1d240e5bd1eb54546181216056`

原始标题：fix(grok): 展示模型目录诊断并保留编辑草稿

- **结论：需适配**。
- **实际代码与HEAD证据**：新增`GrokModelDiagnosticsPanel.vue`和EditAccountModal的诊断读取/sync代际状态；API/types加可选字段，mapping模式同步只刷新诊断而不把目录追加进草稿。HEAD该panel不存在，EditAccountModal沿用普通白名单选择器。
- **收益/冲突**：目录诊断UI新功能。必须保护编辑中的base_url/mapping，不把后台保存配置同步误写尚未提交的表单；账号切换/关闭/卸载的晚响应不得覆盖新账号。
- **验证/依赖**：依赖216/217；从219–228取最终状态而非逐次重放，测试draft保留、旧响应隔离、optional字段兼容、错误/刷新失败显示；实际新增测试例：`keeps optional diagnostics from the existing account detail response`、`preserves discovery source and diagnostics on the existing sync URL`。
- **定位**：`frontend/src/api/admin/accounts.ts`、`frontend/src/components/account/EditAccountModal.vue`、`frontend/src/components/account/GrokModelDiagnosticsPanel.vue`、`frontend/src/components/account/ModelWhitelistSelector.vue`、`frontend/src/i18n/locales/en/admin/accounts.ts` 等（实际 11 个变更文件）。

### 219 `ae5705da1d0b13c351d52329067eb2763e986f8f`

原始标题：fix(grok): 同账号刷新不再卡住目录同步

- **结论：需适配**。
- **实际代码与HEAD证据**：`EditAccountModal.vue:syncGrokDiagnostics/resetGrokDiagnostics`用grokDiagnosticsSyncVersion单独管理同步生命周期，脱离普通refresh使用的diagnosticsGeneration。HEAD尚无这套诊断同步状态。
- **收益/冲突**：修复218同账号刷新碰撞后finally不清syncing导致按钮永远禁用；这是新增UI的依赖补丁。
- **验证/依赖**：随218最终状态纳入；两个重叠请求分别成功/失败都能解锁并保留用户草稿，不能只测promise成功；实际新增测试例：`re-enables saved-config sync when a same-account selector refresh overlaps the request`、`re-enables saved-config sync when a same-account selector refresh overlaps a failed request`。
- **定位**：`frontend/src/components/account/EditAccountModal.vue`。

### 220 `2db7ce3d25a9b2dc8285bbc1fbf51185c1211f43`

原始标题：fix(grok): 隔离过期目录同步并在切模式后刷新诊断

- **结论：建议引入**。
- **实际代码与HEAD证据**：`ModelWhitelistSelector.vue:syncUpstreamModels`加accountId/卸载代际隔离，EditAccountModal中v-show保持selector并在同步完成刷新诊断。HEAD sync读响应后直接emit更新、toast且finally清loading，没有代际token或卸载处理。
- **收益/冲突**：其中selector晚响应污染当前账号/草稿是已有组件真实缺口，可独立最小修；诊断刷新/v-show部分依赖218。不能为修selector强制引入全套诊断UI。
- **验证/依赖**：带入A同步慢、切到B并开始新同步，A晚到不能改B的选择/toast/loading；卸载/关闭和模式切换测试。兼容原有preview同步入口；实际新增测试例：`does not append an older account sync result to the current whitelist or toast its success`、`refreshes persisted diagnostics after a whitelist sync when switching to mapping without changing the mapping draft`。
- **定位**：`frontend/src/components/account/EditAccountModal.vue`、`frontend/src/components/account/ModelWhitelistSelector.vue`。

### 221 `02db748cd5a8767378c5f174eedf83367655e3bb`

原始标题：fix(grok): 保留受管目录快照并阻止非法时间与敏感失败日志

- **结论：建议引入**。
- **实际代码与HEAD证据**：`account_repo.go:lockAndMergeAccountProbeExtra`在行锁内保留最新grok_observed_models/upstream_model_metadata；`admin_account.go:UpdateAccount`不接受表单覆盖；后台观察日志改为kind/status而不是展开err URL；panel强化日期合法性。HEAD后台日志仍slog error err，普通编辑受管字段未含这两键。
- **收益/冲突**：即使暂缓诊断UI，现有后台目录和metadata也会被旧表单/并发更新覆盖，URL错误日志有敏感查询串风险。应先独立最小移植数据库受管字段保留与安全日志；panel日期改动依赖218。
- **验证/依赖**：带入旧表单/缺键/注入/并发CAS交错的受管字段保留、transport URL secret不出日志；整组落地再验闰日/非法日期/未来时间；实际新增测试例：`TestGrokManagedExtraSurvivesOrdinaryUpdate`、`TestGrokManagedExtraUpdateOverlapsCatalogCAS`。
- **定位**：`backend/internal/repository/account_repo.go`、`backend/internal/service/admin_account.go`、`backend/internal/service/grok_observed_models.go`、`frontend/src/components/account/GrokModelDiagnosticsPanel.vue`。

### 222 `0d374b25494d7b49915b51af15f9a7eff99a87b7`

原始标题：fix(grok): 精简目录诊断并按需展开详情

- **结论：需适配**。
- **实际代码与HEAD证据**：`GrokModelDiagnosticsPanel.vue`改摘要+可展开详情/差异，owned状态映射、相对时间和MappingRow组件，英语/中文文案同步。HEADpanel缺失。
- **收益/冲突**：纯新增诊断界面交互改进，不影响网关路由/计费；没有216/218时无需单独移植，不因视觉完善扩大当前任务。
- **验证/依赖**：依赖218/221并合并223/224最终语义；组件测试fresh/stale/source_changed、失败保留历史、fallback、无目录、键盘焦点/展开可访问性；实际新增测试例：`shows source, last success, the latest attempt, and mapping differences for a fresh catalog`、`renders the same sections in English`。
- **定位**：`frontend/src/components/account/GrokModelDiagnosticsPanel.vue`、`frontend/src/i18n/locales/en/admin/accounts.ts`、`frontend/src/i18n/locales/zh/admin/accounts.ts`。

### 223 `c68d312372999f696ddca35bb6f394061878602d`

原始标题：fix(grok): 未知目录详情改为历史标题

- **结论：需适配**。
- **实际代码与HEAD证据**：`GrokModelDiagnosticsPanel.vue`两个标题条件从仅stale/source_changed改为`effectiveState !== fresh`；unknown目录也标历史来源/历史模型。HEAD组件不存在。
- **收益/冲突**：避免unknown数据被显示成当前可信上游事实，随222最后状态即可。
- **验证/依赖**：依赖222；unknown/非法或未来时间时标题必须历史，fresh仍显示当前目录。
- **定位**：`frontend/src/components/account/GrokModelDiagnosticsPanel.vue`。

### 224 `32c00d6e72135eb367d80d014ff1a929deffae4e`

原始标题：fix(grok): 诊断文案不再读取继承属性

- **结论：需适配**。
- **实际代码与HEAD证据**：panel新增`ownedLabelKey`用hasOwnProperty再查ATTEMPT_LABEL_KEYS/ERROR_LABEL_KEYS；HEAD没有该panel或两张映射表。
- **收益/冲突**：避免constructor/toString等继承属性流入翻译函数；是新UI边界数据处理，按最终版实现。
- **验证/依赖**：依赖222；测试未知status/error_code、原型属性和configured fallback标题，不把自定义状态静默当成功；实际新增测试例：`shows a fresh snapshot gap as the previous catalog when the latest attempt fell back`。
- **定位**：`frontend/src/components/account/GrokModelDiagnosticsPanel.vue`。

### 225 `21b5b49d0a5a673c051d16ad807d6b57be3694d5`

原始标题：fix(grok): 统一目录同步入口并保留模式隔离

- **结论：需适配**。
- **实际代码与HEAD证据**：`EditAccountModal.vue:requestGrokCatalogSync`统一诊断/whitelist按钮，selector defineExpose(syncUpstreamModels,isSyncingUpstream)，:key按account.id隔离，showSyncButton支持隐藏重复入口。HEAD组件没有公开方法及诊断panel。
- **收益/冲突**：减少重复同步入口，但必须在白名单模式仍按原语义追加，在mapping模式不碰草稿；这是218UI方案的终态组成。
- **验证/依赖**：依赖218–224及220隔离；验每种mode只有一个按钮、预览/非Grok保留旧按钮、同账号保焦点、账号切换收折、过期失败不toast；实际新增测试例：`keeps the diagnostics panel and one shared sync button visible in both modes`、`warns once for partial metadata when a mapping sync succeeds and refreshes once`。
- **定位**：`frontend/src/components/account/EditAccountModal.vue`、`frontend/src/components/account/ModelWhitelistSelector.vue`。

### 226 `7f18239be2c09bd7c08663bd4d55ee1f73d42571`

原始标题：fix(grok): 对齐模型编辑布局与响应式展示

- **结论：需适配**。
- **实际代码与HEAD证据**：`EditAccountModal.vue`Grok映射编辑从通用双色按钮改独立语义化布局、响应式行与索引key、aria属性；HEAD仍通用映射布局，没有grok-model-mapping-row专用结构。
- **收益/冲突**：仅在接受目录诊断新功能后采用最终UI；duplicate from/to行必须能独立编辑/删，避免用内容key复用DOM。不要连带重排其它平台表单。
- **验证/依赖**：依赖225；带入duplicate映射行编辑/删除断言，键盘操作与窄屏/深浅主题需视觉验收，本次未做；实际新增测试例：`edits and deletes duplicate Grok mapping rows without reusing their values as keys`。
- **定位**：`frontend/src/components/account/EditAccountModal.vue`、`frontend/src/i18n/locales/en/admin/accounts.ts`、`frontend/src/i18n/locales/zh/admin/accounts.ts`。

### 227 `9534f1c089d67fbfac8784489579791bd50df0e4`

原始标题：fix(grok): 提高删除图标对比度并补齐同步按钮 Label in Name

- **结论：需适配**。
- **实际代码与HEAD证据**：`EditAccountModal.vue`Grok删图标text-gray-400改gray-500并dark:gray-400；panel sync的aria-label包含可见syncCatalog文字。HEAD没有诊断panel/Grok独立删按钮样式。
- **收益/冲突**：是226/225界面的对比度与Label in Name修正；不单独改其它平台红色删除按钮。
- **验证/依赖**：依赖225/226；可取最终classes和aria-label测试，仍需实际深浅主题对比度/读屏检查。
- **定位**：`frontend/src/components/account/EditAccountModal.vue`、`frontend/src/components/account/GrokModelDiagnosticsPanel.vue`。

### 228 `dfc13ce7f9ff9300c524d21d871f672c91d235c2`

原始标题：fix(grok): 修复模型目录审计相关 lint 错误

- **结论：需适配**。
- **实际代码与HEAD证据**：`grokCatalogFingerprint`简化拼串，`ModelWhitelistSelector.vue`finally改if(isCurrentSync())块，测试map断言/清理lint；HEAD没有fingerprint函数/代际finally分支。
- **收益/冲突**：上游构建清理补丁，选目录功能最终状态时需纳入；不能用此提交为新增hash合理性背书。
- **验证/依赖**：依赖211/220/225；对实际移植文件运行项目现有lint，保留错误/取消时loading与完成事件语义。
- **定位**：`backend/internal/service/grok_model_catalog.go`、`frontend/src/components/account/ModelWhitelistSelector.vue`。

### 229 `de908ed6d29339fec04dcd9b39dab95b47a49575`

原始标题：fix(ratelimit): 国产 Coding Plan 的频率型 429 改用秒级冷却

- **结论：建议引入**。
- **实际代码与HEAD证据**：`ratelimit_cn_providers.go:applyCNProviderReactive429`在窗口冷却前辨认纯频率文案，并要求已探测各窗口低于90%；HEAD余额判定后Coding Plan 429直接cooldownCNProviderToQuotaSnapshotReset。
- **收益/冲突**：避免一次频控误停几小时。快照缺失/接近耗尽/包含额度字样仍长冷却；上游承认快照最多滞后探测周期，可能短时间重复429，不能声称精确分类。
- **验证/依赖**：带入频率、额度、余额不足、快照缺失/90%边界和月窗口测试；保留现有可配置短冷却，核对真实供应商文案；实际新增测试例：`TestHandleUpstreamError_ZhipuFrequency429UsesShortCooldown`、`TestHandleUpstreamError_ZhipuFrequency429WithExhaustedSnapshotKeepsWindowCooldown`。
- **定位**：`backend/internal/service/ratelimit_cn_providers.go`。

### 230 `7bce2cfb45d8cf6e8ea7f9cf2710c9febb8a2f51`

原始标题：fix(claude): match JavaScript UTF-16 billing fingerprint semantics

- **结论：已覆盖**。
- **实际代码与HEAD证据**：`gateway_billing_block.go:computeClaudeCodeFingerprintFromText/javascriptUTF16CodeUnitAt`已经按JavaScript码元索引4/7/20采样、短文本补0、采样后合并surrogate并用U+FFFD处理孤立半对；`gateway_billing_header_test.go:TestClaudeCodeFingerprintMatchesJavaScriptUTF16Indexing`已有中文预期d80。上游从UTF8 byte切换UTF16解决的缺陷本地已手工解决。
- **收益/冲突**：保留本地提取第一条非meta user文本及wire UA同步能力，不用上游较短实现覆盖更完整的本地语义。
- **验证/依赖**：现有中文断言已读，未执行；上游emoji/混合采样半对用例可补作回归，需保持本地isMeta筛选预期；实际新增测试例：`TestClaudeCodeFingerprint_JavaScriptUTF16`、`TestSyncBillingHeaderVersion_UnicodeSuffix`。
- **定位**：`backend/internal/service/gateway_billing_block.go`。

### 231 `917dbdc705d69a65e1fccdd0bba2dc0f7afdbc67`

原始标题：fix(gemini): count fileData image outputs for billing

- **结论：建议引入**。
- **实际代码与HEAD证据**：`gemini_image_output_accounting.go:countGeminiImageOutputs/geminiPartIsImage`从inlineData/inline_data扩到fileData/file_data，校验image MIME和非空fileUri/file_uri，同part计一次。HEAD `countGeminiInlineImageOutputs`只识别内联base64。
- **收益/冲突**：真实file引用输出会漏图或仅靠模型启发式计1，多图/自定义别名会漏费。保留按payload最大值而非累加，防累计SSE重复收费。
- **验证/依赖**：带入fileData多图、camel/snake混用、空URI/非image、不重复part和累计chunks；跟踪原生非流及流观测调用；实际新增测试例：`TestCountGeminiImageOutputs`、`TestHandleNativeNonStreamingResponse_FileDataImageBilling`。
- **定位**：`backend/internal/service/gemini_image_output_accounting.go`。

### 232 `5db2d6956ba60fb5c9401823561a166d38295095`

原始标题：fix(billing): retain DeepSeek V4 Pro pricing after September 14

- **结论：待核实**。
- **实际代码与HEAD证据**：`billing_service.go:deepseekProBilledAsFlash`和2026-09-14切换被删除，Pro保持Pro价；HEAD仍在该日期之后强制Flash三档价。`applyModelSpecificPricingPolicyEx`为默认目录强制，分组/渠道自定价另行豁免。
- **收益/冲突**：本地明确受日期价卡分支影响，潜在持续少计。上游注释声称官方后续保留Pro，当前静态git证据不能核实商业价格事实；不应仅凭新注释立即改实收金额。
- **验证/依赖**：优先取得已授权的实际价卡/账单证据；核实后带入固定日期前后Pro及版本名、峰谷倍率、自定价不被覆盖回归。本次不联网/不访问真实账单；实际新增测试例：`TestCalculateCostUnified_DeepseekProRetainsRatesAfterSeptember14`。
- **定位**：`backend/internal/service/billing_service.go`。

### 233 `d0afece15f9f16da32193816c2cc50dbb04938e5`

原始标题：fix(model-plaza): prefer group model pricing over image fallback tiers

- **结论：建议引入**。
- **实际代码与HEAD证据**：`model_plaza_service.go:fillDisplayPricing`匹配分组模型级image/per_request价后立即返回。HEAD只赋m.Pricing仍继续resolver/fallback，最后 `plazaImageDisplayPricing`可能用分组image_price_1k/2k/4k覆盖。
- **收益/冲突**：修复广场显示和实际按model_pricing扣费不一致；不触及收费计算，仅维护既有优先级。
- **验证/依赖**：带入分组模型图片分档/固定价与分组兜底冲突的展示断言，确认token模型仍能显示上下文阶梯；实际新增测试例：`TestListPlazaGroups_GroupModelImagePricingWinsOverGroupImageTiers`、`TestListPlazaGroups_GroupModelFlatImagePriceWinsOverGroupImageTiers`。
- **定位**：`backend/internal/service/model_plaza_service.go`。

### 234 `3e18e78ab1d06d3cce1fc019d8969acceafaaf5e`

原始标题：fix(usage): DeepSeek prompt_cache_hit_tokens 计为缓存读取

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_gateway_response_handling.go:openAICacheReadTokensFromUsage`候选字段增加prompt_cache_hit_tokens。HEAD列表没有该DeepSeek字段；共享usage parser由JSON/SSE/兼容Chat路径使用。
- **收益/冲突**：DeepSeek prompt_tokens=hit+miss，漏认cache会把缓存命中作为普通输入计费。单点共享修复比各入口重复转换更合适。
- **验证/依赖**：带入DeepSeek JSON与SSE cache hit计cache读取、输入不重复计数、已有标准cached_tokens优先级；核对所有usage调用方；实际新增测试例：`TestOpenAIUsageDeepSeekPromptCacheHitTokensCountAsCacheRead`。
- **定位**：`backend/internal/service/openai_gateway_response_handling.go`。

### 235 `ba5b21879430fdbe3ab87dc791b09bcdece5e4a7`

原始标题：fix(gateway): 修复 OpenCode 转发 DeepSeek 的角色兼容

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_chat_roles.go:requiresSystemChatRole`新增upstreamModel参数，OpenCode官方host/IsOpenCodeGo且实际映射型号DeepSeek时developer→system。HEAD只看DeepSeek/Kimi/Zhipu平台和严格host，OpenCode复用入口未区分供应商型号。
- **收益/冲突**：修复OpenCode转发DeepSeek角色拒绝；不能对OpenCode全部型号改system，必须使用出站model和选中的目标host。
- **验证/依赖**：带入自定义请求别名映射DeepSeek、非DeepSeek保持developer、官方host/账号平台两路、原请求不变以保障failover；实际新增测试例：`TestNormalizeStrictChatDeveloperRoles_OpenCodeModelRouting`。
- **定位**：`backend/internal/service/openai_chat_roles.go`。

### 236 `f9652804894eb150942af2b108bbce670594460c`

原始标题：fix(pagination): keep page offsets from overflowing int

- **结论：建议引入**。
- **实际代码与HEAD证据**：`pagination.PaginationParams.Offset`在乘法前限制Page，`response.ParsePagination`同样限制；ops token stats filter及prompt audit严格handler拒绝越界。HEAD Offset仍直接(Page-1)*Limit，ParsePagination不限制乘法。
- **收益/冲突**：真实HTTP查询page可到int上限，溢出负offset/后续slice panic。作为产品入口数字范围检查符合用户boundary规则；普通内部路径用安全算术，不另建门禁。
- **验证/依赖**：须和237一起：初版parseInt自身仍可能先溢出；带入math.MaxInt/MinInt、offset+limit、API返回metadata及严格handler测试；实际新增测试例：`TestPaginationParamsOffsetDoesNotOverflow`。
- **定位**：`backend/internal/handler/admin/ops_dashboard_handler.go`、`backend/internal/pkg/pagination/pagination.go`、`backend/internal/pkg/response/response.go`、`backend/internal/securityaudit/prompt_handler.go`。

### 237 `9aa16c8c66326afdd2e1b070e3403ac36e8a7050`

原始标题：fix(pagination): validate numeric ranges and reuse safe offsets

- **结论：建议引入**。
- **实际代码与HEAD证据**：`response.go:parseInt`从手工累乘换strconv.ParseUint(...,IntSize-1)，`api_key_service.go:paginateAPIKeys`复用params.Offset。HEAD手工累乘不检查溢出，APIKey内存分页再次直接乘法。
- **收益/冲突**：补236仅限页码但解析早已溢出的漏洞，统一安全偏移防负切片；不引入新分页框架。
- **验证/依赖**：联合236，跑解析/响应元数据、仓库paginateSlice、合并余额历史和APIKey slice边界断言；覆盖超int长数字、无符号十进制和正常默认回退；实际新增测试例：`TestParseOpsOpenAITokenStatsFilter_PaginationBoundaries`、`TestParsePaginationBoundariesAndResponse`。
- **定位**：`backend/internal/pkg/response/response.go`、`backend/internal/service/api_key_service.go`。

### 238 `cde6ddfd8f41e1cf173ed83ac42e79f1e8af34c8`

原始标题：fix(images): reject oversized multipart uploads instead of truncating them

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_images.go:parseOpenAIImagesMultipartRequest`读limit+1并超限报错，单part由20MiB改50MiB；`grok_media.go:parseGrokMediaMultipartRequest`在总body限额后完整读part。HEAD两处仍LimitReader(part,20MiB)直接ReadAll，不知道数据被截断。
- **收益/冲突**：真实上传大图当前被静默截坏再发上游，必须修。50MiB是另一个容量策略变化，可只先做旧限额+1明确拒绝；Grok全读前必须确认本地handler总body限额，避免取消内存边界。日志仅记MIME/长度，勿记录图像。
- **验证/依赖**：带入20MiB以上完整保留、50MiB以上拒绝、总body限额、多part及OAuth上传；本次未构造大图/跑测试；实际新增测试例：`TestParseOpenAIImagesRequest_MultipartKeepsUploadAbove20MB`、`TestParseOpenAIImagesRequest_MultipartRejectsUploadAbovePartLimit`。
- **定位**：`backend/internal/service/grok_media.go`、`backend/internal/service/openai_images.go`、`backend/internal/service/openai_images_responses.go`。

### 239 `14a5fe3050616e35b5f123614f57e0e6adcaf069`

原始标题：fix(antigravity): reject malformed non-streaming tool calls

- **结论：建议引入**。
- **实际代码与HEAD证据**：`antigravity.TransformGeminiToClaude`检查首candidate FinishReason MALFORMED_FUNCTION_CALL，collector遇到没有parts的错误尾帧也返回ErrMalformedFunctionCall，compat mapper转502。HEADtransformer直接交processor，collector只选有效parts，错误尾帧可能变正常完成。
- **收益/冲突**：避免无效工具调用被报告成功；沿用上游失败信号，不为内部任意状态加假设guard。
- **验证/依赖**：带入非流直包/v1包、SSE最后无parts错误、chat/responses兼容502及正常工具结果，核对直接原生流既有处理；实际新增测试例：`TestMalformedFunctionCallTransform`、`TestAntigravityMalformedFunctionCallNonStreaming`。
- **定位**：`backend/internal/pkg/antigravity/response_transformer.go`、`backend/internal/service/antigravity_gateway_compat.go`、`backend/internal/service/antigravity_gateway_streaming.go`。

### 240 `88d9c43b3e9a30d9e7f74a369114eff0f5674379`

原始标题：fix(admin): 管理员保护规则返回 400 而不是 500

- **结论：建议引入**。
- **实际代码与HEAD证据**：`admin_user.go:ensureNotLastAdmin/UpdateUser/DeleteUser`把最后管理员降权、禁用管理员、删除管理员的普通errors.New变为infraerrors.BadRequest稳定code。HEAD仍普通error，被handler ErrorFrom分类为500。
- **收益/冲突**：校正产品API错误语义，保护规则本身保留，不扩张权限或改管理员计数。
- **验证/依赖**：带入三项400及合法先降级后操作、repo真实故障仍500；无需新增参数校验；实际新增测试例：`TestAdminService_UpdateUser_AdminGuardsAreClientErrors`。
- **定位**：`backend/internal/service/admin_user.go`。

### 241 `0a2a29b32e6724714c69822296ccd05a6b86bfdd`

原始标题：chore(deps): bump golang.org/x/net to v0.62.0 (GO-2026-6860)

- **结论：建议引入**。
- **实际代码与HEAD证据**：`backend/go.mod`将x/net0.60→0.62、x/crypto0.57→0.58并联动sync/term/sys/text/tools。HEAD x/net0.58、x/crypto等更旧。真实网络入口通过Go HTTP/upstream client消费这些库。
- **收益/冲突**：上游标记GO-2026-6860安全修复，应优先最小依赖升级；不复制上游整个go.mod/go.sum，保留本地module和直接依赖。
- **验证/依赖**：未联网核实漏洞详细影响、未跑govulncheck或下载；需在本地工具链179/180/194一致后解析依赖并执行单测/漏洞扫描，lock/sum只审变更版本未逐行校验摘要。
- **定位**：`backend/go.mod`。

### 242 `9eba617367469e3e37fa4aa131275499ea2f8c14`

原始标题：fix(channels): preserve web_search_emulation platform dimensions on update

- **结论：需适配**。
- **实际代码与HEAD证据**：`ChannelService.applyUpdateInput`新增`mergeWebSearchEmulationDimensions`，只合并web_search_emulation平台维度；缺键继承，显式{}或null清除。HEAD仍`channel.FeaturesConfig = input.FeaturesConfig`。本地ChannelsView已用supportsWebSearchEmulation呈现多平台，且全局状态未知仍显示设置，比上游原界面更新。
- **收益/冲突**：仅补服务端维度保留及disabled平台显式false；当前formToAPI仍跳过未启用section。不能覆盖本地全局配置不可用时保留显示/禁用行为，也不要拷贝上游重复平台set或做通用deep merge。
- **验证/依赖**：带入缺平台保留、显式false覆盖、空/null清除、复制不修改原map及其它features仍整块语义；沿用本地supportsWebSearchEmulation白名单与现有组件测试；实际新增测试例：`TestMergeWebSearchEmulationDimensions_MissingDimensionsInherited`、`TestMergeWebSearchEmulationDimensions_RequestValueWins`。
- **定位**：`backend/internal/service/channel_service.go`、`frontend/src/i18n/locales/en/admin/channels.ts`、`frontend/src/i18n/locales/zh/admin/channels.ts`、`frontend/src/views/admin/ChannelsView.vue`。

### 243 `1352e899ecd418ecf0b81a5151a9c992c953d945`

原始标题：test(channels): use checked type assertions in web search merge tests

- **结论：需适配**。
- **实际代码与HEAD证据**：`channel_websearch_merge_test.go`将单返回类型断言改成value,ok并require.True，生产函数未改。HEAD没有242的新merge测试/函数。
- **收益/冲突**：随242移植最终测试形式以符合errcheck；这是test角色检查，不是新增生产类型防御。
- **验证/依赖**：依赖242；运行项目lint和merge用例，不因类型断言形式改变误报业务修复。
- **定位**：`backend/internal/service/channel_websearch_merge_test.go`。

### 244 `de44e3a0bfd63db10fc733a71d6577f5070a6b8e`

原始标题：fix(gemini): 分组账号都声明模型映射时，/v1beta/models 列表与详情按声明的模型应答

- **结论：需适配**。
- **实际代码与HEAD证据**：`GeminiV1BetaListModels/GetModel`在每个可调度Gemini账号均有映射时使用DeclaredModelIDs并集，再合并Antigravity目录和分组白名单；`DeclaredModelsCover`匹配wildcard/归一别名。HEADhandler仍选单账号代理models；service模型端点还在大兼容service中，独立gemini_models_endpoints.go文件缺失。
- **收益/冲突**：修复模型列表宣称与实际请求映射不一致，但不能直接新建同名实现而留下旧函数重定义；按当前布局最小接入，不必为移植先拆大文件。
- **验证/依赖**：上游依赖先前model endpoint重构；适配当前SelectAccountForModelsEndpoints调用，验证混合组/映射全配与一个未配、list/detail相同来源、allowlist及wildcard；实际新增测试例：`TestGeminiNativeModelsUsesDeclaredMappings`、`TestGeminiNativeModelsUnmappedAccountKeepsUpstream`。
- **定位**：`backend/internal/handler/gemini_v1beta_handler.go`、`backend/internal/service/gemini_models_endpoints.go`。

### 245 `31564b2e86ccb6f4de348a21eb9e47b366f326ba`

原始标题：fix(gemini): 刷新 Gemini 默认模型列表，移除已停用的 2.x 模型

- **结论：需适配**。
- **实际代码与HEAD证据**：`gemini.DefaultModels/geminicli.DefaultModels`移除2.x/旧preview，新增图像lite及3.x型号；DefaultTestModel改3.5-flash，`geminiDefaultTestModel`对GoogleOne映射默认测试模型；前端whitelist和图像模型识别同改。HEAD仍2.0-flash默认及旧列表。
- **收益/冲突**：默认测试打停用模型会失败，目录应更新；但是否停服/当前供应商白名单属于外部事实，不能从提交标题推断本地代理都移除了2.x，GoogleOne/Antigravity别名也有本地差异。
- **验证/依赖**：落地前核对实际支持目录/既有离线工件；按后端fallback、管理测试、前端候选和image识别全链测试，保留用户自定义映射可接受旧ID；实际新增测试例：`TestGeminiDefaultTestModel`、`TestIsImageGenerationModel_GeminiFlashLiteImage`。
- **定位**：`backend/internal/pkg/gemini/models.go`、`backend/internal/pkg/geminicli/models.go`、`backend/internal/service/account_test_service.go`、`backend/internal/service/antigravity_gateway_streaming.go`、`frontend/src/composables/useModelWhitelist.ts`。

### 246 `4197fa62021ca1fe7933fe3eb08ae119dc1fe91b`

原始标题：feat(gemini): 同账号重试次数改为可配置 gateway.max_same_account_retries_gemini

- **结论：需适配**。
- **实际代码与HEAD证据**：`config.Gateway.MaxSameAccountRetriesGemini`默认4并范围校验，`geminiMaxAttemptsPerAccount`用于三个兼容/native循环；签名降级stage不消耗普通重试预算。HEAD硬编码geminiMaxRetries=5。
- **收益/冲突**：新配置提供运维控制，默认总尝试次数维持5；签名降级独立预算是行为变化，必须防循环/过度延迟。沿用config来源，不新增额外面板开关。
- **验证/依赖**：配置环境映射及0/1/默认4/非法范围、429策略仅检查一次、签名stage+普通失败预算测试；保留本地context取消和sticky选择；实际新增测试例：`TestLoadGeminiSameAccountRetries`、`TestGeminiForward_SameAccountRetriesFollowConfig`。
- **定位**：`backend/internal/config/config.go`、`backend/internal/service/gemini_chat_completions_compat_service.go`、`backend/internal/service/gemini_messages_compat_service.go`。

### 247 `a207412375448f85e816b1b56139813d04d831d8`

原始标题：fix(gemini): 绑定显式缓存的请求在池模式上游网关账号上按配置同账号重试

- **结论：需适配**。
- **实际代码与HEAD证据**：`boundCacheUpstreamPassthroughError`对绑定缓存+池模式+状态白名单设置SameAccountRetryable/RequestScopedTransient并记retry事件；用尽仍透传，不冷却持有上游网关账号。HEAD没有gemini_cached_content.go及绑定缓存透传error字段整套路径。
- **收益/冲突**：依赖前面批次显式缓存绑定实现，当前单独移植不能发挥作用；不能为成功率切换账号，因为cache仅在指定账号。
- **验证/依赖**：在绑定缓存主功能引入后再取，验证429成功重试/预算耗尽原样透传、502不禁号、非池模式与不在状态白名单不重试，和246预算分清；实际新增测试例：`TestGeminiV1BetaModels_BoundCachePoolModeRetriesUpstream429`、`TestGeminiV1BetaModels_BoundCacheRetryExhaustedPassesThroughUpstream429`。
- **定位**：`backend/internal/service/gemini_cached_content.go`、`backend/internal/service/gemini_messages_compat_service.go`。

### 248 `b2b1bcf260b584dc4d268a8ae36bec31a2de2130`

原始标题：fix(models): 模型列表刷新进入 singleflight 后复查缓存，避免并发冷启动重复请求上游

- **结论：建议引入**。
- **实际代码与HEAD证据**：`openai_codex_models_service.go:refreshCachedOpenAIModels`在DoChan闭包入口复查cacheFresh后直接返回；HEAD闭包只取cached构造ETag然后总会fetch。
- **收益/冲突**：修复调用方读缓存到进入singleflight间另一轮已刷新造成重复上游请求，改动很小；保持304续期、后台超时和stale策略。
- **验证/依赖**：原提交没有新测试；增加一个受控调度回归：首次外层miss后由别的刷新填fresh，进入闭包应不调用fetch。保留ETag/模型目录缓存既有测试。
- **定位**：`backend/internal/service/openai_codex_models_service.go`。

### 249 `a9240d5016c226b341242a8c54a01a8605cbbc72`

原始标题：chore: sync VERSION to 0.2.19 [skip ci]

- **结论：不引入**。
- **实际代码与HEAD证据**：仅backend/cmd/server/VERSION从0.2.18升0.2.19，无业务diff。
- **收益/冲突**：不把上游发布号当本地功能集或兼容承诺。
- **验证/依赖**：按本地发布规则验版本，静态评审无需测试。
- **定位**：`backend/cmd/server/VERSION`。

### 250 `6b2fdfcac262ec826be24c9ca857f39e40ef7119`

原始标题：fix(openai): return JSON for non-stream OAuth passthrough /v1/responses

- **结论：建议引入**。
- **实际代码与HEAD证据**：`forwardOpenAIPassthrough`保留客户端原始reqStream，而不是OAuth normalizer强制stream=true后回读；两个SSE→JSON helper显式覆盖已复制的Content-Type。HEAD passthrough仍`reqStream = gjson.GetBytes(body,"stream").Bool()`，聚合后c.Data前未Set响应头。
- **收益/冲突**：非流OAuth /v1/responses当前可能变SSE或返回JSON正文但text/event-stream头；compact保持非流。不要影响0.164 Lite模式的原始形态和工具恢复。
- **验证/依赖**：联合251/253；客户端false/true、legacy/透传、compact keepalive与headers、usage完整保留的mock测试；实际新增测试例：`TestOpenAIGatewayService_OAuthPassthrough_StreamFalseReturnsJSON`、`TestOpenAIGatewayService_OAuthPassthrough_StreamTrueStillStreams`。
- **定位**：`backend/internal/service/openai_gateway_passthrough.go`、`backend/internal/service/openai_gateway_response_handling.go`。

### 251 `9b5407449c8d0b8f41abba4756da3bab47a67945`

原始标题：fix(openai): return JSON for incomplete/cancelled non-stream Responses

- **结论：建议引入**。
- **实际代码与HEAD证据**：`extractOpenAINonCompletedTerminalResponse`处理incomplete/cancelled/canceled，从terminal.response取JSON，两个聚合helper调用。HEAD只有extractCodexFinalResponse(completed/done)，没有此helper，非完成终止会回原始SSE。
- **收益/冲突**：保证非流客户端收到status/incomplete_details的单个JSON；failed/error仍现有失败转移/错误响应，不能误吞成成功。
- **验证/依赖**：依赖250，带入incomplete、两种cancel拼写、有/无response对象、usage/工具恢复、无terminal和failed仍报错；实际新增测试例：`TestOpenAIGatewayService_OAuthStreamFalseIncompleteReturnsJSON`、`TestOpenAIGatewayService_OAuthStreamFalseCancelledReturnsJSON`。
- **定位**：`backend/internal/service/openai_gateway_passthrough.go`、`backend/internal/service/openai_gateway_response_handling.go`。

### 252 `c01b2cb817663e79c3ce1db1d611ffe33f2d589a`

原始标题：ci: retrigger CI (previous run hit Docker Hub rate limit)

- **结论：不引入**。
- **实际代码与HEAD证据**：这是空提交：git show只有提交元数据，树无文件变更，用于重触Docker Hub限流后的CI。
- **收益/冲突**：没有可移植代码或本地CI修复，不制造空提交。
- **验证/依赖**：无需测试；不能把上游某次重跑成功当本地构建验证。
- **定位**：空树变更，无文件。

### 253 `4db8402b5081d2f3caa2b870b64aa7c18ff057ed`

原始标题：fix(openai): detect SSE body without Content-Type in OAuth passthrough non-stream

- **结论：建议引入**。
- **实际代码与HEAD证据**：`handleNonStreamingResponsePassthrough`增加OAuth且bodyHasSSEFraming判定。HEAD原生handleNonStreamingResponse早已支持同判断，但透传函数仍只看Content-Type，虽上面observer按SSE观察，并未转JSON。
- **收益/冲突**：补齐无Content-Type的OAuth SSE聚合，不能把所有APIKey JSON中出现data:字串都误判；复用当前行级bodyHasSSEFraming即可。
- **验证/依赖**：联合250/251带入缺头OAuth completed→JSON、普通APIKey JSON文本包含data:不变、usage/model/tool恢复和compact响应回归；实际新增测试例：`TestOpenAIGatewayService_OAuthStreamFalseWithoutUpstreamContentTypeReturnsJSON`。
- **定位**：`backend/internal/service/openai_gateway_passthrough.go`。
