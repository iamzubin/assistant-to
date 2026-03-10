# assistant-to vs overstory: Feature Comparison

## Focus: Low Context Bloat

This document compares assistant-to (our Go-based swarm orchestrator) with overstory (TypeScript/Bun) to identify features we may be missing that could help reduce context bloat.

---

## Current assistant-to Features

| Feature | Status |
|---------|--------|
| SQLite mailbox (mail_send, mail_list, mail_check) | ✅ |
| Git worktree isolation | ✅ |
| Task lifecycle (pending → started → scouted → building → review → merging → complete) | ✅ |
| Agent roles: Scout, Builder, Reviewer, Merger | ✅ |
| Coordinator orchestrator | ✅ |
| TUI dashboard (dash) | ✅ |
| Worktree merge/teardown | ✅ |
| Session management (tmux) | ✅ |
| Agent spawning via MCP tools | ✅ |
| Task CRUD operations | ✅ |

---

## Missing Features (from overstory)

### High Priority for Context Bloat

| Feature | overstory | assistant-to | Impact |
|---------|-----------|--------------|--------|
| **Mail ingestion/storage** | SQLite WAL with typed protocol, broadcast | Basic mail | HIGH - Currently mail bloat is a known issue (#5 was filed) |
| **Checkpoint/compaction** | Session checkpoint save/restore for compaction survivability | None | HIGH - Could reduce context by checkpointing and resuming |
| **Context priming (prime)** | `ov prime --compact` loads compacted context | None | HIGH - Would help keep context lean |
| **Token instrumentation** | Extracts metrics from runtime transcript JSONL | None | MEDIUM - Would help track/context usage |

### Medium Priority

| Feature | overstory | assistant-to | Impact |
|---------|-----------|--------------|--------|
| **Tiered watchdog** | Tier 0 (daemon), Tier 1 (AI triage), Tier 2 (monitor agent) | Basic session monitoring | MEDIUM - Could auto-recover stalled agents without manual intervention |
| **Task groups** | Batch tracking with auto-close | Single tasks only | LOW - Nice to have for批量 |
| **Run management** | Orchestration run lifecycle (start/complete) | Implicit | LOW - Would help group work sessions |
| **Doctor/health checks** | 11 categories of health checks | None | LOW - Would help debug issues |

### Nice to Have

| Feature | overstory | assistant-to | Impact |
|---------|-----------|--------------|--------|
| **Multiple runtime adapters** | Claude, Pi, Copilot, Codex, Gemini, Sapling, OpenCode | OpenCode only | LOW - We only support OpenCode |
| **Gateway providers** | Route via z.ai, OpenRouter | Direct only | LOW - Not critical |
| **Shell init delay config** | Configurable shell startup delay | None | LOW - Workaround for slow shells |
| **Worktree clean** | Remove completed worktrees | Manual | LOW - Convenience |

---

## Observability Gap

overstory has rich observability that could help debug context issues:

| Tool | overstory | assistant-to |
|------|-----------|--------------|
| `ov trace` | Agent/task timeline | None |
| `ov errors` | Aggregated error view | None |
| `ov replay` | Interleaved chronological replay | None |
| `ov feed` | Unified real-time event stream | None |
| `ov logs` | NDJSON log query | None |
| `ov costs` | Token/cost analysis | None |
| `ov metrics` | Session metrics | None |
| `ov run list/show` | Orchestration run details | None |

---

## Recommendations for Low Context Bloat

1. **Implement mail ingestion** - Already started (#5), need to complete
2. **Add checkpoint system** - Save/restore agent state to avoid re-sending context
3. **Add `prime --compact`** - Load minimal context for resuming agents
4. **Add token instrumentation** - Track context usage per agent
5. **Add watchdog** - Auto-recover stalled agents (reduces need for manual monitoring)

---

## Summary

assistant-to is simpler by design (Go vs TypeScript), but missing key features for context management. The biggest gaps for low context bloat are:
- Mail storage/ingestion (in progress)
- Checkpoint/compaction
- Context priming with compaction
- Token instrumentation

overstory trades complexity for robustness - 1,142 commits vs our ~20. However, for a leaner implementation, we could adopt just the high-priority items above.
