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

const projectSelectCols = `id, name, client, start_date, end_date, status, carnet_required,
	       client_reference, order_number, delivery_address, notes, created_at, updated_at`

func scanProject(row pgx.Row) (models.Project, error) {
	var p models.Project
	err := row.Scan(&p.ID, &p.Name, &p.Client, &p.StartDate, &p.EndDate, &p.Status,
		&p.CarnetRequired, &p.ClientReference, &p.OrderNumber, &p.DeliveryAddress, &p.Notes,
		&p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (a *API) ListProjects(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	rows, err := a.DB.Query(r.Context(), `
		SELECT `+projectSelectCols+` FROM projects
		WHERE ($1 = '' OR status::text = $1)
		ORDER BY start_date DESC`, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	projects := []models.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		projects = append(projects, p)
	}
	writeJSON(w, http.StatusOK, projects)
}

func (a *API) GetProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := scanProject(a.DB.QueryRow(r.Context(), `SELECT `+projectSelectCols+` FROM projects WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type projectWriteRequest struct {
	Name            string               `json:"name"`
	Client          *string              `json:"client"`
	StartDate       time.Time            `json:"start_date"`
	EndDate         time.Time            `json:"end_date"`
	Status          models.ProjectStatus `json:"status"`
	CarnetRequired  bool                 `json:"carnet_required"`
	ClientReference *string              `json:"client_reference"`
	OrderNumber     *string              `json:"order_number"`
	DeliveryAddress *string              `json:"delivery_address"`
	Notes           *string              `json:"notes"`
}

func (req projectWriteRequest) validate() error {
	if req.Name == "" {
		return errBadRequest("name is required")
	}
	if req.StartDate.IsZero() || req.EndDate.IsZero() {
		return errBadRequest("start_date and end_date are required")
	}
	if req.EndDate.Before(req.StartDate) {
		return errBadRequest("end_date must be on or after start_date")
	}
	return nil
}

func (req projectWriteRequest) statusOrDefault() models.ProjectStatus {
	if req.Status == "" {
		return models.ProjectStatusTentative
	}
	return req.Status
}

func (a *API) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req projectWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p, err := scanProject(a.DB.QueryRow(r.Context(), `
		INSERT INTO projects (name, client, start_date, end_date, status, carnet_required, client_reference, order_number, delivery_address, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+projectSelectCols,
		req.Name, req.Client, req.StartDate, req.EndDate, req.statusOrDefault(), req.CarnetRequired,
		req.ClientReference, req.OrderNumber, req.DeliveryAddress, req.Notes,
	))
	if err != nil {
		writeError(w, http.StatusBadRequest, "insert failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (a *API) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req projectWriteRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	p, err := scanProject(a.DB.QueryRow(r.Context(), `
		UPDATE projects SET name=$1, client=$2, start_date=$3, end_date=$4, status=$5,
		       carnet_required=$6, client_reference=$7, order_number=$8, delivery_address=$9, notes=$10, updated_at=now()
		WHERE id=$11
		RETURNING `+projectSelectCols,
		req.Name, req.Client, req.StartDate, req.EndDate, req.statusOrDefault(), req.CarnetRequired,
		req.ClientReference, req.OrderNumber, req.DeliveryAddress, req.Notes, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "update failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusConflict, "cannot delete project with linked booking requests")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
