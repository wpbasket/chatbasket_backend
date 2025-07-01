package model

// Like represents a like on a post or comment
type Like struct {
	Id         string `json:"$id"`                  // Appwrite document ID
	UserId     string `json:"userId"`               // Who liked
	TargetId   string `json:"targetId"`             // Post or comment ID
	TargetType string `json:"targetType"`           // "post" or "comment"
	CreatedAt  string `json:"$createdAt,omitempty"` // When the like happened
	UpdatedAt  string `json:"$updatedAt,omitempty"` // When the like happened
}

// PublicLikeCount is returned to frontend with count + current user’s status
type PublicLikeCount struct {
	TargetId    string `json:"targetId"`    // Post or comment ID
	TargetType  string `json:"targetType"`  // "post" or "comment"
	LikeCount   int64  `json:"likeCount"`   // Total likes
	LikedByUser bool   `json:"likedByUser"` // Whether the logged-in user liked it
}

// LikeWithUser is returned when showing who liked something
type PublicLike struct {
	Id        string            `json:"id"`        // Like ID
	TargetId  string            `json:"targetId"`  // Post/comment ID
	User      PreviewPublicUser `json:"user"`      // Minimal user info
	CreatedAt string            `json:"createdAt"` // Like time
}

// ToLikeWithUser combines Like data with the user’s preview info
func ToPublicLike(like Like, user PreviewPublicUser) PublicLike {
	return PublicLike{
		Id:        like.Id,
		TargetId:  like.TargetId,
		User:      user,
		CreatedAt: like.CreatedAt,
	}
}

