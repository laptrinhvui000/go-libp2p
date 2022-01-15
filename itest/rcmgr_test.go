package itest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rcmgr "github.com/libp2p/go-libp2p-resource-manager"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
)

func makeRcmgrOption(t *testing.T, limiter *rcmgr.BasicLimiter, test string) func(int) libp2p.Option {
	return func(i int) libp2p.Option {
		var opts []rcmgr.Option

		if os.Getenv("LIBP2P_TEST_RCMGR_TRACE") == "1" {
			opts = append(opts, rcmgr.WithTrace(fmt.Sprintf("%s-%d.json.gz", test, i)))
		}

		mgr, err := rcmgr.NewResourceManager(limiter, opts...)
		if err != nil {
			t.Fatal(err)
		}
		return libp2p.ResourceManager(mgr)
	}
}

func closeRcmgrs(echos []*Echo) {
	for _, e := range echos {
		e.Host.Network().ResourceManager().Close()
	}
}

func TestResourceManagerConnInbound(t *testing.T) {
	// this test checks that we can not exceed the inbound conn limit at system level
	// we specify: 1 conn per peer, 3 conns total, and we try to create 4 conns
	limiter := rcmgr.NewFixedLimiter(1 << 30)
	limiter.SystemLimits = limiter.SystemLimits.WithConnLimit(3, 1024, 1024)
	limiter.DefaultPeerLimits = limiter.DefaultPeerLimits.WithConnLimit(1, 16, 16)

	echos := createEchos(t, 5, makeRcmgrOption(t, limiter, "TestResourceManagerConnInbound"))
	defer closeEchos(echos)
	defer closeRcmgrs(echos)

	for i := 1; i < 4; i++ {
		err := echos[i].Host.Connect(context.Background(), peer.AddrInfo{ID: echos[0].Host.ID()})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	for i := 1; i < 4; i++ {
		count := len(echos[i].Host.Network().ConnsToPeer(echos[0].Host.ID()))
		if count != 1 {
			t.Fatalf("expected %d connections to peer, got %d", 1, count)
		}
	}

	err := echos[4].Host.Connect(context.Background(), peer.AddrInfo{ID: echos[0].Host.ID()})
	if err == nil {
		t.Fatal("expected ResourceManager to block incoming connection")
	}
}

func TestResourceManagerConnOutbound(t *testing.T) {
	// this test checks that we can not exceed the inbound conn limit at system level
	// we specify: 1 conn per peer, 3 conns total, and we try to create 4 conns
	limiter := rcmgr.NewFixedLimiter(1 << 30)
	limiter.SystemLimits = limiter.SystemLimits.WithConnLimit(1024, 3, 1024)
	limiter.DefaultPeerLimits = limiter.DefaultPeerLimits.WithConnLimit(16, 1, 16)
	echos := createEchos(t, 5, makeRcmgrOption(t, limiter, "TestResourceManagerConnOutbound"))
	defer closeEchos(echos)
	defer closeRcmgrs(echos)

	for i := 1; i < 4; i++ {
		err := echos[0].Host.Connect(context.Background(), peer.AddrInfo{ID: echos[i].Host.ID()})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	for i := 1; i < 4; i++ {
		count := len(echos[i].Host.Network().ConnsToPeer(echos[0].Host.ID()))
		if count != 1 {
			t.Fatalf("expected %d connections to peer, got %d", 1, count)
		}
	}

	err := echos[0].Host.Connect(context.Background(), peer.AddrInfo{ID: echos[4].Host.ID()})
	if err == nil {
		t.Fatal("expected ResourceManager to block incoming connection")
	}
}

func TestResourceManagerServiceInbound(t *testing.T) {
	// this test checks that we can not exceed the inbound stream limit at service level
	// we specify: 3 streams for the service, and we try to create 4 streams
	limiter := rcmgr.NewFixedLimiter(1 << 30)
	limiter.DefaultServiceLimits = limiter.DefaultServiceLimits.WithStreamLimit(3, 1024, 1024)
	echos := createEchos(t, 5, makeRcmgrOption(t, limiter, "TestResourceManagerServiceInbound"))
	defer closeEchos(echos)
	defer closeRcmgrs(echos)

	ready := new(chan struct{})
	echos[0].BeforeDone = waitForChannel(ready, time.Minute)

	for i := 1; i < 5; i++ {
		err := echos[i].Host.Connect(context.Background(), peer.AddrInfo{ID: echos[0].Host.ID()})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	*ready = make(chan struct{})

	var once sync.Once
	var wg sync.WaitGroup
	for i := 1; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			err := echos[i].Echo(echos[0].Host.ID(), "hello libp2p")
			if err != nil {
				t.Log(err)
				once.Do(func() {
					close(*ready)
				})
			}
		}(i)
	}
	wg.Wait()

	checkEchoStatus(t, echos[0], EchoStatus{
		StreamsIn:             4,
		EchosIn:               3,
		EchosOut:              3,
		ResourceServiceErrors: 1,
	})
}

func TestResourceManagerServicePeerInbound(t *testing.T) {
	// this test checks that we cannot exceed the per peer inbound stream limit at service level
	// we specify: 2 streams per peer for echo, and we try to create 3 streams
	limiter := rcmgr.NewFixedLimiter(1 << 30)
	limiter.ServicePeerLimits = map[string]rcmgr.Limit{
		EchoService: limiter.DefaultPeerLimits.WithStreamLimit(2, 1024, 1024),
	}
	echos := createEchos(t, 5, makeRcmgrOption(t, limiter, "TestResourceManagerServicePeerInbound"))
	defer closeEchos(echos)
	defer closeRcmgrs(echos)

	count := new(int32)
	ready := new(chan struct{})
	echos[0].BeforeDone = waitForBarrier(count, ready, time.Minute)

	for i := 1; i < 5; i++ {
		err := echos[i].Host.Connect(context.Background(), peer.AddrInfo{ID: echos[0].Host.ID()})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	*count = 4
	*ready = make(chan struct{})

	var wg sync.WaitGroup
	for i := 1; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			err := echos[i].Echo(echos[0].Host.ID(), "hello libp2p")
			if err != nil {
				t.Log(err)
			}
		}(i)
	}
	wg.Wait()

	checkEchoStatus(t, echos[0], EchoStatus{
		StreamsIn:             4,
		EchosIn:               4,
		EchosOut:              4,
		ResourceServiceErrors: 0,
	})

	*ready = make(chan struct{})
	echos[0].BeforeDone = waitForChannel(ready, time.Minute)

	var once sync.Once
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := echos[2].Echo(echos[0].Host.ID(), "hello libp2p")
			if err != nil {
				t.Log(err)
				once.Do(func() {
					close(*ready)
				})
			}
		}()
	}
	wg.Wait()

	checkEchoStatus(t, echos[0], EchoStatus{
		StreamsIn:             7,
		EchosIn:               6,
		EchosOut:              6,
		ResourceServiceErrors: 1,
	})
}

func waitForBarrier(count *int32, ready *chan struct{}, timeout time.Duration) func() error {
	return func() error {
		if atomic.AddInt32(count, -1) == 0 {
			close(*ready)
		}

		select {
		case <-*ready:
			return nil
		case <-time.After(timeout):
			return fmt.Errorf("timeout")
		}
	}
}

func waitForChannel(ready *chan struct{}, timeout time.Duration) func() error {
	return func() error {
		select {
		case <-*ready:
			return nil
		case <-time.After(timeout):
			return fmt.Errorf("timeout")
		}
	}
}
