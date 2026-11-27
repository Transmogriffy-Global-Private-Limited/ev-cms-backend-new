package customerauth

import (
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/models"
)

// VehicleCompatibilityFields are customer declarations, not verified charger or
// BMS facts. Every nil field means unknown; nil is never converted to false or 0.
// Connector identifiers are normalized labels, not an assertion that the HAL
// supports the matching physical plug.
type VehicleCompatibilityFields struct {
	BatteryCapacityWh *int64    `json:"battery_capacity_wh,omitempty"`
	ConnectorTypes    *[]string `json:"connector_types,omitempty"`
	SupportsAC        *bool     `json:"supports_ac,omitempty"`
	SupportsDC        *bool     `json:"supports_dc,omitempty"`
	MaxACPowerW       *int64    `json:"max_ac_power_w,omitempty"`
	MaxDCPowerW       *int64    `json:"max_dc_power_w,omitempty"`
}

var vehicleConnectorLabel = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._+/-]{0,39}$`)

func invalidVehicleCompatibility(field, message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "invalid_vehicle_" + field, Message: message}
}

func copyVehicleI64(p *int64) *int64 {
	if p == nil {
		return nil
	}
	n := *p
	return &n
}
func copyVehicleBool(p *bool) *bool {
	if p == nil {
		return nil
	}
	b := *p
	return &b
}
func copyVehicleStrings(p *[]string) *[]string {
	if p == nil {
		return nil
	}
	a := append([]string(nil), (*p)...)
	return &a
}

func copyVehicleCompatibility(v VehicleCompatibilityFields) VehicleCompatibilityFields {
	return VehicleCompatibilityFields{
		BatteryCapacityWh: copyVehicleI64(v.BatteryCapacityWh),
		ConnectorTypes:    copyVehicleStrings(v.ConnectorTypes),
		SupportsAC:        copyVehicleBool(v.SupportsAC),
		SupportsDC:        copyVehicleBool(v.SupportsDC),
		MaxACPowerW:       copyVehicleI64(v.MaxACPowerW),
		MaxDCPowerW:       copyVehicleI64(v.MaxDCPowerW),
	}
}

func normalizeVehicleCompatibility(v VehicleCompatibilityFields) (VehicleCompatibilityFields, error) {
	n := copyVehicleCompatibility(v)
	for _, item := range []struct {
		name string
		p    *int64
	}{
		{"battery_capacity_wh", n.BatteryCapacityWh},
		{"max_ac_power_w", n.MaxACPowerW},
		{"max_dc_power_w", n.MaxDCPowerW},
	} {
		if item.p != nil && *item.p <= 0 {
			return VehicleCompatibilityFields{}, invalidVehicleCompatibility(item.name, item.name+" must be positive when supplied.")
		}
	}
	if n.SupportsAC != nil && !*n.SupportsAC && n.MaxACPowerW != nil {
		return VehicleCompatibilityFields{}, invalidVehicleCompatibility("max_ac_power_w", "AC intake power cannot be set when supports_ac is false.")
	}
	if n.SupportsDC != nil && !*n.SupportsDC && n.MaxDCPowerW != nil {
		return VehicleCompatibilityFields{}, invalidVehicleCompatibility("max_dc_power_w", "DC intake power cannot be set when supports_dc is false.")
	}
	if n.ConnectorTypes != nil {
		if len(*n.ConnectorTypes) == 0 || len(*n.ConnectorTypes) > 8 {
			return VehicleCompatibilityFields{}, invalidVehicleCompatibility("connector_types", "Supply 1 to 8 connector labels, or null for unknown.")
		}
		seen := make(map[string]bool, len(*n.ConnectorTypes))
		for i, raw := range *n.ConnectorTypes {
			label := strings.ToUpper(strings.TrimSpace(raw))
			if !vehicleConnectorLabel.MatchString(label) || seen[label] {
				return VehicleCompatibilityFields{}, invalidVehicleCompatibility("connector_types", "Connector labels must be unique, nonblank and use valid characters (max 40).")
			}
			seen[label] = true
			(*n.ConnectorTypes)[i] = label
		}
	}
	return n, nil
}

func vehicleCompatibilityFromModel(m models.Vehicle) VehicleCompatibilityFields {
	return copyVehicleCompatibility(VehicleCompatibilityFields{
		BatteryCapacityWh: m.BatteryCapacityWh,
		ConnectorTypes:    m.ConnectorTypes,
		SupportsAC:        m.SupportsAC,
		SupportsDC:        m.SupportsDC,
		MaxACPowerW:       m.MaxACPowerW,
		MaxDCPowerW:       m.MaxDCPowerW,
	})
}

func setVehicleModelCompatibility(m *models.Vehicle, v VehicleCompatibilityFields) {
	v = copyVehicleCompatibility(v)
	m.BatteryCapacityWh = v.BatteryCapacityWh
	m.ConnectorTypes = v.ConnectorTypes
	m.SupportsAC = v.SupportsAC
	m.SupportsDC = v.SupportsDC
	m.MaxACPowerW = v.MaxACPowerW
	m.MaxDCPowerW = v.MaxDCPowerW
}

// A field is applied only if it actually appeared in PATCH. Present null clears.
// Validation is over the resulting whole tuple, including preexisting fields.
func patchVehicleCompatibility(original VehicleCompatibilityFields, request UpdateVehicleRequest) (VehicleCompatibilityFields, error) {
	v := copyVehicleCompatibility(original)
	if request.batteryCapacityWhSet {
		v.BatteryCapacityWh = request.BatteryCapacityWh
	}
	if request.connectorTypesSet {
		v.ConnectorTypes = request.ConnectorTypes
	}
	if request.supportsACSet {
		v.SupportsAC = request.SupportsAC
	}
	if request.supportsDCSet {
		v.SupportsDC = request.SupportsDC
	}
	if request.maxACPowerWSet {
		v.MaxACPowerW = request.MaxACPowerW
	}
	if request.maxDCPowerWSet {
		v.MaxDCPowerW = request.MaxDCPowerW
	}
	return normalizeVehicleCompatibility(v)
}

// Caller holds the vehicle row lock. Mutating only semantic differences avoids
// artificial version increments and audit events for normalization-only no-ops.
func changedVehicleCompatibility(before, after VehicleCompatibilityFields, updates map[string]any, changed []string) []string {
	for _, f := range []struct {
		name, column string
		old, next    any
	}{
		{"battery_capacity_wh", "battery_capacity_wh", before.BatteryCapacityWh, after.BatteryCapacityWh},
		{"connector_types", "connector_types", before.ConnectorTypes, after.ConnectorTypes},
		{"supports_ac", "supports_ac", before.SupportsAC, after.SupportsAC},
		{"supports_dc", "supports_dc", before.SupportsDC, after.SupportsDC},
		{"max_ac_power_w", "max_ac_power_w", before.MaxACPowerW, after.MaxACPowerW},
		{"max_dc_power_w", "max_dc_power_w", before.MaxDCPowerW, after.MaxDCPowerW},
	} {
		if !reflect.DeepEqual(f.old, f.next) {
			updates[f.column] = f.next
			changed = append(changed, f.name)
		}
	}
	return changed
}
