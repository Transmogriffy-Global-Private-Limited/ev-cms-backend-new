package customerauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CowAddressFields contains only customer-editable information. A geocode or
// coordinate supplied by a client is unverified until a future geocoder validates it.
type CowAddressFields struct {
	Label                  string   `json:"label"`
	Line1                  string   `json:"line1"`
	Line2                  string   `json:"line2"`
	Locality               string   `json:"locality"`
	City                   string   `json:"city"`
	State                  string   `json:"state"`
	PostalCode             string   `json:"postal_code"`
	CountryCode            string   `json:"country_code"`
	Latitude               *float64 `json:"latitude,omitempty"`
	Longitude              *float64 `json:"longitude,omitempty"`
	Landmark               string   `json:"landmark"`
	BuildingInstructions   string   `json:"building_instructions"`
	Floor                  string   `json:"floor"`
	ContactInstructions    string   `json:"contact_instructions"`
	GeocodeProvider        string   `json:"geocode_provider"`
	GeocodeProviderPlaceID string   `json:"geocode_provider_place_id"`
}

type CreateCowAddressRequest struct {
	CowAddressFields
	IsDefault bool `json:"is_default"`
}

type CowAddressView struct {
	ID uuid.UUID `json:"id"`
	CowAddressFields
	IsDefault bool      `json:"is_default"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CowAddressPage struct {
	Addresses    []CowAddressView `json:"addresses"`
	HasMore      bool             `json:"has_more"`
	NextBefore   *time.Time       `json:"next_before,omitempty"`
	NextBeforeID *uuid.UUID       `json:"next_before_id,omitempty"`
}

type CowAddressPatch struct {
	ExpectedVersion *int64                     `json:"expected_version"`
	Changes         map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON distinguishes omitted fields from explicit null clears. It
// rejects unsupported/server-owned fields before the database is touched.
func (p *CowAddressPatch) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("address update must be an object")
	}
	p.Changes = make(map[string]json.RawMessage)
	for k, v := range raw {
		if k == "expected_version" {
			if err := json.Unmarshal(v, &p.ExpectedVersion); err != nil {
				return err
			}
			continue
		}
		if !cowAddressEditable[k] {
			return fmt.Errorf("unknown or immutable address field %q", k)
		}
		p.Changes[k] = v
	}
	if p.ExpectedVersion == nil || *p.ExpectedVersion < 1 || len(p.Changes) == 0 {
		return errors.New("positive expected_version and at least one editable field required")
	}
	return nil
}

type CowAddressVersionRequest struct {
	ExpectedVersion *int64 `json:"expected_version"`
}

type cowAddressRow struct {
	ID               uuid.UUID `gorm:"column:id;primaryKey"`
	CPOID            uuid.UUID `gorm:"column:cpo_id"`
	CustomerID       uuid.UUID `gorm:"column:customer_id"`
	CowAddressFields `gorm:"embedded"`
	IsDefault        bool      `gorm:"column:is_default"`
	IsActive         bool      `gorm:"column:is_active"`
	Version          int64     `gorm:"column:version"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (cowAddressRow) TableName() string { return "cow_customer_addresses" }

var cowAddressLimits = map[string]int{
	"label": 80, "line1": 255, "line2": 255, "locality": 120,
	"city": 120, "state": 120, "postal_code": 20, "country_code": 2,
	"landmark": 255, "building_instructions": 500, "floor": 40,
	"contact_instructions": 500, "geocode_provider": 80, "geocode_provider_place_id": 255,
}
var cowAddressEditable = map[string]bool{
	"label": true, "line1": true, "line2": true, "locality": true, "city": true,
	"state": true, "postal_code": true, "country_code": true, "latitude": true,
	"longitude": true, "landmark": true, "building_instructions": true, "floor": true,
	"contact_instructions": true, "geocode_provider": true, "geocode_provider_place_id": true,
}

func badCowAddress(code, message string) error {
	return &APIError{Status: http.StatusBadRequest, Code: code, Message: message}
}
func cowAddressConflict() error {
	return &APIError{Status: http.StatusConflict, Code: "address_version_conflict", Message: "Saved address changed. Read it again before updating."}
}
func cowAddressMissing() error {
	return &APIError{Status: http.StatusNotFound, Code: "address_not_found", Message: "Saved address was not found."}
}

func normalizeCowAddress(a *CowAddressFields) error {
	// Reflect over the fixed public struct to keep validation consistent across
	// create, partial update and temporary service-address snapshots.
	v := reflect.ValueOf(a).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() != reflect.String {
			continue
		}
		key := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		val := strings.TrimSpace(f.String())
		if key == "country_code" {
			val = strings.ToUpper(val)
		}
		if utf8.RuneCountInString(val) > cowAddressLimits[key] {
			return badCowAddress("invalid_address", "Address field is too long: "+key)
		}
		f.SetString(val)
	}
	for _, s := range []string{a.Label, a.Line1, a.City, a.State, a.PostalCode, a.CountryCode} {
		if s == "" {
			return badCowAddress("invalid_address", "Label, line1, city, state, postal_code and country_code are required.")
		}
	}
	if len(a.CountryCode) != 2 || a.CountryCode[0] < 'A' || a.CountryCode[0] > 'Z' || a.CountryCode[1] < 'A' || a.CountryCode[1] > 'Z' {
		return badCowAddress("invalid_address", "country_code must be two ISO-letter uppercase characters.")
	}
	if (a.Latitude == nil) != (a.Longitude == nil) {
		return badCowAddress("invalid_coordinates", "Latitude and longitude must be supplied or cleared together.")
	}
	if a.Latitude != nil {
		if math.IsNaN(*a.Latitude) || math.IsInf(*a.Latitude, 0) || *a.Latitude < -90 || *a.Latitude > 90 || math.IsNaN(*a.Longitude) || math.IsInf(*a.Longitude, 0) || *a.Longitude < -180 || *a.Longitude > 180 {
			return badCowAddress("invalid_coordinates", "Coordinates are outside the valid range.")
		}
	}
	if a.GeocodeProviderPlaceID != "" && a.GeocodeProvider == "" {
		return badCowAddress("invalid_address", "geocode_provider is required when provider place ID is supplied.")
	}
	return nil
}

