package chkbit

type Status string

const (
	STATUS_PANIC       Status = "EXC"
	STATUS_ERR_IDX     Status = "ERX"
	STATUS_ERR_DMG     Status = "DMG"
	STATUS_UP_WARN_OLD Status = "old"
	STATUS_UPDATE      Status = "upd"
	STATUS_NEW         Status = "new"
	STATUS_OK          Status = "ok "
	STATUS_IGNORE      Status = "ign"
	STATUS_MISSING     Status = "del"
	STATUS_INFO        Status = "msg"

	// internal
	STATUS_UPDATE_INDEX Status = "xup"
)

func (s Status) String() string {
	return (string)(s)
}

func (s Status) IsErrorOrWarning() bool {
	return s == STATUS_PANIC || s == STATUS_ERR_DMG || s == STATUS_ERR_IDX || s == STATUS_UP_WARN_OLD
}

func (s Status) IsVerbose() bool {
	return s == STATUS_OK || s == STATUS_IGNORE
}

type LogEvent struct {
	Stat    Status
	Message string
}

type PerfEvent struct {
	NumFiles int64
	NumBytes int64
}
