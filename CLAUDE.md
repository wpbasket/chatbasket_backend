### Core Principles

1. **Think Before Coding**: Don't assume. Don't hide confusion. Surface tradeoffs.
   - State your assumptions explicitly. If uncertain, ask.
   - If multiple interpretations exist, present them - don't pick silently.
   - If a simpler approach exists, say so. Push back when warranted.
   - If something is unclear, stop. Name what's confusing. Ask.
2. **Simplicity First**: Minimum code that solves the problem. Nothing speculative.
   - No features beyond what was asked.
   - No abstractions for single-use code.
   - No "flexibility" or "configurability" that wasn't requested.
   - No error handling for impossible scenarios.
   - If you write 200 lines and it could be 50, rewrite it.
   - Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.
3. **Surgical Changes**: Touch only what you must. Clean up only your own mess.
   - When editing existing code:
     - Don't "improve" adjacent code, comments, or formatting.
     - Don't refactor things thant aren't broken.
     - Match existing style, even if you'd do it differently.
     - If you notice unrelated dead code, mention it - don't delete it.
   - When your changes create orphans:
     - Remove imports/variables/functions that YOUR changes made unused.
     - Don't remove pre-existing dead code unless asked.
   - The test: Every changed line should trace directly to the user's request.
4. **Goal-Driven Execution**: Define success criteria. Loop until verified.
   - Transform tasks into verifiable goals:
     - "Add validation" → "Write tests for invalid inputs, then make them pass"
     - "Fix the bug" → "Write a test that reproduces it, then make it pass"
     - "Refactor X" → "Ensure tests pass before and after"
   - For multi-step tasks, state a brief plan:
     ```
     1. [Step] → verify: [check]
     2. [Step] → verify: [check]
     3. [Step] → verify: [check]
     ```
     Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.
5. **Semantic Preference**: Utilize tools with **semantic embeddings** (e.g., GitNexus `query`) for conceptual discovery whenever the specific task permits.
6. **Tiered Repository Discovery**: Utilize **group-level queries** (e.g., `gitnexus_group_query`) against **`cb-group`** for global discovery and cross-repo architectural queries. For deep implementation, refactoring, or impact analysis, switch to the specific **sub-repo index** (e.g., `chatbasket_backend`) to ensure maximum precision and local context.
7. **Strategic Documentation**: If Wiki is present, then utilize the **GitNexus Wiki** (located in sub-repo `.gitnexus/wiki/` folders) when the task requires high-level architectural understanding, module mapping, or strategic context before deep-diving into code.
8. **Context-mode MCP Rules**: [MUST READ] (See full rules at the bottom of this file)
9. **Manual Route Overrides**: Due to GitNexus Go parser limitations (Echo struct method handlers) and custom React Native HTTP client wrappers, automatic API contract linking is skipped. When adding new backend/frontend endpoints, you **MUST** manually add them as overrides in `~/.gitnexus/groups/cb-group/group.yaml` under `links:` and run `npx gitnexus group sync cb-group --allow-stale`.


