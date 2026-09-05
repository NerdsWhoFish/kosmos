package operations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"net/mail"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"go.opentelemetry.io/otel"
)

const signingConsent = "I agree to use electronic records and signatures, have reviewed this document, and intend my signature to be binding."

var signingIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var signingUploadSlot = make(chan struct{}, 1)

type SigningPage struct {
	Width  float64 `json:"width" firestore:"width"`
	Height float64 `json:"height" firestore:"height"`
}

type SigningField struct {
	ID       string  `json:"id" firestore:"id"`
	Type     string  `json:"type" firestore:"type"`
	Label    string  `json:"label" firestore:"label"`
	Page     int     `json:"page" firestore:"page"`
	X        float64 `json:"x" firestore:"x"`
	Y        float64 `json:"y" firestore:"y"`
	Width    float64 `json:"width" firestore:"width"`
	Height   float64 `json:"height" firestore:"height"`
	Required bool    `json:"required" firestore:"required"`
}

type SigningEvent struct {
	Action string    `json:"action" firestore:"action"`
	At     time.Time `json:"at" firestore:"at"`
}

type SigningRequest struct {
	ID                  string         `json:"id" firestore:"id"`
	Title               string         `json:"title" firestore:"title"`
	FileName            string         `json:"fileName" firestore:"fileName"`
	Status              string         `json:"status" firestore:"status"`
	Pages               []SigningPage  `json:"pages" firestore:"pages"`
	Fields              []SigningField `json:"fields" firestore:"fields"`
	Revision            int            `json:"revision" firestore:"revision"`
	SignerName          string         `json:"signerName" firestore:"signerName"`
	CompletedSignerName string         `json:"completedSignerName,omitempty" firestore:"completedSignerName,omitempty"`
	SignerEmail         string         `json:"signerEmail" firestore:"signerEmail"`
	CreatedAt           time.Time      `json:"createdAt" firestore:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt" firestore:"updatedAt"`
	ExpiresAt           *time.Time     `json:"expiresAt,omitempty" firestore:"expiresAt,omitempty"`
	CompletedAt         *time.Time     `json:"completedAt,omitempty" firestore:"completedAt,omitempty"`
	OriginalSHA256      string         `json:"originalSHA256" firestore:"originalSHA256"`
	UploadedSHA256      string         `json:"uploadedSHA256,omitempty" firestore:"uploadedSHA256,omitempty"`
	Flattened           bool           `json:"flattened,omitempty" firestore:"flattened,omitempty"`
	SignedSHA256        string         `json:"signedSHA256,omitempty" firestore:"signedSHA256,omitempty"`
	Events              []SigningEvent `json:"events" firestore:"events"`
	Consent             string         `json:"consent,omitempty" firestore:"consent,omitempty"`
	OriginalObject      string         `json:"-" firestore:"originalObject"`
	UploadedObject      string         `json:"-" firestore:"uploadedObject,omitempty"`
	SignedObject        string         `json:"-" firestore:"signedObject,omitempty"`
	TokenHash           string         `json:"-" firestore:"tokenHash,omitempty"`
	CreatedBy           string         `json:"-" firestore:"createdBy"`
}

func (m *Module) registerSigningRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/signing-requests", m.signingRequests)
	mux.HandleFunc("POST /api/v1/signing-requests", m.createSigningRequest)
	mux.HandleFunc("GET /api/v1/signing-requests/{id}", m.getSigningRequest)
	mux.HandleFunc("PUT /api/v1/signing-requests/{id}", m.editSigningRequest)
	mux.HandleFunc("POST /api/v1/signing-requests/{id}/link", m.createSigningLink)
	mux.HandleFunc("POST /api/v1/signing-requests/{id}/revoke", m.revokeSigningRequest)
	mux.HandleFunc("GET /api/v1/signing-requests/{id}/pdf", m.signingPDF)
	mux.HandleFunc("GET /api/v1/signing/{id}", m.publicSigningRequest)
	mux.HandleFunc("GET /api/v1/signing/{id}/pdf", m.publicSigningPDF)
	mux.HandleFunc("POST /api/v1/signing/{id}/complete", m.completeSigningRequest)
}

func signingHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
}

func (m *Module) signingRequests(w http.ResponseWriter, r *http.Request) {
	signingHeaders(w)
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	writeStorePage[SigningRequest](w, r, m.store, scope, "signingRequests", "requests", "signing_load_failed", "Could not load signing requests", pagination.Spec{Key: "operations.signing-requests", OrderBy: "createdAt", Direction: pagination.Descending, ValueKind: pagination.TimeValue})
}

func (m *Module) createSigningRequest(w http.ResponseWriter, r *http.Request) {
	signingHeaders(w)
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	ctx, span := otel.Tracer("kosmos.signing").Start(r.Context(), "signing.upload")
	defer span.End()
	r = r.WithContext(ctx)
	if !m.allowSigningRate(w, r, scope, "upload", 20) {
		return
	}
	select {
	case signingUploadSlot <- struct{}{}:
		defer func() { <-signingUploadSlot }()
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "signing_busy", "Another document is being prepared. Please try again shortly.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+(64<<10))
	if err := r.ParseMultipartForm(maxUploadSize + (64 << 10)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Upload one PDF no larger than 10 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	title := strings.TrimSpace(r.FormValue("title"))
	if !signingText(title, 160) {
		writeError(w, http.StatusBadRequest, "invalid_title", "Enter a document title of 1 to 160 supported characters")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil || len(r.MultipartForm.File["file"]) != 1 {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Choose one PDF")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
	if err != nil || len(data) > maxUploadSize {
		writeError(w, http.StatusBadRequest, "invalid_pdf", "Could not read PDF, or it exceeds 10 MB")
		return
	}
	prepared, pages, flattened, err := prepareSigningPDF(ctx, data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pdf", err.Error())
		return
	}
	id, err := newID()
	if err != nil {
		signingFailure(w)
		return
	}
	name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if len(name) > 180 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		name = "document.pdf"
	}
	now := time.Now().UTC()
	item := SigningRequest{ID: id, Title: title, FileName: name, Status: "draft", Pages: pages, Fields: []SigningField{}, Revision: 1, CreatedAt: now, UpdatedAt: now, OriginalSHA256: signingHash(prepared), UploadedSHA256: signingHash(data), Flattened: flattened, OriginalObject: scope + "/signing/" + id + "/original.pdf", CreatedBy: actor.Email, Events: []SigningEvent{{Action: "created", At: now}}}
	objects := []string{item.OriginalObject}
	cleanup := func() {
		for _, object := range objects {
			m.cleanupSigningObject(ctx, scope, item.ID, object)
		}
	}
	if flattened {
		item.UploadedObject = scope + "/signing/" + id + "/uploaded.pdf"
		objects = append(objects, item.UploadedObject)
		if err := m.blobs.Put(ctx, item.UploadedObject, "application/pdf", bytes.NewReader(data)); err != nil {
			cleanup()
			signingFailure(w)
			return
		}
	}
	if err := m.blobs.Put(ctx, item.OriginalObject, "application/pdf", bytes.NewReader(prepared)); err != nil {
		cleanup()
		signingFailure(w)
		return
	}
	if err := m.store.Create(ctx, scope, "signingRequests", id, item); err != nil {
		cleanup()
		signingFailure(w)
		return
	}
	slog.InfoContext(ctx, "signing request created", "signing.flattened", flattened)
	writeJSON(w, http.StatusCreated, item)
}

func (m *Module) loadSigningRequest(w http.ResponseWriter, r *http.Request, scope string) (SigningRequest, bool) {
	signingHeaders(w)
	var item SigningRequest
	if !signingIDPattern.MatchString(r.PathValue("id")) {
		writeError(w, http.StatusNotFound, "signing_not_found", "Signing request not found")
		return item, false
	}
	err := m.store.Get(r.Context(), scope, "signingRequests", r.PathValue("id"), &item)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusNotFound, "signing_not_found", "Signing request not found")
		return item, false
	}
	if err != nil {
		signingFailure(w)
		return item, false
	}
	return cloneSigningRequest(item), true
}

func (m *Module) getSigningRequest(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	item, ok := m.loadSigningRequest(w, r, scope)
	if ok {
		writeJSON(w, http.StatusOK, item)
	}
}

func validateSigningFields(fields []SigningField, pages []SigningPage, requireSignature bool) error {
	if len(fields) > 100 {
		return errors.New("Use at most 100 fields")
	}
	seen := make(map[string]bool)
	hasSignature := false
	for _, field := range fields {
		if !signingIDPattern.MatchString(field.ID) || seen[field.ID] {
			return errors.New("Each field needs a unique identifier")
		}
		seen[field.ID] = true
		if !oneOf(field.Type, "signature", "date", "name", "text") || !signingText(field.Label, 80) {
			return errors.New("Choose a supported field type and a label of 1 to 80 characters")
		}
		if field.Page < 1 || field.Page > len(pages) {
			return errors.New("Place each field on a document page")
		}
		for _, value := range []float64{field.X, field.Y, field.Width, field.Height} {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return errors.New("Field position is invalid")
			}
		}
		if field.X < 0 || field.Y < 0 || field.Width < 0.05 || field.Height < 0.015 || field.X+field.Width > 1 || field.Y+field.Height > 1 {
			return errors.New("Keep fields within the page and large enough to read")
		}
		if field.Type == "signature" && field.Required {
			hasSignature = true
		}
	}
	if requireSignature && !hasSignature {
		return errors.New("Add at least one required signature field")
	}
	return validateSigningFieldLayout(fields, pages)
}

