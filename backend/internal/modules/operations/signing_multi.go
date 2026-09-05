package operations

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const maxSigningSigners = 10

type SigningSignerInput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type SigningSigner struct {
	ID                  string            `json:"id" firestore:"id"`
	Name                string            `json:"name" firestore:"name"`
	Email               string            `json:"email,omitempty" firestore:"email"`
	CompletedSignerName string            `json:"completedSignerName,omitempty" firestore:"completedSignerName,omitempty"`
	SignedAt            *time.Time        `json:"signedAt,omitempty" firestore:"signedAt,omitempty"`
	Consent             string            `json:"consent,omitempty" firestore:"consent,omitempty"`
	Session             *SigningSession   `json:"session,omitempty" firestore:"session,omitempty"`
	SignedSHA256        string            `json:"signedSHA256,omitempty" firestore:"signedSHA256,omitempty"`
	TokenHash           string            `json:"-" firestore:"tokenHash,omitempty"`
	Values              map[string]string `json:"-" firestore:"values,omitempty"`
	SignedObject        string            `json:"-" firestore:"signedObject,omitempty"`
}

func prepareSigningSigners(input []SigningSignerInput) ([]SigningSigner, error) {
	if len(input) < 1 || len(input) > maxSigningSigners {
		return nil, errors.New("Choose between 1 and 10 signers")
	}
	seen := make(map[string]bool, len(input))
	signers := make([]SigningSigner, len(input))
	for i, signer := range input {
		signer.Name, signer.Email = strings.TrimSpace(signer.Name), strings.TrimSpace(signer.Email)
		address, err := mail.ParseAddress(signer.Email)
		if !signingIDPattern.MatchString(signer.ID) || seen[signer.ID] || !signingText(signer.Name, 120) || !signingText(signer.Email, 254) || err != nil || address.Address != signer.Email {
			return nil, errors.New("Each signer needs a unique identifier, name, and email address")
		}
		seen[signer.ID] = true
		signers[i] = SigningSigner{ID: signer.ID, Name: signer.Name, Email: signer.Email}
	}
	return signers, nil
}

func signingSignerByID(signers []SigningSigner, id string) *SigningSigner {
	for i := range signers {
		if signers[i].ID == id {
			return &signers[i]
		}
	}
	return nil
}

func validateSigningAssignments(fields []SigningField, signers []SigningSigner, requireSignature bool) error {
	if len(signers) == 0 {
		for _, field := range fields {
			if field.SignerID != "" {
				return errors.New("Add the signer before assigning a field")
			}
		}
		return nil
	}
	required := make(map[string]bool, len(signers))
	for _, field := range fields {
		if signingSignerByID(signers, field.SignerID) == nil {
			return errors.New("Assign every field to a signer on this request")
		}
		if field.Type == "signature" && field.Required {
			required[field.SignerID] = true
		}
	}
	if requireSignature {
		for _, signer := range signers {
			if !required[signer.ID] {
				return errors.New("Add at least one required signature field for every signer")
			}
		}
	}
	return nil
}

func (m *Module) createParallelSigningLinks(w http.ResponseWriter, r *http.Request, scope string, item SigningRequest, expiresDays int) {
	if expiresDays < 1 || expiresDays > 90 {
		writeError(w, 400, "invalid_recipient", "Choose a signing expiry from 1 to 90 days")
		return
	}
	if err := validateSigningFields(item.Fields, item.Pages, true); err != nil {
		writeError(w, 400, "invalid_fields", err.Error())
		return
	}
	if err := validateSigningAssignments(item.Fields, item.Signers, true); err != nil {
		writeError(w, 400, "invalid_fields", err.Error())
		return
	}
	for _, signer := range item.Signers {
		var fields []SigningField
		for _, field := range item.Fields {
			if field.SignerID == signer.ID {
				fields = append(fields, field)
			}
		}
		if err := validateSigningPreparedFields(fields, item.Pages, signer.Name); err != nil {
			writeError(w, 400, "invalid_fields", err.Error())
			return
		}
	}
	if len(m.key) < 32 {
		signingFailure(w)
		return
	}
	type link struct {
		SignerID   string `json:"signerId"`
		SigningURL string `json:"signingUrl"`
	}
	links := make([]link, len(item.Signers))
	for i := range item.Signers {
		signer := &item.Signers[i]
		token := m.signingRecipientToken(scope, item.ID, signer.ID)
		signer.TokenHash = signingHash([]byte(token))
		links[i] = link{SignerID: signer.ID, SigningURL: "/sign#" + item.ID + "." + token}
	}
	now := time.Now().UTC()
	expires := now.Add(time.Duration(expiresDays) * 24 * time.Hour)
	item.Status, item.ExpiresAt = "pending", &expires
	item.TokenHash, item.SignerName, item.SignerEmail = "", "", ""
	item.Events = append(item.Events, SigningEvent{Action: "link_created", At: now})
	next := itemWithNextRevision(item)
	store, ok := m.store.(signingStore)
	if !ok {
		signingFailure(w)
		return
	}
	if err := store.ReplaceSigningRequest(r.Context(), scope, item.Revision, "draft", next); err != nil {
		if errors.Is(err, errSigningConflict) || errors.Is(err, errNotFound) {
			signingConflict(w)
		} else {
			signingFailure(w)
		}
		return
	}
	writeJSON(w, 200, map[string]any{"request": next, "signingLinks": links})
}

