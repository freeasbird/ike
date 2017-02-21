package cli

import (
	"context"
	cxt "context"
	"net"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/msgboxio/ike"
	"github.com/msgboxio/ike/platform"
)

type IkeCmd struct {
	// map of initiator spi -> session
	sessions, initiators ike.Sessions
	conn                 ike.Conn
	cb                   ike.SessionCallback
}

func NewCmd(conn ike.Conn, cb ike.SessionCallback) *IkeCmd {
	return &IkeCmd{
		sessions:   ike.NewSessions(),
		initiators: ike.NewSessions(),
		conn:       conn,
		cb:         cb,
	}
}

func (i *IkeCmd) AddSa(sa *platform.SaParams) error {
	return i.cb.AddSa(nil, sa)
}
func (i *IkeCmd) RemoveSa(sa *platform.SaParams) error {
	return i.cb.RemoveSa(nil, sa)
}

// dont call cb.onError
func (i *IkeCmd) onError(session *ike.Session, err error) {
	// break before make
	if err == ike.ErrorRekeyRequired {
		session.Close(cxt.DeadlineExceeded)
	} else {
		session.Close(cxt.Canceled)
	}
}

// runs on main goroutine
func (i *IkeCmd) watchSession(spi uint64, session *ike.Session) {
	i.sessions.Add(spi, session)
	// wait for session to finish
	go func() {
		<-session.Done()
		i.sessions.Remove(spi)
		session.Logger.Infof("Removed IKE SA")
	}()
}

// newSession handles IKE_SA_INIT requests & replies
func (i *IkeCmd) newSession(msg *ike.Message, pconn ike.Conn, config *ike.Config, log *logrus.Logger) (*ike.Session, error) {
	spi := ike.SpiToInt64(msg.IkeHeader.SpiI)
	// check if this is a IKE_SA_INIT response
	session, found := i.initiators.Get(spi)
	if found {
		// TODO - check if we already have a connection to this host
		// close the initiator session if we do
		// check if incoming message is an acceptable Init Response
		if err := ike.CheckInitResponseForSession(session, msg); err != nil {
			if ce, ok := err.(ike.CookieError); ok {
				// let retransmission take care to sending init with cookie
				// session is always returned for CookieError
				session.SetCookie(ce.Cookie)
			} else {
				log.Warning("drop packet: ", err)
			}
			return session, err
		}
		// rewrite LocalAddr
		ike.ContextCallback(session).SetAddresses(msg.LocalAddr, msg.RemoteAddr)
		// remove from initiators map
		i.initiators.Remove(spi)
	} else {
		// is it a IKE_SA_INIT req ?
		if err := ike.CheckInitRequest(config, msg); err != nil {
			// handle errors that need reply: COOKIE or DH
			if reply := ike.InitErrorNeedsReply(msg, config, err); reply != nil {
				data, err := reply.Encode(nil, false, log)
				if err != nil {
					log.Errorf("error encoding init reply %+v", err)
				}
				pconn.WritePacket(data, msg.RemoteAddr)
			}
			return nil, err
		}
		// create and run session
		cb := &ike.SessionData{
			Conn:   i.conn,
			Local:  msg.LocalAddr,
			Remote: msg.RemoteAddr,
			Cb:     i.cb,
		}
		cxt := ike.WithCallback(context.Background(), cb)
		var err error
		session, err = ike.NewResponder(cxt, config, msg, log)
		if err != nil {
			return nil, err
		}
		go session.Run()
	}
	return session, nil
}

// runs on main goroutine
func (i *IkeCmd) processPacket(msg *ike.Message, config *ike.Config, log *logrus.Logger) {
	// convert spi to uint64 for map lookup
	spi := ike.SpiToInt64(msg.IkeHeader.SpiI)
	// check if a session exists
	session, found := i.sessions.Get(spi)
	if !found {
		var err error
		session, err = i.newSession(msg, i.conn, config, log)
		if err != nil {
			return
		}
		// host based selectors can be added directly since both addresses are available
		loc := ike.AddrToIp(msg.LocalAddr)
		rem := ike.AddrToIp(msg.RemoteAddr)
		if err := session.AddHostBasedSelectors(loc, rem); err != nil {
			log.Warningf("could not add selectors for %s=>%s", loc, rem)
			log.Warn(err)
		}
		i.watchSession(spi, session)
	}
	session.PostMessage(msg)
}

// RunInitiator starts & watches over on initiator session
func (i *IkeCmd) RunInitiator(remoteAddr net.Addr, config *ike.Config, log *logrus.Logger) {
	go func() {
		for { // restart conn
			cb := &ike.SessionData{
				Conn:   i.conn,
				Remote: remoteAddr,
				Cb:     i.cb,
			}
			withCb := ike.WithCallback(context.Background(), cb)
			initiator, err := ike.NewInitiator(withCb, config, log)
			if err != nil {
				log.Errorln("could not start Initiator: ", err)
				return
			}
			i.initiators.Add(ike.SpiToInt64(initiator.IkeSpiI), initiator)
			go initiator.Run()
			// wait for initiator to finish
			<-initiator.Done()
			// TODO - currently this is break before make
			if err = initiator.Err(); err == cxt.DeadlineExceeded {
				initiator.Logger.Info("ReKeying: ")
				continue
			} else if err == cxt.Canceled {
				break
			}
			time.Sleep(time.Second * 5)
		}
	}()
}

// ShutDown closes all active IKE sessions
func (i *IkeCmd) ShutDown(err error) {
	// shutdown sessions
	i.sessions.ForEach(func(session *ike.Session) {
		// rely on this to drain replies
		session.Close(err)
		// wait until client is done
		<-session.Done()
	})
}

// Run loops until there is a socket error
func (i *IkeCmd) Run(config *ike.Config, log *logrus.Logger) error {
	for {
		// this will return with error when there is a socket error
		msg, err := ike.ReadMessage(i.conn, log)
		if err != nil {
			return err
		}
		i.processPacket(msg, config, log)
	}
}
