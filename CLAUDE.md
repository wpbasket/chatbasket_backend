### Core Principles
1. **Semantic Preference**: Utilize tools with **semantic embeddings** (e.g., GitNexus `query`) for conceptual discovery whenever the specific task permits.
2. **Tiered Repository Discovery**: Utilize **group-level queries** (e.g., `gitnexus_group_query`) against **`cb-group`** for global discovery and cross-repo architectural queries. For deep implementation, refactoring, or impact analysis, switch to the specific **sub-repo index** (e.g., `chatbasket_backend`) to ensure maximum precision and local context.
3. **Strategic Documentation**: If Wiki is present, then utilize the **GitNexus Wiki** (located in sub-repo `.gitnexus/wiki/` folders) when the task requires high-level architectural understanding, module mapping, or strategic context before deep-diving into code.

---

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **chatbasket_backend** (2957 symbols, 5791 relationships, 251 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/chatbasket_backend/context` | Codebase overview, check index freshness |
| `gitnexus://repo/chatbasket_backend/clusters` | All functional areas |
| `gitnexus://repo/chatbasket_backend/processes` | All execution flows |
| `gitnexus://repo/chatbasket_backend/process/{name}` | Step-by-step execution trace |

## Cross-Repo Groups

This repository is listed under GitNexus **group(s): cb-group** (see `~/.gitnexus/groups/`). For cross-repo analysis, use MCP tools `impact`, `query`, and `context` with `repo` set to `@<groupName>` or `@<groupName>/<memberPath>` (paths match keys in that group’s `group.yaml`). Use `group_list` / `group_sync` for membership and sync. From the terminal: `npx gitnexus group list`, `npx gitnexus group sync <name>`, `npx gitnexus group impact <name> --target <symbol> --repo <group-path>`.

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

## Global Playwright Setup
- Playwright is installed globally on this Windows machine via npm (`playwright@1.59.1`).
- Prefer using the global `playwright` command instead of creating local temporary Playwright installs.
- When requiring Playwright from ad-hoc Node scripts, set `NODE_PATH` to the global npm root first in PowerShell: `$env:NODE_PATH = (npm root -g)`.
- Microsoft Edge is available at: `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`.
