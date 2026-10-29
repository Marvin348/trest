package application

import "net/http"

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /projects/{projectId}/applications", h.Create)
}
