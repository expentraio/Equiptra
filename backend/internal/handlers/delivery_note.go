package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-pdf/fpdf"
	"github.com/jackc/pgx/v5"

	"equiptra/internal/assets"
	"equiptra/internal/models"
)

// DeliveryNoteLine is one row of the generated delivery note: a serialized
// asset gets its own row (asset number + serial shown); a bulk product's
// units are grouped into a single row with a summed quantity and no
// asset/serial, matching the real reference document (536-244...pdf).
type DeliveryNoteLine struct {
	Description  string  `json:"description"`
	Quantity     int     `json:"quantity"`
	AssetNumber  *string `json:"asset_number,omitempty"`
	SerialNumber *string `json:"serial_number,omitempty"`
	IsAccessory  bool    `json:"is_accessory"`
}

type deliveryNoteView struct {
	Project       models.Project     `json:"project"`
	Lines         []DeliveryNoteLine `json:"lines"`
	TotalWeightKg float64            `json:"total_weight_kg"`
	TotalValue    float64            `json:"total_value"`
}

func (a *API) buildDeliveryNoteView(r *http.Request, projectID int64) (*deliveryNoteView, error) {
	project, err := scanProject(a.DB.QueryRow(r.Context(), `SELECT `+projectSelectCols+` FROM projects WHERE id = $1`, projectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errNotFound("project not found")
	}
	if err != nil {
		return nil, err
	}

	rows, err := a.DB.Query(r.Context(), `
		SELECT p.name, p.is_accessory, a.id, a.asset_number, a.serial_number, a.is_bulk,
		       COALESCE(p.weight_kg, 0), COALESCE(a.replacement_value, 0)
		FROM booking_allocations ba
		JOIN booking_requests br ON br.id = ba.booking_request_id
		JOIN assets a ON a.id = ba.asset_id
		JOIN products p ON p.id = a.product_id
		WHERE br.project_id = $1 AND ba.status IN ('allocated', 'checked_out')
		ORDER BY br.created_at, ba.created_at`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	view := &deliveryNoteView{Project: project, Lines: []DeliveryNoteLine{}}
	bulkLineIndex := map[int64]int{} // asset_id -> index into view.Lines, for grouping bulk units

	for rows.Next() {
		var name string
		var isAccessory bool
		var assetID int64
		var assetNumber, serialNumber *string
		var isBulk bool
		var weightKg, replacementValue float64
		if err := rows.Scan(&name, &isAccessory, &assetID, &assetNumber, &serialNumber, &isBulk, &weightKg, &replacementValue); err != nil {
			return nil, err
		}
		view.TotalWeightKg += weightKg
		view.TotalValue += replacementValue

		if isBulk {
			if idx, ok := bulkLineIndex[assetID]; ok {
				view.Lines[idx].Quantity++
				continue
			}
			bulkLineIndex[assetID] = len(view.Lines)
			view.Lines = append(view.Lines, DeliveryNoteLine{Description: name, Quantity: 1, IsAccessory: isAccessory})
			continue
		}
		view.Lines = append(view.Lines, DeliveryNoteLine{
			Description: name, Quantity: 1, AssetNumber: assetNumber, SerialNumber: serialNumber, IsAccessory: isAccessory,
		})
	}
	return view, rows.Err()
}

func (a *API) GetDeliveryNoteView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	view, err := a.buildDeliveryNoteView(r, id)
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

func multilineOrDash(s *string) []string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return []string{"—"}
	}
	return strings.Split(strings.TrimSpace(*s), "\n")
}

func strOrDash(s *string) string {
	if s == nil || *s == "" {
		return "—"
	}
	return *s
}

