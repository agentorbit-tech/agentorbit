package cron

// Advisory-lock IDs used to coordinate background workers across replicas.
// Each lock ID is unique to a worker and stable for the life of the codebase
// — postgres pg_try_advisory_xact_lock relies on the integer value, not on
// any naming convention. The 0xA0_00_00_xx prefix encodes "AgentOrbit cron"
// for ease of identification when poking at pg_locks.
const (
	LockHardDelete      int64 = 0xA0_00_00_01
	LockSessionClosure  int64 = 0xA0_00_00_02
	LockAlertEvaluation int64 = 0xA0_00_00_03
	LockRetentionPurge  int64 = 0xA0_00_00_04
)