func patchCowAddress(fields *CowAddressFields, changes map[string]json.RawMessage) error {
	// Use a typed JSON round trip to preserve all unmentioned values. The edit
	// whitelist and strict primitive validation prevent arbitrary field writes.
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	var merged map[string]json.RawMessage
	if err = json.Unmarshal(data, &merged); err != nil {
		return err
	}
	for k, v := range changes {
		v = bytes.TrimSpace(v)
		if !cowAddressEditable[k] {
			return badCowAddress("invalid_address", "Unknown address field.")
		}
		if k == "label" || k == "line1" || k == "city" || k == "state" || k == "postal_code" || k == "country_code" {
			if string(v) == "null" {
				return badCowAddress("invalid_address", "Required address field cannot be null.")
			}
		}
		if k == "latitude" || k == "longitude" {
			if string(v) != "null" {
				var n float64
				if err = json.Unmarshal(v, &n); err != nil {
					return badCowAddress("invalid_coordinates", "Coordinate must be numeric.")
				}
			}
		} else if string(v) != "null" {
			var s string
			if err = json.Unmarshal(v, &s); err != nil {
				return badCowAddress("invalid_address", "Address field must be text: "+k)
			}
		}
		if string(v) == "null" && k != "latitude" && k != "longitude" {
			v = json.RawMessage(`""`)
		}
		merged[k] = v
	}
	data, err = json.Marshal(merged)
	if err != nil {
		return err
	}
	// A nil pointer on omitted latitude/longitude remains nil after JSON.
	var target CowAddressFields
	if err = json.Unmarshal(data, &target); err != nil {
		return badCowAddress("invalid_address", "Invalid address update.")
	}
	if err = normalizeCowAddress(&target); err != nil {
		return err
	}
	*fields = target
	return nil
}

