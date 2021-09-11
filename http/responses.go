package http

import (
	"encoding/json"
	"github.com/yavosh/smtpbox/http/content"
	"log"
	"net/http"
)

const (
	responseString404 = "<html><head/><body><h1>Not found</h1></body></html>"
	responseString500 = "<html><head/><body><h1>System error</h1></body></html>"
)

func StringResponse(w http.ResponseWriter, status int, contentType string, content string) {
	w.Header().Add("Content-Type", contentType)
	w.WriteHeader(status)
	_, err := w.Write([]byte(content))
	if err != nil {
		log.Printf("error writing response %v", err)
	}
}

func JSONResponse(w http.ResponseWriter, status int, value interface{}) {
	payload, err := json.Marshal(value)
	if err != nil {
		log.Printf("error writing response %v", err)
		http.Error(w, "system error", http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", content.ApplicationJSON)
	w.WriteHeader(status)
	_, err = w.Write(payload)
	if err != nil {
		log.Printf("error writing response %v", err)
	}
}

func Response404(w http.ResponseWriter) {
	StringResponse(w, http.StatusNotFound, content.TextHTML, responseString404)
}

func Server500(w http.ResponseWriter) {
	StringResponse(w, http.StatusInternalServerError, content.TextHTML, responseString500)
}
