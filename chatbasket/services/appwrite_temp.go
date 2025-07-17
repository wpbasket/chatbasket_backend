package services

import (
	"chatbasket/appwriteinternal"
)

type GlobalService struct {
	Appwrite *appwriteinternal.AppwriteService
}

func NewGlobalService(app *appwriteinternal.AppwriteService) *GlobalService {
	return &GlobalService{Appwrite: app}
}