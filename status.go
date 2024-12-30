package chkbit

type Status string

const (
	StatusPanic         Status = "EXC"
	StatusErrorIdx      Status = "ERX"
	StatusErrorDamage   Status = "DMG"
	StatusUpdateWarnOld Status = "old"
	StatusUpdate        Status = "upd"
	StatusNew           Status = "new"
	StatusOK            Status = "ok "
	StatusIgnore        Status = "ign"
	StatusMissing       Status = "del"
	StatusInfo          Status = "msg"

	// internal
	StatusUpdateIndex Status = "xup"
)

func (s Status) String() string {
	return (string)(s)
}

func (s Status) IsErrorOrWarning() bool {
	return s == StatusPanic || s == StatusErrorDamage || s == StatusErrorIdx || s == StatusUpdateWarnOld
}

func (s Status) IsVerbose() bool {
	return s == StatusOK || s == StatusIgnore
}

type LogEvent struct {
	Stat    Status
	Message string
}

type PerfEvent struct {
	NumFiles int64
	NumBytes int64
}
