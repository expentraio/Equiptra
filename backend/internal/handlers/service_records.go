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

	"equiptra/internal/middleware"
	"equiptra/internal/models"
)

// assetHasOpenFault reports whether an asset currently has any
// open/in_progress service_records entry — the availability rule from the
// fault-reporting addendum, checked at allocation time only (forward-looking,
// never retroactive against an allocation that already exists). Matches the
// existing convention of inline Go helpers (computeShortage,
// findAllocationConflicts) rather than a SQL view, since nothing else in
// this schema is modeled as a view.
func assetHasOpenFault(ctx context.Context, db *pgxpool.Pool, assetID int64) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM service_records
			WHERE asset_id = $1 AND status IN ('open', 'in_progress')
		)`, assetID,
	).Scan(&exists)
	return exists, err
}

const serviceRecordSelectCols = `
	sr.id, sr.asset_id, sr.date_reported, sr.fault_description, sr.status, sr.monday_item_id, sr.source,
	sr.resolved_date, sr.resolution_notes, sr.reporter_user_id, sr.reporter_name, sr.reporter_email,
	sr.resolved_by, sr.created_at, sr.updated_at,
	a.asset_number, a.serial_number, a.product_id, p.name, ru.name, rb.name`

func scanServiceRecord(row pgx.Row) (models.ServiceRecord, error) {
	var s models.ServiceRecord
	err := row.Scan(&s.ID, &s.AssetID, &s.DateReported, &s.FaultDescription, &s.Status, &s.MondayItemID, &s.Source,
		&s.ResolvedDate, &s.ResolutionNotes, &s.ReporterUserID, &s.ReporterName, &s.ReporterEmail,
		&s.ResolvedBy, &s.CreatedAt, &s.UpdatedAt,
		&s.AssetNumber, &s.SerialNumber, &s.ProductID, &s.ProductName, &s.ReporterUserName, &s.ResolvedByName)
	return s, err
}

const serviceRecordJoins = `
	FROM service_records sr
	JOIN assets a ON a.id = sr.asset_id
	JOIN products p ON p.id = a.product_id
	LEFT JOIN users ru ON ru.id = sr.reporter_user_id
	LEFT JOIN users rb ON rb.id = sr.resolved_by`

func (a *API) ListServiceRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	assetID := q.Get("asset_id")
	rows, err := a.DB.Query(r.Context(), `
		SELECT `+serviceRecordSelectCols+serviceRecordJoins+`
		WHERE ($1 = '' OR sr.status::text = $1)
		  AND ($2 = '' OR sr.asset_id = $2::bigint)
		ORDER BY sr.date_reported DESC`, status, assetID)
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

func (a *API) GetServiceRecord(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	s, err := scanServiceRecord(a.DB.QueryRow(r.Context(), `
		SELECT `+serviceRecordSelectCols+serviceRecordJoins+`
		WHERE sr.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "service record not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

type serviceRecordWriteRequest struct {
	AssetID          int64                `json:"asset_id"`
	DateReported     time.Time            `json:"date_reported"`
	FaultDescription string               `json:"fault_description"`
	Status           models.ServiceStatus `json:"status"`
	MondayItemID     *string              `json:"monday_item_id"`
	ResolvedDate     *time.Time           `json:"resolved_date"`
	ResolutionNotes  *string              `json:"resolution_notes"`
}

