package personal_contact

import (
	"chatbasket-api/internal/platform/kit"
	"net/http"

	"github.com/labstack/echo/v5"
)

type contactHandler struct {
	Service *contactService
}

func newContactHandler(service *contactService) *contactHandler {
	return &contactHandler{Service: service}
}

func (h *contactHandler) GetContacts(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	res, err := h.Service.GetContacts(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) CreateContact(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload CreateContactPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.CreateContact(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) RemoveContactNickname(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload RemoveContactNicknamePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.RemoveContactNickname(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) CheckContactExistance(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload CheckContactExistancePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	if payload.ContactUsername == "" {
		return kit.NewError(http.StatusBadRequest, "bad_request", "contact_username is required")
	}

	resp, err := h.Service.CheckContactExistance(
		c.Request().Context(),
		&payload,
		userID,
	)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *contactHandler) AcceptContactRequest(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload AcceptContactRequestPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.AcceptContactRequest(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) RejectContactRequest(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload RejectContactRequestPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.RejectContactRequest(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) DeleteContact(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload DeleteContactPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	if len(payload.ContactUserId) == 0 {
		return kit.NewError(http.StatusBadRequest, "bad_request", "contact_user_id is required")
	}

	res, err := h.Service.DeleteContact(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) UndoContactRequest(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload UndoContactRequestPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.UndoContactRequest(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) GetContactRequests(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	resp, err := h.Service.GetContactRequests(c.Request().Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *contactHandler) UpdateContactNickname(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload UpdateContactNicknamePayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.UpdateContactNickname(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

func (h *contactHandler) BlockUser(c *echo.Context) error {
	userID, err := kit.ExtractUserID(c)
	if err != nil {
		return err
	}

	var payload BlockUserPayload
	if err := c.Bind(&payload); err != nil {
		return kit.NewError(http.StatusBadRequest, "bad_request", "invalid request payload")
	}

	res, err := h.Service.BlockUser(c.Request().Context(), &payload, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