func signingTokenRecipient(token string) string {
	if len(token) < 48 || len(token) > 111 || !strings.HasPrefix(token, "s1_") || token[len(token)-44] != '_' {
		return ""
	}
	id := token[3 : len(token)-44]
	if !signingIDPattern.MatchString(id) {
		return ""
	}
	return id
}

func (m *Module) signingRecipientToken(scope, id, signerID string) string {
	return "s1_" + signerID + "_" + sign(m.key, "kosmos-signing-recipient-v1|"+scope+"|"+id+"|"+signerID)
}

func signingSignerExpiry(signer SigningSigner) *time.Time {
	if signer.SignedAt == nil {
		return nil
	}
	expires := signer.SignedAt.Add(signingPostSignWindow)
	return &expires
}

func publicSigningView(item SigningRequest) SigningRequest {
	view := cloneSigningRequest(item)
	view.CurrentSignerID, view.AccessExpiresAt = item.CurrentSignerID, item.AccessExpiresAt
	if len(item.Signers) == 0 {
		return view
	}
	view.Fields = []SigningField{}
	for _, field := range item.Fields {
		if field.SignerID == item.CurrentSignerID {
			view.Fields = append(view.Fields, field)
		}
	}
	view.SignerName, view.SignerEmail, view.CompletedSignerName, view.Consent, view.Session = "", "", "", "", nil
	for i := range view.Signers {
		if view.Signers[i].ID != item.CurrentSignerID {
			view.Signers[i].Email, view.Signers[i].CompletedSignerName, view.Signers[i].Consent = "", "", ""
			view.Signers[i].Session = nil
		}
	}
	return view
}

