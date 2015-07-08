package state

import (
	"msgbox.io/context"
	"msgbox.io/log"
)

type IkeEventId int

const (
	ACQUIRE IkeEventId = iota + 1
	CONNECT
	REAUTH

	// notifications
	N_COOKIE
	N_INVALID_KE
	N_NO_PROPOSAL_CHOSEN

	// messages
	IKE_SA_INIT
	IKE_AUTH
	DELETE_IKE_SA
	IKE_REKEY // CREATE_CHILD_SA

	// successes
	IKE_SA_INIT_SUCCESS
	IKE_AUTH_SUCCESS
	DELETE_IKE_SA_SUCCESS

	// errors
	INVALID_KE

	// internal state machine messages
	MSG_IKE_REKEY
	MSG_IKE_DPD
	MSG_IKE_CRL_UPDATE
	MSG_IKE_REAUTH
	MSG_IKE_TERMINATE

	// requests
	IKE_SA_DELETE_REQUEST

	// timeouts
	IKE_TIMEOUT

	StateEntry
	StateExit
)

type IkeEvent struct {
	Id      IkeEventId
	Message interface{}
}

type IkeSaState int

const (
	SMI_INIT IkeSaState = iota + 1
	SMI_INIT_WAIT
	SMI_AUTH_WAIT
	SMI_AUTH_PEER
	SMI_EAP
	SMI_INSTALLCSA_DL
	SMI_INSTALLCSA

	SMR_INIT
	SMR_AUTH
	SMR_AUTH_FINALIZE
	SMR_AUTH_RESPONSE_ID
	SMR_AUTH_RESPONSE
	SMR_EAP_INITATOR_REQUEST
	SMR_EAP_AAA_REQUEST
	SMR_AUTH_DL_PEER
	SMR_CFG_WAIT

	SM_MATURE
	SM_REKEY
	SM_CRL_UPDATE
	SM_REAUTH

	SM_TERMINATE
	SM_DYING
	SM_DEAD

	// child sa states
	// SA_INIT
	// SA_CREATE
	// SA_CREATE_WAIT
	// SA_MATURE
	// SA_REKEY
	// SA_REKEY_WAIT
	// SA_REMOVE
	// SA_DEAD
)

type FsmHandler interface {
	SendIkeSaInit()
	SendIkeAuth()
	SendSaRekey()
	SendSaDeleteRequest()
	SendSaDeleteResponse()

	HandleSaInit(interface{})
	HandleSaAuth(interface{})
	HandleSaRekey(interface{})

	InstallChildSa()
	RemoveSa()

	HandleSaDead()

	DownloadCrl()
}

type StateFunc func(*Fsm, IkeEvent)

type Fsm struct {
	FsmHandler
	context.Context

	events chan IkeEvent

	StateFunc
	State IkeSaState
}

func MakeFsm(h FsmHandler, initial StateFunc, parent context.Context) (s *Fsm) {
	s = &Fsm{
		FsmHandler: h,
		Context:    parent,
		events:     make(chan IkeEvent, 10),
		StateFunc:  Idle,
	}
	// go to initial state
	s.stateChange(initial)
	go s.run()
	return
}

func (s *Fsm) PostEvent(evt IkeEvent) {
	select {
	case <-s.Done(): // will return immediately if closed
		break
	default:
		// log.V(2).Infof("Post: Event %s, in State %s", evt.Id, s.State)
		s.events <- evt
	}
}

func (s *Fsm) runEvent(evt IkeEvent) {
	select {
	case <-s.Done(): // will return immediately if closed
		break
	default:
		log.V(1).Infof("Run: Event %s, in State %s", evt.Id, s.State)
		s.StateFunc(s, evt)
	}
}

func (s *Fsm) run() {
Done:
	for {
		select {
		case <-s.Done():
			break Done
		case ev := <-s.events:
			s.runEvent(ev)
		}
	}

	close(s.events)

	return
}

func (s *Fsm) stateChange(fn StateFunc) {
	prev := s.State
	// execute exit event synchronously
	s.StateFunc(s, IkeEvent{Id: StateExit})
	// change state
	s.StateFunc = fn
	// execute new state entry event
	s.StateFunc(s, IkeEvent{Id: StateEntry})
	log.V(1).Infof("Change: Previous %s, Current %s", prev, s.State)
}

func Idle(*Fsm, IkeEvent) {
}

// initial state:
func SmiInit(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SMI_INIT
	case ACQUIRE, CONNECT, REAUTH:
		// init cipher suite from config
		// create tkm
		// if nat-t is enabled, calculate hash of ips
		// crate IKE_SA_INIT and send
		// change to SMI_AUTH
		s.stateChange(SmiInitWait)
	case StateExit:
	}
	return
}

func SmiInitWait(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SMI_INIT_WAIT
		s.SendIkeSaInit()
	case N_INVALID_KE, N_COOKIE:
		// recerate IKE_SA_INIT and send
	case IKE_SA_INIT:
		s.HandleSaInit(evt.Message)
	case IKE_SA_INIT_SUCCESS:
		s.stateChange(SmiAuthWait)
	case N_NO_PROPOSAL_CHOSEN:
		s.stateChange(SmDead)
	case IKE_TIMEOUT:
		s.stateChange(SmDead)
	case INVALID_KE:
		s.stateChange(SmTerminate)
	}
	return
}

func SmiAuthWait(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SMI_AUTH_WAIT
		s.SendIkeAuth()
	case IKE_AUTH:
		s.HandleSaAuth(evt.Message)
	case IKE_AUTH_SUCCESS:
		s.stateChange(SmMature)
	default:
	}
	return
}

// initial
func SmrInit(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SMR_INIT
	case IKE_SA_INIT:
		s.HandleSaInit(evt.Message)
	case IKE_SA_INIT_SUCCESS:
		s.SendIkeSaInit()
		s.stateChange(SmrAuth)
	}
}

// wait for AUTH
func SmrAuth(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SMR_AUTH
	case IKE_AUTH:
		s.HandleSaAuth(evt.Message)
	case IKE_AUTH_SUCCESS:
		s.SendIkeAuth()
		s.stateChange(SmMature)
	}
}

func SmMature(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SM_MATURE
		s.InstallChildSa()
	case MSG_IKE_REKEY:
		s.SendSaRekey()
		s.stateChange(SmRekey)
	case MSG_IKE_TERMINATE:
		s.stateChange(SmTerminate)
	case IKE_REKEY:
		s.HandleSaRekey(evt.Message)
	}
	return
}

func SmRekey(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SM_REKEY
	case IKE_REKEY:
		s.HandleSaRekey(evt.Message)
	}
	return
}

func SmDead(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SM_DEAD
		s.HandleSaDead()
	}
	return
}

// remove SA & send delete reqest
func SmTerminate(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SM_TERMINATE
		s.RemoveSa()
	case MSG_IKE_TERMINATE:
		s.SendSaDeleteRequest()
	case DELETE_IKE_SA:
		s.stateChange(SmDead)
	case IKE_SA_DELETE_REQUEST:
		s.stateChange(SmDead)
	case IKE_TIMEOUT:
		s.stateChange(SmDead)
	}
	return
}

//
func SmDying(s *Fsm, evt IkeEvent) {
	switch evt.Id {
	case StateEntry:
		s.State = SM_DYING
		s.SendSaDeleteResponse()
	case MSG_IKE_TERMINATE:
		s.stateChange(SmDead)
	}
	return
}