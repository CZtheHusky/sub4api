# 上游逐提交评审：086–170

基准：本地 `c67360f44`；上游冻结 `a3298b46ebe239fb0720b068a1eb3383955bdac2`；merge-base `3040209f205472038c1ba745a1bedd2edd9053b1`。共85条。只写评审文档，未移植代码。

方法与限制：每条打开实际git show补丁并对照本地相关源码；不等于全部文件逐行审计。086/113/120/132/145/155/156等大提交展开关键手写路径；外围调用方、全部locale和全部测试没有穷尽审阅。生成ent/wire、golden及lock只核对用途/结构，未逐行审；二进制赞助图片未逐像素审。相关测试仅静态选读声明和关键断言，未运行。

现有CLI0.164（eb6ac2192）、WS ctx_pool、sticky、shadow、本地搜索与计价/安全策略必须保留。补丁不等价不代表缺功能；本批只有112确认已覆盖。重点候选为109、125、148、158、160–165；平台/i18n/显式缓存/大型协议特性需要先适配依赖。

### 086 `874871a25a1441399d1cc416ded79c564897671f`

- 原始标题：feat(platform): 接入 Cline 平台
- 结论：**需适配**。
- 实际改动、收益与冲突：新增 Cline 平台、积分/Pass/免费模型钱包分类、官方主机账号探测与钱包级冷却；providerDefaultTestModel 按订阅和冷却切换测试模型。当前缺 cline.go、provider_profile.go、domain/platforms.go 与前端 platformCatalog，不能只拣平台枚举。
- 当前仓库证据：`backend/internal/domain/constants.go` 没有PlatformCline；`backend/internal/service/account.go` 的GetCodingPlanProvider无Cline分支，`cline.go`/`cline_usage.go`/`provider_profile.go`当前不存在。前端也缺platformCatalog.builtin.json，说明不是枚举单点遗漏。
- 关键改动文件：`backend/internal/service/cline.go`；`backend/internal/service/cline_usage.go`；`backend/internal/service/ratelimit_cline.go`；`backend/internal/service/provider_profile.go`；`backend/internal/domain/constants.go`；另有21个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/domain/platforms_test.go`；`backend/internal/model/error_passthrough_rule_test.go`等14个文件（静态选读；未运行）。
- 验证要求与依赖：需先评估前序多协议供应商框架；核验三个钱包互不停调、非官方主机不得向官方账号接口泄露 Key、充值恢复；合并087修正。

### 087 `926441461269cd92178ecd5e65030ead6991df34`

- 原始标题：fix(provider): 修复 Claude 计费与钱包冷却并隔离模型目录缓存
- 结论：**需适配**。
- 实际改动、收益与冲突：modelProtocolCatalogKey 改为账号+地址隔离，lookup 共享 firstLoadDeadline 并响应 ctx 取消；Claude计费判断从所有多协议账号收窄为 CN/OpenCode；Cline错误只解析 error.code/message 而非回显 metadata，400不再触发钱包冷却。当前目录与 Cline 框架均不存在。
- 当前仓库证据：`backend/internal/service/model_protocol_catalog.go` 与 `ratelimit_cline.go` 不存在；本地OpenAI网关未采用该resolveUpstreamProtocolFor目录分派。计费修正需对照本地openai_gateway_usage.go中实际平台计价路径独立处理。
- 关键改动文件：`backend/internal/service/model_protocol_catalog.go`；`backend/internal/service/ratelimit_cline.go`；`backend/internal/service/openai_gateway_usage.go`；`backend/internal/handler/admin/platform_validation.go`；`backend/internal/service/account_test_service.go`；另有4个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/handler/admin/group_handler_platform_test.go`；`backend/internal/service/cline_test.go`等6个文件（静态选读；未运行）。
- 验证要求与依赖：按独立修复拆分；先引入对应框架再测双账号鉴权隔离、冷启动并发截止、Claude价格不被误套，以及Cline回显内容不能冷却钱包。

### 088 `7ca08d521f8e635c01b4d998546679042b8afc49`

- 原始标题：fix(frontend): 按平台能力展示模型同步入口并同步 Zen 规则
- 结论：**需适配**。
- 实际改动、收益与冲突：ModelWhitelistSelector 与 ChannelsView 以 supportsUpstreamModelSync/supportsPricingModelSync 控制入口；Zen qwen3.8-max 强制 chat_completions。当前缺平台能力目录，不能直接引用新增函数。
- 当前仓库证据：`frontend/src/constants/platformCatalog.ts` 不存在；当前ModelWhitelistSelector与ChannelsView无法直接调用supportsUpstreamModelSync/supportsPricingModelSync，必须接本地平台定义。
- 关键改动文件：`frontend/src/components/account/ModelWhitelistSelector.vue`；`frontend/src/constants/platformCatalog.builtin.json`；`frontend/src/constants/platformCatalog.ts`；`frontend/src/views/admin/ChannelsView.vue`。
- 相关测试：`frontend/src/components/account/__tests__/CreateAccountModal.spec.ts`；`frontend/src/components/account/__tests__/ModelWhitelistSelector.spec.ts`等5个文件（静态选读；未运行）。
- 验证要求与依赖：依本地平台清单实现最小能力判定；测不支持同步的平台不发请求、渠道同步期间禁重复点击、Zen 路由；不为此移植整个目录框架。

### 089 `387ee20ed11bc221198714d15fcedcf8c134e9d0`

- 原始标题：test(platform): 补录制路由基准与平台约束迁移防护
- 结论：**需适配**。
- 实际改动、收益与冲突：新增路由录制脚本、固定上游版本 golden 与平台约束迁移测试；脚本通过 git archive 固定 revision，再运行矩阵驱动，不能当本地业务真值。
- 当前仓库证据：本地不存在 `backend/internal/service/testdata/record_upstream_protocol_routing_main.py` 与所需golden；它们基于固定上游revision生成，不能证明本地定制路由等价。
- 关键改动文件：`backend/internal/service/testdata/record_upstream_protocol_routing_main.py`。
- 相关测试：`backend/internal/service/opencode_unsupported_models_routing_test.go`；`backend/internal/service/upstream_protocol_routing_matrix_test.go`等3个文件（静态选读；未运行）。
- 验证要求与依赖：前序平台框架未落地；可借鉴测试意图，但必须把本地多协议、WS/sticky行为纳入预期，不能照录上游冻结contract。未逐行审3931行生成golden。

### 090 `c61f6ebaa8083f95d7caf9415a359a20715c134d`

- 原始标题：fix(security): 升级 Go 1.27.2 与 x/net 修复安全扫描
- 结论：**建议引入**。
- 实际改动、收益与冲突：Go工具链/Docker/CI统一1.27.2，x/net0.60、x/crypto0.57及相关x库升级。当前 go.mod 仍 go1.27.0，Docker/CI旧版本仍需联动。
- 当前仓库证据：`backend/go.mod:3` 为go1.27.0；Dockerfile与CI补丁仍以1.27.0为旧值，需工具链、镜像与x库统一升级。
- 关键改动文件：`.github/workflows/backend-ci.yml`；`.github/workflows/release.yml`；`.github/workflows/security-scan.yml`；`Dockerfile`；`backend/Dockerfile`；另有2个手写/配置文件，范围未在此穷举。
- 验证要求与依赖：与091/092成套验证编译、ent/schema导入、HTTP2 keepalive/GOAWAY；本次只核对提交内容，未联网确认漏洞公告、镜像或版本可用性，正式采用前重查。

### 091 `5f93bb3b3fb30360560d2a3b9e688ba52c5b9471`

- 原始标题：fix(ci): 升级 lint 支持 Go 1.27.2 并保留 HTTP2 兼容检查
- 结论：**需适配**。
- 实际改动、收益与冲突：CI golangci-lint升2.14.0；只对http2 ConfigureServer/ConfigureTransports与GoAwayError的弃用报警作窄范围排除，保留现有 HTTP2 兼容实现。
- 当前仓库证据：`.github/workflows/backend-ci.yml` 与 `backend/.golangci.yml` 尚无提交新增的v2.14.0和HTTP2窄范围SA1019配置；本地HTTP2实现必须维持现有兼容处理。
- 关键改动文件：`.github/workflows/backend-ci.yml`；`backend/.golangci.yml`。
- 验证要求与依赖：依090；排除模式必须改成本地实际文件/行并保留 idle timeout 与 PING 的行为测试，不能全局屏蔽staticcheck。

### 092 `d85cebecc4a1d2e5e39f1e80f9876b66a5ce0e31`

- 原始标题：fix(ci): 升级 schema 导入器支持 Go 1.27.2 导出格式
- 结论：**需适配**。
- 实际改动、收益与冲突：golang.org/x/tools 从0.49升0.50以兼容新Go导出数据格式；只有go.mod/go.sum变化。
- 当前仓库证据：`backend/go.mod` 当前x/tools没有本次v0.50.0升级；该导入器修复只在采用新Go导出格式后有对应意义。
- 关键改动文件：`backend/go.mod`。
- 验证要求与依赖：依090的工具链升级；验证ent/schema代码生成及导入，不单独回滚本地较新的相关依赖。

### 093 `dab3b87ea2b9770f29b52c55e548d84475b41f71`

- 原始标题：fix(openai): declare web_search when OAuth history replays web_search_call
- 结论：**需适配**。
- 实际改动、收益与冲突：OAuth请求历史含web_search_call且未声明工具时补cached-only web_search；无调用方工具时固定tool_choice=none；map与原始JSON两条入口同时接入。当前没有openai_oauth_web_search_history.go。
- 当前仓库证据：`backend/internal/service/openai_oauth_web_search_history.go` 不存在；本地Codex transform与request-body兼容层没有调用ensureOpenAIOAuthWebSearchToolForHistory，不能认定已有历史声明补全。
- 关键改动文件：`backend/internal/service/openai_codex_transform.go`；`backend/internal/service/openai_gateway_request_body.go`；`backend/internal/service/openai_oauth_web_search_history.go`。
- 相关测试：`backend/internal/service/openai_oauth_web_search_history_forward_test.go`；`backend/internal/service/openai_oauth_web_search_history_test.go`（静态选读；未运行）。
- 验证要求与依赖：必须连094的Lite additional_tools修复；核对本地CLI0.164的history、WS ctx_pool、compact及自有搜索桥，不能把OAuth声明处理套给API Key。

### 094 `64caa9af8b31a2da615564511be4cd08ce88aa1b`

- 原始标题：fix(openai): declare web_search via additional_tools for Responses Lite
- 结论：**需适配**。
- 实际改动、收益与冲突：Lite把补充声明放input.additional_tools并插在末尾compaction_trigger之前，复用已存在声明；compact不重写。Forward/passthrough/WS兼容选项联动。
- 当前仓库证据：本地缺openAIAdditionalToolsInsertIndex及该history helper；`openai_gateway_request_body.go` 的兼容层需接入本地Lite/compact/WS入口，不能照搬上游options结构。
- 关键改动文件：`backend/internal/service/openai_codex_transform.go`；`backend/internal/service/openai_gateway_forward.go`；`backend/internal/service/openai_gateway_passthrough.go`；`backend/internal/service/openai_gateway_request_body.go`；`backend/internal/service/openai_oauth_web_search_history.go`。
- 相关测试：`backend/internal/service/openai_oauth_web_search_history_forward_test.go`；`backend/internal/service/openai_oauth_web_search_history_test.go`（静态选读；未运行）。
- 验证要求与依赖：依093，合并095；覆盖显式tool_choice、已有additional_tools、尾部compaction_trigger、compact、WS多轮；保持本地0.164 wire shape和ctx_pool。

