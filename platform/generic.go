//+build !linux

package platform

import (
	"context"
	"net"

	"github.com/go-kit/kit/log"
)

func InstallChildSa(*SaParams, log.Logger) error {
	return nil
}

func RemoveChildSa(*SaParams, log.Logger) error {
	return nil
}

func SetSocketBypas(conn net.Conn, family uint16) (err error) {
	return
}

func ListenForEvents(context.Context, func(interface{}), log.Logger) {
	return
}
