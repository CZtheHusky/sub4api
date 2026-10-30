# Codex 内部审核模型发现修复

日期：2026-10-29（PDT）。诊断历史见 [上游评审台账](../../docs/upstream-review/README.md)。

## 问题与修复

原模型目录逻辑默认删除全部 `codex-auto-*`。即使上游提供 `codex-auto-review`，Codex CLI 0.164.0 也无法发现它，内部 Guardian 审核因此可能退回父模型，并优先使用 low。已观察到的 Astra low 审核线程符合该路径；这不表示主线程的 xhigh 被网关降档。

修复覆盖 `projectAccountModelsBody` 的账号映射前过滤，以及 `mergeConfiguredCodexModelsManifest` 的最终过滤：上游目录的 `codex-auto-review` 默认保留为 `visibility=hide`，保留审核能力、推理档位和未知元数据；账号映射仍需匹配，已启用的分组模型白名单仍执行准入判断。精确选中的审核模型继续按原规则显示在菜单；通配允许的审核模型保持隐藏。其他 `codex-auto-*` 继续要求精确显式启用。

本地合成目录在构建前复用原有 `FilterCodexModelIDsForGroup`，避免把内置默认模型误当作上游实际提供的审核能力。没有增加模型别名、改写 Responses/WS 请求、改动推理参数、开启整个白名单或修改版本身份。

官方 0.164.0 源码依据（提交 `6aaac694803004a63c6f393d153c96d22521a6b9`）：

- [ModelsManager](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/models-manager/src/manager.rs)：内部模型列表保留隐藏条目。
- [ModelPreset](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/protocol/src/openai_models.rs)：`visibility` 决定菜单显示，认证模式仍按 `supported_in_api` 过滤。
- [Guardian 选择器](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/ext/guardian-reviewer/src/model.rs)：从内部列表选首选审核模型；缺失时回退父模型并优先 low。

## 本地验证

- 在无外网、真实 HOME 不可见、清空环境变量的隔离环境运行 Go 测试；四个包 `internal/service`、`internal/handler`、`internal/server/routes`、`internal/server/middleware` 中模型目录与白名单相关用例通过。
- 覆盖现网同形的审核模型账号精确映射，关闭白名单但保存旧选择、通配允许、精确选择、白名单拒绝、其他自动模式过滤、合成目录不虚构审核模型、元数据保留、最终 ETag/304 和缓存复用。
- OAuth 使用本地 HTTP 假上游和假 token 验证隐藏审核条目原样保留；固定账号 handler 使用现有假 HTTPUpstream 验证同一过滤链路。
- 从 pjdev-A100 只读复制其 0.164.0 CLI 二进制到临时目录，在空账号、无外网环境加载本地假目录。实际 `model/list`：`includeHidden=false` 只列 Astra；`includeHidden=true` 同时列出 `codex-auto-review`，并标记隐藏。没有生成请求。

验证材料：本机 `/tmp/sub4api-auto-review-20261029/`；最终 Go 通过日志 `after-mapping.log`。`before-final.log` 证明原版本删除审核模型；`before-mapping.log` 证明仅修最终过滤仍会在账号映射阶段失败。CLI 复制由用户 systemd 单元 `sub4api-auto-review-copy-cli-20261029.service` 托管，完整命令见该目录 `copy-cli.sh`，日志 `copy-cli.log`。

## 验证边界

没有使用实际账号主动测试。下述真实结果全部来自用户正常使用产生的自然流量，只读核对缓存、线程元数据、日志标识符和用量记录，未抓取提示词或工具参数。已证实当前会话切换成功；未验收全部会话、所有认证形态、后续限流或 OpenAI 实际计费。目录发现仍受上游能力与账号/分组访问策略约束。

## ccvps 发布

修复提交：`b4355c7aea76c917012a4a7b1eeeea5de0a6049f`（最终目录与合成目录），`767665f8400dd5ace519c8ab68e1c9d714cb9986`（账号映射阶段）。第一版于 20:03:52 PDT 部署，但收尾发现现网账号存在审核模型精确映射、且透传关闭，映射阶段仍提前删除条目；补充同形回归用例并修复后重新构建发布。最终验收以 `767665f84` 为准。

