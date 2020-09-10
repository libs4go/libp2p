package p2p

import (
	"net"
	"testing"

	"github.com/inconshreveable/muxado"
	"github.com/stretchr/testify/require"
)

func TestMux(t *testing.T) {

	go testServer(t)

	testClient(t)
}

func testClient(t *testing.T) {

	conn, err := net.Dial("tcp", "127.0.0.1:1812")

	require.NoError(t, err)

	session := muxado.Client(conn, nil)

	server, err := session.Accept()

	require.NoError(t, err)

	var buff [100]byte

	len, err := server.Read(buff[:])

	require.NoError(t, err)

	require.Equal(t, string(buff[:len]), "hello")

	client, err := session.Open()

	require.NoError(t, err)

	_, err = client.Write([]byte("world"))

	require.NoError(t, err)

	len, err = server.Read(buff[:])

	require.NoError(t, err)

	require.Equal(t, string(buff[:len]), "p2p!!!!!")
}

func testServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:1812")

	require.NoError(t, err)

	conn, err := listener.Accept()

	require.NoError(t, err)

	session := muxado.Server(conn, nil)

	client, err := session.Open()

	require.NoError(t, err)

	_, err = client.Write([]byte("hello"))

	require.NoError(t, err)

	server, err := session.Accept()

	require.NoError(t, err)

	var buff [100]byte

	len, err := server.Read(buff[:])

	require.NoError(t, err)

	require.Equal(t, string(buff[:len]), "world")

	_, err = client.Write([]byte("p2p!!!!!"))

	require.NoError(t, err)

}
