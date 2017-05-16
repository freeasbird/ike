package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/msgboxio/ike"
	"github.com/msgboxio/ike/platform"
	"github.com/msgboxio/ike/protocol"
	"github.com/pkg/errors"
)

func waitForSignal(cancel context.CancelFunc, logger log.Logger) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	sig := <-c
	// sig is a ^C, handle it
	cancel()
	level.Warn(logger).Log("signal", sig.String())
}

var localPskID = &ike.PskIdentities{
	Primary: "ak@msgbox.io",
	Ids:     map[string][]byte{"ak@msgbox.io": []byte("foo")},
}

var remotePskID = &ike.PskIdentities{
	Primary: "ak@msgbox.io",
	Ids:     map[string][]byte{"ak@msgbox.io": []byte("foo")},
}

var isDebug bool

func loadConfig() (config *ike.Config, localString string, remoteString string, err error) {
	flag.StringVar(&localString, "local", "0.0.0.0:4500", "address to bind to")
	flag.StringVar(&remoteString, "remote", "", "address to connect to")

	var localTunnel, remoteTunnel string
	flag.StringVar(&localTunnel, "localnet", "", "local network")
	flag.StringVar(&remoteTunnel, "remotenet", "", "remote network")

	var caFile, certFile, keyFile, peerID string
	flag.StringVar(&caFile, "ca", "", "PEM encoded ca certificate")
	flag.StringVar(&certFile, "cert", "", "PEM encoded peer certificate")
	flag.StringVar(&keyFile, "key", "", "PEM encoded peer key")
	flag.StringVar(&peerID, "peerid", "", "Peer ID")

	var useESN bool
	flag.BoolVar(&useESN, "esn", useESN, "use ESN")

	flag.BoolVar(&isDebug, "debug", isDebug, "debug logs")
	flag.Parse()

	config = ike.DefaultConfig()

	// crypto keys & names
	if caFile != "" && peerID != "" {
		roots, _err := ike.LoadRoot(caFile)
		err = errors.Wrapf(_err, "loading %s", caFile)
		if err != nil {
			return
		}
		config.RemoteID = &ike.CertIdentity{
			Roots: roots,
			Name:  peerID,
		}
	}
	if certFile != "" && keyFile != "" {
		certs, _err := ike.LoadCerts(certFile)
		err = errors.Wrapf(_err, "loading %s", certFile)
		if err != nil {
			return
		}

		key, _err := ike.LoadKey(keyFile)
		err = errors.Wrapf(_err, "loading %s", keyFile)
		if err != nil {
			return
		}
		config.LocalID = &ike.CertIdentity{
			Certificate: certs[0],
			PrivateKey:  key,
		}
	}

	if config.RemoteID == nil || config.LocalID == nil {
		config.RemoteID = remotePskID
		config.LocalID = localPskID
	}

	if localTunnel == "" && remoteTunnel == "" {
		config.IsTransportMode = true
	} else {
		_, localnet, _err := net.ParseCIDR(localTunnel)
		err = _err
		if err != nil {
			return
		}
		_, remotenet, _err := net.ParseCIDR(remoteTunnel)
		err = _err
		if err != nil {
			return
		}
		isInitiator := true
		if remoteString == "" {
			isInitiator = false
		}
		err = config.AddNetworkSelectors(localnet, remotenet, isInitiator)
	}
	if useESN {
		config.ProposalEsp.GetType(protocol.TRANSFORM_TYPE_ESN).TransformId = uint16(protocol.ESN)
	}
	return
}

func main() {
	config, localString, remoteString, err := loadConfig()
	if err != nil {
		fmt.Printf("Error: %+v\n", err)
		panic(err)
	}

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	if isDebug {
		logger = level.NewFilter(logger, level.AllowDebug())
		// crypto.DebugCrypto = true
	}
	logger = log.With(logger, "ts", log.DefaultTimestamp, "caller", log.DefaultCaller)

	cxt, cancel := context.WithCancel(context.Background())
	go waitForSignal(cancel, logger)

	ifs, _ := net.InterfaceAddrs()
	logger.Log("interfaces", fmt.Sprintf("%v", ifs))
	// this should load the xfrm modules
	// requires root
	cb := func(msg interface{}) {
		logger.Log("xfrm:", spew.Sprintf("%#v", msg))
	}
	platform.ListenForEvents(cxt, cb, logger)

	pconn, err := ike.Listen("udp", localString, logger)
	if err != nil {
		panic(fmt.Sprintf("Listen: %+v", err))
	}
	// requires root
	if err := platform.SetSocketBypas(pconn.Inner()); err != nil {
		panic(fmt.Sprintf("Bypass: %+v", err))
	}

	cmd := ike.NewCmd(pconn, &ike.SessionCallback{
		InstallPolicy: func(session *ike.Session, pol *protocol.PolicyParams) error {
			return platform.InstallPolicy(pol, logger, session.IsInitiator())
		},
		RemovePolicy: func(session *ike.Session, pol *protocol.PolicyParams) error {
			return platform.RemovePolicy(pol, logger, session.IsInitiator())
		},
		InstallChildSa: func(session *ike.Session, sa *platform.SaParams) error {
			return platform.InstallChildSa(sa, logger)
		},
		RemoveChildSa: func(session *ike.Session, sa *platform.SaParams) error {
			return platform.RemoveChildSa(sa, logger)
		},
	})

	if remoteString != "" {
		remoteAddr, err := net.ResolveUDPAddr("udp", remoteString)
		if err != nil {
			panic(err)
		}
		cmd.RunInitiator(remoteAddr, config, logger)
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	closing := false
	go func() {
		// wait for app shutdown
		<-cxt.Done()
		closing = true
		cmd.ShutDown(cxt.Err())
		// this will cause cmd.Run to return
		pconn.Close()
		wg.Done()
	}()

	// this will return when there is a socket error
	err = cmd.Run(config, logger)
	if !closing {
		// ignore when caused by the close call above
		logger.Log("Error", err)
		if isDebug {
			fmt.Printf("Stack: %+v\n", err)
		}
		cancel()
	}
	// wait for remaining sessions to shutdown
	wg.Wait()
	fmt.Printf("shutdown: %v\n", cxt.Err())
}
