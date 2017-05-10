package ike

import (
	"net"
	"time"

	"github.com/msgboxio/ike/crypto"
	"github.com/msgboxio/ike/protocol"
	"github.com/pkg/errors"
)

type Config struct {
	ProposalIke, ProposalEsp protocol.Transforms

	LocalID, RemoteID Identity
	AuthMethod        protocol.AuthMethod

	TsI, TsR             []*protocol.Selector
	IsTransportMode      bool
	ThrottleInitRequests bool
	Lifetime             time.Duration
}

// StrongSwan recommendations for cipher suite
// aes128-sha256-modp3072 (AES-CBC-128, SHA-256 as HMAC and DH key exchange with 3072 bit key length)
// suite B
// aes128gcm16-prfsha256-ecp256 (AES-GCM-128 AEAD, SHA-256 as PRF and ECDH key exchange with 256 bit key length)
// aes256gcm16-prfsha384-ecp384 (AES-GCM-256 AEAD, SHA-384 as PRF and ECDH key exchange with 384 bit key length)

func DefaultConfig() *Config {
	return &Config{
		ProposalIke: crypto.Chacha20poly1305Prfsha256Ecp256,
		// ProposalIke: crypto.Aes128gcm16Prfsha256Ecp256,
		// ProposalIke: crypto.Aes256gcm16Prfsha384Ecp384,

		ProposalEsp: crypto.Chacha20poly1305,
		// ProposalEsp: crypto.Aes128Sha256,
		// ProposalEsp: crypto.Aes256gcm16,

		AuthMethod: protocol.AUTH_DIGITAL_SIGNATURE,
		// ThrottleInitRequests: true,
		Lifetime: time.Hour,
	}
}

// CheckProposals checks if incoming proposals include our configuration
func (cfg *Config) CheckProposals(prot protocol.ProtocolID, proposals protocol.Proposals) error {
	for _, prop := range proposals {
		if prop.ProtocolID != prot {
			continue
		}
		// select first acceptable one from the list
		switch prot {
		case protocol.IKE:
			if cfg.ProposalIke.Within(prop.SaTransforms) {
				return nil
			}
		case protocol.ESP:
			if cfg.ProposalEsp.Within(prop.SaTransforms) {
				return nil
			}
		}
	}
	return errors.New("acceptable proposals are missing")
}

func SelectorFromAddress(addr *net.IPNet) ([]*protocol.Selector, error) {
	first, last, err := IPNetToFirstLastAddress(addr)
	if err != nil {
		return nil, err
	}
	stype := protocol.TS_IPV4_ADDR_RANGE
	if len(first) == net.IPv6len {
		stype = protocol.TS_IPV6_ADDR_RANGE
	}
	return []*protocol.Selector{&protocol.Selector{
		Type:         stype,
		IpProtocolId: 0,
		StartPort:    0,
		Endport:      65535,
		StartAddress: first,
		EndAddress:   last,
	}}, nil
}

// AddSelector builds selector from address & mask
func (cfg *Config) AddSelector(initiator, responder *net.IPNet) (err error) {
	tsI, err := SelectorFromAddress(initiator)
	if err != nil {
		return
	}
	cfg.TsI = tsI
	tsR, err := SelectorFromAddress(responder)
	if err != nil {
		return
	}
	cfg.TsR = tsR
	return
}

func (cfg *Config) AddHostBasedSelectors(local, remote net.IP, isInitiator bool) error {
	slen := len(local) * 8
	ini := remote
	res := local
	if isInitiator {
		ini = local
		res = remote
	}
	err := cfg.AddSelector(
		&net.IPNet{IP: ini, Mask: net.CIDRMask(slen, slen)},
		&net.IPNet{IP: res, Mask: net.CIDRMask(slen, slen)})
	if err != nil {
		return errors.Wrapf(err, "could not add selectors for %s=>%s", ini, res)
	}
	return nil
}

func ProposalFromTransform(prot protocol.ProtocolID, trs protocol.Transforms, spi []byte) []*protocol.SaProposal {
	return []*protocol.SaProposal{
		&protocol.SaProposal{
			IsLast:       true,
			Number:       1,
			ProtocolID:   prot,
			Spi:          append([]byte{}, spi...),
			SaTransforms: trs.AsList(),
		},
	}
}
