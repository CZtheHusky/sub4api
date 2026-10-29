# 合并提交检查（150 项）

使用 `git show --remerge-diff <SHA>` 将每个双亲合并重新合并，并对照当时实际保存的结果。150 项全部成功重建；143 项差异为空，7 项存在额外的冲突解决或合并后修改。空差异表示在本机 Git 合并算法下没有额外代码，不把它重复计算为独立移植候选；其父提交中的代码仍由三个普通提交批次评审。

## 有额外差异的 7 项

### `5800abf1cb8fd30bea2273b47e2d72b2738cd1a7` — 需适配

`gateway_handler.go` 的 `compositeAvailableModels` 与 `codexModelIDsForGroup`，配合 `CompositeRouteResolver.ListExactPublicModels/FilterCodexModels`：不仅列出 exact route 别名，还按生产 `matchCompositeRoute` 的 endpoint、exact/prefix、priority、ID 顺序，排除有效目标为 TypeSafe 的 Codex 目录条目。增加路由覆盖账号别名、默认回退、白名单和查询失败的测试。

本地 `CompositeRouteResolver` 只有 `Resolve`，目录没有普通 exact route 汇入及 TypeSafe 路由过滤，存在目录与真实路由不一致的缺口；结合普通提交 `1888de135` 移植有价值。不能直接覆盖本地 handler：本地返回 `([]string, error)`，并接入 `CompleteCompositeBPSModelCatalog`，使用 nil/非 nil 空切片区分回退；BPS 显式路由和错误传播必须保留。验证 exact/prefix 优先级、TypeSafe 隔离及 BPS 空目录/查询失败。

### `07d51a6aeff41824730aa8276afef150e3d03a03` — 不引入合并本身

`frontend/package.json`/锁文件的冲突解决保留 Vue 3.5.43，但将 SheetJS 0.20.3 URL 依赖退回 npm `xlsx ^0.18.5`，并恢复相应传递依赖；另把服务 VERSION 改为 0.2.14。本地仍为 `xlsx ^0.18.5`，Vue 声明 `^3.4.0`。不能因“sync upstream security updates”标题就整包复制；应分别评审依赖补丁，核对最终 lock，而非只看包声明。这里识别版本回退，不声称本轮已验证漏洞公告或运行依赖审计。

### `b68684271670b4b301abb9ca61a8fd41750c9448` — 需适配（随 structured-input fallback 系列）

`openai_gateway_forward.go` 冲突解决选择统一 `resolveUpstreamProtocol` 分流，保留调用 raw Chat 转发时传递 `startTime`；测试补充 Kimi adaptive、Zhipu Responses→Chat、CommandCode 路由时不得扩大 OpenAI 专用重试范围。本地仍使用 `shouldForwardOpenAIResponsesViaRawChatCompletions`，没有这套 provider profile 或 structured-input fallback；不是独立小修复。若引入 `983cf297d/c15d858ec/63800c50d`，必须参考这些排除场景，不能把 OAuth、原生 Responses 或跨供应商请求都重试一次。

### `9d5e63a598c827dde2d8b864ac7719125615bd82` — 不独立引入

只解决 `EditAccountModal.spec.ts` 的测试块冲突：将 Gemini 显式缓存上游开关和临时不可调度 scope 两组 describe 各自完整保留，分别初始化 mock。本地尚无这两个功能的完整上游实现。选择移植对应功能时移植有效断言，不能只合并测试块；不产生独立生产行为。

### `8afd2abd979fa4bf657a73cbf0900bcebd95e812` — 需适配（随 Gemini 4 系列）

`BillingService` 保留 Argon 基础价，并让 Cyber 专用价卡判断先于宽泛 `gemini-4-argon` 匹配；资源价表保留 cyber-permissive。`pricing_service_test.go` 删除重复加入的三组 Argon 测试，`pricing_service.go` 仅注释格式选择。本地价表/计费 fallback 无 Argon，适配时应从 `c6c9935ae/373d43533/feab6d80d` 系列整体选择必要代码，保留足够测试；不要孤立移植“删除测试”。金额是提交中的值，本轮没有复核外部官方实时价格。

### `a431e53f9e3da9097b1b9defc1f118553bd5b4fa` — 不独立引入

