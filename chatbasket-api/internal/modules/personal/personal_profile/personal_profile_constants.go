package personal_profile

import "time"

// R2 Presigned URL lifetime (per spec §3.D — 15 minutes)
const r2PresignedURLLifetime = 15 * time.Minute

// Pending avatar upload TTL (per spec §4.A — 2 hours)
const pendingAvatarUploadTTL = 2 * time.Hour
