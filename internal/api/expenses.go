package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/domain"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// ExpenseHandler handles expense-related API endpoints.
type ExpenseHandler struct {
	Pool           *pgxpool.Pool
	ClassifyEngine *intelligence.ExpenseClassifier
	Cfg            *config.Config
}

// NewExpenseHandler creates a new expense handler.
func NewExpenseHandler(pool *pgxpool.Pool, engine *intelligence.ExpenseClassifier, cfg *config.Config) *ExpenseHandler {
	return &ExpenseHandler{Pool: pool, ClassifyEngine: engine, Cfg: cfg}
}

// RegisterRoutes registers expense API routes on the given Chi router.
func (h *ExpenseHandler) RegisterRoutes(r chi.Router) {
	r.Route("/api/expenses", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/export", h.Export)
		r.Get("/{id}", h.Get)
		r.Patch("/{id}", h.Correct)
		r.Post("/{id}/classify", h.ClassifyEndpoint)
		r.Post("/suggestions/{id}/accept", h.AcceptSuggestion)
		r.Post("/suggestions/{id}/dismiss", h.DismissSuggestion)
	})
}

var amountPattern = regexp.MustCompile(`^\d{1,10}\.\d{2}$`)
var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var filenameClean = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

const (
	maxVendorLen        = 200
	maxNotesLen         = 2000
	maxPaymentMethodLen = 100
	maxExpenseBodySize  = 64 << 10 // 64 KB — more than enough for correction/classify JSON
)

// sanitizeCSVCell prevents CSV injection (formula injection) by prefixing
// dangerous leading characters with a single quote. OWASP recommendation.
func sanitizeCSVCell(s string) string {
	if len(s) == 0 {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r', '\n':
		return "'" + s
	}
	return s
}

