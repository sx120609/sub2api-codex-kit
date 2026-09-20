# NOTICE

This repository is a modified distribution of [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api), licensed under the GNU Lesser General Public License v3.0. The original copyright and license are preserved in `LICENSE`.

Turn-State harvesting, pooling, and replacement behavior is adapted from [DouDOU-start/codex-state-kit](https://github.com/DouDOU-start/codex-state-kit).

## Modifications in this fork

- Add a gateway-side Codex Turn-State kit: probe, cache, and replace `x-codex-turn-state` for ChatGPT OAuth accounts.
- Extend Codex fingerprint convergence with a sticky 2–3 device pool that mimics `codex-cli`, `codex-app`, and `opencode`, instead of pinning all shared-account traffic onto one device fingerprint.
- Admin API and account UI for kit status, bound token length, and device-pool size.