func (m *Module) editSigningRequest(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	item, ok := m.loadSigningRequest(w, r, scope)
	if !ok {
		return
	}
	var input struct {
		Revision int            `json:"revision"`
		Fields   []SigningField `json:"fields"`
	}
	if !decodeJSON(w, r, &input, 128<<10) {
		return
	}
	if item.Status != "draft" || input.Revision != item.Revision {
		signingConflict(w)
		return
	}
	if err := validateSigningFields(input.Fields, item.Pages, false); err != nil {
		writeError(w, 400, "invalid_fields", err.Error())
		return
	}
	item.Fields = nonNil(input.Fields)
	if m.saveSigningRequest(w, r, scope, item, "draft") {
		writeJSON(w, 200, itemWithNextRevision(item))
	}
}

func (m *Module) createSigningLink(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	item, ok := m.loadSigningRequest(w, r, scope)
	if !ok {
		return
	}
	var input struct {
		Revision    int    `json:"revision"`
		SignerName  string `json:"signerName"`
		SignerEmail string `json:"signerEmail"`
		ExpiresDays int    `json:"expiresDays"`
	}
	if !decodeJSON(w, r, &input, 4096) {
		return
	}
	if item.Status != "draft" || input.Revision != item.Revision {
		signingConflict(w)
		return
	}
	input.SignerName = strings.TrimSpace(input.SignerName)
	input.SignerEmail = strings.TrimSpace(input.SignerEmail)
	address, err := mail.ParseAddress(input.SignerEmail)
	if !signingText(input.SignerName, 120) || err != nil || address.Address != input.SignerEmail || !signingText(input.SignerEmail, 254) || input.ExpiresDays < 1 || input.ExpiresDays > 90 {
		writeError(w, 400, "invalid_recipient", "Enter the recipient's name, email address, and an expiry from 1 to 90 days")
		return
	}
	if err := validateSigningFields(item.Fields, item.Pages, true); err != nil {
		writeError(w, 400, "invalid_fields", err.Error())
		return
	}
	if err := validateSigningPreparedFields(item.Fields, item.Pages, input.SignerName); err != nil {
		writeError(w, 400, "invalid_fields", err.Error())
		return
	}
	if len(m.key) < 32 {
		signingFailure(w)
		return
	}
	secret := m.signingToken(scope, item.ID)
	expires := time.Now().UTC().Add(time.Duration(input.ExpiresDays) * 24 * time.Hour)
	item.TokenHash, item.Status, item.SignerName, item.SignerEmail, item.ExpiresAt = signingHash([]byte(secret)), "pending", input.SignerName, input.SignerEmail, &expires
	item.Events = append(item.Events, SigningEvent{Action: "link_created", At: time.Now().UTC()})
	if !m.saveSigningRequest(w, r, scope, item, "draft") {
		return
	}
	writeJSON(w, 200, map[string]any{"request": itemWithNextRevision(item), "signingUrl": "/sign#" + item.ID + "." + secret})
}