// The CPO status and module grant are evaluated at use time. A CPO-row SHARE
// lock for mutations orders them against the Run 1 entitlement UPDATE lock.
func (service *Service) requireCowCustomer(ctx context.Context, tx *gorm.DB, principal Principal, mutating bool) error {
	if principal.CPOID == uuid.Nil || principal.CustomerID == uuid.Nil {
		return &APIError{Status: 403, Code: "forbidden", Message: "Customer session context required."}
	}
	type stateRow struct {
		CPOStatus      string `gorm:"column:cpo_status"`
		CustomerStatus string `gorm:"column:customer_status"`
		Enabled        bool   `gorm:"column:enabled"`
	}
	var state stateRow
	sql := `SELECT c.status AS cpo_status, u.status AS customer_status, COALESCE(e.enabled,false) AS enabled
        FROM customers u JOIN cpos c ON c.id=u.cpo_id
        LEFT JOIN cpo_module_entitlements e ON e.cpo_id=c.id AND e.module_key='cow'
        WHERE u.cpo_id=? AND u.id=?`
	if mutating {
		sql += ` FOR SHARE OF c`
	}
	result := tx.WithContext(ctx).Raw(sql, principal.CPOID, principal.CustomerID).Scan(&state)
	if result.Error != nil {
		return fmt.Errorf("verify CoW address access: %w", result.Error)
	}
	if result.RowsAffected != 1 || state.CPOStatus != string(constants.CPOStatusActive) || state.CustomerStatus != string(constants.CustomerStatusActive) || !state.Enabled {
		return &APIError{Status: 403, Code: "cow_unavailable", Message: "CoW service addresses are not available to this customer."}
	}
	return nil
}

func (service *Service) withCowAddressWrite(ctx context.Context, principal Principal, body func(*gorm.DB) error) error {
	return service.database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := service.requireCowCustomer(ctx, tx, principal, true); err != nil {
			return err
		}
		// Scan the locked identifier into a text field. Raw().Scan(&uuid.UUID)
		// treats the UUID array as a byte slice rather than a UUID scalar.
		var locked struct {
			ID string `gorm:"column:id"`
		}
		result := tx.Raw(`SELECT id::text AS id FROM customers WHERE cpo_id=? AND id=? FOR UPDATE`, principal.CPOID, principal.CustomerID).Scan(&locked)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 || locked.ID != principal.CustomerID.String() {
			return &APIError{Status: 403, Code: "forbidden", Message: "Customer account unavailable."}
		}
		return body(tx)
	})
}

