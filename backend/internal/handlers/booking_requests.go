package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"equiptra/internal/models"
)

const bookingRequestSelectCols = `
	br.id, br.project_id, br.product_id, br.placeholder_description, br.quantity_requested,
	br.date_out, br.date_in, br.status, br.shortage_flag, br.sub_hire_notes, br.created_at, br.updated_at,
	p.name, p.category, a.is_bulk, pr.name,
	COALESCE((SELECT COUNT(*) FROM booking_allocations ba WHERE ba.booking_request_id = br.id AND ba.status IN ('allocated','checked_out')), 0),
	COALESCE((SELECT COUNT(*) FROM booking_allocations ba WHERE ba.booking_request_id = br.id), 0)`

// isBulkAgg reports whether the product's assets are bulk-tracked (used in
// bookingRequestSelectCols via a correlated lateral — see queryBookingRequests).

func scanBookingRequest(row pgx.Row) (models.BookingRequest, error) {
	var br models.BookingRequest
	err := row.Scan(&br.ID, &br.ProjectID, &br.ProductID, &br.PlaceholderDescription, &br.QuantityRequested,
		&br.DateOut, &br.DateIn, &br.Status, &br.ShortageFlag, &br.SubHireNotes, &br.CreatedAt, &br.UpdatedAt,
		&br.ProductName, &br.Category, &br.IsBulk, &br.ProjectName, &br.AllocatedCount, &br.TotalAllocationCount)
	return br, err
}

func queryBookingRequests(ctx context.Context, db *pgxpool.Pool, whereClause string, args ...interface{}) ([]models.BookingRequest, error) {
	rows, err := db.Query(ctx, `
		SELECT `+bookingRequestSelectCols+`
		FROM booking_requests br
		JOIN projects pr ON pr.id = br.project_id
		LEFT JOIN products p ON p.id = br.product_id
		LEFT JOIN LATERAL (
			SELECT bool_or(is_bulk) AS is_bulk FROM assets WHERE assets.product_id = br.product_id
		) a ON true
		`+whereClause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	requests := []models.BookingRequest{}
	for rows.Next() {
		br, err := scanBookingRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, br)
	}
	return requests, rows.Err()
}

