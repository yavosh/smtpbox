package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type apiResponse struct {
	Result any `json:"result,omitempty"`
}

type apiError struct {
	Error string `json:"error"`
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		slog.Error("error marshalling response", "error", err)
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err = w.Write(payload); err != nil {
		slog.Error("error writing response", "error", err)
	}
}

func jsonOK(w http.ResponseWriter, result any) {
	jsonResponse(w, http.StatusOK, apiResponse{Result: result})
}

func jsonNotFound(w http.ResponseWriter) {
	jsonResponse(w, http.StatusNotFound, apiError{Error: "not found"})
}

func jsonServerError(w http.ResponseWriter) {
	jsonResponse(w, http.StatusInternalServerError, apiError{Error: "internal server error"})
}
