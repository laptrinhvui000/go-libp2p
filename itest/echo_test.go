package itest

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
)

func createEchos(t *testing.T, count int, opts ...libp2p.Option) []*Echo {
	result := make([]*Echo, 0, count)

	for i := 0; i < count; i++ {
		h, err := libp2p.New(opts...)
		if err != nil {
			t.Fatal(err)
		}

		e := NewEcho(h)
		result = append(result, e)
	}

	for i := 0; i < count; i++ {
		for j := 0; j < count; j++ {
			if i == j {
				continue
			}

			result[i].Host.Peerstore().AddAddrs(result[j].Host.ID(), result[j].Host.Addrs(), peerstore.PermanentAddrTTL)
		}
	}

	return result
}

func checkEchoStatus(t *testing.T, e *Echo, expected EchoStatus) {
	t.Helper()

	status := e.Status()

	if status.StreamsIn != expected.StreamsIn {
		t.Fatalf("expected %d streams in, got %d", expected.StreamsIn, status.StreamsIn)
	}
	if status.EchosIn != expected.EchosIn {
		t.Fatalf("expected %d echos in, got %d", expected.EchosIn, status.EchosIn)
	}
	if status.EchosOut != expected.EchosOut {
		t.Fatalf("expected %d echos out, got %d", expected.EchosOut, status.EchosOut)
	}
	if status.IOErrors != expected.IOErrors {
		t.Fatalf("expected %d I/O errors, got %d", expected.IOErrors, status.IOErrors)
	}
	if status.ResourceServiceErrors != expected.ResourceServiceErrors {
		t.Fatalf("expected %d service resource errors, got %d", expected.ResourceServiceErrors, status.ResourceServiceErrors)
	}
	if status.ResourceReservationErrors != expected.ResourceReservationErrors {
		t.Fatalf("expected %d reservation resource errors, got %d", expected.ResourceReservationErrors, status.ResourceReservationErrors)
	}
}

func TestEcho(t *testing.T) {
	echos := createEchos(t, 2)

	err := echos[0].Host.Connect(context.TODO(), peer.AddrInfo{ID: echos[1].Host.ID()})
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		for _, e := range echos {
			e.Host.Close()
		}
	}()

	if err = echos[0].Echo(echos[1].Host.ID(), "hello libp2p"); err != nil {
		t.Fatal(err)
	}

	checkEchoStatus(t, echos[1], EchoStatus{
		StreamsIn: 1,
		EchosIn:   1,
		EchosOut:  1,
	})
}
