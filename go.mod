module github.com/libs4go/p2p.git

go 1.14

require (
	github.com/golang/protobuf v1.4.2
	github.com/hashicorp/yamux v0.0.0-20200609203250-aecfd211c9ce // indirect
	github.com/inconshreveable/muxado v0.0.0-20160802230925-fc182d90f26e
	github.com/libs4go/bcf4go v0.0.13
	github.com/libs4go/errors v0.0.3
	github.com/libs4go/scf4go v0.0.8
	github.com/libs4go/slf4go v0.0.4
	github.com/libs4go/stf4go v0.0.2
	github.com/multiformats/go-multiaddr v0.3.1
	github.com/stretchr/testify v1.6.1
	google.golang.org/grpc v1.32.0
)

// replace github.com/libs4go/stf4go v0.0.2 => ../stf4go
