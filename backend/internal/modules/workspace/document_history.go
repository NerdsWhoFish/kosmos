package workspace

import (
	"errors"
	"slices"
	"time"
)

var ErrDocumentConflict = errors.New("document revision changed")

func documentSnapshot(document Document, now time.Time) DocumentRevision {
	return DocumentRevision{DocumentID: document.ID, Title: document.Title, Body: document.Body, Links: slices.Clone(document.Links), Revision: document.Revision, CreatedAt: now}
}
