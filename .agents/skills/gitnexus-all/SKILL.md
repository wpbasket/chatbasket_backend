---
name: gitnexus-all
description: Full clean and re-index of all member projects for GitNexus and sync cb-group
---

# GitNexus All Projects Indexing

## Instructions
- **MUST FOLLOW**: Do not check the script file; run directly.

// turbo-all
1. Start the comprehensive GitNexus re-analysis for all member projects:
```bash
python3 helper_cb_backend/python/gitnexus_analyze.py --all
```

2. **Full Rebuild** (If specifically requested to clean all indices):
```bash
python3 helper_cb_backend/python/gitnexus_rebuild.py --all
```
