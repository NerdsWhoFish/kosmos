package firestorepage

import (
	"context"
	"errors"
	"reflect"

	"cloud.google.com/go/firestore"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
	"google.golang.org/api/iterator"
)

func List(ctx context.Context, collection *firestore.CollectionRef, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.Elem().Kind() != reflect.Slice {
		return pagination.Metadata{}, errors.New("pagination target must be a slice pointer")
	}
	direction := firestore.Asc
	if spec.Direction == pagination.Descending {
		direction = firestore.Desc
	}
	query := collection.Query
	for _, filter := range spec.Filters {
		query = query.Where(filter.Field, "==", filter.Value)
	}
	query = query.OrderBy(spec.OrderBy, direction).OrderBy(firestore.DocumentID, direction)
	value, id, hasCursor, err := request.After(spec)
	if err != nil {
		return pagination.Metadata{}, err
	}
	if hasCursor {
		query = query.StartAfter(value, collection.Doc(id))
	}
	query = query.Limit(request.Limit + 1)

	items := reflect.MakeSlice(targetValue.Elem().Type(), 0, request.Limit+1)
	iter := query.Documents(ctx)
	defer iter.Stop()
	for {
		document, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return pagination.Metadata{}, err
		}
		item := reflect.New(items.Type().Elem())
		if err := document.DataTo(item.Interface()); err != nil {
			return pagination.Metadata{}, err
		}
		id := item.Elem().FieldByName("ID")
		if id.IsValid() && id.CanSet() && id.Kind() == reflect.String {
			id.SetString(document.Ref.ID)
		}
		items = reflect.Append(items, item.Elem())
	}
	targetValue.Elem().Set(items)
	return pagination.Apply(target, request, spec)
}
