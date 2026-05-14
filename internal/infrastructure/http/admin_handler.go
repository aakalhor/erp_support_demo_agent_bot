package http

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/shared/logger"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/usecase"
)

//go:embed templates/*.html
var adminTemplatesFS embed.FS

// AdminHandler renders the admin UI and handles its form submissions.
// All routes are mounted under /admin in router.go.
type AdminHandler struct {
	service *usecase.AdminService
	log     logger.Logger
	tplList *template.Template
	tplEdit *template.Template
}

func NewAdminHandler(s *usecase.AdminService, log logger.Logger) (*AdminHandler, error) {
	funcs := template.FuncMap{
		"joinTags": func(tags []string) string { return strings.Join(tags, ", ") },
		"searchBlob": func(r domain.FAQRecord) string {
			return strings.ToLower(strings.Join([]string{
				r.ID, r.ClientID, r.Module, r.Product, r.Question, r.Answer,
				strings.Join(r.Tags, " "),
			}, " "))
		},
	}
	list, err := template.New("list").Funcs(funcs).
		ParseFS(adminTemplatesFS, "templates/layout.html", "templates/list.html")
	if err != nil {
		return nil, fmt.Errorf("parse list template: %w", err)
	}
	edit, err := template.New("edit").Funcs(funcs).
		ParseFS(adminTemplatesFS, "templates/layout.html", "templates/edit.html")
	if err != nil {
		return nil, fmt.Errorf("parse edit template: %w", err)
	}
	return &AdminHandler{service: s, log: log, tplList: list, tplEdit: edit}, nil
}

// Register mounts every admin route on the provided ServeMux. Uses the
// Go 1.22+ method-aware routing syntax.
func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", h.list)
	mux.HandleFunc("GET /admin/", h.list)
	mux.HandleFunc("GET /admin/faq/new", h.newForm)
	mux.HandleFunc("POST /admin/faq", h.create)
	mux.HandleFunc("GET /admin/faq/{id}/edit", h.editForm)
	mux.HandleFunc("POST /admin/faq/{id}", h.update)
	mux.HandleFunc("POST /admin/faq/{id}/delete", h.delete)
	mux.HandleFunc("POST /admin/reindex", h.reindex)
	mux.HandleFunc("POST /admin/knowledge", h.saveKnowledge)
}

// pageData is the value passed to every template. Keep keys here in
// sync with the layout/list/edit templates.
type pageData struct {
	Title            string
	Stats            string
	Flash            string
	FlashKind        string // "ok" | "err"
	Records          []domain.FAQRecord
	Clients          []string
	Record           domain.FAQRecord
	IsNew            bool
	PostAction       string
	GeneralKnowledge string
}

func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	records, err := h.service.List()
	if err != nil {
		h.fail(w, "list", err)
		return
	}
	clients := uniqueClients(records)

	stats := fmt.Sprintf("%d records · %d client IDs", len(records), len(clients))
	flash, kind := flashFromQuery(r.URL.Query())

	data := pageData{
		Title:            "FAQs",
		Stats:            stats,
		Flash:            flash,
		FlashKind:        kind,
		Records:          records,
		Clients:          clients,
		GeneralKnowledge: h.service.GeneralKnowledge(),
	}
	if err := h.tplList.ExecuteTemplate(w, "layout", data); err != nil {
		h.log.Errorf("render list: %v", err)
	}
}

func (h *AdminHandler) saveKnowledge(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.redirectFlash(w, r, "/admin", "Invalid form: "+err.Error(), "err")
		return
	}
	text := r.FormValue("text")
	if err := h.service.SaveGeneralKnowledge(text); err != nil {
		h.redirectFlash(w, r, "/admin", "Submit failed: "+err.Error(), "err")
		return
	}
	msg := "General knowledge submitted to the bot."
	if strings.TrimSpace(text) == "" {
		msg = "General knowledge cleared; the bot will rely solely on the FAQs."
	}
	h.redirectFlash(w, r, "/admin", msg, "ok")
}

func (h *AdminHandler) newForm(w http.ResponseWriter, r *http.Request) {
	flash, kind := flashFromQuery(r.URL.Query())
	data := pageData{
		Title:      "New FAQ",
		Stats:      "new record",
		Flash:      flash,
		FlashKind:  kind,
		Record:     domain.FAQRecord{ClientID: domain.ClientGlobal, RiskLevel: domain.RiskLow, SourceType: "admin"},
		IsNew:      true,
		PostAction: "/admin/faq",
	}
	if err := h.tplEdit.ExecuteTemplate(w, "layout", data); err != nil {
		h.log.Errorf("render new: %v", err)
	}
}

