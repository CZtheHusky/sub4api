# Codex CLI 0.164.0：本地对齐记录

日期：2026-10-26（本地时区），部署记录更新于 2026-10-28。仓库：`/home/caozhe/workspace/sub4api`。修改基线：`e59fb52e2327aad880cbf2f2c983a161b0df4a3b`。下方 10 月 26–27 日的验收与授权边界是当时记录；最新部署状态见本节。

## ccvps 部署（2026-10-28）

用户明确授权替换 ccvps 后，已将 `sub2api.service` 更新为对齐提交 `eb6ac219288c43706ec77b7a074c12ec9ca0ff0e`。服务于 **11:53:38 UTC（04:53:38 PDT）**启动，11:53:41 UTC 首轮检查通过，11:54:42 UTC 复核仍为同一进程、`NRestarts=0`。此前“禁止使用实际账号测试”的限制仍有效：本次未发起实际账号推理请求。

| 项目 | 部署结果 |
| --- | --- |
| 原程序 | `1.2.0`，提交 `e59fb52e2327aad880cbf2f2c983a161b0df4a3b` |
| 新程序 | `1.2.0-custom.codex164.eb6ac2192`，提交 `eb6ac219288c43706ec77b7a074c12ec9ca0ff0e` |
| 发布包 | 从已提交源码在本机断网、空白 HOME 环境重建，`CGO_ENABLED=0`、Linux amd64、`-tags=embed`，包含控制台；构建中的 3 项 i18n 测试、类型检查、前端及后端构建成功 |
| 本机发布包检查 | 同一完整二进制在隔离环境启动，首页与 6 个 JS/CSS 资源可访问；没有使用真实配置或凭据 |
| 生效版本设置 | `openai_codex_client_version`：`0.161.0` → `0.164.0` |
| 其他版本设置 | 自动同步保持 `false`；同步存储值仍为 `0.163.0`，被手工设置覆盖 |
| 数据库迁移 | 本次提交未新增迁移；303 个本地迁移与现网对应记录全部匹配，无待执行迁移；现网另有 8 条历史迁移记录，保留 |
| 现网检查 | 磁盘与运行中程序 SHA256 一致；`/health` 正常；`/setup/status` 为 `needs_setup=false`；首页与 6 个 JS/CSS 资源均正常 |
| 保留项 | `config.yaml`、systemd 单元内容未变；PostgreSQL 与 Redis 进程 ID 分别仍为 `819`、`666`，没有重启 |

新程序 SHA256：`fd65e7adfd46069126c7ab24325d324a4200d9944502fac55fe3d15e45c781e5`。旧程序 SHA256：`51374c61e80c2c544398c5395d910118ad67fe58e03a989ae144cb5b69cb98c1`。迁移校验复用应用的 `TrimSpace` 后 SHA256 语义，未改数据库中的 checksum。

远端备份目录（`root` 所有、权限 `0700`）：

```text
/opt/sub2api/backups/codex164-eb6ac2192-20261028T115020Z
```

目录内包括旧二进制、原配置、原 systemd 单元、3 项版本设置快照 `before.json`、PostgreSQL 自定义格式备份 `sub2api.pgdump`（14,851,992 字节，`pg_restore --list` 成功）、备份校验和、部署/回滚脚本及核验结果。数据库备份始终留在远端私有目录，未下载或打印凭据。备份和上传完成后才执行替换；新服务检查失败时部署脚本会自动恢复旧程序和原手工版本。此次部署没有触发自动回滚，也未在现网执行回滚演练。

需要回滚时在本机执行以下命令；它只恢复程序和原手工版本，不还原数据库，不覆盖此后产生的业务数据：

```sh
ssh ccvps 'systemd-run --unit=sub2api-rollback-codex164-eb6ac2192 \
  --property=RuntimeMaxSec=15min --property=UMask=0077 \
  --property=StandardOutput=append:/opt/sub2api/backups/codex164-eb6ac2192-20261028T115020Z/rollback.log \
  --property=StandardError=append:/opt/sub2api/backups/codex164-eb6ac2192-20261028T115020Z/rollback.log \
  /bin/bash /opt/sub2api/backups/codex164-eb6ac2192-20261028T115020Z/rollback.sh'
```

所有耗时不定的步骤均由 systemd 托管，没有使用 PTY 保活。完整命令已保存在对应脚本和单元 `ExecStart` 中：

