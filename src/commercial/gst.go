// Package commercial holds the small, transport-independent rules shared by
// CPO commercial writes and customer charging reads.
package commercial

import (
	"errors"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/shopspring/decimal"
)

var ErrInvalidHubGST = errors.New("invalid hub GST relationship")

// ValidateGSTComponents validates values that are meaningful even before a
// profile is assigned to a Hub. A profile cannot mix split and integrated tax.
func ValidateGSTComponents(sgst, cgst, igst *decimal.Decimal) error {
	if sgst == nil || cgst == nil || igst == nil {
		return ErrInvalidHubGST
	}
	for _, rate := range []*decimal.Decimal{sgst, cgst, igst} {
		if rate.Sign() < 0 || rate.Cmp(decimal.NewFromInt(100)) > 0 {
			return ErrInvalidHubGST
		}
	}
	if !igst.IsZero() && (!sgst.IsZero() || !cgst.IsZero()) {
		return ErrInvalidHubGST
	}
	return nil
}

// ValidateHubGST verifies the complete state/rate relationship, not merely a
// partial PATCH. Zero is valid for a tax component that is not applicable.
func ValidateHubGST(hubState, gstState constants.IndianState, sgst, cgst, igst *decimal.Decimal) error {
	if !hubState.Valid() || !gstState.Valid() || ValidateGSTComponents(sgst, cgst, igst) != nil {
		return ErrInvalidHubGST
	}
	if hubState == gstState {
		if !igst.IsZero() {
			return ErrInvalidHubGST
		}
		return nil
	}
	if !sgst.IsZero() || !cgst.IsZero() || igst == nil {
		return ErrInvalidHubGST
	}
	return nil
}

// GSTMultiplier applies the three persisted components as one commercial tax
// rate. Callers validate structural Hub/GST compatibility separately.
func GSTMultiplier(sgst, cgst, igst decimal.Decimal) decimal.Decimal {
	rate := sgst.Add(cgst).Add(igst).Div(decimal.NewFromInt(100))
	return decimal.NewFromInt(1).Add(rate)
}
