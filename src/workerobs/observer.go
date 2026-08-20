// Package workerobs defines the small operational-observation socket used by
// long-running domain loops. It deliberately contains no platform policy.
package workerobs

import "context"

type Observer interface {
	Heartbeat(context.Context, string, string) error
	JobCompleted(context.Context, string, string) error
	MarkUnhealthy(context.Context, string, string) error
}