func (m *Module) revokeSigningRequest(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	item, ok := m.loadSigningRequest(w, r, scope)
	if !ok {
		return
	}
	var input struct {
		Revision int `json:"revision"`
	}
	if !decodeJSON(w, r, &input, 1024) {
		return
	}
	if input.Revision != item.Revision || !oneOf(item.Status, "draft", "pending") {
		signingConflict(w)
		return
	}
	previous := item.Status
	item.Status = "revoked"
	item.Events = append(item.Events, SigningEvent{Action: "revoked", At: time.Now().UTC()})
	if m.saveSigningRequest(w, r, scope, item, previous) {
		writeJSON(w, 200, itemWithNextRevision(item))
	}
}

func itemWithNextRevision(item SigningRequest) SigningRequest {
	item.Revision++
	item.UpdatedAt = time.Now().UTC()
	return item
}

func (m *Module) saveSigningRequest(w http.ResponseWriter, r *http.Request, scope string, item SigningRequest, previous string) bool {
	store, ok := m.store.(signingStore)
	if !ok {
		signingFailure(w)
		return false
	}
	err := store.ReplaceSigningRequest(r.Context(), scope, item.Revision, previous, itemWithNextRevision(item))
	if errors.Is(err, errSigningConflict) {
		signingConflict(w)
		return false
	}
	if err != nil {
		signingFailure(w)
		return false
	}
	slog.InfoContext(r.Context(), "signing request updated", "signing.status", item.Status)
	return true
}

func (m *Module) publicSigningAccess(w http.ResponseWriter, r *http.Request) (SigningRequest, bool) {
	signingHeaders(w)
	token := r.Header.Get("X-Kosmos-Signing-Token")
	if len(token) != 43 || len(r.Header.Values("X-Kosmos-Signing-Token")) != 1 {
		writeError(w, 404, "signing_not_found", "This signing link is invalid")
		return SigningRequest{}, false
	}
	if len(m.key) < 32 || subtle.ConstantTimeCompare([]byte(token), []byte(m.signingToken(m.publicScope, r.PathValue("id")))) != 1 {
		writeError(w, 404, "signing_not_found", "This signing link is invalid")
		return SigningRequest{}, false
	}
	item, ok := m.loadSigningRequest(w, r, m.publicScope)
	if !ok {
		return item, false
	}
	if subtle.ConstantTimeCompare([]byte(item.TokenHash), []byte(signingHash([]byte(token)))) != 1 {
		writeError(w, 404, "signing_not_found", "This signing link is invalid")
		return item, false
	}
	if item.Status == "revoked" || item.ExpiresAt == nil || !item.ExpiresAt.After(time.Now()) {
		writeError(w, 410, "signing_unavailable", "This signing link has expired or been revoked. Ask the sender for a new request.")
		return item, false
	}
	if !oneOf(item.Status, "pending", "completed") {
		writeError(w, 404, "signing_not_found", "This signing link is invalid")
		return item, false
	}
	if !m.allowSigningRate(w, r, m.publicScope, "request:"+item.ID, 120) {
		return item, false
	}
	return item, true
}

