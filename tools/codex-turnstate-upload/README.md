# 本地 292 Turn-State 上传工具

本机用 Codex 的 ChatGPT 登录态向 `chatgpt.com` 探测 `x-codex-turn-state`，筛出约 292 字节的 token，再上传到 Sub2API。

实测：在开发机上 4/4 次探测都拿到了 292。服务端必须先有对应的 OpenAI OAuth 账号（`chatgpt_account_id` 一致），否则上传会 404。

```powershell
python tools/codex-turnstate-upload/upload.py --harvest-only
python tools/codex-turnstate-upload/upload.py --server http://155.103.66.233:8081 --email admin@kit.local --password "..."
```

可选 `--account-id` 指定 Sub2API 账号 ID；不传则按本地 `auth.json` 里的 `tokens.account_id` 匹配。
