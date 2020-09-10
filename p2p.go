package p2p

import (
	"bytes"
	"context"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/inconshreveable/muxado"
	"github.com/libs4go/bcf4go/key"
	"github.com/libs4go/errors"
	"github.com/libs4go/slf4go"
	"github.com/libs4go/stf4go"
	"github.com/libs4go/stf4go/transports/tls"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/grpc"
)

// ScopeOfAPIError .
const errVendor = "p2p"

// errors
var (
	ErrAddr         = errors.New("did address error", errors.WithVendor(errVendor), errors.WithCode(-1))
	ErrPeerNotFound = errors.New("peer not register", errors.WithVendor(errVendor), errors.WithCode(-2))
	ErrClosed       = errors.New("peer host closed", errors.WithVendor(errVendor), errors.WithCode(-3))
)

var didBip32Path = "m/44'/201910'/0'/0/0"
var didDriverName = "w3"

// Host p2p host
type Host interface {
	Register(paddr multiaddr.Multiaddr) error
	Find(id string) ([]multiaddr.Multiaddr, bool)
	Dial(ctx context.Context, id string, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error)
	Server(opts ...grpc.ServerOption) *grpc.Server
}

type hostImpl struct {
	sync.RWMutex
	slf4go.Logger
	mnemonic        string
	dialOps         []stf4go.Option
	listenOps       []stf4go.Option
	persistence     bool
	protectPassword string
	datapath        string
	laddrs          []multiaddr.Multiaddr
	peers           map[string][]multiaddr.Multiaddr
	storage         Storage
	id              key.Key
	tlsKeyStore     []byte
	accepted        chan net.Conn
	muxSessions     map[string]muxado.Session
}

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