func (m *Module) allowSigningRate(w http.ResponseWriter, r *http.Request, scope, key string, limit int) bool {
	allowed, retry, err := m.store.AllowRateLimit(r.Context(), scope, deterministicID("signing|"+key), limit, time.Minute, time.Now().UTC())
	if err != nil {
		signingFailure(w)
		return false
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
		writeError(w, 429, "signing_rate_limited", "Too many requests. Please try again shortly.")
		return false
	}
	return true
}

func (m *Module) publicSigningRequest(w http.ResponseWriter, r *http.Request) {
	item, ok := m.publicSigningAccess(w, r)
	if ok {
		writeJSON(w, 200, item)
	}
}

func (m *Module) signingPDF(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	item, ok := m.loadSigningRequest(w, r, scope)
	if ok {
		m.serveSigningPDF(w, r, item, r.URL.Query().Get("uploaded") == "true")
	}
}

func (m *Module) publicSigningPDF(w http.ResponseWriter, r *http.Request) {
	item, ok := m.publicSigningAccess(w, r)
	if ok {
		if r.URL.Query().Get("uploaded") == "true" {
			writeError(w, 400, "invalid_pdf_version", "The uploaded source is available only to the sender")
			return
		}
		m.serveSigningPDF(w, r, item, false)
	}
}

func (m *Module) serveSigningPDF(w http.ResponseWriter, r *http.Request, item SigningRequest, uploaded bool) {
	object := item.OriginalObject
	name := "document.pdf"
	if uploaded {
		if r.URL.Query().Get("completed") == "true" {
			writeError(w, 400, "invalid_pdf_version", "Choose the uploaded or completed PDF")
			return
		}
		if item.UploadedObject != "" {
			object = item.UploadedObject
		}
		name = "uploaded-document.pdf"
	}
	if r.URL.Query().Get("completed") == "true" {
		if item.Status != "completed" || item.SignedObject == "" {
			writeError(w, 409, "signing_incomplete", "This document has not been signed yet")
			return
		}
		object, name = item.SignedObject, "signed-document.pdf"
	}
	source, err := m.blobs.Open(r.Context(), object)
	if err != nil {
		signingFailure(w)
		return
	}
	defer source.Close()
	signingHeaders(w)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, source); err != nil {
		slog.ErrorContext(r.Context(), "signing PDF transfer failed")
	}
}