### 095 `85e31109535f65f99201456b59a0490104bc0a87`

- 原始标题：fix(lint): check type assertions in web_search history helper and test
- 结论：**不引入**。
- 实际改动、收益与冲突：仅把additionalToolsIndex已确定为map的断言改为逗号ok并修测试断言以满足lint；本地没有该helper。
- 当前仓库证据：本地无被修改的 `openai_oauth_web_search_history.go`；这是未引入093/094代码中的lint跟进，不是当前独立缺陷。
- 关键改动文件：`backend/internal/service/openai_oauth_web_search_history.go`。
- 相关测试：`backend/internal/service/openai_oauth_web_search_history_test.go`（静态选读；未运行）。
- 验证要求与依赖：若引入093/094则随实现处理lint；不能独立当功能补丁，不新增无业务依据的运行时防御。

### 096 `3a6fd1c9db07203ca308aaba69e502bc1f35b307`

- 原始标题：chore: sync VERSION to 0.2.15 [skip ci]
- 结论：**不引入**。
- 实际改动、收益与冲突：仅把backend/cmd/server/VERSION从0.2.14改0.2.15，与本地发布版本线无功能联系。
- 当前仓库证据：只对照 `backend/cmd/server/VERSION` 的发布标识；0.2.15不是本地功能覆盖证据。
- 关键改动文件：`backend/cmd/server/VERSION`。
- 验证要求与依赖：保留本地版本，不借上游版本号声明本地已具备相关功能。

### 097 `29b2d721f46bddf5dc2555aad4adc063e0181d3b`

- 原始标题：test(web): use shipped SVG asset in embedded frontend tests
- 结论：**建议引入**。
- 实际改动、收益与冲突：embedded frontend两个测试将/logo.png与image/png改为实际随前端交付的/logo.svg与image/svg+xml。
- 当前仓库证据：`frontend/public/logo.svg` 存在；`backend/internal/web/embed_test.go:653` 与`:738`仍请求/logo.png并断言PNG，两处与实际资产不符。
- 改动范围：仅测试/夹具。
- 相关测试：`backend/internal/web/embed_test.go`（静态选读；未运行）。
- 验证要求与依赖：核对本地构建资产存在及embed构建标签；只改夹具与类型断言，不改静态资源路由。

### 098 `66b7ce08e710ec5f9c5206c292ffcf3972fbe8cc`

- 原始标题：fix(frontend): show full account group names
- 结论：**建议引入**。
- 实际改动、收益与冲突：AccountGroupsCell删除max-h-14裁切、取消max-w-24截断；GroupBadge新增默认true的truncate选项，账号表显式false。
- 当前仓库证据：`frontend/src/components/account/AccountGroupsCell.vue` 仍通过固定行高/宽度限制组名；`GroupBadge.vue`无新增truncate prop，需要两组件联动。
- 关键改动文件：`frontend/src/components/account/AccountGroupsCell.vue`；`frontend/src/components/common/GroupBadge.vue`。
- 验证要求与依赖：检查其它GroupBadge调用方维持默认单行；验证长中文/无空格名称与+N按钮、移动端表格宽度。

### 099 `5753552b958084a7695fec7dfd923e30304ee108`

- 原始标题：fix(gemini): preserve authorization URL error messages
- 结论：**建议引入**。
- 实际改动、收益与冲突：useGeminiOAuth生成授权URL失败改用extractApiErrorMessage，读取后端message/detail而非只看detail；类型unknown。
- 当前仓库证据：`frontend/src/composables/useGeminiOAuth.ts` 的generateAuthUrl catch仍只取err.response?.data?.detail；本地已有apiError工具可复用。
- 关键改动文件：`frontend/src/composables/useGeminiOAuth.ts`。
- 相关测试：`frontend/src/composables/__tests__/useGeminiOAuth.errors.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：复用本地apiError工具，保留国际化fallback；测message、detail、普通Error与空错误，避免丢失真实授权失败原因。

### 100 `8e3d75b57b59b6bf8e9e695af27dab70ac360109`

- 原始标题：fix(monitor): refresh cached availability on automatic reload
- 结论：**建议引入**。
- 实际改动、收益与冲突：ChannelStatusV1View将自动刷新也指向refresh，reload后清理availability详情缓存；旧autoRefresh仅reload导致展开详情仍旧。
- 当前仓库证据：`frontend/src/views/user/ChannelStatusV1View.vue` 仍区分reload与manualReload；自动刷新绑定reload，未共用带详情缓存失效的refresh入口。
- 关键改动文件：`frontend/src/views/user/ChannelStatusV1View.vue`。
- 相关测试：`frontend/src/views/user/__tests__/ChannelStatusV1View.details.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：测手动和自动刷新后缓存失效，silent不闪加载状态；默认v2退休流程应保持不主动探测。

### 101 `bd2f53b855b79c6c20f9dee2187c78fa4ab7158c`

- 原始标题：fix(groups): reject nonpositive rate multiplier edits
- 结论：**建议引入**。
- 实际改动、收益与冲突：GroupRateMultipliersModal.updateLocalRate拒绝非有限及<=0，保留空值表示继承。当前只isNaN，0、负值、Infinity能留在表单。
- 当前仓库证据：`frontend/src/components/admin/group/GroupRateMultipliersModal.vue:421` 的updateLocalRate仅isNaN(num)后赋值，非有限及非正值未被过滤；空串分支已有继承语义。
- 关键改动文件：`frontend/src/components/admin/group/GroupRateMultipliersModal.vue`。
- 相关测试：`frontend/src/components/admin/group/__tests__/GroupRateMultipliersModal.edit.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：测0/负值/Infinity不提交、空值清除覆盖、正小数保存；该表单是真实管理员输入入口。

### 102 `d01628975d9f4d2d3ac9f9792df1bc5946a87061`

- 原始标题：fix(select): preserve keyboard input during IME composition
- 结论：**建议引入**。
- 实际改动、收益与冲突：Select.onDropdownKeyDown在switch前识别e.isComposing，避免中文输入法选词的方向/Enter键触发导航或提交。
- 当前仓库证据：`frontend/src/components/common/Select.vue` 的onDropdownKeyDown直接switch(e.key)，无e.isComposing；输入法Enter会进入正常选项处理。
- 关键改动文件：`frontend/src/components/common/Select.vue`。
- 相关测试：`frontend/src/components/common/__tests__/Select.ime.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：注意diff的hunk上下文提到onTriggerKeyDown，但新增行实际位于onDropdownKeyDown；测组合输入期间Enter与箭头不选项，组合结束照常操作。

### 103 `2bfc49477c0860058d2ff27d8ca40b0c3a1ca31c`

- 原始标题：fix(ops): initialize custom time ranges in local time
- 结论：**建议引入**。
- 实际改动、收益与冲突：OpsDashboardHeader自定义时间初始化用formatDateTimeLocalInput代替UTC toISOString切片。当前datetime-local以UTC显示、随后又按本地解析，存在时区偏移。
- 当前仓库证据：`frontend/src/views/admin/ops/components/OpsDashboardHeader.vue:181` 使用oneHourAgo.toISOString().slice(0,16)，提交时又new Date(localInput).toISOString，UTC显示被当本地时间解析。
- 关键改动文件：`frontend/src/views/admin/ops/components/OpsDashboardHeader.vue`。
- 相关测试：`frontend/src/views/admin/ops/components/__tests__/OpsDashboardHeader.timezone.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：验证非UTC/跨日期时区一小时前与当前时间，提交时ISO转换保持原瞬时；复用既有format工具。

### 104 `ba280b3d5ea4d71e2f15fdb56ce60c4a05f422c1`

- 原始标题：feat(ops): show account names in request details table
- 结论：**建议引入**。
- 实际改动、收益与冲突：OpsRequestDetailsModal用lite账号分页列表构建ID→名称缓存，明细表和卡片显示名字，找不到退#ID；缺ID强刷一次。
- 当前仓库证据：`frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue` 当前无accountNameById/refreshAccountNames；`ops/utils/accountNameMap.ts`不存在，尚无本提交的名字补全缓存。
- 关键改动文件：`frontend/src/views/admin/ops/components/OpsRequestDetailsModal.vue`；`frontend/src/views/admin/ops/utils/accountNameMap.ts`。
- 相关测试：`frontend/src/views/admin/ops/components/__tests__/OpsRequestDetailsModal.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：本地列表接口支持lite/include_scheduler_score；核验分页/删除账号fallback与并发弹窗，避免新增完整账号敏感字段请求；可以复用既有缓存如存在。

### 105 `69d6d106fd365d8ed23b5a1c82eb3a968acc1454`

- 原始标题：fix(monitor): allow adding missing platforms in v2 settings
- 结论：**建议引入**。
- 实际改动、收益与冲突：MonitorSettingsPanel计算尚未配置的CONCRETE_PLATFORM_OPTIONS，允许向draft.platforms追加enabled:true/models:[]平台。
- 当前仓库证据：`frontend/src/features/channel-monitor-v2/MonitorSettingsPanel.vue` 当前无availablePlatforms/selectedPlatform/addPlatform逻辑，缺失平台不能通过本提交所增加的控件加入draft。
- 关键改动文件：`frontend/src/features/channel-monitor-v2/MonitorSettingsPanel.vue`。
- 相关测试：`frontend/src/features/channel-monitor-v2/__tests__/MonitorSettingsPanel.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：按本地实际平台枚举调整；测已配置项不重复、保存重开持久化、新平台无模型不误触发主动探测。

### 106 `c4224045dc5cbf16f40c7b31aeb639aa0e211101`

- 原始标题：fix(payment): calculate QR countdowns from elapsed time
- 结论：**建议引入**。
- 实际改动、收益与冲突：PaymentQRCodeView计时从remainingSeconds--改为固定deadline减Date.now，后台标签页节流后倒计时恢复正确。
- 当前仓库证据：`frontend/src/views/user/PaymentQRCodeView.vue:165`仍remainingSeconds.value--，回调暂停时倒计时也暂停，未按绝对deadline追赶。
- 关键改动文件：`frontend/src/components/payment/PaymentQRDialog.vue`；`frontend/src/components/payment/PaymentStatusPanel.vue`；`frontend/src/views/user/PaymentQRCodeView.vue`。
- 相关测试：`frontend/src/components/payment/__tests__/paymentCountdown.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：测暂停回调/恢复、到期只处理一次；不能只断言setInterval调用次数。

### 107 `a56720855cf0c3ca38aea6da2a24c73e57ec1342`

