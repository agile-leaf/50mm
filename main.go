package main

import (
	"crypto/subtle"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
)

const DEBUG = true

var app *App
var templates *template.Template

type AuthCredentialsProvider interface {
	GetAuthUser() string
	GetAuthPass() string
}

type BasePageContext struct {
	SiteUrl      string
	CanonicalUrl string

	MetaTitle string
	SiteTitle string
}

type IndexPageContext struct {
	*BasePageContext

	Albums []*Album
}

type ImagePageContext struct {
	*BasePageContext

	Photo      Renderable
	Slug       string
	AlbumTitle string
}

type AlbumPageContext struct {
	*BasePageContext

	AlbumTitle string

	Photos                 []Renderable
	NumImagesToLoadAtStart int

	OgPhoto Renderable // OpenGraph image meta tag
}

func executeTemplateHelper(w io.Writer, templateName string, ctx interface{}) {
	if DEBUG {
		tmpl := template.Must(template.ParseFiles(fmt.Sprintf("templates/%s", templateName)))
		tmpl.Execute(w, ctx)
	} else {
		templates.ExecuteTemplate(w, templateName, ctx)
	}
}

func handleImagePage(slug string, album *Album, w http.ResponseWriter, r *http.Request) {
	if album.HasAuth() && !checkAndRequireAuth(w, r, album) {
		return
	}
	imgUrl := album.site.GetPhotoForKey(album.BucketPrefix + slug)

	ctx := &ImagePageContext{
		&BasePageContext{
			album.site.GetCanonicalUrl().String(),
			album.GetCanonicalUrl().String(),
			album.MetaTitle,
			album.site.SiteTitle,
		},
		imgUrl,
		slug,
		album.AlbumTitle,
	}
	executeTemplateHelper(w, "photo.html", ctx)
}

func handleAlbumPage(album *Album, w http.ResponseWriter, r *http.Request) {
	if album.HasAuth() && !checkAndRequireAuth(w, r, album) {
		return
	}

	if albumOrdering, err := album.GetOrderedPhotos(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	} else {
		imageUrls := albumOrdering.Ordering
		ctx := &AlbumPageContext{
			&BasePageContext{
				album.site.GetCanonicalUrl().String(),
				album.GetCanonicalUrl().String(),
				album.MetaTitle,
				album.site.SiteTitle,
			},
			album.AlbumTitle,
			imageUrls,
			10,
			nil,
		}
		if coverPhoto, err := album.GetCoverPhoto(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		} else {
			ctx.OgPhoto = coverPhoto
		}
		executeTemplateHelper(w, "album.html", ctx)
	}
}

func handleAlbumsIndex(site *Site, w http.ResponseWriter, r *http.Request) {
	ctx := &IndexPageContext{
		&BasePageContext{
			site.GetCanonicalUrl().String(),
			site.GetCanonicalUrl().String(),
			site.MetaTitle,
			site.SiteTitle,
		},

		site.GetAlbumsForIndex(),
	}

	executeTemplateHelper(w, "index.html", ctx)
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

		album, err := site.GetAlbumForPath(path)
		if err != nil {
			// path isn't an album; see if it's an album + image
			i := strings.LastIndex(path, "/") + 1
			albumPath := path[:i]
			slug := path[i:]

			album, err = site.GetAlbumForPath(albumPath)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(err.Error()))
				return
			}

			if album.ImageExists(slug) {
				handleImagePage(slug, album, w, r)
				return
			}

			// Couldn't find the image in this album...just redirect to album
			http.Redirect(w, r, albumPath, http.StatusMovedPermanently)
			return
		}
		// Redirect to canonical album page (with trailing slash) if necessary
		if path[len(path)-1] != '/' {
			http.Redirect(w, r, path+"/", http.StatusMovedPermanently)
			return
		}
		handleAlbumPage(album, w, r)
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
	templates = template.Must(template.ParseFiles("templates/album.html"))

	http.HandleFunc("/", siteHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static/"))))

	fmt.Printf("Starting server at port %s\n", app.port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", app.port), nil); err != nil {
		fmt.Printf("Unable to start server. Error: %s\n", err.Error())
	}
}
