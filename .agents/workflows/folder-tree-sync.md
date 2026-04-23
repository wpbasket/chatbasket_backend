---
description: Synchronize backend and frontend folder trees in system-map.md
---

This workflow synchronizes the folder tree documentation by running the Python script in helper_cb_backend.

// turbo
1. Delete the existing system-map.md file:
```powershell
Remove-Item -Force w:\codewp\cb\chatbasket_backend\docs\system-map.md -ErrorAction SilentlyContinue
```

// turbo
2. Generate the new system-map.md file:
```powershell
python w:\codewp\cb\helper_cb_backend\python\generate_both_trees.py w:\codewp\cb\chatbasket_backend\docs\system-map.md
```