# Language

Use **English only** for all user-visible output. Do not reply in any other language.

### Core Principles

1. **Think Before Coding & Zero Assumptions**: Don't assume. Verify facts. Surface tradeoffs.
   - Strictly NO assumptions: inspect and double-check code, types, and dependencies directly from source files before acting.
   - State your assumptions explicitly. If uncertain, ask before guessing.
   - If multiple interpretations exist, present them - don't pick silently.
   - If a simpler approach exists, say so. Push back when warranted.
   - If something is unclear, stop. Name what's confusing. Ask.
2. **Simplicity First**: Direct, complete solutions without unnecessary bloat.
   - Fully solve the problem and handle all real domain edge cases, but do not add unrequested features.
   - Avoid premature abstractions, generic wrappers, or extra config layers for single-use code.
   - Handle realistic state and failure modes; avoid speculative defensive layers for states impossible in the system model.
   - Clear and direct beats clever and over-engineered.
3. **Surgical Changes**: Touch only what you must, but own the full ripple effects.
   - When editing existing code:
     - Don't "improve" adjacent code, comments, or formatting.
     - Don't refactor things that aren't broken (but DO update callers/dependents affected by your changes).
     - Match existing style, even if you'd do it differently.
     - If you notice unrelated dead code, mention it - don't delete it.
   - When your changes create orphans or breaks:
     - Fix all callers and downstream references broken by your change.
     - Remove imports/variables/functions that YOUR changes made unused.
     - Don't remove pre-existing dead code unless asked.
   - The test: Every changed line must trace directly to fulfilling the user's request and handling its direct blast radius.
4. **Goal-Driven Execution & Strict Verification**: Two-phase verification. Immediate compiler/typecheck verification. Real tests only. Zero regressions.
   - **MANDATORY Immediate Compiler/Typecheck Rule**:
     - Whenever you edit or create ANY source or test file in any language or project, you MUST run the appropriate project compiler / typechecker / build verification command (auto-discovered from project configuration, toolchain configs, etc.) **IMMEDIATELY** after saving the edit, and **BEFORE** executing test runners or any subsequent action.
     - Never run test commands on un-typechecked or un-compiled code. If compiler/typecheck verification fails, fix errors first.
   - **Strict Two-Phase Bug Verification Protocol**:
     - **Phase 1 (Before change / Baseline Failure Reproduction)**:
       1. Write a real, non-trivial test asserting the desired behavior/filter/fix against existing code.
       2. Run the project's typecheck / compiler check to verify test types and syntax are 100% valid.
       3. Run the project's test command against the old/buggy production code to confirm it **fails for the expected reason**.
       4. Keep the test in place — do NOT delete or revert tests.
     - **Phase 2 (After change / Verification & Zero Regressions)**:
       1. Modify or implement the production code to resolve the issue or fulfill the requirement.
       2. Run the project's typecheck / compiler check immediately to verify 0 compilation/type errors.
       3. Run the targeted test command to confirm the new test passes cleanly.
       4. Run affected caller and regression suites to confirm zero regressions across all connected flows.
   - **Strict Test Quality & Preservation Standards**:
     - STRICTLY NO dummy, superficial, mock-everything, or trivial pass-through tests. Write actual verifying tests that validate logic under real conditions.
     - Tests must assert real logic, edge cases, error conditions, and realistic payload behaviors.
     - Never delete or weaken tests to bypass failures; only update existing tests when the underlying design or requirement intentionally changes.
     - Never consider a task done unless both the targeted new tests and the regression suite pass cleanly.
   - For multi-step tasks, state a brief plan:
     ```
     1. [Step] → verify: [check]
     2. [Step] → verify: [check]
     3. [Step] → verify: [check]
     ```
     Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.
