# 上游提交静态评审：001–085

评审基线：本地 `c67360f44`；上游冻结 `a3298b46ebe239fb0720b068a1eb3383955bdac2`，共同祖先 `3040209f205472038c1ba745a1bedd2edd9053b1`。

范围：逐提交查看真实 `git show` 补丁并对照当前源码、关键直接调用入口和测试意图。无 patch-id 等价并不等于缺失；007、056、064通过现有实现确认已覆盖。静态评审没有执行测试、构建、依赖审计、数据库迁移或真实账号/远程服务验证。新增测试列为建议验证，不能解读为已运行。对大重构审关键手写实现和调用链；不逐行审 lock、截图、ent生成代码及builtin生成JSON。

实施总约束：保留本地 CLI0.164/`eb6ac2192` 对齐、WS `ctx_pool`、legacy sticky、OpenAI BPS及TypeSafe语义。所有“建议引入”仅表示代码意图值得移植，不表示可直接cherry-pick或已通过回归。

结论计数：不引入 9；需适配 21；建议引入 52；已覆盖 3。

优先候选：004/005（规则模型范围）、014/015（WS定价/图片用量）、016–023（协议兼容）、039/046（误改价格/设置）、060（加密reasoning恢复）、072（价格显示数量级）。077–085应独立于这些修复决策；尤其081/082/084不能丢本地BPS或覆盖迁移历史。

### 001 `b8dece9000c68815a5b867ca5a1e6f236e173905`

原始标题：chore: sync VERSION to 0.2.13 [skip ci]

**结论：不引入。** 只把服务端 VERSION 从0.2.12改成0.2.13；版本标签不代表补丁已移植。

- 关键改动文件/函数：`backend/cmd/server/VERSION`。
- 当前仓库证据：本地维护sub4api发布线，不能把上游版本号当本地发布版本。
- 收益/冲突：无运行逻辑收益。
- 验证/依赖：随本地正式发布管理版本，无需单独测试。

### 002 `5a25507dcbb33fc40e3ef0659af210f206229a31`

原始标题：feat(account): 临时不可调度规则新增作用范围 scope 选项

**结论：需适配。** TempUnschedulableRule增加scope；triggerTempUnschedulable在account/model间选择；创建/编辑表单、类型和中英文同步。

- 关键改动文件/函数：`backend/internal/service/account.go`（`normalizeTempUnschedScope`）；`backend/internal/service/ratelimit_service.go`（`tempUnschedRuleUsesModelScope`）；`backend/internal/service/temp_unsched.go`；`frontend/src/components/account/CreateAccountModal.vue`（`tempUnschedScopeOptions`）；`frontend/src/components/account/EditAccountModal.vue`（`tempUnschedScopeOptions`, `resolveTempUnschedScope`）；`frontend/src/i18n/locales/en/admin/accounts.ts`；`frontend/src/i18n/locales/zh/admin/accounts.ts`；`frontend/src/types/index.ts`。
- 当前仓库证据：当前ratelimit_service.go仍以modelKey非空且非401决定模型隔离，account.go和TempUnschedState没有Scope；创建/编辑保存只写错误码、关键词、时长和描述。
- 收益/冲突：让管理员明确决定隔离粒度；需保持旧规则空scope兼容，不能把现存规则统一改成model。
- 验证/依赖：与003–005一组；验证旧401、显式account/model、未知模型、池模式及前端保存往返。

### 003 `a46249aac5611fca71fb0d955ad19be7602aca71`

原始标题：test(account): 补充临时不可调度作用范围单元测试

**结论：建议引入。** 新增temp_unsched_scope_test.go，覆盖解析、隔离范围及持久化失败；EditAccountModal.spec.ts补旧scope迁移和卡片选择。