func addressView(row cowAddressRow) CowAddressView {
	return CowAddressView{ID: row.ID, CowAddressFields: row.CowAddressFields, IsDefault: row.IsDefault, Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
func (service *Service) findCowAddress(ctx context.Context, tx *gorm.DB, p Principal, id uuid.UUID) (cowAddressRow, error) {
	if id == uuid.Nil {
		return cowAddressRow{}, badCowAddress("invalid_address_id", "Address ID must be a non-zero UUID.")
	}
	var row cowAddressRow
	err := tx.WithContext(ctx).Where("id=? AND cpo_id=? AND customer_id=? AND is_active=true", id, p.CPOID, p.CustomerID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return cowAddressRow{}, cowAddressMissing()
	}
	if err != nil {
		return cowAddressRow{}, err
	}
	return row, nil
}

func (service *Service) CreateCowAddress(ctx context.Context, p Principal, req CreateCowAddressRequest) (CowAddressView, error) {
	if err := normalizeCowAddress(&req.CowAddressFields); err != nil {
		return CowAddressView{}, err
	}
	var out CowAddressView
	err := service.withCowAddressWrite(ctx, p, func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&cowAddressRow{}).Where("cpo_id=? AND customer_id=? AND is_active=true", p.CPOID, p.CustomerID).Count(&n).Error; err != nil {
			return err
		}
		now := service.now().UTC()
		makeDefault := req.IsDefault || n == 0
		previousDefault := ""
		if makeDefault && n > 0 {
			oldID, err := service.cowCurrentDefaultID(tx, p)
			if err != nil {
				return err
			}
			if oldID != uuid.Nil {
				previousDefault = oldID.String()
			}
		}
		if makeDefault && n > 0 {
			if err := service.clearCowAddressDefault(tx, p, now).Error; err != nil {
				return err
			}
		}
		row := cowAddressRow{ID: uuid.New(), CPOID: p.CPOID, CustomerID: p.CustomerID, CowAddressFields: req.CowAddressFields, IsDefault: makeDefault, IsActive: true, Version: 1, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := createCustomerAudit(tx, p.CustomerID, p.CPOID, "COW_ADDRESS_CREATED", "COW_ADDRESS", row.ID, models.JSONB{"is_default": makeDefault, "previous_default_address_id": previousDefault}, now); err != nil {
			return err
		}
		out = addressView(row)
		return nil
	})
	return out, err
}

// Caller holds the customer row lock; this reads identity only, never address PII.
func (service *Service) cowCurrentDefaultID(tx *gorm.DB, p Principal) (uuid.UUID, error) {
	var row cowAddressRow
	err := tx.Select("id").Where("cpo_id=? AND customer_id=? AND is_active=true AND is_default=true", p.CPOID, p.CustomerID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}
func (service *Service) clearCowAddressDefault(tx *gorm.DB, p Principal, now time.Time) *gorm.DB {
	return tx.Model(&cowAddressRow{}).Where("cpo_id=? AND customer_id=? AND is_active=true AND is_default=true", p.CPOID, p.CustomerID).Updates(map[string]any{"is_default": false, "version": gorm.Expr("version+1"), "updated_at": now})
}
func (service *Service) GetCowAddress(ctx context.Context, p Principal, id uuid.UUID) (CowAddressView, error) {
	if err := service.requireCowCustomer(ctx, service.database, p, false); err != nil {
		return CowAddressView{}, err
	}
	row, err := service.findCowAddress(ctx, service.database, p, id)
	if err != nil {
		return CowAddressView{}, err
	}
	return addressView(row), nil
}
func (service *Service) ListCowAddresses(ctx context.Context, p Principal, limit int, before *time.Time, beforeID *uuid.UUID) (CowAddressPage, error) {
	if err := service.requireCowCustomer(ctx, service.database, p, false); err != nil {
		return CowAddressPage{}, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return CowAddressPage{}, badCowAddress("invalid_limit", "Limit must be 1 to 100.")
	}
	if (before == nil) != (beforeID == nil) {
		return CowAddressPage{}, badCowAddress("invalid_cursor", "before and before_id must be supplied together.")
	}
	q := service.database.WithContext(ctx).Model(&cowAddressRow{}).Where("cpo_id=? AND customer_id=? AND is_active=true", p.CPOID, p.CustomerID)
	if before != nil {
		if *beforeID == uuid.Nil {
			return CowAddressPage{}, badCowAddress("invalid_cursor", "before_id cannot be zero UUID.")
		}
		q = q.Where("(created_at,id)<(?,?)", *before, *beforeID)
	}
	var rows []cowAddressRow
	if err := q.Order("created_at DESC,id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return CowAddressPage{}, err
	}
	page := CowAddressPage{Addresses: make([]CowAddressView, 0, cowMinInt(len(rows), limit)), HasMore: len(rows) > limit}
	if page.HasMore {
		rows = rows[:limit]
	}
	for _, row := range rows {
		page.Addresses = append(page.Addresses, addressView(row))
	}
	if page.HasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		page.NextBefore = &last.CreatedAt
		page.NextBeforeID = &last.ID
	}
	return page, nil
}
func cowMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (service *Service) UpdateCowAddress(ctx context.Context, p Principal, id uuid.UUID, patch CowAddressPatch) (CowAddressView, error) {
	if patch.ExpectedVersion == nil || *patch.ExpectedVersion < 1 || len(patch.Changes) == 0 {
		return CowAddressView{}, badCowAddress("invalid_request", "Positive expected_version and editable fields are required.")
	}
	var out CowAddressView
	err := service.withCowAddressWrite(ctx, p, func(tx *gorm.DB) error {
		row, err := service.findCowAddress(ctx, tx, p, id)
		if err != nil {
			return err
		}
		if row.Version != *patch.ExpectedVersion {
			return cowAddressConflict()
		}
		prev := row.CowAddressFields
		if err := patchCowAddress(&row.CowAddressFields, patch.Changes); err != nil {
			return err
		}
		if reflect.DeepEqual(prev, row.CowAddressFields) {
			out = addressView(row)
			return nil
		}
		if row.Version == math.MaxInt64 {
			return cowAddressConflict()
		}
		now := service.now().UTC()
		row.Version++
		row.UpdatedAt = now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		keys := make([]string, 0, len(patch.Changes))
		for k := range patch.Changes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if err := createCustomerAudit(tx, p.CustomerID, p.CPOID, "COW_ADDRESS_UPDATED", "COW_ADDRESS", id, models.JSONB{"changed_fields": keys}, now); err != nil {
			return err
		}
		out = addressView(row)
		return nil
	})
	return out, err
}
func (service *Service) SetCowAddressDefault(ctx context.Context, p Principal, id uuid.UUID, version int64) (CowAddressView, error) {
	if version < 1 {
		return CowAddressView{}, badCowAddress("invalid_request", "Positive expected_version is required.")
	}
	var out CowAddressView
	err := service.withCowAddressWrite(ctx, p, func(tx *gorm.DB) error {
		row, err := service.findCowAddress(ctx, tx, p, id)
		if err != nil {
			return err
		}
		if row.Version != version {
			return cowAddressConflict()
		}
		if row.IsDefault {
			out = addressView(row)
			return nil
		}
		if row.Version == math.MaxInt64 {
			return cowAddressConflict()
		}
		oldID, err := service.cowCurrentDefaultID(tx, p)
		if err != nil {
			return err
		}
		now := service.now().UTC()
		if err := service.clearCowAddressDefault(tx, p, now).Error; err != nil {
			return err
		}
		row.IsDefault = true
		row.Version++
		row.UpdatedAt = now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if err := createCustomerAudit(tx, p.CustomerID, p.CPOID, "COW_ADDRESS_DEFAULT_CHANGED", "COW_ADDRESS", id, models.JSONB{"previous_default_address_id": oldID.String(), "new_default_address_id": id.String()}, now); err != nil {
			return err
		}
		out = addressView(row)
		return nil
	})
	return out, err
}
func (service *Service) DeleteCowAddress(ctx context.Context, p Principal, id uuid.UUID, version int64) error {
	if version < 1 {
		return badCowAddress("invalid_request", "Positive expected_version query parameter is required.")
	}
	return service.withCowAddressWrite(ctx, p, func(tx *gorm.DB) error {
		row, err := service.findCowAddress(ctx, tx, p, id)
		if err != nil {
			return err
		}
		if row.Version != version {
			return cowAddressConflict()
		}
		if row.Version == math.MaxInt64 {
			return cowAddressConflict()
		}
		wasDefault := row.IsDefault
		replacementID := ""
		now := service.now().UTC()
		row.IsActive = false
		row.IsDefault = false
		row.Version++
		row.UpdatedAt = now
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if wasDefault {
			var next cowAddressRow
			result := tx.Where("cpo_id=? AND customer_id=? AND is_active=true", p.CPOID, p.CustomerID).Order("created_at ASC,id ASC").Take(&next)
			if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return result.Error
			}
			if result.Error == nil {
				if next.Version == math.MaxInt64 {
					return cowAddressConflict()
				}
				replacementID = next.ID.String()
				next.IsDefault = true
				next.Version++
				next.UpdatedAt = now
				if err := tx.Save(&next).Error; err != nil {
					return err
				}
			}
		}
		return createCustomerAudit(tx, p.CustomerID, p.CPOID, "COW_ADDRESS_REMOVED", "COW_ADDRESS", id, models.JSONB{"was_default": wasDefault, "replacement_default_address_id": replacementID}, now)
	})
}

