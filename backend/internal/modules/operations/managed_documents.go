package operations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
)

const maxManagedDocumentSize = 100 << 20

var managedDocumentKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type managedDocumentInput struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	Links []workspace.RecordLink `json:"links,omitempty"`
}

type managedFile struct {
	name        string
	contentType string
	content     []byte
	hash        string
}

func (m *Module) syncManagedDocument(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	if actor.Kind != "api" || actor.Access != "write" {
		writeError(w, http.StatusForbidden, "permission_denied", "A read-and-write API credential is required")
		return
	}
	sourceKey, err := url.PathUnescape(strings.TrimSpace(r.PathValue("sourceKey")))
	if err != nil || !managedDocumentKeyPattern.MatchString(sourceKey) {
		writeError(w, http.StatusBadRequest, "invalid_source_key", "Source key must use 1 to 128 letters, numbers, dots, underscores, or hyphens")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxManagedDocumentSize)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_managed_document", "Managed document payload is invalid or larger than 100 MB")
		return
	}
	defer r.MultipartForm.RemoveAll()
	var input managedDocumentInput
	decoder := json.NewDecoder(strings.NewReader(r.FormValue("document")))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_managed_document", "The document part must contain valid JSON")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_managed_document", "The document part must contain valid JSON")
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" || len(input.Title) > 160 || len(input.Body) > 100000 || !validManagedDocumentLinks(input.Links) {
		writeError(w, http.StatusBadRequest, "invalid_managed_document", "Document title, body, or links are invalid")
		return
	}
	files, err := readManagedFiles(r.MultipartForm.File["files"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_managed_file", err.Error())
		return
	}
	before, err := m.workspace.ListDocuments(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "managed_document_sync_failed", "Could not inspect managed documents")
		return
	}
	var previous *workspace.Document
	for index := range before {
		if before[index].SourceKey == sourceKey {
			previous = &before[index]
			break
		}
	}
	document, created, err := m.workspace.SyncManagedDocument(r.Context(), scope, sourceKey, workspace.Document{Title: input.Title, Body: input.Body, Links: input.Links})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "managed_document_sync_failed", "Could not save managed document")
		return
	}
	attachments, changed, err := m.reconcileManagedAttachments(r, scope, actor, document.ID, files)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "managed_document_sync_failed", "Could not synchronize managed document files")
		return
	}
	documentChanged := created || previous == nil || previous.Title != document.Title || previous.Body != document.Body || previous.Revision != document.Revision
	if documentChanged || changed {
		_ = m.audit(r.Context(), scope, actor.Email, "managed_document.synced", "document", document.ID, document.Title)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"document": document, "attachments": attachments})
}

