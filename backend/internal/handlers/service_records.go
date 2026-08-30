package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"equiptra/internal/models"
)

const serviceRecordSelectCols = `id, asset_id, date_reported, fault_description, status, monday_item_id, source, resolved_date, resolution_notes, created_at, updated_at`

func scanServiceRecord(row pgx.Row) (models.ServiceRecord, error) {
	var s models.ServiceRecord
	err := row.Scan(&s.ID, &s.AssetID, &s.DateReported, &s.FaultDescription, &s.Status,
		&s.MondayItemID, &s.Source, &s.ResolvedDate, &s.ResolutionNotes, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (a *API) ListServiceRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	assetID := q.Get("asset_id")
	rows, err := a.DB.Query(r.Context(), `
		SELECT `+serviceRecordSelectCols+` FROM service_records
		WHERE ($1 = '' OR status::text = $1)
		  AND ($2 = '' OR asset_id = $2::bigint)
		ORDER BY date_reported DESC`, status, assetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	records := []models.ServiceRecord{}
	for rows.Next() {
		s, err := scanServiceRecord(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		records = append(records, s)
	}
	writeJSON(w, http.StatusOK, records)
}

type serviceRecordWriteRequest struct {
	AssetID          int64                `json:"asset_id"`
	DateReported     time.Time            `json:"date_reported"`
	FaultDescription string               `json:"fault_description"`
	Status           models.ServiceStatus `json:"status"`
	MondayItemID     *string              `json:"monday_item_id"`
	Source           models.ServiceSource `json:"source"`
	ResolvedDate     *time.Time           `json:"resolved_date"`
	ResolutionNotes  *string              `json:"resolution_notes"`
}

// CreateServiceRecord is the write step called by the Monday.com
// fault-reporting Lambda after it creates the Monday item (see brief §3/§4),
// and also covers mid-rental fault reports filed the same way while an
// asset is checked out. The check-in damage path creates its row directly
// in CheckinAllocation instead of via this HTTP endpoint.
func (a *API) CreateServiceRecord(w http.ResponseWriter, r *http.Request) {
	var req serviceRecordWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AssetID == 0 || req.FaultDescription == "" || req.DateReported.IsZero() {
		writeError(w, http.StatusBadRequest, "asset_id, date_reported and fault_description are required")
		return
	}
	status := req.Status
	if status == "" {
		status = models.ServiceStatusOpen
	}
	source := req.Source
	if source == "" {
		source = models.ServiceSourceMondayReport
	}

	s, err := scanServiceRecord(a.DB.QueryRow(r.Context(), `
		INSERT INTO service_records (asset_id, date_reported, fault_description, status, monday_item_id, source, resolved_date, resolution_notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+serviceRecordSelectCols,
		req.AssetID, req.DateReported, req.FaultDescription, status, req.MondayItemID, source, req.ResolvedDate, req.ResolutionNotes,
	))
	if err != nil {
		writeError(w, http.StatusBadRequest, "insert failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (a *API) UpdateServiceRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req serviceRecordWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := req.Status
	if status == "" {
		status = models.ServiceStatusOpen
	}

	s, err := scanServiceRecord(a.DB.QueryRow(r.Context(), `
		UPDATE service_records SET asset_id=$1, date_reported=$2, fault_description=$3, status=$4,
		       monday_item_id=$5, resolved_date=$6, resolution_notes=$7, updated_at=now()
		WHERE id=$8
		RETURNING `+serviceRecordSelectCols,
		req.AssetID, req.DateReported, req.FaultDescription, status, req.MondayItemID, req.ResolvedDate, req.ResolutionNotes, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "service record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "update failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}