---

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **chatbasket_backend** (3603 symbols, 9042 relationships, 273 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> Index stale? Run `node .gitnexus/run.cjs analyze` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? `npx gitnexus analyze` (npm 11 crash → `npm i -g gitnexus`; #1939).

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows. For regression review, compare against the default branch: `detect_changes({scope: "compare", base_ref: "main"})`.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `query({search_query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `context({name: "symbolName"})`.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method without first running `impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit changes without running `detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/chatbasket_backend/context` | Codebase overview, check index freshness |
| `gitnexus://repo/chatbasket_backend/clusters` | All functional areas |
| `gitnexus://repo/chatbasket_backend/processes` | All execution flows |
| `gitnexus://repo/chatbasket_backend/process/{name}` | Step-by-step execution trace |

## Cross-Repo Groups

This repository is listed under GitNexus **group(s): cb-group** (see `~/.gitnexus/groups/`). For cross-repo analysis, use MCP tools `impact`, `query`, and `context` with `repo` set to `@<groupName>` or `@<groupName>/<memberPath>` (paths match keys in that group’s `group.yaml`). Use `group_list` / `group_sync` for membership and sync. From the project root: `node .gitnexus/run.cjs group list`, `node .gitnexus/run.cjs group sync <name>`, `node .gitnexus/run.cjs group impact <name> --target <symbol> --repo <group-path>` (the `.gitnexus/run.cjs` path is repo-root-relative).

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

---

# context-mode — MANDATORY routing rules

context-mode MCP tools available. Rules protect context window from flooding. One unrouted command dumps 56 KB into context. Some agents have routing via hooks (`tool_call` blocks `curl`/`wget`) AND these instructions. Hooks = hard enforcement; rules = completeness for redirections hooks cannot catch. Follow strictly.

## Available MCP Tools (11 tools)

1. `ctx_batch_execute`: Run multiple commands in ONE call, auto-index all output, and search with multiple queries.
2. `ctx_doctor`: Diagnose context-mode installation.
3. `ctx_execute`: Execute code in a sandboxed subprocess (javascript, typescript, python, shell, etc.). Only stdout enters context.
4. `ctx_execute_file`: Read a file and process it in a sandboxed subprocess without loading contents into context.
5. `ctx_fetch_and_index`: Fetches URL content, converts HTML to markdown, and indexes into searchable knowledge base.
6. `ctx_index`: Index documentation or knowledge content into a searchable BM25 knowledge base.
7. `ctx_insight`: Opens the context-mode Insight dashboard in the browser to show personal analytics.
8. `ctx_purge`: DESTRUCTIVE — permanently delete indexed content.
9. `ctx_search`: Search indexed content.
10. `ctx_stats`: Returns context consumption statistics for the current session.
11. `ctx_upgrade`: Upgrade context-mode to the latest version.

## Think in Code — MANDATORY

Analyze/count/filter/compare/search/parse/transform data: **write code** via `ctx_execute(language, code)`, `console.log()` only the answer. Do NOT read raw data into context. PROGRAM the analysis, not COMPUTE it. Pure JavaScript — Node.js built-ins only (`fs`, `path`, `child_process`). `try/catch`, handle `null`/`undefined`. One script replaces ten tool calls.

## BLOCKED — do NOT use

### curl / wget — FORBIDDEN
Do NOT use `curl`/`wget` in `bash` or via generic `run_command`. Dumps raw HTTP into context.
Use: `ctx_fetch_and_index(url, source)` or `ctx_execute(language: "javascript", code: "const r = await fetch(...)")`

### Inline HTTP — FORBIDDEN
No `node -e "fetch(..."`, `python -c "requests.get(..."`. Bypasses sandbox.
Use: `ctx_execute(language, code)` — only stdout enters context

### Direct web fetching — FORBIDDEN
Raw HTML can exceed 100 KB.
Use: `ctx_fetch_and_index(url, source)` then `ctx_search(queries)`

## REDIRECTED — use sandbox

### Shell / Bash (>20 lines output)
`shell` or `bash` ONLY for: `git`, `mkdir`, `rm`, `mv`, `cd`, `ls`, `npm install`, `pip install`.
Otherwise: `ctx_batch_execute(commands, queries)` or `ctx_execute(language: "shell", code: "...")`

### File reading (for analysis)
Reading to **edit** → native read tools are correct. Reading to **analyze/explore/summarize** → `ctx_execute_file(path, language, code)`.

### grep / find / search (large results)
Use `ctx_execute(language: "shell", code: "grep ...")` in sandbox.

## Tool selection

0. **MEMORY**: `ctx_search(sort: "timeline")` — after resume, check prior context before asking user.
1. **GATHER**: `ctx_batch_execute(commands, queries)` — runs all commands, auto-indexes, returns search. ONE call replaces 30+. Each command: `{label: "header", command: "..."}`.
2. **FOLLOW-UP**: `ctx_search(queries: ["q1", "q2", ...])` — all questions as array, ONE call (default relevance mode).
3. **PROCESSING**: `ctx_execute(language, code)` | `ctx_execute_file(path, language, code)` — sandbox, only stdout enters context.
4. **WEB**: `ctx_fetch_and_index(url, source)` then `ctx_search(queries)` — raw HTML never enters context.
5. **INDEX**: `ctx_index(content, source)` — store in FTS5 for later search.

## Parallel I/O batches

For multi-URL fetches or multi-API calls, **always** include `concurrency: N` (1-8):

- `ctx_batch_execute(commands: [3+ network commands], concurrency: 5)` — gh, curl, dig, docker inspect, multi-region cloud queries
- `ctx_fetch_and_index(requests: [{url, source}, ...], concurrency: 5)` — multi-URL batch fetch

**Use concurrency 4-8** for I/O-bound work (network calls, API queries). **Keep concurrency 1** for CPU-bound (npm test, build, lint) or commands sharing state (ports, lock files, same-repo writes).

GitHub API rate-limit: cap at 4 for `gh` calls.

## Output

Write artifacts to FILES — never inline. Return: file path + 1-line description.
Descriptive source labels for `ctx_search(source: "label")`.
Terse like caveman. Technical substance exact. Only fluff die.
Drop: articles, filler (just/really/basically), pleasantries, hedging. Fragments OK. Short synonyms. Code unchanged.
Pattern: [thing] [action] [reason]. [next step]. Auto-expand for: security warnings, irreversible actions, user confusion.

## Session Continuity

Skills, roles, and decisions persist for the entire session. Do not abandon them as the conversation grows.

## Memory

Session history is persistent and searchable. On resume, search BEFORE asking the user:

| Need | Command |
|------|---------|
| What were we working on? | `ctx_search(queries: ["summary"], source: "compaction", sort: "timeline")` |
| What did we decide? | `ctx_search(queries: ["decision"], source: "decision", sort: "timeline")` |
| What NOT to repeat? | `ctx_search(queries: ["rejected"], source: "rejected-approach")` |
| What constraints exist? | `ctx_search(queries: ["constraint"], source: "constraint")` |

DO NOT ask "what were we working on?" — SEARCH FIRST.
If search returns 0 results, proceed as a fresh session.

## ctx commands

| Command | Action |
|---------|--------|
| `ctx stats` | Call `stats` MCP tool, display full output verbatim |
| `ctx doctor` | Call `doctor` MCP tool, run returned shell command, display as checklist |
| `ctx upgrade` | Call `upgrade` MCP tool, run returned shell command, display as checklist |
| `ctx purge` | Call `purge` MCP tool with confirm: true. Warns before wiping knowledge base. |
| `ctx insight` | Call `insight` MCP tool, opens local dashboard for session analytics. |

After /clear or /compact: knowledge base and session stats preserved. Use `ctx purge` to start fresh.

## Windows notes

**PowerShell cmdlets** — Sandbox uses bash. PowerShell cmdlets (`Format-List`, `Get-Culture`, etc.) fail with `command not found`. Wrap with `pwsh -NoProfile -Command "..."`.

**Relative paths** — Sandbox CWD is temp dir, not project root. Convert to absolute paths. Ask user to confirm if unknown.

**Windows drive letters** — Sandbox runs Git Bash / MSYS2. `X:\path` → `/x/path` (lowercase, no `/mnt/`). Never emit `/mnt/<letter>/`.

**Quote paths** — Spaces in paths cause splits. Always double-quote: `rg "symbol" "$REPO_ROOT/some dir/Source"`.

---