func (a *API) ListBookingRequests(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	var requests []models.BookingRequest
	var err error
	if projectID != "" {
		requests, err = queryBookingRequests(r.Context(), a.DB, `WHERE br.project_id = $1 ORDER BY br.date_out`, projectID)
	} else {
		requests, err = queryBookingRequests(r.Context(), a.DB, `ORDER BY br.date_out DESC`)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (a *API) GetBookingRequest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	requests, err := queryBookingRequests(r.Context(), a.DB, `WHERE br.id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if len(requests) == 0 {
		writeError(w, http.StatusNotFound, "booking request not found")
		return
	}
	writeJSON(w, http.StatusOK, requests[0])
}

// computeShortage implements the brief's shortage rule: true if the total
// quantity requested — this request plus every other live (reserved /
// partially_allocated / out) request for the same product whose date range
// overlaps — exceeds the product's total active stock. Only meaningful once
// a real product is attached; callers should skip it while product_id is
// null (placeholder requests never show a shortage).
func computeShortage(ctx context.Context, db *pgxpool.Pool, productID int64, quantityRequested int, dateOut, dateIn time.Time, excludeRequestID *int64) (bool, error) {
	var stock, committed int
	err := db.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(SUM(CASE WHEN is_bulk THEN quantity ELSE 1 END), 0)
			   FROM assets WHERE product_id = $1 AND status = 'active') AS stock,
			(SELECT COALESCE(SUM(quantity_requested), 0)
			   FROM booking_requests
			   WHERE product_id = $1
			     AND status IN ('reserved', 'partially_allocated', 'out')
			     AND ($4::bigint IS NULL OR id != $4)
			     AND $2 < date_in AND $3 > date_out) AS committed`,
		productID, dateOut, dateIn, excludeRequestID,
	).Scan(&stock, &committed)
	if err != nil {
		return false, err
	}
	return committed+quantityRequested > stock, nil
}

// recomputeBookingRequestStatus derives status from its allocations rather
// than trusting a manually-set value, except for 'cancelled' which is a
// deliberate user action left untouched here.
func recomputeBookingRequestStatus(ctx context.Context, db *pgxpool.Pool, requestID int64) error {
	var productID *int64
	var quantityRequested int
	var currentStatus models.BookingRequestStatus
	err := db.QueryRow(ctx, `SELECT product_id, quantity_requested, status FROM booking_requests WHERE id = $1`, requestID).
		Scan(&productID, &quantityRequested, &currentStatus)
	if err != nil {
		return err
	}
	if currentStatus == models.BookingRequestStatusCancelled {
		return nil
	}
	if productID == nil {
		_, err := db.Exec(ctx, `UPDATE booking_requests SET status = 'draft', updated_at = now() WHERE id = $1`, requestID)
		return err
	}

	var activeCount, checkedOutCount, returnedCount, totalCount int
	err = db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('allocated', 'checked_out')),
			COUNT(*) FILTER (WHERE status = 'checked_out'),
			COUNT(*) FILTER (WHERE status = 'returned'),
			COUNT(*)
		FROM booking_allocations WHERE booking_request_id = $1`, requestID,
	).Scan(&activeCount, &checkedOutCount, &returnedCount, &totalCount)
	if err != nil {
		return err
	}

	var newStatus models.BookingRequestStatus
	switch {
	case totalCount > 0 && activeCount == 0:
		newStatus = models.BookingRequestStatusReturned
	case activeCount >= quantityRequested && checkedOutCount >= activeCount && activeCount > 0:
		newStatus = models.BookingRequestStatusOut
	case activeCount > 0:
		newStatus = models.BookingRequestStatusPartiallyAllocated
	default:
		newStatus = models.BookingRequestStatusReserved
	}

	_, err = db.Exec(ctx, `UPDATE booking_requests SET status = $1, updated_at = now() WHERE id = $2`, newStatus, requestID)
	return err
}

type bookingRequestWriteRequest struct {
	ProjectID              int64     `json:"project_id"`
	ProductID              *int64    `json:"product_id"`
	PlaceholderDescription *string   `json:"placeholder_description"`
	QuantityRequested      int       `json:"quantity_requested"`
	DateOut                time.Time `json:"date_out"`
	DateIn                 time.Time `json:"date_in"`
	SubHireNotes           *string   `json:"sub_hire_notes"`
}

func (req bookingRequestWriteRequest) validate() error {
	if req.ProjectID == 0 {
		return errBadRequest("project_id is required")
	}
	if req.ProductID == nil && (req.PlaceholderDescription == nil || *req.PlaceholderDescription == "") {
		return errBadRequest("either product_id or placeholder_description is required")
	}
	if req.QuantityRequested <= 0 {
		return errBadRequest("quantity_requested must be greater than zero")
	}
	if req.DateOut.IsZero() || req.DateIn.IsZero() {
		return errBadRequest("date_out and date_in are required")
	}
	if req.DateIn.Before(req.DateOut) {
		return errBadRequest("date_in must be on or after date_out")
	}
	return nil
}

func (a *API) CreateBookingRequest(w http.ResponseWriter, r *http.Request) {
	var req bookingRequestWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var projectStatus models.ProjectStatus
	err := a.DB.QueryRow(r.Context(), `SELECT status FROM projects WHERE id = $1`, req.ProjectID).Scan(&projectStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project lookup failed")
		return
	}
	if projectStatus == models.ProjectStatusCancelled || projectStatus == models.ProjectStatusCompleted {
		writeError(w, http.StatusBadRequest, "cannot create a booking request for a "+string(projectStatus)+" project")
		return
	}

	shortage := false
	status := models.BookingRequestStatusDraft
	if req.ProductID != nil {
		status = models.BookingRequestStatusReserved
		var err error
		shortage, err = computeShortage(r.Context(), a.DB, *req.ProductID, req.QuantityRequested, req.DateOut, req.DateIn, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "shortage check failed")
			return
		}
	}

	var id int64
	err = a.DB.QueryRow(r.Context(), `
		INSERT INTO booking_requests (project_id, product_id, placeholder_description, quantity_requested, date_out, date_in, status, shortage_flag, sub_hire_notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		req.ProjectID, req.ProductID, req.PlaceholderDescription, req.QuantityRequested, req.DateOut, req.DateIn, status, shortage, req.SubHireNotes,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "insert failed: "+err.Error())
		return
	}

	requests, err := queryBookingRequests(r.Context(), a.DB, `WHERE br.id = $1`, id)
	if err != nil || len(requests) == 0 {
		writeError(w, http.StatusInternalServerError, "fetch after insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, requests[0])
}

func (a *API) UpdateBookingRequest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req bookingRequestWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	shortage := false
	if req.ProductID != nil {
		shortage, err = computeShortage(r.Context(), a.DB, *req.ProductID, req.QuantityRequested, req.DateOut, req.DateIn, &id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "shortage check failed")
			return
		}
	}

	tag, err := a.DB.Exec(r.Context(), `
		UPDATE booking_requests SET product_id=$1, placeholder_description=$2, quantity_requested=$3,
		       date_out=$4, date_in=$5, shortage_flag=$6, sub_hire_notes=$7, updated_at=now()
		WHERE id=$8`,
		req.ProductID, req.PlaceholderDescription, req.QuantityRequested, req.DateOut, req.DateIn, shortage, req.SubHireNotes, id,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update failed: "+err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "booking request not found")
		return
	}

	// Extending/editing dates or product can change status (e.g. product just
	// got attached) — recompute from current allocation state, per brief's
	// "extending a rental" note.
	if err := recomputeBookingRequestStatus(r.Context(), a.DB, id); err != nil {
		writeError(w, http.StatusInternalServerError, "status recompute failed")
		return
	}

	requests, err := queryBookingRequests(r.Context(), a.DB, `WHERE br.id = $1`, id)
	if err != nil || len(requests) == 0 {
		writeError(w, http.StatusInternalServerError, "fetch after update failed")
		return
	}
	writeJSON(w, http.StatusOK, requests[0])
}

func (a *API) CancelBookingRequest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE booking_requests SET status = 'cancelled', updated_at = now() WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cancel failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "booking request not found")
		return
	}
	requests, err := queryBookingRequests(r.Context(), a.DB, `WHERE br.id = $1`, id)
	if err != nil || len(requests) == 0 {
		writeError(w, http.StatusInternalServerError, "fetch after cancel failed")
		return
	}
	writeJSON(w, http.StatusOK, requests[0])
}

func (a *API) DeleteBookingRequest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `DELETE FROM booking_requests WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusConflict, "cannot delete a booking request with allocations — cancel it instead")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "booking request not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
