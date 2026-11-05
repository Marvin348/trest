package deployment

import "net/http"

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("POST /applications/{applicationId}/deploy", h.Deploy)
}
