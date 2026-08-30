package handlers

import (
	"errors"
	"net/http"
	"strconv"

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

func (a *API) ListProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	category := q.Get("category")

	rows, err := a.DB.Query(r.Context(), `
		SELECT `+productSelectCols+`
		FROM products
		WHERE ($1 = '' OR name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR category = $2)
		ORDER BY name`,
		search, category,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	products := []models.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		products = append(products, p)
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
