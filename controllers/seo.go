package controllers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// swVersion is set at startup and changes on each restart
var swVersion = fmt.Sprintf("%d", time.Now().Unix())

func SEO() (string, *SEOController) {
	return "seo", &SEOController{}
}

type SEOController struct {
	application.Controller
}

func (c *SEOController) Setup(app *application.App) {
	c.Controller.Setup(app)
	auth := app.Use("auth").(*AuthController)

	http.Handle("GET /robots.txt", app.Serve("robots.txt", auth.Optional))
	http.Handle("GET /sitemap.xml", app.ProtectFunc(c.sitemap, auth.Optional))
	http.Handle("GET /manifest.json", app.ProtectFunc(c.manifest, auth.Optional))
	http.Handle("GET /sw.js", app.ProtectFunc(c.serviceWorker, auth.Optional))
	http.Handle("GET /google3c5c81d2e70ab3e1.html", app.Serve("google.html", auth.Optional))
}

func (c SEOController) Handle(r *http.Request) application.Handler {
	c.Request = r
	return &c
}

func (c *SEOController) Version() string {
	return swVersion
}

func (c *SEOController) sitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	c.Render(w, r, "sitemap.xml", nil)
}

func (c *SEOController) manifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	c.Render(w, r, "manifest.json", nil)
}

func (c *SEOController) serviceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Render(w, r, "sw.js", nil)
}
