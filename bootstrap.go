package p2p

import (
	"bytes"

	"github.com/libs4go/bcf4go/key"
	"github.com/libs4go/errors"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"
)

func (host *hostImpl) boostrap() error {

	if err := host.boostrapMnemonic(); err != nil {
		return err
	}

	// create local id

	id, err := key.DriveKey(didDriverName, host.mnemonic, didBip32Path)

	if err != nil {
		host.E("invalid mnemonic {@mnemonic}", host.mnemonic)
		return err
	}

	host.id = id

	var buff bytes.Buffer

	err = key.Encode("web3.standard", id.PriKey(), key.Property{
		"password": host.protectPassword,
	}, &buff)

	if err != nil {
		return err
	}

	host.tlsKeyStore = buff.Bytes()

	var laddrs []multiaddr.Multiaddr

	for _, laddr := range host.laddrs {
		laddr, err := didAddress(host.id.Address(), laddr)

		if err != nil {
			return err
		}

		host.D("listen on {@laddr}", laddr.String())

		laddrs = append(laddrs, laddr)
	}

	host.laddrs = laddrs

	if err := host.listen(); err != nil {
		return err
	}

	host.grpcServer = grpc.NewServer()

	return nil
}

func (host *hostImpl) boostrapMnemonic() error {
	if host.mnemonic == "" {
		if host.mnemonic == "" {
			mnemonic, err := key.RandomMnemonic(16)

			if err != nil {
				return errors.Wrap(err, "load local key error")
			}

			host.mnemonic = mnemonic
		}
	}

	return nil
}
