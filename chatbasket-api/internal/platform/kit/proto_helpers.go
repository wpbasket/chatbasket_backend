package kit

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// OptionalTimestamp returns a *timestamppb.Timestamp suitable for an
// optional google.protobuf.Timestamp field. nil inputs produce nil,
// keeping the field absent on the wire.
func OptionalTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// OptionalString returns a *string suitable for an optional string field.
// Empty inputs produce nil, keeping the field absent on the wire.
func OptionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
