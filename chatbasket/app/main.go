package main

import (
	"chatbasket/routes"
	"net/http"

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
		AllowOrigins: []string{"http://localhost:8081"},
		// AllowOrigins: []string{"https://chatbasket.me"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		// AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "x-api-key"},
		AllowCredentials: true,
	}))

	err := godotenv.Load("../.env")
	if err != nil {
		e.Logger.Fatal("Error loading .env file", err)
	}

	routes.RegisterRoutes(e)

	e.GET("/hello", hello)
	e.Logger.Fatal(e.Start(":8000"))
}

func hello(c echo.Context) error {
	return c.String(http.StatusOK, "Hello")
}