// List handles GET /api/expenses with query filters.
func (h *ExpenseHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Parse and validate filters
	from := q.Get("from")
	to := q.Get("to")
	var fromDate, toDate time.Time
	if from != "" {
		var err error
		fromDate, err = time.Parse("2006-01-02", from)
		if err != nil {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE_FORMAT", "from must be YYYY-MM-DD")
			return
		}
	}
	if to != "" {
		var err error
		toDate, err = time.Parse("2006-01-02", to)
		if err != nil {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE_FORMAT", "to must be YYYY-MM-DD")
			return
		}
	}
	if from != "" && to != "" && fromDate.After(toDate) {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE_RANGE", "from must be before to")
		return
	}

	// Build query
	conditions := []string{"metadata ? 'expense'"}
	args := []any{}
	argIdx := 1

	if from != "" {
		conditions = append(conditions, fmt.Sprintf("(metadata->'expense'->>'date')::date >= $%d", argIdx))
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		conditions = append(conditions, fmt.Sprintf("(metadata->'expense'->>'date')::date <= $%d", argIdx))
		args = append(args, to)
		argIdx++
	}
	if classification := q.Get("classification"); classification != "" {
		conditions = append(conditions, fmt.Sprintf("metadata->'expense'->>'classification' = $%d", argIdx))
		args = append(args, classification)
		argIdx++
	}
	if category := q.Get("category"); category != "" {
		conditions = append(conditions, fmt.Sprintf("metadata->'expense'->>'category' = $%d", argIdx))
		args = append(args, category)
		argIdx++
	}
	if vendor := q.Get("vendor"); vendor != "" {
		conditions = append(conditions, fmt.Sprintf("LOWER(metadata->'expense'->>'vendor') LIKE '%%' || LOWER($%d) || '%%'", argIdx))
		args = append(args, vendor)
		argIdx++
	}
	if currency := q.Get("currency"); currency != "" {
		conditions = append(conditions, fmt.Sprintf("metadata->'expense'->>'currency' = $%d", argIdx))
		args = append(args, currency)
		argIdx++
	}
	if q.Get("needs_review") == "true" {
		conditions = append(conditions, "(metadata->'expense'->>'extraction_status' != 'complete' OR metadata->'expense'->>'amount_missing' = 'true')")
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count + summary query
	summaryQuery := fmt.Sprintf(`
		SELECT
			COALESCE(metadata->'expense'->>'currency', 'USD') AS currency,
			COUNT(*) AS count,
			COALESCE(SUM(CAST(NULLIF(metadata->'expense'->>'amount', '') AS NUMERIC)), 0)::text AS total
		FROM artifacts
		WHERE %s
		GROUP BY metadata->'expense'->>'currency'
	`, whereClause)

	summaryRows, err := h.Pool.Query(r.Context(), summaryQuery, args...)
	if err != nil {
		slog.Error("expense summary query failed", "error", err)
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query expenses")
		return
	}
	defer summaryRows.Close()

	type currencySummary struct {
		Currency string `json:"currency"`
		Count    int    `json:"count"`
		Total    string `json:"total"`
	}
	var summaries []currencySummary
	totalCount := 0
	for summaryRows.Next() {
		var cs currencySummary
		if err := summaryRows.Scan(&cs.Currency, &cs.Count, &cs.Total); err != nil {
			continue
		}
		summaries = append(summaries, cs)
		totalCount += cs.Count
	}

	// Main data query with pagination
	limit := 50
	dataQuery := fmt.Sprintf(`
		SELECT id, title, metadata->'expense' AS expense, source_id
		FROM artifacts
		WHERE %s
		ORDER BY (metadata->'expense'->>'date')::date DESC NULLS LAST, id DESC
		LIMIT %d
	`, whereClause, limit)

	rows, err := h.Pool.Query(r.Context(), dataQuery, args...)
	if err != nil {
		slog.Error("expense list query failed", "error", err)
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query expenses")
		return
	}
	defer rows.Close()

	type expenseItem struct {
		ID      string          `json:"id"`
		Title   string          `json:"title"`
		Expense json.RawMessage `json:"expense"`
		Source  string          `json:"source"`
	}
	var expenses []expenseItem
	for rows.Next() {
		var item expenseItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Expense, &item.Source); err != nil {
			continue
		}
		expenses = append(expenses, item)
	}
	if expenses == nil {
		expenses = []expenseItem{}
	}
	if summaries == nil {
		summaries = []currencySummary{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"expenses": expenses,
		},
		"meta": map[string]any{
			"count": totalCount,
			"summary": map[string]any{
				"total_by_currency": summaries,
			},
		},
	})
}