| 步骤 / 托管位置 | 单元 | 命令 / 输出 |
| --- | --- | --- |
| 本机完整构建 | `sub4api-ccvps-build-eb6ac2192-v2.service` | `/bin/bash /tmp/sub4api-ccvps-eb6ac2192-20261028/build.sh`；`build2.log`；产物 `output/sub2api` |
| 本机上传 | `sub4api-ccvps-upload-eb6ac2192.service` | `/usr/bin/scp -C -B -o StrictHostKeyChecking=yes -o ConnectTimeout=15 /tmp/sub4api-ccvps-eb6ac2192-20261028/output/sub2api ccvps:/opt/sub2api/backups/codex164-eb6ac2192-20261028T115020Z/sub2api.candidate`；本机目录 `upload.log` |
| 远端备份 | `sub2api-backup-codex164-eb6ac2192.service` | `/bin/bash <远端备份目录>/backup.sh`；`backup.log` |
| 远端替换 | `sub2api-deploy-codex164-eb6ac2192.service` | `/bin/bash <远端备份目录>/apply.sh`；`deploy.log`、`verify-new.json`、`postcheck.json` |

以上成功单元退出码均为 0。首次构建曾因测试缓存目录只读而失败；随后只为 `.vite` 缓存挂载临时可写目录，第二次构建成功，未修改生产源码。启动窗口日志没有 ERROR/FATAL；仍有原配置相关警告（TOTP/payment 固定密钥、URL allowlist、JWT 持久化值、可信代理、CORS）及原先缺失的定价 fallback 文件提示，本次未修改这些配置或资源。完整日志保存在远端备份目录的 `startup-journal.jsonl`。

**验收边界：**此次确认部署、启动和控制台可用；此前的 HTTP/WS 模拟测试证据继续有效，但没有新增真实上游 HTTP/WS、真实缓存命中、错误率或长会话验证，不能把健康接口通过等同于真实账号链路验收。

## 替换现有服务的判断（2026-10-27）

已完成本机离线候选版验收，可以进入另获授权后的真实账号小流量验证；仍不足以直接替换现有服务。最终所选回归 **5241 个测试/子测试通过、0 失败、1 项真实 API 测试按授权边界跳过**；此前 72 项 race 检查通过，之后仅修改测试。安装版 CLI 的五个 WS 场景已有直连/经过转发层对照，本轮另完成完整后端的模拟上游验证。

已补齐：HTTP/WS 鉴权与余额不足拒绝、正式调度链（账号均为构造数据）、用量入库与扣费一致性、Lite 合成前缀跨请求/重连稳定性、基线→候选→基线的启动回退；旧调度失败已通过修正测试账号配置解决。完整补丁包含被本地忽略的新增 Go 文件，复现工具和两份后端可执行文件均已落盘。

替换前仍需：获授权后测试真实上游缓存、错误率及长会话表现；核对目标环境的手工/同步版本设置，确保声明为 `0.164.0`。自动同步逻辑仍可覆盖内置值，不能只凭二进制中的常量认定线上身份版本。尚未核验 ccvps 的配置和数据，也没有做部署。

UUIDv5/v7 的分支原因已解释并由同一二进制 A/B 验证，不再列为“源码与安装包冲突”。真实缓存效果仍未测量。没有部署或切换服务，也没有执行真实账号请求。

## 目标和授权边界

按照[核心开发指南](codex-cli-alignment-core-guide.md)补齐应用层差异：指令与工具前缀、WS 即时中断、服务端重试建议、合成请求顺序，以及 0.164.0 的内置版本声明。

所有执行均在本机。没有连接 ccvps，没有替换、重启其服务；没有读取实际账号凭据或使用实际账号发起测试。公开 GitHub 源码查询与隔离测试是两条独立路径。

成功标准是本地正式发送路径的格式与控制流经过验证，不是宣称 TLS、HTTP/2 或上游风控判定与官方客户端完全一致。

## 官方基线与本机样本

