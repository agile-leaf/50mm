package main

import (
	"crypto/subtle"
	"fmt"
	"html/template"
	"net/http"
)

var app *App
var templates *template.Template

type BasePageContext struct {
	SiteUrl string

	MetaTitle string
	SiteTitle string
}

type AlbumsIndexPageContext struct {
	*BasePageContext
	a *Album
}

type AlbumPageContext struct {
	*BasePageContext

	AlbumTitle string

	ImageUrls              []string
	NumImagesToLoadAtStart int

	OgImageUrl string // OpenGraph image meta tag
}

func handleAlbumPage(album *Album, w http.ResponseWriter, r *http.Request) {
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
		ctx := &AlbumPageContext{
			&BasePageContext{
				album.GetCanonicalUrl().String(),
				album.MetaTitle,
				album.site.SiteTitle,
			},
			album.AlbumTitle,
			imageUrls,
			10,
			"",
		}
		if coverPhotoUrl, err := album.GetCoverPhotoUrl(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		} else {
			ctx.OgImageUrl = coverPhotoUrl
		}
		templates.ExecuteTemplate(w, "home.html", ctx)
	}
}

func handleAlbumIndex(site *Site, w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Albums List"))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	domain := r.Host
	path := r.URL.Path

	if site, err := app.SiteForDomain(domain); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else {
		if site.HasAlbumIndex && path == "/" {
			handleAlbumIndex(site, w, r)
			return
		}

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
			handleAlbumPage(album, w, r)
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
