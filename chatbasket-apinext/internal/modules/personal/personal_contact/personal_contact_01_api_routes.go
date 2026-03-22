package personal_contact

import (
	"chatbasket-apinext/internal/platform/middleware"

	"github.com/labstack/echo/v5"
)

// Register initializes the Contact module dependencies and registers its routes.
func Register(personalGroup *echo.Group, contactService *contactService, authProvider middleware.AuthSessionProvider) {
	handler := newContactHandler(contactService)

	// Apply Auth Middleware to all personal routes
	// Note: personalGroup already has AuthSessionMiddleware applied in routes.go
	
	// Contact Routes
	contacts := personalGroup.Group("/contacts")
	
	contacts.GET("", handler.GetContacts)
	contacts.POST("/check", handler.CheckContactExistance)
	contacts.POST("/add", handler.CreateContact)
	contacts.POST("/accept", handler.AcceptContactRequest)
	contacts.POST("/reject", handler.RejectContactRequest)
	contacts.POST("/undo", handler.UndoContactRequest)
	contacts.POST("/nickname/update", handler.UpdateContactNickname)
	contacts.POST("/nickname/remove", handler.RemoveContactNickname)
	contacts.POST("/delete", handler.DeleteContact)
	contacts.POST("/block", handler.BlockUser)
	contacts.GET("/requests", handler.GetContactRequests)
}
