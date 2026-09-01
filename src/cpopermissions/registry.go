// Package cpopermissions defines the source-controlled CPO capability catalog.
// Membership rows may grant or deny a key from this catalog, but cannot invent
// an authorization surface.
package cpopermissions

import (
	"sort"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/constants"
)

const (
	OrganizationRead       = "organization.read"
	OrganizationManage     = "organization.manage"
	StaffRead              = "staff.read"
	StaffManage            = "staff.manage"
	StaffPermissionsManage = "staff.permissions.manage"
	HubsRead               = "hubs.read"
	HubsManage             = "hubs.manage"
	ChargersRead           = "chargers.read"
	ChargersManage         = "chargers.manage"
	ChargersOperations     = "chargers.operations"
	TariffsRead            = "tariffs.read"
	TariffsManage          = "tariffs.manage"
	CustomersRead          = "customers.read"
	ChargingSessionsRead   = "charging_sessions.read"
	ChargingTracesRead     = "charging_traces.read"
	AnalyticsRead          = "analytics.read"
	SupportRead            = "support.read"
	SupportCreate          = "support.create"
	SupportReply           = "support.reply"
	SettingsRead           = "settings.read"
	SettingsManage         = "settings.manage"
)

type Definition struct {
	Key         string `json:"key"`
	Module      string `json:"module"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var catalog = []Definition{
	{OrganizationRead, "organization", "View organization", "View the CPO organization and subscription."},
	{OrganizationManage, "organization", "Manage organization", "Change the CPO administrative profile."},
	{StaffRead, "staff", "View staff", "View active and historical CPO staff memberships."},
	{StaffManage, "staff", "Manage staff", "Invite, update, suspend, activate, or revoke CPO staff."},
	{StaffPermissionsManage, "staff", "Manage staff permissions", "Change roles and explicit permission overrides within the caller's delegation authority."},
	{HubsRead, "hubs", "View hubs", "View CPO hubs and their assignments."},
	{HubsManage, "hubs", "Manage hubs", "Create or change CPO hubs, assignments, visibility, and GST linkage."},
	{ChargersRead, "chargers", "View chargers", "View CPO chargers, connectors, images, and status."},
	{ChargersManage, "chargers", "Manage chargers", "Create or change CPO chargers and their configuration."},
	{ChargersOperations, "chargers", "Operate chargers", "Access live operational charger and fleet evidence; command routes require this capability when introduced."},
	{TariffsRead, "tariffs", "View tariffs", "View tariffs, GST, user groups, and commercial configuration."},
	{TariffsManage, "tariffs", "Manage tariffs", "Change tariffs, GST, and customer-group commercial configuration."},
	{CustomersRead, "customers", "View customers", "View CPO-local customers, wallets, and customer transaction history."},
	{ChargingSessionsRead, "charging_sessions", "View charging sessions", "View CPO charging-session and charger-transaction projections."},
	{ChargingTracesRead, "charging_traces", "View charging traces", "View diagnostic CMS and HAL transaction evidence for a CPO charging session."},
	{AnalyticsRead, "analytics", "View analytics", "View CPO analytics and summarized operational reporting."},
	{SupportRead, "support", "View support", "View the CPO's support queue and ticket history."},
	{SupportCreate, "support", "Create support tickets", "Open a new support ticket for the CPO."},
	{SupportReply, "support", "Reply to support tickets", "Reply to an existing CPO support ticket."},
	{SettingsRead, "settings", "View settings", "View CPO settings and invoice branding."},
	{SettingsManage, "settings", "Manage settings", "Change CPO settings and invoice branding."},
}

var roleDefaults = map[constants.CPORole][]string{
	constants.CPORoleOwner: allKeys(),
	constants.CPORoleAdmin: allKeys(),
	constants.CPORoleOperator: {
		OrganizationRead, HubsRead, HubsManage, ChargersRead, ChargersManage,
		ChargersOperations, TariffsRead, CustomersRead, ChargingSessionsRead, ChargingTracesRead,
		AnalyticsRead, SupportRead, SupportCreate, SupportReply, SettingsRead,
	},
	constants.CPORoleViewer: {
		OrganizationRead, HubsRead, ChargersRead, TariffsRead, CustomersRead,
		ChargingSessionsRead, AnalyticsRead, SupportRead, SettingsRead,
	},
}

func allKeys() []string {
	keys := make([]string, 0, len(catalog))
	for _, definition := range catalog {
		keys = append(keys, definition.Key)
	}
	return keys
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

// RoleDefaults returns a copy so callers cannot mutate source-controlled role
// policy. Roles are explicit bundles; no role has wildcard evaluator behavior.
func RoleDefaults(role constants.CPORole) []string {
	result := append([]string(nil), roleDefaults[role]...)
	sort.Strings(result)
	return result
}

func RoleAllows(role constants.CPORole, key string) bool {
	for _, allowed := range roleDefaults[role] {
		if allowed == key {
			return true
		}
	}
	return false
}

// Effective applies the one canonical override ordering: DENY, then ALLOW,
// then the explicit role bundle. Inputs are copied and normalized by callers.
func Effective(role constants.CPORole, allow, deny []string) []string {
	effective := make(map[string]bool)
	for _, key := range roleDefaults[role] {
		effective[key] = true
	}
	denied := make(map[string]bool, len(deny))
	for _, key := range deny {
		if Known(key) {
			denied[key] = true
			delete(effective, key)
		}
	}
	for _, key := range allow {
		if Known(key) && !denied[key] {
			effective[key] = true
		}
	}
	result := make([]string, 0, len(effective))
	for key := range effective {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