// ExportDeliveryNotePDF renders the LDMtv-branded delivery note: header with
// client reference / order number / delivery address / rental dates, an
// itemized table (accessories indented and italicized), computed totals,
// and the verbatim hire T&Cs appended on continuation pages (brief §6/§3).
func (a *API) ExportDeliveryNotePDF(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	view, err := a.buildDeliveryNoteView(r, id)
	if errors.As(err, new(errNotFound)) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	const leftMargin = 15.0
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(leftMargin, 10, leftMargin)
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	colWidths := []float64{95, 15, 35, 35}
	headers := []string{"Item", "Qty", "Asset Number", "Serial Number"}

	drawItemTableHeader := func() {
		pdf.SetX(leftMargin)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(120, 120, 120)
		pdf.SetTextColor(255, 255, 255)
		for i, h := range headers {
			pdf.CellFormat(colWidths[i], 7, h, "", 0, "L", true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetTextColor(0, 0, 0)
	}

	inItemTable := false
	pdf.SetHeaderFunc(func() {
		if inItemTable {
			pdf.SetY(12)
			drawItemTableHeader()
		}
	})
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()

	// Logo, top right.
	if len(assets.LDMLogoPNG) > 0 {
		pdf.RegisterImageOptionsReader("ldm_logo", fpdf.ImageOptions{ImageType: "PNG"}, bytes.NewReader(assets.LDMLogoPNG))
		pdf.ImageOptions("ldm_logo", 155, 10, 40, 0, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	}

	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(180, 102, 26)
	pdf.CellFormat(100, 12, "Delivery Note", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetX(leftMargin)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(100, 7, tr(view.Project.Name), "", 1, "L", false, 0, "")
	pdf.SetX(leftMargin)
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(100, 6, time.Now().Format("02/01/2006"), "", 1, "L", false, 0, "")
	pdf.Ln(6)

	// Info block: one full-width labelled row per field, stacked — simpler
	// and more robust across page/content-height variation than trying to
	// mirror the reference PDF's two-column layout.
	labelW, valueW := 45.0, 135.0
	infoRow := func(label, value string) {
		pdf.SetX(leftMargin)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(245, 245, 245)
		pdf.CellFormat(labelW, 8, label, "1", 0, "L", true, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.CellFormat(valueW, 8, tr(value), "1", 1, "L", false, 0, "")
	}
	infoRow("Your Reference", strOrDash(view.Project.ClientReference))
	infoRow("Order Number", strOrDash(view.Project.OrderNumber))

	addrLines := multilineOrDash(view.Project.DeliveryAddress)
	pdf.SetX(leftMargin)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(245, 245, 245)
	addrHeight := 6.0 * float64(len(addrLines))
	pdf.CellFormat(labelW, addrHeight, "Delivery Address", "1", 0, "L", true, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.MultiCell(valueW, 6, tr(strings.Join(addrLines, "\n")), "1", "L", false)

	rentalRange := fmt.Sprintf("%s to %s", view.Project.StartDate.Format("02/01/2006"), view.Project.EndDate.Format("02/01/2006"))
	infoRow("Rental", rentalRange)
	infoRow("Total Weight", fmt.Sprintf("%.2f kgs", view.TotalWeightKg))
	infoRow("Insurance Value", fmt.Sprintf("GBP %s", formatMoney(view.TotalValue)))
	pdf.Ln(6)

	inItemTable = true
	drawItemTableHeader()
	for _, line := range view.Lines {
		pdf.SetX(leftMargin)
		name := line.Description
		style := ""
		indent := 0.0
		if line.IsAccessory {
			name += " (accessory)"
			style = "I"
			indent = 6
		}
		pdf.SetFont("Helvetica", style, 9)
		if indent > 0 {
			pdf.CellFormat(indent, 6.5, "", "", 0, "", false, 0, "")
		}
		pdf.CellFormat(colWidths[0]-indent, 6.5, tr(name), "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.CellFormat(colWidths[1], 6.5, strconv.Itoa(line.Quantity), "", 0, "L", false, 0, "")
		pdf.CellFormat(colWidths[2], 6.5, strOrEmpty(line.AssetNumber), "", 0, "L", false, 0, "")
		pdf.CellFormat(colWidths[3], 6.5, strOrEmpty(line.SerialNumber), "", 0, "L", false, 0, "")
		pdf.Ln(-1)
	}
	inItemTable = false

	// T&Cs appendix — verbatim boilerplate, new pages, no item-table header.
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 7.5)
	for _, para := range strings.Split(assets.TandCText, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if isTandCHeading(para) {
			pdf.Ln(2)
			pdf.SetFont("Helvetica", "B", 8.5)
			pdf.MultiCell(0, 4, tr(para), "", "L", false)
			pdf.SetFont("Helvetica", "", 7.5)
			continue
		}
		pdf.MultiCell(0, 3.6, tr(para), "", "L", false)
		pdf.Ln(0.5)
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="delivery-note-%d.pdf"`, id))
	if err := pdf.Output(w); err != nil {
		writeError(w, http.StatusInternalServerError, "pdf generation failed")
	}
}

func isTandCHeading(para string) bool {
	return len(para) < 60 && strings.ToUpper(para) == para && !strings.Contains(para, "\n")
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func formatMoney(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