- 原始标题：fix(ui): refresh countdown when tab becomes visible
- 结论：**建议引入**。
- 实际改动、收益与冲突：UsageProgressBar在visibilitychange且重新可见时立即刷新now，并在卸载移除监听；修暂停标签页后重置倒计时滞留。
- 当前仓库证据：`frontend/src/components/account/UsageProgressBar.vue` 没有refreshClockWhenVisible监听；已有interval时钟不能保证标签页恢复时立即更新。
- 关键改动文件：`frontend/src/components/account/UsageProgressBar.vue`。
- 相关测试：`frontend/src/components/account/__tests__/UsageProgressBar.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：测可见/隐藏与卸载，保留useIntervalFn现有pause/resume生命周期。

### 108 `27de27ac6496e647c59866d9ba57de61ac356771`

- 原始标题：fix(risk-control): remove deleted groups from moderation scope
- 结论：**需适配**。
- 实际改动、收益与冲突：分组删除同事务锁settings行并从moderation group_ids剔除ID；写配置先锁活分组再锁settings，防并发删除后配置写回死ID。
- 当前仓库证据：`backend/internal/repository/group_repo.go` deleteCascade未包含本提交的moderation settings锁与JSONB清理；`setting_repo.go`没有SetContentModerationConfig事务写法，服务仍普通Set。
- 关键改动文件：`backend/internal/repository/group_repo.go`；`backend/internal/repository/setting_repo.go`；`backend/internal/service/content_moderation.go`。
- 相关测试：`backend/internal/repository/group_repo_integration_test.go`（静态选读；未运行）。
- 验证要求与依赖：涉及本地group deleteCascade与内容审核配置缓存；测试并发锁序、缺组、畸形旧JSON不阻断删除、其他设置不变，确认已有缓存失效路径同步。

### 109 `f48866958b1ddcda57dab6844b6a3ade7783bc2d`

- 原始标题：fix(openai): only inject the Codex image generation bridge for OpenAI accounts
- 结论：**建议引入**。
- 实际改动、收益与冲突：isCodexImageGenerationBridgeEnabled开头限制account.Platform==OpenAI，之后仍维持账号→渠道→全局优先级。本地函数无平台限制，非OpenAI经此服务可被注入不支持的hosted工具。
- 当前仓库证据：`backend/internal/service/openai_gateway_service.go:626`直接从account覆盖项开始，没有平台检查；调用方 `openai_gateway_forward.go:407` 与 `openai_ws_forwarder_ingress.go:364`都会据此决定注入，HTTP/WS均受影响。
- 关键改动文件：`backend/internal/service/openai_gateway_service.go`。
- 相关测试：`backend/internal/service/openai_image_generation_controls_test.go`（静态选读；未运行）。
- 验证要求与依赖：测非OpenAI全局/渠道/账号开启均不注入，OpenAI原有开关仍有效；保留本地图像意图与Grok专用桥。

### 110 `0f189af8de8374cca7ddd4095a39135fe4725814`

- 原始标题：fix(groups): ignore stale RPM override loads
- 结论：**建议引入**。
- 实际改动、收益与冲突：GroupRPMOverridesModal增加loadVersion；show/group.id/卸载使旧请求失效，加载先清空数据，finally只更新当前版本loading。
- 当前仓库证据：`frontend/src/components/admin/group/GroupRPMOverridesModal.vue` 当前没有loadVersion请求版本标记；watch与loadEntries无法排除旧组响应晚返回覆盖新组。
- 关键改动文件：`frontend/src/components/admin/group/GroupRPMOverridesModal.vue`。
- 相关测试：`frontend/src/components/admin/group/__tests__/GroupRPMOverridesModal.requests.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：测A组请求晚于B组返回、关闭后响应、卸载；避免旧响应覆盖正在编辑的组。

### 111 `43768680d38ccdbd8fffc2cb91d3c1fc778a61c6`

- 原始标题：fix(billing): 取消 GPT-6.1 Sol 订阅长上下文附加费
- 结论：**需适配**。
- 实际改动、收益与冲突：CostInput增加DisableModelLongContextPricing；实际账单凭据为OAuthLike且最终上游模型为Sol时豁免模型长上下文加价，自定义区间价保持。RecordUsage与账户统计统一带豁免。
- 当前仓库证据：`backend/internal/service/billing_service.go` 的CostInput没有DisableModelLongContextPricing；`openai_gateway_usage.go`没有openAISubscriptionLongContextExempt，当前无法按实际OAuth凭据与Sol出站模型应用本次豁免。
- 关键改动文件：`backend/internal/service/billing_service.go`；`backend/internal/service/openai_gateway_usage.go`。
- 相关测试：`backend/internal/service/gateway_image_reasoning_pricing_test.go`；`backend/internal/service/openai_alpha_search_billing_test.go`等5个文件（静态选读；未运行）。
- 验证要求与依赖：本地有独立profit/WS计价、shadow实际凭据与长上下文门控；必须覆盖普通/WS/订阅/API Key/映射别名/自定义区间/账户统计，不能只改CalculateCostUnified。

### 112 `07185812655ec2e0740ca76882879c0b9ae5ee0a`

- 原始标题：fix(codex): raise GPT-6.1 Sol generated context default to 500k
- 结论：**已覆盖**。
- 实际改动、收益与冲突：本地newConfiguredCodexModelDescriptor对IsGPT61SolModelSpelling分支已经设置ContextWindow=500_000，且显式上游元数据仍覆盖；这是手工移植的可核对代码。
- 当前仓库证据：`backend/internal/service/openai_codex_models_service.go:459` 的newConfiguredCodexModelDescriptor在Sol分支显式ContextWindow=500_000；完整相关分支已对照，语义已覆盖。
- 关键改动文件：`backend/internal/service/openai_codex_models_service.go`。
- 相关测试：`backend/internal/service/openai_codex_models_service_test.go`（静态选读；未运行）。
- 验证要求与依赖：不重复应用；保留本地CLI0.164元数据结构与MaxContextWindow/guardian等新增字段。

### 113 `9a51333101a849bb948abf94c5ed53682e373edd`

- 原始标题：feat(groups): 分组名称与描述支持多语言
- 结论：**需适配**。
- 实际改动、收益与冲突：新增GroupI18n、Accept-Language首选语言解析、admin编辑、用户侧DTO本地化、JSONB迁移与ent生成文件，覆盖广场/支付/订阅/Key/用量。当前缺domain/group_i18n.go及数据库字段。
- 当前仓库证据：当前没有 `backend/internal/domain/group_i18n.go`、`handler/dto/group_localize.go`、`service/group_i18n.go`、迁移242及GroupI18nField组件，分组模型/DTO/持久化链需一起引入。
- 关键改动文件：`backend/internal/domain/group_i18n.go`；`backend/internal/service/group_i18n.go`；`backend/internal/handler/dto/group_localize.go`；`backend/internal/repository/group_repo.go`；`backend/migrations/242_group_i18n.sql`；另有22个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/domain/group_i18n_test.go`；`backend/internal/handler/dto/group_localize_test.go`等8个文件（静态选读；未运行）。
- 验证要求与依赖：与114成套；必须重生本地ent并合并现有分组字段，不复制生成文件；测管理端回写原文、用户翻译fallback、复制分组、缓存隔离。未逐行审生成ent。

### 114 `8b74741efe35a42c67bbb35b821af9a12aca2c24`

- 原始标题：fix(groups): 广场 Vary 保留 Origin，分组译文语言代码统一大小写
- 结论：**需适配**。
- 实际改动、收益与冲突：CanonicalLocale规范BCP47大小写，写入拒绝规范化重复键，旧异常大小写按确定顺序选择；广场追加Vary而非覆盖Origin。
- 当前仓库证据：当前没有GroupI18n/CanonicalLocale实现，不能独立应用语言键规范化；`backend/internal/handler/model_plaza_handler.go`需要在本地CORS之后追加Vary，前提是先引入113的按语言响应。
- 关键改动文件：`backend/internal/domain/group_i18n.go`；`backend/internal/handler/model_plaza_handler.go`；`backend/internal/service/group_i18n.go`。
- 相关测试：`backend/internal/domain/group_i18n_test.go`；`backend/internal/handler/model_plaza_handler_test.go`等4个文件（静态选读；未运行）。
- 验证要求与依赖：依113；验证zh-Hant-TW、扩展子标签、同语言重复、CORS Vary: Origin与Accept-Language并存。

### 115 `d56f5e787976c8b2a7bd471b9a3c30b0ccee50c1`

- 原始标题：feat(gateway): adapt Claude Haiku 5.5
- 结论：**需适配**。
- 实际改动、收益与冲突：新增Haiku5.5模型/effort/价格、100k整单阶梯、adaptive thinking与采样限制、电脑工具beta及OAuth不错误补temperature。当前IsHaiku55与模型条目未落地。
- 当前仓库证据：`backend/internal/pkg/claude/effort_catalog.go`没有IsHaiku55；`frontend/src/composables/useModelWhitelist.ts`无Haiku5.5预设。本地虽已有Opus/Sonnet5.5分支，不代表Haiku语义已支持。
- 关键改动文件：`backend/internal/pkg/claude/constants.go`；`backend/internal/pkg/claude/effort_catalog.go`；`backend/internal/service/billing_service.go`；`backend/internal/service/gateway_claude_oauth_body.go`；`backend/internal/service/gateway_request.go`；另有3个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/pkg/claude/constants_model_test.go`；`backend/internal/pkg/claude/effort_catalog_test.go`等6个文件（静态选读；未运行）。
- 验证要求与依赖：需116/117补齐Responses桥与top_p精确0.99；核对本地现有Claude5.5协议分支与动态价格，价格真实性本次未外查。

### 116 `26f70c6a873bbeed126f1944068a7eb183c89f16`

- 原始标题：fix(gateway): wire Claude Haiku 5.5 into Responses bridge
- 结论：**需适配**。
- 实际改动、收益与冲突：Responses→Anthropic与反向响应/流状态扩展Haiku5.5签名与redacted thinking保留；Haiku允许forced tool use但拒between_tools，区别Opus/Sonnet。
- 当前仓库证据：当前ResponsesToAnthropicRequest与AnthropicToResponsesResponse未引用IsHaiku55，缺Haiku特有forced-tool/disabled/signature语义；需在 `backend/internal/pkg/apicompat` 两向桥中补齐。
- 关键改动文件：`backend/internal/pkg/apicompat/anthropic_to_responses_response.go`；`backend/internal/pkg/apicompat/responses_to_anthropic_request.go`；`backend/internal/service/gateway_request.go`。
- 相关测试：`backend/internal/pkg/apicompat/anthropic_responses_test.go`（静态选读；未运行）。
- 验证要求与依赖：依115/117；测流/非流thinking签名、tool_choice强制、disabled高effort冲突，不能简单把Haiku当Sonnet复制。

### 117 `c4bdc484753acc95f9a775854af303146389286c`

- 原始标题：fix(haiku): 修正采样边界、默认模型配置与 lint
- 结论：**需适配**。
- 实际改动、收益与冲突：修Haiku显式top_p只允许0.99（不是[.99,1]）；前端白名单与预设映射补Haiku；OAuth条件等价改写满足lint。
- 当前仓库证据：`frontend/src/composables/useModelWhitelist.ts`无claude-haiku-5-5条目；本地也无115的Haiku专用校验分支，本次top_p精确边界修复需随Haiku支持一起落地。
- 关键改动文件：`backend/internal/pkg/apicompat/responses_to_anthropic_request.go`；`backend/internal/service/gateway_claude_oauth_body.go`；`backend/internal/service/gateway_request.go`；`frontend/src/composables/useModelWhitelist.ts`。
- 相关测试：`backend/internal/pkg/apicompat/anthropic_responses_test.go`；`backend/internal/service/gateway_request_haiku55_test.go`等3个文件（静态选读；未运行）。
- 验证要求与依赖：随115/116整体移植；测top_p=1被拒、0.99有效、Sonnet既有范围保持；前端与后端模型目录一致。

### 118 `b669dc6daf1a9909b02ecf1f9cb692d88875e086`

- 原始标题：test(frontend): 将模型白名单回归用例纳入 CI
- 结论：**建议引入**。
- 实际改动、收益与冲突：Makefile FRONTEND_CRITICAL_VITEST加入useModelWhitelist.spec.ts；防模型预设修复后测试未进入CI。
- 当前仓库证据：`Makefile` 的FRONTEND_CRITICAL_VITEST未列入src/composables/__tests__/useModelWhitelist.spec.ts；是否本地实际CI入口仍需移植时执行验证。
- 关键改动文件：`Makefile`。
- 验证要求与依赖：确认本地测试存在及实际CI调用此目标；若没有则将相关本地用例接入已有测试清单。

