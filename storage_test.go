package p2p

import (
	"testing"

	"github.com/libs4go/bcf4go/key"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestStorage(t *testing.T) {
	storage, err := newStorage("./data")

	require.NoError(t, err)

	require.NotNil(t, storage)

	mnemonic, err := key.RandomMnemonic(16)

	require.NoError(t, err)

	err = storage.SaveLocalKey(mnemonic, "test")

	require.NoError(t, err)

	mnemonic1, err := storage.LocalKey("test")

	require.NoError(t, err)

	require.Equal(t, mnemonic, mnemonic1)

	k, err := key.RandomKey("eth")

	require.NoError(t, err)

	m1, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1")

	require.NoError(t, err)

	m2, err := multiaddr.NewMultiaddr("/ip6/::1")

	require.NoError(t, err)

	err = storage.PutPeer(k.Address(), m1, 0)

	require.NoError(t, err)

	err = storage.PutPeer(k.Address(), m2, 0)

	require.NoError(t, err)

	ms, err := storage.GetPeer(k.Address())

	require.NoError(t, err)

	require.Equal(t, len(ms), 2)
}
