package p2p

import (
	"github.com/libs4go/bcf4go/key"
	"github.com/libs4go/errors"
	"github.com/multiformats/go-multiaddr"
)

const protocolID = 900

var protocolDID = multiaddr.Protocol{
	Name:       "did",
	Code:       protocolID,
	VCode:      multiaddr.CodeToVarint(protocolID),
	Transcoder: transcoderDID,
}

func p2pStB(s string) ([]byte, error) {
	return []byte(s), nil
}

func p2pVal(b []byte) error {

	if !key.ValidAddress(didDriverName, string(b)) {
		return errors.Wrap(ErrAddr, "invalid did %s", string(b))
	}

	return nil
}

func p2pBtS(b []byte) (string, error) {
	return string(b), nil
}

var transcoderDID = multiaddr.NewTranscoderFromFunctions(p2pStB, p2pBtS, nil)

var kcpMultiAddr multiaddr.Multiaddr

func init() {

	if err := multiaddr.AddProtocol(protocolDID); err != nil {
		panic(err)
	}
}

func didAddress(did string, prefix multiaddr.Multiaddr) (multiaddr.Multiaddr, error) {

	c, err := multiaddr.NewComponent("did", did)

	if err != nil {
		return nil, errors.Wrap(err, "make multiaddr.Component did error")
	}

	return multiaddr.Join(prefix, c), nil
}

func didFromAddress(addr multiaddr.Multiaddr) (multiaddr.Multiaddr, string, error) {
	prefix, c := multiaddr.SplitLast(addr)

	if prefix == nil || c == nil {
		return nil, "", errors.Wrap(ErrAddr, "invalid did address %s", addr.String())
	}

	if c.Protocol().Code != protocolID {
		return nil, "", errors.Wrap(ErrAddr, "invalid did address %s", addr.String())
	}

	return prefix, c.Value(), nil
}
