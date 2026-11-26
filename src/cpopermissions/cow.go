package cpopermissions

// These are the explicit Charger on Wheels administrative capabilities from
// the Sept 24 normative authorization annex, prefixed to avoid collision with
// the existing EV-CMS capability namespace. The catalog defines permissions;
// it does not claim that all corresponding CoW feature routes exist yet.
const (
	CowUsersRead            = "cow.users.read"
	CowUsersManage          = "cow.users.manage"
	CowDriversRead          = "cow.drivers.read"
	CowDriversReview        = "cow.drivers.review"
	CowDriversManage        = "cow.drivers.manage"
	CowDriversSuspend       = "cow.drivers.suspend"
	CowFleetRead            = "cow.fleet.read"
	CowFleetManage          = "cow.fleet.manage"
	CowFleetMaintenance     = "cow.fleet.maintenance"
	CowFleetAssign          = "cow.fleet.assign"
	CowSchedulesRead        = "cow.schedules.read"
	CowSchedulesManage      = "cow.schedules.manage"
	CowDispatchRead         = "cow.dispatch.read"
	CowDispatchManage       = "cow.dispatch.manage"
	CowDispatchOverride     = "cow.dispatch.override"
	CowJobsRead             = "cow.jobs.read"
	CowJobsManage           = "cow.jobs.manage"
	CowJobsCancel           = "cow.jobs.cancel"
	CowJobsReassign         = "cow.jobs.reassign"
	CowChargingRead         = "cow.charging.read"
	CowChargingCommand      = "cow.charging.command"
	CowChargingReconcile    = "cow.charging.reconcile"
	CowChargingTraceRead    = "cow.charging.trace.read"
	CowPricingRead          = "cow.pricing.read"
	CowPricingManage        = "cow.pricing.manage"
	CowPaymentsRead         = "cow.payments.read"
	CowPaymentsManage       = "cow.payments.manage"
	CowRefundsManage        = "cow.refunds.manage"
	CowInvoicesRead         = "cow.invoices.read"
	CowInvoicesManage       = "cow.invoices.manage"
	CowCompensationRead     = "cow.compensation.read"
	CowCompensationManage   = "cow.compensation.manage"
	CowCompensationAdjust   = "cow.compensation.adjust"
	CowRatingsRead          = "cow.ratings.read"
	CowSupportRead          = "cow.support.read"
	CowSupportReply         = "cow.support.reply"
	CowSupportManage        = "cow.support.manage"
	CowIncidentsRead        = "cow.incidents.read"
	CowIncidentsManage      = "cow.incidents.manage"
	CowNotificationsRead    = "cow.notifications.read"
	CowNotificationsManage  = "cow.notifications.manage"
	CowExportsManage        = "cow.exports.manage"
	CowSystemSettingsRead   = "cow.system.settings.read"
	CowSystemSettingsManage = "cow.system.settings.manage"
	CowAuditRead            = "cow.audit.read"
)

