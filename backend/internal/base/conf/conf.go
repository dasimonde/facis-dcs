package conf

import "time"

func TransactionTimeout() time.Duration {
	return 5 * time.Minute
}

func OutboxProcessorTimeOut() time.Duration {
	return 1 * time.Second
}

func EventBusTopic() string {
	return "digital-contracting-service"
}

func GlobalAuditTrailName() string {
	return "GLOBAL_AUDIT_TRAIL"
}

func LoginAttemptsThresholdInDuration() int {
	return 5
}

func LoginLockoutDuration() time.Duration {
	return 15 * time.Minute
}