func (m *Module) completeSigningRequest(w http.ResponseWriter, r *http.Request) {
	ctx, span := otel.Tracer("kosmos.signing").Start(r.Context(), "signing.complete")
	defer span.End()
	r = r.WithContext(ctx)
	item, ok := m.publicSigningAccess(w, r)
	if !ok {
		return
	}
	if r.Header.Get("X-Kosmos-CSRF") != "1" {
		writeError(w, 403, "csrf_required", "Signing must be submitted from the signing page")
		return
	}
	var input struct {
		Values     map[string]string `json:"values"`
		SignerName string            `json:"signerName"`
		Consent    bool              `json:"consent"`
	}
	if !decodeJSON(w, r, &input, 64<<10) {
		return
	}
	if item.Status == "completed" {
		writeJSON(w, 200, item)
		return
	}
	if !m.allowSigningRate(w, r, m.publicScope, "complete:"+item.ID, 10) {
		return
	}
	input.SignerName = strings.TrimSpace(input.SignerName)
	if !input.Consent || !signingText(input.SignerName, 120) {
		writeError(w, 400, "consent_required", "Enter your full name and agree to electronic signing")
		return
	}
	now := time.Now().UTC()
	values := make(map[string]string, len(item.Fields))
	known := make(map[string]bool, len(item.Fields))
	for _, field := range item.Fields {
		known[field.ID] = true
		value := strings.TrimSpace(input.Values[field.ID])
		switch field.Type {
		case "date":
			value = now.Format("2006-01-02")
		case "name":
			value = input.SignerName
		}
		if (field.Required && value == "") || (value != "" && !signingText(value, 200)) {
			writeError(w, 400, "invalid_values", "Complete every required field using at most 200 supported characters")
			return
		}
		values[field.ID] = value
	}
	for id := range input.Values {
		if !known[id] {
			writeError(w, 400, "invalid_values", "A submitted field does not belong to this request")
			return
		}
	}
	source, err := m.blobs.Open(ctx, item.OriginalObject)
	if err != nil {
		signingFailure(w)
		return
	}
	data, err := io.ReadAll(io.LimitReader(source, maxUploadSize+1))
	_ = source.Close()
	if err != nil || len(data) > maxUploadSize || signingHash(data) != item.OriginalSHA256 {
		signingFailure(w)
		return
	}
	certificate := SigningCertificate{ID: item.ID, DocumentTitle: item.Title, SignerName: input.SignerName, SignerEmail: item.SignerEmail, OriginalSHA256: item.OriginalSHA256, UploadedSHA256: item.UploadedSHA256, SignedAt: now, Consent: signingConsent}
	output, err := renderSigningPDF(data, item.Fields, values, certificate)
	if err != nil {
		writeError(w, 400, "signing_render_failed", err.Error())
		return
	}
	attempt, err := newID()
	if err != nil {
		signingFailure(w)
		return
	}
	item.SignedObject = m.publicScope + "/signing/" + item.ID + "/signed-" + attempt + ".pdf"
	if err := m.blobs.Put(ctx, item.SignedObject, "application/pdf", bytes.NewReader(output)); err != nil {
		m.cleanupSigningObject(ctx, m.publicScope, item.ID, item.SignedObject)
		signingFailure(w)
		return
	}
	item.Status, item.CompletedSignerName, item.CompletedAt, item.SignedSHA256, item.Consent = "completed", input.SignerName, &now, signingHash(output), signingConsent
	item.Events = append(item.Events, SigningEvent{Action: "completed", At: now})
	store, ok := m.store.(signingStore)
	if !ok {
		m.cleanupSigningObject(ctx, m.publicScope, item.ID, item.SignedObject)
		signingFailure(w)
		return
	}
	err = store.ReplaceSigningRequest(ctx, m.publicScope, item.Revision, "pending", itemWithNextRevision(item))
	if err != nil {
		// A timeout may follow a committed transaction. Never delete its winning PDF.
		var current SigningRequest
		readErr := m.store.Get(ctx, m.publicScope, "signingRequests", item.ID, &current)
		if readErr == nil {
			if current.SignedObject != item.SignedObject {
				_ = m.blobs.Delete(ctx, item.SignedObject)
			}
			if current.Status == "completed" {
				writeJSON(w, 200, current)
				return
			}
		}
		if errors.Is(err, errSigningConflict) {
			signingConflict(w)
		} else {
			signingFailure(w)
		}
		return
	}
	slog.InfoContext(ctx, "document signing completed")
	writeJSON(w, 200, itemWithNextRevision(item))
}

func (m *Module) cleanupSigningObject(ctx context.Context, scope, id, object string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var current SigningRequest
	err := m.store.Get(ctx, scope, "signingRequests", id, &current)
	if errors.Is(err, errNotFound) || (err == nil && current.OriginalObject != object && current.UploadedObject != object && current.SignedObject != object) {
		if err := m.blobs.Delete(ctx, object); err != nil && !errors.Is(err, errNotFound) {
			slog.ErrorContext(ctx, "signing orphan cleanup failed")
		}
	} else if err != nil {
		slog.ErrorContext(ctx, "signing orphan ownership check failed")
	}
}

func (m *Module) signingToken(scope, id string) string {
	return sign(m.key, "kosmos-signing-v1|"+scope+"|"+id)
}

func signingHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func signingText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit && strings.IndexFunc(value, unicode.IsControl) < 0 && validateSigningText(value) == nil
}

func signingFailure(w http.ResponseWriter) {
	writeError(w, 500, "signing_failed", "Could not save or load this signing request. Please try again.")
}
func signingConflict(w http.ResponseWriter) {
	writeError(w, 409, "signing_changed", "This request changed or is no longer editable. Reload it before continuing.")
}
