// Package cpopermissions defines the stable, source-controlled capability
// catalog used by CPO membership authorization.  Keys are deliberately not
// tenant-configurable: a database row may grant or deny a known capability but
// cannot create a new authorization surface.
package cpopermissions

import "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"

const (
	OrganizationRead   = "organization.read"
	OrganizationManage = "organization.manage"
	NetworkRead        = "network.read"
	NetworkManage      = "network.manage"
	CommercialRead     = "commercial.read"
	CommercialManage   = "commercial.manage"
	OperationsRead     = "operations.read"
	OperationsManage   = "operations.manage"
	CustomersRead      = "customers.read"
	StaffRead          = "staff.read"
	StaffManage        = "staff.manage"
	SupportRead        = "support.read"
	SupportManage      = "support.manage"
)

type Definition struct {
	Key         string `json:"key"`
	Module      string `json:"module"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var catalog = []Definition{
	{OrganizationRead, "organization", "View organization", "View the CPO organization and subscription."},
	{OrganizationManage, "organization", "Manage organization", "Change CPO organization settings."},
	{NetworkRead, "network", "View network", "View hubs, chargers, connectors, and status."},
	{NetworkManage, "network", "Manage network", "Create or change hubs, chargers, connectors, and status."},
	{CommercialRead, "commercial", "View commercial data", "View tariffs, GST, wallets, and charging transactions."},
	{CommercialManage, "commercial", "Manage commercial data", "Change tariffs, GST, user groups, and commercial settings."},
	{OperationsRead, "operations", "View operations", "View fleet operations, sessions, and operational events."},
	{OperationsManage, "operations", "Manage operations", "Issue authorized operational commands."},
	{CustomersRead, "customers", "View customers", "View CPO-local customer information and usage."},
	{StaffRead, "staff", "View staff", "View CPO staff memberships and effective permissions."},
	{StaffManage, "staff", "Manage staff", "Invite, update, suspend, revoke, and configure CPO staff."},
	{SupportRead, "support", "View support", "View CPO support tickets."},
	{SupportManage, "support", "Manage support", "Create and reply to CPO support tickets."},
}

func Catalog() []Definition {
	result := make([]Definition, len(catalog))
	copy(result, catalog)
	return result
}

func Known(key string) bool {
	for _, definition := range catalog {
		if definition.Key == key {
			return true
		}
	}
	return false
}

func RoleAllows(role constants.CPORole, key string) bool {
	switch role {
	case constants.CPORoleOwner, constants.CPORoleAdmin:
		return Known(key)
	case constants.CPORoleOperator:
		switch key {
		case OrganizationRead, NetworkRead, NetworkManage, CommercialRead,
			OperationsRead, OperationsManage, CustomersRead, SupportRead, SupportManage:
			return true
		}
	case constants.CPORoleViewer:
		switch key {
		case OrganizationRead, NetworkRead, CommercialRead, OperationsRead, CustomersRead, SupportRead:
			return true
		}
	}
	return false
}