### 119 `3bfd40f5afd820ff7fae32f3f26c75fe3e0dd504`

- 原始标题：fix(gemini): 引用缓存不存在的 403 不再停用账号并换号
- 结论：**建议引入**。
- 实际改动、收益与冲突：ForwardNative在错误策略检查前识别cached content not found，原样回客户端，不处罚账号/重试换号；缓存资源账号绑定错误换号无法修复。
- 当前仓库证据：`backend/internal/service/gemini_messages_compat_service.go` ForwardNative循环仍先checkErrorPolicyInLoop，未有cachedContentNotFoundInLoop前置识别，可能先处罚账号再处理请求级错误。
- 关键改动文件：`backend/internal/service/gemini_messages_compat_service.go`。
- 相关测试：`backend/internal/service/gemini_cached_content_not_found_test.go`（静态选读；未运行）。
- 验证要求与依赖：可先于120独立修复原生透传缓存引用；测403缓存不存在不写rate limit/temp-unschedulable，其余403策略不变。

### 120 `d5a802f84a6ae3405bee2a6e0160675785abf237`

- 原始标题：feat(gemini): 支持显式上下文缓存（AI Studio / Vertex / 自建网关上游）
- 结论：**需适配**。
- 实际改动、收益与冲突：新增cachedContents CRUD handler/service/repository与243迁移，随机公开ID映射账号/Key资源，原生生成绑定账号；增加缓存存储计费、固定金额预留、前端上游能力声明。当前资源层、路由与表均缺失。
- 当前仓库证据：本地没有 `gemini_cached_contents_handler.go`、`gemini_cached_content.go`、`gemini_cached_content_repo.go` 或243迁移；`gemini_v1beta_handler.go`无resolveGeminiCachedContentBinding，未实现网关自管缓存资源/绑定计费。
- 关键改动文件：`backend/internal/handler/gemini_cached_contents_handler.go`；`backend/internal/service/gemini_cached_content.go`；`backend/internal/repository/gemini_cached_content_repo.go`；`backend/internal/handler/gemini_v1beta_handler.go`；`backend/migrations/243_gemini_cached_contents.sql`；另有18个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/handler/endpoint_test.go`；`backend/internal/handler/gemini_cached_contents_billing_test.go`等12个文件（静态选读；未运行）。
- 验证要求与依赖：这是完整新子系统，不宜单补丁带入；至少联动121/123/128/129/132/133，核验owner隔离、TTL、上游失败补偿、删除/延期、计费幂等、跨节点租约；保留本地会话粘性隔离。展开检查手写核心，未逐行审wire_gen/生成工件。

### 121 `3abd64658297f5ec47ad09120a0a26d6081d42de`

- 原始标题：fix(ops): Gemini 原生路由的网关本地错误按错误类型归类
- 结论：**需适配**。
- 实际改动、收益与冲突：Google格式本地错误另存OpsLocalErrorType/Code，区分请求级缓存引用失败与上游故障；安全审计google错误同步类型，不让Google缺type导致错误归因。
- 当前仓库证据：`backend/internal/service/ops_upstream_context.go`无本提交的OpsLocalErrorType/OpsRequestScopedError状态；`gemini_v1beta_handler.go`与ops_error_logger.go尚未串起Google错误类型登记，需兼容现有审计归因逻辑。
- 关键改动文件：`backend/internal/handler/gemini_cached_contents_handler.go`；`backend/internal/handler/gemini_v1beta_handler.go`；`backend/internal/handler/ops_error_logger.go`；`backend/internal/handler/security_audit_errors.go`；`backend/internal/service/gemini_messages_compat_service.go`；另有1个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/handler/ops_error_logger_google_test.go`；`backend/internal/service/gemini_cached_content_not_found_test.go`（静态选读；未运行）。
- 验证要求与依赖：本地已有ops分类定制，独立抽取v1beta/ops可获益；缓存部分依120。测billing限额归业务、缓存不存在归permission、真实上游故障不被掩盖。

### 122 `64331a97921bd5a43e7b54ba6a517476c985e8e5`

- 原始标题：ui(gemini): Vertex 显示名改为 Agent Platform (Vertex)
- 结论：**不引入**。
- 实际改动、收益与冲突：只把Vertex前端显示名改Agent Platform (Vertex)，不修改认证/路由。
- 当前仓库证据：`frontend/src/components/common/PlatformTypeBadge.vue`仍使用Vertex显示名；只是文案选择，不影响 `vertex_service_account.go` 后端认证。
- 关键改动文件：`frontend/src/components/account/CreateAccountModal.vue`；`frontend/src/components/common/PlatformTypeBadge.vue`。
- 验证要求与依赖：没有功能收益或当前命名需求；若未来统一品牌展示可单改locale文案，勿为此引入123。

### 123 `81e6ec5346c645fa03d794495198e32e4b910598`

- 原始标题：feat(gemini): Vertex 账号支持 API Key 认证
- 结论：**需适配**。
- 实际改动、收益与冲突：Vertex Gemini支持auth_mode=apikey及无project Express全局URL，setVertexGeminiAuth统一Bearer与API Key认证；SA显式缓存仍仅OAuth token可用；前端创建/编辑加入认证模式。
- 当前仓库证据：`backend/internal/service/vertex_service_account.go`没有IsVertexAPIKey/buildVertexGeminiAccountURL/setVertexGeminiAuth；当前项目级URL构造要求project_id且转发取SA access token，未覆盖Express Key模式。
- 关键改动文件：`backend/internal/service/account_test_service.go`；`backend/internal/service/gemini_cached_content.go`；`backend/internal/service/gemini_chat_completions_compat_service.go`；`backend/internal/service/gemini_messages_compat_service.go`；`backend/internal/service/vertex_service_account.go`；另有2个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/service/gemini_cached_content_test.go`；`backend/internal/service/gemini_vertex_api_key_forward_test.go`等5个文件（静态选读；未运行）。
- 验证要求与依赖：当前IsVertexAPIKey/buildVertexGeminiAccountURL不存在，不能凭部分UI相似判覆盖；三条转发、测试、脱敏、快照投影、模型目录全部核验；不得将Anthropic Vertex也开放API Key。

### 124 `2588675ce7c0fa13189379489cc4edabe48f6fcd`

- 原始标题：fix(gemini): 第三方上游 API Key 与 Vertex 不再套用 AI Studio 等级和模拟配额
- 结论：**建议引入**。
- 实际改动、收益与冲突：第三方Gemini API Key与Vertex不套AI Studio模拟日配额/等级；第三方429只按明确retryDelay或短窗fallback，避免误停至PST午夜；前端隐藏不适用统计。
- 当前仓库证据：`backend/internal/service/account.go`无IsGeminiThirdPartyAPIKey；`gemini_quota.go`的geminiQuotaTierKeyForAccount未新增第三方/Vertex排除；UI未按官方主机来源限制展示模拟额度。
- 关键改动文件：`backend/internal/service/account.go`；`backend/internal/service/gemini_messages_compat_service.go`；`backend/internal/service/gemini_quota.go`；`frontend/src/components/account/AccountUsageCell.vue`；`frontend/src/components/account/CreateAccountModal.vue`；另有1个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/service/gemini_error_policy_test.go`；`backend/internal/service/gemini_quota_test.go`等4个文件（静态选读；未运行）。
- 验证要求与依赖：独立抽取后端/前端主机识别，当前无IsGeminiThirdPartyAPIKey；前端geminiExplicitCache工具依120，需就地复用/提取最小判定；测官方默认/自定义/Vertex。

### 125 `1c68f9996df8e0bd7933d94d4ca826bc3dc570c7`

- 原始标题：feat(gemini): 适配 AI Studio AQ. 格式 Key
- 结论：**建议引入**。
- 实际改动、收益与冲突：日志脱敏与channel monitor补AQ.Ab长Key正则，并更新输入placeholder；当前redact没有此格式处理。
- 当前仓库证据：`backend/internal/util/logredact/redact.go` 与 `channel_monitor_checker.go` 的现有模式都没有本提交AQ.Ab长Key匹配；新格式会绕过基于旧AIza格式的脱敏。
- 关键改动文件：`backend/internal/service/channel_monitor_checker.go`；`backend/internal/util/logredact/redact.go`；`frontend/src/components/account/CreateAccountModal.vue`；`frontend/src/components/account/EditAccountModal.vue`。
- 相关测试：`backend/internal/service/channel_monitor_checker_body_test.go`；`backend/internal/util/logredact/redact_test.go`（静态选读；未运行）。
- 验证要求与依赖：真实凭据输出边界优先；测AQ.Ab Key不出现在日志、普通文本不过度截断、原AIza/sk/JWT仍有效；不需要真实Key。

### 126 `6f9b02f12c3d414ed324460a0e82d7793a471f72`

- 原始标题：feat(gemini): Vertex 服务账号支持同步上游模型列表
- 结论：**需适配**。
- 实际改动、收益与冲突：fetchUpstreamModelList新增Vertex SA模型同步，分页读取Google publisher目录、只保留Gemini并复用响应读取/代理链。当前没有Vertex publisher-model专用路径。
- 当前仓库证据：`backend/internal/service/upstream_models.go:727` 的fetchUpstreamModelList当前无fetchVertexGeminiUpstreamModels及vertexPublisherModels分页实现，虽有其它平台上游同步，不能判Vertex已支持。
- 关键改动文件：`backend/internal/service/upstream_models.go`。
- 相关测试：`backend/internal/service/upstream_models_vertex_test.go`（静态选读；未运行）。
- 验证要求与依赖：与131共用端点实现，避免重复；测分页、区域/global、认证、API Key不走不支持的publisher列表。

### 127 `c9e4b267b6fac1f00060eab983e606f7a2b2d4fc`

- 原始标题：fix(account): 同步上游模型失败时显示后端返回的错误原因
- 结论：**建议引入**。
- 实际改动、收益与冲突：编辑账号和ModelWhitelistSelector同步上游模型失败均用extractApiErrorMessage，展示后端错误原因。
- 当前仓库证据：`frontend/src/components/account/EditAccountModal.vue` 的syncAntigravityUpstreamModels与ModelWhitelistSelector的syncUpstreamModels仍选择error.message；未读取后端响应message/detail。
- 关键改动文件：`frontend/src/components/account/EditAccountModal.vue`；`frontend/src/components/account/ModelWhitelistSelector.vue`。
- 相关测试：`frontend/src/components/account/__tests__/ModelWhitelistSelector.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：本地仍取error.message而非响应message/detail；测试两入口错误体与fallback；无需依赖新平台目录。

### 128 `05d7946af84228589e5a6a8d8b6bc08d686ba778`

- 原始标题：fix(gemini): 显式缓存错误透传收窄到上游网关账号，延长沿用创建时计费口径
- 结论：**需适配**。
- 实际改动、收益与冲突：显式缓存持久化创建时ChannelUsage，延期沿用计费模型/渠道；upstream passthrough限制绑定缓存的自建网关；调度投影保留base_url/auth_mode/能力键；请求ID纳入强制usage。
- 当前仓库证据：本地无GeminiCachedContent资源模型/表，因而不存在ChannelUsage持久化与BoundUpstreamPassthrough标记；`scheduler_cache.go`凭据投影必须在引入120后保留能力字段。
- 关键改动文件：`backend/internal/handler/gemini_cached_contents_handler.go`；`backend/internal/repository/gemini_cached_content_repo.go`；`backend/internal/repository/scheduler_cache.go`；`backend/internal/service/gateway_service.go`；`backend/internal/service/gateway_usage_billing.go`；另有3个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/handler/gemini_cached_contents_billing_test.go`；`backend/internal/handler/gemini_cached_contents_handler_test.go`等7个文件（静态选读；未运行）。
- 验证要求与依赖：依120/123；测延期后渠道改价不偷换创建口径、官方账号429仍执行正常策略、跨节点缓存账号筛选、幂等计费；需244迁移。

