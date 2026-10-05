package orders

import (
	"github.com/Violetory/e-com/internal/json"
	"log"
	"net/http"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	var params createOrderParams

	if err := json.Read(r, &params); err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	createdOrder, err := h.service.PlaceOrder(r.Context(), params)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}

	json.Write(w, http.StatusCreated, createdOrder)
}
