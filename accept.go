package p2p

import (
	"net"

	"github.com/inconshreveable/muxado"
	"github.com/libs4go/stf4go"
	"github.com/libs4go/stf4go/transports/tls"
)

func (host *hostImpl) listen() error {

	for _, laddr := range host.laddrs {

		listener, err := stf4go.Listen(laddr, append(host.listenOps, tls.WithKey(host.id))...)

		if err != nil {
			return err
		}

		go host.accept(listener)
	}

	return nil
}

func (host *hostImpl) accept(listener stf4go.Listener) {
	for {
		conn, err := listener.Accept()

		host.D("listener {@laddr} accept {@raddr}", listener.Addr().String(), conn.RemoteAddr().String())

		if err != nil {
			host.E("listener {@addr} accept error: {@err}", listener.Addr().String(), err)
			continue
		}

		_, did, err := didFromAddress(conn.RemoteAddr())

		if err != nil {
			host.E("parse remote peer addr {@addr} err {@err}", conn.RemoteAddr().String(), err)
			continue
		}

		muxSession := muxado.Server(conn, nil)

		host.handleMuxAccept(did, muxSession, false)
	}
}

func (host *hostImpl) muxAcceptLoop(id string, muxSession muxado.Session, isClient bool) {

	defer func() {
		host.Lock()
		pair, ok := host.muxSessions[id]

		if ok {
			if isClient && pair.out == muxSession {
				pair.out = nil
			}

			if !isClient && pair.in == muxSession {
				pair.in = nil
			}

			if pair.in == nil && pair.out == nil {
				delete(host.muxSessions, id)
			}
		}
		host.Unlock()

		muxSession.Close()
	}()

	host.D("peer {@lid} mux session {@id} accept ...", host.ID(), id)

	for {
		conn, err := muxSession.Accept()

		host.D("peer {@lid} mux session {@id} accept one", host.ID(), id)
		if err != nil {
			host.E("peer session {@id} accept err: {@err}", id, err)

			code, _ := muxado.GetError(err)

			if code == muxado.SessionClosed || code == muxado.PeerEOF {
				host.D("peer {@lid} mux session {@id} accept -- finish", host.ID(), id)
				return
			}

			continue
		}

		host.accepted <- newP2PNetConn(host.ID(), id, conn)
	}
}

func (host *hostImpl) handleMuxAccept(id string, muxSession muxado.Session, isClient bool) muxado.Session {

	host.Lock()

	pair, ok := host.muxSessions[id]

	if !ok {
		pair = &sessionPair{}
	}

	var old muxado.Session

	if isClient {
		old = pair.out
		pair.out = muxSession
	} else {
		old = pair.in
		pair.in = muxSession
	}

	host.muxSessions[id] = pair

	host.Unlock()

	if old != nil {
		host.D("(client:{@client})host {@lid} got exists mux session {@rid}", isClient, host.ID(), id)
		old.Close()
	}

	go host.muxAcceptLoop(id, muxSession, isClient)

	return muxSession
}

func (host *hostImpl) Accept() (net.Conn, error) {

	conn, ok := <-host.accepted

	if !ok {
		return nil, ErrClosed
	}

	return conn, nil
}

func (host *hostImpl) Close() error {
	close(host.accepted)
	return nil
}
