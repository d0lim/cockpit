package lifecycle

type WaitReason string

const (
	WaitNone              WaitReason = ""
	WaitActiveRun         WaitReason = "active-run"
	WaitPoolAllKept       WaitReason = "pool-all-kept"
	WaitPoolOver          WaitReason = "pool-over"
	WaitInProgress        WaitReason = "in-progress"
	WaitDockerUnavailable WaitReason = "docker-unavailable"
	WaitNoPortBlock       WaitReason = "no-port-block"
)

var WaitLabels = map[WaitReason]string{
	WaitActiveRun:         "an active run is in progress",
	WaitPoolAllKept:       "every environment in the kept pool has keep on",
	WaitPoolOver:          "waiting for the kept pool overflow to clear",
	WaitInProgress:        "waiting for an operation in progress",
	WaitDockerUnavailable: "Docker does not respond",
	WaitNoPortBlock:       "no port block is free",
}

type StopReason string

const (
	StopNone   StopReason = ""
	StopPerson StopReason = "person"
	StopCap    StopReason = "cap"
	StopDone   StopReason = "done"
)

var StopLabels = map[StopReason]string{
	StopPerson: "stopped by a person",
	StopCap:    "stopped because the running cap was exceeded",
	StopDone:   "stopped automatically after the run finished",
}

type DestroyReason string

const (
	DestroyNone   DestroyReason = ""
	DestroyPerson DestroyReason = "person"
	DestroyPool   DestroyReason = "pool"
)

var DestroyLabels = map[DestroyReason]string{
	DestroyPerson: "destroyed by a person",
	DestroyPool:   "destroyed because the kept pool overflowed",
}
