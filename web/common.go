package web

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/yavosh/smtpmocker/web/content"
)

func WriteContentString(w http.ResponseWriter, status int, contentType string, content string) {
	w.Header().Add("Content-Type", contentType)
	w.WriteHeader(status)
	_, err := w.Write([]byte(content))
	if err != nil {
		log.Printf("error writing response %v", err)
	}
}

func WriteJSON(w http.ResponseWriter, status int, value interface{}) {
	if payload, err := json.Marshal(value); err != nil {
		log.Printf("error writing response %v", err)
		Server500(w)
		return
	} else {
		w.Header().Add("Content-Type", content.ApplicationJSON)
		w.WriteHeader(status)
		_, err := w.Write(payload)
		if err != nil {
			log.Printf("error writing response %v", err)
		}
	}
}

func Server404(w http.ResponseWriter) {
	WriteContentString(w, http.StatusNotFound, content.TextHTML, "<html><head/><body><h1>Not hotdog</h1></body></html>")
}

func Server500(w http.ResponseWriter) {
	WriteContentString(w, http.StatusInternalServerError, content.TextHTML, "<html><head/><body><h1>System error</h1></body></html>")
}
