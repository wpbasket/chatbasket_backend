-- ===========================================
-- Contacts Queries for sqlc
-- ===========================================

-- name: GetUserContacts :many
-- Retrieves user contacts (people YOU added) with raw restriction data for Go processing
SELECT
    cu.id,
    cu.name,
    cu.b64_cipher_chacha20poly1305_username AS username,
    cu.bio,
    uc.created_at AS contact_created_at,
    
    -- Raw avatar data (Go applies visibility logic)
    a.file_id AS avatar_file_id,
    a.token_id AS avatar_token_id,
    a.token_secret AS avatar_token_secret,
    a.token_expiry AS avatar_token_expiry,
    
    -- Global restriction flags (Priority 1 & 2)
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    
    -- Global exemption flags (Priority 1 & 2 override)
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    
    -- User-level restriction flags (Priority 3 & 4)
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar

FROM user_contacts uc
INNER JOIN users cu 
    ON uc.contact_user_id = cu.id 
    AND cu.is_admin_blocked IS FALSE
    AND cu.profile_type IN ('public', 'personal')
LEFT JOIN avatars a 
    ON cu.id = a.user_id 
    AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions ugr 
    ON cu.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions ugre 
    ON cu.id = ugre.user_id 
    AND ugre.exempted_user_id = $1
LEFT JOIN user_restrictions ur 
    ON cu.id = ur.user_id 
    AND ur.restricted_user_id = $1
WHERE uc.owner_user_id = $1
ORDER BY uc.created_at DESC;


-- ===========================================
-- People Who Added You Query
-- ===========================================

-- name: GetUsersWhoAddedYou :many
-- Retrieves users who have added YOU as a contact with raw restriction data for Go processing
SELECT
    cu.id,
    cu.name,
    cu.b64_cipher_chacha20poly1305_username AS username,
    cu.bio,
    uc.created_at AS contact_created_at,
    
    -- Raw avatar data (Go applies visibility logic)
    a.file_id AS avatar_file_id,
    a.token_id AS avatar_token_id,
    a.token_secret AS avatar_token_secret,
    a.token_expiry AS avatar_token_expiry,
    
    -- Global restriction flags (Priority 1 & 2)
    COALESCE(ugr.restrict_profile, FALSE) AS global_restrict_profile,
    COALESCE(ugr.restrict_avatar, FALSE) AS global_restrict_avatar,
    
    -- Global exemption flags (Priority 1 & 2 override)
    COALESCE(ugre.exception_profile, FALSE) AS exception_global_profile,
    COALESCE(ugre.exception_avatar, FALSE) AS exception_global_avatar,
    
    -- User-level restriction flags (Priority 3 & 4)
    COALESCE(ur.restrict_profile, FALSE) AS user_restrict_profile,
    COALESCE(ur.restrict_avatar, FALSE) AS user_restrict_avatar

FROM user_contacts uc
INNER JOIN users cu 
    ON uc.owner_user_id = cu.id 
    AND cu.is_admin_blocked IS FALSE
    AND cu.profile_type IN ('public', 'personal')
LEFT JOIN avatars a 
    ON cu.id = a.user_id 
    AND a.avatar_type = 'profile'
LEFT JOIN user_global_restrictions ugr 
    ON cu.id = ugr.user_id
LEFT JOIN user_global_restriction_exemptions ugre 
    ON cu.id = ugre.user_id 
    AND ugre.exempted_user_id = $1
LEFT JOIN user_restrictions ur 
    ON cu.id = ur.user_id 
    AND ur.restricted_user_id = $1
WHERE uc.contact_user_id = $1
ORDER BY uc.created_at DESC;


-- ===========================================
-- Avatar Privacy Circuit Breaker Logic
-- ===========================================
-- Both queries return RAW restriction flags for Go to process.
-- Go applies the following priority order (circuit breaker pattern):
--
-- Priority 1: Global PROFILE restriction
--   → If restrict_profile = TRUE AND exception_profile = FALSE → HIDE avatar
--   → If restrict_profile = TRUE AND exception_profile = TRUE → SHOW avatar
--   → Otherwise, continue to Priority 2
--
-- Priority 2: Global AVATAR restriction  
--   → If restrict_avatar = TRUE AND exception_avatar = FALSE → HIDE avatar
--   → If restrict_avatar = TRUE AND exception_avatar = TRUE → SHOW avatar
--   → Otherwise, continue to Priority 3
--
-- Priority 3: User-level PROFILE restriction
--   → If user_restrict_profile = TRUE → HIDE avatar
--   → Otherwise, continue to Priority 4
--
-- Priority 4: User-level AVATAR restriction
--   → If user_restrict_avatar = TRUE → HIDE avatar
--   → Otherwise, SHOW avatar
--
-- Each level short-circuits evaluation (circuit breaker pattern).
-- Privacy checks work identically for both queries because:
--   - cu.id = the contact being viewed (their restrictions apply)
--   - $1 = the viewer (you, checking if you're restricted/exempted)
--   - Direction of contact relationship doesn't affect privacy logic
-- ===========================================


