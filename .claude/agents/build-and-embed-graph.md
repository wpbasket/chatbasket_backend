---
name: build-and-embed-graph
description: Build code review knowledge graph using Python and embed it using project MCP
trigger: Use when the user wants to build or rebuild the code review graph with embeddings
model: sonnet
---

# Build and Embed Code Review Graph

Build the code review knowledge graph and embed it for semantic search.

## Steps

1. **Build the graph** using Python command:
   - Run: `python -m code_review_graph build` via Bash tool
   - This parses the codebase and creates nodes and edges

2. **Embed the graph** using project MCP tool:
   - **STRICT REQUIREMENT**: MUST use `mcp__code-review-graph__embed_graph_tool` from project .mcp.json
   - DO NOT use any built-in MCP tools
   - DO NOT use Python command for embedding
   - Set repo_root parameter to the current working directory if needed
   - This enables semantic search capabilities

## Requirements

- **Build**: MUST use `python -m code_review_graph build` via Bash tool (not MCP tool)
- **Embed**: MUST use `mcp__code-review-graph__embed_graph_tool` from project .mcp.json (not built-in MCP, not Python command)
- These requirements are defined in CLAUDE.md and override any default behavior

## Output

Report the results:
- Number of files parsed
- Number of nodes and edges created
- Number of embeddings generated
- Confirm semantic search is active
