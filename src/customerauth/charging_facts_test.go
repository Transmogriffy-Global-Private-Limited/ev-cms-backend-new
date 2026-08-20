package customerauth

import (
	"testing"

	"github.com/google/uuid"
)

func TestEstablishedHALCommandIdentityConflicts(t *testing.T) {
	authoritative := uuid.New()
	different := uuid.New()
	zero := uuid.Nil
	for _, test := range []struct {
		name     string
		recorded *uuid.UUID
		want     bool
	}{
		{name: "unknown nil", recorded: nil, want: false},
		{name: "invalid historical zero", recorded: &zero, want: false},
		{name: "same nonzero", recorded: &authoritative, want: false},
		{name: "different nonzero", recorded: &different, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := establishedHALCommandIdentityConflicts(test.recorded, authoritative); got != test.want {
				t.Fatalf("conflict=%v, want %v", got, test.want)
			}
		})
	}
}
