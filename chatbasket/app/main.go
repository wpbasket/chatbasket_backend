package main

import (
	"chatbasket/appwrite"
	"chatbasket/routes"
	"chatbasket/services"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

func main() {
	e := echo.New()
	e.Logger.SetLevel(log.ERROR)

	e.Use(middleware.Logger())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE},
	}))

	err := godotenv.Load("../.env")
	if err != nil {
		e.Logger.Fatal("Error loading .env file", err)
	}

	as := appwrite.NewAppwriteService(
		os.Getenv("APPWRITE_ENDPOINT"),
		os.Getenv("APPWRITE_PROJECT_ID"),
		os.Getenv("APPWRITE_API_KEY"),
		os.Getenv("APPWRITE_DATABASE_ID"),
		os.Getenv("APPWRITE_USERS_COLLECTION_ID"),
		os.Getenv("APPWRITE_POSTS_COLLECTION_ID"),
		os.Getenv("APPWRITE_COMMENTS_COLLECTION_ID"),
		os.Getenv("APPWRITE_BLOCK_COLLECTION_ID"),
		os.Getenv("APPWRITE_LIKES_COLLECTION_ID"),
		os.Getenv("APPWRITE_FOLLOW_COLLECTION_ID"),
		os.Getenv("APPWRITE_REFRESH_TOKENS_COLLECTION_ID"),
	)
	userService := services.NewUserService(as)

	routes.RegisterRoutes(e, userService)


	e.GET("/hello", hello)
	e.Logger.Fatal(e.Start(":8000"))
}

func hello(c echo.Context) error {
	return c.String(http.StatusOK, "Hello")
}
