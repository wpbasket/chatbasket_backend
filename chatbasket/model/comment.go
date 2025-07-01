package model

// 🔐 Comment is the internal model stored in Appwrite
type Comment struct {
	Id             string   `json:"$id"`                      // Appwrite document ID
	PostId         string   `json:"postId"`                   // Linked post ID
	UserId         string   `json:"userId"`                   // Commenter's user ID
	Content        string   `json:"content,omitempty"`           // Comment content
	Images         []string `json:"images,omitempty"`         // Optional attached images
	BlockedByOwner bool     `json:"blockedByOwner,omitempty"` // ✅ Hidden by post owner
	BlockedByAdmin bool     `json:"blockedByAdmin,omitempty"` // ✅ Hidden by admin/moderator
	CreatedAt      string   `json:"$createdAt,omitempty"`     // Timestamp
	UpdatedAt      string   `json:"$updatedAt,omitempty"`     // Last edit time
}

// 🌐 PublicComment is the version sent to frontend with author info
type PublicComment struct {
	Id        string            `json:"id"`           // Comment ID
	Content   string            `json:"content"`         // Comment content
	Images    []string          `json:"images,omitempty"`
	Author    PreviewPublicUser `json:"author"`       // Minimal author info
	CreatedAt string            `json:"createdAt"`    // Timestamp
	UpdatedAt string            `json:"updatedAt"`    // Last edit time
}

// 🔁 ToPublicComment transforms internal comment + author to frontend version
func ToPublicComment(comment Comment, author PreviewPublicUser) PublicComment {
	return PublicComment{
		Id:        comment.Id,
		Content:   comment.Content,
		Images:    comment.Images,
		Author:    author,
		CreatedAt: comment.CreatedAt,
		UpdatedAt: comment.UpdatedAt,
	}
}

// 🛡️ IsCommentBlocked checks if comment is blocked (by admin or post owner)
func IsCommentBlocked(c Comment) bool {
	return c.BlockedByOwner || c.BlockedByAdmin
}
