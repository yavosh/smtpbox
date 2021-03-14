package web

import (
	"github.com/yavosh/smtpmocker/web/content"
	"net/http"
)

func NewHttpHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", Index)
	mux.HandleFunc("/health", Health)
	return mux
}

func Index(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		Server404(w)
		return
	}

	WriteContentString(w, http.StatusOK, content.TextHTML, "<html><head/><body><h1>hello world</h1></body></html>")
}

func Health(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
