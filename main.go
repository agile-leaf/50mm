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
	path := r.URL.Path

	if site, err := app.SiteForDomain(domain); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else {
		if album, err := site.GetAlbumForPath(path); err != nil {
			// If the album name doesn't have a / at it's end, try to see if we can find that
			if path[len(path)-1] != '/' {
				if _, err := site.GetAlbumForPath(path + "/"); err == nil {
					// If we did find it, redirect user to there
					http.Redirect(w, r, path+"/", http.StatusMovedPermanently)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(err.Error()))
			return
		} else {
			if len(album.AuthUser) > 0 && len(album.AuthPass) > 0 {
				if u, p, ok := r.BasicAuth(); !ok || u != album.AuthUser || subtle.ConstantTimeCompare([]byte(p), []byte(album.AuthPass)) != 1 {
					w.Header().Set("WWW-Authenticate", `Basic realm="You need a username/password to access this site"`)
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized\n"))
					return
				}
			}

			if imageUrls, err := album.GetAllImageUrls(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
			} else {
				type HomeContext struct {
					SiteUrl string

					MetaTitle  string
					SiteTitle  string
					AlbumTitle string

					LoadAtStartImageUrls []string
					LazyLoadImageUrls    []string

					OgImageUrl string // OpenGraph image meta tag
				}

				loadAtStartImageUrls, lazyLoadImageUrls := imageUrls, []string{}
				if len(imageUrls) > 10 {
					loadAtStartImageUrls = imageUrls[:10]
					lazyLoadImageUrls = imageUrls[10:]
				}
				ctx := &HomeContext{
					album.GetCanonicalUrl(),
					album.MetaTitle,
					site.SiteTitle,
					album.AlbumTitle,
					loadAtStartImageUrls,
					lazyLoadImageUrls,
					"",
				}
				if len(imageUrls) > 0 {
					ctx.OgImageUrl = imageUrls[0]
				}
				templates.ExecuteTemplate(w, "home.html", ctx)
			}
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
