package main

import (
	"crypto/subtle"
	"fmt"
	"html/template"
	"net/http"
)

var app *App
var templates *template.Template

type AuthCredentialsProvider interface {
	GetAuthUser() string
	GetAuthPass() string
}

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
	if album.HasAuth() && !checkAndRequireAuth(w, r, album) {
		return
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

func handleAlbumsIndex(site *Site, w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Albums List"))
}

func siteHandler(w http.ResponseWriter, r *http.Request) {
	domain := r.Host
	path := r.URL.Path

	if site, err := app.SiteForDomain(domain); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else {
		if site.HasAlbumIndex && path == "/" {
			if site.HasAuth() && !checkAndRequireAuth(w, r, site) {
				return
			}

			handleAlbumsIndex(site, w, r)
			return
		}

		if album, err := site.GetAlbumForPath(path); err != nil {
			// If the path doesn't have a / at it's end, try to see if we can find an album after adding the /
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

func checkAndRequireAuth(w http.ResponseWriter, r *http.Request, provider AuthCredentialsProvider) bool {
	if u, p, ok := r.BasicAuth(); !ok || u != provider.GetAuthUser() || subtle.ConstantTimeCompare([]byte(p), []byte(provider.GetAuthPass())) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="You need a username/password to access this page"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized\n"))
		return false
	}
	return true
}

func main() {
	app = NewApp()
	templates = template.Must(template.ParseFiles("templates/home.html"))

	http.HandleFunc("/", siteHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))

	fmt.Printf("Starting server at port %s\n", app.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", app.port), nil); err != nil {
		fmt.Printf("Unable to start server. Error: %s\n", err.Error())
	}
}
