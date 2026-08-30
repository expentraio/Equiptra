package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"equiptra/internal/middleware"
	"equiptra/internal/models"
)

const allocationSelectCols = `
	ba.id, ba.booking_request_id, ba.asset_id, ba.status, ba.checked_out_at, ba.checked_out_by,
	ba.inspection_passed, ba.condition_out_notes, ba.checked_in_at, ba.checked_in_by,
	ba.condition_in_notes, ba.damage_flag, ba.damage_service_record_id, ba.created_at, ba.updated_at,
	a.asset_number, a.serial_number, a.is_bulk, p.name,
	uo.name, ui.name`

func scanAllocation(row pgx.Row) (models.BookingAllocation, error) {
	var ba models.BookingAllocation
	err := row.Scan(&ba.ID, &ba.BookingRequestID, &ba.AssetID, &ba.Status, &ba.CheckedOutAt, &ba.CheckedOutBy,
		&ba.InspectionPassed, &ba.ConditionOutNotes, &ba.CheckedInAt, &ba.CheckedInBy,
		&ba.ConditionInNotes, &ba.DamageFlag, &ba.DamageServiceRecordID, &ba.CreatedAt, &ba.UpdatedAt,
		&ba.AssetNumber, &ba.SerialNumber, &ba.IsBulk, &ba.ProductName,
		&ba.CheckedOutByName, &ba.CheckedInByName)
	return ba, err
}