func readManagedFiles(headers []*multipart.FileHeader) ([]managedFile, error) {
	files := make([]managedFile, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		name := filepath.Base(strings.TrimSpace(header.Filename))
		key := strings.ToLower(name)
		if name == "" || name == "." {
			return nil, errors.New("every managed file needs a filename")
		}
		if _, exists := seen[key]; exists {
			return nil, errors.New("managed file basenames must be unique")
		}
		seen[key] = struct{}{}
		file, err := header.Open()
		if err != nil {
			return nil, errors.New("could not read a managed file")
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
		_ = file.Close()
		if readErr != nil || len(content) > maxUploadSize {
			return nil, errors.New("each managed file must be 10 MB or smaller")
		}
		contentType := attachmentContentType(content, name)
		if !supportedAttachmentContentType(contentType) {
			return nil, errors.New("managed files must be PNG, JPEG, WebP, SVG, PDF, Markdown, text, JSON, or CSS")
		}
		digest := sha256.Sum256(content)
		files = append(files, managedFile{name: name, contentType: contentType, content: content, hash: hex.EncodeToString(digest[:])})
	}
	return files, nil
}

func (m *Module) reconcileManagedAttachments(r *http.Request, scope string, actor Identity, documentID string, files []managedFile) ([]Attachment, bool, error) {
	var stored []Attachment
	if err := m.store.List(r.Context(), scope, "attachments", &stored); err != nil {
		return nil, false, err
	}
	existing := make(map[string]Attachment)
	for _, item := range stored {
		if item.RecordType == "document" && item.RecordID == documentID {
			existing[strings.ToLower(item.FileName)] = item
		}
	}
	desired := make(map[string]struct{}, len(files))
	result := make([]Attachment, 0, len(files))
	changed := false
	for _, file := range files {
		key := strings.ToLower(file.name)
		desired[key] = struct{}{}
		id := deterministicID("managed-attachment:" + documentID + ":" + key)
		prior, exists := existing[key]
		if exists && prior.ID == id && prior.ContentHash == file.hash {
			prior.DownloadURL = m.downloadURL(scope, prior.ID, time.Now().Add(15*time.Minute))
			prior.ViewURL = prior.DownloadURL + "&disposition=inline"
			result = append(result, prior)
			continue
		}
		if exists && prior.ID != id {
			if err := m.blobs.Delete(r.Context(), prior.ObjectName); err != nil && !errors.Is(err, errNotFound) {
				return nil, false, err
			}
			if err := m.store.Delete(r.Context(), scope, "attachments", prior.ID); err != nil && !errors.Is(err, errNotFound) {
				return nil, false, err
			}
		}
		objectName := scope + "/" + id + filepath.Ext(file.name)
		if err := m.blobs.Put(r.Context(), objectName, file.contentType, bytes.NewReader(file.content)); err != nil {
			return nil, false, err
		}
		createdAt := time.Now().UTC()
		if exists && !prior.CreatedAt.IsZero() {
			createdAt = prior.CreatedAt
		}
		item := Attachment{ID: id, FileName: file.name, ContentType: file.contentType, Size: int64(len(file.content)), Kind: "attachment", RecordType: "document", RecordID: documentID, ObjectName: objectName, ContentHash: file.hash, CreatedBy: actor.Email, CreatedAt: createdAt}
		if err := m.store.Put(r.Context(), scope, "attachments", item.ID, item); err != nil {
			return nil, false, err
		}
		item.DownloadURL = m.downloadURL(scope, item.ID, time.Now().Add(15*time.Minute))
		item.ViewURL = item.DownloadURL + "&disposition=inline"
		result = append(result, item)
		changed = true
	}
	for key, item := range existing {
		if _, keep := desired[key]; keep {
			continue
		}
		if err := m.blobs.Delete(r.Context(), item.ObjectName); err != nil && !errors.Is(err, errNotFound) {
			return nil, false, err
		}
		if err := m.store.Delete(r.Context(), scope, "attachments", item.ID); err != nil && !errors.Is(err, errNotFound) {
			return nil, false, err
		}
		changed = true
	}
	return result, changed, nil
}

func validManagedDocumentLinks(links []workspace.RecordLink) bool {
	for _, link := range links {
		if !oneOf(link.Type, "account", "contact", "opportunity", "cost", "document") || strings.TrimSpace(link.ID) == "" {
			return false
		}
	}
	return true
}

func attachmentContentType(content []byte, name string) string {
	extension := strings.ToLower(filepath.Ext(name))
	switch extension {
	case ".svg":
		return "image/svg+xml"
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".css":
		return "text/css"
	}
	detected := http.DetectContentType(content)
	if strings.HasPrefix(detected, "text/plain") {
		return "text/plain"
	}
	if detected == "application/octet-stream" && extension == ".pdf" {
		return "application/pdf"
	}
	return detected
}

func supportedAttachmentContentType(contentType string) bool {
	return oneOf(contentType, "image/jpeg", "image/png", "image/webp", "image/svg+xml", "application/pdf", "text/plain", "text/markdown", "application/json", "text/css")
}
