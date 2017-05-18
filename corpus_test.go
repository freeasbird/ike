package ike

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"testing"
	"time"

	"github.com/msgboxio/ike/crypto"
	"github.com/msgboxio/ike/protocol"
)

func newConfig() (config *Config) {
	config = DefaultConfig()
	config.AddNetworkSelectors(
		&net.IPNet{IP: net.IPv4(192, 168, 10, 2).To4(), Mask: net.CIDRMask(32, 32)},
		&net.IPNet{IP: net.IPv4(10, 10, 10, 2).To4(), Mask: net.CIDRMask(32, 32)},
		true)
	return
}

var ids = PskIdentities{
	Primary: "ak@msgbox.io",
	Ids:     map[string][]byte{"ak@msgbox.io": []byte("foo")},
}

func initiatorTkm(t *testing.T) *Tkm {
	config := newConfig()
	suite, err := crypto.NewCipherSuite(config.ProposalIke)
	if err != nil {
		t.Fatal(err)
	}
	tkm, err := NewTkmInitiator(suite, nil)
	if err != nil {
		t.Fatal(err)
	}
	// nonce for responder
	if nr, err := createNonce(suite.Prf.Length * 8); err != nil {
		t.Fatal(err)
	} else {
		tkm.Nr = nr
	}
	return tkm
}

func TestIkeMsgGen(t *testing.T) {
	cfg := newConfig()
	tkm := initiatorTkm(t)
	ikeSpi := MakeSpi()
	sess := &Session{
		isInitiator: true,
		tkm:         tkm,
		cfg:         *cfg,
		IkeSpiI:     ikeSpi,
		IkeSpiR:     MakeSpi(),
	}
	// init msg
	init := InitFromSession(sess)
	init.IkeHeader.MsgID = 42
	// encode & write init msg
	initIb, err := init.Encode(tkm, true, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("protocol/fuzz/corpus/corpus/sa_init_gen", initIb, 0644); err != nil {
		t.Fatal(err)
	}
	// auth
	authI, err := makeAuth(&authParams{
		false, true,
		cfg.IsTransportMode,
		sess.IkeSpiI, sess.IkeSpiR,
		protocol.ProposalFromTransform(protocol.IKE, cfg.ProposalIke, sess.IkeSpiI),
		cfg.TsI, cfg.TsR,
		&PskAuthenticator{tkm, true, pskTestID},
		time.Hour,
	}, initIb, logger)
	if err != nil {
		t.Fatal(err)
	}
	// overwrite NextPayload
	authI.IkeHeader.NextPayload = protocol.PayloadTypeIDi
	authI.IkeHeader.MsgID = 43
	// encode & write authI msg
	authIb, err := authI.Encode(tkm, true, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("protocol/fuzz/corpus/corpus/authI_gen", authIb, 0644); err != nil {
		t.Fatal(err)
	}
}

func testCorpusDecode(t *testing.T) {
	files, err := ioutil.ReadDir("protocol/fuzz/corpus/corpus/")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, err := ioutil.ReadFile("protocol/fuzz/corpus/corpus/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}

		hdr, err := protocol.DecodeIkeHeader(data)
		if err != nil {
			t.Errorf("hdr: %s:%s", file.Name(), err)
			t.Fail()
		}
		plData := data[protocol.IKE_HEADER_LEN:]
		payloads, err := protocol.DecodePayloads(plData, hdr.NextPayload)
		if err != nil {
			t.Errorf("pld: %s:%s", file.Name(), err)
			t.Fail()
		}
		// ensure encoding is same
		if enc := hdr.Encode(); !bytes.Equal(enc, data[:protocol.IKE_HEADER_LEN]) {
			t.Errorf("%s:%s", file.Name(), "unequal header")
			t.Fail()
		}
		if pld := protocol.EncodePayloads(payloads); !bytes.Equal(pld, plData[:len(pld)]) {
			t.Errorf("%s:%s", file.Name(), "unequal payload")
			t.Fail()
		}

		fmt.Println(file.Name())
		msg, err := decodeMessage(data, nil, false)
		if err != nil {
			t.Errorf("%s:%s", file.Name(), err)
			t.Fail()
		}
		js, err := json.MarshalIndent(msg, "", " ")
		if err != nil {
			t.Errorf("%s:%s", file.Name(), err)
			t.Fail()
		}
		t.Log("file", file.Name(), "data", string(js))
	}
}

func TestCorpusDecode(t *testing.T) {
	// testCorpusDecode(t)
}
