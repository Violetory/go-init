package template

import (
	"log"
	"net/http"

	"example.com/go-init-template/internal/json"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Template(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Template(r.Context())
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.Write(w, http.StatusOK, result)
}