| 项目 | 最终结果 |
| --- | --- |
| 原程序 | `1.2.0-custom.reset-default.7b0b5bf1a`；保留其自动额度重置更新 |
| 最终程序 | `1.2.0-custom.review-discovery.767665f84` |
| 启动时间 | 2026-10-29 **20:12:12 PDT** / 2026-10-30 **03:12:12 UTC** |
| 构建 | 提交快照，断网、空 HOME，Linux amd64、`CGO_ENABLED=0`、`-tags=embed`；首轮完整前端构建的 3 项 i18n 测试与类型检查通过，最终后端复用本轮已构建且源码无变化的网页资源 |
| 本地发布包检查 | 最终二进制启动成功，首页及 6 个 JS/CSS 资源检查通过 |
| 线上检查 | `/health` 正常；`/setup/status` 为 `needs_setup=false`；首页和 6 个资源正常；运行中与磁盘二进制一致 |
| 稳定性复核 | 03:13:14 UTC 仍为 PID `36498`、`NRestarts=0`；所查日志无 ERROR/FATAL/PANIC，WARN 计数 9 |
| 数据库 | 没有新增或待执行迁移；303 个本地迁移匹配现网，另有 8 条历史记录保留 |
| 配置与依赖 | 配置、systemd 单元和 3 项版本设置不变；手工 CLI 版本 `0.164.0`，自动同步 `false`；PostgreSQL/Redis PID 仍为 `819`/`666` |

最终二进制 SHA256：`1aa6667e0ecca62e76db5d79e9be65b98cee15f5a999276e1488b09621910f3c`。

### 自然流量验收

目标主会话：`01a17678-7b5f-73d0-9978-f4ee0af08c95`，pjdev-A100 的实际 CLI 为 `0.164.0`。

- 03:12:41 UTC 新建 Guardian 线程 `01a18acb-9bc2-7c72-9c31-9266b946d800`：`source={"subagent":{"other":"guardian"}}`、`model=codex-auto-review`、`reasoning_effort=low`。
- 03:12:43.584–03:12:49.369 UTC，ccvps 记录到 3 次该主会话的审核调用，`model` 与 `upstream_model` 均为 `codex-auto-review`。3 个响应 ID 均与 pjdev-A100 的 CLI feedback 日志精确匹配；这些 feedback 行自身没有 thread_id，主会话归属取自网关 session_id。
- 03:12:53 UTC 的客户端缓存从 9 个模型变为 10 个，新增 `codex-auto-review`，`visibility=hide`、`supported_in_api=true`。
- 主线程状态仍为 `gpt-6-astra / xhigh`；网关同时记录了 Astra xhigh 请求。当前会话已自行刷新并开始使用审核模型，不需要手动切主模型或重开会话。

本机只读证据：`client-after.jsonl`、`usage-after.json`、`match-after.json`、`verify-final.json`、`postcheck.json`。本结论证明所选样本的审核选模恢复，不声称所有 low 请求都会消失或推导计费节省。

### 备份与回滚

远端私有目录：`/opt/sub2api/backups/review-discovery-b4355c7ae-20261030T0300Z`，权限 `0700`。保存原 `7b0b5bf1a` 程序、配置、systemd 单元、版本设置快照、PostgreSQL 自定义格式备份（16,661,825 字节，目录检查通过）及部署/回滚/核验脚本。备份未下载。部署失败自动回滚未触发；未在生产演练回滚。

必要时只恢复原程序，不恢复数据库以避免覆盖后续业务数据：

```sh
ssh ccvps 'systemd-run --unit=sub2api-rollback-review-767665f84 \
  --property=RuntimeMaxSec=15min --property=UMask=0077 \
  --property=StandardOutput=append:/opt/sub2api/backups/review-discovery-b4355c7ae-20261030T0300Z/rollback.log \
  --property=StandardError=append:/opt/sub2api/backups/review-discovery-b4355c7ae-20261030T0300Z/rollback.log \
  /bin/bash /opt/sub2api/backups/review-discovery-b4355c7ae-20261030T0300Z/rollback.sh'
```

耗时不定的操作均由 systemd 托管，不使用 PTY 保活。完整命令保存在相应脚本及单元 ExecStart：

| 操作 | 单元 | 脚本 / 输出 |
| --- | --- | --- |
| 本机完整构建 | `sub4api-review-discovery-build-b4355c7ae.service` | 本机验证目录 `build.sh`、`build.log`、`output/sub2api` |
| 本机最终构建 | `sub4api-review-discovery-build-767665f84.service` | `build-v2.sh`、`build-inside-v2.sh`、`build-v2.log`、`output/sub2api-v2` |
| 本机上传 | `sub4api-review-discovery-upload-767665f84.service` | `upload-v2.sh`、`upload-v2.log` |
| 远端备份 | `sub2api-backup-review-b4355c7ae.service` | 远端备份目录 `backup.sh`、`backup.log` |
| 远端最终替换 | `sub2api-deploy-review-767665f84.service` | `apply-v2.sh`、`deploy-v2.log`、`verify-new.json`、`postcheck.json` |
