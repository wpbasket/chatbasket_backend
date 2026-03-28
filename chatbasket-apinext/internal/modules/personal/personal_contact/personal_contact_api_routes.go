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
	
	contacts.GET("/get", handler.GetContacts)
	contacts.POST("/check-existence", handler.CheckContactExistance)
	contacts.POST("/create", handler.CreateContact)
	contacts.POST("/delete", handler.DeleteContact)
	contacts.GET("/requests/get", handler.GetContactRequests)
	contacts.POST("/requests/accept", handler.AcceptContactRequest)
	contacts.POST("/requests/reject", handler.RejectContactRequest)
	contacts.POST("/requests/undo", handler.UndoContactRequest)
	contacts.POST("/update-nickname", handler.UpdateContactNickname)
	contacts.POST("/block", handler.BlockUser)
	contacts.POST("/remove-nickname", handler.RemoveContactNickname)
}
