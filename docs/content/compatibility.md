---
title: Compatibility survey
weight: 12
---

Checked **2026-11-11** against primary documentation and upstream sources. This survey drives the generic [connection workflow](../other-apps); it is not a claim that every version of every GUI has been tested. Product names, bundled runtime versions and authentication models change independently.

## What the current ecosystem needs

The recurring surfaces are a native CLI, a vendor SDK that starts a CLI, Codex app-server, and ACP between an editor and an agent. These can share aims' CLI homes without copying logins. API-native runtimes and managed application logins may own different credentials and conversation stores, even when their provider names match.

Upstream adapters increasingly bundle compatible native runtimes. Environment routing preserves those versions; globally replacing `claude` or `codex`, forcing executable overrides into every SDK, or building a second protocol proxy creates avoidable compatibility work. aims therefore offers optional executable links and transparent native/ACP launch commands, with no global PATH edits or automatic token migration.

| Surface examined | Evidence and current integration | Validation |
| --- | --- | --- |
| Codex app-server | The [official protocol](https://learn.chatgpt.com/docs/app-server) provides authentication, history and streamed events to rich clients. Use `aims bridge run codex -- app-server`. | Documented surface; stdio, process and account routing tested with a stand-in. |
| Claude Agent SDK | The [TypeScript reference](https://code.claude.com/docs/en/agent-sdk/typescript) documents bundled native CLI packages and `pathToClaudeCodeExecutable`. Use environment routing by default; a launcher is an intentional executable override. | Documented override; single-executable contract tested. |
| Codex SDK | The [official SDK documentation](https://learn.chatgpt.com/docs/codex-sdk) describes the Python SDK's pinned runtime and `CodexConfig(codex_bin=...)`. Preserve the packaged version unless deliberately overriding it. | Documentation inspected; native launch contract tested. |
| Zed | [External Agents](https://zed.dev/docs/ai/external-agents) use ACP Registry or custom `command`/`args`/`env`; importing existing threads is a separate flow. Use `aims bridge acp <tool>`. | Current configuration shape inspected; ACP launch contract tested. |
| JetBrains and other ACP clients | The [ACP transport](https://agentclientprotocol.com/protocol/v1/transports) specifies stdio JSON-RPC; [session loading](https://agentclientprotocol.com/protocol/v1/session-setup) is negotiated. Reuse the generic ACP command arrays. | Protocol specification inspected; no separate GUI compatibility promise. |
| Cline: Claude Code provider | [Cline's current Anthropic guide](https://docs.cline.bot/provider-config/anthropic.md) explicitly offers a Claude Code subscription mode and a Claude CLI path. Select aims' Claude launcher there. | Provider configuration documented; executable and stream contracts tested. |
| Cline: native OAuth/API and ACP | [OpenAI provider setup](https://docs.cline.bot/provider-config/openai.md) describes Cline's own API/OAuth paths. [Cline ACP](https://docs.cline.bot/usage/acp.md) is its own agent runtime and reuses credentials saved by `cline auth`. These are not automatically the native Codex CLI account. | Separate provider boundaries documented; no credential store changes. |
| Paradev, formerly Superset | The [current upstream README](https://github.com/ParadevAI/Paradev) names Claude Code, Codex and other CLI agents, terminal presets and a retained `superset` CLI name. Use the environment route or an explicit native CLI launcher/prefix appropriate to the preset. | Source/docs inspected; terminal process contract tested, GUI not driven. |
| Vibe Kanban | Its [Codex executor](https://github.com/BloopAI/vibe-kanban/blob/b4d98330da74be564c4a566ce4430c3b8c09f8a9/crates/executors/src/executors/codex.rs) honors `CODEX_HOME` and starts a pinned packaged CLI. [Command overrides](https://github.com/BloopAI/vibe-kanban/blob/b4d98330da74be564c4a566ce4430c3b8c09f8a9/crates/executors/src/command.rs) exist. Prefer `aims exec -- <launcher>` to preserve its package versions; select a local executable override only intentionally. | Executor source inspected; environment contract tested. |
| cmux | The [upstream project](https://github.com/manaflow-ai/cmux) provides terminal-based Claude Code/Codex sessions and Claude teams. Native CLI commands/launchers fit that surface; session UI and team orchestration remain cmux's responsibility. | Source/docs inspected; native process contract tested. |
| T3 Code | Managed Codex accounts can share transcript directories while owning authentication separately. Use the optional `aims bridge t3` existing-CLI export or a supported custom CLI path; import threads through T3. | Installed T3 Code 0.1.8 schemas checked; live managed sessions were not modified. |
| OpenCode and Pi | [OpenCode providers](https://opencode.ai/docs/providers) and [Pi providers](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/providers.md) define their own connections/credential storage. Their native provider modes do not become Claude Code/Codex sessions merely by setting aims' home variables. | Current provider docs inspected; no native auth files modified. |
| Coder Agents | [Current Coder documentation](https://coder.com/docs/ai-coder/agents) says its harness talks directly to the configured LLM provider and is not a Claude Code/Codex wrapper. Configure its own provider, or use a separately supported CLI execution path. | Documented boundary; no automatic login sharing claimed. |
| Cursor SDK and remote runtimes | [Cursor's SDK Bridge](https://cursor.com/docs/sdk/bridge.md) uses its own SDK/auth protocol. Warp's [automation platform documentation](https://docs.warp.dev/_llms-txt/automation-platform.txt) also distinguishes harnesses and execution hosts. A local aims home is not automatically a remote/managed account. | Current docs inspected; no cloud account or host state changed. |

## Upstream changes worth tracking

- The maintained Codex ACP adapter is now [agentclientprotocol/codex-acp](https://github.com/agentclientprotocol/codex-acp); the old zed-industries repository is archived. The current stable release checked was [v2.6.0](https://github.com/agentclientprotocol/codex-acp/releases/tag/v2.6.0), published 2026-11-08. Its `CODEX_PATH` option deliberately overrides the packaged CLI; aims' ACP route leaves it alone.
- The maintained Claude ACP adapter is [agentclientprotocol/claude-agent-acp](https://github.com/agentclientprotocol/claude-agent-acp); stable [v0.105.0](https://github.com/agentclientprotocol/claude-agent-acp/releases/tag/v0.105.0) was published 2026-11-10. Its native override is `CLAUDE_CODE_EXECUTABLE`; aims preserves it. Draft protocol extensions belong to the upstream adapter and its client negotiation.
- Zed's current docs deprecate extension-provided agent installation in favor of ACP Registry, and explicitly distinguish native agent settings from Zed settings.
- Superset is now Paradev. Integration guidance should follow its current repository and docs while recognizing that the CLI name remains `superset`.
- [AgentAPI's upstream README](https://github.com/coder/agentapi) marks it deprecated and recommends Coder Agents or maintained registry modules. It is not a recommended new dependency for aims.

The adapter source revisions inspected were Codex `ab7a968e982a95d8a765c1e6c6682a520d4f7638` and Claude `4a3ff6a1eb16075b96676616ab2bbb98694a4d24`, both current at the checked date. Development branch behavior can precede a stable release. aims installs neither an adapter nor a preview channel automatically.

## What aims actually verifies

Tests compile the real aims executable and run it next to a stand-in native CLI/ACP adapter, inside temporary homes. They cover argument preservation, an initial response while stdin stays open, a large UTF-8 stream byte for byte, separate stderr, Unix PID replacement and signals, exit status, changed preferences for new processes, existing-process stability, per-worker project order, explicit profiles, a desktop client's minimal PATH, custom `AIMS_HOME`, preserved adapter runtime overrides, private databases and credentials, and install/update/uninstall ownership checks. These tests make no model requests or real login changes.

The remaining app-specific question is whether a particular release exposes these connection surfaces and imports sessions as expected. An app that overwrites environment, forces a managed login, or stores conversation metadata elsewhere needs its own supported connection setting. aims reports and documents that boundary instead of merging unknown databases or copying rotating credentials.