func (h *AdminHandler) editForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := h.service.Get(id)
	if err != nil {
		h.redirectFlash(w, r, "/admin", "Could not load FAQ: "+err.Error(), "err")
		return
	}
	flash, kind := flashFromQuery(r.URL.Query())
	data := pageData{
		Title:      "Edit " + rec.ID,
		Stats:      "editing " + rec.ID,
		Flash:      flash,
		FlashKind:  kind,
		Record:     rec,
		IsNew:      false,
		PostAction: "/admin/faq/" + rec.ID,
	}
	if err := h.tplEdit.ExecuteTemplate(w, "layout", data); err != nil {
		h.log.Errorf("render edit: %v", err)
	}
}

func (h *AdminHandler) create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.redirectFlash(w, r, "/admin/faq/new", "Invalid form: "+err.Error(), "err")
		return
	}
	rec := recordFromForm(r, domain.FAQRecord{})
	saved, err := h.service.Create(rec)
	if err != nil {
		h.redirectFlash(w, r, "/admin/faq/new", "Create failed: "+err.Error(), "err")
		return
	}
	h.redirectFlash(w, r, "/admin", "Created "+saved.ID+" and reindexed.", "ok")
}

func (h *AdminHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		h.redirectFlash(w, r, "/admin/faq/"+id+"/edit", "Invalid form: "+err.Error(), "err")
		return
	}
	existing, err := h.service.Get(id)
	if err != nil {
		h.redirectFlash(w, r, "/admin", "Could not load FAQ: "+err.Error(), "err")
		return
	}
	rec := recordFromForm(r, existing)
	rec.ID = id // path wins; never let the body change the ID
	saved, err := h.service.Update(rec)
	if err != nil {
		h.redirectFlash(w, r, "/admin/faq/"+id+"/edit", "Save failed: "+err.Error(), "err")
		return
	}
	h.redirectFlash(w, r, "/admin", "Saved "+saved.ID+" and reindexed.", "ok")
}

func (h *AdminHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.service.Delete(id); err != nil {
		h.redirectFlash(w, r, "/admin", "Delete failed: "+err.Error(), "err")
		return
	}
	h.redirectFlash(w, r, "/admin", "Deleted "+id+" and reindexed.", "ok")
}

func (h *AdminHandler) reindex(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Reindex(); err != nil {
		h.redirectFlash(w, r, "/admin", "Reindex failed: "+err.Error(), "err")
		return
	}
	h.redirectFlash(w, r, "/admin", "Reindex complete.", "ok")
}

// recordFromForm populates a FAQRecord from posted form values, using
// the supplied base record for fields the form doesn't touch.
func recordFromForm(r *http.Request, base domain.FAQRecord) domain.FAQRecord {
	get := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	base.ID = get("id")
	base.ClientID = get("client_id")
	base.CompanyType = get("company_type")
	base.Product = get("product")
	base.Module = get("module")
	base.Question = get("question")
	base.Answer = get("answer")
	base.SourceTitle = get("source_title")
	base.SourceType = get("source_type")
	base.RiskLevel = domain.RiskLevel(strings.ToLower(get("risk_level"))).Normalize()
	base.EscalationRequired = r.FormValue("escalation_required") != ""
	base.Tags = splitTags(get("tags"))
	return base
}

func splitTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func uniqueClients(records []domain.FAQRecord) []string {
	seen := make(map[string]struct{}, len(records))
	for _, r := range records {
		seen[r.ClientID] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// fail renders a minimal 500 page when the templates themselves blew
// up. Used sparingly; happy-path errors flow through redirectFlash.
func (h *AdminHandler) fail(w http.ResponseWriter, op string, err error) {
	h.log.Errorf("admin %s: %v", op, err)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("Admin " + op + " failed: " + err.Error()))
}

// redirectFlash performs a POST/REDIRECT/GET with the flash message
// encoded into the query string.
func (h *AdminHandler) redirectFlash(w http.ResponseWriter, r *http.Request, to string, msg string, kind string) {
	q := url.Values{}
	q.Set("flash", msg)
	q.Set("kind", kind)
	sep := "?"
	if strings.Contains(to, "?") {
		sep = "&"
	}
	http.Redirect(w, r, to+sep+q.Encode(), http.StatusSeeOther)
}

func flashFromQuery(q url.Values) (string, string) {
	msg := q.Get("flash")
	if msg == "" {
		return "", ""
	}
	kind := q.Get("kind")
	if kind != "ok" && kind != "err" {
		kind = "ok"
	}
	return msg, kind
}

