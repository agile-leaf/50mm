package main

import (
	"fmt"
	"html/template"
	"net/http"
)

var app *App
var templates *template.Template

func homeHandler(w http.ResponseWriter, r *http.Request) {
	domain := r.Host
	if site, err := app.SiteForDomain(domain); err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
	} else {
		if imageUrls, err := site.GetAllImageUrls(); err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		} else {
			type HomeContext struct {
				Title     string
				ImageUrls []string
			}

			ctx := &HomeContext{
				site.MetaTitle,
				imageUrls,
			}
			templates.ExecuteTemplate(w, "home.html", ctx)
		}
	}
}

func main() {
	app = NewApp()
	templates = template.Must(template.ParseFiles("templates/home.html"))

	http.HandleFunc("/", homeHandler)

	fmt.Printf("Starting server at port %s\n", app.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", app.port), nil); err != nil {
		fmt.Printf("Unable to start server. Error: %s\n", err.Error())
	}
}
