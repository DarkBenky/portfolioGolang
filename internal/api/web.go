package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"
)

// mountWeb serves the built single page app from the embedded filesystem. Unknown
// paths without an extension fall back to index.html so client side navigation works.
func (s *Server) mountWeb(e *echo.Echo) {
	if s.Web == nil {
		return
	}
	e.GET("/*", func(c echo.Context) error {
		requested := strings.TrimPrefix(path.Clean(c.Request().URL.Path), "/")
		if requested == "" || requested == "." {
			requested = "index.html"
		}
		file, err := s.Web.Open(requested)
		if err != nil {
			requested = "index.html"
			file, err = s.Web.Open(requested)
			if err != nil {
				return c.String(http.StatusNotFound, "UI not built")
			}
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil || info.IsDir() {
			return c.String(http.StatusNotFound, "Not found")
		}
		if strings.HasSuffix(requested, ".html") {
			c.Response().Header().Set("Cache-Control", "no-cache")
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(c.Response(), c.Request(), requested, info.ModTime(), file)
		return nil
	})
}
