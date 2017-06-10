package main

import (
	"crypto/subtle"
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
		if len(site.AuthUser) > 0 && len(site.AuthPass) > 0 {
			if u, p, ok := r.BasicAuth(); !ok || u != site.AuthUser || subtle.ConstantTimeCompare([]byte(p), []byte(site.AuthPass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="You need a username/password to access this site"`)
				w.WriteHeader(401)
				w.Write([]byte("Unauthorized\n"))
				return
			}
		}

		if imageUrls, err := site.GetAllImageUrls(); err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
		} else {
			type HomeContext struct {
				MetaTitle            string
				SiteTitle            string
				AlbumTitle           string
				LoadAtStartImageUrls []string
				LazyLoadImageUrls    []string
			}

			loadAtStartImageUrls, lazyLoadImageUrls := imageUrls, imageUrls
			if len(imageUrls) > 10 {
				loadAtStartImageUrls = loadAtStartImageUrls[:10]
				lazyLoadImageUrls = lazyLoadImageUrls[10:]
			}
			ctx := &HomeContext{
				site.MetaTitle,
				site.SiteTitle,
				site.AlbumTitle,
				loadAtStartImageUrls,
				lazyLoadImageUrls,
			}
			templates.ExecuteTemplate(w, "home.html", ctx)
		}
	}
}

func main() {
	app = NewApp()
	templates = template.Must(template.ParseFiles("templates/home.html"))

	http.HandleFunc("/", homeHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))

	fmt.Printf("Starting server at port %s\n", app.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", app.port), nil); err != nil {
		fmt.Printf("Unable to start server. Error: %s\n", err.Error())
	}
}
