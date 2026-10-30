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

验证材料：本机 `/tmp/sub4api-auto-review-20261029/`；Go 通过日志 `/tmp/sub4api-auto-review-after2.log`。CLI 复制由用户 systemd 单元 `sub4api-auto-review-copy-cli-20261029.service` 托管，完整命令见该目录 `copy-cli.sh`，日志 `copy-cli.log`。

## 验证边界

没有使用实际账号主动测试。尚未确认生产上游目录是否提供该模型、其 `supported_in_api` 等认证条件是否允许当前客户端选择，也未证明生产 Guardian 已切换成功。该修复恢复网关应保留的发现能力，不承诺上游权限。现有 CLI 需要后续刷新模型目录，已创建的审核线程不会因此自动换模型。

## ccvps 发布

发布结果将在完成替换及健康检查后补记。构建基于已提交源码，保留当前线上 `7b0b5bf1a` 的自动额度重置更新；本修复不新增数据库迁移。