// Get handles GET /api/expenses/{id}.
func (h *ExpenseHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var title string
	var expense json.RawMessage
	var source string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT title, metadata->'expense', source_id
		FROM artifacts
		WHERE id = $1 AND metadata ? 'expense'
	`, id).Scan(&title, &expense, &source)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeExpenseError(w, http.StatusNotFound, "EXPENSE_NOT_FOUND", "Expense not found")
			return
		}
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query expense")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"id":      id,
			"title":   title,
			"expense": json.RawMessage(expense),
			"source":  source,
		},
	})
}

// Correct handles PATCH /api/expenses/{id}.
func (h *ExpenseHandler) Correct(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	r.Body = http.MaxBytesReader(w, r.Body, maxExpenseBodySize)
	var req domain.ExpenseCorrectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
		return
	}

	// Validate fields
	if req.Amount != nil && !amountPattern.MatchString(*req.Amount) {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_AMOUNT", "Amount must be digits with exactly 2 decimal places")
		return
	}
	if req.Currency != nil && !currencyPattern.MatchString(*req.Currency) {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_CURRENCY", "Currency must be a 3-letter ISO 4217 code")
		return
	}
	if req.Date != nil {
		if !datePattern.MatchString(*req.Date) {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE", "Date must be YYYY-MM-DD")
			return
		}
		if _, err := time.Parse("2006-01-02", *req.Date); err != nil {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE", "Date must be a valid YYYY-MM-DD date")
			return
		}
	}
	if req.Classification != nil {
		switch *req.Classification {
		case "business", "personal", "uncategorized":
		default:
			writeExpenseError(w, http.StatusBadRequest, "INVALID_CLASSIFICATION", "Classification must be business, personal, or uncategorized")
			return
		}
	}
	if req.Category != nil && h.Cfg != nil && len(h.Cfg.ExpensesCategories) > 0 {
		validCategory := false
		for _, c := range h.Cfg.ExpensesCategories {
			if c.Slug == *req.Category {
				validCategory = true
				break
			}
		}
		if !validCategory {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_CATEGORY", "Category must be a configured category slug")
			return
		}
	}
	if req.Vendor != nil && len(*req.Vendor) > maxVendorLen {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_VENDOR", fmt.Sprintf("Vendor must be at most %d characters", maxVendorLen))
		return
	}
	if req.Notes != nil && len(*req.Notes) > maxNotesLen {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_NOTES", fmt.Sprintf("Notes must be at most %d characters", maxNotesLen))
		return
	}
	if req.PaymentMethod != nil && len(*req.PaymentMethod) > maxPaymentMethodLen {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_PAYMENT_METHOD", fmt.Sprintf("Payment method must be at most %d characters", maxPaymentMethodLen))
		return
	}

	// Fetch existing expense metadata
	var metadataRaw json.RawMessage
	var vendorRaw string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT metadata->'expense', COALESCE(metadata->'expense'->>'vendor_raw', '')
		FROM artifacts
		WHERE id = $1
	`, id).Scan(&metadataRaw, &vendorRaw)

	if err != nil {
		if err == pgx.ErrNoRows {
			writeExpenseError(w, http.StatusNotFound, "EXPENSE_NOT_FOUND", "Artifact not found")
			return
		}
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query artifact")
		return
	}
	if metadataRaw == nil {
		writeExpenseError(w, http.StatusUnprocessableEntity, "NOT_AN_EXPENSE", "Artifact is not an expense")
		return
	}

	var expense domain.ExpenseMetadata
	if err := json.Unmarshal(metadataRaw, &expense); err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "PARSE_FAILED", "Failed to parse expense metadata")
		return
	}

	// Apply corrections
	var correctedFields []string
	if req.Vendor != nil {
		expense.Vendor = *req.Vendor
		correctedFields = append(correctedFields, "vendor")
		// Create vendor alias from correction
		if vendorRaw != "" && h.ClassifyEngine != nil {
			_ = h.ClassifyEngine.CreateVendorAlias(r.Context(), vendorRaw, *req.Vendor)
		}
	}
	if req.Date != nil {
		expense.Date = req.Date
		correctedFields = append(correctedFields, "date")
	}
	if req.Amount != nil {
		expense.Amount = req.Amount
		expense.AmountMissing = false
		correctedFields = append(correctedFields, "amount")
	}
	if req.Currency != nil {
		expense.Currency = *req.Currency
		correctedFields = append(correctedFields, "currency")
	}
	if req.Category != nil {
		expense.Category = *req.Category
		correctedFields = append(correctedFields, "category")
	}
	if req.Classification != nil {
		expense.Classification = *req.Classification
		correctedFields = append(correctedFields, "classification")
	}
	if req.Notes != nil {
		expense.Notes = req.Notes
		correctedFields = append(correctedFields, "notes")
	}
	if req.PaymentMethod != nil {
		expense.PaymentMethod = req.PaymentMethod
		correctedFields = append(correctedFields, "payment_method")
	}

	expense.UserCorrected = true
	// Merge corrected fields (don't duplicate)
	for _, f := range correctedFields {
		if !containsStr(expense.CorrectedFields, f) {
			expense.CorrectedFields = append(expense.CorrectedFields, f)
		}
	}

	// Write back
	expenseJSON, err := json.Marshal(expense)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "MARSHAL_FAILED", "Failed to marshal expense")
		return
	}

	_, err = h.Pool.Exec(r.Context(), `
		UPDATE artifacts SET metadata = jsonb_set(metadata, '{expense}', $1::jsonb)
		WHERE id = $2
	`, string(expenseJSON), id)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update expense")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"id":               id,
			"corrected_fields": correctedFields,
			"expense":          expense,
		},
	})
}

