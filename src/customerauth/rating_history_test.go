package customerauth

import (
	"testing"

	"github.com/google/uuid"
)

func TestValidateCustomerRatingHistoryQueryCursorValues(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	timestamp := "2026-09-22T12:30:45.123Z"
	for _, test := range []struct {
		name    string
		sort    string
		value   string
		wantErr bool
	}{
		{name: "overall rating lower bound", sort: "overall_rating", value: "1"},
		{name: "overall rating upper bound", sort: "overall_rating", value: "5"},
		{name: "updated timestamp", sort: "updated_at", value: timestamp},
		{name: "session timestamp", sort: "session_start_time", value: timestamp},
		{name: "rating below range", sort: "overall_rating", value: "0", wantErr: true},
		{name: "rating above range", sort: "overall_rating", value: "6", wantErr: true},
		{name: "malformed rating", sort: "overall_rating", value: "NaN", wantErr: true},
		{name: "malformed update timestamp", sort: "updated_at", value: "not-a-time", wantErr: true},
		{name: "malformed session timestamp", sort: "session_start_time", value: "null", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := test.value
			query := CustomerRatingHistoryQuery{
				SortBy: test.sort, CursorValue: &value, CursorID: &id,
			}
			err := validateCustomerRatingHistoryQuery(&query)
			if (err != nil) != test.wantErr {
				t.Fatalf("validation error=%v, wantErr=%t", err, test.wantErr)
			}
		})
	}
}
