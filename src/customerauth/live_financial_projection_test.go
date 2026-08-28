package customerauth

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChargingSessionFinancialProjectionJSONSeparatesProjectedAndFinalAmounts(
	t *testing.T,
) {
	amount := "12.34"

	projected := ChargingSessionFinancialProjectionView{
		ChargingSessionView: ChargingSessionView{
			Currency: "INR",
		},
		ProjectedAmount: &amount,
	}

	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}

	text := string(encoded)

	if !strings.Contains(text, `"projected_amount":"12.34"`) {
		t.Fatalf("projected amount missing from JSON: %s", text)
	}

	if strings.Contains(text, `"total_amount"`) {
		t.Fatalf(
			"final total amount must remain absent before completion: %s",
			text,
		)
	}
}
