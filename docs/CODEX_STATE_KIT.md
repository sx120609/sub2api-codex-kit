# Codex State Kit 集成

本 fork 把 [Codex State Kit](https://github.com/DouDOU-start/codex-state-kit) 的核心能力嵌进 Sub2API 网关，并增强了 Codex「完全收敛」的设备指纹管理。

ChatGPT OAuth 登录、账号级出口代理仍使用 Sub2API 原有后台，不再捆绑桌面 WARP。

## Turn-State

开启后，网关会：

1. 用账号凭据和绑定代理，向 `https://chatgpt.com/backend-api/codex/responses` 并发探测，读取响应头 `x-codex-turn-state`。
2. 按 Fernet 时间戳解析，排除 308–316 长度的降智 token，优先缓存 292 / 332。
3. **仅当客户端已经回带该头**（后续回合）时，用缓存值替换。首次请求保持不携带。
4. 按模型分池，40 分钟有效、35 分钟预取；账号切换会清空缓存。

探测会消耗上游额度。请先给账号绑定出口代理。

### 打开方式

1. 配置 `gateway.codex_state_kit.enabled: true`（默认开启）。
2. 在后台编辑 OpenAI OAuth 账号，打开 **Codex Turn-State Kit**。
3. 可选绑定长度 292 / 332，或保持自动识别。

## 多设备指纹

原版「完全收敛」把共享 OAuth 账号的所有请求收敛到 **一台设备 + 一个会话 + 一个线程**。流量一大，上游看起来像同一台设备在刷。

现在默认摊到 **3 台虚拟设备**，分别模拟：

| 槽位 | 客户端 | originator | User-Agent 前缀 |
| --- | --- | --- | --- |
| 0 | Codex CLI | `codex_cli_rs` | `codex_cli_rs/{version}` |
| 1 | Codex App | `codex_app` | `codex_app/{version}` |
| 2 | OpenCode | `opencode` | `opencode/{version}` |

也可选 2 台（cli + app）或 1 台（历史单设备）。按 API Key + 客户端 session 粘性散列：同一客户端始终落在同一台设备上。`full` 仍然在每台虚拟设备内收敛会话和线程。

## 配置

```yaml
gateway:
  codex_state_kit:
    enabled: true
    harvest_concurrency: 10
    check_interval_seconds: 30
    default_model: "gpt-5.4"
```

账号 `extra` 字段：

| 键 | 含义 |
| --- | --- |
| `codex_state_kit_enabled` | 是否对本账号采集/替换 Turn-State |
| `codex_state_kit_bound_token_len` | 292 / 332，缺省自动 |
| `codex_fingerprint_mode` | `off` / `device` / `session` / `full` |
| `codex_fingerprint_pool_size` | 虚拟设备数，默认 1 |
