---
name: gitnexus-chatbasket-backend
description: Full clean and re-index of the chatbasket_backend project for GitNexus
---

# GitNexus Re-index (chatbasket_backend)

## Instructions
- **MUST FOLLOW**: Do not check the script file; run directly.

// turbo-all
1. Start the comprehensive GitNexus re-analysis:
```bash
python3 helper_cb_backend/python/gitnexus_analyze.py chatbasket_backend
```

2. **Full Rebuild** (If specifically requested to clean the index):
```bash
python3 helper_cb_backend/python/gitnexus_rebuild.py chatbasket_backend
```

3. **Fix broken index** (If `analyze` fails with an index corruption error):
   - FTS index inconsistent (`FTS index 'file_fts' is inconsistent: ... missing during delete`):
   ```bash
   cd chatbasket_backend && npx gitnexus analyze --repair-fts
   ```
   - Duplicate embedding primary key (`Found duplicated primary key value ... violates the uniqueness constraint`):
   ```bash
   cd chatbasket_backend && npx gitnexus analyze --embeddings --drop-embeddings
   ```
