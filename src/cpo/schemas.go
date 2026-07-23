package cpo

import (
	"time"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
	"github.com/google/uuid"
)

type CreateRequest struct {
	Slug         string                   `json:"slug"`
	BusinessName string                   `json:"business_name"`
	CompanyType  constants.CPOCompanyType `json:"company_type"`
	GSTIN        *string                  `json:"gstin,omitempty"`
	Address      string                   `json:"address"`
	City         string                   `json:"city"`
	State        string                   `json:"state"`
	Pincode      string                   `json:"pincode"`
	Admin        InitialAdminRequest      `json:"admin"`
}

type InitialAdminRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
}

type SetAppIDRequest struct {
	AppID string `json:"app_id"`
}

type View struct {
	ID             uuid.UUID                `json:"id"`
	Slug           string                   `json:"slug"`
	BusinessName   string                   `json:"business_name"`
	CompanyType    constants.CPOCompanyType `json:"company_type"`
	GSTIN          *string                  `json:"gstin,omitempty"`
	Address        string                   `json:"address"`
	City           string                   `json:"city"`
	State          string                   `json:"state"`
	Pincode        string                   `json:"pincode"`
	Status         constants.CPOStatus      `json:"status"`
	AppID          string                   `json:"app_id"`
	AppIDMode      constants.CPOAppIDMode   `json:"app_id_mode"`
	AppIDUpdatedAt time.Time                `json:"app_id_updated_at"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
}

type InitialAdminView struct {
	UserID          uuid.UUID         `json:"user_id"`
	Email           string            `json:"email"`
	FullName        string            `json:"full_name"`
	Role            constants.CPORole `json:"role"`
	IdentityCreated bool              `json:"identity_created"`
}

type CreateResponse struct {
	CPO   View             `json:"cpo"`
	Admin InitialAdminView `json:"admin"`
}
