package p2p

import (
	"context"
	"net"
	"time"

	"github.com/inconshreveable/muxado"
	"github.com/libs4go/errors"
	"github.com/libs4go/stf4go"
	"github.com/libs4go/stf4go/transports/tls"
	"google.golang.org/grpc"
)

func (host *hostImpl) Dial(ctx context.Context, id string, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	dialOpsPrepended := append([]grpc.DialOption{host.dialOption(ctx), grpc.WithInsecure()}, dialOpts...)

	return grpc.DialContext(ctx, id, dialOpsPrepended...)
}

func (host *hostImpl) dial(ctx context.Context, remote string) (stf4go.Conn, error) {

	host.D("host {@lid} dial to {@id}", host.ID(), remote)

	addrs, ok := host.Find(remote)

	if !ok {
		return nil, errors.Wrap(ErrPeerNotFound, "peer %s not register", remote)
	}

	addrs = shuffle(addrs)

	for _, addr := range addrs {
		da, err := didAddress(remote, addr)

		if err != nil {
			return nil, err
		}

		host.D("host {@lid} dial to {@addr}", host.ID(), addr.String())

		conn, err := stf4go.Dial(ctx, da, append(host.dialOps, tls.WithKey(host.id))...)

		if err != nil {
			host.E("host {@lid} dial to {@addr} err {@err}", host.ID(), addr.String(), err)
			return nil, err
		}

		return conn, nil
	}

	return nil, errors.Wrap(ErrPeerNotFound, "peer %s not register", remote)
}

func (host *hostImpl) dialOption(ctx context.Context) grpc.DialOption {
	return grpc.WithDialer(func(remote string, timeout time.Duration) (net.Conn, error) {

		host.D("host {@lid} dial to {@rid}", host.ID(), remote)

		host.RLock()
		pair, ok := host.muxSessions[remote]
		host.RUnlock()

		if ok {

			var session muxado.Session

			if pair.out != nil {
				session = pair.out
			} else {
				session = pair.in
			}

			if session != nil {
				host.D("host {@lid} dial to {@rid} got exists session", host.ID(), remote)
				conn, err := session.Open()

				if err != nil {
					host.D("host {@lid} dial to {@rid} got exists session err {@err}", host.ID(), remote, err)
				}

				return newP2PNetConn(host.ID(), remote, conn), err
			}
		}

		underlyConn, err := host.dial(ctx, remote)

		if err != nil {
			return nil, err
		}

		muxSession := muxado.Client(underlyConn, nil)

		muxSession = host.handleMuxAccept(remote, muxSession, true)

		return muxSession.Open()
	})
}
