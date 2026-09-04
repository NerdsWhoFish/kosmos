package pagination

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

type record struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func TestCursorRoundTripPreservesStableOrdering(t *testing.T) {
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	spec := Spec{Key: "test.records", OrderBy: "updatedAt", Direction: Descending, ValueKind: TimeValue}
	items := []record{
		{ID: "d", UpdatedAt: now.Add(-time.Hour)},
		{ID: "b", UpdatedAt: now},
		{ID: "c", UpdatedAt: now.Add(-time.Hour)},
		{ID: "a", UpdatedAt: now},
	}

	firstRequest, err := Parse(httptest.NewRequest("GET", "/records?limit=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	first := append([]record(nil), items...)
	firstMetadata, err := Apply(&first, firstRequest, spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{first[0].ID, first[1].ID}; got[0] != "b" || got[1] != "a" || firstMetadata.NextCursor == "" {
		t.Fatalf("first page IDs = %v, metadata = %#v", got, firstMetadata)
	}

	secondRequest, err := Parse(httptest.NewRequest("GET", "/records?limit=2&cursor="+firstMetadata.NextCursor, nil))
	if err != nil {
		t.Fatal(err)
	}
	second := append([]record(nil), items...)
	secondMetadata, err := Apply(&second, secondRequest, spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{second[0].ID, second[1].ID}; got[0] != "d" || got[1] != "c" || secondMetadata.NextCursor != "" {
		t.Fatalf("second page IDs = %v, metadata = %#v", got, secondMetadata)
	}
	value, id, ok, err := secondRequest.After(spec)
	if err != nil || !ok || id != "a" || !value.(time.Time).Equal(now) {
		t.Fatalf("decoded cursor = %v, %q, %t, %v", value, id, ok, err)
	}
}

func TestPageBoundaryDoesNotEmitCursorForExactPage(t *testing.T) {
	spec := Spec{Key: "test.names", OrderBy: "name", Direction: Ascending, ValueKind: StringValue}
	request, err := Parse(httptest.NewRequest("GET", "/records?limit=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	items := []record{{ID: "two", Name: "B"}, {ID: "one", Name: "A"}}
	metadata, err := Apply(&items, request, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "one" || metadata.NextCursor != "" {
		t.Fatalf("page = %#v, metadata = %#v", items, metadata)
	}
}

func TestCursorIsBoundToQuery(t *testing.T) {
	spec := Spec{Key: "test.filtered", OrderBy: "name", Direction: Ascending, ValueKind: StringValue, Filters: []Filter{{Field: "name", Value: "A"}}}
	request, _ := Parse(httptest.NewRequest("GET", "/records?limit=1", nil))
	items := []record{{ID: "one", Name: "A"}, {ID: "two", Name: "A"}}
	metadata, err := Apply(&items, request, spec)
	if err != nil {
		t.Fatal(err)
	}
	cursorRequest, err := Parse(httptest.NewRequest("GET", "/records?cursor="+metadata.NextCursor, nil))
	if err != nil {
		t.Fatal(err)
	}
	wrongSpec := spec
	wrongSpec.Key = "test.other-filter"
	if _, _, _, err := cursorRequest.After(wrongSpec); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("After() error = %v", err)
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   error
	}{
		{name: "zero limit", target: "/records?limit=0", want: ErrInvalidLimit},
		{name: "limit above maximum", target: "/records?limit=101", want: ErrInvalidLimit},
		{name: "noncanonical limit", target: "/records?limit=050", want: ErrInvalidLimit},
		{name: "malformed cursor", target: "/records?cursor=not-a-cursor", want: ErrInvalidCursor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(httptest.NewRequest("GET", test.target, nil))
			if !errors.Is(err, test.want) {
				t.Fatalf("Parse() error = %v, want %v", err, test.want)
			}
		})
	}
}
