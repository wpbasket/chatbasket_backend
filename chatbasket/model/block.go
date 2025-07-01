package model

// Block represents a record of one user blocking another.
type Block struct {
	Id        string `json:"$id"`              // Document ID
	BlockerId string `json:"blockerId"`        // Who initiated the block
	BlockedId string `json:"blockedId"`        // Who is being blocked
	Reason    string `json:"reason,omitempty"` // Optional reason (spam, abuse, etc
	CreatedAt string `json:"$createdAt"`       // Timestamp of blocking
	UpdatedAt string `json:"$updatedAt"`       // Timestamp of last update
}


// BlockView is used in frontend to show blocked users with details
type BlockView struct {
	User      PreviewPublicUser `json:"user"`      // The blocked user's public preview
	Reason    string            `json:"reason"`    // Reason for blocking
	BlockedAt string            `json:"blockedAt"` // Timestamp of block
}

// ToBlockView creates a view-ready response from a Block and its user info
func ToBlockView(block Block, user PreviewPublicUser) BlockView {
	return BlockView{
		User:      user,
		Reason:    block.Reason,
		BlockedAt: block.CreatedAt,
	}
}

// ✅ Utility function to check if a user is blocked by another user
func IsUserBlocked(blocks []Block, blockerId, blockedId string) bool {
	for _, block := range blocks {
		if block.BlockerId == blockerId && block.BlockedId == blockedId {
			return true
		}
	}
	return false
}