func (m *Module) completeParallelSigningRequest(w http.ResponseWriter, r *http.Request, item SigningRequest) {
	signerID := item.CurrentSignerID
	currentSigner := signingSignerByID(item.Signers, signerID)
	if currentSigner == nil {
		writeError(w, 403, "signing_download_only", "Use your personal signing link to sign")
		return
	}
	if currentSigner.SignedAt != nil {
		writeJSON(w, 200, publicSigningView(item))
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
	if !m.allowSigningRate(w, r, m.publicScope, "complete:"+item.ID+":"+signerID, 10) {
		return
	}
	input.SignerName = strings.TrimSpace(input.SignerName)
	if !input.Consent || !signingText(input.SignerName, 120) {
		writeError(w, 400, "consent_required", "Enter your full name and agree to electronic signing")
		return
	}
	now := time.Now().UTC()
	values := make(map[string]string)
	for _, field := range item.Fields {
		if field.SignerID != signerID {
			continue
		}
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
		if _, ok := values[id]; !ok {
			writeError(w, 400, "invalid_values", "A submitted field is not assigned to you")
			return
		}
	}
	session, err := captureSigningSession(r, m.intakeSigningKey, m.verifySignedIntake, now)
	if err != nil {
		writeError(w, 403, "signing_session_unverified", "Signing session could not be verified. Reload the signing page and try again.")
		return
	}
	candidate := *currentSigner
	candidate.CompletedSignerName, candidate.SignedAt, candidate.Consent, candidate.Session, candidate.Values = input.SignerName, &now, signingConsent, &session, values
	source, err := m.blobs.Open(r.Context(), item.OriginalObject)
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
	store, ok := m.store.(signingStore)
	if !ok {
		signingFailure(w)
		return
	}
	for attempt := 0; attempt <= maxSigningSigners; attempt++ {
		if item.Status != "pending" && item.Status != "completed" {
			signingConflict(w)
			return
		}
		currentSigner = signingSignerByID(item.Signers, signerID)
		if currentSigner == nil {
			signingConflict(w)
			return
		}
		if currentSigner.SignedAt != nil {
			item.CurrentSignerID, item.AccessExpiresAt = signerID, signingSignerExpiry(*currentSigner)
			writeJSON(w, 200, publicSigningView(item))
			return
		}
		if item.Status != "pending" || item.ExpiresAt == nil || !item.ExpiresAt.After(time.Now()) {
			writeError(w, 410, "signing_unavailable", "This signing link has expired")
			return
		}
		next := cloneSigningRequest(item)
		*signingSignerByID(next.Signers, signerID) = candidate
		allSigned := true
		for _, signer := range next.Signers {
			allSigned = allSigned && signer.SignedAt != nil
		}
		if allSigned {
			completedAt := time.Now().UTC()
			next.Status, next.CompletedAt = "completed", &completedAt
		}
		fields, allValues, certificate := parallelSigningDocument(next)
		output, err := renderSigningPDF(data, fields, allValues, certificate)
		if err != nil {
			writeError(w, 400, "signing_render_failed", err.Error())
			return
		}
		objectID, err := newID()
		if err != nil {
			signingFailure(w)
			return
		}
		object := m.publicScope + "/signing/" + item.ID + "/signed-" + objectID + ".pdf"
		if err := m.blobs.Put(r.Context(), object, "application/pdf", bytes.NewReader(output)); err != nil {
			m.cleanupSigningObject(r.Context(), m.publicScope, item.ID, object)
			signingFailure(w)
			return
		}
		next.SignedObject, next.SignedSHA256 = object, signingHash(output)
		winner := signingSignerByID(next.Signers, signerID)
		winner.SignedObject, winner.SignedSHA256 = object, next.SignedSHA256
		next.Events = append(next.Events, SigningEvent{Action: "signer_completed", At: now})
		if allSigned {
			next.Events = append(next.Events, SigningEvent{Action: "completed", At: *next.CompletedAt})
		}
		next = itemWithNextRevision(next)
		err = store.ReplaceSigningRequest(r.Context(), m.publicScope, item.Revision, "pending", next)
		if err == nil {
			next.CurrentSignerID, next.AccessExpiresAt = signerID, signingSignerExpiry(candidate)
			slog.InfoContext(r.Context(), "signer completed document", "signing.status", next.Status)
			writeJSON(w, 200, publicSigningView(next))
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		var current SigningRequest
		readErr := m.store.Get(ctx, m.publicScope, "signingRequests", item.ID, &current)
		cancel()
		m.cleanupSigningObject(r.Context(), m.publicScope, item.ID, object)
		if readErr != nil {
			if errors.Is(readErr, errNotFound) {
				signingConflict(w)
			} else {
				signingFailure(w)
			}
			return
		}
		if !errors.Is(err, errSigningConflict) {
			committed := signingSignerByID(current.Signers, signerID)
			if committed == nil || committed.SignedAt == nil {
				signingFailure(w)
				return
			}
		}
		item = cloneSigningRequest(current)
	}
	signingConflict(w)
}

func parallelSigningDocument(item SigningRequest) ([]SigningField, map[string]string, SigningCertificate) {
	certificate := SigningCertificate{ID: item.ID, DocumentTitle: item.Title, OriginalSHA256: item.OriginalSHA256, UploadedSHA256: item.UploadedSHA256, Status: item.Status}
	values := make(map[string]string)
	for _, signer := range item.Signers {
		certificate.Signers = append(certificate.Signers, SigningCertificateSigner{ID: signer.ID, Name: signer.Name, Email: signer.Email, CompletedSignerName: signer.CompletedSignerName, SignedAt: signer.SignedAt, Consent: signer.Consent, Session: signer.Session})
		if signer.SignedAt != nil {
			for id, value := range signer.Values {
				values[id] = value
			}
		}
	}
	var fields []SigningField
	for _, field := range item.Fields {
		if signer := signingSignerByID(item.Signers, field.SignerID); signer != nil && signer.SignedAt != nil {
			fields = append(fields, field)
		}
	}
	return fields, values, certificate
}
