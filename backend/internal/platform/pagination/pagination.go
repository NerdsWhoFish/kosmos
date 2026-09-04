package pagination

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	"net/http"
	"strconv"
)

const (
	DefaultLimit  = 50
	MaxLimit      = 100
	cursorSize    = 9
	cursorVersion = 1
)

var (
	ErrInvalidLimit  = errors.New("limit must be an integer between 1 and 100")
	ErrInvalidCursor = errors.New("cursor is invalid")
)

type Request struct {
	Limit  int
	offset int
}

type Metadata struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"nextCursor"`
}

func Parse(r *http.Request) (Request, error) {
	page := Request{Limit: DefaultLimit}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > MaxLimit || strconv.Itoa(limit) != raw {
			return Request{}, ErrInvalidLimit
		}
		page.Limit = limit
	}

	rawCursor := r.URL.Query().Get("cursor")
	if rawCursor == "" {
		return page, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(rawCursor)
	if err != nil || len(payload) != cursorSize || payload[0] != cursorVersion {
		return Request{}, ErrInvalidCursor
	}
	offset := binary.BigEndian.Uint64(payload[1:])
	if offset == 0 || offset > uint64(math.MaxInt) {
		return Request{}, ErrInvalidCursor
	}
	page.offset = int(offset)
	return page, nil
}

func Slice[T any](items []T, page Request) ([]T, Metadata) {
	metadata := Metadata{Limit: page.Limit}
	if page.offset >= len(items) {
		return []T{}, metadata
	}

	end := page.offset + page.Limit
	if end > len(items) {
		end = len(items)
	}
	if end < len(items) {
		metadata.NextCursor = encodeCursor(end)
	}
	return items[page.offset:end], metadata
}

func encodeCursor(offset int) string {
	payload := make([]byte, cursorSize)
	payload[0] = cursorVersion
	binary.BigEndian.PutUint64(payload[1:], uint64(offset))
	return base64.RawURLEncoding.EncodeToString(payload)
}
