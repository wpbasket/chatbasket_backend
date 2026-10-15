---
name: folder-tree-sync
description: Synchronize backend and frontend folder trees in system-map.md
---

# Folder Tree Sync

## Instructions
- **MUST FOLLOW**: Do not check the script file; run directly.

This workflow synchronizes the folder tree documentation by running the Python script in helper_cb_backend.

// turbo
1. Delete the existing system-map.md file:
```bash
rm -f chatbasket_backend/docs/system-map.md
```

// turbo
2. Generate the new system-map.md file:
```bash
python3 helper_cb_backend/python/generate_both_trees.py chatbasket_backend/docs/system-map.md
```
