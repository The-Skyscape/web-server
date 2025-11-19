package controllers

import (
	"encoding/json"
	"net/http"
)

// JSON sends a JSON response with the given status code and data
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// JSONError sends a JSON error response
func JSONError(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{
		"error": message,
	})
}

// JSONSuccess sends a JSON success response with data
func JSONSuccess(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, data)
}
