package model

// Follow represents a follower-following relationship between two users.
type Follow struct {
	Id           string `json:"$id"`                     // Appwrite document ID
	FollowerId   string `json:"followerId"`              // The user who is following
	FollowingId  string `json:"followingId"`             // The user who is being followed
	IsMuted      bool   `json:"isMuted,omitempty"`       // Mute the followed user's content
	CreatedAt    string `json:"$createdAt,omitempty"`    // Follow timestamp
	UpdatedAt    string `json:"$updatedAt,omitempty"`    // Optional: for re-follow or tracking
}

// FollowView is what frontend receives for follower/following list
type FollowView struct {
	User       PreviewPublicUser `json:"user"`        // The followed/following user
	FollowedAt string            `json:"followedAt"`  // Follow timestamp
	IsMuted    bool              `json:"isMuted"`     // Whether muted
}

// ToFollowView converts a Follow + user info into a clean API response
func ToFollowView(follow Follow, user PreviewPublicUser) FollowView {
	return FollowView{
		User:       user,
		FollowedAt: follow.CreatedAt,
		IsMuted:    follow.IsMuted,
	}
}
