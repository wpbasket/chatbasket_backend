package main

import (
	"chatbasket/model"
	"chatbasket/routes"
	"net/http"
	"os"
	// "time"

	// "github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
)

func main() {
	e := echo.New()
	e.Logger.SetLevel(log.ERROR)
	// e.HideBanner = true
	// e.Pre(middleware.RemoveTrailingSlash())
	// e.Use(middleware.Recover())
	// e.Use(middleware.RequestID())
	// e.Use(middleware.Secure())
	// e.Use(middleware.GzipWithConfig(middleware.GzipConfig{Level: 5}))
	// e.Use(middleware.BodyLimit("10M"))

	e.Use(middleware.Logger())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		// AllowOrigins: []string{"http://localhost:8081"},
		AllowOrigins: []string{"https://chatbasket.me"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		// AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key"},
		AllowCredentials: true,
	}))

	// err := godotenv.Load("../.env")
	// if err != nil {
	// 	e.Logger.Fatal("Error loading .env file", err)
	// }

	// e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))

	routes.RegisterRoutes(e)

	e.GET("/", hello)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Fallback
	}
	// // HTTP server timeouts for production safety
	// e.Server.ReadHeaderTimeout = 5 * time.Second
	// e.Server.ReadTimeout = 15 * time.Second
	// e.Server.WriteTimeout = 15 * time.Second
	// e.Server.IdleTimeout = 60 * time.Second
	e.Logger.Fatal(e.Start(":" + port))
}

func hello(c echo.Context) error {
	return c.JSON(http.StatusOK, &model.StatusOkay{Status: true, Message: "Hello "})
}
