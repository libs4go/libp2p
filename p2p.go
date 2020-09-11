package p2p

import (
	"context"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/inconshreveable/muxado"
	"github.com/libs4go/bcf4go/key"
	_ "github.com/libs4go/bcf4go/key/encoding"               //
	_ "github.com/libs4go/bcf4go/key/provider"               //
	didProvider "github.com/libs4go/bcf4go/key/provider/did" //
	"github.com/libs4go/errors"
	"github.com/libs4go/slf4go"
	"github.com/libs4go/stf4go"
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
	ErrTLS          = errors.New("underlying transports must include stf4go/transports/tls", errors.WithVendor(errVendor), errors.WithCode(-4))
	ErrPeerID       = errors.New("remote peer id is not match", errors.WithVendor(errVendor), errors.WithCode(-5))
)

var didBip32Path = "m/44'/201910'/0'/0/0"
var didDriverName = "p2p"

func init() {
	didProvider.Vendor("p2p", "p2p")
}

// Host p2p host
type Host interface {
	ID() string
	LocalAddrs() []multiaddr.Multiaddr
	Register(paddr multiaddr.Multiaddr) error
	Find(id string) ([]multiaddr.Multiaddr, bool)
	Peers() []string
	Dial(ctx context.Context, id string, dialOpts ...grpc.DialOption) (*grpc.ClientConn, error)
	Server() *grpc.Server
	Run() error
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
	muxSessions     map[string]*sessionPair
	grpcServer      *grpc.Server
}

type sessionPair struct {
	in  muxado.Session
	out muxado.Session
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
		Logger:      slf4go.Get("p2p"),
		datapath:    "./data",
		peers:       make(map[string][]multiaddr.Multiaddr),
		muxSessions: make(map[string]*sessionPair),
		accepted:    make(chan net.Conn),
	}

	for _, opt := range options {
		if err := opt(host); err != nil {
			return nil, errors.Wrap(err, "call opt error")
		}
	}

	if host.laddrs == nil {
		opt := AddrStrings("/ip4/0.0.0.0/udp/1912/kcp/tls")

		if err := opt(host); err != nil {
			return nil, errors.Wrap(err, "call opt error")
		}
	}

	if !isExists(host.datapath) {
		if err := os.MkdirAll(host.datapath, 0644); err != nil {
			return nil, errors.Wrap(err, "create data path %s error", host.datapath)
		}
	}

	if host.persistence {

		storage, err := newStorage(host.datapath)

		if err != nil {
			return nil, errors.Wrap(err, "create storage %s error", host.datapath)
		}

		host.storage = storage
	}

	if err := host.boostrap(); err != nil {
		return nil, err
	}

	return host, nil
}

func (host *hostImpl) Run() error {
	return host.grpcServer.Serve(host)
}

func (host *hostImpl) ID() string {
	return host.id.Address()
}

func (host *hostImpl) LocalAddrs() []multiaddr.Multiaddr {
	return host.laddrs
}

func (host *hostImpl) Peers() []string {
	host.RLock()
	defer host.RUnlock()

	var ids []string

	for id := range host.peers {
		ids = append(ids, id)
	}

	return ids
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

	if prefix == nil {
		return errors.Wrap(err, "p2p register addr must have prefix")
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

func (host *hostImpl) Server() *grpc.Server {

	return host.grpcServer
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

type p2pNetConn struct {
	net.Conn
	laddr *p2pAddr
	raddr *p2pAddr
}

func newP2PNetConn(lid, rid string, conn net.Conn) net.Conn {
	return &p2pNetConn{
		Conn:  conn,
		laddr: &p2pAddr{id: lid},
		raddr: &p2pAddr{id: rid},
	}
}

func (conn *p2pNetConn) LocalAddr() net.Addr {
	return conn.laddr
}
func (conn *p2pNetConn) RemoteAddr() net.Addr {
	return conn.raddr
}
