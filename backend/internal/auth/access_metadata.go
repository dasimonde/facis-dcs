package auth

import (
	"context"
	"errors"
	"log"
	"reflect"

	"digital-contracting-service/internal/auth/db"

	goa "goa.design/goa/v3/pkg"
)

type accessMetadata struct {
	Scope, Justification string
	DID                  *string
}
type accessMetadataKey struct{}

// AccessMetadataMiddleware records decoded metadata and the final access decision.
func AccessMetadataMiddleware(next goa.Endpoint) goa.Endpoint {
	return func(ctx context.Context, request any) (any, error) {
		metadata := accessMetadata{}
		value := reflect.ValueOf(request)
		if value.Kind() == reflect.Pointer && !value.IsNil() {
			value = value.Elem()
		}
		if value.IsValid() && value.Kind() == reflect.Struct {
			metadata.Scope = metadataString(value, "Scope")
			metadata.Justification = metadataString(value, "Justification")
			did := metadataString(value, "Did")
			if did == "" {
				did = metadataString(value, "ResourceID")
			}
			if did != "" {
				metadata.DID = &did
			}
		}
		pending := &accessFinalizer{}
		ctx = context.WithValue(ctx, accessMetadataKey{}, metadata)
		ctx = context.WithValue(ctx, accessFinalizerKey{}, pending)
		result, err := next(ctx, request)
		if pending.record != nil {
			if recordErr := pending.record(err); recordErr != nil {
				// A read that states a justification is an audit read
				// (DCS-FR-PACM-04): it is answered only once its access record
				// is written.
				if metadata.Justification != "" {
					return nil, goa.PermanentError("internal_error", "the access could not be recorded: %v", recordErr)
				}
				log.Printf("could not record an access attempt: %v", recordErr)
			}
		}
		return result, err
	}
}

func accessMetadataFromContext(ctx context.Context) accessMetadata {
	metadata, _ := ctx.Value(accessMetadataKey{}).(accessMetadata)
	return metadata
}

type accessFinalizerKey struct{}
type accessFinalizer struct{ record func(error) error }

// accessOutcome maps the answer of an authenticated request to the access
// log outcome (DCS-FR-CSA-18, DCS-FR-PACM-04): a refusal by the role or read
// policy is DENIED, any other error is FAILED.
func accessOutcome(err error) string {
	if err == nil {
		return db.AccessOutcomeSuccess
	}
	var named interface{ ErrorName() string }
	if errors.As(err, &named) && (named.ErrorName() == "forbidden" || named.ErrorName() == "unauthorized") {
		return db.AccessOutcomeDenied
	}
	return db.AccessOutcomeFailed
}

func metadataString(value reflect.Value, name string) string {
	field := value.FieldByName(name)
	if field.IsValid() && field.Kind() == reflect.Pointer && !field.IsNil() {
		field = field.Elem()
	}
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}
	return ""
}