5. **Semantic Preference**: Utilize tools with **semantic embeddings** (e.g., GitNexus `query`) for conceptual discovery whenever the specific task permits (see the [GitNexus Semantic & Embedding Query Guide](#gitnexus-semantic--embedding-query-guide) below).
6. **Tiered Repository Discovery**: Utilize **group-level queries** (e.g., `gitnexus_group_query`) against **`cb-group`** for global discovery and cross-repo architectural queries. For deep implementation, refactoring, or impact analysis, switch to the specific **sub-repo index** (e.g., `chatbasket_backend`) to ensure maximum precision and local context.
7. **Manual Route Overrides (required for every new endpoint)**: Auto-extraction does NOT detect the frontend ConnectRPC `createClient(Service, rpcApiClient)` pattern (`chatbasket`) nor cross-repo links without a manifest — every proto `rpc` and any custom `fetch`/Echo handler needs a manual entry in `~/.gitnexus/groups/cb-group/group.yaml` under `links:`, then `npx gitnexus group sync cb-group`. Existing file already contains 59 `grpc` links covering all current protos; new endpoints must follow the same shape.
   - gRPC (most common): `contract` is `package.Service/Method` exactly as in the `.proto` (`from` = provider, `to` = consumer, keep `role: provider`):
     ```yaml
     links:
       - from: chatbasket_backend
         to: chatbasket
         type: grpc
         contract: rpc_personal_contact.v1.ContactService/MyNewMethod
         role: provider
     ```
   - HTTP (Echo / fetch wrappers): `contract` is `[METHOD::]path` (explicit method preferred):
     ```yaml
       - from: chatbasket_backend
         to: chatbasket
         type: http
         contract: GET::/api/personal/my-route
         role: provider
     ```


---

# GitNexus Semantic & Embedding Query Guide

GitNexus utilizes a hybrid search index combining **BM25 keyword matching** and **semantic vector embeddings**, merged via **Reciprocal Rank Fusion (RRF)**. This allows you to find concepts, execution flows, and symbol mappings using natural language instead of pure code syntax or grep.

## 1. Single Repository vs. Group Mode Queries

* **Single Repository Queries**: Specify the direct repository name in the `repo` field (e.g. `"<repo_name>"`).
  ```javascript
  query({
    repo: "<repo_name>",
    search_query: "<concept_or_topic_description>"
  })
  ```
* **Group-Level Queries (Cross-Repo)**: Set `repo` to `@<groupName>` (e.g. `@<group_name>`) to query all repositories in the GitNexus group.
  ```javascript
  query({
    repo: "@<group_name>",
    search_query: "<cross_repo_concept_description>"
  })
  ```
* **Group Member-Specific Queries**: Set `repo` to `@<groupName>/<repoPathKey>` (e.g. `@<group_name>/<member_repo_path>`) to target a specific member under the group environment.

## 2. Query Parameters Reference

| Parameter | Type | Required | Description |
|---|---|---|---|
| `search_query` | String | **Yes** | Natural language concept, symptom, error message, or keyword to query. |
| `repo` | String | No | Target repository name or group format (e.g. `@<group_name>`). |
| `goal` | String | No | Explain what you want to accomplish (e.g., `"find the <concept_name> logic"`). Helps vector ranking. |
| `task_context` | String | No | Set the context of the work you are performing (e.g., `"verifying that <logic_flow_name> is referenced"`). Helps vector ranking. |
| `service` | String | No | Path prefix (e.g., `"<service_subdirectory>"`). In group mode, only returns processes/symbols falling under this path prefix. |
| `limit` | Number | No | Maximum number of execution processes (flows) to return. Defaults to 5. |
| `max_symbols` | Number | No | Maximum number of symbols to return per process. Defaults to 10. |
| `include_content` | Boolean | No | If `true`, includes the full source code content of matched symbols inside the query response under a `content` field. Defaults to `false`. |
| `branch` | String | No | Scope to a specific branch index (for multi-branch repositories). Omit for primary branch. |

## 3. Best Practices for Semantic Queries

* **Search by Concept instead of Code Syntax**: Rather than grepping for strict class/method names (e.g. `<ExactSymbolName>`), search for natural language descriptions of the logic (e.g. `"<description_of_behavior>"`). The semantic embedding engine matches synonyms and caller patterns even if names differ.
* **Inspect the Processes First**: Identify relevant execution flows in the returned `processes` list, then drill down into the symbols participating in those flows via `process_symbols`.
* **Utilize `include_content` for Quick Inspection**: If you need to quickly read a symbol's implementation without leaving the search tool or calling `view_file`, set `include_content: true` to get the source code inline.
* **Use `service` to Filter Monorepos or Groups**: When running queries in group mode (`@<group_name>`), pass the specific subdirectory segment (e.g., `service: "<service_subdirectory>"`) to filter out other subprojects and focus the search scope.
* **Follow up with `context`**: Once you have identified a suspect symbol name from `query()`, call `context({ name: "<symbol_name>", repo: "<repo_name>" })` to see its full incoming/outgoing calls, properties, accesses, and process participations.

---

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **chatbasket_backend** (7121 symbols, 18211 relationships, 525 execution flows).

> Index stale? Run `node .gitnexus/run.cjs analyze --index-only` from the project root — it auto-selects an available runner. No `.gitnexus/run.cjs` yet? Bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).

## Always Do

- **MUST run impact before editing.** Use `impact({target: "symbolName", direction: "upstream"})` or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .`; report callers, processes, and risk. Never substitute grep for graph analysis.
- **MUST analyze graph changes before committing.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "main"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "main" --repo .`.
- MUST warn on HIGH/CRITICAL `risk` pre-edit; never use `riskSharedAxes` to waive a HIGH/CRITICAL `risk` warning. Compare File/symbol: MCP File omits axes; Graph-RAG expands File.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- **MUST use `query({search_query: "concept"})` for concepts/flows, `context({name: "symbolName"})` for a named symbol, or `impact` for blast radius, on read-only callers, dependencies, imports, or execution flow.** Graph first; text search only for empty/`UNKNOWN`/literals.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis.

## Resources

| Resource | Use for |
| --- | --- |
| `gitnexus://repo/chatbasket_backend/context` | Codebase overview, check index freshness |
| `gitnexus://repo/chatbasket_backend/clusters` | All functional areas |
| `gitnexus://repo/chatbasket_backend/processes` | All execution flows |
| `gitnexus://repo/chatbasket_backend/process/{name}` | Step-by-step execution trace |

## Cross-Repo Groups

This repository is listed under GitNexus **group(s): cb-group** (see `~/.gitnexus/groups/`). For cross-repo analysis, use MCP tools `impact`, `query`, and `context` with `repo` set to `@<groupName>` or `@<groupName>/<memberPath>` (paths match keys in that group’s `group.yaml`). Use `group_list` / `group_sync` for membership and sync. From the project root: `node .gitnexus/run.cjs group list`, `node .gitnexus/run.cjs group sync <name>`, `node .gitnexus/run.cjs group impact <name> --target <symbol> --repo <group-path>` (the `.gitnexus/run.cjs` path is repo-root-relative).

## CLI

| Task | Read this skill file |
| --- | --- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

## Workflow Command Files

When the user asks to run any workflow file, **check `/personaldata/cb/.agents/commands/` first** — it holds the predefined command files. Follow the file.

List the folder first to find the exact workflow file.
---
name: gitnexus-debugging
description: "Use when the user is debugging a bug, tracing an error, or asking why something fails. Examples: \"Why is X failing?\", \"Where does this error come from?\", \"Trace this bug\""
---

# Debugging with GitNexus

## When to Use

- "Why is this function failing?"
- "Trace where this error comes from"
- "Who calls this method?"
- "This endpoint returns 500"
- Investigating bugs, errors, or unexpected behavior

## Bind the repository first

A root cause traced in the wrong repository is a wrong root cause.

Call `list_repos {}` before the first tool call. With one indexed repository,
use the examples below as written. With more than one, pass `repo` on every
call: an omitted `repo` normally errors, but under an MCP policy with a
configured default it resolves to that default silently. If you cannot tell
which repository is meant, stop and ask. This matters most for `cypher`, whose
statement carries no in-band hint of which database it ran against.

`list_repos` is paginated, so page with `offset: pagination.nextOffset` until
`hasMore` is false before concluding a repository is absent.

A stale index describes the code from before your bug, so refresh before
trusting a trace, and state the repository and index freshness with the
diagnosis.

## Workflow

```
0. list_repos {}                                          → Bind repo
1. query({search_query: "<error or symptom>"})            → Find related execution flows
2. context({name: "<suspect>"})                    → See callers/callees/processes
3. READ gitnexus://repo/{name}/process/{name}                → Trace execution flow
4. cypher({statement: "MATCH path..."})                 → Custom traces if needed
```

> If "Index is stale" → run `node .gitnexus/run.cjs analyze` in terminal.

## Checklist

```
- [ ] list_repos {} — bind repo; explicit repo when >1 indexed, ask if ambiguous
- [ ] Understand the symptom (error message, unexpected behavior)
- [ ] query for error text or related code
- [ ] Identify the suspect function from returned processes
- [ ] context to see callers and callees
- [ ] Trace execution flow via process resource if applicable
- [ ] cypher for custom call chain traces if needed
- [ ] Read source files to confirm root cause
- [ ] State the repository and index freshness with the diagnosis
```

## Debugging Patterns

| Symptom              | GitNexus Approach                                          |
| -------------------- | ---------------------------------------------------------- |
| Error message        | `query` for error text → `context` on throw sites |
| Wrong return value   | `context` on the function → trace callees for data flow    |
| Intermittent failure | `context` → look for external calls, async deps            |
| Performance issue    | `context` → find symbols with many callers (hot paths)     |
| Recent regression    | `detect_changes` to see what your changes affect — pass `worktree` for a linked worktree |
| "How does A reach B?" | `trace` between the two symbols — shortest call chain in one call |

## Tools

**query** — find code related to error:

```
query({search_query: "payment validation error", repo: "my-app"})
→ Processes: CheckoutFlow, ErrorHandling
→ Symbols: validatePayment, handlePaymentError, PaymentException
```

**context** — full context for a suspect:

```
context({name: "validatePayment", repo: "my-app"})
→ Incoming calls: processCheckout, webhookHandler
→ Outgoing calls: verifyCard, fetchRates (external API!)
→ Processes: CheckoutFlow (step 3/7)
```

**cypher** — custom call chain traces. Pass `repo` alongside the statement; the
Cypher text itself names no repository, so the result is unattributable without
it:

```cypher
MATCH path = (a)-[:CodeRelation {type: 'CALLS'}*1..2]->(b:Function {name: "validatePayment"})
RETURN [n IN nodes(path) | n.name] AS chain
```

**trace** — shortest call chain between two symbols ("how does A reach B?"), one call instead of chaining `context` hops:

```
trace({ from: "processCheckout", to: "fetchRates", repo: "my-app" })
→ status: ok, hopCount: 3
→ hops: processCheckout → validatePayment → verifyCard → fetchRates
→ edges: CALLS (1.0), CALLS (0.95), CALLS (1.0)
```

When no path exists, `trace` reports the furthest reachable node — exactly where the chain breaks (dynamic dispatch, reflection, or an external boundary).

## Example: "Payment endpoint returns 500 intermittently"

```
0. list_repos {}
   → total: 2 (my-app, billing-api) — bind my-app explicitly on every call

1. query({search_query: "payment error handling", repo: "my-app"})
   → Processes: CheckoutFlow, ErrorHandling
   → Symbols: validatePayment, handlePaymentError

2. context({name: "validatePayment", repo: "my-app"})
   → Outgoing calls: verifyCard, fetchRates (external API!)

3. READ gitnexus://repo/my-app/process/CheckoutFlow
   → Step 3: validatePayment → calls fetchRates (external)

4. Root cause: fetchRates calls external API without proper timeout
   Repository: my-app  Index: current
```

With a single indexed repository, step 0 returns `total: 1` and the `repo`
argument drops out of every call above.
