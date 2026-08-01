package personal_contact

import (
	"net/http"

	rpc_personal_contactv1connect "chatbasket-api/gen/proto/personal/personal_contact/rpc_personal_contactv1connect"

	"github.com/labstack/echo/v5"
)

// Register initializes the Contact module dependencies and registers its routes.
func Register(personalGroup *echo.Group, contactService *contactService) {
	handler := newContactHandler(contactService)

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
	contacts.POST("/remove-nickname", handler.RemoveContactNickname)

	// Block Routes
	blocks := contacts.Group("/blocks")
	blocks.GET("/get", handler.GetBlocks)
	blocks.POST("/create", handler.BlockUser)

	// Connect RPC Routes
	connectServer := newContactConnectServer(contactService)
	path, connectHandler := rpc_personal_contactv1connect.NewContactServiceHandler(
		connectServer,
	)
	personalGroup.Any(path+"*", echo.WrapHandler(http.StripPrefix("/api/personal", connectHandler)))
}
