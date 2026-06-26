package services

// GlobalService holds all global service dependencies that modules need.
type GlobalService struct {
	CORSOrigin string
}

// NewGlobalService creates a new GlobalService instance with all dependencies.
func NewGlobalService(corsOrigin string) *GlobalService {
	return &GlobalService{
		CORSOrigin: corsOrigin,
	}
}