### 129 `f0ee427f0ad2f7eab6ab8f095ba05041af3a48f8`

- 原始标题：feat(gemini): 显式缓存存储价内置官方价并按 Pro / Flash 系列兜底
- 结论：**需适配**。
- 实际改动、收益与冲突：CacheStoragePricePerTokenHour优先override再精确内置型号，再Pro/Flash系列兜底；新增gemini_cache_storage_pricing.go。
- 当前仓库证据：`backend/internal/service/gemini_cache_storage_pricing.go`不存在，BillingService也没有该显式缓存存储定价入口；不是本地token缓存读写价的重复功能。
- 关键改动文件：`backend/internal/service/billing_service.go`；`backend/internal/service/gemini_cache_storage_pricing.go`；`backend/internal/service/pricing_service.go`；`deploy/config.example.yaml`。
- 相关测试：`backend/internal/service/gemini_cache_storage_pricing_test.go`（静态选读；未运行）。
- 验证要求与依赖：依120；内置/覆盖/未知模型0价拒绝、命名空间与变体解析需测。价格外部时效未核实，不能当已验证官方价格。

### 130 `4426593e9d7dc38a009f28da038ac2906a57c17e`

- 原始标题：fix(gemini): 3.6 Flash 兜底价改为官方优惠价
- 结论：**建议引入**。
- 实际改动、收益与冲突：Gemini3.6 Flash fallback与内置价表降为input0.75e-6/output3.75e-6/cache0.075e-6，priority也减半。本地billing_service.go:482仍1.5/7.5/0.15，资源表gemini-3.6-flash:2618同样旧价；相邻3.7价已更新不能据此认定3.6已覆盖。
- 当前仓库证据：`backend/internal/service/billing_service.go:482`的gemini-3.6-flash仍1.5e-6/7.5e-6/0.15e-6；`backend/resources/model-pricing/model_prices_and_context_window.json:2618`的同名键也为旧价，3.7的优惠价不能用于判断3.6覆盖。
- 关键改动文件：`backend/internal/service/billing_service.go`。
- 相关测试：`backend/internal/service/account_stats_pricing_test.go`；`backend/internal/service/pricing_service_test.go`（静态选读；未运行）。
- 验证要求与依赖：同步fallback与内置资源价，测exact/tier别名与priority避免只改展示。提交注明优惠截止2026-12-31，采用前核实官方价格与切换策略；本次未验证价格源。

### 131 `daebb7a80c720763a01e5cef8799c030aed0b058`

- 原始标题：fix(gemini): 模型列表与模型详情接口支持 Agent Platform (Vertex) 账号
- 结论：**需适配**。
- 实际改动、收益与冲突：原List/Get模型从AIStudio专用选择/GET改SelectAccountForModelsEndpoints和ForwardListModels/ForwardGetModel，支持Vertex并复用publisher列表逻辑。
- 当前仓库证据：本地没有 `backend/internal/service/gemini_models_endpoints.go`；`gemini_v1beta_handler.go` List/Get模型仍调用SelectAccountForAIStudioEndpoints与ForwardAIStudioGET，尚非Vertex端点路径。
- 关键改动文件：`backend/internal/handler/gemini_v1beta_handler.go`；`backend/internal/service/gemini_messages_compat_service.go`；`backend/internal/service/gemini_models_endpoints.go`；`backend/internal/service/upstream_models.go`。
- 相关测试：`backend/internal/handler/gemini_v1beta_handler_test.go`；`backend/internal/handler/gemini_vertex_models_test.go`等3个文件（静态选读；未运行）。
- 验证要求与依赖：当前gemini_models_endpoints.go不存在且handler仍走AI Studio；依126的Vertex元数据，兼顾123 API Key不支持列表的限制；测模型响应名/方法与原生路由。

### 132 `26b31a83e4190a3b38bce112fc597defbfb9515b`

- 原始标题：fix(gemini): 显式缓存修改串行化、失败回滚，确定金额费用按 Key / 订阅预留
- 结论：**需适配**。
- 实际改动、收益与冲突：缓存PATCH/DELETE加数据库mutation token/lease串行化与后台恢复回滚；固定费用按订阅/API Key预留并读取权威用量，释放与usage落库衔接；245迁移。
- 当前仓库证据：本地无缓存mutation_token/mutation_lease_until模型与245迁移；`billing_inflight_reservation.go`虽有余额预留，但没有该提交围绕缓存固定费用的订阅/Key scope预留完整链。
- 关键改动文件：`backend/internal/service/gemini_cached_content.go`；`backend/internal/handler/gemini_cached_contents_handler.go`；`backend/internal/service/billing_cache_service.go`；`backend/internal/service/billing_inflight_reservation.go`；`backend/migrations/245_gemini_cached_contents_mutation_lease.sql`；另有6个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/cmd/server/wire_gen_test.go`；`backend/internal/handler/gemini_cached_contents_billing_test.go`等8个文件（静态选读；未运行）。
- 验证要求与依赖：依120/128并合133；跨系统与计费边界必须保留租约/补偿，不因简化删安全措施。测并发409、进程中断、续期失败回滚、双请求额度穿透、redis失效策略。

### 133 `43a3c197cb7663582ad1b6b21c761b84776193c9`

- 原始标题：fix(gemini): 显式缓存恢复流程区分账号已删除与暂不支持缓存，上游必已过期后作废意图
- 结论：**需适配**。
- 实际改动、收益与冲突：stale mutation恢复区分账号删除和暂不支持缓存；只在上游最长TTL+时钟宽限必过期时才放弃意图，避免能力关闭后错误删本地记录。
- 当前仓库证据：本地没有GeminiCachedContentService恢复流程；该区分deleted/unsupported及TTL耗尽的修复只能随132恢复子系统应用，单独无落点。
- 关键改动文件：`backend/internal/service/gemini_cached_content.go`。
- 相关测试：`backend/internal/handler/gemini_cached_contents_billing_test.go`（静态选读；未运行）。
- 验证要求与依赖：依132；测账号删除、凭据暂时不支持、恢复失败保留意图、TTL与一小时宽限边界。

### 134 `cfbbfd6b3b41bbdeee748e25d8aa3de866b811c7`

- 原始标题：fix(antigravity): isolate image model rate limits
- 结论：**建议引入**。
- 实际改动、收益与冲突：Antigravity图片Gemini模型不再附加共享Gemini文本模型冷却键；请求键计算与写入冷却键两处都收窄。
- 当前仓库证据：`backend/internal/service/model_rate_limit.go` 对Antigravity Gemini家族键当前未用isAntigravityGeminiImageModel排除图片；需要请求键和写入键一起改才能真正隔离。
- 关键改动文件：`backend/internal/service/model_rate_limit.go`。
- 相关测试：`backend/internal/service/antigravity_rate_limit_test.go`；`backend/internal/service/model_rate_limit_test.go`（静态选读；未运行）。
- 验证要求与依赖：本地model_rate_limit仍会混共享键；测图像429不牵连文本、文本限流不阻图像、同图像模型仍冷却。

### 135 `ba2a71733272b75e948c28a6fb41ffa506a40edc`

- 原始标题：fix(gemini): preserve images from tool results
- 结论：**建议引入**。
- 实际改动、收益与冲突：Claude tool_result中的base64图片抽为Gemini inlineData追加，functionResponse保留文字，纯图片时用明确占位文本；当前转换仅提取content文本。
- 当前仓库证据：`backend/internal/service/gemini_messages_compat_service.go` 的convertClaudeMessagesToGeminiContents在tool_result中仍提取文字，缺extractClaudeToolResultImageParts与toolImageParts追加。
- 关键改动文件：`backend/internal/service/gemini_messages_compat_service.go`。
- 相关测试：`backend/internal/service/gemini_messages_compat_service_test.go`（静态选读；未运行）。
- 验证要求与依赖：覆盖图像工具结果/混合/多tool结果的顺序、MIME与base64保留；不把模型生成图像误当输入工具图像。

### 136 `492c63059980ea5797cc2c34ba642ffee9a56aee`

- 原始标题：fix(gemini): satisfy test lint
- 结论：**需适配**。
- 实际改动、收益与冲突：只修135新增测试的类型断言lint，逐层require.True后取functionResponse/inlineData。
- 当前仓库证据：135新增的TestConvertClaudeMessagesToGeminiGenerateContent_ExtractsToolResultImages在本地尚未引入；该提交仅细化测试断言，不改生产转换。
- 改动范围：仅测试/夹具。
- 相关测试：`backend/internal/service/gemini_messages_compat_service_test.go`（静态选读；未运行）。
- 验证要求与依赖：随135用例带入；本地该回归用例未存在，单独应用没有收益。

### 137 `ce6242fca1ef84da4855a92cd80117028b3a99c0`

- 原始标题：fix(quota): recognize international coding endpoints
- 结论：**需适配**。
- 实际改动、收益与冲突：Kimi Coding官方域名识别扩api.kimi.ai，严格解析https host、443与coding路径，避免contains将恶意域名当官方并发送凭据；加入默认允许域名。
- 当前仓库证据：`backend/internal/service/account.go` 的GetCodingPlanProvider与config默认上游域名尚未新增本次api.kimi.ai识别；需对照本地CN provider判断结构，而非直接复制上游函数位置。
- 关键改动文件：`backend/internal/config/config.go`；`backend/internal/service/account.go`。
- 相关测试：`backend/internal/config/config_test.go`；`backend/internal/service/account_kimi_coding_test.go`等4个文件（静态选读；未运行）。
- 验证要求与依赖：本地GetCodingPlanProvider与CN平台注册结构不同，应复用本地官方主机判断；测国内/国际、端口、userinfo、query、fragment、伪子域，别放宽SSRF边界。

### 138 `983cf297da59e3be23b5e3f2c4eded945b53f408`

- 原始标题：fix(gateway): retry rejected structured input through chat endpoint
- 结论：**需适配**。
- 实际改动、收益与冲突：自动探测Responses支持的OpenAI API Key上游拒绝结构化input时，chat入站重试原始chat endpoint；不对原生Responses入站做同样降级。
- 当前仓库证据：`backend/internal/service/openai_gateway_chat_completions.go` 没有isConvertedResponsesInputStringRejection对应判断；本地已有原始chat降级流程，但未包含该受限结构化input400重试条件。
- 关键改动文件：`backend/internal/service/openai_gateway_chat_completions.go`。
- 相关测试：`backend/internal/service/openai_gateway_chat_completions_test.go`（静态选读；未运行）。
- 验证要求与依赖：必须捆139/141，原始宽regex可误判并重复请求；本地fallback已有多套路径，需要合并到现有单一分派点，保持模型映射/计费/响应语义。

### 139 `c15d858eca81b269c41ba3bc161f923b179a860c`

- 原始标题：fix(gateway): preserve Responses semantics in chat fallback
- 结论：**需适配**。
- 实际改动、收益与冲突：收紧错误判定为完整输入校验错误，读取错误体返回complete标志；raw chat fallback沿用最初startTime，Responses fallback也传递时间保留统计语义。
- 当前仓库证据：本地openai_gateway_cc_pipeline.go读取上游错误未新增readOpenAIUpstreamErrorWithCompleteness；raw chat/Responses fallback的startTime传递需逐入口核对，不能只匹配既有错误字符串。
- 关键改动文件：`backend/internal/service/openai_gateway_cc_pipeline.go`；`backend/internal/service/openai_gateway_chat_completions.go`；`backend/internal/service/openai_gateway_chat_completions_raw.go`；`backend/internal/service/openai_gateway_forward.go`；`backend/internal/service/openai_gateway_responses_chat_fallback.go`。
- 相关测试：`backend/internal/service/openai_gateway_chat_completions_test.go`；`backend/internal/service/openai_gateway_responses_chat_fallback_test.go`（静态选读；未运行）。
- 验证要求与依赖：依138并联141；测截断错误体不触发、函数工具/stream/response格式一致、总延迟包含首请求；保持本地错误解析与重写。

### 140 `f7f1112cdab09e9bee982d77ae253d5d20a56e73`

- 原始标题：fix(deps): update Vue and source map security patches
- 结论：**建议引入**。
- 实际改动、收益与冲突：Vue依赖下限从^3.4.0提升^3.5.42，同时lock更新source-map相关安全补丁。当前package.json仍^3.4.0。
- 当前仓库证据：`frontend/package.json:32`仍vue:^3.4.0；`frontend/pnpm-lock.yaml:58`实际锁3.5.26，低于该提交的3.5.42。安全漏洞可达性仍需官方公告/扫描复核，不能只用版本差断言可利用。
- 关键改动文件：`frontend/package.json`。
- 验证要求与依赖：应以本地lock实际解析版本评估已覆盖程度；不覆盖整个上游lock，重算本地依赖并跑类型/构建与关键组件测试；本次未查漏洞公告。

### 141 `63800c50d462efc6436e96b9dd46ae8f98a054bc`

- 原始标题：fix(gateway): guard structured-input fallback against unsafe retries
- 结论：**需适配**。
- 实际改动、收益与冲突：structured-input重试前确认ctx/client未取消且响应未提交；错误JSON检查重复键、歧义与执行痕迹，避免把已执行请求当校验失败重放。
- 当前仓库证据：当前没有138的structured-input专门重试分支，因此也没有对应cancel/committed/歧义JSON联合保护；这些保护必须随新增重试一起引入，不能单独加无调用分支。
- 关键改动文件：`backend/internal/service/openai_gateway_chat_completions.go`。
- 相关测试：`backend/internal/service/openai_gateway_chat_completions_test.go`（静态选读；未运行）。
- 验证要求与依赖：与138/139一起；必须用本地committed标记和SSE路径判断，测取消/已写响应/重复键/非校验业务错误均零重放。

### 142 `daa79157491f25939b502f5c040eb11768a46e30`

- 原始标题：fix(deps): update SheetJS past expired security exceptions
- 结论：**建议引入**。
- 实际改动、收益与冲突：SheetJS从npm xlsx^0.18.5改官方0.20.3 tarball并更新lock，移除过期安全例外。当前仍0.18.5声明。
- 当前仓库证据：`frontend/package.json:37`声明xlsx:^0.18.5，与补丁官方0.20.3 tarball不同；本地导入导出使用处需在换包后验证。
- 关键改动文件：`frontend/package.json`。
- 验证要求与依赖：核对导入导出调用方/打包器和网络供应链策略，验证已有表格模板与恶意文件安全回归；不凭包标题宣称安全扫描通过。

### 143 `4d9ac3e482106d2da3a39fe3e546d8d6801f1cc1`

- 原始标题：chore: sync VERSION to 0.2.16 [skip ci]
- 结论：**不引入**。
- 实际改动、收益与冲突：仅版本从0.2.15升0.2.16。
- 当前仓库证据：该补丁只写 `backend/cmd/server/VERSION` 的0.2.16，没有生产函数变化；本地版本由本地发布流程决定。
- 关键改动文件：`backend/cmd/server/VERSION`。
- 验证要求与依赖：沿用本地版本线，不混入上游release身份。

### 144 `9c2cca9e627291bc719bada3d57de23b625a4180`

- 原始标题：chore: update sponsors
- 结论：**不引入**。
- 实际改动、收益与冲突：README多语言赞助商行、头像/图片资产更新，与网关业务无关。
- 当前仓库证据：补丁仅README/赞助图片；当前本地多语言README路径不同，不能整包覆盖宣传资产。
- 关键改动文件：`README.md`；`README_CN.md`；`README_JA.md`；`assets/partners/logos/aimzoon.jpg`；`assets/partners/logos/modelflare.png`；另有1个手写/配置文件，范围未在此穷举。
- 验证要求与依赖：不要覆盖本地README/品牌或删除本地赞助信息；二进制赞助图片未逐像素检查。

### 145 `373d43533a0fa06e1993af1eedd61154db8ebfb0`

- 原始标题：feat(antigravity): support Gemini 4 Argon and accurate upstream model catalogs
- 结论：**需适配**。
- 实际改动、收益与冲突：新增Gemini4 Argon/Claude5.5档位目录、付费tier识别、新hub UA、账号测试图片结果与提示词、上游目录原始元数据；移除运行时默认白名单补齐。
- 当前仓库证据：`backend/internal/domain/constants.go`与 `pkg/antigravity/claude_types.go` 未有本次完整Argon/Claude5.5 tier目录；`account.go`仍有本地默认透传补齐，而145先删除该行为，必须采用154最终策略。
- 关键改动文件：`backend/internal/domain/constants.go`；`backend/internal/pkg/antigravity/claude_types.go`；`backend/internal/service/upstream_models.go`；`backend/internal/service/antigravity_gateway_service.go`；`backend/internal/service/account.go`；另有15个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/domain/constants_test.go`；`backend/internal/handler/admin/account_handler_available_models_test.go`等14个文件（静态选读；未运行）。
- 验证要求与依赖：与153/154合并评估，154后续恢复legacy默认补齐，不能接受145中间态破坏本地默认透传。测catalog来源、实际上游模型支持、Codex元数据与图片测试；不替换本地CLI0.164版本。

