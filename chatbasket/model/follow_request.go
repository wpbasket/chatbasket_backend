package model

// FollowRequest represents a request from one user to follow another.
type FollowRequest struct {
    Id          string `json:"$id"`                     // Appwrite document ID
    RequesterId string `json:"requesterId"`             // The user who sent the request
    TargetId    string `json:"targetId"`                // The user who received the request
    Status      string `json:"status"`                  // "pending", "approved", "denied"
    CreatedAt   string `json:"$createdAt,omitempty"`    // Request timestamp
    UpdatedAt   string `json:"$updatedAt,omitempty"`    // Last update time
}

// FollowRequestView is what the frontend receives for the requests list.
type FollowRequestView struct {
    Id        string            `json:"id"`        // The Id of the request itself
    UserId    PreviewPublicUser `json:"userId"`    // The user who sent the request
    Status    string            `json:"status"`    // Will be "pending", "approved", or "denied"    
    CreatedAt string            `json:"createdAt"` // When the request was sent
    UpdatedAt string            `json:"updatedAt"` // Last update time
}


