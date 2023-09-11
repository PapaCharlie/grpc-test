package grpctest

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var resPointer atomic.Pointer[Response]

type server struct {
	UnimplementedEchoServiceServer
	*testing.B
}

func (s server) Echo(request *Request, echoServer EchoService_EchoServer) error {
	for i := int32(0); i < request.Count; i++ {
		if err := echoServer.Send(resPointer.Load()); err != nil {
			return err
		}
	}
	return nil
}

func benchmarkServer(b *testing.B, conns []grpc.ClientConnInterface) {
	req := &Request{Count: int32(b.N)}

	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(len(conns))
	for _, conn := range conns {
		c := NewEchoServiceClient(conn)

		go func() {
			defer done.Done()
			start.Wait()
			stream, err := c.Echo(context.Background(), req)
			require.NoError(b, err)

			var res Response
			for i := 0; i < b.N; i++ {
				require.NoError(b, stream.RecvMsg(&res))
				res.Reset()
			}
		}()
	}

	b.ResetTimer()
	start.Done()
	done.Wait()
}

func runBenchmark(b *testing.B, pool grpc.SendBufferPool) {
	s := grpc.NewServer(grpc.ServerSendBufferPool(pool))
	l, err := net.Listen("tcp", "localhost:0")
	require.NoError(b, err)
	RegisterEchoServiceServer(s, server{})
	go func() {
		require.NoError(b, s.Serve(l))
		require.NoError(b, l.Close())
	}()

	recvPool := grpc.NewSharedBufferPool()

	conns := make([]grpc.ClientConnInterface, 100)
	for i := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conns[i], err = grpc.DialContext(
			ctx,
			l.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithRecvBufferPool(recvPool),
		)
		cancel()
		require.NoError(b, err)
	}

	for size := 1 << 9; size <= 1<<16; size <<= 1 {
		resPointer.Store(&Response{
			Buffer: make([]byte, size),
		})
		b.Run(fmt.Sprintf("%d/%d", len(conns), size), func(b *testing.B) {
			benchmarkServer(b, conns)
		})
		runtime.GC()
	}
}

type noopSendBufferPool struct{}

func (n noopSendBufferPool) Get() []byte {
	return nil
}

func (n noopSendBufferPool) Put([]byte) {
}

func BenchmarkServerThroughput(b *testing.B) {
	var pool grpc.SendBufferPool
	if ok, _ := strconv.ParseBool(os.Getenv("SEND_BUFFER_POOL")); ok {
		pool = grpc.NewSendBufferPool()
	} else {
		pool = noopSendBufferPool{}
	}
	runBenchmark(b, pool)
}
