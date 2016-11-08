package protocol

import (
	"encoding/hex"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/msgboxio/packets"
	"github.com/pkg/errors"
)

func DecodeIkeHeader(b []byte, log *logrus.Logger) (h *IkeHeader, err error) {
	h = &IkeHeader{}
	if len(b) < IKE_HEADER_LEN {
		return nil, errors.Wrap(ERR_INVALID_SYNTAX, fmt.Sprintf("Packet Too short : %d", len(b)))
	}
	if len(b) > MAX_IKE_MESSAGE_LEN {
		return nil, errors.Wrap(ERR_INVALID_SYNTAX, fmt.Sprintf("Packet Too large : %d", len(b)))
	}
	h.SpiI = append([]byte{}, b[:8]...)
	h.SpiR = append([]byte{}, b[8:16]...)
	pt, _ := packets.ReadB8(b, 16)
	h.NextPayload = PayloadType(pt)
	ver, _ := packets.ReadB8(b, 16+1)
	h.MajorVersion = ver >> 4
	h.MinorVersion = ver & 0x0f
	et, _ := packets.ReadB8(b, 16+2)
	h.ExchangeType = IkeExchangeType(et)
	flags, _ := packets.ReadB8(b, 16+3)
	h.Flags = IkeFlags(flags)
	h.MsgId, _ = packets.ReadB32(b, 16+4)
	h.MsgLength, _ = packets.ReadB32(b, 16+8)
	if h.MsgLength < IKE_HEADER_LEN {
		return nil, errors.Wrap(ERR_INVALID_SYNTAX, fmt.Sprintf("Bad Message Length in header : %d", h.MsgLength))
	}
	if h.MsgLength > MAX_IKE_MESSAGE_LEN {
		return nil, errors.Wrap(ERR_INVALID_SYNTAX, fmt.Sprintf("Bad Message Length in header : %d", h.MsgLength))
	}
	log.Debugf("Ike Header: %+v from \n%s", *h, hex.Dump(b[:IKE_HEADER_LEN]))
	return
}

func (h *IkeHeader) Encode(log *logrus.Logger) (b []byte) {
	b = make([]byte, IKE_HEADER_LEN)
	copy(b, h.SpiI[:])
	copy(b[8:], h.SpiR[:])
	packets.WriteB8(b, 16, uint8(h.NextPayload))
	packets.WriteB8(b, 17, h.MajorVersion<<4|h.MinorVersion)
	packets.WriteB8(b, 18, uint8(h.ExchangeType))
	packets.WriteB8(b, 19, uint8(h.Flags))
	packets.WriteB32(b, 20, h.MsgId)
	packets.WriteB32(b, 24, h.MsgLength)
	log.Debugf("Ike Header: %+v to \n%s", *h, hex.Dump(b))
	return
}