### 146 `feab6d80dc09fa2d64894d20cdca64fb4e460931`

- 原始标题：fix(antigravity): preserve tool_result images, native Gemini functionCall/thoughtSignature, and composite /v1beta routing
- 结论：**需适配**。
- 实际改动、收益与冲突：Antigravity tool_result inline图像/文档保留；原生functionCall补thoughtSignature；stream转nonstream收集全部parts而非仅text/image；composite /v1beta可路由Antigravity。
- 当前仓库证据：`backend/internal/pkg/antigravity/request_transformer.go`缺extractToolResultInlineParts；`antigravity_gateway_gemini.go`未接本次functionCall签名补全，stream合并路径也未采用collectedParts完整收集。
- 关键改动文件：`backend/internal/handler/gemini_v1beta_handler.go`；`backend/internal/pkg/antigravity/request_transformer.go`；`backend/internal/service/antigravity_gateway_gemini.go`；`backend/internal/service/antigravity_gateway_streaming.go`。
- 相关测试：`backend/internal/handler/gemini_v1beta_handler_test.go`；`backend/internal/pkg/antigravity/request_transformer_test.go`等3个文件（静态选读；未运行）。
- 验证要求与依赖：当前有原生stream定制和composite逻辑，逐项移植；测混合parts顺序/签名、函数工具结果图片与文档、复合组非Gemini拒绝。

### 147 `92c9433d40db69b00f8e158fc573e2b796c4a678`

- 原始标题：chore: update sponsors
- 结论：**不引入**。
- 实际改动、收益与冲突：删除README中的赞助商表格条目，只涉宣传信息。
- 当前仓库证据：补丁只删README赞助行，无生产函数或配置变化；本地品牌文档独立维护。
- 关键改动文件：`README.md`；`README_CN.md`；`README_JA.md`。
- 验证要求与依赖：保留本地文档；无需业务测试。

### 148 `d19265ddb218c79e951011e6dec54fe17a215223`

