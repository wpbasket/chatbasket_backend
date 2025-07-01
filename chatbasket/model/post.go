package model

// Post is the full internal model stored in Appwrite
type Post struct {
	Id               string   `json:"$id"`                            // Appwrite document ID
	UserId           string   `json:"userId"`                         // Author's user ID
	Title            string   `json:"title"`                          // Post title
	Content          string   `json:"content,omitempty"`              // Full content
	Images           []string `json:"images,omitempty"`               // Optional image URLs
	VisibleTo        string   `json:"visibleTo"`                      // "public", "private", "followers"
	DisableComments  bool     `json:"disableComments,omitempty"`      // ❌ Prevent commenting
	DisableLikes     bool     `json:"disableLikes,omitempty"`         // ❌ Prevent liking
	IsAdminBlocked   bool     `json:"isAdminBlocked,omitempty"`       // 🚫 Flag to hide from public
	AdminBlockReason string   `json:"adminBlockReason,omitempty"`     // 📝 Reason for block
	CreatedAt        string   `json:"$createdAt,omitempty"`           // Created timestamp
	UpdatedAt        string   `json:"$updatedAt,omitempty"`           // Updated timestamp
}

// PublicPost is the version of Post shown to frontend clients
type PublicPost struct {
	Id              string            `json:"id"`                     // Post ID
	Title           string            `json:"title"`                  // Title
	Content         string            `json:"content"`                // Full content
	Images          []string          `json:"images,omitempty"`       // Images
	Author          PreviewPublicUser `json:"author"`                 // Author summary
	CreatedAt       string            `json:"createdAt"`              // When posted
	CommentCount    int64             `json:"commentCount"`           // Total comments
	DisableComments bool              `json:"disableComments"`        // Whether comments are allowed
	DisableLikes    bool              `json:"disableLikes"`           // Whether likes are allowed
}

// ToPublicPost maps internal Post and author data to PublicPost for frontend
func ToPublicPost(post Post, author PreviewPublicUser, commentCount int64) PublicPost {
	return PublicPost{
		Id:              post.Id,
		Title:           post.Title,
		Content:         post.Content,
		Images:          post.Images,
		Author:          author,
		CreatedAt:       post.CreatedAt,
		CommentCount:    commentCount,
		DisableComments: post.DisableComments,
		DisableLikes:    post.DisableLikes,
	}
}

// ✅ Check if the post is blocked by an admin
func IsPostBlockedByAdmin(post Post) bool {
	return post.IsAdminBlocked
}

// ✅ Can a user comment on this post?
func CanUserComment(post Post) bool {
	return !post.IsAdminBlocked && !post.DisableComments
}

// ✅ Can a user like this post?
func CanUserLike(post Post) bool {
	return !post.IsAdminBlocked && !post.DisableLikes
}
