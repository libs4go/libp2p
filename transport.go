package p2p

import (
	"github.com/libs4go/bcf4go/key"
	"github.com/libs4go/errors"
	"github.com/libs4go/slf4go"
	"github.com/libs4go/stf4go"
	_ "github.com/libs4go/stf4go/transports/kcp" //
	_ "github.com/libs4go/stf4go/transports/tcp" //
	"github.com/libs4go/stf4go/transports/tls"
	"github.com/multiformats/go-multiaddr"
)

const protocolID = 900

var protocolDID = multiaddr.Protocol{
	Name:       "did",
	Code:       protocolID,
	VCode:      multiaddr.CodeToVarint(protocolID),
	Size:       multiaddr.LengthPrefixedVarSize,
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

var transcoderDID = multiaddr.NewTranscoderFromFunctions(p2pStB, p2pBtS, p2pVal)

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

	return prefix.Encapsulate(c), nil
}

func didFromAddress(addr multiaddr.Multiaddr) (multiaddr.Multiaddr, string, error) {
	prefix, c := multiaddr.SplitLast(addr)

	if c == nil {
		return nil, "", errors.Wrap(ErrAddr, "invalid did address %s", addr.String())
	}

	if c.Protocol().Code != protocolID {
		return nil, "", errors.Wrap(ErrAddr, "invalid did address %s", addr.String())
	}

	return prefix, c.Value(), nil
}

type p2pTransport struct {
	slf4go.Logger
}

func (transport *p2pTransport) String() string {
	return "stf4go-transport-p2p"
}

func (transport *p2pTransport) Protocols() []multiaddr.Protocol {
	return []multiaddr.Protocol{
		protocolDID,
	}
}

func getTLSConn(conn stf4go.Conn) (tls.Conn, bool) {

	for conn != nil {
		tlsConn, ok := conn.(tls.Conn)

		if ok {
			return tlsConn, true
		}

		conn = conn.Underlying()
	}

	return nil, false
}

func (transport *p2pTransport) Client(conn stf4go.Conn, raddr multiaddr.Multiaddr, options *stf4go.Options) (stf4go.Conn, error) {

	_, rid, err := didFromAddress(raddr)

	if err != nil {
		return nil, err
	}

	tlsConn, ok := getTLSConn(conn)

	if !ok {
		return nil, errors.Wrap(ErrTLS, "expect transport not found")
	}

	pubkey := <-tlsConn.RemoteKey()

	id := key.PubKeyToAddress(didDriverName, pubkey)

	if rid != id {
		return nil, errors.Wrap(ErrPeerID, "connect to %s, recv id %s", rid, id)
	}

	lid := key.PubKeyToAddress(didDriverName, tlsConn.LocalKey())

	return newP2PConn(conn, rid, lid)
}

func (transport *p2pTransport) Server(conn stf4go.Conn, laddr multiaddr.Multiaddr, options *stf4go.Options) (stf4go.Conn, error) {

	tlsConn, ok := getTLSConn(conn)

	if !ok {
		return nil, errors.Wrap(ErrTLS, "expect transport not found")
	}

	pubkey := <-tlsConn.RemoteKey()

	rid := key.PubKeyToAddress(didDriverName, pubkey)

	lid := key.PubKeyToAddress(didDriverName, tlsConn.LocalKey())

	return newP2PConn(conn, rid, lid)
}

func newP2PTransport() *p2pTransport {
	return &p2pTransport{
		Logger: slf4go.Get("stf4go-transport-p2p"),
	}
}

type p2pConn struct {
	stf4go.Conn
	laddr multiaddr.Multiaddr
	raddr multiaddr.Multiaddr
}

func newP2PConn(conn stf4go.Conn, rid string, lid string) (stf4go.Conn, error) {

	lc, err := multiaddr.NewComponent("did", lid)

	if err != nil {
		return nil, err
	}

	rc, err := multiaddr.NewComponent("did", rid)

	if err != nil {
		return nil, err
	}

	return &p2pConn{
		Conn:  conn,
		laddr: multiaddr.Join(conn.LocalAddr(), lc),
		raddr: multiaddr.Join(conn.RemoteAddr(), rc),
	}, nil
}

func (conn *p2pConn) LocalAddr() multiaddr.Multiaddr {
	return conn.laddr
}

func (conn *p2pConn) RemoteAddr() multiaddr.Multiaddr {
	return conn.raddr
}

func (conn *p2pConn) Underlying() stf4go.Conn {
	return conn.Conn
}

func init() {
	stf4go.RegisterTransport(newP2PTransport())
}