- 关键改动文件/函数：无生产代码改动。
- 当前仓库证据：当前生产结构无scope，对应新增Go测试文件不存在；现有EditAccountModal调用buildTempUnschedRules时也未携带scope。
- 收益/冲突：测试意图是防止隔离范围扩大和编辑后语义变化，不能独立于002运行。
- 验证/依赖：依赖002；移植后运行scope测试和编辑表单测试，并保留本地其他表单行为。
- 补丁测试证据（未运行）：`backend/internal/service/temp_unsched_scope_test.go`（func TestGetTempUnschedulableRules_ParsesScope(t *testing.T) {）；`frontend/src/components/account/__tests__/EditAccountModal.spec.ts`（describe('EditAccountModal temp-unschedulable rule scope', () => {）。

### 004 `fc82e831026a15eaf609cc1f7cd4d4a3690e5cad`

原始标题：fix(account): 显式模型级 401 规则在网关故障转移入口生效

**结论：需适配。** 新增tempUnschedRulesApply/tempUnschedHitUsesModelScope/unauthorizedRequiresCredentialHandling，贯通CheckErrorPolicy、HandleUpstreamError和OpenAI runtime block。

- 关键改动文件/函数：`backend/internal/service/openai_account_runtime_block_fastpath.go`；`backend/internal/service/ratelimit_service.go`（`tempUnschedRulesApply`, `tempUnschedHitUsesModelScope`, `unauthorizedRequiresCredentialHandling`）。
- 当前仓库证据：当前三个入口均显式排除401；openai_account_runtime_block_fastpath.go中modelTempMatched也以非401为前提。
- 收益/冲突：修复管理员模型级401规则无效，同时OAuth/shadow/token_revoked仍必须走凭据处置；不得使真实凭据失效仅冷却模型。
- 验证/依赖：依赖002/003；验证401首次/重复升级、OpenAI revoked/Unauthorized、OAuth/shadow、池模式、冷却后其他模型可调度。
- 补丁测试证据（未运行）：`backend/internal/service/temp_unsched_scope_test.go`（func TestOpenAIFailoverSideEffects_401FollowsTempUnschedScope(t *testing.T) {）。

### 005 `2023c290e1bbfa63e3f05bdddeeae1f264b8e637`

原始标题：fix(account): 计数、Bedrock 与重试耗尽入口把上游模型交给错误处理

**结论：需适配。** CountTokens、Anthropic passthrough、Bedrock及重试耗尽函数向错误处理传实际模型；Bedrock用account.GetMappedModel结果而非AWS模型ID。

- 关键改动文件/函数：`backend/internal/service/gateway_anthropic_passthrough.go`；`backend/internal/service/gateway_bedrock.go`；`backend/internal/service/gateway_count_tokens.go`（`forwardCountTokensAnthropicAPIKeyPassthrough`）；`backend/internal/service/gateway_forward.go`；`backend/internal/service/gateway_upstream_response.go`（`handleRetryExhaustedSideEffects`, `handleRetryExhaustedError`）；`backend/internal/service/openai_gateway_count_tokens.go`。
- 当前仓库证据：当前ForwardCountTokens和ForwardResponsesInputTokens/ForwardCountTokensAsAnthropic调用HandleUpstreamError未传模型；handleRetryExhaustedSideEffects仍是无模型签名，Bedrock错误四条分支同样丢模型。
- 收益/冲突：避免模型级规则降成整账号停调；本地count_tokens有额外兼容逻辑，按调用链补参数，勿覆盖整个文件。
- 验证/依赖：依赖002/004；静态检查全部直接调用方，再测count_tokens、Bedrock映射、重试耗尽和普通failover分支。
- 补丁测试证据（未运行）：`backend/internal/service/temp_unsched_scope_test.go`（func TestCountTokensEntries_ExplicitModelScope401OnlyCoolsModel(t *testing.T) {）。

### 006 `bbba01dae905f0036def71cb9fe4785fcab794c9`

原始标题：fix: enable API-key discovery for remote Codex catalogs

**结论：需适配。** UseKeyModal的四类Codex配置只在remote目录模式增加features.api_key_model_discovery=true。

- 关键改动文件/函数：`frontend/src/components/keys/UseKeyModal.vue`。
- 当前仓库证据：当前配置已输出model_catalog_url、env_key/experimental_bearer_token及requires_openai_auth=false，却未输出api_key_model_discovery。
- 收益/冲突：可恢复API Key远程目录发现；本地已按CLI0.164对齐，先用该版本实际支持的配置项验证，不能复制旧模板回退当前设置。
- 验证/依赖：对本地四种生成器做TOML/快照检查；使用匿名本地HTTP模拟目录发现，不接真实账号；依赖CLI0.164行为核实。

### 007 `d1aac6b98989b79d66c0f52bd18dc8c3142e7601`

原始标题：fix(payment): close EasyPay forged-callback sign reuse (fixes #7881)

**结论：已覆盖。** EasyPay.VerifyNotification拒绝订单创建专用参数；CanonicalizeReturnURL清除用户query，阻断签名串拼接复用。

- 关键改动文件/函数：`backend/internal/payment/provider/easypay.go`；`backend/internal/service/payment_resume_service.go`。
- 当前仓库证据：本地easypay.go的easyPayNotifyAllowedParams及循环拒绝未知键完全存在；payment_resume_service.go明确parsed.RawQuery=""且保留同源canonical路径校验。
- 收益/冲突：两个安全子项均已手工覆盖；不能因无patch-id等价重复移植或放宽白名单。
- 验证/依赖：现有easypay_notify_security_test.go和payment_resume_service_test.go可做回归；本轮仅静态阅读，未运行测试。
- 补丁测试证据（未运行）：`backend/internal/payment/provider/easypay_notify_security_test.go`（func TestEasyPayNotifyRejectsForgedSignReuseCallback(t *testing.T) {）；`backend/internal/service/payment_resume_service_test.go`（func TestCanonicalizeReturnURLStripsSmuggledTradeStatus(t *testing.T) {）。

### 008 `6e315616edeeedf55cf477ab8f27cb8d7c68ebfb`

原始标题：fix(setup): stop shipping guessable admin credentials on fresh installs (fixes #7850)

**结论：需适配。** setup统一登录邮箱规则和bcrypt72字节上限，首次创建时生成随机邮箱/密码；CLI、环境默认值、Compose、部署脚本和文档同时调整。

- 关键改动文件/函数：`README.md`；`README_CN.md`；`README_JA.md`；`backend/internal/setup/cli.go`；`backend/internal/setup/handler.go`（`maxPasswordBytes`）；`backend/internal/setup/setup.go`（`bootstrapAdminUser`, `prepareAdminCredentials`, `generateAdminEmail`）；`deploy/.env.example`；`deploy/APPLE_CONTAINER.md`；`deploy/README.md`；`deploy/docker-compose.dev.yml`；其余为配套调用点/类型/展示修改。
- 当前仓库证据：当前CLI默认admin@example.com、Compose默认admin@sub2api.local；validateEmail只mail.ParseAddress，validatePassword上限128；AutoSetupFromEnv仍固定邮箱。
- 收益/冲突：真实安装边界改进，避免创建无法登录的邮箱及bcrypt超长失败；随机邮箱并不替代强密码。README_CN/JA在本地不存在，不应为此恢复已移除文档。
- 验证/依赖：验证首次安装/已有管理员不变、显式凭据、随机生成失败、8–72字节与多字节输入、Web/CLI登录一致；适配本地部署文件，勿动用户新增ccvps文件。
- 补丁测试证据（未运行）：`backend/internal/integration/e2e_user_flow_test.go`；`backend/internal/setup/setup_test.go`（func TestPrepareAdminCredentialsGeneratesMissingValues(t *testing.T) {）。

### 009 `0f9d460dc65d2509107aff542459e18677e06291`

原始标题：fix(deps): resolve frontend audit failures (vue, source-map-js, xlsx exceptions)

**结论：需适配。** Vue要求提升到^3.5.43，新增source-map-js覆盖规则；更新lock和xlsx审计例外有效期/理由。

- 关键改动文件/函数：`.github/audit-exceptions.yml`；`frontend/package.json`。
- 生成/媒体文件：`frontend/pnpm-lock.yaml`；仅核对用途/存在状态，未逐行或逐像素评审。
- 当前仓库证据：当前package.json仍vue ^3.4.0且无source-map-js override；audit-exceptions中的xlsx例外仍2026-10-06到期。
- 收益/冲突：可处理该次审计失败，但例外延期不是漏洞修复；不能把上游当时安全版本直接认作当前最优。
- 验证/依赖：在实施时验证锁定版本、实际漏洞公告及xlsx仅写导出调用链，重新安装/审计并跑前端检查；本轮无联网审计，lock未逐行审。

### 010 `3f1a2ea0a760730e3bc528105c00b4ee4f23e469`

原始标题：chore: sync VERSION to 0.2.14 [skip ci]

**结论：不引入。** VERSION改为0.2.14。

- 关键改动文件/函数：`backend/cmd/server/VERSION`。
- 当前仓库证据：当前服务端VERSION属于本地发布线；无业务代码变化。
- 收益/冲突：只移版本会误示本地拥有该版全部改动。
- 验证/依赖：并入本地发布流程，无独立功能验证。

### 011 `c0e5a4670e24a5a63f54272abafbf0acd64f1016`

原始标题：fix(codex): never advertise null service_tiers in models manifest

**结论：需适配。** applyCodexToolCapabilities把service_tiers:null归一为[]；intersectUpstreamModelMetadata冲突时同样输出[]。

- 关键改动文件/函数：`backend/internal/service/openai_codex_model_metadata.go`。
- 当前仓库证据：当前apply允许null直接通过，交集冲突默认null；本地configuredCodexModelDescriptor虽然声明ServiceTiers切片，能力覆盖仍可能把它重新写成null。
- 收益/冲突：修复整个远程目录因必需数组为null解析失败；保留本地CLI0.164新增能力字段及comp_hash数值兼容。
- 验证/依赖：增加显式null、来源冲突、空列表、已有权威覆盖测试；检查最终manifest JSON而非只测helper；与012合并成最小修改。
- 补丁测试证据（未运行）：`backend/internal/service/openai_codex_model_metadata_test.go`（func TestCodexToolCapabilitiesNeverAdvertiseNullServiceTiers(t *testing.T) {）。

### 012 `dd7ee217034569a32bbbf10eb3a628196d9ea2fb`

原始标题：style(codex): use tagged switch for capability conflict fallback

**结论：不引入。** 仅把能力冲突fallback的if/else改成tagged switch。

- 关键改动文件/函数：`backend/internal/service/openai_codex_model_metadata.go`。
- 当前仓库证据：当前intersectUpstreamModelMetadata仍if分支，语义与原补丁相同但尚未包含011的service_tiers分支。
- 收益/冲突：无独立行为收益；实施011时可自然采用switch，无需单独移植样式提交。
- 验证/依赖：依赖011的实质回归即可。

### 013 `18380398d587e17a0653a8c8a23585c53d56e380`

原始标题：chore(codex): drop added comments

**结论：不引入。** 移除011中解释service_tiers数组约束的注释及测试注释。

- 关键改动文件/函数：`backend/internal/service/openai_codex_model_metadata.go`。
- 当前仓库证据：当前本地未包含这些新增注释；已有能力校验注释服务于CLI0.164兼容。
- 收益/冲突：无运行收益，不删本地有用原因说明。
- 验证/依赖：不需要单独验证。
- 补丁测试证据（未运行）：`backend/internal/service/openai_codex_model_metadata_test.go`。

### 014 `73381fc9c7648dac3eed2f3f900a97ad225c86ff`

原始标题：fix(openai-ws): bill each later turn with the current group pricing

**结论：需适配。** ResponsesWebSocket在BeforeTurn通过APIKeyService.GetByKey刷新同key同group定价，按turn存当前及前轮快照，利润门与RecordUsage使用同快照。

- 关键改动文件/函数：`backend/internal/handler/openai_gateway_handler.go`（`begin`, `set`, `forTurn`, `refreshOpenAIWSTurnBillingAPIKey`, `withOpenAIWSTurnBillingGroup`）。
- 当前仓库证据：当前BeforeTurn只WithOpenAITurnPricingContext(ctx, apiKey.GroupID)，AfterTurn的RecordUsage仍用建连apiKey；没有openAIWSTurnBillingAPIKeys。
- 收益/冲突：修复长连接调价后仍按旧价计费；不得改变连接绑定的调度组、用户/额度快照或本地ctx_pool/legacy sticky，失败应维持连接快照。
- 验证/依赖：验证第二轮调价、利润门同步、换组/平台/订阅类型不采纳、认证缓存刷新失败及BeforeTurn/AfterTurn交错；跑WS本地模拟测试，不能用真实账户。
- 补丁测试证据（未运行）：`backend/internal/handler/openai_gateway_handler_test.go`；`backend/internal/handler/openai_gateway_ws_group_pricing_test.go`（func TestOpenAIResponsesWebSocket_GroupRateChangeAppliesToNextTurn(t *testing.T) {）。

### 015 `b6a157c19c5741aa8b7b06f14e1db2b8270a7311`

原始标题：fix(openai): account image input usage on WS passthrough

**结论：需适配。** relay Usage增加ImageInputTokens，parseUsageAndAccumulate读取input/prompt详情及tool_usage.image_gen回填，非零合并、累积与adapter两处结果映射同步。

- 关键改动文件/函数：`backend/internal/service/openai_ws_v2/passthrough_relay.go`；`backend/internal/service/openai_ws_v2_passthrough_adapter.go`。
- 当前仓库证据：当前relay只跟踪ImageOutputTokens，adapter的OpenAIUsage映射未传ImageInputTokens；HTTP计费已有图像输入字段，数据在WS relay链路丢失。
- 收益/冲突：修复图片输入漏记、补齐WS/HTTP口径；需同时覆盖逐轮和最终结果，避免跨轮遗留和重复累计。
- 验证/依赖：移植两个新增图片用量测试，验证显式usage优先、tool回填、prompt别名、纯文本后续轮和最终合计；保留本地WS模式选择。
- 补丁测试证据（未运行）：`backend/internal/service/openai_ws_v2/passthrough_relay_image_usage_test.go`（func TestRelay_ImageUsageMatchesHTTPResponsesAccounting(t *testing.T) {）；`backend/internal/service/openai_ws_v2_passthrough_image_usage_test.go`（func TestOpenAIWSPassthroughTurnResultCarriesImageUsage(t *testing.T) {）。

### 016 `f3d752693f26cc4565de196c1305ec467187b9e5`

原始标题：fix(apicompat): flatten named chat tool_choice for Responses

**结论：建议引入。** ChatCompletionsToResponses把命名tool_choice.function.name展平到name，字符串/已是Responses对象保持。

- 关键改动文件/函数：`backend/internal/pkg/apicompat/chatcompletions_to_responses.go`（`convertChatToolChoiceToResponses`）。
- 当前仓库证据：当前out.ToolChoice=req.ToolChoice直接透传；同文件已有legacy function_call转换，命名现代选择仍使用错误形状。
- 收益/冲突：可避免上游拒绝指定工具，改动局限协议转换包。
- 验证/依赖：测试auto/none/required、命名嵌套、已展平、未知对象及原始字段；使用新增tool_choice测试。
- 补丁测试证据（未运行）：`backend/internal/pkg/apicompat/chatcompletions_to_responses_tool_choice_test.go`（func TestChatCompletionsToResponses_ToolChoiceShape(t *testing.T) {）。

### 017 `9a8ab4f88e7318a830be7ad0c299862798bb8d70`

原始标题：fix(apicompat): keep chat developer messages as developer role

**结论：建议引入。** chatMessageToResponsesItems把developer并入system转换并保持m.Role。

- 关键改动文件/函数：`backend/internal/pkg/apicompat/chatcompletions_to_responses.go`。
- 当前仓库证据：当前switch只识别system，developer落到default的chatUserToResponses；chatSystemToResponses固定写system。
- 收益/冲突：防止开发者约束降级成用户消息；两处必须一起改。
- 验证/依赖：验证system/developer/user角色及字符串/多段内容，运行新增developer_role测试。
- 补丁测试证据（未运行）：`backend/internal/pkg/apicompat/chatcompletions_to_responses_developer_role_test.go`（func TestChatCompletionsToResponses_DeveloperRolePreserved(t *testing.T) {）。

### 018 `7a767d5ce874d155ab644d5965d50dff80f3b5df`

原始标题：fix(apicompat): pair legacy chat function calls with their results

**结论：建议引入。** legacyFunctionCallIDs按会话顺序生成不冲突ID并按函数名FIFO配对旧assistant.function_call和role=function结果。

- 关键改动文件/函数：`backend/internal/pkg/apicompat/chatcompletions_to_responses.go`（`assign`, `claim`）。
- 当前仓库证据：当前convertChatMessagesToResponsesInput逐条无状态转换；chatFunctionToResponses直接CallID:m.Name，不能与assistant生成的调用ID一致。
- 收益/冲突：修复旧SDK历史重放及重复同名函数结果配对；保留现代tool_calls路径及无匹配结果的既有fallback。
- 验证/依赖：运行新增legacy_function_call测试：重复名称、重放稳定性、现代ID碰撞、显式ToolCallID和孤立结果。
- 补丁测试证据（未运行）：`backend/internal/pkg/apicompat/chatcompletions_to_responses_legacy_function_call_test.go`（func TestChatCompletionsToResponses_LegacyFunctionCallHistoryPaired(t *testing.T) {）。

### 019 `775c1a3c1dcd23744b234623162149aa565b8eb0`

原始标题：fix(apicompat): preserve refusals when converting Responses to Chat

**结论：建议引入。** ResponsesContentPart/ChatMessage/ChatDelta补Refusal；非流式、refusal.delta、BufferedResponseAccumulator和HasContent/BuildOutput贯通。

- 关键改动文件/函数：`backend/internal/pkg/apicompat/responses_to_chatcompletions.go`（`resToChatHandleRefusalDelta`）；`backend/internal/pkg/apicompat/types.go`。
- 当前仓库证据：当前ResponsesToChatCompletions只读output_text，流式switch无response.refusal.delta，缓冲器只认text/functions/reasoning。
- 收益/冲突：避免拒绝回答变成空成功或触发错误failover；必须同时改结构和三条输出路径。
- 验证/依赖：新增refusal测试覆盖普通响应、流式delta、断尾缓冲、拒绝与文本/工具并存；检查JSON omitempty语义。
- 补丁测试证据（未运行）：`backend/internal/pkg/apicompat/responses_to_chatcompletions_refusal_test.go`（func TestResponsesToChatCompletions_RefusalPreserved(t *testing.T) {）。

### 020 `15f469c86b77378a493aed688aa755ab2daa5a3f`

原始标题：fix(gateway): ignore negative content block index in buffered Anthropic paths

**结论：建议引入。** 四条Anthropic缓冲聚合在切片前加idx>=0，仍按长度上界过滤。

- 关键改动文件/函数：`backend/internal/service/gateway_forward_as_chat_completions.go`；`backend/internal/service/gateway_forward_as_responses.go`；`backend/internal/service/openai_gateway_chat_completions_anthropic_native.go`；`backend/internal/service/openai_gateway_responses_anthropic_native.go`。
- 当前仓库证据：本地handleCCBufferedFromAnthropic、handleResponsesBufferedStreamingResponse及两个native对应函数均只有idx<len(finalResp.Content)。
- 收益/冲突：非受控上游负index会触发panic；四入口共享同故障，不能只补一处。
- 验证/依赖：运行新增negative_index表驱动测试，含负数、越界及合法块仍正确聚合。
- 补丁测试证据（未运行）：`backend/internal/service/anthropic_buffered_negative_index_test.go`（func TestAnthropicBufferedAggregators_IgnoreNegativeBlockIndex(t *testing.T) {）。

### 021 `412718edf590e8fe39220eb52a05e6fdab071a68`

原始标题：fix(apicompat): always include `signature` on Anthropic thinking blocks

**结论：建议引入。** AnthropicContentBlock/ClaudeContentItem序列化thinking时显式输出空signature；Antigravity两个startBlock构造同补空值。

- 关键改动文件/函数：`backend/internal/pkg/antigravity/claude_types.go`（`MarshalJSON`）；`backend/internal/pkg/antigravity/stream_transformer.go`；`backend/internal/pkg/apicompat/types.go`。
- 当前仓库证据：当前Anthropic MarshalJSON只强制thinking；ClaudeContentItem没有此自定义序列化，processThinking/emitEmptyThinkingWithSignature起始块无signature。
- 收益/冲突：严格客户端要求键存在；需覆盖Antigravity独立手工map输出而不只结构体。
- 验证/依赖：验证所有thinking JSON包含thinking/signature，即使空；非thinking块保持原样；跑wire及Antigravity序列化测试。
- 补丁测试证据（未运行）：`backend/internal/pkg/antigravity/claude_types_test.go`（func TestClaudeContentItem_ThinkingAlwaysHasSignature(t *testing.T) {）；`backend/internal/pkg/apicompat/anthropic_responses_test.go`；`backend/internal/pkg/apicompat/thinking_signature_wire_test.go`（func TestAnthropicContentBlock_ThinkingAlwaysHasSignature(t *testing.T) {）。

### 022 `da4419e626110c854d356eedfb060b8306f619a8`

原始标题：fix(anthropic): keep cache TTL order valid on the mimic path

**结论：建议引入。** enforceCacheControlLimit拆块数限制后按tools→system→messages→顶层修TTL顺序，把最后1h之前的ephemeral断点提升为1h。

- 关键改动文件/函数：`backend/internal/service/gateway_claude_oauth_body.go`（`enforceCacheControlBlockLimit`, `normalizeCacheControlTTLOrder`）。
- 当前仓库证据：当前函数只移除多余cache_control，无normalizeCacheControlTTLOrder；mimic路径可注入5m工具/系统断点后保留用户messages的1h。
- 收益/冲突：避免有效请求经mimic变上游400；注意只改已有ephemeral、不创建断点且保留4块限制。
- 验证/依赖：新增TTL顺序测试含缺ttl视5m、顶层、有效请求不变、非ephemeral；与现有1h注入/块数测试一起验证。
- 补丁测试证据（未运行）：`backend/internal/service/gateway_cache_ttl_order_test.go`（func TestNormalizeCacheControlTTLOrder_RaisesBreakpointsBeforeLast1h(t *testing.T) {）。

### 023 `852f0f824fd7e7f038ac8a3649701bafbbf600b1`

原始标题：fix(anthropic): preserve requested tool change beta headers

**结论：建议引入。** Claude beta常量增加两种工具变化token；OAuth mimic仅透传客户端明确携带且不被策略drop的三种兼容beta。

- 关键改动文件/函数：`backend/internal/pkg/claude/constants.go`；`backend/internal/service/gateway_upstream_request.go`。
- 当前仓库证据：当前computeFinalAnthropicBeta的mimic分支只保留structured-outputs，其他工具变化beta被过滤。
- 收益/冲突：让请求体工具变化与头部能力一致；不能默认启用未知beta或推断缺少伴随token。
- 验证/依赖：新增gateway_tool_changes_beta测试覆盖显式请求、未请求、drop优先级、普通OAuth和API Key路径。
- 补丁测试证据（未运行）：`backend/internal/service/gateway_tool_changes_beta_test.go`（func TestBuildUpstreamRequestToolChangeBetas(t *testing.T) {）。

### 024 `61e1afdd499b37668fdcda374ea51aac7ec9ee3f`

原始标题：perf(openai): lazily rebuild input when stripping namespaces

**结论：建议引入。** namespace清理延迟到首个实际删除才分配buffer，按gjson相对Index复制未修改原文片段。

- 关键改动文件/函数：`backend/internal/service/openai_responses_namespace.go`。
- 当前仓库证据：当前stripOpenAIResponsesInputNamespaces无论是否修改都Grow/逐item转换并重建数组。
- 收益/冲突：减少长历史无namespace请求分配，同时保留空白/原始表示；偏移错误会损坏请求，需实测而非只看性能标题。
- 验证/依赖：用新增保留原文、null、keepToolCallNamespaces测试及benchmark；验证input在请求内非零偏移、Unicode、空数组。
- 补丁测试证据（未运行）：`backend/internal/service/openai_responses_namespace_benchmark_test.go`；`backend/internal/service/openai_responses_namespace_test.go`（func TestStripOpenAIResponsesInputNamespacesPreservesUnchangedSpans(t *testing.T) {）。

### 025 `3613174cf653b77508c0ed10eda816d658c928c1`

原始标题：fix(grok): fail over silent-refusal empty response.completed on Grok Responses streams

**结论：建议引入。** 空response.completed/done静默拒绝failover条件增加PlatformGrok，已有输出/usage/error条件维持。

- 关键改动文件/函数：`backend/internal/service/openai_gateway_response_handling.go`。
- 当前仓库证据：当前handleStreamingResponseWithReasoning只对PlatformOpenAI启用此保护；forwardGrokResponses使用同响应处理链。
- 收益/冲突：防止Grok空成功记0/0；只在尚未向客户端输出时切换，不重放已消费结果。
- 验证/依赖：与026一组；验证Grok空终结failover、有效输出成功、已发数据不切换。

### 026 `27f4a1780fc7dddec8dccef1a5f06bfaa445645f`

原始标题：test(grok): cover empty response.completed failover for PlatformGrok

**结论：建议引入。** openai_gateway_responses_empty_completed_test新增Grok空终结失败及正常输出成功用例。

- 关键改动文件/函数：无生产代码改动。
- 当前仓库证据：当前生产判断只有OpenAI；本地测试文件存在但新增Grok用例未覆盖该分支。
- 收益/冲突：把025的业务意图固定，不能当独立生产修复。
- 验证/依赖：依赖025，运行该文件对应测试并覆盖response.done别名。
- 补丁测试证据（未运行）：`backend/internal/service/openai_gateway_responses_empty_completed_test.go`（func TestOpenAIResponsesEmptyCompletedFailsOverGrok(t *testing.T) {）。

### 027 `9f84204c2183fc9b822f2dbc2136b8911d3bb4c0`

原始标题：fix(streaming): preserve chat stream heartbeat comments

**结论：建议引入。** handleCCStreamingFromAnthropic把ping转成SSE注释 ': ping' 并Flush，写失败终止，不进入首token计时。

- 关键改动文件/函数：`backend/internal/service/gateway_forward_as_chat_completions.go`。
- 当前仓库证据：当前processAnthropicEvent遇ping直接return false丢弃；firstChunk计时在其后，位置适合保持。
- 收益/冲突：长时间thinking时保持下游连接，同时不向OpenAI客户端泄漏Anthropic event:ping。
- 验证/依赖：新增ping测试检查即时Flush、TTFT不被改变、写失败终止及正常消息转换。
- 补丁测试证据（未运行）：`backend/internal/service/gateway_forward_as_chat_completions_ping_test.go`（func TestHandleCCStreamingFromAnthropic_PingFlushesImmediately(t *testing.T) {）。

### 028 `aa3b04900019e018030e1fa909bc9a5e803a759e`

原始标题：fix(opencode): cap Retry-After hint at 24h for usage refresh backoff

**结论：建议引入。** nextOpenCodeGoUsageDelay先把上游Retry-After限在0至24h，再进入现有退避和jitter计算。

- 关键改动文件/函数：`backend/internal/service/opencode_go_usage.go`。
- 当前仓库证据：当前base本地封顶后会被retryAfterDuration重新抬高，minimumDelay也未封顶，可让自动刷新长时间冻结。
- 收益/冲突：真实网络提示不能无限期推迟刷新；本提交只约束hint，不声称所有jitter结果绝对不超过24h。
- 验证/依赖：运行大Retry-After刷新测试，补边界负值/24h和正常hint；不访问OpenCode。
- 补丁测试证据（未运行）：`backend/internal/service/opencode_go_usage_test.go`（func TestOpenCodeGoUsageRefreshClampsHugeRetryAfter(t *testing.T) {）。

### 029 `480f668ba9388d9ec4323ed09ebfe05d55691620`

原始标题：fix(opencode): cap Retry-After hint for billing probe schedule

**结论：建议引入。** nextProbeDelay同样钳制Retry-After，unsupportedProbeDelay说明更新。

- 关键改动文件/函数：`backend/internal/service/upstream_billing_probe.go`。
- 当前仓库证据：当前nextProbeDelay在retryAfter大于interval时直接return原hint，绕过upstreamBillingProbeMaxDelay。
- 收益/冲突：防止账单探测长期停止；与028属于不同调度函数，不能只移一处。
- 验证/依赖：验证nextProbeDelay/unsupportedProbeDelay大hint最多24h、正常hint和失败重探；运行新增probe测试。
- 补丁测试证据（未运行）：`backend/internal/service/upstream_billing_probe_test.go`（func TestUpstreamBillingProbeRetryAfterIsCappedAtMaxDelay(t *testing.T) {）。

### 030 `df89228c2db370080557a2c41887f3c741aa65d0`

原始标题：fix(opencode): report 403 as neutral forbidden in usage snapshot

**结论：建议引入。** OpenCode usage刷新403使用中性错误key forbidden，前端解释订阅或WAF/策略均可能。

- 关键改动文件/函数：`backend/internal/service/opencode_go_usage.go`；`frontend/src/i18n/locales/en/admin/accounts.ts`；`frontend/src/i18n/locales/zh/admin/accounts.ts`。
- 当前仓库证据：当前refreshLoadedAccount把所有403写成subscription required，前端两语言errors无forbidden。
- 收益/冲突：改正误诊，不改变停调/重试策略。
- 验证/依赖：运行403快照测试并检查前端i18n键显示，401/429策略保持。
- 补丁测试证据（未运行）：`backend/internal/service/opencode_go_usage_test.go`（func TestOpenCodeGoUsageRefresh403Forbidden(t *testing.T) {）。

### 031 `0dccd7b6f59c880e5ddc78a9a9f74f9dabf47db3`

原始标题：fix(opencode): correct Zen qwen3.8-max endpoint and reject unsupported models

**结论：需适配。** Zen精确qwen3.8-max规则在qwen*前走CC；三入站和连通测试显式拒绝gemini-/jev-专有端点模型。

- 关键改动文件/函数：`backend/internal/service/account_test_service.go`；`backend/internal/service/openai_gateway_chat_completions.go`；`backend/internal/service/openai_gateway_forward.go`；`backend/internal/service/openai_gateway_messages.go`；`backend/internal/service/opencode_go.go`（`IsOpenCodeUnsupportedModel`, `writeOpenCodeUnsupportedModelError`）；`frontend/src/components/account/credentialsBuilder.ts`。
- 当前仓库证据：本地DefaultOpenCodeZenProtocolRules和credentialsBuilder默认表仅有qwen*→Anthropic；网关各OpenCode入口无IsOpenCodeUnsupportedModel。
- 收益/冲突：端点修正可独立引入；专有模型拒绝须限于OpenCode标准网关，不能误封本地TypeSafe /systemone或其他平台；已有账号保存的规则不应强制改写。
- 验证/依赖：测试三入站错误形状、连通测试、精确规则优先和自定义protocol_rules；先核对当前官方协议事实，本轮仅基于补丁证据。
- 补丁测试证据（未运行）：`backend/internal/service/opencode_go_test.go`（func TestIsOpenCodeUnsupportedModel(t *testing.T) {）；`backend/internal/service/opencode_unsupported_models_test.go`（func TestForwardOpenCodeUnsupportedModels_ChatCompletionsReturns400(t *testing.T) {）。

### 032 `54810f8c820f3a7911b9955124af15fea616e77c`

原始标题：fix(grok): include consent token and origin in SSO device approval

**结论：建议引入。** xAI SSO保存verify页body、解析隐藏consent_token，approve补Origin/Referer；额外头只应用首跳。

- 关键改动文件/函数：`backend/internal/pkg/xai/sso_device.go`（`ssoConsentToken`, `do`）。
- 当前仓库证据：当前convert丢verify响应body，approve只传user_code/action/principal字段；do无额外头参数，已有safeXAIAuthURL限制。
- 收益/冲突：恢复设备授权所需会话信息；不得把token写日志或把Origin/Referer转发到未信任重定向。
- 验证/依赖：运行ssoConsentToken和本地HTTP流程测试，覆盖属性顺序/HTML实体/无token、重定向首跳和可信域检查；不做真实SSO。
- 补丁测试证据（未运行）：`backend/internal/pkg/xai/sso_device_test.go`（func TestSSOConsentToken(t *testing.T) {）。

### 033 `0173861a5aaa9e92519263a1f5685850712a3e14`

原始标题：fix(channel-monitor): build zhipu probe URL from the configured endpoint

**结论：建议引入。** callProvider使用monitorRequestPath按智谱端点决定/v1/chat/completions或/api/paas/v4路径，识别已含paas/v4和官方子域。

- 关键改动文件/函数：`backend/internal/service/channel_monitor_checker.go`（`monitorRequestPath`, `isZhipuOfficialHost`）。
- 当前仓库证据：当前直接joinURL(endpoint, adapter.buildPath(model))，provider固定path不区分中转/官方网关。
- 收益/冲突：修复自定义Zhipu监控地址拼错；继续复用joinURL的路径前缀处理。
- 验证/依赖：新增表覆盖官方根、coding/paas/v4、中转/v1、尾斜线、伪造官方后缀及其他provider不变。
- 补丁测试证据（未运行）：`backend/internal/service/channel_monitor_endpoint_test.go`（func TestMonitorRequestPath_Zhipu(t *testing.T) {）。

### 034 `856d7ea9056b08b9f52831d085741370e1863959`

原始标题：fix: expose mapped antigravity test models

**结论：建议引入。** AccountHandler.GetAvailableModels的Antigravity分支读显式model_mapping请求侧key并排序/补展示名，空映射回默认。

- 关键改动文件/函数：`README.md`；`README_CN.md`；`backend/internal/handler/admin/account_handler.go`（`antigravityAccountTestModels`）。
- 当前仓库证据：当前Antigravity直接antigravity.DefaultModels；实际TestAccountConnection仍按account模型映射发上游，选择器与执行不一致。
- 收益/冲突：自定义映射别名可用于连通测试；不能用GetModelMapping加入管理员没配置的兼容别名。
- 验证/依赖：测试map[string]any/map[string]string、空/空白key、默认回退、映射key实际发送value；跳过README添加并结合035。
- 补丁测试证据（未运行）：`backend/internal/handler/admin/account_handler_available_models_test.go`（func TestAccountHandlerGetAvailableModels_Antigravity(t *testing.T) {）。

### 035 `f385bba928706420de54beb8517a6c91a63c03d2`

原始标题：docs: keep connectivity test fix out of README

**结论：不引入。** 只撤销034新增的英/中文README账号测试模型段落。

- 关键改动文件/函数：`README.md`；`README_CN.md`。
- 当前仓库证据：本地README没有该段，README_CN.md不存在。
- 收益/冲突：已处于无新增说明的目标状态；034只取代码和测试即可。
- 验证/依赖：无独立测试。

### 036 `5ef517d0fc0d8bbc43940e79bf023d6182ad080e`

原始标题：fix(accounts): attribute connectivity test failure logs

**结论：建议引入。** 新增account_test_logging.go：测试ID、账号/平台/来源、模型/模式归因；先脱敏后截断，后台入口设置source。

- 关键改动文件/函数：`backend/internal/service/account_test_logging.go`（`accountTestBackgroundKey`, `initAccountTestLogger`, `bindAccountTestPlatform`, `logAccountTestError`）；`backend/internal/service/account_test_service.go`。
- 当前仓库证据：当前sendErrorAndEnd用log.Printf裸错误文本；RunTestBackground构造Gin上下文后与HTTP共用TestAccountConnection，无独立日志归因。
- 收益/冲突：便于定位哪个账号测试失败，并防止凭据出现在错误日志；复用已有logger/logredact。
- 验证/依赖：新增日志测试检查HTTP请求ID、后台关联、缺失账号、脱敏顺序和长度；不记录原始真实凭据。
- 补丁测试证据（未运行）：`backend/internal/service/account_test_logging_test.go`（func TestAccountTestFailureLogAttribution(t *testing.T) {）。

### 037 `71e97d1353a1cdb788c82a0e8b9020e4e1e0e614`

原始标题：fix(models): enforce account mappings for remote Codex discovery

**结论：需适配。** ApplyPinnedCodexModelsMapping取消仅pinned开关限制，所有远程发现投影账号映射；未变化body/单目录保留原文和ETag。

- 关键改动文件/函数：`backend/internal/service/openai_codex_models_pinned.go`；`backend/internal/service/openai_models_list.go`。
- 当前仓库证据：当前handler调用映射函数，但函数被!CodexModelsManifestConfig.Enabled短路；projectAccountModelsBody重编码，FetchPinned无单结果原响应捷径。
- 收益/冲突：修复普通remote发现泄漏不可服务模型；本地CLI0.164 manifest字段/ETag与BPS目录不能丢。
- 验证/依赖：验证普通/pinned远程、通配映射、显式空列表、单目录/重复ID、无变化保ETag、未知字段保留；本地生成目录维持现有政策。
- 补丁测试证据（未运行）：`backend/internal/handler/openai_codex_models_handler_test.go`（func TestCodexModelsDiscoveryProjectsWildcardMappingWithoutAllowlist(t *testing.T) {）；`backend/internal/service/openai_codex_models_pinned_test.go`（func TestMergeCodexModelsManifestBodiesSingleBodyDeduplicates(t *testing.T) {）；`backend/internal/service/openai_models_list_test.go`（func TestApplyCodexModelsMappingPreservesUnchangedBodyAndETag(t *testing.T) {）。

### 038 `abdfbb704cef4f0d6bc4e5fec416d063806753a3`

原始标题：fix(web): bypass bare API aliases from embedded frontend fallback

**结论：建议引入。** shouldBypassEmbeddedFrontend补裸API别名和子路径，避免SPA返回index.html。

- 关键改动文件/函数：`backend/internal/web/embed_on.go`。
- 当前仓库证据：当前只覆盖/api等前缀以及/models、/responses、images/videos部分路径，缺/chat/completions、embeddings、messages/count_tokens、tts等。
- 收益/冲突：未匹配方法/路由时API应得到API错误而非HTML；要对照本地routes里已实现别名，不新增路由功能。
- 验证/依赖：带embed构建标签运行裸路径回归，验证GET/POST与真实前端页面不被误旁路。
- 补丁测试证据（未运行）：`backend/internal/web/embed_test.go`（func TestEmbeddedFrontendBypassesBareAPIAliases(t *testing.T) {）。

### 039 `440784914210ab4e628699752b6a3d3554bc6412`

原始标题：fix(channels): 追加模型时不再改写已留空的价格条目

**结论：建议引入。** PricingEntryCard.onModelsUpdate只在原条目无模型且一次新增一个模型时自动填默认价。

- 关键改动文件/函数：`Makefile`；`frontend/src/components/admin/channel/PricingEntryCard.vue`。
- 当前仓库证据：当前只addedModels.length===0时退出；已有模型但价格故意留空时追加模型会取新模型价覆写整条。
- 收益/冲突：防止按模型官方回退的渠道条目被静默改价；单模型新条目的现有便利仍保留。
- 验证/依赖：新增组件测试验证已有模型/多模型新增不改价、单模型首次自动填充、显式价格不覆盖；Makefile检查项同步。
- 补丁测试证据（未运行）：`frontend/src/components/admin/channel/__tests__/PricingEntryCard.modelDefaultPrice.spec.ts`（describe('PricingEntryCard default price auto-fill', () => {）。

### 040 `8811c125ecaa8d5bfa538d870c8c447d9b66aedc`

原始标题：fix(groups): ignore stale rate multiplier loads

**结论：建议引入。** GroupRateMultipliersModal用loadVersion隔离弹窗/组切换，清旧列表、仅当前请求更新状态，卸载失效。

- 关键改动文件/函数：`frontend/src/components/admin/group/GroupRateMultipliersModal.vue`。
- 当前仓库证据：当前watch只props.show；loadEntries返回后无版本检查，旧组响应可覆盖新组并过早关闭loading。
- 收益/冲突：防止管理员在错误用户倍率列表上继续操作；保留本地分组平台展示。
- 验证/依赖：延迟promise测试旧请求晚成功/失败、同弹窗切组、关闭重开和卸载。
- 补丁测试证据（未运行）：`frontend/src/components/admin/group/__tests__/GroupRateMultipliersModal.requests.spec.ts`（describe('group rate multiplier request ownership', () => {）。

### 041 `3f2f66f3280d5c6ca457eb57a01adac83b67052d`

原始标题：fix(profile): preserve username edits made during save

**结论：建议引入。** ProfileEditForm保存submittedUsername，按savedUsername识别干净草稿，仅未继续编辑时采用服务端返回名。

- 关键改动文件/函数：`frontend/src/components/user/profile/ProfileEditForm.vue`。
- 当前仓库证据：当前props.initialUsername watcher无条件覆盖username，保存请求直接取当前username；authStore更新可触发覆盖。
- 收益/冲突：用户保存中继续输入不会丢草稿，干净表单仍跟随服务器规范化结果。
- 验证/依赖：组件测试干净同步、保存中编辑、服务器规范化名、保存失败和连续保存。
- 补丁测试证据（未运行）：`frontend/src/components/user/profile/__tests__/ProfileEditForm.draft.spec.ts`（describe('profile username draft', () => {）。

### 042 `f022a04573268e107e2b0438f6049c5083a669c5`

原始标题：fix(admin): use the saved passthrough rule enabled state

**结论：建议引入。** ErrorPassthroughRulesModal.toggleEnabled采用服务端返回updated.enabled。

- 关键改动文件/函数：`frontend/src/components/admin/ErrorPassthroughRulesModal.vue`。
- 当前仓库证据：当前await后rule.enabled=!rule.enabled；API对象本身可能已被更新，二次取反把已保存状态翻回。
- 收益/冲突：让UI以保存结果为准，避免重复操作状态错乱。
- 验证/依赖：测试开关双向、失败不改、API共享对象和返回最终状态。
- 补丁测试证据（未运行）：`frontend/src/components/admin/__tests__/ErrorPassthroughRulesModal.toggle.spec.ts`（describe('error passthrough enabled state', () => {）。

### 043 `795a7e27831985178c75a59ab45266953962940d`

原始标题：fix(payment): stop Airwallex checkout after leaving the page

**结论：建议引入。** AirwallexPaymentView卸载置disposed，SDK导入和init两个await后检查，阻止迟到checkout跳转。

- 关键改动文件/函数：`frontend/src/views/user/AirwallexPaymentView.vue`。
- 当前仓库证据：当前onMounted在这两个await后直接redirectToCheckout/window.location.assign，组件没有onUnmounted失效标记。
- 收益/冲突：用户已离开页面时不再被支付SDK带走；只取消后续UI动作，不回滚已创建订单。
- 验证/依赖：mock SDK导入/init延迟和卸载，验证未卸载仍正常跳转；不用真实支付。
- 补丁测试证据（未运行）：`frontend/src/views/user/__tests__/AirwallexPaymentView.lifecycle.spec.ts`（describe('Airwallex checkout lifecycle', () => {）。

### 044 `7b1aa93148d5cb019f6687dff821c4660aac1231`

原始标题：fix(groups): preserve numeric values when editing RPM overrides

**结论：建议引入。** GroupRPMOverridesModal.updateLocalRpm以Number和整数判定替换parseInt，空字符串不写。

- 关键改动文件/函数：`frontend/src/components/admin/group/GroupRPMOverridesModal.vue`。
- 当前仓库证据：当前parseInt(value,10)把1e3变1、12.5变12；数值输入可能以指数形式表示。
- 收益/冲突：避免保存错误RPM限制；0保留合法，非整数不静默截断。
- 验证/依赖：测试0、1e3、空/空白、负数和小数，验证保存payload。
- 补丁测试证据（未运行）：`frontend/src/components/admin/group/__tests__/GroupRPMOverridesModal.edit.spec.ts`（describe('existing RPM override edits', () => {）。

### 045 `b07b1fb5d9fe7cbb7f066ea03bd480a742ba1d04`

原始标题：fix(ops): validate alert rule durations as whole minutes

**结论：建议引入。** OpsAlertRulesCard要求sustained_minutes/cooldown_minutes为整数分钟，数值范围不变。

- 关键改动文件/函数：`frontend/src/views/admin/ops/components/OpsAlertRulesCard.vue`。
- 当前仓库证据：当前Number.isFinite允许小数，后端分钟字段是整数；表单可提交被拒绝配置。
- 收益/冲突：在实际管理边界匹配后端契约；阈值字段允许小数，不应一并限制。
- 验证/依赖：新增duration组件测试覆盖1/1440、0冷却、小数分钟拒绝、小数metric阈值仍接受。
- 补丁测试证据（未运行）：`frontend/src/views/admin/ops/components/__tests__/OpsAlertRulesCard.duration.spec.ts`（describe('alert rule integer durations', () => {）。

### 046 `81bd85ec55eb6abcb8f8e977577e4dd14117f974`

原始标题：fix(ops): require loaded settings before saving

**结论：建议引入。** OpsSettingsDialog.loadAllSettings成功后置settingsLoaded；saveAllSettings及按钮同时挡加载、失败默认值和重复保存。

- 关键改动文件/函数：`frontend/src/views/admin/ops/components/OpsSettingsDialog.vue`（`settingsLoaded`）。
- 当前仓库证据：当前保存按钮仅看saving/validation，初始加载或某请求失败仍可能拿默认threshold提交覆盖真实配置。
- 收益/冲突：避免真实设置丢失，属于不可随意删除的产品保存边界保护。
- 验证/依赖：mock并行加载部分未结束/失败时不得发保存请求，全部成功后允许，重复点击只一次。
- 补丁测试证据（未运行）：`frontend/src/views/admin/ops/components/__tests__/OpsSettingsDialog.loading.spec.ts`（describe('ops settings load guard', () => {）。

### 047 `b47204251393d5eb673fb1db5601df3fd2262b8a`

原始标题：fix(ops): discard stale alert event pages after filter changes

**结论：建议引入。** OpsAlertEventsCard首屏递增listRequestId，分页沿用epoch，筛选/卸载使旧页失效并各自保护finally。

- 关键改动文件/函数：`frontend/src/views/admin/ops/components/OpsAlertEventsCard.vue`。
- 当前仓库证据：当前loadFirstPage/loadMore无请求归属，筛选后的列表可能混入旧before_id游标结果。
- 收益/冲突：避免筛选列表内容错乱及loading状态错误。
- 验证/依赖：测试旧分页晚到、旧首屏失败、筛选重置、卸载与当前请求loading。
- 补丁测试证据（未运行）：`frontend/src/views/admin/ops/components/__tests__/OpsAlertEventsCard.pagination.spec.ts`（describe('alert events filtered pagination', () => {）。

### 048 `4551fb0d86e5d78aac3af450c40434886c02fcf4`

原始标题：fix(monitor): ignore obsolete template manager list responses

**结论：建议引入。** MonitorTemplateManagerDialog.fetchTemplates版本化，关闭/卸载递增，只当前响应更新列表及错误/loading。

- 关键改动文件/函数：`frontend/src/components/admin/monitor/MonitorTemplateManagerDialog.vue`。
- 当前仓库证据：当前list请求结果直接赋templates，关闭后旧请求仍可覆盖重开会话。
- 收益/冲突：防止模板列表闪回及已关闭弹窗报错。
- 验证/依赖：mock关闭重开、旧失败、重复fetch与卸载；当前返回仍可编辑/删除。
- 补丁测试证据（未运行）：`frontend/src/components/admin/monitor/__tests__/MonitorTemplateManagerDialog.requests.spec.ts`（describe('monitor template list requests', () => {）。

### 049 `8f636513f7d4427e51d9688595f55a3b71c42ada`

原始标题：fix(admin): keep scheduled test results scoped to the expanded plan

**结论：建议引入。** ScheduledTestsPanel以expandedPlanId同步watch清空results/失效请求；toggleExpand只接收当前版本结果。

- 关键改动文件/函数：`frontend/src/components/admin/account/ScheduledTestsPanel.vue`。
- 当前仓库证据：当前直接results.value=await listResults(planId,20)，多计划切换/同计划折叠重开时会显示过时结果。
- 收益/冲突：保持执行结果属于当前展开计划，避免把别的账号测试结果当当前结果。
- 验证/依赖：测试A→B、同计划重开、失败/卸载、loading不会被旧请求结束。
- 补丁测试证据（未运行）：`frontend/src/components/admin/account/__tests__/ScheduledTestsPanel.results.spec.ts`（describe('scheduled test result selection', () => {）。

### 050 `1c996fe301d761beb1b443a383cfe3d76abcd87b`

原始标题：fix(payment): stop late Stripe callbacks after leaving the page

**结论：建议引入。** StripePaymentView卸载失效；loadStripe后、mountPaymentElement/startPolling/scheduleClose前检查disposed。

- 关键改动文件/函数：`frontend/src/views/user/StripePaymentView.vue`。
- 当前仓库证据：当前卸载只清已有timer；在途WeChat确认返回后仍可startPolling或scheduleClose，清理先于创建不能阻止。
- 收益/冲突：避免离页后重启轮询、关闭窗口或跳转；不改变支付确认本身。
- 验证/依赖：新增Stripe生命周期测试mock延迟QR结果，验证无新timer/跳转，正常流程与支付成功仍有效。
- 补丁测试证据（未运行）：`frontend/src/views/user/__tests__/StripePaymentView.spec.ts`（it('does not restart polling when a WeChat QR result arrives after leaving', async () => {）。

### 051 `4b6ed15a8b79db6c26be31fd4c20c80b8978d8d0`

原始标题：fix(custom-pages): ignore obsolete Markdown responses

**结论：建议引入。** CustomPageView对fetch、resp.text和nextTick后操作加markdownRequestVersion，切slug/非markdown/卸载失效。

- 关键改动文件/函数：`frontend/src/views/user/CustomPageView.vue`。
- 当前仓库证据：当前fetchAndRenderMarkdown可让旧slug正文/错误覆盖新页面，finally仍无条件injectCopyButtons。
- 收益/冲突：防止内容/目录错配及卸载后DOM注入，现有Markdown净化策略必须保留。
- 验证/依赖：测试headers/body不同阶段乱序、旧失败、新loading、切iframe与卸载，保持净化和图片URL处理。
- 补丁测试证据（未运行）：`frontend/src/views/user/__tests__/CustomPageView.race.spec.ts`（describe('custom Markdown page request ordering', () => {）。

### 052 `abbe42ac44130c9edd65728ff4e3fae3c323b5a1`

原始标题：fix(profile): preserve email binding draft during user refresh

**结论：建议引入。** ProfileIdentityBindingsSection增加dirty标记和resetEmailBindingForm，同用户后台刷新不覆盖验证码收件地址/草稿，换用户重置。

- 关键改动文件/函数：`Makefile`；`frontend/src/components/user/profile/ProfileIdentityBindingsSection.vue`（`isEmailBindingFormDirty`, `resetEmailBindingForm`）。
- 当前仓库证据：当前props.user watcher在每次刷新时用user.email覆写emailBindingForm.email，绑定成功只手动清code/password。
- 收益/冲突：防止验证码发送后目标地址被改回旧邮箱及跨用户草稿残留。
- 验证/依赖：新增测试含发送中未主动编辑也应dirty、主动清空地址、换用户、绑定成功重置；同步Makefile测试项。
- 补丁测试证据（未运行）：`frontend/src/components/user/profile/__tests__/ProfileIdentityBindingsSection.spec.ts`（describe('email form synchronization', () => {）。

### 053 `d5f490e106a4103e1e521a4a0648b1af9ac1a744`

原始标题：fix(usage): replace misleading long-context multiplier badge

**结论：建议引入。** UsageTable把长上下文标记x2改成翻译文字和按定价档位解释的tooltip。

- 关键改动文件/函数：`frontend/src/components/admin/usage/UsageTable.vue`；`frontend/src/i18n/locales/en/admin/resources.ts`；`frontend/src/i18n/locales/zh/admin/resources.ts`。
- 当前仓库证据：当前模板仍在long_context_billing_applied时显示x2；计费结构并不承诺输入输出统一倍率。
- 收益/冲突：改正展示误导，不改实际金额。
- 验证/依赖：组件断言标记和两语言tooltip；无需运行账务写入测试。
- 补丁测试证据（未运行）：`frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`。

### 054 `679a7186fb03830c5ad30303daeff83ca53b69d4`

原始标题：feat(accounts): compact account search and filters

**结论：不引入。** AccountTableFilters改成紧凑双层布局、更多筛选折叠/激活数量与标签，AccountsView对齐。

- 关键改动文件/函数：`frontend/src/components/admin/account/AccountTableFilters.vue`（`moreFiltersOpen`, `moreFiltersId`, `activeSecondaryCount`）；`frontend/src/i18n/locales/en/admin/accounts.ts`；`frontend/src/i18n/locales/zh/admin/accounts.ts`；`frontend/src/views/admin/AccountsView.vue`。
- 当前仓库证据：当前组件仍flex-wrap平铺；事件由父AccountsView处理，补丁不改变筛选查询语义。
- 收益/冲突：属于布局偏好，非修复；无用户布局需求时不必扩大移植范围。
- 验证/依赖：若以后采纳，需桌面/窄屏键盘/可访问性验证及折叠不清空筛选测试；本轮未做截图QA。
- 补丁测试证据（未运行）：`frontend/src/components/admin/account/__tests__/AccountTableFilters.spec.ts`（describe('AccountTableFilters', () => {）。

### 055 `1888de135dd76378ee7a9c6f30ac42aeac88da5f`

原始标题：fix(models): list composite exact route aliases

**结论：需适配。** 为组合模型目录补enabled exact public aliases，普通/models不限定endpoint，Codex限定responses；resolver/service/handler一起改。

- 关键改动文件/函数：`backend/internal/handler/gateway_handler.go`（`compositeAvailableModels`）；`backend/internal/service/composite_route_resolver.go`（`ListExactPublicModels`）；`backend/internal/service/gateway_service.go`（`GetCompositeRouteModels`）。
- 当前仓库证据：本地compositeAvailableModels已改成(ctx,groupID,includeSystemOne)([]string,error)，最后调用CompleteCompositeBPSModelCatalog；尚无ListExactPublicModels/GetCompositeRouteModels。
- 收益/冲突：有用但不能直接套旧签名。需结合5800abf1合并提交对includeSystemOne/FilterCodexModels和真实目标的增强；保留BPS补全、nil/empty/error语义及TypeSafe专用端点过滤。
- 验证/依赖：测试exact/prefix、disabled、any/messages/responses路由、public别名去重、TypeSafe目标、BPS有效/空/失败目录；与父审merges.md联动。
- 补丁测试证据（未运行）：`backend/internal/handler/gateway_models_test.go`（func TestGatewayCompositeRouteModelsAppearInBothCatalogs(t *testing.T) {）。

### 056 `6f6a27f9c6e5526bfe5c45ff1cfd4d7cca33750d`

原始标题：feat(usage): show per-request output TPS by default

**结论：已覆盖。** UsageTable默认显示每请求output_tokens/总耗时tok/s；latencyHealth排除图片/不适用请求类型/无有效时长；中英解释及测试。

- 关键改动文件/函数：`frontend/src/components/admin/usage/UsageTable.vue`；`frontend/src/i18n/locales/en/dashboard.ts`；`frontend/src/i18n/locales/zh/dashboard.ts`；`frontend/src/utils/latencyHealth.ts`（`formatUsageOutputRate`）。
- 生成/媒体文件：`assets/screenshots/usage-output-tps-desktop.png`、`assets/screenshots/usage-output-tps-mobile.png`；仅核对用途/存在状态，未逐行或逐像素评审。
- 当前仓库证据：当前UsageTable有data-testid=output-tps并调用formatUsageOutputRate，latencyHealth.ts相同实现；latencyHealth.spec.ts已有50.0、33.7及无效样本测试。
- 收益/冲突：该功能已手工移植；无需再次引入截图或重复实现。
- 验证/依赖：已有组件/工具测试可回归；本轮未执行，不能称测试通过。
- 补丁测试证据（未运行）：`frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`（it('shows output TPS in the latency cell using total duration', () => {）；`frontend/src/utils/__tests__/latencyHealth.spec.ts`（describe('formatUsageOutputRate', () => {）；`frontend/src/views/user/__tests__/UsageView.spec.ts`。

### 057 `38dd23f834b3f21c20dc26ad1d32b5fa2c7b1b0b`

原始标题：chore: remove PR screenshots

**结论：不引入。** 删除056加入的两张PR截图，无生产代码。

- 关键改动文件/函数：无生产代码改动。
- 生成/媒体文件：`assets/screenshots/usage-output-tps-desktop.png`、`assets/screenshots/usage-output-tps-mobile.png`；仅核对用途/存在状态，未逐行或逐像素评审。
- 当前仓库证据：本地assets/screenshots/usage-output-tps-desktop.png/mobile.png仍存在，056功能已覆盖；截图不影响运行。
- 收益/冲突：仅PR媒体清理，无运行收益；当前评审不授权顺手删本地文档资产。
- 验证/依赖：不需要测试，未逐像素审截图。

### 058 `1817a8428ace707df0ebf568f85c0df9c6433ebd`

原始标题：feat(ops): show per-request output TPS percentiles

**结论：需适配。** Dashboard仓库对raw/preagg/auto统一追加原始usage样本的P5/P10/P50/Avg输出速率，超时降级；DTO/API/卡片/骨架同步。

- 关键改动文件/函数：`backend/internal/repository/ops_repo_dashboard.go`（`queryOutputTPS`）；`backend/internal/service/ops_dashboard_models.go`；`frontend/src/api/admin/ops.ts`；`frontend/src/i18n/locales/en/admin/ops.ts`；`frontend/src/i18n/locales/zh/admin/ops.ts`；`frontend/src/views/admin/ops/components/OpsDashboardHeader.vue`；`frontend/src/views/admin/ops/components/OpsDashboardSkeleton.vue`。
- 当前仓库证据：当前GetDashboardOverview各模式直接return，OpsDashboardOverview只有QPS/TPS而无OutputTPS；056的单条TPS已经存在但不等于整体分布。
- 收益/冲突：可观察慢请求，不能平均小时分位数；preagg模式仍会新增raw查询，必须评估大窗口数据库成本与保留期样本偏差。
- 验证/依赖：运行新增集成样本/超时策略测试及前端卡片测试，实际解释执行计划和大窗口测量后再采纳；本轮无DB/性能验证。
- 补丁测试证据（未运行）：`backend/internal/repository/ops_repo_dashboard_integration_test.go`（func TestOpsOutputTPSDistribution(t *testing.T) {）；`backend/internal/repository/ops_repo_dashboard_timeout_test.go`（func TestOpsOutputTPSOverviewFailurePolicy(t *testing.T) {）；`frontend/src/views/admin/ops/components/__tests__/OpsDashboardHeader.spec.ts`（describe('OpsDashboardHeader output TPS', () => {）。

### 059 `18182f112765884dfbfa411b8f1c359251ede75e`

原始标题：style(ops): align output TPS tooltip indentation

**结论：不引入。** 仅修058中英文tooltip缩进。

- 关键改动文件/函数：`frontend/src/i18n/locales/en/admin/ops.ts`；`frontend/src/i18n/locales/zh/admin/ops.ts`。
- 当前仓库证据：本地尚无这两条outputTps tooltip。
- 收益/冲突：无独立效果；实施058时使用正确缩进即可。
- 验证/依赖：依赖058的前端检查，无单独测试。

### 060 `a4ea8059b7023ebf774dc1a9e78604712e0ff35f`

原始标题：fix(openai): recover rejected encrypted reasoning signatures

**结论：建议引入。** Forward对严格匹配三段说明的thinking_signature_invalid按invalid_encrypted_content同一路径只重试一次。

- 关键改动文件/函数：`backend/internal/service/openai_gateway_forward.go`。
- 当前仓库证据：当前HTTP重试条件仅upstreamCode==invalid_encrypted_content；已有ensureReqBody和签名恢复/lineage逻辑可以复用。
- 收益/冲突：恢复受上游不同错误码影响的加密reasoning；不能对普通thinking签名错误一概删除内容重试。
- 验证/依赖：新增hotpath测试验证精确匹配、无关错误不重试、一次上限、lineage；保留本地WS恢复、ctx_pool和sticky策略。
- 补丁测试证据（未运行）：`backend/internal/service/openai_gateway_service_hotpath_test.go`（func TestOpenAIGatewayService_Forward_HTTPThinkingSignatureRecoveryAndLineage(t *testing.T) {）。

### 061 `c641453123c363f286e94b35611691212e5e651c`

原始标题：fix(auth): share pending step-up verification prompts

**结论：建议引入。** useStepUp.prompt共用pendingPrompt，成功/取消同时清pending和resolver。

- 关键改动文件/函数：`frontend/src/composables/useStepUp.ts`。
- 当前仓库证据：当前每次prompt都创建Promise并覆盖唯一resolver，并发敏感操作会让早先Promise永远未完成。
- 收益/冲突：修复同一验证弹窗的并发等待，不改变step-up授权有效期。
- 验证/依赖：新增并发prompt测试：同Promise、成功全部继续、取消全部失败、下一次独立提示；检查调用方仍按boolean处理。
- 补丁测试证据（未运行）：`frontend/src/composables/__tests__/useStepUp.spec.ts`（describe('useStepUp concurrent prompts', () => {）。

### 062 `21d90fb5e52cf8f4689973dfe569953f525442e2`

原始标题：fix(auth): keep cleared compliance state isolated from pending requests

**结论：建议引入。** adminCompliance store用stateVersion隔离reset之前的fetch/accept，旧finally也不能改变新loading。

- 关键改动文件/函数：`frontend/src/stores/adminCompliance.ts`。
- 当前仓库证据：当前App.vue退出/身份变化调用adminComplianceStore.reset，但在途fetchStatus/accept仍直接写status/forceVisible。
- 收益/冲突：防止退出后恢复旧管理员合规状态，跨会话数据隔离收益明确。
- 验证/依赖：新增reset测试含成功/拒绝、旧accept、正常fetch/accept，验证新会话loading不被旧请求覆盖。
- 补丁测试证据（未运行）：`frontend/src/stores/__tests__/adminCompliance.reset.spec.ts`（it('does not repopulate cleared state when an old request succeeds', async () => {）。

### 063 `75420462a605f1f1ec834266b89dc8a4f2b64499`

原始标题：fix(plugins): ignore obsolete configuration UI sessions

**结论：建议引入。** PluginsView配置UI session版本化，打开/关闭递增，卸载复用closeConfiguration清理桥请求。

- 关键改动文件/函数：`frontend/src/views/admin/PluginsView.vue`。
- 当前仓库证据：当前openConfiguration直接uiSession.value=await createUISession，closeConfiguration清状态却不使在途请求失效。
- 收益/冲突：避免旧插件session出现在新插件配置中；原postMessage origin/session校验和pending清理不能削弱。
- 验证/依赖：延迟会话响应/旧失败/关闭重开及卸载测试，验证桥消息仍只属于当前session。
- 补丁测试证据（未运行）：`frontend/src/views/admin/__tests__/PluginsView.spec.ts`（it('ignores an old session error while the reopened configuration is loading', async () => {）。

### 064 `e0019da463226defa0c1efe0ca788a9e38f21044`

原始标题：fix(ui): limit Escape to the most recently opened dialog

**结论：已覆盖。** BaseDialog.handleEscape只允许openDialogs栈顶响应。

- 关键改动文件/函数：`frontend/src/components/common/BaseDialog.vue`。
- 当前仓库证据：当前BaseDialog.vue的条件已有[...openDialogs].pop()===dialogId；openDialogs登记/删除仍由show生命周期维护。
- 收益/冲突：已存在同样修复，重复移植无收益；最上层禁止Escape时父层也不能关闭。
- 验证/依赖：现有BaseDialog.spec/scrollLock/ids测试未见上游新增Escape专项文件；实现已覆盖，可引入Escape专项测试验证栈重开和顶层禁止Escape，本轮不运行。
- 补丁测试证据（未运行）：`frontend/src/components/common/__tests__/BaseDialog.escape.spec.ts`（describe('stacked dialog Escape handling', () => {）。

### 065 `00a922c429db9c822b55aabc8f38d321a451cafd`

原始标题：fix(backup): prevent polling from restarting after navigation

**结论：建议引入。** BackupView卸载disposed，startPolling/startRestorePolling拒绝卸载后的迟到重启。

- 关键改动文件/函数：`frontend/src/views/admin/BackupView.vue`。
- 当前仓库证据：当前onBeforeUnmount只stop两个timer，onMounted/创建/恢复请求返回后仍可启动新timer。
- 收益/冲突：避免已离页后台持续轮询与提示；不干扰实际后台备份/恢复任务。
- 验证/依赖：用fake timers+延迟API测试挂载/创建/恢复晚到，不调用真实备份服务或恢复数据。
- 补丁测试证据（未运行）：`frontend/src/views/admin/__tests__/BackupView.spec.ts`。

### 066 `8a495b0a54f5202986f56e793ff741dc3f013c12`

原始标题：fix(usage): ignore obsolete cleanup task list responses

**结论：建议引入。** UsageCleanupDialog.loadTasks以taskRequestId接收最新页，关闭/卸载失效，保护catch/finally。

- 关键改动文件/函数：`frontend/src/components/admin/usage/UsageCleanupDialog.vue`。
- 当前仓库证据：当前loadTasks读分页后直接赋tasks/total，poll/翻页可乱序；handleClose仅停timer，无法使在途结果失效。
- 收益/冲突：清理任务属于数据删除界面，避免错误任务列表与loading影响管理员判断；不改任务执行策略。
- 验证/依赖：新增分页和关闭重开测试，mock任务API，不执行真实清理；验证当前轮询继续。
- 补丁测试证据（未运行）：`frontend/src/components/admin/usage/__tests__/UsageCleanupDialog.spec.ts`（describe('cleanup task pagination', () => {）。

### 067 `8bc375b59266946582cffb819e0b1b8f803be460`

原始标题：fix(batch-image): allow access lookup retries after request failures

**结论：建议引入。** useBatchImageAccess在请求失败时loaded=false，允许后续消费者重试；成功无权限仍缓存。

- 关键改动文件/函数：`frontend/src/composables/useBatchImageAccess.ts`。
- 当前仓库证据：当前catch把hasAllowedBatchImageKey=false且loaded=true，AppSidebar/Dashboard/QuickActions后续普通refresh被缓存短路。
- 收益/冲突：一次临时网络失败不应永久隐藏批量图片入口；无需新重试循环或自动后台任务。
- 验证/依赖：新增retry测试分别验证失败后第二消费者重试、成功空列表不反复取数和pending合并。
- 补丁测试证据（未运行）：`frontend/src/composables/__tests__/useBatchImageAccess.retry.spec.ts`（describe('batch image access lookup', () => {）。

### 068 `cee76030be9ff629605901007481adc80a06242a`

原始标题：fix(announcements): report failed read confirmations accurately

**结论：建议引入。** announcement store.markAsRead返回boolean；AnnouncementBell只在成功后toast并关详情。

- 关键改动文件/函数：`frontend/src/components/common/AnnouncementBell.vue`；`frontend/src/stores/announcements.ts`。
- 当前仓库证据：当前store吞API失败并无返回，Bell无条件成功提示和closeDetail。
- 收益/冲突：避免已读失败被显示成功，用户可保留详情后重试。
- 验证/依赖：新增Bell测试失败保持打开、成功才提示/关闭；检查其他markAsRead调用方无需返回值也保持可用。
- 补丁测试证据（未运行）：`frontend/src/components/common/__tests__/AnnouncementBell.spec.ts`（describe('announcement read confirmation', () => {）。

### 069 `7d0bf9c87fdf1cf0572895f6e7aac098c4f094ed`

原始标题：fix(usage): preserve zero user multipliers in cost details

**结论：建议引入。** UsageTable用户倍率展示从||1改??1。

- 关键改动文件/函数：`frontend/src/components/admin/usage/UsageTable.vue`。
- 当前仓库证据：当前tooltipData?.rate_multiplier||1把合法零倍率显示成1x；只是展示，账单值本身未被此处改动。
- 收益/冲突：正确显示免费/零倍率请求，避免费用解释错误。
- 验证/依赖：在UsageTable测试中覆盖0、null/undefined和正倍率，渲染文本与真实金额分离。
- 补丁测试证据（未运行）：`frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts`。

### 070 `636faa695ba3bf205909b40d2a9498ecd947675c`

原始标题：fix(profile): ignore obsolete notification email code responses

**结论：建议引入。** ProfileBalanceNotifyCard卸载失效；发送新地址验证码后还检查pe仍在pendingEmails，已保存邮箱发送后也查disposed。

- 关键改动文件/函数：`frontend/src/components/user/profile/ProfileBalanceNotifyCard.vue`。
- 当前仓库证据：当前sendCodeFor/ForSaved await返回后无生命周期检查，可给已删条目/已卸载组件创建60秒timer。
- 收益/冲突：防止幽灵倒计时；保留正常验证码发送及服务端限制。
- 验证/依赖：fake timers测试删除地址、切页/卸载和正常成功，断言无新timer；不实际发邮件。
- 补丁测试证据（未运行）：`frontend/src/components/user/profile/__tests__/ProfileBalanceNotifyCard.spec.ts`（it('does not start a saved email timer after unmount', async () => {）。

### 071 `e180a939ac7eb0210d941a52f4bef12140c9e943`

原始标题：fix(users): show normalized errors when bulk updating limits

**结论：建议引入。** BulkEditUserModal.catch复用extractApiErrorMessage替代只读response.data的拼接。

- 关键改动文件/函数：`frontend/src/components/admin/user/BulkEditUserModal.vue`。
- 当前仓库证据：当前catch仅message/detail字段；本地utils/apiError已提供统一规范化错误，API层可能已返回标准错误对象。
- 收益/冲突：正确显示批量限额失败原因，复用已有工具无需新抽象。
- 验证/依赖：使用现有BulkEditUserModal测试覆盖已归一错误、Axios形状和fallback；不做真实批量限额更新。
- 补丁测试证据（未运行）：`frontend/src/components/admin/user/__tests__/BulkEditUserModal.spec.ts`。

### 072 `5ace0699b2e8550c10bcfb5b07342d9af4be7380`

原始标题：fix(pricing): preserve exponents when trimming scaled prices

**结论：建议引入。** formatScaled用Number(toPrecision(10)).toString移除小数尾零，避免正则删指数尾零。

- 关键改动文件/函数：`frontend/src/utils/pricing.ts`。
- 当前仓库证据：当前replace(/\.?0+$/, '')会把科学记数法指数末尾0删掉；PricingRow、SupportedModelChip、PlazaModelPricingTable直接调用此公共工具。
- 收益/冲突：修复价格显示数量级错误，不改真实计费；格式保留10有效位和minFractionDigits补位。
- 验证/依赖：新增pricing.formatScaled测试含1e-10/1e+20、普通整数、小数、零及最小小数位；不引入通用格式层。
- 补丁测试证据（未运行）：`frontend/src/utils/__tests__/pricing.formatScaled.spec.ts`（describe('formatScaled', () => {）。

### 073 `2e2b8f6d59621d66459841b89d104a4c0e912c53`

原始标题：fix(auth): avoid starting OAuth code cooldowns after unmount

**结论：建议引入。** PendingOAuthCreateAccountForm卸载置disposed，sendEmailCode返回后不再显示成功或startCountdown。

- 关键改动文件/函数：`frontend/src/components/auth/PendingOAuthCreateAccountForm.vue`。
- 当前仓库证据：当前onUnmounted只clearCountdown，在途handleSendCode可稍后重新建timer。
- 收益/冲突：切换登录/注册流程后不留验证码计时器，授权创建请求逻辑不变。
- 验证/依赖：组件fake timer测试发送中卸载、正常发送、失败和再次进入；不实际发验证码。
- 补丁测试证据（未运行）：`frontend/src/components/auth/__tests__/PendingOAuthCreateAccountForm.spec.ts`（it('does not restart the code cooldown after switching away during a send', async () => {）。

### 074 `8167d85b32480de46fd140dc91b1c7d35742c6a1`

原始标题：fix(ui): close selectors when they become disabled

**结论：建议引入。** Select/ProxySelector监听disabled变化并关闭已打开下拉；Proxy同时清搜索。

- 关键改动文件/函数：`frontend/src/components/common/ProxySelector.vue`；`frontend/src/components/common/Select.vue`。
- 当前仓库证据：当前toggle禁止disabled时新打开，但已打开后disabled变化没有收起watch；调用账号表单保存中会改disabled。
- 收益/冲突：已禁用控件不能继续从残留菜单交互；复用现有组件不改全部调用方。
- 验证/依赖：新增disabledSelectors测试关闭、重新启用可打开、代理搜索清空和不额外emit选择。
- 补丁测试证据（未运行）：`frontend/src/components/common/__tests__/disabledSelectors.spec.ts`（describe('selectors disabled while open', () => {）。

### 075 `3a1a37b7b9e12aaf5a7547966394750b85629097`

原始标题：fix(ops): retain byte units for sub-byte throughput

**结论：建议引入。** formatBytes指数Math.max(0, floor(log))，小于1字节仍使用Bytes。

- 关键改动文件/函数：`frontend/src/utils/format.ts`。
- 当前仓库证据：当前小数吞吐的指数为负导致sizes[-1]未定义；opsFormatters把bytes/seconds直接传该工具。
- 收益/冲突：修复长期平均吞吐0<x<1时undefined单位；无需扩大到负数等未请求输入防御。
- 验证/依赖：新增formatBytes测试0、0.1/0.5、1、KB边界及ops速率显示。
- 补丁测试证据（未运行）：`frontend/src/utils/__tests__/formatBytes.spec.ts`（describe('fractional byte rates', () => {）。

### 076 `7cd3dc85c3f8826064ec29766f2e20f100741e40`

原始标题：fix(channel-monitor): require request samples before health scoring

**结论：建议引入。** ChannelMonitorV2HealthForWithThresholds在总RequestCount不足MinimumSample时整体返回unknown。

- 关键改动文件/函数：`backend/internal/service/channel_monitor_v2.go`。
- 当前仓库证据：当前只给ErrorRate维度检查请求数，TTFT.SampleCount/CacheRateDenominator独立达到阈值仍可生成overall分数，即总请求样本不足。
- 收益/冲突：避免缓存token分母大但请求很少时宣称健康；阈值归一、权重和足量样本行为保持。
- 验证/依赖：新增请求数边界、巨大缓存分母、小TTFT样本、足量高错误critical测试；核对UI unknown状态。
- 补丁测试证据（未运行）：`backend/internal/service/channel_monitor_v2_test.go`（func TestChannelMonitorV2HealthRequiresRequestSamples(t *testing.T) {）。

### 077 `c063a4016eeebdc21d3ebd07e792194576119433`

原始标题：refactor(provider): 以 provider profile 收敛多协议供应商默认端点表

**结论：需适配。** ProviderProfile集中默认模式、端点、NativeResponses及OpenCode默认规则，Account多个baseURL函数改查表。

- 关键改动文件/函数：`backend/internal/service/account.go`；`backend/internal/service/domain_constants.go`；`backend/internal/service/opencode_go.go`；`backend/internal/service/provider_profile.go`（`Endpoints`, `DefaultBaseURL`, `LookupProviderProfile`, `providerProfile`, `defaultProviderBaseURL`）。
- 当前仓库证据：当前account.go仍按平台switch，opencode_go.go维护openCodeDefaultChat/AnthropicBaseURL；provider_profile.go不存在。
- 收益/冲突：可减少新增供应商漏配，但纯结构调整回归面较大；不要先移短命ChatBaseURL版本再补078，直接评估最终形态。
- 验证/依赖：与078/079/083成组；用现有CN/OpenCode测试逐模式逐协议比较自定义base/api_base_urls优先级、空/未知模式、DeepSeek路径。
- 补丁测试证据（未运行）：`backend/internal/service/provider_profile_test.go`（func TestProviderProfile_DefaultBaseURLsMatchLegacySwitches(t *testing.T) {）。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 078 `3f8eea08079a244c9def66d0998fd4f79e9e1d25`

原始标题：refactor(provider): profile 端点改为按协议登记，Responses 路径约定并入 profile

**结论：需适配。** ProviderEndpoints改BaseURLs按协议表，Responses能力从键派生；ResponsesPath在profile覆盖DeepSeek的/responses。

- 关键改动文件/函数：`backend/internal/service/account.go`；`backend/internal/service/openai_gateway_request_body.go`；`backend/internal/service/provider_profile.go`（`SupportsProtocol`, `providerSupportsProtocol`）。
- 当前仓库证据：当前SupportsNativeCNResponses平台switch且buildOpenAIResponsesURLForPlatform直接判断DeepSeek；该功能已有，只是来源不同。
- 收益/冲突：不是新增DeepSeek能力；若采纳077应采用此较新结构，保留自定义base的版本路径拼接规则。
- 验证/依赖：依赖077最终形态；验证所有默认端点、Zhipu无responses、DeepSeek带/不带版本路径、自定义/代理地址。
- 补丁测试证据（未运行）：`backend/internal/service/cn_providers_test.go`；`backend/internal/service/provider_profile_test.go`（func TestProviderProfile_BaseURLKeysAreNativeProtocols(t *testing.T) {）。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 079 `8ac2936a3d8cc9c61b02e3f92f10de343afbed78`

原始标题：refactor(gateway): 三个入站入口的上游协议分流收敛为 resolveUpstreamProtocol

**结论：需适配。** 抽resolveUpstreamProtocol统一三入站分流，ProviderRouting区分by_inbound/by_model，OpenCode规则通用化并抽Responses形状转换。

- 关键改动文件/函数：`backend/internal/service/account.go`；`backend/internal/service/admin_account.go`；`backend/internal/service/openai_gateway_chat_completions.go`；`backend/internal/service/openai_gateway_forward.go`；`backend/internal/service/openai_gateway_messages.go`；`backend/internal/service/openai_gateway_messages_anthropic_native.go`（`resolveMappedUpstreamModel`）；`backend/internal/service/opencode_go.go`（`DefaultOpenCodeGoProtocolRules`, `DefaultOpenCodeZenProtocolRules`, `isNativeUpstreamProtocol`, `protocolRulePatternMatches`, `matchProtocolRules`, `configuredProtocolRules`, `parseProtocolRules`, `protocolRuleItems`等）；`backend/internal/service/provider_profile.go`（`routesByModel`）；`backend/internal/service/upstream_protocol_routing.go`（`resolveUpstreamProtocol`, `resolveModelRoutedProtocol`, `upstreamRoutingModel`, `convertResponsesShapedChatBody`）。
- 当前仓库证据：本地Forward/forwardAsChatCompletions/ForwardAsAnthropic各自有OpenCode和adaptive分支；upstream_protocol_routing.go不存在；额外BPS转发/模型归一必须保留。
- 收益/冲突：减少重复判断但触及入口顺序；不可把Grok专用路径、Responses-shaped Chat和本地并行工具归一移错先后。
- 验证/依赖：依赖077/078；三入站×显式协议/adaptive×CN/OpenCode×模型映射/规则/shape矩阵，与080一起避免短暂回归。
- 补丁测试证据（未运行）：`backend/internal/service/opencode_go_test.go`；`backend/internal/service/provider_profile_test.go`；`backend/internal/service/upstream_protocol_routing_matrix_test.go`（func TestUpstreamProtocolRoutingMatrixMatchesLegacy(t *testing.T) {）；`backend/internal/service/upstream_protocol_routing_test.go`（func TestResolveUpstreamProtocol(t *testing.T) {）。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 080 `105c955c8856a0ac842094d0cb1b030fd33ad37e`

原始标题：fix(gateway): /v1/responses 转 Anthropic 时统一以去空白的请求模型兜底映射

**结论：需适配。** 重构后的Responses→Anthropic统一传trim后的reqModel，不再对by_model传空fallback。

- 关键改动文件/函数：`backend/internal/service/openai_gateway_forward.go`。
- 当前仓库证据：本地未采用079：CN路径已传reqModel，OpenCode路径仍传空；resolveMappedUpstreamModel改名也不存在。
- 收益/冲突：局部场景有收益，但不是无冲突一行修复；需验证fallback是否抢过账号model_mapping及计费模型归一，不能盲改所有空串。
- 验证/依赖：依赖079或手动把trim/fallback意图落在当前OpenCode分支；测试前后空白、未命中映射和命中映射的上游/计费名一致。
- 补丁测试证据（未运行）：`backend/internal/service/upstream_protocol_routing_test.go`（func TestResponsesToNativeAnthropicTrimsBillingModel(t *testing.T) {）。

### 081 `3b89cc08dd732b208b27351f77796912855dbb55`

原始标题：refactor(platform): 后端平台列表改由平台清单派生

**结论：需适配。** domain/platforms.go集中平台清单；binding、路由、scheduler、quota、pricing/model等白名单派生；validator从间接改直接依赖。

- 关键改动文件/函数：`backend/go.mod`；`backend/internal/domain/platforms.go`（`Platforms`, `LookupPlatform`, `IsConcretePlatform`, `IsGroupPlatform`, `ConcretePlatformIDs`, `PlatformIDsWhere`, `CompositePrecedencePlatformIDs`, `UsesOpenAIGateway`等）；`backend/internal/handler/admin/channel_handler.go`（`platformLiteLLMProvider`）；`backend/internal/handler/admin/group_handler.go`；`backend/internal/handler/admin/platform_validation.go`（`init`）；`backend/internal/handler/gateway_handler.go`；`backend/internal/handler/openai_gateway_handler.go`；`backend/internal/model/error_passthrough_rule.go`；`backend/internal/server/routes/gateway.go`；`backend/internal/service/account.go`；其余为配套调用点/类型/展示修改。
- 当前仓库证据：本地清单分散在domain_constants、路由、group请求oneof、schedulerSnapshotPlatforms；本地额外PlatformOpenAIBPS已在域常量/配额清单/独立调度中使用，上游platformList没有它。
- 收益/冲突：直接移植会让BPS从允许平台/调度/定价消失，属于实质回归；保留BPS专用网关语义，不可简单登记为普通OpenAI。
- 验证/依赖：先逐项列出BPS和TypeSafe能力，再验证platform predicates、binding、composite优先顺序、快照重建及定价；不为形式统一强行扩大范围。
- 补丁测试证据（未运行）：`backend/internal/domain/platforms_test.go`（func TestPlatformListMatchesLegacyLists(t *testing.T) {）；`backend/internal/service/platform_list_test.go`（func TestPlatformListDerivedListsMatchLegacy(t *testing.T) {）。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 082 `eefa7dd9dc90e5bddb40a3f0dc2ea8df0dcf1476`

原始标题：refactor(platform): 平台白名单由数据库约束改为应用层校验

**结论：需适配。** 删除quota/composite数据库CHECK，改domain清单+ent validator+原生SQL validateQuotaPlatforms，auth快照仍脱事务。

- 关键改动文件/函数：`backend/ent/schema/composite_model_route.go`；`backend/ent/schema/user_platform_quota.go`；`backend/internal/domain/constants.go`；`backend/internal/repository/user_platform_quota_repo.go`（`validateQuotaPlatforms`）；`backend/internal/service/auth_service.go`；`backend/migrations/242_drop_platform_check_constraints.sql`。
- 生成/媒体文件：`backend/ent/runtime/runtime.go`；仅核对用途/存在状态，未逐行或逐像素评审。
- 当前仓库证据：本地ent枚举和SQL CHECK含openai_bps/typesafe；245_openai_bps_platform.sql和247_add_typesafe_platform.sql会在上游新增242迁移之后重建这些CHECK。
- 收益/冲突：直接引入242不会达到最终去CHECK效果；已部署库需新的迁移编号。数据一致性防护不能仅因方便新增平台就删除，必须先证明所有写入口等价受保护。
- 验证/依赖：依赖081且保留BPS；审核原生SQL/ent所有写入、升级顺序、未知平台拒绝和已有行；在隔离DB跑新迁移集成测试，生成runtime不逐行人工审。
- 补丁测试证据（未运行）：`backend/internal/repository/platform_list_db_integration_test.go`（func TestMigration242DropsPlatformCheckConstraints(t *testing.T) {）；`backend/migrations/platform_check_constraints_drop_migration_test.go`（func TestDropPlatformCheckConstraintsMigration(t *testing.T) {）。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 083 `c556aee8fd37bbe41f8dccc18eca2df594baa5bf`

原始标题：refactor(platform): 多协议供应商的转发与探测判断改查 profile

**结论：需适配。** 通过RoutesProtocolByInbound/IsMultiProtocolAPIKey替代CN/OpenCode枚举，覆盖auth scheme、Responses probe/count、billing候选、403、billing probe和models。

- 关键改动文件/函数：`backend/internal/handler/admin/account_handler.go`；`backend/internal/service/anthropic_apikey_auth.go`；`backend/internal/service/openai_apikey_responses_probe.go`；`backend/internal/service/openai_gateway_count_tokens.go`；`backend/internal/service/openai_gateway_forward.go`；`backend/internal/service/openai_gateway_usage.go`；`backend/internal/service/provider_profile.go`（`RoutesProtocolByInbound`）；`backend/internal/service/ratelimit_service.go`；`backend/internal/service/upstream_billing_probe.go`；`backend/internal/service/upstream_models.go`。
- 当前仓库证据：当前上述函数仍IsCNProvider或IsOpenCodeGo分支，provider_profile.go不存在；本地额外OpenAI BPS不属于这些供应商默认规则。
- 收益/冲突：为新聚合供应商继承行为做准备，现有供应商多数语义保持；必须保留by_model不被异步probe降成CC、本地计数估算和403冷却边界。
- 验证/依赖：依赖077–079，逐函数现有CN/OpenCode回归和新profile模拟；验证API Key auth头、计数不触网及403不会误永久停号。
- 补丁测试证据（未运行）：`backend/internal/service/platform_list_test.go`（func TestProviderProfileAccountPredicatesMatchLegacy(t *testing.T) {）。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 084 `7ffa38a8ae42a4a9f1796ab0fdeecab82e1a6bcc`

原始标题：feat(frontend): 平台列表与多协议账号表单改由平台清单驱动

**结论：需适配。** BuildPlatformCatalog导出后端平台/profile为前端builtin JSON；platformCatalog.ts、表单credentialsBuilder、平台选项/配额/颜色/渠道等改为目录驱动。

- 关键改动文件/函数：`backend/internal/service/platform_catalog.go`（`BuildPlatformCatalog`, `providerProfileInfo`）；`frontend/src/components/account/CreateAccountModal.vue`（`accountMode`, `isMultiProtocolPlatform`, `isGenericMultiProtocolPlatform`, `extraMultiProtocolPlatforms`, `genericAccountModes`, `routesByModel`, `providerModeLabel`, `currentOpenCodeOrCNMode`等）；`frontend/src/components/account/EditAccountModal.vue`（`isGenericMultiProtocolAccount`, `genericAccountModes`, `adaptivePresetPlatform`, `editAccountMode`, `cnPresetMode`, `currentOpenCodeOrCNMode`, `NATIVE_PROTOCOL_LABEL_KEYS`, `editAdaptiveProtocolOptions`等）；`frontend/src/components/account/credentialsBuilder.ts`（`NATIVE_API_PROTOCOLS`, `isMultiProtocolApiKeyPlatform`, `providerSupportsProtocol`, `cnSupportsNativeResponses`, `providerNativeProtocols`, `providerRoutesByModel`, `providerAccountModes`, `resolveProviderAccountMode`等）；`frontend/src/constants/platformCatalog.ts`（`BUILTIN_PLATFORM_CATALOG`, `catalogState`, `setPlatformCatalog`, `resetPlatformCatalog`, `listPlatforms`, `listPlatformIds`, `getPlatformSpec`, `isKnownPlatform`等）；`frontend/src/constants/platforms.ts`（`CONCRETE_PLATFORM_OPTIONS`, `GROUP_PLATFORM_OPTIONS`）；`Makefile`；`frontend/src/api/admin/settings.ts`；`frontend/src/api/admin/users.ts`（`platformQuotaPlatforms`）；`frontend/src/components/account/ModelWhitelistSelector.vue`（`canSyncUpstreamPlatform`）；其余为配套调用点/类型/展示修改。
- 生成/媒体文件：`frontend/src/constants/platformCatalog.builtin.json`；仅核对用途/存在状态，未逐行或逐像素评审。
- 当前仓库证据：本地没有platform_catalog.go或platformCatalog.*，仍手写平台/端点；Create/Edit/各配额组件已有BPS和TypeSafe等本地扩展，直接用上游builtin会遗漏。
- 收益/冲突：可统一前后端数据但范围大，不能只拷JSON或表单；需保留本地CLI0.164生成配置、平台特有行为、字段和文案。
- 验证/依赖：依赖077–083；运行目录同步与Create/Edit保存往返、切模式保留自定义endpoint/rules、quota完整列表、BPS/TypeSafe图标选项；builtin生成文件未逐行审。
- 补丁测试证据（未运行）：`backend/internal/service/platform_catalog_test.go`（func TestBuildPlatformCatalogMirrorsPlatformListAndProfiles(t *testing.T) {）；`frontend/src/api/__tests__/settings.authSourceDefaults.spec.ts`（it("无参数时返回平台清单中的全部平台全 null", () => {）；`frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`（describe('providers using the generic form', () => {）；`frontend/src/components/account/__tests__/EditAccountModal.spec.ts`（describe('providers using the generic form', () => {）；另有配套测试，未逐项审读。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。

### 085 `e2939cd8fa724741fb555712e3ad1d9ce9bb5cf9`

原始标题：feat(platform): 接入 Command Code 平台

**结论：不引入。** 新增Command Code平台/profile、四个alpha只读用量接口、积分/窗口快照、402/403/429细分冷却、多协议rule集合及动态/models协议目录缓存，前后端全面接入。

- 关键改动文件/函数：`backend/internal/service/command_code.go`（`DefaultCommandCodeTestModel`, `DefaultCommandCodeProtocolRules`, `IsCommandCode`）；`backend/internal/service/command_code_usage.go`（`commandCodeUsageSupported`, `UnmarshalJSON`, `Error`, `unauthorized`, `isCommandCodeAuthError`, `get`, `fetch`, `isApplicationError`等）；`backend/internal/service/ratelimit_command_code.go`（`commandCodeSpendLimitIsModelScoped`, `classifyCommandCodeUsageError`, `commandCodeResetAt`, `handleCommandCodeUsageError`, `handleCommandCodeSpendLimit`）；`backend/internal/service/model_protocol_catalog.go`（`entryLocked`, `lookup`, `store`, `modelEndpointProtocol`, `parseModelProtocolCatalog`, `modelCatalogProtocols`, `modelProtocolCatalogKey`, `modelProtocolCatalogHeaders`等）；`backend/internal/service/upstream_protocol_routing.go`（`resolveUpstreamProtocol`, `resolveModelRoutedProtocolFor`, `modelProtocolSet`, `supportedCatalogProtocols`）；`backend/internal/service/opencode_go.go`（`normalizeProtocolSet`, `matchProtocolRuleSet`, `protocolRuleProtocols`）；`backend/internal/service/provider_profile.go`；`backend/internal/service/platform_catalog.go`（`cloneProtocolRules`）；`Makefile`；`backend/internal/domain/constants.go`；其余为配套调用点/类型/展示修改。
- 生成/媒体文件：`frontend/src/constants/platformCatalog.builtin.json`；仅核对用途/存在状态，未逐行或逐像素评审。
- 当前仓库证据：本地无command_code.go/usage、ratelimit_command_code、model_protocol_catalog；CN余额/额度循环只认识既有供应商；尚无077–084所需profile/catalog结构。
- 收益/冲突：这是新平台能力而非通用缺陷修复，当前没有接入需求证据；依赖重构链且增加后台网络、缓存和冷却规则，不建议为了追上游整体搬入。
- 验证/依赖：若未来要求接入，先适配077–084并保留BPS；重点验证whoami组织隔离、订阅查询失败旧月窗保留、充值只解除特定窗口停调、模型级spend limit、官方/自定义目录缓存隔离及首载2秒等待；只用本地HTTP fixture。
- 补丁测试证据（未运行）：`backend/internal/service/command_code_test.go`（func TestCommandCodeProfileRoutesByModel(t *testing.T) {）；`backend/internal/service/command_code_usage_test.go`（func TestCommandCodeQuotaProbeParsesWindowsAndBalance(t *testing.T) {）；`backend/internal/service/model_protocol_catalog_test.go`（func TestParseModelProtocolCatalog(t *testing.T) {）；`backend/internal/service/upstream_protocol_routing_matrix_test.go`；另有配套测试，未逐项审读。
- 审查边界：本条为跨文件重构/新平台，已审关键手写实现及当前对应入口，未逐行读完全部配套文件，也未穷尽所有直接调用方。上述为移植风险判断，不能据此认定整个提交可无冲突引入；实施前仍须补完全调用链与回归审查。
