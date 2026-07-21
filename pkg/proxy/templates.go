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

type landingRoute struct {
	Name       string
	URL        string
	Port       string
	ProjectDir string
}

type landingData struct {
	Routes []landingRoute
}

type notFoundData struct {
	Hostname string
	Port     string
}
