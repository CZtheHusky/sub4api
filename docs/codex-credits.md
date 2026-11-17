# Codex 点数（credits）与额度接管

OpenAI OAuth 账号的点数余额（credits）与重置卡（rate_limit_reset_credits）是两套独立的机制：

- **重置卡**：消费一张即可重置 5h/7d 用量窗口，由 `openai_quota_service.go` 的查询/自动用卡流程管理。
- **点数余额**：计划额度（5h/7d 窗口）用完后，ChatGPT 后端自动按余额继续为请求计费。OpenAI 定价页原话："After you reach your included limits, available credits let you continue working." 请求侧无需任何标记或开关；账号是否允许消耗点数由 OpenAI 账号侧设置决定，网关只能感知、不能修改。

## 点数余额的采集与展示

- 管理端额度查询（`/wham/usage`）返回 `credits`（`has_credits` / `unlimited` / `balance`），写入账号 Extra 的 `codex_credits_snapshot`，账号卡片的点数列展示。
- 推理响应携带 `x-codex-credits-has-credits` / `x-codex-credits-unlimited` / `x-codex-credits-balance` 响应头；网关在热路径解析它们并刷新同一快照（30 秒节流，见 `openai_credit_overage.go`）。因此账号卡片的余额随真实消耗自动更新，无需手动刷新。

## 额度接管开关（credit_overage_enabled）

账号级 Extra 布尔键，仅对 OpenAI OAuth 母账号生效，**默认关闭**（开启会真实消耗账号余额）。开启后的行为：

1. 5h/7d 用量达到 100%（或其他自动暂停/用卡阈值）时，账号**不再被摘出调度**，请求继续转发，由 ChatGPT 按点数余额自动计费。
2. 接管期间跳过自动用卡触发——重置卡留给点数耗尽后的窗口重置。
3. 调度评分的额度余量（headroom）因子不因接管清零豁免，接管账号在打满后得分为 0，自然成为其他账号耗尽后的兜底。
4. 退出条件（满足其一即自动回到原有的"摘除 + 自动用卡 + 等窗口重置"路径）：
   - 响应头显示 `has_credits=false` 且非 `unlimited`；
   - 收到 quota 型 `429 usage_limit_reached`——被拒信号优先于余额展示，网关会立即把快照置为 `has_credits=false`；
   - 管理员关闭开关。

## 边界

- 网关不发起点数的购买或消费动作；计费完全由 ChatGPT 后端自动进行。
- 点数与重置卡互不通用：点数接管不影响重置卡数量，重置卡消费也不消耗点数。
- spark 影子账号不参与接管；credits 状态与豁免均以母账号为准。
