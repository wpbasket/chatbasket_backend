package handler

import "chatbasket/services"

type PostHandler struct {
	Service *services.GlobalService
}

func NewPostHandler(service *services.GlobalService) *PostHandler {
	return &PostHandler{Service: service}
}