// UpdateServiceRecord is how staff work a fault through its lifecycle from
// the Services/Repairs tab (open -> in_progress -> resolved). resolved_by is
// set here (not by the DB trigger, which only knows to stamp resolved_date —
// it has no notion of which user is making the request) whenever the status
// transitions to resolved.
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
	if req.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}

	var resolvedBy *int64
	if req.Status == models.ServiceStatusResolved {
		if claims, ok := middleware.UserFromContext(r.Context()); ok {
			resolvedBy = &claims.UserID
		}
	}

	// resolvedBy is already nil unless req.Status == resolved (computed
	// above), so COALESCE here is enough — no need for a SQL-side status
	// check, which previously caused Postgres to fail inferring $4's type
	// from two different contexts in the same query.
	tag, err := a.DB.Exec(r.Context(), `
		UPDATE service_records SET asset_id=$1, date_reported=$2, fault_description=$3, status=$4,
		       monday_item_id=$5, resolved_date=$6, resolution_notes=$7,
		       resolved_by = COALESCE($8::bigint, resolved_by),
		       updated_at=now()
		WHERE id=$9`,
		req.AssetID, req.DateReported, req.FaultDescription, req.Status,
		req.MondayItemID, req.ResolvedDate, req.ResolutionNotes, resolvedBy, id,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, "update failed: "+err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "service record not found")
		return
	}

	s, err := scanServiceRecord(a.DB.QueryRow(r.Context(), `
		SELECT `+serviceRecordSelectCols+serviceRecordJoins+`
		WHERE sr.id = $1`, id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fetch after update failed")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

type faultReportRequest struct {
	AssetID          int64   `json:"asset_id"`
	FaultDescription string  `json:"fault_description"`
	ReporterName     *string `json:"reporter_name"`
	ReporterEmail    *string `json:"reporter_email"`
}

// CreateFaultReport is the public fault-report form's write endpoint —
// mounted outside RequireAuth (see cmd/api/main.go) and rate-limited, since
// it's reachable by anyone, not just known freelancers. A logged-in staff
// member's session (if present) supplies reporter_user_id automatically;
// otherwise reporter_name/reporter_email are required. This replaces the
// old Monday.com-relay-facing CreateServiceRecord endpoint as the sole
// manual fault-creation path.
func (a *API) CreateFaultReport(w http.ResponseWriter, r *http.Request) {
	var req faultReportRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AssetID == 0 || req.FaultDescription == "" {
		writeError(w, http.StatusBadRequest, "asset_id and fault_description are required")
		return
	}

	var reporterUserID *int64
	if claims, ok := middleware.UserFromContext(r.Context()); ok {
		reporterUserID = &claims.UserID
	} else {
		if req.ReporterName == nil || *req.ReporterName == "" || req.ReporterEmail == nil || *req.ReporterEmail == "" {
			writeError(w, http.StatusBadRequest, "reporter_name and reporter_email are required when not logged in")
			return
		}
	}

	var id int64
	err := a.DB.QueryRow(r.Context(), `
		INSERT INTO service_records (asset_id, date_reported, fault_description, status, source, reporter_user_id, reporter_name, reporter_email)
		VALUES ($1, CURRENT_DATE, $2, 'open', 'field_report', $3, $4, $5)
		RETURNING id`,
		req.AssetID, req.FaultDescription, reporterUserID, req.ReporterName, req.ReporterEmail,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusBadRequest, "insert failed: "+err.Error())
		return
	}

	s, err := scanServiceRecord(a.DB.QueryRow(r.Context(), `
		SELECT `+serviceRecordSelectCols+serviceRecordJoins+`
		WHERE sr.id = $1`, id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fetch after insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

// PublicAssetSearchResult is a deliberately narrow view of an asset for the
// unauthenticated fault-report form's lookup field — no pricing, location,
// or notes, unlike the authenticated asset search.
type PublicAssetSearchResult struct {
	AssetID      int64   `json:"asset_id"`
	AssetNumber  *string `json:"asset_number,omitempty"`
	SerialNumber *string `json:"serial_number,omitempty"`
	ProductName  string  `json:"product_name"`
	Category     *string `json:"category,omitempty"`
}

// SearchPublicAssets backs the public fault-report form's asset lookup.
// Mounted outside RequireAuth and rate-limited alongside CreateFaultReport.
func (a *API) SearchPublicAssets(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	if len(search) < 2 {
		writeJSON(w, http.StatusOK, []PublicAssetSearchResult{})
		return
	}
	rows, err := a.DB.Query(r.Context(), `
		SELECT a.id, a.asset_number, a.serial_number, p.name, p.category
		FROM assets a JOIN products p ON p.id = a.product_id
		WHERE a.status = 'active'
		  AND (p.name ILIKE '%' || $1 || '%'
		       OR a.asset_number ILIKE '%' || $1 || '%'
		       OR a.serial_number ILIKE '%' || $1 || '%')
		ORDER BY p.name
		LIMIT 20`, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	results := []PublicAssetSearchResult{}
	for rows.Next() {
		var res PublicAssetSearchResult
		if err := rows.Scan(&res.AssetID, &res.AssetNumber, &res.SerialNumber, &res.ProductName, &res.Category); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, results)
}
