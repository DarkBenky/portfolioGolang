package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	SamplesPerFile = 256
	GENERIC_TEXTS  = iota
	MATH_TEXTS
	CODE_MIX_TEXTS
	OPEN_THOUGHTS_TEXTS
	FINE_BERT_TEXTS
	SCIENCE_TEXTS // ArXiv and other scientific texts
	PATH_TO_SAVE  = "/media/user/2TB/DATA"
)

type TokenizedTexts struct {
	TokenizedTexts [][]int `json:"tokenized_texts"`
	Categories     []int   `json:"categories"`
	Sizes          []int   `json:"sizes"`
}

func saveTokenizedTexts(TokenizedTexts TokenizedTexts, samplesPerFile int) error {
	// Implement the logic to save tokenized texts to a file or database
	return nil
}

func saveData(c echo.Context) error {

}

func health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func main() {
	e := echo.New()

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	e.GET("/health", health)
	e.POST("/save-data", saveDate)

	e.Logger.Fatal(e.Start(":8080"))
}
