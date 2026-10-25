package project

import "net/http"


func RegisterProjectRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /projects", h.Create)
}
