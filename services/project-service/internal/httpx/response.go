package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
)


func WriteJSON(w http.ResponseWriter, status int, data any) error {
	body, err := json.Marshal(data)

	if err != nil {
		return fmt.Errorf("marshal JSON response: %w", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("write JSON response: %w", err)
	}

	return nil
}
