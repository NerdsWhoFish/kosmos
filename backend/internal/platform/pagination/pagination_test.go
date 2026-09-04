package pagination

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	request := httptest.NewRequest("GET", "/records", nil)
	page, err := Parse(request)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if page.Limit != DefaultLimit || page.offset != 0 {
		t.Fatalf("Parse() = %#v", page)
	}
}

func TestSliceCursorRoundTrip(t *testing.T) {
	request := httptest.NewRequest("GET", "/records?limit=2", nil)
	firstRequest, err := Parse(request)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	first, metadata := Slice([]string{"a", "b", "c"}, firstRequest)
	if len(first) != 2 || first[0] != "a" || first[1] != "b" || metadata.NextCursor == "" {
		t.Fatalf("first page = %#v, metadata = %#v", first, metadata)
	}

	request = httptest.NewRequest("GET", "/records?limit=2&cursor="+metadata.NextCursor, nil)
	secondRequest, err := Parse(request)
	if err != nil {
		t.Fatalf("Parse() second page error = %v", err)
	}
	second, metadata := Slice([]string{"a", "b", "c"}, secondRequest)
	if len(second) != 1 || second[0] != "c" || metadata.NextCursor != "" {
		t.Fatalf("second page = %#v, metadata = %#v", second, metadata)
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
