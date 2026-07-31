// Package conf holds timing configuration specific to the contract workflow
// engine (currently just the expiry cron poll interval; see cronjobs.go).
package conf

import "time"

func ExpirationCronJobTimeOut() time.Duration {
	return 1 * time.Minute
}