var cowCatalog = []Definition{
	{CowUsersRead, "cow", "View CoW users", "CoW permission for users read; enforced by its owning feature endpoint when implemented."},
	{CowUsersManage, "cow", "Manage CoW users", "CoW permission for users manage; enforced by its owning feature endpoint when implemented."},
	{CowDriversRead, "cow", "View CoW drivers", "CoW permission for drivers read; enforced by its owning feature endpoint when implemented."},
	{CowDriversReview, "cow", "Review CoW drivers", "CoW permission for drivers review; enforced by its owning feature endpoint when implemented."},
	{CowDriversManage, "cow", "Manage CoW drivers", "CoW permission for drivers manage; enforced by its owning feature endpoint when implemented."},
	{CowDriversSuspend, "cow", "Suspend CoW drivers", "CoW permission for drivers suspend; enforced by its owning feature endpoint when implemented."},
	{CowFleetRead, "cow", "View CoW fleet", "CoW permission for fleet read; enforced by its owning feature endpoint when implemented."},
	{CowFleetManage, "cow", "Manage CoW fleet", "CoW permission for fleet manage; enforced by its owning feature endpoint when implemented."},
	{CowFleetMaintenance, "cow", "Maintain CoW fleet", "CoW permission for fleet maintenance; enforced by its owning feature endpoint when implemented."},
	{CowFleetAssign, "cow", "Assign CoW fleet", "CoW permission for fleet assign; enforced by its owning feature endpoint when implemented."},
	{CowSchedulesRead, "cow", "View CoW schedules", "CoW permission for schedules read; enforced by its owning feature endpoint when implemented."},
	{CowSchedulesManage, "cow", "Manage CoW schedules", "CoW permission for schedules manage; enforced by its owning feature endpoint when implemented."},
	{CowDispatchRead, "cow", "View CoW dispatch", "CoW permission for dispatch read; enforced by its owning feature endpoint when implemented."},
	{CowDispatchManage, "cow", "Manage CoW dispatch", "CoW permission for dispatch manage; enforced by its owning feature endpoint when implemented."},
	{CowDispatchOverride, "cow", "Override CoW dispatch", "CoW permission for dispatch override; enforced by its owning feature endpoint when implemented."},
	{CowJobsRead, "cow", "View CoW jobs", "CoW permission for jobs read; enforced by its owning feature endpoint when implemented."},
	{CowJobsManage, "cow", "Manage CoW jobs", "CoW permission for jobs manage; enforced by its owning feature endpoint when implemented."},
	{CowJobsCancel, "cow", "Cancel CoW jobs", "CoW permission for jobs cancel; enforced by its owning feature endpoint when implemented."},
	{CowJobsReassign, "cow", "Reassign CoW jobs", "CoW permission for jobs reassign; enforced by its owning feature endpoint when implemented."},
	{CowChargingRead, "cow", "View CoW charging", "CoW permission for charging read; enforced by its owning feature endpoint when implemented."},
	{CowChargingCommand, "cow", "Command CoW charging", "CoW permission for charging command; enforced by its owning feature endpoint when implemented."},
	{CowChargingReconcile, "cow", "Reconcile CoW charging", "CoW permission for charging reconcile; enforced by its owning feature endpoint when implemented."},
	{CowChargingTraceRead, "cow", "View CoW charging trace", "CoW permission for charging trace read; enforced by its owning feature endpoint when implemented."},
	{CowPricingRead, "cow", "View CoW pricing", "CoW permission for pricing read; enforced by its owning feature endpoint when implemented."},
	{CowPricingManage, "cow", "Manage CoW pricing", "CoW permission for pricing manage; enforced by its owning feature endpoint when implemented."},
	{CowPaymentsRead, "cow", "View CoW payments", "CoW permission for payments read; enforced by its owning feature endpoint when implemented."},
	{CowPaymentsManage, "cow", "Manage CoW payments", "CoW permission for payments manage; enforced by its owning feature endpoint when implemented."},
	{CowRefundsManage, "cow", "Manage CoW refunds", "CoW permission for refunds manage; enforced by its owning feature endpoint when implemented."},
	{CowInvoicesRead, "cow", "View CoW invoices", "CoW permission for invoices read; enforced by its owning feature endpoint when implemented."},
	{CowInvoicesManage, "cow", "Manage CoW invoices", "CoW permission for invoices manage; enforced by its owning feature endpoint when implemented."},
	{CowCompensationRead, "cow", "View CoW compensation", "CoW permission for compensation read; enforced by its owning feature endpoint when implemented."},
	{CowCompensationManage, "cow", "Manage CoW compensation", "CoW permission for compensation manage; enforced by its owning feature endpoint when implemented."},
	{CowCompensationAdjust, "cow", "Adjust CoW compensation", "CoW permission for compensation adjust; enforced by its owning feature endpoint when implemented."},
	{CowRatingsRead, "cow", "View CoW ratings", "CoW permission for ratings read; enforced by its owning feature endpoint when implemented."},
	{CowSupportRead, "cow", "View CoW support", "CoW permission for support read; enforced by its owning feature endpoint when implemented."},
	{CowSupportReply, "cow", "Reply CoW support", "CoW permission for support reply; enforced by its owning feature endpoint when implemented."},
	{CowSupportManage, "cow", "Manage CoW support", "CoW permission for support manage; enforced by its owning feature endpoint when implemented."},
	{CowIncidentsRead, "cow", "View CoW incidents", "CoW permission for incidents read; enforced by its owning feature endpoint when implemented."},
	{CowIncidentsManage, "cow", "Manage CoW incidents", "CoW permission for incidents manage; enforced by its owning feature endpoint when implemented."},
	{CowNotificationsRead, "cow", "View CoW notifications", "CoW permission for notifications read; enforced by its owning feature endpoint when implemented."},
	{CowNotificationsManage, "cow", "Manage CoW notifications", "CoW permission for notifications manage; enforced by its owning feature endpoint when implemented."},
	{CowExportsManage, "cow", "Manage CoW exports", "CoW permission for exports manage; enforced by its owning feature endpoint when implemented."},
	{CowSystemSettingsRead, "cow", "View CoW system settings", "CoW permission for system settings read; enforced by its owning feature endpoint when implemented."},
	{CowSystemSettingsManage, "cow", "Manage CoW system settings", "CoW permission for system settings manage; enforced by its owning feature endpoint when implemented."},
	{CowAuditRead, "cow", "View CoW audit", "CoW permission for audit read; enforced by its owning feature endpoint when implemented."},
}
