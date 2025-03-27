package pour

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"go.viam.com/rdk/logging"
)

//go:embed web/dist
var staticFS embed.FS

func init() {
	temp, err := staticFS.Open("web/dist/index.html")
	if err != nil {
		panic(err)
	}
	defer temp.Close()
}

func createAndRunWebServer(g *gen, port int, logger logging.Logger) (*http.Server, error) {

	mux := http.NewServeMux()

	fsToUse, err := fs.Sub(staticFS, "web/dist")
	if err != nil {
		return nil, err
	}

	mux.HandleFunc("/help", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("/index.html?host=%s&payload=%s&authEntity=%s", g.address, g.payload, g.entity), http.StatusFound)
	})

	mux.Handle("/", http.FileServerFS(fsToUse))

	webServer := &http.Server{}
	webServer.Handler = &cookieSetter{g, mux}
	webServer.Addr = fmt.Sprintf(":%d", port)

	go func() {
		logger.Infof("starting webserver on %v", webServer.Addr)
		err := webServer.ListenAndServe()
		if err != nil {
			logger.Errorf("ListenAndServe error: %v", err)
		}
	}()

	return webServer, nil
}

type cookieSetter struct {
	g       *gen
	handler http.Handler
}

func (cs *cookieSetter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "host", Value: cs.g.address})
	http.SetCookie(w, &http.Cookie{Name: "authEntity", Value: cs.g.entity})
	http.SetCookie(w, &http.Cookie{Name: "payload", Value: cs.g.payload})
	cs.handler.ServeHTTP(w, r)
}