- 原始标题：fix(gateway): 拒绝含 OpenAI 网关 OAuth 的 composite 歧义路由，防止凭据误入 Anthropic 管线
- 结论：**需适配**。
- 实际改动、收益与冲突：CompositeModelOwnership标记OpenAI网关OAuth声明；跨平台歧义配置在HTTP/WS明确拒绝，并在Anthropic建请求与count_tokens前阻断外平台OAuth，防凭据误送。
- 当前仓库证据：`backend/internal/service/composite_route_resolver.go`未有本次OAuthAmbiguous判定，`gateway_upstream_request.go`缺anthropicMessagesCredentialAllowed边界检查；已有本地WS composite测试仍需扩展此攻击/误配置路径。
- 关键改动文件：`backend/internal/handler/openai_gateway_handler.go`；`backend/internal/server/routes/gateway.go`；`backend/internal/service/composite_model_route.go`；`backend/internal/service/composite_route_resolver.go`；`backend/internal/service/gateway_count_tokens.go`；另有2个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/server/routes/composite_platform_test.go`；`backend/internal/service/composite_platform_test.go`等6个文件（静态选读；未运行）。
- 验证要求与依赖：优先安全修复；本地已有WS composite与凭据failover测试，须复用而非覆盖。用配置候选而非健康候选定归属，验证临时限流不改变归属、HTTP/WS/count_tokens均未发错凭据。

### 149 `1774b79dcbf9ba25a007772c015d29758b7fdbc2`

- 原始标题：fix(zhipu): 修正 GLM Coding Plan 5h/weekly 用量窗口标反
- 结论：**建议引入**。
- 实际改动、收益与冲突：Zhipu CREDIT_LIMIT fallback也先按unit(3=5h/6=weekly)分类，不能按reset时间排序，否则周末周窗口更早重置导致标反。
- 当前仓库证据：`backend/internal/service/cn_provider_quota_service.go` 的parseZhipuTokenTiers未引入本次CREDIT_LIMIT携带unit并显式分类的完整修正；reset排序在周窗口提前重置时不可靠。
- 关键改动文件：`backend/internal/service/cn_provider_quota_service.go`。
- 相关测试：`backend/internal/service/cn_providers_test.go`（静态选读；未运行）。
- 验证要求与依赖：核对本地parseZhipuTokenTiers实现；测CREDIT/TOKENS、周期边界、无unit降级，不改用量百分比口径。

### 150 `89ad4c2caa0a2c9f5a393f9d1c9c7e3c73f7c3b3`

- 原始标题：fix(gemini): Vertex 共享容量不足的 429 不冷却账号，改为同号退避重试
- 结论：**建议引入**。
- 实际改动、收益与冲突：Vertex 429 RESOURCE_EXHAUSTED且无RetryInfo/QuotaFailure/ErrorInfo的共享容量不足不写账号冷却，交请求内退避重试；真正项目配额仍短冷却。
- 当前仓库证据：`backend/internal/service/gemini_messages_compat_service.go`无isVertexSharedCapacity429；现有Vertex429处理仍统一账号短冷却，无法将共享容量不足与账号限额区分。
- 关键改动文件：`backend/internal/service/gemini_messages_compat_service.go`。
- 相关测试：`backend/internal/service/gemini_error_policy_test.go`；`backend/internal/service/gemini_vertex_api_key_forward_test.go`（静态选读；未运行）。
- 验证要求与依赖：需确认本地三条Gemini转发重试一致且不会无限同号重放；测容量不足/配额/预留吞吐分类、最大尝试和取消。

### 151 `20c4bf5ae59e4cb94669165436bb57ee16c41cb3`

- 原始标题：fix(gemini): Vertex SA 已不存在时账号置为 error 并换号
- 结论：**建议引入**。
- 实际改动、收益与冲突：token交换错误变结构化vertexTokenRequestError；仅invalid_grant account not found触发SA账号error并换号，三入口共享；Ops标account_auth。
- 当前仓库证据：`backend/internal/service/gemini_vertex_token_error.go`不存在；vertex_service_account.go的交换失败仍fmt.Errorf文本，Forward/ForwardNative/CC未有handleVertexServiceAccountNotFound换号处理。
- 关键改动文件：`backend/internal/service/gemini_chat_completions_compat_service.go`；`backend/internal/service/gemini_messages_compat_service.go`；`backend/internal/service/gemini_vertex_token_error.go`；`backend/internal/service/vertex_service_account.go`。
- 相关测试：`backend/internal/service/gemini_vertex_token_error_test.go`（静态选读；未运行）。
- 验证要求与依赖：当前只fmt.Errorf上抛；测已删除SA不反复调度、临时网络/其它invalid_grant不永久停号、对外不泄露SA错误细节；复用本地credential failover规范。

### 152 `14cd5871752dabfc38069e9a7d87f282da0c9eda`

- 原始标题：fix(gemini): 每个上游错误响应只检查一次错误策略
- 结论：**建议引入**。
- 实际改动、收益与冲突：三条Gemini循环缓存当前响应checkedPolicy，最终处理复用，避免同一个上游错误重复CheckErrorPolicy写冷却/处罚。
- 当前仓库证据：`gemini_messages_compat_service.go:897/:1378`及 `gemini_chat_completions_compat_service.go:130`仍用matched/rebuilt的checkErrorPolicyInLoop；缺checkedPolicy/finalErrorPolicy复用，最终响应会再检查策略。
- 关键改动文件：`backend/internal/service/gemini_chat_completions_compat_service.go`；`backend/internal/service/gemini_messages_compat_service.go`。
- 相关测试：`backend/internal/service/gemini_error_policy_single_check_test.go`（静态选读；未运行）。
- 验证要求与依赖：每次新resp重置缓存，未预检查状态保持nil；测单响应只检查一次、重试新响应仍检查、响应体恢复不丢失。

### 153 `c6c9935ae37035ba69555701e2fb103ab4993f5e`

- 原始标题：feat(gemini): 补齐 Gemini 4 Argon 计费
- 结论：**需适配**。
- 实际改动、收益与冲突：新增Gemini4 Argon优惠价与cyber-permissive非优惠价，thinking tier归一到base；本地billing_service/pricing_service尚无Argon型号匹配。不能因为相同数字出现在其他型号而当覆盖。
- 当前仓库证据：当前 `backend/internal/service/billing_service.go` 与 `pricing_service.go` 搜索gemini-4-argon/cyber-permissive无命中，缺Argon fallback、cyber独立价与thinking-tier归一；资源表也需随型号支持核对。
- 关键改动文件：`backend/internal/service/billing_service.go`；`backend/internal/service/pricing_service.go`。
- 相关测试：`backend/internal/service/gemini_cache_storage_pricing_test.go`；`backend/internal/service/pricing_service_test.go`（静态选读；未运行）。
- 验证要求与依赖：核对cyber先于普通family匹配、资源价格表/priority价和促销截止；保留本地Cybersafety/计价分支。父审merge8afd2abd也修正cyber优先顺序，应采用最终实现。

### 154 `9d3cc4507ff773e8f52e7340f20a5ffcfe8df22f`

- 原始标题：fix(antigravity): keep legacy default passthroughs and bill Claude 5.5 tier IDs at base rates
- 结论：**需适配**。
- 实际改动、收益与冲突：恢复Antigravity旧Gemini默认透传列表，并标记注入条目以免污染上游能力完整度；Claude5.5-high/medium/low归base价格而非旧家族。
- 当前仓库证据：`backend/internal/service/account.go`现有resolveModelMapping已经补齐大部分legacy Gemini默认透传；缺本次isAntigravityInjectedDefaultPassthrough标识和Claude5.5 tier计价归一，需要只补差额。
- 关键改动文件：`backend/internal/service/account.go`；`backend/internal/service/billing_service.go`；`backend/internal/service/pricing_service.go`；`backend/internal/service/upstream_models.go`。
- 相关测试：`backend/internal/handler/gateway_models_test.go`；`backend/internal/service/account_wildcard_test.go`等5个文件（静态选读；未运行）。
- 验证要求与依赖：本地已有默认补齐大部分逻辑；只补缺失的Claude档位计费与能力同步排除，避免重引145删除行为。测原始用户显式映射与注入默认区别、定价精确匹配。

### 155 `64211de8defd7144c1e99f72770278adeae3c2a5`

- 原始标题：feat(gemini): 兼容入口转发思考档位并流式返回思考摘要
- 结论：**需适配**。
- 实际改动、收益与冲突：Gemini兼容请求把2.5 thinkingBudget/3+ thinkingLevel与effort映射，历史reasoning剥离，stream thought独立thinking块/delta返回，并记录ReasoningEffort。
- 当前仓库证据：`backend/internal/service/gemini_compat_thinking.go`不存在；现有Gemini兼容流有部分thinking/signature逻辑，但未实现本次budget/level映射、历史剥离与双协议思考delta完整组合。
- 关键改动文件：`backend/internal/service/gemini_compat_thinking.go`；`backend/internal/service/gemini_messages_compat_service.go`；`backend/internal/service/gemini_chat_completions_compat_service.go`。
- 相关测试：`backend/internal/service/gemini_compat_thinking_test.go`；`backend/internal/service/gemini_native_reasoning_pricing_test.go`（静态选读；未运行）。
- 验证要求与依赖：当前有部分thinking转换但缺gemini_compat_thinking.go；合并现有signature/重试与CC/Claude两套流，测不把思考混入正文、不复述历史、budget边界、非思考模型不注入。

### 156 `2a8cb088af205586b514a27f6a63e9bcf7adb4e6`

- 原始标题：feat(apicompat): configure minimum output token floor
- 结论：**需适配**。
- 实际改动、收益与冲突：输出token floor新增gateway.min_output_tokens默认128下限16，贯穿CC/Anthropic/legacy Responses/WS转换器；兼容旧API函数仍默认128。
- 当前仓库证据：`backend/internal/config/config.go`没有Gateway.MinOutputTokens，`pkg/apicompat`仍使用固定floor；上游新增apicompat_config.go在本地也不存在，配置传播尚未实现。
- 关键改动文件：`backend/internal/config/config.go`；`backend/internal/service/apicompat_config.go`；`backend/internal/pkg/apicompat/anthropic_to_responses.go`；`backend/internal/pkg/apicompat/chatcompletions_to_responses.go`；`backend/internal/service/openai_ws_forwarder_ingress.go`；另有17个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/config/config_test.go`；`backend/internal/pkg/apicompat/chatcompletions_anthropic_bridge_test.go`等3个文件（静态选读；未运行）。
- 验证要求与依赖：本地转换调用多、WS0.164 ingress已有扩展；用户AGENTS反对薄wrapper，不照搬apicompat_config.go四个薄helper，按本地层次最小传参；测所有入口默认不变、显式floor和实际请求。

### 157 `867563ae76f3ff99ed20dfed93ee216a7939ce3f`

- 原始标题：fix(gateway): honor Anthropic protocol base URLs
- 结论：**需适配**。
- 实际改动、收益与冲突：Anthropic出站普通/透传/count_tokens统一优先GetAnthropicProtocolBaseURL，并同源用于认证头和Ollama token clamp。
- 当前仓库证据：`backend/internal/service/account.go:1581`已有GetAnthropicProtocolBaseURL；但gateway_upstream_request.go、gateway_anthropic_passthrough.go与gateway_count_tokens.go仍有GetBaseURL构造点，能力未串入所有出站路径。
- 关键改动文件：`backend/internal/service/gateway_anthropic_passthrough.go`；`backend/internal/service/gateway_count_tokens.go`；`backend/internal/service/gateway_upstream_request.go`。
- 相关测试：`backend/internal/service/gateway_anthropic_base_url_test.go`（静态选读；未运行）。
- 验证要求与依赖：本地已有GetAnthropicProtocolBaseURL但相关构造仍用GetBaseURL；核对多协议配置存在与服务入口，测URL/认证头/计数/clamp同源，避免误传凭据。

### 158 `16611a8ee0b459ee002d50efb5a90d9d06d92ff0`

- 原始标题：fix(openai): isolate Codex imports by workspace
- 结论：**建议引入**。
- 实际改动、收益与冲突：buildCodexStoredIdentityKeys由单user索引改account+user组合，仅无accountID时保留user fallback；长度前缀组合避免键拼接歧义。
- 当前仓库证据：`backend/internal/handler/admin/account_codex_import.go:913` buildCodexStoredIdentityKeys仍无条件按userID加user键；该函数用于导入存量索引并在:956被调用，存在同user不同account/workspace命中同索引的路径。
- 关键改动文件：`backend/internal/handler/admin/account_codex_import.go`。
- 相关测试：`backend/internal/handler/admin/account_codex_import_test.go`（静态选读；未运行）。
- 验证要求与依赖：当前仍按user合并，跨workspace误更新账号风险；测同user异workspace不合并、同workspace更新、access-token-only升级和email退路，保留本地导入额外凭据字段。

### 159 `c2cca510f2dc46f400bd8f9dcc4a5de55859cf51`

