package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.DELETE},
	}))
	
	e.GET("/hello", hello)
	e.Logger.Fatal(e.Start(":8000"))
}

func hello(c echo.Context) error {
	return c.String(http.StatusOK, "Hello")
}