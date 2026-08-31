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

const productSelectCols = `id, legacy_id, name, category, manufacturer, weight_kg,
	       country_of_origin_code, is_accessory, barcode, image_url, description, active, created_at, updated_at`

func scanProduct(row pgx.Row) (models.Product, error) {
	var p models.Product
	err := row.Scan(&p.ID, &p.LegacyID, &p.Name, &p.Category, &p.Manufacturer,
		&p.WeightKg, &p.CountryOfOriginCode, &p.IsAccessory, &p.Barcode, &p.ImageURL, &p.Description, &p.Active,
		&p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// ProductListItem is a product row enriched with unit counts for the
// grouped Products browse page: TotalUnits is active-asset stock (bulk
// contributes its held quantity, serialized contributes 1 each);
// AvailableUnits subtracts units currently tied up in an active
// (allocated/checked_out) booking_allocation, regardless of that
// allocation's date range — this is a general browse page with no
// candidate date range of its own to test overlap against, so "available"
// here means "not currently spoken for at all", not "free for a specific
// window" (that finer-grained check still happens at allocation time).
type ProductListItem struct {
	models.Product
	TotalUnits     int `json:"total_units"`
	AvailableUnits int `json:"available_units"`
}

func (a *API) ListProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	category := q.Get("category")

	rows, err := a.DB.Query(r.Context(), `
		SELECT p.id, p.legacy_id, p.name, p.category, p.manufacturer, p.weight_kg,
		       p.country_of_origin_code, p.is_accessory, p.barcode, p.image_url, p.description, p.active,
		       p.created_at, p.updated_at,
		       COALESCE(units.total, 0) AS total_units,
		       GREATEST(COALESCE(units.total, 0) - COALESCE(alloc.active_count, 0) - COALESCE(faulted.count, 0), 0) AS available_units
		FROM products p
		LEFT JOIN (
			SELECT product_id, SUM(CASE WHEN is_bulk THEN quantity ELSE 1 END)::int AS total
			FROM assets WHERE status = 'active' GROUP BY product_id
		) units ON units.product_id = p.id
		LEFT JOIN (
			SELECT a.product_id, COUNT(*)::int AS active_count
			FROM booking_allocations ba
			JOIN assets a ON a.id = ba.asset_id
			WHERE ba.status IN ('allocated', 'checked_out')
			GROUP BY a.product_id
		) alloc ON alloc.product_id = p.id
		LEFT JOIN (
			-- Assets with an open/in_progress fault: a whole faulted bulk asset
			-- withholds its full held quantity, matching CreateAllocation's
			-- all-or-nothing block against that asset_id. Still counted in
			-- total_units (still physically active stock) — only subtracted
			-- from available_units, same treatment as an active allocation.
			SELECT a.product_id, SUM(CASE WHEN a.is_bulk THEN a.quantity ELSE 1 END)::int AS count
			FROM assets a
			WHERE a.status = 'active'
			  AND EXISTS (
			      SELECT 1 FROM service_records sr
			      WHERE sr.asset_id = a.id AND sr.status IN ('open', 'in_progress')
			  )
			GROUP BY a.product_id
		) faulted ON faulted.product_id = p.id
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR p.category = $2)
		ORDER BY p.name`,
		search, category,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	products := []ProductListItem{}
	for rows.Next() {
		var item ProductListItem
		p := &item.Product
		if err := rows.Scan(&p.ID, &p.LegacyID, &p.Name, &p.Category, &p.Manufacturer,
			&p.WeightKg, &p.CountryOfOriginCode, &p.IsAccessory, &p.Barcode, &p.ImageURL, &p.Description, &p.Active,
			&p.CreatedAt, &p.UpdatedAt, &item.TotalUnits, &item.AvailableUnits); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		products = append(products, item)
	}
	writeJSON(w, http.StatusOK, products)
}

func (a *API) GetProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := scanProduct(a.DB.QueryRow(r.Context(), `SELECT `+productSelectCols+` FROM products WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// CurrentAllocationInfo is one active (allocated/checked_out) allocation
// against an asset — a plain slice rather than a single nullable field
// because a bulk asset's single row can have several concurrent active
// allocations, unlike a serialized asset which normally has at most one.
type CurrentAllocationInfo struct {
	AllocationID int64     `json:"allocation_id"`
	ProjectName  string    `json:"project_name"`
	DateOut      time.Time `json:"date_out"`
	DateIn       time.Time `json:"date_in"`
	Status       string    `json:"status"`
}

// ProductAssetItem is one asset row for the product detail page's asset
// list, with whatever it's currently out on (if anything) attached.
type ProductAssetItem struct {
	models.Asset
	CurrentAllocations []CurrentAllocationInfo `json:"current_allocations"`
}

// ListProductAssets powers the product detail page: every asset for this
// product, each annotated with its current (active) allocations.
func (a *API) ListProductAssets(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	assetRows, err := a.DB.Query(r.Context(), `
		SELECT `+assetSelectCols+`
		FROM assets a JOIN products p ON p.id = a.product_id
		WHERE a.product_id = $1
		ORDER BY a.asset_number NULLS LAST`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	items := []ProductAssetItem{}
	assetIDs := []int64{}
	for assetRows.Next() {
		asset, err := scanAsset(assetRows)
		if err != nil {
			assetRows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		items = append(items, ProductAssetItem{Asset: asset, CurrentAllocations: []CurrentAllocationInfo{}})
		assetIDs = append(assetIDs, asset.ID)
	}
	assetRows.Close()

	if len(assetIDs) == 0 {
		writeJSON(w, http.StatusOK, items)
		return
	}

	allocRows, err := a.DB.Query(r.Context(), `
		SELECT ba.asset_id, ba.id, pr.name, br.date_out, br.date_in, ba.status
		FROM booking_allocations ba
		JOIN booking_requests br ON br.id = ba.booking_request_id
		JOIN projects pr ON pr.id = br.project_id
		WHERE ba.asset_id = ANY($1) AND ba.status IN ('allocated', 'checked_out')
		ORDER BY br.date_out`, assetIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer allocRows.Close()

	byAsset := map[int64][]CurrentAllocationInfo{}
	for allocRows.Next() {
		var assetID int64
		var info CurrentAllocationInfo
		if err := allocRows.Scan(&assetID, &info.AllocationID, &info.ProjectName, &info.DateOut, &info.DateIn, &info.Status); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		byAsset[assetID] = append(byAsset[assetID], info)
	}

	for i := range items {
		if allocs, ok := byAsset[items[i].ID]; ok {
			items[i].CurrentAllocations = allocs
		}
	}
	writeJSON(w, http.StatusOK, items)
}

type productWriteRequest struct {
	Name                string   `json:"name"`
	Category            *string  `json:"category"`
	Manufacturer        *string  `json:"manufacturer"`
	WeightKg            *float64 `json:"weight_kg"`
	CountryOfOriginCode *string  `json:"country_of_origin_code"`
	IsAccessory         bool     `json:"is_accessory"`
	Barcode             *string  `json:"barcode"`
	ImageURL            *string  `json:"image_url"`
	Description         *string  `json:"description"`
	Active              *bool    `json:"active"`
}

func (a *API) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req productWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	p, err := scanProduct(a.DB.QueryRow(r.Context(), `
		INSERT INTO products (name, category, manufacturer, weight_kg, country_of_origin_code, is_accessory, barcode, image_url, description, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+productSelectCols,
		req.Name, req.Category, req.Manufacturer, req.WeightKg, req.CountryOfOriginCode, req.IsAccessory, req.Barcode, req.ImageURL, req.Description, active,
	))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "insert failed")
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (a *API) UpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req productWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	p, err := scanProduct(a.DB.QueryRow(r.Context(), `
		UPDATE products SET name=$1, category=$2, manufacturer=$3, weight_kg=$4,
		       country_of_origin_code=$5, is_accessory=$6, barcode=$7, image_url=$8, description=$9, active=$10, updated_at=now()
		WHERE id=$11
		RETURNING `+productSelectCols,
		req.Name, req.Category, req.Manufacturer, req.WeightKg, req.CountryOfOriginCode, req.IsAccessory, req.Barcode, req.ImageURL, req.Description, active, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) DeleteProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusConflict, "cannot delete product with linked assets")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
