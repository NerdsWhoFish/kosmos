package operations

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
)

func (m *Module) allowPublicIntake(w http.ResponseWriter, r *http.Request) bool {
	address, err := clientIP(r)
	if m.verifySignedIntake {
		if len(m.intakeSigningKey) < 32 {
			writeError(w, http.StatusServiceUnavailable, "intake_unavailable", "Could not accept this request. Please try again")
			return false
		}
		address, err = signedClientIP(r, m.intakeSigningKey, time.Now().UTC())
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_client_address", "Could not verify this request")
		return false
	}
	key := sign(m.key, "intake:"+address)
	allowed, retryAfter, err := m.store.AllowRateLimit(r.Context(), m.publicScope, key, 5, time.Hour, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "intake_unavailable", "Could not accept this request. Please try again")
		return false
	}
	if !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()+1))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Please wait before trying again")
	}
	return allowed
}

func (m *Module) recordInquiry(w http.ResponseWriter, r *http.Request, contact workspace.Contact, name, company, phone, source, message string) bool {
	details := []string{"Public inquiry", "Name: " + name, "Email: " + contact.Email}
	for _, field := range []struct{ label, value string }{{"Company", company}, {"Phone", phone}, {"Source", source}} {
		if value := strings.TrimSpace(field.value); value != "" {
			details = append(details, field.label+": "+value)
		}
	}
	if message != "" {
		details = append(details, "", message)
	}
	activity, err := m.workspace.CreateActivity(r.Context(), m.publicScope, workspace.Activity{ContactID: contact.ID, Kind: "note", Body: strings.Join(details, "\n"), OccurredAt: time.Now().UTC()})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "inquiry_save_failed", "Could not save your message. Please try again")
		return false
	}
	_ = m.notify(r.Context(), m.publicScope, "New inquiry", name+" via "+normalizedOr(source, "contact-form"), "lead", "/contacts/"+contact.ID, "inquiry:"+activity.ID)
	return true
}
