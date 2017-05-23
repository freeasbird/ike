package ike

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"testing"

	"math/big"
	"math/rand"

	"github.com/msgboxio/ike/protocol"
)

func newConfig() (config *Config) {
	config = testConfig()
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

func TestIkeMsgGen(t *testing.T) {
	cfg := newConfig()
	cfg.LocalID = pskTestID
	cfg.RemoteID = pskTestID
	sess, _ := NewInitiator(cfg, &net.IPAddr{}, nil, &SessionCallback{}, logger)
	// init msg
	init := InitFromSession(sess)
	init.IkeHeader.MsgID = 42
	// encode & write init msg
	initIb, err := init.Encode(sess.tkm, true, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("protocol/fuzz/corpus/corpus/sa_init_gen", initIb, 0644); err != nil {
		t.Fatal(err)
	}
	// auth
	sess.initIb = initIb
	no, _ := createNonce(sess.tkm.Ni.BitLen())
	err = sess.CreateIkeSa(no, big.NewInt(rand.Int63()), MakeSpi(), MakeSpi())
	if err != nil {
		t.Fatal(err)
	}
	authI, err := authFromSession(sess)
	if err != nil {
		t.Fatal(err)
	}
	authI.IkeHeader.MsgID = 43
	// encode & write authI msg
	authIb, err := authI.Encode(sess.tkm, true, logger)
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