// Persistence storage peers/id and etc .. persistence
// password protect the storage data
func Persistence(password string) Option {
	return func(host *hostImpl) error {
		host.persistence = true
		host.protectPassword = password
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

// New .
func New(options ...Option) (Host, error) {

	host := &hostImpl{
		Logger:   slf4go.Get("p2p"),
		datapath: "./data",
		peers:    make(map[string][]multiaddr.Multiaddr),
	}

	for _, opt := range options {
		if err := opt(host); err != nil {
			return nil, errors.Wrap(err, "call opt error")
		}
	}

	if host.laddrs == nil {
		opt := AddrStrings(
			"/ip4/0.0.0.0/udp/1912/kcp/tls",
			"/ip6/::1/udp/1912/kcp/tls")

		if err := opt(host); err != nil {
			return nil, errors.Wrap(err, "call opt error")
		}
	}

	if !isExists(host.datapath) {
		if err := os.MkdirAll(host.datapath, 0644); err != nil {
			return nil, errors.Wrap(err, "create data path %s error", host.datapath)
		}
	}

	storage, err := newStorage(host.datapath)

	if err != nil {
		return nil, errors.Wrap(err, "create storage %s error", host.datapath)
	}

	host.storage = storage

	if err := host.boostrap(); err != nil {
		return nil, err
	}

	return host, nil
}

func (host *hostImpl) boostrap() error {

	if err := host.boostrapMnemonic(); err != nil {
		return err
	}

	// create local id

	id, err := key.DriveKey(didDriverName, host.mnemonic, didBip32Path)

	if err != nil {
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

	if err := host.listen(); err != nil {
		return err
	}

	return nil
}

func (host *hostImpl) listen() error {
	for _, laddr := range host.laddrs {
		laddr, err := didAddress(host.id.Address(), laddr)

		if err != nil {
			return err
		}

		listener, err := stf4go.Listen(laddr, host.listenOps...)

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

		if err != nil {
			host.E("listener %s accept error: %s", listener.Addr().String(), err)
			continue
		}

		_, did, err := didFromAddress(conn.RemoteAddr())

		if err != nil {
			host.E("parse remote peer addr %s err %s", conn.RemoteAddr().String(), err)
			continue
		}

		muxSession := muxado.Server(conn, nil)

		go host.handleMuxAccept(did, muxSession)
	}
}

func (host *hostImpl) handleMuxAccept(id string, muxSession muxado.Session) {

	defer func() {
		host.Lock()
		session, ok := host.muxSessions[id]

		if ok && session == muxSession {
			delete(host.muxSessions, id)
		}
		host.Unlock()

		muxSession.Close()
	}()

	host.Lock()
	_, ok := host.muxSessions[id]

	if !ok {
		host.muxSessions[id] = muxSession
	}

	host.Unlock()

	if !ok {
		return
	}

	for {
		conn, err := muxSession.Accept()

		if err != nil {
			host.E("peer session %s accept error: %s", id, err)

			code, _ := muxado.GetError(err)

			if code == muxado.SessionClosed {
				return
			}

			continue
		}

		host.accepted <- conn
	}
}

func (host *hostImpl) boostrapMnemonic() error {
	if host.mnemonic == "" {
		if host.persistence {
			mnemonic, err := host.storage.LocalKey(host.protectPassword)

			if err != nil {
				return errors.Wrap(err, "load local key error")
			}

			host.mnemonic = mnemonic
		}

		if host.mnemonic == "" {
			mnemonic, err := key.RandomMnemonic(12)

			if err != nil {
				return errors.Wrap(err, "load local key error")
			}

			host.mnemonic = mnemonic

			if host.persistence {
				if err := host.storage.SaveLocalKey(mnemonic, host.protectPassword); err != nil {
					return errors.Wrap(err, "save local key error")
				}
			}
		}
	}

	return nil
}

func (host *hostImpl) checkAddr(id string, addr multiaddr.Multiaddr) bool {
	addrs, ok := host.Find(id)

	if !ok && host.persistence {
		var err error
		addrs, err = host.storage.GetPeer(id)

		if err != nil {
			host.E("get peer {@id} from storage error", id)
			return false
		}

		host.Lock()
		host.peers[id] = addrs
		host.Unlock()
	}

	for _, m := range addrs {
		if m.Equal(addr) {
			return true
		}
	}

	return false
}

func (host *hostImpl) Find(id string) ([]multiaddr.Multiaddr, bool) {
	host.RLock()
	defer host.RUnlock()
	addrs, ok := host.peers[id]

	return addrs, ok
}

func (host *hostImpl) Register(addr multiaddr.Multiaddr) error {
	prefix, id, err := didFromAddress(addr)

	if err != nil {
		return errors.Wrap(err, "p2p register addr must end with /did")
	}

	if host.checkAddr(id, prefix) {
		return nil
	}

	host.Lock()
	host.peers[id] = append(host.peers[id], prefix)
	host.Unlock()

	if host.persistence {
		if err := host.storage.PutPeer(id, prefix, 0); err != nil {
			return errors.Wrap(err, "save peer %s with addr %s error", id, prefix)
		}
	}

	return nil
}

func shuffle(vals []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	ret := make([]multiaddr.Multiaddr, len(vals))
	n := len(vals)
	for i := 0; i < n; i++ {
		randIndex := r.Intn(len(vals))
		ret[i] = vals[randIndex]
		vals = append(vals[:randIndex], vals[randIndex+1:]...)
	}
	return ret
}

func (host *hostImpl) Dial(ctx context.Context, id string, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	dialOpsPrepended := append([]grpc.DialOption{host.dialOption(ctx)}, dialOpts...)

	return grpc.DialContext(ctx, id, dialOpsPrepended...)
}

func (host *hostImpl) dial(ctx context.Context, remote string) (stf4go.Conn, error) {
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

		conn, err := stf4go.Dial(ctx, da, tls.KeyWeb3(host.tlsKeyStore), tls.KeyPassword(host.protectPassword), tls.KeyProvider(didDriverName))

		if err != nil {
			return nil, err
		}

		return conn, nil
	}

	return nil, errors.Wrap(ErrPeerNotFound, "peer %s not register", remote)
}

func (host *hostImpl) dialOption(ctx context.Context) grpc.DialOption {
	return grpc.WithDialer(func(remote string, timeout time.Duration) (net.Conn, error) {
		host.RLock()
		session, ok := host.muxSessions[remote]
		host.RUnlock()

		if ok {
			return session.Open()
		}

		underlyConn, err := host.dial(ctx, remote)

		if err != nil {
			return nil, err
		}

		muxSession := muxado.Client(underlyConn, nil)

		go host.handleMuxAccept(remote, muxSession)

		return muxSession.Open()
	})
}

func (host *hostImpl) Server(opts ...grpc.ServerOption) *grpc.Server {

	server := grpc.NewServer(opts...)

	go server.Serve(host)

	return server
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

type p2pAddr struct {
	id string
}

func (addr *p2pAddr) Network() string {
	return "did"
}

func (addr *p2pAddr) String() string {
	return addr.id
}

func (host *hostImpl) Addr() net.Addr {
	return &p2pAddr{id: host.id.Address()}
}