`GeminiMessagesCompatService.ForwardAsChatCompletions` 同时保留清空历史 `ReasoningContent`/相关 reasoning 状态的逻辑，以及新的 `chatCompletionsToResponses(s.cfg, &ccReq)` 调用。前者属于 Gemini 思考兼容 `64211de8d`，后者属于最小输出 token 配置 `2a8cb088a`。本地直接调用 `apicompat.ChatCompletionsToResponses`；只有选择这两个系列后才需要该组合，不单独移植 merge。

### `3f4e13aaba9be9c714711ef5b897af335fb0d7ff` — 不独立引入

`billing_service.go` 将 DeepSeek `getModelPricingAt` 的说明放回函数前，同时保留 Gemini 显式缓存 `CacheStoragePricePerTokenHour` 方法。额外差异是冲突标记清除与注释位置，没有独立于父提交的新计费逻辑；DeepSeek 价格修复看 `5db2d6956`，显式缓存看批次二。

## 完整游标

以下逐项对应本轮所有合并提交，顺序为 Git 逆时间遍历。父 SHA 可通过 `git show --no-patch --format=%P <SHA>` 查询。

| SHA | 标题 | 重建检查 |
| --- | --- | --- |
| `a3298b46ebe239fb0720b068a1eb3383955bdac2` | Merge pull request #7979 from ShuminFu/fix/oauth-passthrough-non-stream-7978 | 差异为空；父提交另审 |
| `dece26153621959ccd7635814f4b2d08304e681a` | Merge pull request #8373 from feeeei/fix/openai-models-cache-singleflight-recheck | 差异为空；父提交另审 |
| `184193f01ea0c681e2ab31c238c067aa9e7f34a6` | Merge pull request #8369 from feeeei/fix/gemini-bound-cache-pool-retry | 差异为空；父提交另审 |
| `7311b865f327e5e69f16e40a3a4236c08da2048b` | Merge pull request #8287 from feeeei/feat/gemini-same-account-retries-config | 差异为空；父提交另审 |
| `bf02c1a6e0f78816b728e1e16854afa5c8fabe6b` | Merge pull request #8395 from feeeei/fix/gemini-model-lists | 差异为空；父提交另审 |
| `8b3f33f28ae3f2e5707b810eca187a3b40780638` | Merge pull request #8390 from wc4440222/pr/fix-websearch-emulation-dimensions | 差异为空；父提交另审 |
| `b2e7c609430736bd0a4b89c5b27a079ccef820d4` | Merge pull request #8352 from ychy7001/fix/admin-guard-400 | 差异为空；父提交另审 |
| `f178e5ae32659e8901644a9f2378a0439ffa36ae` | Merge pull request #8362 from lkon2112/fix/nonstream-malformed-function-call | 差异为空；父提交另审 |
| `449b65a2ed9ae43c8438105b41c500619253090f` | Merge pull request #8348 from SipengXie2024/fix/images-upload-part-limit | 差异为空；父提交另审 |
| `2c9d9760d8d60bc403d50d3d9b56d77c56164adf` | Merge pull request #8303 from SipengXie2024/fix/pagination-offset-overflow | 差异为空；父提交另审 |
| `af4462ebaef34d68c1e73e8622c4e195648fef75` | Merge pull request #8376 from fxjs/fix/opencode-deepseek-developer-role | 差异为空；父提交另审 |
| `8e231ff5d66afb5789f5a876872d2c8867828e82` | Merge pull request #8354 from ychy7001/fix/deepseek-cache-hit-tokens | 差异为空；父提交另审 |
| `98bcee7a8934220215268cccb80f15f3cb16091b` | Merge pull request #8366 from wx-11/fix/model-plaza-group-pricing | 差异为空；父提交另审 |
| `3f1ae7ee76aecd8d5066e6549690a7cd66e471f2` | Merge pull request #8003 from Yukinoshita-Yukino/codex/fix-deepseek-pro-pricing | 差异为空；父提交另审 |
| `bb021d330e2067687d6362dc48991e63b3e56d6c` | Merge pull request #8326 from r266-tech/codex/gemini-file-images | 差异为空；父提交另审 |
| `3bd166babc300281f1830e6d7ce24256265a1d20` | Merge pull request #8377 from StarryKira/codex/fix-cc-version-utf16 | 差异为空；父提交另审 |
| `0232cb7137b840926e2dd23c62f1bee1c8557071` | Merge pull request #8286 from coder-zhw/fix/cn-frequency-429-short-cooldown | 差异为空；父提交另审 |
| `15701ba187af5827d740901d53421bd69ed97717` | Merge pull request #8339 from aomsir/fix/grok-model-catalog-audit | 差异为空；父提交另审 |
| `b332f169f9c6c10722016ff7a09bc9c9658a3152` | Merge pull request #8364 from wucm667/feat/issue-8363-gpt61-terra | 差异为空；父提交另审 |
| `ba81c7fd758e425d67f856d601147ef0c5263d3f` | Merge pull request #8323 from wx-11/images-price | 差异为空；父提交另审 |
| `37067e8e340e9440d173a92b4faa05e8ce58368f` | Merge pull request #8319 from lyen1688/feat/scoped-admin-api-keys | 差异为空；父提交另审 |
| `c5b811284aea2c4922561da2f0a78256334b23df` | Merge pull request #8305 from apple050620312/fix/openai-overload-recovery-20261023 | 差异为空；父提交另审 |
| `dba770d56a0ee0ae5bad1b3dc5e11614d323163c` | Merge pull request #8170 from StarryKira/fix/issue-8119-grok-fast-pricing | 差异为空；父提交另审 |
| `21cc926933163e015d2b632be07055759c983541` | Merge pull request #8275 from apple050620312/fix/docker-go-1.27.3 | 差异为空；父提交另审 |
| `aa48a8bbd80c6ac956dbcbaba651f417b3cf78e3` | Merge pull request #8255 from wucm667/fix/issue-8248-cloudflare-json-403 | 差异为空；父提交另审 |
| `888c6057333e011b8cfeb81a867f445d229d1432` | Merge pull request #8188 from wucm667/fix/issue-8187-tool-schema-constants | 差异为空；父提交另审 |
| `5e45fa482b186145b84ddb2d79bc93a509dba1d2` | Merge pull request #8224 from wucm667/fix/issue-8222-deepseek-image-size | 差异为空；父提交另审 |
| `624874b313241c46a34c68eb843c021c7dbdacaf` | Merge pull request #8269 from feeeei/perf/account-quota-increment-variants | 差异为空；父提交另审 |
| `a34f373973bf547ccec301c7405eb45194e1f28a` | Merge pull request #8251 from feeeei/perf/scheduler-snapshot-memo | 差异为空；父提交另审 |
| `b3e66f79a81400c729dbea78183f150509478343` | Merge pull request #8250 from feeeei/perf/model-mapping-signature | 差异为空；父提交另审 |
| `b6c366380bd2639c939871a2cc8663cb4f5db32e` | Merge pull request #8249 from feeeei/perf/lazy-single-antigravity-probe | 差异为空；父提交另审 |
| `b7512305123aff307d3ab92840f2077eabc2529f` | Merge pull request #8210 from feeeei/fix/anthropic-compat-stream-disconnect-billing | 差异为空；父提交另审 |
| `98f83b7263227cea945652e29759a5b7df9f92fb` | Merge pull request #8167 from feeeei/fix/gemini-group-image-generation-gate | 差异为空；父提交另审 |
| `4c7e9b5b6b0664e12fbc5a69efffc06e7a363c6c` | Merge pull request #8166 from feeeei/feat/vertex-account-quota | 差异为空；父提交另审 |
| `9a1680042a2315df7fcfa925f9235bfead528ad6` | Merge pull request #8267 from wucm667/fix/issue-8266-security-scan | 差异为空；父提交另审 |
| `d744c22016f15f37191d90fe8850a66ccb730399` | Merge pull request #8142 from qhmhyp/fix/account-filter-persistence | 差异为空；父提交另审 |
| `4942b2afc70be7291df28bce594eeef21b71e336` | Merge pull request #8139 from zenggang/contribute/openai-preoutput-metadata-20261016 | 差异为空；父提交另审 |
| `8f2cbb53d9d1c40e1865b07ac460571f7d4c48de` | Merge pull request #8177 from feeeei/fix/gemini-negative-input-tokens | 差异为空；父提交另审 |
| `0fa02816afcaea3610171dd229dd910c058c47e4` | Merge pull request #8168 from feeeei/fix/gemini-stream-client-disconnect-billing | 差异为空；父提交另审 |
| `378984d87f2b68db15626ae8f18b9e923ab82b85` | Merge pull request #8223 from Wei-Shaw/fix/async-image-download-policy | 差异为空；父提交另审 |
| `3f4e13aaba9be9c714711ef5b897af335fb0d7ff` | Merge upstream main and resolve pricing comment conflict | 有额外差异，见上文 |
| `060d0160417782cd962a0c7f460f6cd4565d4916` | Merge pull request #8140 from Wei-Shaw/fix/issue-8118-responses-created-at | 差异为空；父提交另审 |
| `b14f316ca2def2fbf379a7413c3f4309cc70acdd` | Merge pull request #8080 from SipengXie2024/fix/openai-messages-web-search-emulation | 差异为空；父提交另审 |
| `43157c478ec29cb765043e2f0cba4f69728dd2f2` | Merge pull request #8121 from Larry-Labs/fix/gpt55-subscription-retirement | 差异为空；父提交另审 |
| `455d317d6a87d8519a52c8769a0727824c403171` | Merge pull request #8114 from feeeei/feat/gemini-compat-thinking | 差异为空；父提交另审 |
| `a431e53f9e3da9097b1b9defc1f118553bd5b4fa` | Merge remote-tracking branch 'origin/main' into pr-8114 | 有额外差异，见上文 |
| `b8cfb7884084393e08f0e2c4027afc3755357b98` | Merge pull request #8133 from Wei-Shaw/fix/easypay-query-confirm | 差异为空；父提交另审 |
| `52b3ca960960cc755bcb73172a99e7394f0e50dc` | Merge pull request #8122 from Wei-Shaw/fix/payment-order-type-refund-hold | 差异为空；父提交另审 |
| `1b2243a4f50621508e79aef4b498a2937a6238ee` | Merge pull request #8120 from Wei-Shaw/revert/inline-account-filters | 差异为空；父提交另审 |
| `698d142468ab15374bc22bd5247759e5b7352006` | Merge pull request #8088 from wucm667/fix/issue-8077-workspace-aware-import | 差异为空；父提交另审 |
| `ddef05527eca7f369b107fd87c4811d696284f38` | Merge pull request #8079 from wucm667/fix/issue-8074-anthropic-base-url | 差异为空；父提交另审 |
| `58b5e858c82dcf14a637762ee1e6c37ec9f86aad` | Merge pull request #8094 from wucm667/feat/issue-8092-configurable-min-output | 差异为空；父提交另审 |
| `857a76d4a0e7b2389926c340555a2f8f5b24a0ae` | Merge pull request #8093 from baryon/contrib/gemini-4-argon-support | 差异为空；父提交另审 |
| `8afd2abd979fa4bf657a73cbf0900bcebd95e812` | Merge remote-tracking branch 'origin/main' into contrib/gemini-4-argon-support | 有额外差异，见上文 |
| `dcde68aa5881ba7eea569ff1d98a860fba1d8b8c` | Merge pull request #8078 from feeeei/feat/gemini-4-argon-pricing | 差异为空；父提交另审 |
| `abdc82725f1c6f1530d2a5835b0af653d352994e` | Merge pull request #8076 from feeeei/fix/gemini-vertex-sa-and-error-policy | 差异为空；父提交另审 |
| `3b83e045ed5ddc8246ebe8b3c4045a1c7ff5968b` | Merge pull request #8060 from feeeei/fix/gemini-vertex-shared-capacity-429 | 差异为空；父提交另审 |
| `fdc7f1fa011a912afb9f7948cb9dc46059c98ddd` | Merge pull request #8087 from coder-zhw/fix/zhipu-credit-limit-window-classification | 差异为空；父提交另审 |
| `3856a02856ca788cc1714362b1dc8a5a4d06b131` | Merge pull request #8106 from Wei-Shaw/fix/composite-oauth-ambiguous-misroute | 差异为空；父提交另审 |
| `01842576b3d5c1e2545b5a99ad555d36e28ed306` | Merge pull request #7810 from wucm667/fix/issue-6247-responses-chat-fallback | 差异为空；父提交另审 |
| `49b2122dc623f7f2face4a2f89dbd6bceffb781a` | Merge pull request #7991 from wucm667/fix/issue-7988-international-quota | 差异为空；父提交另审 |
| `8d7d3185e987f0241f4cd99c2bc2b220378b4860` | Merge pull request #8038 from wucm667/fix/issue-8035-gemini-tool-result-images | 差异为空；父提交另审 |
| `bba85923b96c277b9cbe4a2d21c661ff741c7e29` | Merge pull request #8046 from wucm667/fix/issue-8040-antigravity-image-rate-limit | 差异为空；父提交另审 |
| `49dc6e236380a7198e0658054b0d7c595b597387` | Merge pull request #7828 from feeeei/feat/temp-unsched-rule-scope | 差异为空；父提交另审 |
| `9d5e63a598c827dde2d8b864ac7719125615bd82` | 合并 main 并解决 PR #7828 的测试文件冲突 | 有额外差异，见上文 |
| `53c21fed252cec522035cfc2d4dd890a8c47140b` | Merge pull request #8006 from feeeei/feat/gemini-explicit-cache | 差异为空；父提交另审 |
| `7c5afa4c361b7b69afeb9aaf5b47c0bbe9af0180` | Merge pull request #7957 from xcrong/feat/haiku-5-5-support-7919 | 差异为空；父提交另审 |
| `ab80abc6d1f929a6864c8804fd077b509d21fda2` | Merge pull request #7960 from feeeei/feat/group-i18n | 差异为空；父提交另审 |
| `9591136d45cef26453348ed9532a7ebbce7cf99c` | Merge pull request #8033 from Larry-Labs/fix/gpt61-sol-500k-context | 差异为空；父提交另审 |
| `7b03b1f75a659c4b63f89948a16e04bb00a38180` | Merge pull request #8036 from Wei-Shaw/fix/gpt61-sol-subscription-long-context | 差异为空；父提交另审 |
| `e468262294eb962206ce2d6e4b83f2e8bc43ef02` | Merge pull request #8002 from obst4cle/fix/group-rpm-override-stale-load | 差异为空；父提交另审 |
| `5dbb8f7085936def28c9db74a8e6fa0a56086b93` | Merge pull request #7649 from Yukinoshita-Yukino/fix/codex-image-bridge-openai-only | 差异为空；父提交另审 |
| `6d509c632d1e03351ae3dacc84fb8dc4aee4f709` | Merge pull request #7711 from wucm667/fix/issue-7709-risk-control-groups | 差异为空；父提交另审 |
| `6a6d4728a093022c08070046151dec993d11b825` | Merge pull request #7696 from a408665321/codex/issue-7687-timer-only | 差异为空；父提交另审 |
| `832677a5e6e817f3589130d568a40384f7893228` | Merge pull request #7717 from ouchihao/codex/bug-payment-countdown-drift | 差异为空；父提交另审 |
| `c2885c430fe01d0d88b23d52e062ccbb073d0905` | Merge pull request #7738 from r266-tech/codex/monitor-add-platform | 差异为空；父提交另审 |
| `59fb22791ea7e5987312750cc2ce8bbd91ec8a65` | Merge pull request #7933 from yan9651688/feat/issue-7926-ops-details-account-column | 差异为空；父提交另审 |
| `20b0be5b05c22730db78747fe49ce7e898040d0c` | Merge pull request #7969 from ouchihao/codex/bug-ops-custom-timezone | 差异为空；父提交另审 |
| `d70265306bb21d0194cf8f668d80d808539ffaec` | Merge pull request #7970 from ouchihao/codex/bug-select-ime-navigation | 差异为空；父提交另审 |
| `f2b1556bd4136d3d40fb582737fb1298ec37e72f` | Merge pull request #7972 from ouchihao/codex/bug-group-rate-edit-validation | 差异为空；父提交另审 |
| `0ae1b371fae6d399eabe6391bd4fc675549549d6` | Merge pull request #7973 from ouchihao/codex/bug-monitor-auto-detail-refresh | 差异为空；父提交另审 |
| `da9799542db251badcee0c6cfc08f85adfcab8cd` | Merge pull request #7974 from ouchihao/codex/bug-gemini-auth-url-error | 差异为空；父提交另审 |
| `6977bb63bb1a20a8f70c57df4f1691f83e2c55a4` | Merge pull request #7983 from hutututtt/fix/account-group-names | 差异为空；父提交另审 |
| `b63dafa426a721562db4c5b0d91b9129098f3d8b` | Merge pull request #7982 from wucm667/fix/issue-7981-embedded-static-asset | 差异为空；父提交另审 |
| `b68684271670b4b301abb9ca61a8fd41750c9448` | fix(gateway): sync main routing without widening responses retry | 有额外差异，见上文 |
| `f2669c8cf62555cd92389b3f55920e9e6e7c6ff2` | Merge pull request #7939 from IrisRainbowNeko/fix/issue-7927-web-search-history-tool | 差异为空；父提交另审 |
| `d0392edf066fb0b308c8e1212edfb80539321973` | Merge pull request #7618 from SavitarC/feat/provider-profile | 差异为空；父提交另审 |
| `5fc0e486c3f6a8a191b8bd140f39b60457f611cf` | Merge pull request #7916 from wucm667/fix/issue-7914-monitor-request-sample | 差异为空；父提交另审 |
| `3e69643392e7ec45bbfd3402a245d397d4b79cb8` | Merge pull request #7721 from ouchihao/codex/bug-fractional-byte-rate | 差异为空；父提交另审 |
| `3a012ac0cee4e54e747f0bb5cf292c4883718d94` | Merge pull request #7725 from ouchihao/codex/bug-select-disabled-open | 差异为空；父提交另审 |
| `80dd7288a61af1bc64cbf479b88295c265b9ba6d` | Merge pull request #7728 from ouchihao/codex/bug-oauth-code-unmount | 差异为空；父提交另审 |
| `5a6e826f83f8f6a515b216a3cf9de157f528a3a1` | Merge pull request #7716 from ouchihao/codex/bug-scaled-price-exponent | 差异为空；父提交另审 |
| `d068b866f3e12ecdd2a49040b242e47589ac00bd` | Merge pull request #7718 from ouchihao/codex/bug-bulk-user-error-message | 差异为空；父提交另审 |
| `a61833898763b2b52e211fd6c7b64b3c83d7b985` | Merge pull request #7719 from ouchihao/codex/bug-notify-email-timers | 差异为空；父提交另审 |
| `b0c2c4d623df46e2641493cb81910771d1ed82b8` | Merge pull request #7720 from ouchihao/codex/bug-usage-zero-rate | 差异为空；父提交另审 |
| `9663b5ccf05a0f2efefceef96727170e36042260` | Merge pull request #7722 from ouchihao/codex/bug-announcement-read-failure | 差异为空；父提交另审 |
| `6cfa0f4ef842770b51c3c4616c6996f63ed34409` | Merge pull request #7723 from ouchihao/codex/bug-batch-image-access-retry | 差异为空；父提交另审 |
| `3149d6285178b9f55d5e651f1787f68d6b36afca` | Merge pull request #7724 from ouchihao/codex/bug-cleanup-task-race | 差异为空；父提交另审 |
| `0b51cbe15d1a5ac21b35e7c125cd464306410503` | Merge pull request #7729 from ouchihao/codex/bug-backup-polling-unmount | 差异为空；父提交另审 |
| `3dc2a9c371dc8e0a4abc6e145399615a09c63b37` | Merge pull request #7731 from ouchihao/codex/bug-dialog-escape-stack | 差异为空；父提交另审 |
| `2c13fb595eafa5c36c6c89a79a2445c78c6d1f45` | Merge pull request #7764 from ouchihao/codex/bug-plugin-config-session-race | 差异为空；父提交另审 |
| `068239305ede5f7f4548d421485921f8438958c7` | Merge pull request #7765 from ouchihao/codex/bug-compliance-reset-race | 差异为空；父提交另审 |
| `9e46f2d77ec87f5230f04dc01e53a750ce5ce232` | Merge pull request #7732 from ouchihao/codex/bug-step-up-concurrent-prompts | 差异为空；父提交另审 |
| `2eb7ccb6d258b6e75a7b849edb0119641d9dc024` | Merge pull request #7703 from wucm667/fix/issue-7700-thinking-signature | 差异为空；父提交另审 |
| `745ff1d3ca79a7f39520f627f6aa44d778a11580` | Merge pull request #7911 from lyy0709/codex/usage-output-tps | 差异为空；父提交另审 |
| `8e2c4fad321fe8ce86d577dd7561c371bf602647` | Merge pull request #7793 from wucm667/fix/issue-7784-composite-route-model-list | 差异为空；父提交另审 |
| `0ac774e3785aa2bd3a4d43808f216fcaddd88130` | Merge pull request #7853 from wucm667/feat/issue-7847-compact-account-filters | 差异为空；父提交另审 |
| `ba3af61251d37480e9cb124e1006b0003b8c81d4` | Merge pull request #7897 from wucm667/fix/issue-7894-long-context-marker | 差异为空；父提交另审 |
| `48c647b5d2ef83301de61e4301b1bb12e2f0207b` | Merge pull request #7833 from ShanYi-Hui/fix/preserve-email-binding-draft | 差异为空；父提交另审 |
| `b94974467b418a93638e808fa4797ee45ba200c8` | Merge pull request #7818 from ouchihao/codex/bug-custom-page-request-race | 差异为空；父提交另审 |
| `4747827e355c35f9c62b2ba6b8f1969273d49a46` | Merge pull request #7819 from ouchihao/codex/bug-stripe-payment-unmount | 差异为空；父提交另审 |
| `f3bcad3c3352eb21b351381131dc3cf1bc3373f2` | Merge pull request #7839 from ouchihao/codex/bug-scheduled-results-race | 差异为空；父提交另审 |
| `2c6bd63de92769d55315d5bd9463abd0d2b1920f` | Merge pull request #7840 from ouchihao/codex/bug-monitor-template-list-race | 差异为空；父提交另审 |
| `02aad478f39961619fcbdd8371e33e13f2b020af` | Merge pull request #7841 from ouchihao/codex/bug-alert-events-pagination-race | 差异为空；父提交另审 |
| `486b226b69075bf7b69b0b24f0e612c0ddb07d32` | Merge pull request #7842 from ouchihao/codex/bug-ops-settings-load-guard | 差异为空；父提交另审 |
| `d1202bea5fa8f3dca267cd7ffaa7e2678d9b8b00` | Merge pull request #7843 from ouchihao/codex/bug-alert-rule-integer-duration | 差异为空；父提交另审 |
| `1865f28469e8c66903bfb2c17ccaf31da302b41a` | Merge pull request #7900 from ouchihao/codex/bug-rpm-edit-number | 差异为空；父提交另审 |
| `c2f0edc13716876aeeb316bdf83dcf8848fb2181` | Merge pull request #7901 from ouchihao/codex/bug-airwallex-navigation | 差异为空；父提交另审 |
| `cdbf1d6f4d1f3145ad34697ddb830babdf572f5f` | Merge pull request #7902 from ouchihao/codex/bug-passthrough-toggle-state | 差异为空；父提交另审 |
| `b126b94525f13da89233a01804d864e28adfd97d` | Merge pull request #7903 from ouchihao/codex/bug-profile-username-draft | 差异为空；父提交另审 |
| `7fc5332b45e1ad462b1aa0bba9ca7ae595bb26e3` | Merge pull request #7904 from ouchihao/codex/bug-group-rate-load-race | 差异为空；父提交另审 |
| `4bcad0657b37ecbf127e5cfd886ea7173157426b` | Merge pull request #7800 from duckducktw/fix/pricing-entry-autofill-append | 差异为空；父提交另审 |
| `7ff5cff1bb525faa6c4d35244b72505e03e336ea` | Merge pull request #7822 from neotan/fix/issue-7670-bare-api-alias-bypass | 差异为空；父提交另审 |
| `bc63bd2874d4d77ed39859cd5399d514b53674d5` | Merge pull request #7782 from lyy0709/codex/fix-remote-model-catalog | 差异为空；父提交另审 |
| `458865ed1be1d3aec9408d8a38167603e1922219` | Merge pull request #7878 from Achordchan/fix/account-test-log-context | 差异为空；父提交另审 |
| `3a7fd4ae786f129d9ebfa9d385d3405b935220dd` | Merge pull request #7837 from yJader/fix/antigravity-account-test-models | 差异为空；父提交另审 |
| `4b8c5577050f0a17e348b9bf9a6ef1575cdc24f3` | Merge pull request #7877 from hezhaoqian1/fix/upstream-channel-monitor-zhipu-path | 差异为空；父提交另审 |
| `bb0d4a53c9aada57a8effbdd3f3d31d78ee31550` | Merge pull request #7887 from whhphd/fix/grok-sso-consent | 差异为空；父提交另审 |
| `3c274768c19c73e7403ff27f1d7a7f3b99b8f890` | Merge pull request #7889 from aishangwuji/fix/opencode-zen-routing-and-unsupported-models | 差异为空；父提交另审 |
| `bbf1d37783d37d58edc8b9c07ffec53b075e4de9` | Merge pull request #7891 from aishangwuji/fix/opencode-go-usage-retry-after-and-403 | 差异为空；父提交另审 |
| `90afaf120e311c8b81f0732999b141457bb81fea` | Merge pull request #7854 from wucm667/fix/issue-7852-chat-stream-heartbeats | 差异为空；父提交另审 |
| `6cacccc767f8294c127888c431e430a5b2a22658` | Merge pull request #7775 from YahooYuan666/fix/grok-empty-completed-silent-refusal | 差异为空；父提交另审 |
| `2afe7d45777723d069b0f74e3cac2ef313539cf8` | Merge pull request #7910 from liuyh06/codex/fix-responses-namespace-noop | 差异为空；父提交另审 |
| `129369204eaccac9a0ee3ea6f4ea0b35be7e4130` | Merge pull request #7827 from Achordchan/fix/anthropic-tool-change-betas | 差异为空；父提交另审 |
| `f19cd4fdea288984908ce1c9744dc2ddb7d663d7` | Merge pull request #7844 from cyhhao/fix/anthropic-cache-ttl-order | 差异为空；父提交另审 |
| `e473fba99cafe5dad076e0f5a9df6df3bca10ee5` | Merge pull request #7869 from 3219378872/fix/thinking-signature-key | 差异为空；父提交另审 |
| `cc4a1e3f616cd50491cb7987dfb0459cae380197` | Merge pull request #7789 from SPHINX998/fix/anthropic-buffered-negative-block-index | 差异为空；父提交另审 |
| `9d49d275f34a6859b822f51ce314eb29d74e5273` | Merge pull request #7788 from SPHINX998/fix/apicompat-responses-refusal-to-chat | 差异为空；父提交另审 |
| `8d504668099c6bc1447c713395430a4b479d605e` | Merge pull request #7787 from SPHINX998/fix/apicompat-chat-legacy-function-call | 差异为空；父提交另审 |
| `f9bd97cc157959df1fe1b737dad5d8473dece35c` | Merge pull request #7786 from SPHINX998/fix/apicompat-chat-developer-role | 差异为空；父提交另审 |
| `f087fdb205315838a761cd62ce623e21ae34254b` | Merge pull request #7785 from SPHINX998/fix/apicompat-chat-named-tool-choice | 差异为空；父提交另审 |
| `ef16f8a3d1ca6fbcb06332c652c91223f4caabb4` | Merge pull request #7792 from SPHINX998/fix/ws-passthrough-image-usage | 差异为空；父提交另审 |
| `6db4171cfb7592ee6f81c29a82e9fcba0077155e` | Merge pull request #7779 from SPHINX998/fix/ws-turn-group-pricing | 差异为空；父提交另审 |
| `db29b3a7738e26cfbc30025f033f481ac365ab7a` | Merge pull request #7811 from kingsleydon/fix/codex-service-tiers-null-fallback | 差异为空；父提交另审 |
| `df3c38e10f80deb226f61c9fd3cc0c818b185bb4` | chore: align branch with current upstream main | 差异为空；父提交另审 |
| `07d51a6aeff41824730aa8276afef150e3d03a03` | chore: sync upstream security updates | 有额外差异，见上文 |
| `0363b8cdba8cec3e2ba4b2dbd49c4481143fa55d` | Merge pull request #7893 from Wei-Shaw/fix/issue-7850-admin-bootstrap-hardening | 差异为空；父提交另审 |
| `d97ccc952ff3b7f63b44c45079ee6a28422f1393` | Merge pull request #7888 from honestTai/fix/easypay-callback-forgery | 差异为空；父提交另审 |
| `33d9e72f9098baa58d3562fc632e26578cdbe9a9` | Merge pull request #7851 from ZeroClover/fix/remote-model-catalog-discovery | 差异为空；父提交另审 |
| `5800abf1cb8fd30bea2273b47e2d72b2738cd1a7` | fix(models): preserve composite catalog isolation after base update | 有额外差异，见上文 |