func cowAddressID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("address_id"))
	if err != nil || id == uuid.Nil {
		writeError(c, badCowAddress("invalid_address_id", "Address ID must be a non-zero UUID."))
		return uuid.Nil, false
	}
	return id, true
}
func cowAddressPrincipal(c *gin.Context) (Principal, bool) {
	p, ok := CurrentPrincipal(c)
	if !ok {
		writeError(c, errUnauthorized)
	}
	return p, ok
}
func cowAddressDeleteVersion(c *gin.Context) (int64, bool) {
	value := strings.TrimSpace(c.Query("expected_version"))
	if value == "" {
		writeError(c, badCowAddress("invalid_request", "expected_version query parameter must contain the current positive numeric version."))
		return 0, false
	}
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil || v < 1 {
		writeError(c, badCowAddress("invalid_request", "expected_version query parameter must contain the current positive numeric version."))
		return 0, false
	}
	return v, true
}
func (h *Handler) cowCreateAddress(c *gin.Context) {
	p, ok := cowAddressPrincipal(c)
	if !ok {
		return
	}
	var req CreateCowAddressRequest
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, invalidRequest(err))
		return
	}
	view, err := h.service.CreateCowAddress(c.Request.Context(), p, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, view)
}
func (h *Handler) cowListAddresses(c *gin.Context) {
	p, ok := cowAddressPrincipal(c)
	if !ok {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil {
			writeError(c, badCowAddress("invalid_limit", "Limit must be 1 to 100."))
			return
		}
	}
	var before *time.Time
	var beforeID *uuid.UUID
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(c, badCowAddress("invalid_cursor", "before must be RFC3339 time."))
			return
		}
		before = &t
	}
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(c, badCowAddress("invalid_cursor", "before_id must be a UUID."))
			return
		}
		beforeID = &id
	}
	view, err := h.service.ListCowAddresses(c.Request.Context(), p, limit, before, beforeID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, view)
}
func (h *Handler) cowGetAddress(c *gin.Context) {
	p, ok := cowAddressPrincipal(c)
	if !ok {
		return
	}
	id, ok := cowAddressID(c)
	if !ok {
		return
	}
	view, err := h.service.GetCowAddress(c.Request.Context(), p, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, view)
}
func (h *Handler) cowUpdateAddress(c *gin.Context) {
	p, ok := cowAddressPrincipal(c)
	if !ok {
		return
	}
	id, ok := cowAddressID(c)
	if !ok {
		return
	}
	var req CowAddressPatch
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, invalidRequest(err))
		return
	}
	view, err := h.service.UpdateCowAddress(c.Request.Context(), p, id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, view)
}
func (h *Handler) cowDefaultAddress(c *gin.Context) {
	p, ok := cowAddressPrincipal(c)
	if !ok {
		return
	}
	id, ok := cowAddressID(c)
	if !ok {
		return
	}
	var req CowAddressVersionRequest
	if err := decodeJSON(c, &req); err != nil {
		writeError(c, invalidRequest(err))
		return
	}
	if req.ExpectedVersion == nil {
		writeError(c, badCowAddress("invalid_request", "expected_version is required."))
		return
	}
	view, err := h.service.SetCowAddressDefault(c.Request.Context(), p, id, *req.ExpectedVersion)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, view)
}
func (h *Handler) cowDeleteAddress(c *gin.Context) {
	p, ok := cowAddressPrincipal(c)
	if !ok {
		return
	}
	id, ok := cowAddressID(c)
	if !ok {
		return
	}
	version, ok := cowAddressDeleteVersion(c)
	if !ok {
		return
	}
	if err := h.service.DeleteCowAddress(c.Request.Context(), p, id, version); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
