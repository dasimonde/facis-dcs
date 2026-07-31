package auth

import (
	"context"
	"reflect"

	goa "goa.design/goa/v3/pkg"
)

type accessMetadata struct {
	Scope, Justification string
	DID                  *string
}
type accessMetadataKey struct{}

// AccessMetadataMiddleware enriches the existing authentication-attempt log
// with decoded request fields before JWT authorization runs.
func AccessMetadataMiddleware(next goa.Endpoint) goa.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		metadata := accessMetadata{}
		value := reflect.ValueOf(request)
		if value.Kind() == reflect.Pointer && !value.IsNil() {
			value = value.Elem()
		}
		if value.IsValid() && value.Kind() == reflect.Struct {
			if field := value.FieldByName("Scope"); field.IsValid() {
				if field.Kind() == reflect.String {
					metadata.Scope = field.String()
				} else if field.Kind() == reflect.Pointer && !field.IsNil() {
					metadata.Scope = field.Elem().String()
				}
			}
			if field := value.FieldByName("Justification"); field.IsValid() && field.Kind() == reflect.String {
				metadata.Justification = field.String()
			}
			if field := value.FieldByName("Did"); field.IsValid() && field.Kind() == reflect.Pointer && !field.IsNil() {
				did := field.Elem().String()
				metadata.DID = &did
			}
		}
		return next(context.WithValue(ctx, accessMetadataKey{}, metadata), request)
	}
}

func accessMetadataFromContext(ctx context.Context) accessMetadata {
	metadata, _ := ctx.Value(accessMetadataKey{}).(accessMetadata)
	return metadata
}
