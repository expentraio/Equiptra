package handlers

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"
)

// CarnetLine is one row of the generated carnet view: a product grouped
// across all its booked assets on a project, per the brief's §3 carnet spec
// (description/qty/weight/value/origin).
type CarnetLine struct {
	ProductID           int64   `json:"product_id"`
	Description         string  `json:"description"`
	Quantity            int     `json:"quantity"`
	TotalWeightKg       float64 `json:"total_weight_kg"`
	TotalValue          float64 `json:"total_value"`
	CountryOfOriginCode *string `json:"country_of_origin_code"`
	MissingOrigin       bool    `json:"missing_origin"`
}

type carnetView struct {
	ProjectID     int64        `json:"project_id"`
	ProjectName   string       `json:"project_name"`
	Lines         []CarnetLine `json:"lines"`
	TotalWeightKg float64      `json:"total_weight_kg"`
	TotalValue    float64      `json:"total_value"`
	MissingOrigin bool         `json:"missing_origin"`
}

// buildCarnetView pulls the carnet-relevant lines for a project: assets on
// allocated/checked-out booking_allocations for that project, grouped by
// product. Each allocation row is one physical unit (bulk or serialized —
// see the bulk-allocation design note in migrations/0001_init.sql), so a
// plain COUNT(*) gives the quantity directly.
func (a *API) buildCarnetView(r *http.Request, projectID int64) (*carnetView, error) {
	var projectName string
	err := a.DB.QueryRow(r.Context(), `SELECT name FROM projects WHERE id = $1`, projectID).Scan(&projectName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound("project not found")
	}
	if err != nil {
		return nil, err
	}

	rows, err := a.DB.Query(r.Context(), `
		SELECT p.id, p.name, p.description, p.country_of_origin_code,
		       COUNT(*)::int AS qty,
		       COUNT(*) * COALESCE(p.weight_kg, 0) AS total_weight,
		       SUM(COALESCE(a.replacement_value, 0)) AS total_value
		FROM booking_allocations ba
		JOIN booking_requests br ON br.id = ba.booking_request_id
		JOIN assets a ON a.id = ba.asset_id
		JOIN products p ON p.id = a.product_id
		WHERE br.project_id = $1 AND ba.status IN ('allocated', 'checked_out')
		GROUP BY p.id, p.name, p.description, p.country_of_origin_code
		ORDER BY p.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	view := &carnetView{ProjectID: projectID, ProjectName: projectName, Lines: []CarnetLine{}}
	for rows.Next() {
		var line CarnetLine
		var description *string
		if err := rows.Scan(&line.ProductID, &line.Description, &description,
			&line.CountryOfOriginCode, &line.Quantity, &line.TotalWeightKg, &line.TotalValue); err != nil {
			return nil, err
		}
		line.MissingOrigin = line.CountryOfOriginCode == nil || *line.CountryOfOriginCode == ""
		if line.MissingOrigin {
			view.MissingOrigin = true
		}
		view.TotalWeightKg += line.TotalWeightKg
		view.TotalValue += line.TotalValue
		view.Lines = append(view.Lines, line)
	}
	return view, rows.Err()
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) }

func (a *API) GetCarnetView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	view, err := a.buildCarnetView(r, id)
	if errors.As(err, new(errNotFound)) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) ExportCarnetCSV(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	view, err := a.buildCarnetView(r, id)
	if errors.As(err, new(errNotFound)) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="carnet-%d.csv"`, id))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Description", "Quantity", "Weight (kg)", "Value (GBP)", "Country of Origin"})
	for _, line := range view.Lines {
		origin := ""
		if line.CountryOfOriginCode != nil {
			origin = *line.CountryOfOriginCode
		}
		_ = cw.Write([]string{
			line.Description,
			strconv.Itoa(line.Quantity),
			strconv.FormatFloat(line.TotalWeightKg, 'f', 2, 64),
			strconv.FormatFloat(line.TotalValue, 'f', 2, 64),
			origin,
		})
	}
	cw.Flush()
}

// ExportCarnetPDF generates the customs-facing general list. Since a carnet
// document requires a country of origin on every line, this refuses to
// generate (409) if any line is missing one — see brief §4.
func (a *API) ExportCarnetPDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	view, err := a.buildCarnetView(r, id)
	if errors.As(err, new(errNotFound)) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if view.MissingOrigin {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "one or more items are missing a country of origin; fix in Products before generating a carnet PDF",
			"lines": view.Lines,
		})
		return
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	// fpdf's built-in fonts are Latin-1 only; translate UTF-8 input (accented
	// manufacturer/description text, the em dash below) through cp1252.
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 10, tr("General List — "+view.ProjectName), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, fmt.Sprintf("%d line items", len(view.Lines)), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	colWidths := []float64{80, 20, 25, 30, 25}
	headers := []string{"Description", "Qty", "Weight (kg)", "Value (GBP)", "Origin"}
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetFillColor(230, 244, 239)
	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 8, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 10)
	for _, line := range view.Lines {
		origin := ""
		if line.CountryOfOriginCode != nil {
			origin = *line.CountryOfOriginCode
		}
		pdf.CellFormat(colWidths[0], 8, tr(line.Description), "1", 0, "L", false, 0, "")
		pdf.CellFormat(colWidths[1], 8, strconv.Itoa(line.Quantity), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[2], 8, strconv.FormatFloat(line.TotalWeightKg, 'f', 2, 64), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[3], 8, strconv.FormatFloat(line.TotalValue, 'f', 2, 64), "1", 0, "R", false, 0, "")
		pdf.CellFormat(colWidths[4], 8, origin, "1", 0, "C", false, 0, "")
		pdf.Ln(-1)
	}

	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(colWidths[0]+colWidths[1], 8, "Total", "1", 0, "R", false, 0, "")
	pdf.CellFormat(colWidths[2], 8, strconv.FormatFloat(view.TotalWeightKg, 'f', 2, 64), "1", 0, "R", false, 0, "")
	pdf.CellFormat(colWidths[3], 8, strconv.FormatFloat(view.TotalValue, 'f', 2, 64), "1", 0, "R", false, 0, "")
	pdf.CellFormat(colWidths[4], 8, "", "1", 0, "C", false, 0, "")

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="carnet-%d.pdf"`, id))
	if err := pdf.Output(w); err != nil {
		writeError(w, http.StatusInternalServerError, "pdf generation failed")
	}
}