- 原始标题：revert(accounts): restore inline account filters (#7853)
- 结论：**不引入**。
- 实际改动、收益与冲突：纯UI回退：账号筛选从折叠更多筛选恢复全部inline，并移除AccountsView布局class。
- 当前仓库证据：本地 `frontend/src/components/admin/account/AccountTableFilters.vue` 与AccountsView已按本地布局维护；这次撤销更多筛选设计没有必须移植的业务修复。
- 关键改动文件：`frontend/src/components/admin/account/AccountTableFilters.vue`；`frontend/src/views/admin/AccountsView.vue`。
- 相关测试：`frontend/src/components/admin/account/__tests__/AccountTableFilters.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：本地过滤布局已经单独维护，没有本次用户体验需求；不接受上游revert覆盖本地UI。

### 160 `7c6f70680d76faf2b8e0a2df3669b8966c410f97`

- 原始标题：fix(payment): validate order types and keep refund deductions while pending
- 结论：**建议引入**。
- 实际改动、收益与冲突：订单类型白名单防未知类型绕充值开关后当余额到账；退款pending禁止PrepareRefund再次发起、保留扣减并在终态处理，余额缓存及时失效。
- 当前仓库证据：`backend/internal/service/payment_order.go:122` validateOrderInput只单独处理balance/subscription，未拒未知类型；`payment_fulfillment.go`未知类型仍进入余额履约；`payment_refund.go`仍允许pending重复准备退款。
- 关键改动文件：`backend/internal/service/payment_fulfillment.go`；`backend/internal/service/payment_order.go`；`backend/internal/service/payment_refund.go`；`backend/internal/service/payment_service.go`。
- 相关测试：`backend/internal/service/payment_order_type_test.go`；`backend/internal/service/payment_refund_pending_hold_test.go`等3个文件（静态选读；未运行）。
- 验证要求与依赖：支付真实资金边界优先；连162/163/164/165评估，测未知订单零到账、pending重复操作不双扣/双退、成功/失败余额与订阅日数一致；本次未真实支付。

### 161 `f996b0bfaae10d0894aaf7edced956805ad4229f`

- 原始标题：fix(payment): load Stripe client secret from the local order snapshot
- 结论：**建议引入**。
- 实际改动、收益与冲突：Stripe client_secret不再写URL，读取本浏览器恢复快照且orderId匹配，币种也沿用该快照。当前StripePaymentView仍信任route.query.client_secret。
- 当前仓库证据：`frontend/src/views/user/StripePaymentView.vue:138`仍用String(route.query.client_secret||'')；URL提供的secret与后续getOrder展示的订单不由同一恢复快照绑定。
- 关键改动文件：`frontend/src/views/user/PaymentView.vue`；`frontend/src/views/user/StripePaymentView.vue`。
- 相关测试：`frontend/src/views/user/__tests__/StripePaymentView.spec.ts`（静态选读；未运行）。
- 验证要求与依赖：跨订单拼接风险；测同订单resume、不同order拒绝、无快照提示与旧链接兼容，确认快照写入在跳转前完成。

### 162 `44e35fbf844059c9937c8d910f3da5fdfe0810dc`

- 原始标题：fix(payment): finalize failed pending refunds atomically
- 结论：**建议引入**。
- 实际改动、收益与冲突：pending退款失败的状态认领、归还扣减、审计在同一数据库事务完成，避免中途进程退出产生已失败但余额未归还且下次再扣。latestRefundPendingDetail读取错误不再吞。
- 当前仓库证据：`backend/internal/service/payment_refund.go:557` finalizeRefundFailed仍是旧流程；QueryAndFinalizeRefund:440读取pendingDetail不返回错误，未具备新事务的状态/归还/审计原子性。
- 关键改动文件：`backend/internal/service/payment_refund.go`。
- 相关测试：`backend/internal/service/payment_refund_pending_hold_test.go`；`backend/internal/service/payment_refund_test.go`（静态选读；未运行）。
- 验证要求与依赖：依160的held deduction语义，测并发终态查询、失败事务回滚、重试不重复归还、缓存失效在成功提交后；不能只按响应success测。

### 163 `83337a0c6547037fbd122a6dcf0f6cc71679db08`

- 原始标题：fix(payment): query Alipay refund status and close unsupported order types
- 结论：**建议引入**。
- 实际改动、收益与冲突：Alipay新增QueryRefund，RefundID保存out_request_no供查询；未知订单类型履约标FAILED而非反复500；当前Alipay无QueryRefund。
- 当前仓库证据：`backend/internal/payment/provider/alipay.go`未实现QueryRefund且RefundID仍不能稳定对应out_request_no；`payment_fulfillment.go`无rejectUnsupportedOrderType终结处理，依160/162改造。
- 关键改动文件：`backend/internal/payment/provider/alipay.go`；`backend/internal/service/payment_fulfillment.go`。
- 相关测试：`backend/internal/payment/provider/alipay_test.go`；`backend/internal/service/payment_order_type_test.go`（静态选读；未运行）。
- 验证要求与依赖：与160/162/164整组；测fund_change=N进入pending、旧RefundID兼容与手工核查、同退款终态幂等、未知订单不入账；不能先引163保留其过早not-found判失败。

### 164 `f37b69b77934ab2479b41178ef65ea337cc18041`

- 原始标题：fix(payment): treat Alipay refund query errors as pending and delay not-found failure
- 结论：**建议引入**。
- 实际改动、收益与冲突：Alipay查询检查SDK业务错误，不将空字段当无退款；只在pending审计时间超过10分钟后无记录才判失败，未知时间继续pending；RefundQueryRequest新增RequestedAt。
- 当前仓库证据：`backend/internal/payment/types.go` RefundQueryRequest没有RequestedAt；`payment_refund.go`查询请求未传审计创建时间，无法实现宽限期不误判未找到。Alipay新增查询本身依163。
- 关键改动文件：`backend/internal/payment/provider/alipay.go`；`backend/internal/payment/types.go`；`backend/internal/service/payment_refund.go`。
- 相关测试：`backend/internal/payment/provider/alipay_test.go`；`backend/internal/service/payment_refund_test.go`（静态选读；未运行）。
- 验证要求与依赖：依163；联动service审计创建时间，测系统错误/早查/未知时间不归还扣减，超过宽限的确无记录才失败。

### 165 `1b92038eaf60674e622dcd90d13d79642c422a6f`

- 原始标题：fix(payment): fix EasyPay order queries and confirm notifications before crediting
- 结论：**建议引入**。
- 实际改动、收益与冲突：EasyPay查询改真实GET参数与严格状态/订单/金额解析；已验签通知在同一商户provider实例主动查询确认后才履约，防假成功通知；return_url限制userinfo/host并清RawPath。
- 当前仓库证据：`backend/internal/payment/provider/easypay.go:308`仍POST api.php表单查询且status解为*int；`payment_webhook_handler.go`验签后没有ConfirmNotificationUpstream主动确认，实际金额/订单真实性仍缺本次验证层。
- 关键改动文件：`backend/internal/payment/provider/easypay.go`；`backend/internal/handler/payment_webhook_handler.go`；`backend/internal/service/payment_fulfillment.go`；`backend/internal/service/payment_resume_service.go`；`backend/internal/payment/types.go`。
- 相关测试：`backend/internal/handler/payment_webhook_handler_test.go`；`backend/internal/payment/provider/easypay_confirm_test.go`等5个文件（静态选读；未运行）。
- 验证要求与依赖：真实支付跨系统边界；核对本地已有通知字段签名防护保留，测异商户/异订单/金额不符/签名正确但未支付均零到账、网络失败让网关重试；只用mock，禁止真实扣款。

### 166 `f2e5b55f3e8069df1c02a9a27af537b5fdefa64e`

- 原始标题：test(responses): 模型改写转发测试夹具补齐上游 created_at
- 结论：**建议引入**。
- 实际改动、收益与冲突：模型改写转发测试response.completed夹具补created_at:1700000000，使其满足实际Responses response字段契约；不改生产逻辑。
- 当前仓库证据：`backend/internal/service/openai_response_model_rewrite_test.go` 对应response.completed夹具仍无created_at；这是单测试数据补全，不改生产出站模型重写。
- 改动范围：仅测试/夹具。
- 相关测试：`backend/internal/service/openai_response_model_rewrite_test.go`（静态选读；未运行）。
- 验证要求与依赖：确认本地该测试响应校验是否要求时间戳；运行针对性模型改写用例时验证，避免用不完整夹具误报转发失败。

### 167 `26aab6dc076059efb919d8b88ce33b2b27fde246`

- 原始标题：fix(openai): respect GPT-5.5 retirement on ChatGPT upstreams
- 结论：**需适配**。
- 实际改动、收益与冲突：新增GPT5.5 ChatGPT OAuth/setup/shadow退役判定，调度/compact fallback/sticky previous_response/model list/Codex catalog统一按最终上游模型过滤；API Key保持支持。
- 当前仓库证据：`backend/internal/service/openai_model_retirement.go`不存在；当前调度/compact/sticky依旧以既有模型支持规则选择，未有本次统一GPT5.5 ChatGPT凭据退役判断，API Key路径需保留。
- 关键改动文件：`backend/internal/handler/gateway_handler.go`；`backend/internal/handler/no_account_error.go`；`backend/internal/service/account.go`；`backend/internal/service/gateway_model_availability.go`；`backend/internal/service/gateway_service.go`；另有10个手写/配置文件，范围未在此穷举。
- 相关测试：`backend/internal/handler/gateway_models_test.go`；`backend/internal/handler/no_account_error_test.go`等4个文件（静态选读；未运行）。
- 验证要求与依赖：与168一起；本地缺retirement helper但有较复杂别名、shadow和WS sticky，不能整文件替换；测试映射替代模型、混合凭据、空非nil目录语义及WS恢复；日期事实本次未外查。

### 168 `86f844304591800be66bafa4454c4a5bc8e02514`

- 原始标题：fix(openai): evaluate retirement after public alias projection
- 结论：**需适配**。
- 实际改动、收益与冲突：模型目录先投影客户端别名再按同路由映射过滤退役模型，避免把上游ID当别名再映射导致误保留或误删元数据；不改共享缓存。
- 当前仓库证据：本地无filterRetiredOpenAIPublicModelsBody；`openai_models_list.go`已有public alias投影，未来退役过滤须接在投影之后，不应从上游ID直接套account alias映射。
- 关键改动文件：`backend/internal/service/openai_codex_models_service.go`；`backend/internal/service/openai_model_retirement.go`；`backend/internal/service/openai_models_list.go`；`docs/OPENAI_GPT55_RETIREMENT.md`。
- 相关测试：`backend/internal/service/openai_model_retirement_test.go`（静态选读；未运行）。
- 验证要求与依赖：依167；测上游ID同时为alias键、alias→替代/退役模型、Codex与普通列表、ETag在最终过滤后计算。

### 169 `26e55cf332e73ae25ae0e17f1e0ef90bcbf03e54`

- 原始标题：fix(openai): emulate web_search on /v1/messages for OpenAI API Key accounts
- 结论：**需适配**。
- 实际改动、收益与冲突：将Anthropic纯web_search仿真判断抽共享函数，在OpenAI API Key /v1/messages先执行并写Anthropic响应，前端允许账户开关；本地有专门Responses搜索桥且旧函数明确Anthropic-only。
- 当前仓库证据：`backend/internal/service/gateway_websearch_emulation.go` shouldEmulateWebSearch明确限制PlatformAnthropic；`openai_gateway_messages.go`未调用本次shouldEmulateWebSearchFor，OpenAI API Key Messages仿真入口未覆盖。
- 关键改动文件：`backend/internal/service/account.go`；`backend/internal/service/gateway_websearch_emulation.go`；`backend/internal/service/openai_gateway_messages.go`；`frontend/src/components/account/EditAccountModal.vue`。
- 相关测试：`backend/internal/service/account_websearch_test.go`；`backend/internal/service/gateway_websearch_emulation_test.go`（静态选读；未运行）。
- 验证要求与依赖：需按本地OpenAI消息dispatch接入，不扩大到普通多工具请求；连170确保OAuth保原生，测账单/日志/stream结果一致、Key/渠道开关优先级；保留本地搜索产品策略。

### 170 `7de15113c77cf6d3c431b1840c6997d561cc634d`

- 原始标题：fix(openai): exclude OAuth accounts from web search emulation
- 结论：**需适配**。
- 实际改动、收益与冲突：共享shouldEmulateWebSearchFor显式排除OpenAI OAuth/setup-token，避免渠道开启仿真后抢走原生搜索。
- 当前仓库证据：当前shouldEmulateWebSearch只在Anthropic路径且显式限制平台，已经排除旧入口的OpenAI OAuth；若按169新接OpenAI Messages入口，必须新增API Key类型限制，不能把这一条误称没有生产改动。
- 关键改动文件：`backend/internal/service/gateway_websearch_emulation.go`。
- 相关测试：`backend/internal/service/gateway_websearch_emulation_test.go`（静态选读；未运行）。
- 验证要求与依赖：依169；本地旧Anthropic-only判断已防旧入口OAuth，但新OpenAI入口仍必须有同样分流，测试OAuth与API Key同渠道差异。

## 检查点

已完成85/85条静态取舍；需适配41，建议引入36，不引入7，已覆盖1。

没有运行测试、构建、安全扫描或迁移；没有访问真实账号、远程支付或其它远程服务，没有cherry-pick/stage/commit。日期、价格、退役与依赖安全事实仅依据补丁记载，采用前还需核实官方当前资料。外围调用方未穷尽，移植建议不是集成验收结论。

只修改本评审文件；DEV_GUIDE.md及deploy中的用户未提交文件保持不动。