func queryAllocations(ctx context.Context, db *pgxpool.Pool, whereClause string, args ...interface{}) ([]models.BookingAllocation, error) {
	rows, err := db.Query(ctx, `
		SELECT `+allocationSelectCols+`
		FROM booking_allocations ba
		JOIN assets a ON a.id = ba.asset_id
		JOIN products p ON p.id = a.product_id
		LEFT JOIN users uo ON uo.id = ba.checked_out_by
		LEFT JOIN users ui ON ui.id = ba.checked_in_by
		`+whereClause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allocations := []models.BookingAllocation{}
	for rows.Next() {
		ba, err := scanAllocation(rows)
		if err != nil {
			return nil, err
		}
		allocations = append(allocations, ba)
	}
	return allocations, rows.Err()
}

// AllocationConflict describes another allocation on the same (non-bulk)
// asset whose parent booking_request's date range overlaps this one — the
// brief's allocation-level conflict test, checked at the point a specific
// serial is committed. For bulk assets this isn't used; see
// bulkOverAllocationCount instead.
type AllocationConflict struct {
	AllocationID     int64     `json:"allocation_id"`
	BookingRequestID int64     `json:"booking_request_id"`
	ProjectName      string    `json:"project_name"`
	DateOut          time.Time `json:"date_out"`
	DateIn           time.Time `json:"date_in"`
	Status           string    `json:"status"`
}

func findAllocationConflicts(ctx context.Context, db *pgxpool.Pool, assetID int64, dateOut, dateIn time.Time, excludeAllocationID *int64) ([]AllocationConflict, error) {
	rows, err := db.Query(ctx, `
		SELECT ba.id, ba.booking_request_id, pr.name, br.date_out, br.date_in, ba.status
		FROM booking_allocations ba
		JOIN booking_requests br ON br.id = ba.booking_request_id
		JOIN projects pr ON pr.id = br.project_id
		WHERE ba.asset_id = $1
		  AND ba.status IN ('allocated', 'checked_out')
		  AND $2 < br.date_in AND $3 > br.date_out
		  AND ($4::bigint IS NULL OR ba.id != $4)`,
		assetID, dateOut, dateIn, excludeAllocationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conflicts := []AllocationConflict{}
	for rows.Next() {
		var c AllocationConflict
		if err := rows.Scan(&c.AllocationID, &c.BookingRequestID, &c.ProjectName, &c.DateOut, &c.DateIn, &c.Status); err != nil {
			return nil, err
		}
		conflicts = append(conflicts, c)
	}
	return conflicts, rows.Err()
}

// bulkOverAllocationCount counts other active allocations against the same
// bulk asset whose parent request's dates overlap — compared against the
// asset's held quantity to catch over-committing a bulk line, since many
// concurrent allocations sharing one bulk asset_id is normal, not a conflict.
func bulkOverAllocationCount(ctx context.Context, db *pgxpool.Pool, assetID int64, dateOut, dateIn time.Time, excludeAllocationID *int64) (int, error) {
	var count int
	err := db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM booking_allocations ba
		JOIN booking_requests br ON br.id = ba.booking_request_id
		WHERE ba.asset_id = $1
		  AND ba.status IN ('allocated', 'checked_out')
		  AND $2 < br.date_in AND $3 > br.date_out
		  AND ($4::bigint IS NULL OR ba.id != $4)`,
		assetID, dateOut, dateIn, excludeAllocationID,
	).Scan(&count)
	return count, err
}

func (a *API) ListAllocationsForRequest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	allocations, err := queryAllocations(r.Context(), a.DB, `WHERE ba.booking_request_id = $1 ORDER BY ba.created_at`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	type allocationWithConflicts struct {
		models.BookingAllocation
		Conflicts []AllocationConflict `json:"conflicts"`
	}
	result := make([]allocationWithConflicts, len(allocations))
	for i, ba := range allocations {
		var conflicts []AllocationConflict
		if ba.IsBulk == nil || !*ba.IsBulk {
			var reqRow struct {
				DateOut time.Time
				DateIn  time.Time
			}
			err := a.DB.QueryRow(r.Context(), `SELECT date_out, date_in FROM booking_requests WHERE id = $1`, ba.BookingRequestID).
				Scan(&reqRow.DateOut, &reqRow.DateIn)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "conflict check failed")
				return
			}
			conflicts, err = findAllocationConflicts(r.Context(), a.DB, ba.AssetID, reqRow.DateOut, reqRow.DateIn, &ba.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "conflict check failed")
				return
			}
		}
		result[i] = allocationWithConflicts{BookingAllocation: ba, Conflicts: conflicts}
	}
	writeJSON(w, http.StatusOK, result)
}

type allocateRequest struct {
	AssetID  int64 `json:"asset_id"`
	Override bool  `json:"override"`
}

// CreateAllocation is the pack-out step: pick a specific asset for a
// booking_request. Runs the conflict check (serialized assets) or
// over-allocation check (bulk assets) and, like the old booking conflict
// flow, flags rather than blocks — the caller must resubmit with
// override=true to proceed once shown the warning.
func (a *API) CreateAllocation(w http.ResponseWriter, r *http.Request) {
	requestID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req allocateRequest
	if err := readJSON(r, &req); err != nil || req.AssetID == 0 {
		writeError(w, http.StatusBadRequest, "asset_id is required")
		return
	}

	var br models.BookingRequest
	var projectStatus models.ProjectStatus
	err = a.DB.QueryRow(r.Context(), `
		SELECT br.id, br.product_id, br.date_out, br.date_in, pr.status
		FROM booking_requests br
		JOIN projects pr ON pr.id = br.project_id
		WHERE br.id = $1`, requestID).
		Scan(&br.ID, &br.ProductID, &br.DateOut, &br.DateIn, &projectStatus)
	if err != nil {
		writeError(w, http.StatusNotFound, "booking request not found")
		return
	}
	if projectStatus == models.ProjectStatusCancelled || projectStatus == models.ProjectStatusCompleted {
		writeError(w, http.StatusBadRequest, "cannot allocate against a "+string(projectStatus)+" project")
		return
	}
	if br.ProductID == nil {
		writeError(w, http.StatusBadRequest, "cannot allocate: this request is still a placeholder with no product attached")
		return
	}

	var asset models.Asset
	err = a.DB.QueryRow(r.Context(), `SELECT id, product_id, is_bulk, quantity, status FROM assets WHERE id = $1`, req.AssetID).
		Scan(&asset.ID, &asset.ProductID, &asset.IsBulk, &asset.Quantity, &asset.Status)
	if err != nil {
		writeError(w, http.StatusNotFound, "asset not found")
		return
	}
	if asset.ProductID != *br.ProductID {
		writeError(w, http.StatusBadRequest, "asset does not match this request's product")
		return
	}
	if asset.Status != models.AssetStatusActive {
		writeError(w, http.StatusBadRequest, "asset is not active ("+string(asset.Status)+")")
		return
	}

	var conflicts []AllocationConflict
	if asset.IsBulk {
		count, err := bulkOverAllocationCount(r.Context(), a.DB, asset.ID, br.DateOut, br.DateIn, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "availability check failed")
			return
		}
		if count+1 > asset.Quantity && !req.Override {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":             "over-allocation",
				"message":           "allocating this would exceed the bulk stock currently held for overlapping dates",
				"held_quantity":     asset.Quantity,
				"already_allocated": count,
			})
			return
		}
	} else {
		conflicts, err = findAllocationConflicts(r.Context(), a.DB, asset.ID, br.DateOut, br.DateIn, nil)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "conflict check failed")
			return
		}
		if len(conflicts) > 0 && !req.Override {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":     "allocation conflict",
				"conflicts": conflicts,
			})
			return
		}
	}

	var id int64
	err = a.DB.QueryRow(r.Context(), `
		INSERT INTO booking_allocations (booking_request_id, asset_id)
		VALUES ($1, $2) RETURNING id`, requestID, asset.ID,
	).Scan(&id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed: "+err.Error())
		return
	}

	if err := recomputeBookingRequestStatus(r.Context(), a.DB, requestID); err != nil {
		writeError(w, http.StatusInternalServerError, "status recompute failed")
		return
	}

	allocations, err := queryAllocations(r.Context(), a.DB, `WHERE ba.id = $1`, id)
	if err != nil || len(allocations) == 0 {
		writeError(w, http.StatusInternalServerError, "fetch after insert failed")
		return
	}
	resp := map[string]interface{}{"allocation": allocations[0]}
	if len(conflicts) > 0 {
		resp["conflicts"] = conflicts
	}
	writeJSON(w, http.StatusCreated, resp)
}

// DeleteAllocation un-picks a mistaken allocation — only while it hasn't
// been checked out yet.
func (a *API) DeleteAllocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var requestID int64
	var status models.BookingAllocationStatus
	err = a.DB.QueryRow(r.Context(), `SELECT booking_request_id, status FROM booking_allocations WHERE id = $1`, id).Scan(&requestID, &status)
	if err != nil {
		writeError(w, http.StatusNotFound, "allocation not found")
		return
	}
	if status != models.BookingAllocationStatusAllocated {
		writeError(w, http.StatusConflict, "cannot remove an allocation once it has been checked out")
		return
	}
	if _, err := a.DB.Exec(r.Context(), `DELETE FROM booking_allocations WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if err := recomputeBookingRequestStatus(r.Context(), a.DB, requestID); err != nil {
		writeError(w, http.StatusInternalServerError, "status recompute failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type checkoutRequest struct {
	InspectionPassed  bool    `json:"inspection_passed"`
	ConditionOutNotes *string `json:"condition_out_notes"`
}

// CheckoutAllocation hard-blocks unless inspection_passed is true, per the
// brief — the DB's chk_checkout_requires_inspection constraint backs this up.
func (a *API) CheckoutAllocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req checkoutRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.InspectionPassed {
		writeError(w, http.StatusBadRequest, "checkout is blocked until the pre-release inspection is marked passed")
		return
	}
	claims, _ := middleware.UserFromContext(r.Context())

	var requestID int64
	err = a.DB.QueryRow(r.Context(), `
		UPDATE booking_allocations
		SET status = 'checked_out', checked_out_at = now(), checked_out_by = $1,
		    inspection_passed = true, condition_out_notes = $2, updated_at = now()
		WHERE id = $3 AND status = 'allocated'
		RETURNING booking_request_id`,
		claims.UserID, req.ConditionOutNotes, id,
	).Scan(&requestID)
	if err != nil {
		writeError(w, http.StatusConflict, "checkout failed — allocation not found or already checked out")
		return
	}

	if err := recomputeBookingRequestStatus(r.Context(), a.DB, requestID); err != nil {
		writeError(w, http.StatusInternalServerError, "status recompute failed")
		return
	}

	allocations, err := queryAllocations(r.Context(), a.DB, `WHERE ba.id = $1`, id)
	if err != nil || len(allocations) == 0 {
		writeError(w, http.StatusInternalServerError, "fetch after checkout failed")
		return
	}
	writeJSON(w, http.StatusOK, allocations[0])
}

type checkinRequest struct {
	ConditionInNotes *string `json:"condition_in_notes"`
	DamageFlag       bool    `json:"damage_flag"`
	FaultDescription *string `json:"fault_description"`
}

// CheckinAllocation marks an allocation returned and, if damage_flag is set,
// auto-creates a service_records row (source=checkin_damage) linked back via
// damage_service_record_id — no separate manual reporting step.
func (a *API) CheckinAllocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req checkinRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	claims, _ := middleware.UserFromContext(r.Context())

	var assetID, requestID int64
	err = a.DB.QueryRow(r.Context(), `SELECT asset_id, booking_request_id FROM booking_allocations WHERE id = $1 AND status = 'checked_out'`, id).
		Scan(&assetID, &requestID)
	if err != nil {
		writeError(w, http.StatusConflict, "check-in failed — allocation not found or not currently checked out")
		return
	}

	var damageServiceRecordID *int64
	if req.DamageFlag {
		description := "Damage reported at check-in"
		if req.FaultDescription != nil && *req.FaultDescription != "" {
			description = *req.FaultDescription
		} else if req.ConditionInNotes != nil && *req.ConditionInNotes != "" {
			description = *req.ConditionInNotes
		}
		var recordID int64
		err = a.DB.QueryRow(r.Context(), `
			INSERT INTO service_records (asset_id, date_reported, fault_description, status, source)
			VALUES ($1, CURRENT_DATE, $2, 'open', 'checkin_damage')
			RETURNING id`, assetID, description,
		).Scan(&recordID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "damage service record creation failed")
			return
		}
		damageServiceRecordID = &recordID
	}

	_, err = a.DB.Exec(r.Context(), `
		UPDATE booking_allocations
		SET status = 'returned', checked_in_at = now(), checked_in_by = $1,
		    condition_in_notes = $2, damage_flag = $3, damage_service_record_id = $4, updated_at = now()
		WHERE id = $5`,
		claims.UserID, req.ConditionInNotes, req.DamageFlag, damageServiceRecordID, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "check-in failed: "+err.Error())
		return
	}

	if err := recomputeBookingRequestStatus(r.Context(), a.DB, requestID); err != nil {
		writeError(w, http.StatusInternalServerError, "status recompute failed")
		return
	}

	allocations, err := queryAllocations(r.Context(), a.DB, `WHERE ba.id = $1`, id)
	if err != nil || len(allocations) == 0 {
		writeError(w, http.StatusInternalServerError, "fetch after check-in failed")
		return
	}
	writeJSON(w, http.StatusOK, allocations[0])
}