公开版本为 [rust-v0.164.0](https://github.com/openai/codex/releases/tag/rust-v0.164.0)，发布于 2026-10-23 UTC；tag 指向提交 `6aaac694803004a63c6f393d153c96d22521a6b9`。

| 证据 | 本次采用的结论 |
| --- | --- |
| [core/src/client.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/core/src/client.rs) | 指令进入 developer input；Lite 非空工具放入 additional_tools；请求构建阶段临时补入的前缀使用 UUIDv5，默认增量模式的历史项使用 UUIDv7（见下文）；parallel_tool_calls 在 Lite 下为 false |
| [codex-api/src/common.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/codex-api/src/common.rs) | HTTP/WS 请求类型与顶层声明顺序；普通推理正文不再含顶层 instructions |
| [features/src/lib.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/features/src/lib.rs) | IncrementalTools 为 Stable 且默认开启；InstantInterrupt 默认开启；AsyncCompaction 与 OutputTokenReplay 默认关闭 |
| [responses_websocket.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/codex-api/src/endpoint/responses_websocket.rs)、[retry_after.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/http-client/src/retry_after.rs) | interrupt 确认与生成完成是独立事件；错误可以携带 Retry-After 秒数或 HTTP 日期 |

本机可执行文件为 `~/.codex/packages/standalone/releases/0.164.0-x86_64-unknown-linux-musl/bin/codex`，隔离执行的版本输出为 `codex-cli 0.164.0`。使用同目录的 code-mode host、空白 HOME/CODEX_HOME、假 API Key、自定义本地 provider，完成一次 HTTP SSE 回合，返回 `Local mock verified.`，退出码为 0。

实际捕获的请求符合以下关系：

- 顶层没有 instructions 和 tools；非空工具出现在第一个 additional_tools 项，后续为 developer 指令及用户输入。
- parallel_tool_calls 为 false；模型、stream 位于 input 前面。
- `originator=codex_exec`；UA 声明 0.164.0。此模式不是 TUI，因此不能据此覆盖网关的 TUI 兜底 originator。
- body metadata 包含 `history_initialization=new`（在序列化的 x-codex-turn-metadata 内），请求携带 Lite 标记。

**更正：此前“本机 UUIDv7 与公开源码 UUIDv5 冲突”的结论撤回。** 原因是漏读了上层调用方，把非增量模式的请求构建分支当成默认发送路径。公开 0.164.0 中两条路径同时存在，且已用同一本机二进制的功能开关对照验证。

### 前缀 ID：为什么同一版本同时存在 v7 和 v5

这里讨论的是 `input` 中 `additional_tools` 的 `at_…` 与基础指令的 `msg_…`，不是 session/thread ID，也不是所有 developer 消息都使用同一种算法。

| 路径 | 创建时机与 ID | 保持稳定的方式 |
| --- | --- | --- |
| Responses Lite 增量工具模式（新会话默认开启） | 工具与基础指令先进入会话历史；缺失 ID 经 `ResponseItemId::new` 分配 UUIDv7 | 已有历史项保留 ID，重试和续接复用这些项 |
| 非增量工具模式 | `build_responses_request` 每次临时插入工具和基础指令，用 thread 与内容计算 UUIDv5 | 相同 thread 与相同内容重新计算得到相同 ID |

公开 tag 的调用链：

1. [world_state.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/core/src/session/world_state.rs#L44-L56)根据 Lite 能力、功能开关及当前历史决定是否使用增量模式；已有历史窗口还会考虑是否已有工具声明，不是每次请求只看开关。
2. 增量模式先产生工具/指令历史项；[session/mod.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/core/src/session/mod.rs#L3575-L3597)补齐缺失 ID；[response_item_id.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/protocol/src/response_item_id.rs#L19-L25)的 `new` 使用 `Uuid::now_v7()`。
3. [turn.rs 的 build_prompt](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/core/src/session/turn.rs#L1672-L1694)在增量模式下将 `prompt.tools` 与 `prompt.base_instructions.text` 置空，因此 `client.rs` 不会再次插入 v5 前缀；此前漏掉的正是这一步。
4. 非增量模式保留这两个字段，[client.rs](https://github.com/openai/codex/blob/6aaac694803004a63c6f393d153c96d22521a6b9/codex-rs/core/src/client.rs#L1008-L1041)的 UUIDv5 分支才执行。

同一安装包 A/B 实测：默认模式捕获 `at_…` / 基础指令 `msg_…` 为 v7；仅设置 `features.incremental_tools=false` 后，两者变为 v5。非增量模式下它们先随 `generate=false` 的预热帧发送，随后实际生成帧通过 `previous_response_id` 省略已发送前缀；只检查生成帧会漏看这两个 v5 ID。对基础指令文本 `Use no tools. Local wire probe.`，按捕获的 thread 计算 UUIDv5，与实际 ID `msg_08afa4a0-79a2-50f2-a4b8-0c54a683f4cc` 完全相同。默认 v7 样本在重试与取消后重连时也保留原 ID，不能把 v7 理解成每次发送都随机换 ID。

A/B 单元为 `codex164-cli-id-ab.service`，退出码 0；执行 `/bin/bash /tmp/codex164-alignment.ymc5RW/isolated-cli-no-incremental.sh`，限制 2 分钟、1 GiB、1 CPU。日志 `cli-id-ab.log`，新捕获目录 `cli-ws-no-incremental-capture/continuation`。机器可读证据见 [ID 对照结果](codex-cli-0.164.0-id-ab.json)，复现脚本已补入后文工具包。仍为无外网、假凭据测试。

对网关的影响：原生客户端已有 ID 原样保留，默认 v7 路径无需改写。当前网关给缺失前缀 ID 合成 v5，仅实现了内容稳定的兼容策略，不等于默认增量模式的历史项分配策略；不能据此声称默认模式的 ID 生成已完全对齐。本次解释修正没有变更生产 ID 算法，也未证明 UUID 版本会导致上游拒绝或降载。

### 缓存风险与已验证范围（2026-10-27 补充）

v5/v7 并存本身不是缓存失效的证据；需要分别检查上下文续接与模型前缀缓存。

- **CLI 增量续接：**公开 `client.rs` 的 `get_incremental_items` 比较前一请求和当前请求的已有 input 项。比较函数只忽略特定内部 metadata，没有忽略 ID；若 CLI 自己的旧历史项 ID 改变，会放弃该次增量发送。这是客户端判定，不能直接推导为网关改写后上游 KV 缓存必然失效。
- **模型前缀缓存：**[官方文档](https://developers.openai.com/api/docs/guides/prompt-caching)要求渲染后的前缀匹配。工具内容/顺序、指令和相关请求设置变化可能减少缓存复用；文档未明确 `msg_/at_` 的 UUID 版本如何参与缓存键，不能把 v5 与 v7 的差异直接等同于缓存未命中。公开 API 文档也不等于 `/backend-api/codex` 内部实现规格。
- **当前本地证据：**五个经过网关的 WS 场景中，捕获的 `client_metadata.thread_id` 与 `prompt_cache_key` 各自在场景内保持一致；CLI 默认模式重试/取消后重连时工具和基础指令 ID 保持原值。合成 v5 在相同 thread、相同指令或工具 JSON 下是确定的，但仍依赖上层会话标识与工具序列化稳定，不能自动外推到所有非原生入口、恢复会话或换账号路径。

模拟服务的 usage 是构造值，没有实际模型缓存。因此“本地续接通过”和“真实缓存命中率没有回退”是两个结论，后者尚未验证。真实验收需在获得账号测试授权后比较 `usage.input_tokens_details.cached_tokens` 与相关诊断；目前没有做此类请求，也不应为了外形一致而每次发送重新生成 v7。

### 补充：本机安装包的 WS 实测

单轮 HTTP 采样不足以验收 WS。随后在同样的无外网、空白 HOME、假凭据环境，通过本机 `codex app-server --stdio` 的公开接口驱动推理会话，连接 loopback WS mock；app-server 的控制通道为 stdio，采集的是 CLI 发往推理服务的 Responses WS。未操作当前桌面会话或真实账号。

本机 CLI 直连 mock 的五个场景均通过（`codex164-cli-ws-probe5.service`，退出码 0）：

| 场景 | 实际观察 |
| --- | --- |
| 预热与两轮对话 | 首帧为 `response.create`、`generate=false`、空 input；mock 返回空输出的完成事件后，第一轮引用预热响应；第二轮继续引用上一轮响应，仅发送新增 user item；同一 WS 连接完成 |
| 取消当前任务 | 收到正文片段后调用 `turn/interrupt`，CLI 直接断开原 WS，没有发送 `response.interrupt`；新一轮新建连接并发送完整历史，不带 `previous_response_id` |
| 生成中追加指令，确认先到 | `turn/steer` 触发 `response.interrupt`，携带当前 response_id 与 `mode=discard_partial_items`；收到确认和完成后，在原连接续接新增输入 |
| 生成中追加指令，完成先到 | mock 先发完成、再发确认，同一回合仍正常续接并完成 |
| 流内 Retry-After | mock 返回 503 / server_is_overloaded，error.headers 中给出 `retry-after=1`；CLI 约 1.01 秒后新建 WS、移除旧响应引用并重发完整输入，随后成功 |

因此，不能把“取消任务”和“生成中追加指令”视为同一种上游控制操作。早期 probe 曾错误地期待 `turn/interrupt` 必发控制帧；等待 CLI 确认收到 delta 后重测仍是断连，该预期已改正，未据此修改生产代码。

握手实测包含 `OpenAI-Beta: responses_websockets=2026-02-06`、会话/线程关联头，以及 `permessage-deflate; client_max_window_bits` 压缩提议。mock 未接受压缩，因此这里只验证提议，未测压缩后的字节。WS 前缀 ID 也为 UUIDv7；自定义 app-server 客户端身份为 `codex_ws_local_probe`，不能据此认定 TUI 使用相同 originator。

原始本地记录位于 `/tmp/codex164-alignment.ymc5RW/cli-ws-capture/<场景>/`：`wire.json` 为握手与双向帧，`app-server.json` 为控制接口通知，`summary.json` 为结果。总日志为 `cli-ws-probe5.log`。执行命令为 `/bin/bash /tmp/codex164-alignment.ymc5RW/isolated-cli-ws.sh`，systemd 限制为 5 分钟、1 GiB、1 CPU；每个场景最多 60 秒。

随后将同样五个场景接入本地 `ProxyResponsesWebSocketFromClient`，采用假 OAuth 账号、ctx_pool 和正式 WS dialer，仅将上游目的地改为 loopback mock；五个场景也全部通过。对比两个独立运行捕获的帧，顶层字段顺序、input 项的 type/role 序列、预热与续接引用关系一致；原生前缀仍为 UUIDv7。经网关的 Retry-After 场景约 1.03 秒后重试成功。这里没有经过数据库、鉴权中间件或账号调度，因此不是整套服务验收。

**仍存在握手差异：**直连使用测试客户端声明的 `codex_ws_local_probe` / Ubuntu UA，网关按既有身份规则回退为 `codex-tui` / Windows UA 并增加 `version: 0.164.0`；直连压缩提议是 `permessage-deflate; client_max_window_bits`，网关为 `permessage-deflate`。这证明所测业务流程可以通过，不能证明请求指纹不可区分；未为追求样本相同而修改身份策略或底层压缩实现。

机器可读对照见 [WS 采样结果](codex-cli-0.164.0-ws-sample.json)。[复现工具包](codex-cli-0.164.0-ws-probe.tar.gz)保存 Python probe、临时 Go 测试、Go overlay 映射、隔离执行器，以及关闭增量模式的 A/B probe；按其中的绝对路径解压到 `/tmp/codex164-alignment.ymc5RW` 后可复用。overlay 仅向 Go 构建视图添加测试，不修改生产源码或常规测试集合。

网关测试单元为 `codex164-cli-ws-gateway.service`，退出码 0；日志为 `/tmp/codex164-alignment.ymc5RW/cli-ws-gateway.log`，捕获目录为 `cli-ws-gateway-capture`。systemd 限制 15 分钟、10 GiB、2 CPU。完整启动命令如下：

```sh
systemd-run --user --unit=codex164-cli-ws-gateway \
  --property=RuntimeMaxSec=15min --property=MemoryMax=10G \
  --property=CPUQuota=200% --property=TasksMax=256 \
  --property=StandardOutput=append:/tmp/codex164-alignment.ymc5RW/cli-ws-gateway.log \
  --property=StandardError=append:/tmp/codex164-alignment.ymc5RW/cli-ws-gateway.log \
  /bin/bash /tmp/codex164-alignment.ymc5RW/isolated-cli-gateway.sh \
  test -json -p 2 -tags=unit -overlay=/tmp/task/cli-probe-overlay.json \
  ./internal/service -run '^TestCodex164InstalledCLIViaGateway$' -count=1 -timeout=4m
```

复跑需使用未占用的 systemd 单元名。测试结束后已确认两个成功单元均停止。该次补测只更新记录与测试工具，没有新增生产代码改动。后续完整后端验证见下一节；调度基线旧断言已修正，真实 API 测试继续跳过。

## 完整后端的本机验收（2026-10-27）

使用未修改 HEAD 与当前工作区分别构建后端，均为 `CGO_ENABLED=0`、`-trimpath`，未嵌入网页前端。构建没有下载依赖。两份产物：

- `/home/caozhe/workspace/sub4api/backend/bin/codex164-local-20261027/server-candidate`
- `/home/caozhe/workspace/sub4api/backend/bin/codex164-local-20261027/server-baseline`

这里的 baseline 是仓库未修改 HEAD，**不是从 ccvps 复制的现网二进制**。回退验证只能证明这两份构建对同一临时数据库兼容。

测试由 `codex164-fullstack-final.service` 托管，Docker daemon 管理其子容器。使用本机已有 PostgreSQL 18 与 Redis 8 镜像、新建数据库、假 OAuth token、假网关密钥；标准计费模式没有关闭。PostgreSQL 容器为 `--network none`，Redis、后端、TLS mock 共用该网络命名空间；没有发布端口，路由表为空。测试专用证书仅由测试后端信任，`chatgpt.com` 在该容器内解析到 loopback。未读取真实 HOME 或部署配置。

| 检查 | 结果与意义 |
| --- | --- |
| 鉴权与余额 | 无效密钥、零余额用户在 HTTP 和 WS 入口均被拒绝，没有到达模拟上游 |
| 选号 | 两个测试分组分别选中 HTTP 与 WS 假账号；优先级更高但已禁用的账号没有被选中 |
| HTTP | 首轮、相同请求重发、完整历史续聊和完整历史恢复共 4 个回合完成 |
| WS | 首轮、带响应引用的重连、生成中 interrupt，以及中断后的同连接续聊共 4 个回合完成 |
| Lite 前缀 | 同一凭据、稳定 session/thread/prompt_cache_key 下，4 次 HTTP 与 4 次 WS 各自的工具/指令前缀内容和 ID 完全相同 |
| 用量与计费 | 每个回合恰好 1 条记录，共 8 条；interrupt 控制帧不单独计费；没有禁用账号的记录 |
| 余额 | 构造的每轮 usage 为总输入 100、缓存输入 40、输出 10；数据库按既有语义记录非缓存输入 60、缓存输入 40、输出 10。测试用户从 100 变为 99.99504，与 8 条费用之和一致，冻结余额归零 |
| 启动与回退 | 基线初始化数据库→候选运行→候选正常 SIGTERM 退出（0）→基线重启，8 条用量记录保留 |

普通 Responses 模式也完成同样的 8 回合链路；Lite 模式另验证 `additional_tools` 和指令前缀。数据来自模拟响应，因此上述缓存输入计数只证明解析、记录和扣费正确，不能证明实际缓存命中。

补充边界：生产 handler 对 OAuth **HTTP** 的 `previous_response_id` 返回 400，要求使用 API-key 账号；本次没有改变该策略。HTTP 续聊采用完整历史，WS 使用响应引用。首轮 probe 曾触发这一限制，已记录为测试输入修正，没有把 400 绕过或修改生产逻辑。

持久单测 `TestCodex164PrefixSurvivesFreshHTTPRequests` 经 Forward 的转换与透传两条路径检查稳定前缀，以及换 thread/换凭据时隔离、回到原凭据时恢复原 ID。完整 HTTP 入口的合法请求由上面的服务测试覆盖。验证依赖客户端提供稳定的会话信号；未携带任何会话信号的任意增量请求不在本轮保证范围内。

机器可读结果见 [完整后端结果](codex-cli-0.164.0-fullstack-results.json)，源码和原始 wire/用量记录见 [复现包](codex-cli-0.164.0-fullstack-probe.tar.gz)。包内不包含真实凭据或测试私钥；复跑时重新生成仅在隔离容器内使用的证书。完成后已删除测试容器及其临时数据库卷。

完整监管命令及资源边界：

```sh
systemd-run --user --unit=codex164-build-artifacts \
  --property=RuntimeMaxSec=25min --property=MemoryMax=10G \
  --property=CPUQuota=200% --property=TasksMax=256 \
  --property=StandardOutput=append:/tmp/codex164-alignment.ymc5RW/build-artifacts.log \
  --property=StandardError=append:/tmp/codex164-alignment.ymc5RW/build-artifacts.log \
  /bin/bash /tmp/codex164-alignment.ymc5RW/build-artifacts.sh

systemd-run --user --unit=codex164-fullstack-final \
  --property=RuntimeMaxSec=15min --property=MemoryMax=5G \
  --property=CPUQuota=200% --property=TasksMax=256 \
  --property=StandardOutput=append:/tmp/codex164-alignment.ymc5RW/fullstack-final.log \
  --property=StandardError=append:/tmp/codex164-alignment.ymc5RW/fullstack-final.log \
  /bin/bash /tmp/codex164-alignment.ymc5RW/fullstack/run.sh
```

复跑需用新的单元名，并确认 `codex164-local-*` 容器名空闲。脚本使用的每个容器分别限制内存/CPU；systemd 的内存上限不代替 Docker 容器限制。包内脚本保留本机绝对路径；新环境须先按该目录放置脚本，并以 `git archive e59fb52e2327aad880cbf2f2c983a161b0df4a3b backend` 导出到 `baseline/`。先构建两份产物，再运行服务测试。构建器和复现包均未向用户级守护服务注册自动运行任务。

## 已落地的行为

### 指令和工具前缀

新增共享 `openai_codex_request_prefix.go`，在最后一次 metadata 归一化时执行；HTTP、HTTP passthrough、HTTP→WS、原生 WS 首帧和后续帧共用该处理。

- 普通 OAuth/SetupToken 的顶层 instructions 迁为 developer 消息并删除顶层字段；system 角色转换为 developer，保留原内容和顺序。
- 已有 developer/system 指令时，不再额外注入默认指令。Chat Completions 转换保留指令一次，JSON 输出和混合图片指令也不再重复。
- Lite 空工具列表不制造空 additional_tools；已有工具更新、原生 ID、密文、metadata 补丁保留。
- 缺少 ID 的前缀使用 `namespace=UUIDv5(NAMESPACE_OID, thread_id)`，指令为 `msg_UUIDv5(namespace,text)`，工具为 `at_UUIDv5(namespace,serialized_tools)`。
- Map 发送路径使用统一的不转义 HTML 的 JSON 编码，避免工具 ID 对应的是另一种转义形式。
- legacy compact 和公共 API Key 不套用普通 Codex 指令迁移。

工具 ID 对合成后的实际工具 JSON 稳定；Go map 的字段顺序不等于 Rust 所有 Tool 结构的声明顺序。未实现整个工具类型体系的 Rust 字节复刻，也没有伪造完整 history_initialization 或 CLI 会话历史。

### WS 即时中断和续接

`ctx_pool` 在生成期间保持一个下游读取者，把 response.interrupt 转发到当前生成连接；中断控制帧不进入 create 转换器。确认帧不触发下一轮计费或 create。

完成事件仍负责结算用量和保存完整输出。覆盖确认先到、完成先到两种顺序；完成先到时最多额外等待确认一秒，确认读取失败则丢弃该连接，保留已完成回合的结算和回放结果。队列中的下一个 create 继续按既有续接规则处理。

HTTP bridge 没有同等上游中断能力：生成期间返回 response.interrupt.failed / unsupported_operation，并继续读完 HTTP 响应以保住用量，不虚构成功中断。原有 WS passthrough 使用其既有双向转发通路。

### Retry-After 与错误修复

共享解析器支持非负整秒（包括 0）和 HTTP 日期；事件内部 error.headers 优先，其次 response.error.headers、顶层 headers，最后 HTTP/握手头。有效建议转换为绝对时刻，在同账号重试和普通 WS 重连退避中使用剩余等待时间，不因跨层重新开始倒计时；既有重试预算继续约束下一次请求。

WS 重试耗尽时向下游保留剩余 Retry-After。带有效建议的容量错误保留原始错误码；没有建议时沿用已有兼容改写。这不证明“版本较旧会被上游优先降载”，相关无证据断言已从内置版本注释删除。

指令迁移改变 input 下标，因此 HTTP 400 精确字段修复改读 Request.GetBody 中的最终发送正文，再按上游拒绝的下标重试，避免误删原始正文中的另一个条目。

### 版本和发送格式

内置版本为 `0.164.0`，仍通过既有统一身份解析器生成 UA/version。优先级保持：手工设置 → 同步值 → 内置值。**只改内置常量不会覆盖已有同步值；自动同步仍可能声明更高版本。** 本次没有读取或改变本机数据库、控制台或线上设置。

合成 WS create 帧经正式 socket writer 输出 type、model、stream 等路由字段，再输出 input；复用既有 JSON 排序和编码。原生 RawMessage 保留原有相对顺序。未启用推理 WS zstd；app-server 的 WS 压缩变化不能直接套用到推理协议。RemoteCompactionV2 的 feature 标记移除也未被误当成删除 native compaction 协议。

## 测试方式与结果

机器可读结果见[测试结果清单](codex-cli-0.164.0-test-results.json)。2026-10-27 最终回归通过：service 4617、handler 521、repository 103，共 5241 个测试/子测试，0 失败；真实 API 测试仍跳过 1 项。监管单元 `codex164-release-final.service`，日志 `release-final.log`，退出码 0，选择表达式与下文主要命令相同。

以下保留此前回归与修复过程：扩大回归最初的结果为handler 521 个测试/子测试通过，repository 103 个通过；service 4612 个通过、2 个失败、1 个跳过（计数包含子测试）。两项失败分别为下面独立复现的调度基线问题，以及 Anthropic 兼容入口仍要求空顶层 instructions 的旧断言；后者已改为要求字段省略，并在最终 race 运行中单独覆盖通过。真实 API 对照 `TestEstimateOpenAIInputTokens_CompareWithOpenAIAPI` 因未设置 OPENAI_API_KEY 跳过，符合本次授权边界。

首轮并发检测没有报告数据竞争，但两个生命周期子测试超时：测试客户端收到终止事件后停止读取，没有回应关闭握手。测试已改为按 CLI 行为持续读取到 close，并检查未超过会话期限；生产端也修正了清理阶段的关闭码，普通上游失败保持 handler 原有的 1011，而不是发送正常结束 1000。最终并发复跑通过：service 48 个、handler 24 个测试/子测试，0 失败、0 跳过，无数据竞争报告。最后一轮只有测试断言修正与关闭清理语义变化；后者已由此次 race 的完整生命周期、中断、断连和空闲收尾用例验证。

所有 Go 测试由用户级 systemd 托管，未使用 PTY。主要限制为 `MemoryMax=8G`、`CPUQuota=200%`、`TasksMax=256`、`RuntimeMaxSec=25min`；内部每个包测试超时 8 分钟。Go 为本机缓存的 1.27.0，依赖下载关闭。

测试执行器：`/tmp/codex164-alignment.ymc5RW/isolated-go.sh`。它使用 bubblewrap 隔离 network/PID/IPC/UTS；隐藏 /home、/root、/run；清空环境；只读挂载 backend 源码、Go 工具链和模块缓存。测试仅能访问该网络命名空间内的 loopback；已检查路由表为空、真实 HOME 不可见。构建缓存与任务输出目录单独可写。

完整主要测试命令（在 backend 只读挂载内）：

```text
go test -json -p 2 -tags=unit ./internal/service ./internal/handler ./internal/repository -run 'Codex|OpenAI|SameAccountRetry|ForwardAsChatCompletions|ForwardAsAnthropic|HTTPUpstreamSuite' -count=1 -timeout=8m
```

最终并发验证命令：

```text
go test -json -race -p 1 -tags=unit ./internal/service ./internal/handler -run 'Codex164|CodexWSCompatibilityMetadataOnSocket|ProxyResponsesWebSocket.*(Lifetime|ClientDisconnect|IdleTimeout|HTTPBridgeModeRelaysHTTPStream)|SameAccountRetry|TestForwardAsAnthropic_OAuthKeepsSystemAsDeveloperInput' -count=1 -timeout=5m
```

单元 `codex164-race-final.service`，日志 `/tmp/codex164-alignment.ymc5RW/race-final.log`；内存上限 10 GiB，CPU 200%，运行上限 25 分钟，退出码 0。所选测试包括生成中断、两种确认/完成顺序、下一轮续接、HTTP bridge 中断拒绝、断连后用量收尾、空闲超时、连接寿命与回放边界，以及实际一秒 Retry-After 等待和预算拒绝。

监管单元：`codex164-final-unit.service`；日志：`/tmp/codex164-alignment.ymc5RW/final-unit.log`。中间失败和修复记录留在同目录 round1–round6 等日志，没有用早期通过结果代替最终验证。

已独立确认一个基线失败：`TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_NoAvailableAccount`。用 `git archive HEAD backend` 导出未修改源码，在同样隔离条件运行该测试，仍报“An error is expected but got nil”。2026-10-27 已查明：提交 `f2f98830b1f82538573cebba017d7dbab483bba6` 将 OAuth 默认改为 ctx_pool，而该用例仍把“未配置”当作“禁用”。现仅给测试账号补 `openai_oauth_responses_websockets_v2_mode=off`，维持“无可用 WS 账号”的测试意图；未改变生产调度器。此用例已在最终回归通过。单元 `codex164-baseline-scheduler.service`；日志 `baseline-scheduler.log`；命令为 `go test -json -p 2 -tags=unit ./internal/service -run '^TestOpenAIGatewayService_SelectAccountWithScheduler_RequiredWSV2_NoAvailableAccount$' -count=1 -timeout=1m`。

本机 CLI 实测：单元 `codex164-cli-full.service`，限制 3 分钟/1 GiB/1 CPU；执行 `/bin/bash /tmp/codex164-alignment.ymc5RW/isolated-cli.sh`，其在相同无外网、空白 HOME 条件启动 Python 本地 HTTP mock 和已安装 CLI；CLI 子进程超时 60 秒。输出目录为 `cli-capture`，总日志 `cli-full.log`。这里的 API Key 和响应均为测试构造值。

## 剩余边界与维护注意

没有真实账号测试、线上配置检查、实际服务部署、TLS/HTTP2 指纹测量或风控/降智因果实验。没有把 HTTP bridge 包装成可真实中断；没有实现 CLI 的完整工具发现、历史恢复与全部功能开关。

本提交包含完整后端源码、测试、文档、脱敏样本及两个小型复现包。本机另外保留 `codex-cli-0.164.0-working-tree.patch`、`codex-cli-0.164.0-validation-logs.tar.gz` 和上述构建二进制，这些生成产物不纳入 Git。补丁包含跟踪文件改动及新增源码/测试，已在未修改的 HEAD 导出目录通过 `git apply --check`。另有[脱敏 CLI 样本结构](codex-cli-0.164.0-local-sample.json)，不含凭据或完整提示词。

本地 `.git/info/exclude` 含 `/backend/`，会隐藏新增后端文件。本次按用户提交要求，定向加入 5 个新增 Go 文件及配套文档和复现材料；未修改忽略规则。原有 `DEV_GUIDE.md` 和 ccvps 部署文件的工作区改动不在本次提交内。

曾观察到 openai_gateway_service.go 被恢复为 HEAD 内容，用户确认未手工修改；没有证据确认触发者。本次重新应用并核对最终版本常量与 Retry-After 代码，不能把这个现象归因于用户或线上服务。2026-10-27 的构建和最终回归均基于包含这些改动的工作区。
