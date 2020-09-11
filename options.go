package p2p

import (
	"github.com/libs4go/errors"
	"github.com/libs4go/stf4go"
	"github.com/multiformats/go-multiaddr"
)

// Option .
type Option func(*hostImpl) error

// WithMnemonic set host key with mnemonic
func WithMnemonic(mnemonic string) Option {
	return func(host *hostImpl) error {
		host.mnemonic = mnemonic
		return nil
	}
}

// DialOpts .
func DialOpts(options ...stf4go.Option) Option {
	return func(host *hostImpl) error {
		host.dialOps = options
		return nil
	}
}

// ListenOpts .
func ListenOpts(options ...stf4go.Option) Option {
	return func(host *hostImpl) error {
		host.listenOps = options
		return nil
	}
}

// Addrs set host listen addrs
func Addrs(laddrs ...multiaddr.Multiaddr) Option {
	return func(host *hostImpl) error {
		host.laddrs = laddrs
		return nil
	}
}

// AddrStrings set host listen addrs with string format
func AddrStrings(laddrs ...string) Option {
	return func(host *hostImpl) error {
		for _, laddr := range laddrs {
			ma, err := multiaddr.NewMultiaddr(laddr)

			if err != nil {
				return errors.Wrap(err, "parse multiaddr %s error", laddr)
			}

			host.laddrs = append(host.laddrs, ma)
		}
		return nil
	}
}
