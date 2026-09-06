package proxy

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

var (
	landingTmpl  = template.Must(template.ParseFS(templateFS, "templates/landing.html"))
	notFoundTmpl = template.Must(template.ParseFS(templateFS, "templates/notfound.html"))
)

type landingData struct {
	Routes []LandingRoute
}

type notFoundData struct {
	Hostname string
	Port     string
}
