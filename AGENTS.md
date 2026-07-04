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
     - Don't refactor things that aren't broken.
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
5. **Semantic Preference**: Utilize tools with **semantic embeddings** (e.g., GitNexus `query`) for conceptual discovery whenever the specific task permits (see the [GitNexus Semantic & Embedding Query Guide](#gitnexus-semantic--embedding-query-guide) below).
6. **Tiered Repository Discovery**: Utilize **group-level queries** (e.g., `gitnexus_group_query`) against **`cb-group`** for global discovery and cross-repo architectural queries. For deep implementation, refactoring, or impact analysis, switch to the specific **sub-repo index** (e.g., `chatbasket_backend`) to ensure maximum precision and local context.
7. **Manual Route Overrides**: Due to GitNexus Go parser limitations (Echo struct method handlers) and custom React Native HTTP client wrappers, automatic API contract linking is skipped. When adding new backend/frontend endpoints, you **MUST** manually add them as overrides in `~/.gitnexus/groups/cb-group/group.yaml` under `links:` and run `npx gitnexus group sync cb-group --allow-stale`.


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

This project is indexed by GitNexus as **chatbasket_backend** (3732 symbols, 9423 relationships, 281 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

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

## Workflow

```
1. query({search_query: "<error or symptom>"})            → Find related execution flows
2. context({name: "<suspect>"})                    → See callers/callees/processes
3. READ gitnexus://repo/{name}/process/{name}                → Trace execution flow
4. cypher({statement: "MATCH path..."})                 → Custom traces if needed
```

> If "Index is stale" → run `node .gitnexus/run.cjs analyze` in terminal.

## Checklist

```
- [ ] Understand the symptom (error message, unexpected behavior)
- [ ] query for error text or related code
- [ ] Identify the suspect function from returned processes
- [ ] context to see callers and callees
- [ ] Trace execution flow via process resource if applicable
- [ ] cypher for custom call chain traces if needed
- [ ] Read source files to confirm root cause
```

## Debugging Patterns

| Symptom              | GitNexus Approach                                          |
| -------------------- | ---------------------------------------------------------- |
| Error message        | `query` for error text → `context` on throw sites |
| Wrong return value   | `context` on the function → trace callees for data flow    |
| Intermittent failure | `context` → look for external calls, async deps            |
| Performance issue    | `context` → find symbols with many callers (hot paths)     |
| Recent regression    | `detect_changes` to see what your changes affect           |
| "How does A reach B?" | `trace` between the two symbols — shortest call chain in one call |

## Tools

**query** — find code related to error:

```
query({search_query: "payment validation error"})
→ Processes: CheckoutFlow, ErrorHandling
→ Symbols: validatePayment, handlePaymentError, PaymentException
```

**context** — full context for a suspect:

```
context({name: "validatePayment"})
→ Incoming calls: processCheckout, webhookHandler
→ Outgoing calls: verifyCard, fetchRates (external API!)
→ Processes: CheckoutFlow (step 3/7)
```

**cypher** — custom call chain traces:

```cypher
MATCH path = (a)-[:CodeRelation {type: 'CALLS'}*1..2]->(b:Function {name: "validatePayment"})
RETURN [n IN nodes(path) | n.name] AS chain
```

**trace** — shortest call chain between two symbols ("how does A reach B?"), one call instead of chaining `context` hops:

```
trace({ from: "processCheckout", to: "fetchRates" })
→ status: ok, hopCount: 3
→ hops: processCheckout → validatePayment → verifyCard → fetchRates
→ edges: CALLS (1.0), CALLS (0.95), CALLS (1.0)
```

When no path exists, `trace` reports the furthest reachable node — exactly where the chain breaks (dynamic dispatch, reflection, or an external boundary).

## Example: "Payment endpoint returns 500 intermittently"

```
1. query({search_query: "payment error handling"})
   → Processes: CheckoutFlow, ErrorHandling
   → Symbols: validatePayment, handlePaymentError

2. context({name: "validatePayment"})
   → Outgoing calls: verifyCard, fetchRates (external API!)

3. READ gitnexus://repo/my-app/process/CheckoutFlow
   → Step 3: validatePayment → calls fetchRates (external)

4. Root cause: fetchRates calls external API without proper timeout
```
