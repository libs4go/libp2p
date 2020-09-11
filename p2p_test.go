package p2p

import (
	"context"
	"fmt"
	"testing"

	"github.com/libs4go/p2p.git/echo"
	"github.com/libs4go/scf4go"
	"github.com/libs4go/scf4go/reader/memory"
	"github.com/libs4go/slf4go"
	_ "github.com/libs4go/slf4go/backend/console" //
	"github.com/stretchr/testify/require"
)

//go:generate protoc --proto_path=./echo --go_out=plugins=grpc,paths=source_relative:./echo echo.proto

func createRandomHost(port int) (Host, error) {
	return New(AddrStrings(fmt.Sprintf("/ip4/0.0.0.0/udp/%d/kcp/tls", port)))
}

type echoServer struct {
}

func (s *echoServer) Say(ctx context.Context, request *echo.Request) (*echo.Response, error) {
	return &echo.Response{Message: request.Message}, nil
}

var loggerjson = `
{
	"default":{
		"backend":"null",
		"level":"debug"
	},
	"logger":{
		"p2p":{
			"backend":"null",
			"level":"debug"
		},
		"stf4go-transport-p2p":{
			"backend":"null",
			"level":"debug"
		}

	},
	"backend":{
		"console":{
			"formatter":{
				"output": "@t @l @s @m"
			}
		}
	}
}
`

func init() {
	config := scf4go.New()

	err := config.Load(memory.New(memory.Data(loggerjson, "json")))

	if err != nil {
		panic(err)
	}

	err = slf4go.Config(config)

	if err != nil {
		panic(err)
	}
}

var h1 Host
var h2 Host

var e1 echo.EchoClient
var e2 echo.EchoClient

func TestEcho(t *testing.T) {

	if h1 == nil || h2 == nil {
		h1, h2 = createEchoPair(t)
	}

	if e1 == nil {
		e1 = createEchoClient(t, h1, h2.ID())
	}

	r1, err := e1.Say(context.Background(), &echo.Request{
		Message: "hello s2",
	})

	require.NoError(t, err)

	require.Equal(t, r1.Message, "hello s2")

	if e2 == nil {
		e2 = createEchoClient(t, h2, h1.ID())
	}

	r2, err := e2.Say(context.Background(), &echo.Request{
		Message: "hello s1",
	})

	require.NoError(t, err)

	require.Equal(t, r2.Message, "hello s1")
}

func TestConcurrentConnect(t *testing.T) {

	if h1 == nil || h2 == nil {
		h1, h2 = createEchoPair(t)
	}

	if e1 == nil {
		e1 = createEchoClient(t, h1, h2.ID())
	}

	if e2 == nil {
		e2 = createEchoClient(t, h2, h1.ID())
	}

	r1, err := e1.Say(context.Background(), &echo.Request{
		Message: "hello s2",
	})

	require.NoError(t, err)

	require.Equal(t, r1.Message, "hello s2")

	r2, err := e2.Say(context.Background(), &echo.Request{
		Message: "hello s1",
	})

	require.NoError(t, err)

	require.Equal(t, r2.Message, "hello s1")

	// time.Sleep(time.Second * 12)

	r2, err = e2.Say(context.Background(), &echo.Request{
		Message: "hello s1",
	})

	require.NoError(t, err)

	require.Equal(t, r2.Message, "hello s1")

	r1, err = e1.Say(context.Background(), &echo.Request{
		Message: "hello s2",
	})

	require.NoError(t, err)

	require.Equal(t, r1.Message, "hello s2")
}

func createEchoClient(t require.TestingT, host Host, remote string) echo.EchoClient {
	c1, err := host.Dial(context.Background(), remote)

	require.NoError(t, err)

	e1 := echo.NewEchoClient(c1)

	return e1
}

func createEchoPair(t require.TestingT) (Host, Host) {
	h1, err := createRandomHost(1812)
	require.NoError(t, err)
	require.NotNil(t, h1)

	h2, err := createRandomHost(1813)
	require.NoError(t, err)
	require.NotNil(t, h2)

	s1 := h1.Server()

	require.NotNil(t, s1)

	echo.RegisterEchoServer(s1, &echoServer{})

	s2 := h2.Server()

	require.NotNil(t, s2)

	require.Equal(t, s2, h2.Server())

	echo.RegisterEchoServer(s2, &echoServer{})

	err = h1.Register(h2.LocalAddrs()[0])

	require.NoError(t, err)

	err = h2.Register(h1.LocalAddrs()[0])

	require.NoError(t, err)

	go h1.Run()

	go h2.Run()

	return h1, h2
}

// invoke benchmark command line:
// go test -bench=BenchmarkEcho -run=none -benchtime=3s

func BenchmarkEcho(b *testing.B) {
	b.StopTimer()
	if h1 == nil || h2 == nil {
		h1, h2 = createEchoPair(b)
	}

	if e1 == nil {
		e1 = createEchoClient(b, h1, h2.ID())
	}

	b.StartTimer()

	for i := 0; i < b.N; i++ {
		r2, err := e1.Say(context.Background(), &echo.Request{
			Message: "hello s1",
		})

		require.NoError(b, err)

		require.Equal(b, r2.Message, "hello s1")
	}

}
