package p2p

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/libs4go/bcf4go/key"
	"github.com/libs4go/errors"
	_ "github.com/mattn/go-sqlite3" //
	"github.com/multiformats/go-multiaddr"
	"xorm.io/xorm"
)

// Storage .
type Storage interface {
	LocalKey(password string) (string, error)
	SaveLocalKey(mnemonic string, password string) error
	PutPeer(id string, addr multiaddr.Multiaddr, lease time.Duration) error
	GetPeer(id string) ([]multiaddr.Multiaddr, error)
	RemovePeer(id string, addr multiaddr.Multiaddr) error
}

type peerInfo struct {
	ID      string        `xorm:"unique(ID_ADDR)"`
	Addr    string        `xorm:"unique(ID_ADDR)"`
	Lease   time.Duration `xorm:""`
	Created time.Time     `xorm:"created"`
}

type storageImpl struct {
	rootpath string
	engine   *xorm.Engine
}

func newStorage(path string) (Storage, error) {

	if !isExists(path) {
		if err := os.MkdirAll(path, 0644); err != nil {
			return nil, errors.Wrap(err, "create data path %s error", path)
		}
	}

	dbpath := filepath.Join(path, "peer.db")

	engine, err := xorm.NewEngine("sqlite3", fmt.Sprintf("%s", dbpath))

	if err != nil {
		return nil, errors.Wrap(err, "create sqlite3 db %s error", dbpath)
	}

	if err := engine.Sync2(&peerInfo{}); err != nil {
		return nil, errors.Wrap(err, "sync scheme error")
	}

	storage := &storageImpl{
		rootpath: path,
		engine:   engine,
	}

	return storage, nil
}

func isExists(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func (storage *storageImpl) LocalKey(password string) (string, error) {

	path := filepath.Join(storage.rootpath, "key.json")

	if !isExists(path) {
		return "", nil
	}

	buff, err := ioutil.ReadFile(path)

	if err != nil {
		return "", errors.Wrap(err, "read file %s error", path)
	}

	buff, err = key.Decode("web3.standard", key.Property{
		"password": password,
	}, bytes.NewBuffer(buff))

	if err != nil {
		return "", errors.Wrap(err, "decode localkey file %s error", path)
	}

	return string(buff), nil
}

func (storage *storageImpl) SaveLocalKey(mnemonic string, password string) error {

	var buff bytes.Buffer

	err := key.Encode("web3.standard", []byte(mnemonic), key.Property{
		"password": password,
	}, &buff)

	if err != nil {
		return errors.Wrap(err, "encode local key error")
	}

	path := filepath.Join(storage.rootpath, "key.json")

	err = ioutil.WriteFile(path, buff.Bytes(), 0644)

	if err != nil {
		return errors.Wrap(err, "encode local key %s error", path)
	}

	return nil
}

func (storage *storageImpl) PutPeer(id string, addr multiaddr.Multiaddr, lease time.Duration) error {
	_, err := storage.engine.InsertOne(&peerInfo{
		ID:    id,
		Addr:  addr.String(),
		Lease: lease,
	})

	if err != nil {
		return errors.Wrap(err, "save peer %s %s error", id, addr)
	}

	return nil
}

func (storage *storageImpl) GetPeer(id string) ([]multiaddr.Multiaddr, error) {

	var peerInfos []peerInfo

	err := storage.engine.Where(`"i_d" = ?`, id).Find(&peerInfos)

	if err != nil {
		return nil, errors.Wrap(err, "query peer %s error", id)
	}

	var addrs []multiaddr.Multiaddr

	for _, info := range peerInfos {
		addr, err := multiaddr.NewMultiaddr(info.Addr)

		if err != nil {
			return nil, errors.Wrap(err, "parse multiaddr %s error", addr)
		}

		addrs = append(addrs, addr)
	}

	return addrs, nil
}

func (storage *storageImpl) RemovePeer(id string, addr multiaddr.Multiaddr) error {

	_, err := storage.engine.Where("i_d = ? and addr = ?", id, addr.String()).Delete(&peerInfo{})

	if err != nil {
		return errors.Wrap(err, "remove peer %s error", id)
	}

	return nil
}
