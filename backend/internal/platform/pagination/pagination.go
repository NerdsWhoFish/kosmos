package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultLimit  = 50
	MaxLimit      = 100
	cursorVersion = 2
)

var (
	ErrInvalidLimit  = errors.New("limit must be an integer between 1 and 100")
	ErrInvalidCursor = errors.New("cursor is invalid")
)

type Direction string

const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

type ValueKind string

const (
	StringValue  ValueKind = "string"
	TimeValue    ValueKind = "time"
	IntegerValue ValueKind = "integer"
)

type Filter struct {
	Field string
	Value any
}

type Spec struct {
	Key       string
	OrderBy   string
	Direction Direction
	ValueKind ValueKind
	Filters   []Filter
}

type cursor struct {
	Version int       `json:"v"`
	Key     string    `json:"k"`
	Kind    ValueKind `json:"t"`
	Value   string    `json:"s"`
	ID      string    `json:"i"`
}

type Request struct {
	Limit  int
	cursor *cursor
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
	if err != nil {
		return Request{}, ErrInvalidCursor
	}
	var decoded cursor
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != cursorVersion || decoded.Key == "" || decoded.ID == "" {
		return Request{}, ErrInvalidCursor
	}
	if _, err := decodeValue(decoded.Kind, decoded.Value); err != nil {
		return Request{}, ErrInvalidCursor
	}
	page.cursor = &decoded
	return page, nil
}

func (r Request) After(spec Spec) (any, string, bool, error) {
	if r.cursor == nil {
		return nil, "", false, nil
	}
	if !validSpec(spec) || r.cursor.Key != spec.Key || r.cursor.Kind != spec.ValueKind {
		return nil, "", false, ErrInvalidCursor
	}
	value, err := decodeValue(spec.ValueKind, r.cursor.Value)
	if err != nil {
		return nil, "", false, ErrInvalidCursor
	}
	return value, r.cursor.ID, true, nil
}

func Apply(target any, request Request, spec Spec) (Metadata, error) {
	metadata := Metadata{Limit: request.Limit}
	if !validSpec(spec) {
		return metadata, ErrInvalidCursor
	}
	afterValue, afterID, hasCursor, err := request.After(spec)
	if err != nil {
		return metadata, err
	}
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.Elem().Kind() != reflect.Slice {
		return metadata, errors.New("pagination target must be a slice pointer")
	}
	items := targetValue.Elem()
	filtered := reflect.MakeSlice(items.Type(), 0, items.Len())
	for index := 0; index < items.Len(); index++ {
		item := items.Index(index)
		matches, err := matchesFilters(item, spec.Filters)
		if err != nil {
			return metadata, err
		}
		if matches {
			filtered = reflect.Append(filtered, item)
		}
	}

	var sortErr error
	sort.SliceStable(filtered.Interface(), func(left, right int) bool {
		comparison, err := compareFields(filtered.Index(left), filtered.Index(right), spec)
		if err != nil {
			sortErr = err
			return false
		}
		return comparison < 0
	})
	if sortErr != nil {
		return metadata, sortErr
	}

	start := 0
	if hasCursor {
		start = filtered.Len()
		for index := 0; index < filtered.Len(); index++ {
			comparison, err := compareItemToCursor(filtered.Index(index), spec, afterValue, afterID)
			if err != nil {
				return metadata, err
			}
			if comparison > 0 {
				start = index
				break
			}
		}
	}
	end := start + request.Limit
	hasMore := end < filtered.Len()
	if end > filtered.Len() {
		end = filtered.Len()
	}
	page := filtered.Slice(start, end)
	targetValue.Elem().Set(page)
	if hasMore && page.Len() > 0 {
		last := page.Index(page.Len() - 1)
		value, err := fieldValue(last, spec.OrderBy)
		if err != nil {
			return metadata, err
		}
		id, err := stringField(last, "id")
		if err != nil {
			return metadata, err
		}
		encodedValue, err := encodeValue(spec.ValueKind, value.Interface())
		if err != nil {
			return metadata, err
		}
		payload, err := json.Marshal(cursor{Version: cursorVersion, Key: spec.Key, Kind: spec.ValueKind, Value: encodedValue, ID: id})
		if err != nil {
			return metadata, err
		}
		metadata.NextCursor = base64.RawURLEncoding.EncodeToString(payload)
	}
	return metadata, nil
}

func validSpec(spec Spec) bool {
	return spec.Key != "" && spec.OrderBy != "" && (spec.Direction == Ascending || spec.Direction == Descending) && (spec.ValueKind == StringValue || spec.ValueKind == TimeValue || spec.ValueKind == IntegerValue)
}

func matchesFilters(item reflect.Value, filters []Filter) (bool, error) {
	for _, filter := range filters {
		value, err := fieldValue(item, filter.Field)
		if err != nil {
			return false, err
		}
		if !reflect.DeepEqual(value.Interface(), filter.Value) {
			return false, nil
		}
	}
	return true, nil
}

