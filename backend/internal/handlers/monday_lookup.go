package handlers

import (
	"errors"
	"net/http"
	"strings"

	"equiptra/internal/monday"
)

// MondayProjectLookup backs the "Fetch from Monday" button on new-project
// creation — same access level as CreateProject (not gated further), since
// this only reads and returns data for the frontend to pre-fill a form
// with; no database write happens here. See
// docs/equiptra-monday-lookup-addendum.md.
func (a *API) MondayProjectLookup(w http.ResponseWriter, r *http.Request) {
	orderNumber := strings.TrimSpace(r.URL.Query().Get("order_number"))
	if orderNumber == "" {
		writeError(w, http.StatusBadRequest, "order_number is required")
		return
	}
	if a.Monday == nil {
		writeError(w, http.StatusServiceUnavailable, "Monday lookup isn't configured (MONDAY_API_TOKEN not set) — enter project details manually")
		return
	}

	result, err := a.Monday.LookupByOrderNumber(r.Context(), orderNumber)
	switch {
	case errors.Is(err, monday.ErrNotFound):
		writeError(w, http.StatusNotFound, "no Monday item found for order number "+orderNumber+" — enter project details manually")
		return
	case errors.Is(err, monday.ErrAmbiguous):
		writeError(w, http.StatusConflict, "multiple Monday items match order number "+orderNumber+" — resolve in Monday or enter details manually")
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, "could not reach Monday — enter project details manually")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