// ClassifyEndpoint handles POST /api/expenses/{id}/classify.
func (h *ExpenseHandler) ClassifyEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	r.Body = http.MaxBytesReader(w, r.Body, maxExpenseBodySize)
	var req domain.ClassifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body")
		return
	}

	switch req.Classification {
	case "business", "personal", "uncategorized":
	default:
		writeExpenseError(w, http.StatusBadRequest, "INVALID_CLASSIFICATION", "Must be business, personal, or uncategorized")
		return
	}

	// Get previous classification
	var prevClass string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(metadata->'expense'->>'classification', 'uncategorized')
		FROM artifacts
		WHERE id = $1 AND metadata ? 'expense'
	`, id).Scan(&prevClass)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeExpenseError(w, http.StatusNotFound, "EXPENSE_NOT_FOUND", "Expense not found")
			return
		}
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query expense")
		return
	}

	// Update classification and mark as user-corrected (deduplicate corrected_fields)
	_, err = h.Pool.Exec(r.Context(), `
		UPDATE artifacts SET metadata = jsonb_set(
			jsonb_set(
				jsonb_set(metadata, '{expense,classification}', to_jsonb($1::text)),
				'{expense,user_corrected}', 'true'::jsonb
			),
			'{expense,corrected_fields}',
			CASE
				WHEN COALESCE(metadata->'expense'->'corrected_fields', '[]'::jsonb) @> '["classification"]'::jsonb
				THEN COALESCE(metadata->'expense'->'corrected_fields', '[]'::jsonb)
				ELSE COALESCE(metadata->'expense'->'corrected_fields', '[]'::jsonb) || '["classification"]'::jsonb
			END
		)
		WHERE id = $2 AND metadata ? 'expense'
	`, req.Classification, id)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update classification")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"id":                      id,
			"classification":          req.Classification,
			"previous_classification": prevClass,
		},
	})
}

// AcceptSuggestion handles POST /api/expenses/suggestions/{id}/accept.
func (h *ExpenseHandler) AcceptSuggestion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if h.Pool == nil {
		writeExpenseError(w, http.StatusInternalServerError, "TX_FAILED", "Database pool not available")
		return
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "TX_FAILED", "Failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var artifactID, suggestedClass, vendor string
	err = tx.QueryRow(ctx, `
		SELECT artifact_id, suggested_class, vendor
		FROM expense_suggestions
		WHERE id = $1 AND status = 'pending'
		FOR UPDATE
	`, id).Scan(&artifactID, &suggestedClass, &vendor)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeExpenseError(w, http.StatusNotFound, "SUGGESTION_NOT_FOUND", "Pending suggestion not found")
			return
		}
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query suggestion")
		return
	}

	// Update the artifact's classification
	_, err = tx.Exec(ctx, `
		UPDATE artifacts SET metadata = jsonb_set(metadata, '{expense,classification}', to_jsonb($1::text))
		WHERE id = $2 AND metadata ? 'expense'
	`, suggestedClass, artifactID)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update artifact")
		return
	}

	// Mark suggestion as accepted
	_, err = tx.Exec(ctx, `
		UPDATE expense_suggestions SET status = 'accepted', resolved_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update suggestion")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "COMMIT_FAILED", "Failed to commit transaction")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"suggestion_id":  id,
			"artifact_id":    artifactID,
			"classification": suggestedClass,
			"vendor":         vendor,
		},
	})
}

