---
description: Generate GitNexus Wiki for the backup_cb_docs_references project using local LLM
---

// turbo-all
1. Generate the Wiki:
```powershell
cd w:\codewp\cb\backup_cb_docs_references
npx gitnexus wiki --provider local --base-url http://localhost:20128/v1 --api-key sk-5e1066bd350d68bb-fbgd7h-f098ffef --model my_combo1
```