func compareFields(left, right reflect.Value, spec Spec) (int, error) {
	leftValue, err := fieldValue(left, spec.OrderBy)
	if err != nil {
		return 0, err
	}
	rightValue, err := fieldValue(right, spec.OrderBy)
	if err != nil {
		return 0, err
	}
	comparison, err := compareValues(spec.ValueKind, leftValue.Interface(), rightValue.Interface())
	if err != nil {
		return 0, err
	}
	if spec.Direction == Descending {
		comparison = -comparison
	}
	if comparison != 0 {
		return comparison, nil
	}
	leftID, err := stringField(left, "id")
	if err != nil {
		return 0, err
	}
	rightID, err := stringField(right, "id")
	if err != nil {
		return 0, err
	}
	comparison = strings.Compare(leftID, rightID)
	if spec.Direction == Descending {
		comparison = -comparison
	}
	return comparison, nil
}

func compareItemToCursor(item reflect.Value, spec Spec, cursorValue any, cursorID string) (int, error) {
	value, err := fieldValue(item, spec.OrderBy)
	if err != nil {
		return 0, err
	}
	comparison, err := compareValues(spec.ValueKind, value.Interface(), cursorValue)
	if err != nil {
		return 0, err
	}
	if spec.Direction == Descending {
		comparison = -comparison
	}
	if comparison != 0 {
		return comparison, nil
	}
	id, err := stringField(item, "id")
	if err != nil {
		return 0, err
	}
	comparison = strings.Compare(id, cursorID)
	if spec.Direction == Descending {
		comparison = -comparison
	}
	return comparison, nil
}

func fieldValue(item reflect.Value, name string) (reflect.Value, error) {
	for item.Kind() == reflect.Pointer {
		item = item.Elem()
	}
	if item.Kind() != reflect.Struct {
		return reflect.Value{}, errors.New("pagination item must be a struct")
	}
	typeOfItem := item.Type()
	for index := 0; index < item.NumField(); index++ {
		field := typeOfItem.Field(index)
		if field.Name == name || tagName(field.Tag.Get("firestore")) == name || tagName(field.Tag.Get("json")) == name {
			return item.Field(index), nil
		}
	}
	return reflect.Value{}, errors.New("pagination field " + name + " was not found")
}

func stringField(item reflect.Value, name string) (string, error) {
	value, err := fieldValue(item, name)
	if err != nil || value.Kind() != reflect.String {
		return "", errors.New("pagination field " + name + " must be a string")
	}
	return value.String(), nil
}

func tagName(tag string) string {
	return strings.Split(tag, ",")[0]
}

func encodeValue(kind ValueKind, value any) (string, error) {
	switch kind {
	case StringValue:
		text, ok := value.(string)
		if !ok {
			return "", ErrInvalidCursor
		}
		return text, nil
	case TimeValue:
		value, ok := value.(time.Time)
		if !ok {
			return "", ErrInvalidCursor
		}
		return value.UTC().Format(time.RFC3339Nano), nil
	case IntegerValue:
		reflected := reflect.ValueOf(value)
		if !reflected.IsValid() || reflected.Kind() < reflect.Int || reflected.Kind() > reflect.Int64 {
			return "", ErrInvalidCursor
		}
		return strconv.FormatInt(reflected.Int(), 10), nil
	default:
		return "", ErrInvalidCursor
	}
}

func decodeValue(kind ValueKind, value string) (any, error) {
	switch kind {
	case StringValue:
		return value, nil
	case TimeValue:
		parsed, err := time.Parse(time.RFC3339Nano, value)
		return parsed, err
	case IntegerValue:
		return strconv.ParseInt(value, 10, 64)
	default:
		return nil, ErrInvalidCursor
	}
}

func compareValues(kind ValueKind, left, right any) (int, error) {
	if kind == TimeValue {
		leftTime, leftOK := left.(time.Time)
		rightTime, rightOK := right.(time.Time)
		if !leftOK || !rightOK {
			return 0, ErrInvalidCursor
		}
		if leftTime.Before(rightTime) {
			return -1, nil
		}
		if leftTime.After(rightTime) {
			return 1, nil
		}
		return 0, nil
	}
	leftEncoded, err := encodeValue(kind, left)
	if err != nil {
		return 0, err
	}
	rightEncoded, err := encodeValue(kind, right)
	if err != nil {
		return 0, err
	}
	if kind == IntegerValue {
		leftNumber, _ := strconv.ParseInt(leftEncoded, 10, 64)
		rightNumber, _ := strconv.ParseInt(rightEncoded, 10, 64)
		if leftNumber < rightNumber {
			return -1, nil
		}
		if leftNumber > rightNumber {
			return 1, nil
		}
		return 0, nil
	}
	return strings.Compare(leftEncoded, rightEncoded), nil
}