// DismissSuggestion handles POST /api/expenses/suggestions/{id}/dismiss.
func (h *ExpenseHandler) DismissSuggestion(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if h.Pool == nil {
		writeExpenseError(w, http.StatusInternalServerError, "TX_FAILED", "Database pool not available")
		return
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "TX_FAILED", "Failed to start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var vendor, suggestedClass string
	err = tx.QueryRow(ctx, `
		SELECT vendor, suggested_class
		FROM expense_suggestions
		WHERE id = $1 AND status = 'pending'
		FOR UPDATE
	`, id).Scan(&vendor, &suggestedClass)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeExpenseError(w, http.StatusNotFound, "SUGGESTION_NOT_FOUND", "Pending suggestion not found")
			return
		}
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Failed to query suggestion")
		return
	}

	// Mark suggestion as dismissed
	_, err = tx.Exec(ctx, `
		UPDATE expense_suggestions SET status = 'dismissed', resolved_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to update suggestion")
		return
	}

	// Create suppression entry (within transaction for atomicity)
	_, err = tx.Exec(ctx, `
		INSERT INTO expense_suggestion_suppressions (id, vendor, classification)
		VALUES (gen_random_uuid()::text, $1, $2)
		ON CONFLICT (vendor, classification) DO NOTHING
	`, vendor, suggestedClass)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "UPDATE_FAILED", "Failed to create suppression")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "COMMIT_FAILED", "Failed to commit transaction")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"data": map[string]any{
			"suggestion_id": id,
			"vendor":        vendor,
			"suppressed":    true,
		},
	})
}

// Export handles GET /api/expenses/export with CSV generation.
func (h *ExpenseHandler) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "standard"
	}
	if format != "standard" && format != "quickbooks" {
		writeExpenseError(w, http.StatusBadRequest, "INVALID_FORMAT", "Format must be 'standard' or 'quickbooks'")
		return
	}

	// Build filters (same as List)
	conditions := []string{"metadata ? 'expense'"}
	args := []any{}
	argIdx := 1

	from := q.Get("from")
	to := q.Get("to")
	if from != "" {
		if _, err := time.Parse("2006-01-02", from); err != nil {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE_FORMAT", "from must be YYYY-MM-DD")
			return
		}
		conditions = append(conditions, fmt.Sprintf("(metadata->'expense'->>'date')::date >= $%d", argIdx))
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		if _, err := time.Parse("2006-01-02", to); err != nil {
			writeExpenseError(w, http.StatusBadRequest, "INVALID_DATE_FORMAT", "to must be YYYY-MM-DD")
			return
		}
		conditions = append(conditions, fmt.Sprintf("(metadata->'expense'->>'date')::date <= $%d", argIdx))
		args = append(args, to)
		argIdx++
	}
	if classification := q.Get("classification"); classification != "" {
		conditions = append(conditions, fmt.Sprintf("metadata->'expense'->>'classification' = $%d", argIdx))
		args = append(args, classification)
		argIdx++
	}
	if category := q.Get("category"); category != "" {
		conditions = append(conditions, fmt.Sprintf("metadata->'expense'->>'category' = $%d", argIdx))
		args = append(args, category)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Check row count first
	var totalRows int
	err := h.Pool.QueryRow(r.Context(), fmt.Sprintf("SELECT COUNT(*) FROM artifacts WHERE %s", whereClause), args...).Scan(&totalRows)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Count query failed")
		return
	}
	if totalRows > h.Cfg.ExpensesExportMaxRows {
		writeExpenseError(w, http.StatusRequestEntityTooLarge, "EXPORT_TOO_LARGE",
			fmt.Sprintf("Export exceeds maximum of %d rows (%d matched)", h.Cfg.ExpensesExportMaxRows, totalRows))
		return
	}

	// Pre-query distinct currencies for mixed-currency warning (avoids full buffering)
	currencyQuery := fmt.Sprintf(`
		SELECT DISTINCT COALESCE(metadata->'expense'->>'currency', 'USD') AS currency
		FROM artifacts
		WHERE %s
	`, whereClause)
	currencyRows, err := h.Pool.Query(r.Context(), currencyQuery, args...)
	if err != nil {
		writeExpenseError(w, http.StatusInternalServerError, "QUERY_FAILED", "Currency query failed")
		return
	}
	currencies := make(map[string]bool)
	for currencyRows.Next() {
		var c string
		if err := currencyRows.Scan(&c); err != nil {
			continue
		}
		currencies[c] = true
	}
	currencyRows.Close()

	// Build filename
	classification := q.Get("classification")
	if classification == "" {
		classification = "all"
	}
	month := time.Now().Format("2006-01")
	if from != "" {
		t, err := time.Parse("2006-01-02", from)
		if err == nil {
			month = t.Format("2006-01")
		}
	}
	safeClassification := filenameClean.ReplaceAllString(classification, "")
	if safeClassification == "" {
		safeClassification = "all"
	}
	filename := fmt.Sprintf("smackerel-expenses-%s-%s.csv", safeClassification, month)

	// Set headers
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Mixed currency warning
	if len(currencies) > 1 {
		currList := make([]string, 0, len(currencies))
		for c := range currencies {
			currList = append(currList, c)
		}
		_ = csvWriter.Write([]string{fmt.Sprintf("# Note: Multiple currencies present (%s). No conversion applied.", strings.Join(currList, ", "))})
	}

	stdDateFmt := h.Cfg.ExpensesExportStdDateFormat
	qbDateFmt := h.Cfg.ExpensesExportQBDateFormat

	// Write header
	if format == "quickbooks" {
		_ = csvWriter.Write([]string{"Date", "Payee", "Category", "Amount", "Memo"})
	} else {
		_ = csvWriter.Write([]string{"Date", "Vendor", "Description", "Category", "Amount", "Currency", "Tax", "Payment Method", "Classification", "Source", "Artifact ID"})
	}

	// Stream rows directly from DB cursor to CSV writer (no in-memory buffering)
	dataQuery := fmt.Sprintf(`
		SELECT metadata->'expense' AS expense, id, source_id, title
		FROM artifacts
		WHERE %s
		ORDER BY (metadata->'expense'->>'date')::date ASC NULLS LAST, id ASC
	`, whereClause)

	rows, err := h.Pool.Query(r.Context(), dataQuery, args...)
	if err != nil {
		slog.Error("export data query failed", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var expJSON json.RawMessage
		var rowID, source, title string
		if err := rows.Scan(&expJSON, &rowID, &source, &title); err != nil {
			continue
		}
		var exp domain.ExpenseMetadata
		if err := json.Unmarshal(expJSON, &exp); err != nil {
			continue
		}

		dateStr := ""
		if exp.Date != nil {
			t, err := time.Parse("2006-01-02", *exp.Date)
			if err == nil {
				if format == "quickbooks" {
					dateStr = t.Format(qbDateFmt)
				} else {
					dateStr = t.Format(stdDateFmt)
				}
			}
		}
		amount := ""
		if exp.Amount != nil {
			amount = *exp.Amount
		}
		tax := ""
		if exp.Tax != nil {
			tax = *exp.Tax
		}
		paymentMethod := ""
		if exp.PaymentMethod != nil {
			paymentMethod = *exp.PaymentMethod
		}

		if format == "quickbooks" {
			categoryDisplay := exp.Category
			if h.ClassifyEngine != nil {
				categoryDisplay = h.ClassifyEngine.CategoryDisplayName(exp.Category)
			}
			memo := fmt.Sprintf("Source: %s", source)
			if exp.Notes != nil && *exp.Notes != "" {
				memo = *exp.Notes
			}
			_ = csvWriter.Write([]string{
				dateStr,
				sanitizeCSVCell(exp.Vendor),
				sanitizeCSVCell(categoryDisplay),
				amount,
				sanitizeCSVCell(memo),
			})
		} else {
			_ = csvWriter.Write([]string{
				dateStr,
				sanitizeCSVCell(exp.Vendor),
				sanitizeCSVCell(title),
				sanitizeCSVCell(exp.Category),
				amount, exp.Currency, tax,
				sanitizeCSVCell(paymentMethod),
				exp.Classification, source, rowID,
			})
		}
	}
}

func writeExpenseError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]string{"code": code, "message": message},
	})
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
