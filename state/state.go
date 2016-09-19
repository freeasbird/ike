package state

const (
	STATE_IDLE State = iota
	STATE_START

	STATE_INIT
	STATE_AUTH
	STATE_MATURE

	STATE_FINISHED
)

const (
	SUCCESS Event = iota
	FAIL          // unrecoverable failure

	MSG_INIT
	MSG_AUTH
	MSG_CHILD_SA
	MSG_DELETE_IKE_SA // a notification really

	SMI_START
	REKEY_START

	TIMEOUT
	INIT_FAIL
	AUTH_FAIL

	DELETE_IKE_SA

	FINISHED // internal event
	ENTRY_EVENT
)

type FsmHandler interface {
	// actions
	SendInit() StateEvent
	SendAuth() StateEvent
	StartRetryTimeout() StateEvent
	InstallSa() StateEvent
	RemoveSa() StateEvent
	Finished() StateEvent

	// checks
	HandleIkeSaInit(interface{}) StateEvent
	HandleIkeAuth(interface{}) StateEvent
	CheckSa(interface{}) StateEvent
	HandleCreateChildSa(interface{}) StateEvent

	CheckError(interface{}) StateEvent
}
