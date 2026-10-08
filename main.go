// Command db-mastery serves the learning portal.
//
//	go run .            # http://localhost:8080
//	go run . -addr :3000
//	go run . -dev       # serve web/ from disk while editing lessons
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dev := flag.Bool("dev", false, "serve web/ from disk so lesson edits show up without a restart")
	flag.Parse()

	var site fs.FS = os.DirFS("web")
	if !*dev {
		var err error
		if site, err = fs.Sub(webFS, "web"); err != nil {
			log.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", noCache(http.FileServerFS(site)))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("DB Mastery portal running at http://localhost%s", *addr)
	log.Fatal(srv.ListenAndServe())
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		h.ServeHTTP(w, r)
	})
}
