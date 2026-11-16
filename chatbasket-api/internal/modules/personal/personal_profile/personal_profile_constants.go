package personal_profile

import "time"

// R2 Presigned URL lifetime (15 minutes)
const r2PresignedURLLifetime = 15 * time.Minute

// Pending avatar upload TTL (2 hours)
const pendingAvatarUploadTTL = 2 * time.Hour
