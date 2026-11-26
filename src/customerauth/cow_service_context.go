package customerauth

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Snapshot values are independent copies of mutable customer master data.
// ServiceJob writes must persist them as JSON at job creation and MUST NOT
// read mutable master rows to reconstruct historical job facts.
type CowVehicleSnapshot struct {
	Source         string     `json:"source"` // SAVED or TEMPORARY
	SavedVehicleID *uuid.UUID `json:"saved_vehicle_id,omitempty"`
	VehicleNumber  string     `json:"vehicle_number"`
	VehicleType    *string    `json:"vehicle_type,omitempty"`
	VehicleMake    *string    `json:"vehicle_make,omitempty"`
	VehicleModel   *string    `json:"vehicle_model,omitempty"`
}

type CowAddressSnapshot struct {
	Source         string     `json:"source"`
	SavedAddressID *uuid.UUID `json:"saved_address_id,omitempty"`
	CowAddressFields
}

type CowServiceContextSnapshot struct {
	Vehicle    CowVehicleSnapshot `json:"vehicle"`
	Address    CowAddressSnapshot `json:"address"`
	CapturedAt time.Time          `json:"captured_at"`
}

type CowServiceContextInput struct {
	SavedVehicleID   *uuid.UUID            `json:"saved_vehicle_id,omitempty"`
	TemporaryVehicle *CreateVehicleRequest `json:"temporary_vehicle,omitempty"`
	SavedAddressID   *uuid.UUID            `json:"saved_address_id,omitempty"`
	TemporaryAddress *CowAddressFields     `json:"temporary_address,omitempty"`
}

func cowOwnCopyString(src *string) *string {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}
func cowOwnCopyFloat(src *float64) *float64 {
	if src == nil {
		return nil
	}
	v := *src
	return &v
}
func cowOwnCopyAddressFields(f CowAddressFields) CowAddressFields {
	f.Latitude = cowOwnCopyFloat(f.Latitude)
	f.Longitude = cowOwnCopyFloat(f.Longitude)
	return f
}

// BuildCowServiceContext is a domain boundary, NOT a service-job endpoint.
// It validates exactly one source for each dimension and rechecks CoW access;
// no temporary input is written to saved master tables as a side effect.
func (service *Service) BuildCowServiceContext(ctx context.Context, p Principal, input CowServiceContextInput) (CowServiceContextSnapshot, error) {
	if err := service.requireCowCustomer(ctx, service.database, p, false); err != nil {
		return CowServiceContextSnapshot{}, err
	}
	if (input.SavedVehicleID == nil) == (input.TemporaryVehicle == nil) || (input.SavedAddressID == nil) == (input.TemporaryAddress == nil) {
		return CowServiceContextSnapshot{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_service_context", Message: "Supply exactly one saved ID or temporary object for both vehicle and address."}
	}
	var snapshot CowServiceContextSnapshot
	if input.SavedVehicleID != nil {
		vehicle, err := service.customerVehicle(ctx, service.database, p, *input.SavedVehicleID)
		if err != nil {
			return CowServiceContextSnapshot{}, err
		}
		id := vehicle.ID
		snapshot.Vehicle = CowVehicleSnapshot{Source: "SAVED", SavedVehicleID: &id, VehicleNumber: vehicle.VehicleNumber, VehicleType: cowOwnCopyString(vehicle.Type), VehicleMake: cowOwnCopyString(vehicle.Make), VehicleModel: cowOwnCopyString(vehicle.Model)}
	} else {
		v := *input.TemporaryVehicle
		num, err := normalizeVehicleNumber(v.VehicleNumber)
		if err != nil {
			return CowServiceContextSnapshot{}, err
		}
		typ, err := normalizeVehicleOptional(v.VehicleType, 50, "vehicle_type")
		if err != nil {
			return CowServiceContextSnapshot{}, err
		}
		makeName, err := normalizeVehicleOptional(v.VehicleMake, 100, "vehicle_make")
		if err != nil {
			return CowServiceContextSnapshot{}, err
		}
		model, err := normalizeVehicleOptional(v.VehicleModel, 100, "vehicle_model")
		if err != nil {
			return CowServiceContextSnapshot{}, err
		}
		snapshot.Vehicle = CowVehicleSnapshot{Source: "TEMPORARY", VehicleNumber: num, VehicleType: cowOwnCopyString(typ), VehicleMake: cowOwnCopyString(makeName), VehicleModel: cowOwnCopyString(model)}
	}
	if input.SavedAddressID != nil {
		address, err := service.findCowAddress(ctx, service.database, p, *input.SavedAddressID)
		if err != nil {
			return CowServiceContextSnapshot{}, err
		}
		id := address.ID
		snapshot.Address = CowAddressSnapshot{Source: "SAVED", SavedAddressID: &id, CowAddressFields: cowOwnCopyAddressFields(address.CowAddressFields)}
	} else {
		fields := cowOwnCopyAddressFields(*input.TemporaryAddress)
		if err := normalizeCowAddress(&fields); err != nil {
			return CowServiceContextSnapshot{}, err
		}
		snapshot.Address = CowAddressSnapshot{Source: "TEMPORARY", CowAddressFields: fields}
	}
	snapshot.CapturedAt = service.now().UTC()
	return snapshot, nil
}
