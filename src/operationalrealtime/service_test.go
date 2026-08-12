package operationalrealtime

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/google/uuid"
)

func TestParseCursor(t *testing.T) {
	t.Parallel()

	after, limit, err := ParseCursor("42", "25")
	if err != nil || after != 42 || limit != 25 {
		t.Fatalf("ParseCursor valid = (%d, %d, %v), want (42, 25, nil)", after, limit, err)
	}
	for _, input := range [][2]string{{"-1", ""}, {"not-a-number", ""}, {"", "0"}, {"", "501"}} {
		if _, _, err := ParseCursor(input[0], input[1]); err == nil {
			t.Fatalf("ParseCursor(%q, %q) accepted invalid input", input[0], input[1])
		}
	}
}

func TestWriteSSEUsesDurableEventIDAndSafeJSON(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := WriteSSE(&output, []models.OperationalEvent{{
		ID: 7, CPOID: uuid.New(), EventType: "charger.live_state_changed",
		ResourceType: "CHARGER", ResourceID: "charger-1", Data: models.JSONB{},
		OccurredAt: time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC),
	}})
	if err != nil {
		t.Fatalf("WriteSSE: %v", err)
	}
	text := output.String()
	for _, expected := range []string{"id: 7\n", "event: charger.live_state_changed\n", "\"resource_id\":\"charger-1\"", "\n\n"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("SSE frame %q missing %q", text, expected)
		}
	}
}
