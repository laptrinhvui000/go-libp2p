package itest

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	logging "github.com/ipfs/go-log/v2"
)

const (
	EchoService = "test.echo"
	EchoProtoID = "/test/echo"
)

var (
	echoLog = logging.Logger("echo")
)

type Echo struct {
	Host host.Host

	WaitBeforeRead, WaitBeforeWrite func() error

	mx     sync.Mutex
	status EchoStatus
}

type EchoStatus struct {
	StreamsIn                 int
	EchosIn, EchosOut         int
	IOErrors                  int
	ResourceServiceErrors     int
	ResourceReservationErrors int
}

func NewEcho(h host.Host) *Echo {
	e := &Echo{Host: h}
	h.SetStreamHandler(EchoProtoID, e.handleStream)
	return e
}

func (e *Echo) Status() EchoStatus {
	e.mx.Lock()
	defer e.mx.Unlock()

	return e.status
}

func (e *Echo) handleStream(s network.Stream) {
	defer s.Close()

	e.mx.Lock()
	e.status.StreamsIn++
	e.mx.Unlock()

	if err := s.Scope().SetService(EchoService); err != nil {
		echoLog.Debugf("error attaching stream to echo service: %s", err)

		e.mx.Lock()
		e.status.ResourceServiceErrors++
		e.mx.Unlock()

		s.Reset()
		return
	}

	if err := s.Scope().ReserveMemory(4096, network.ReservationPriorityAlways); err != nil {
		echoLog.Debugf("error reserving memory: %s", err)

		e.mx.Lock()
		e.status.ResourceReservationErrors++
		e.mx.Unlock()

		s.Reset()
		return
	}

	if e.WaitBeforeRead != nil {
		if err := e.WaitBeforeRead(); err != nil {
			echoLog.Debugf("error waiting before read: %s", err)

			s.Reset()
			return
		}
	}

	buf := make([]byte, 4096)

	s.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := s.Read(buf)
	switch {
	case err == io.EOF:
		if n == 0 {
			return
		}

	case err != nil:
		echoLog.Debugf("I/O error : %s", err)

		e.mx.Lock()
		e.status.IOErrors++
		e.mx.Unlock()

		s.Reset()
		return
	}

	e.mx.Lock()
	e.status.EchosIn++
	e.mx.Unlock()

	if e.WaitBeforeWrite != nil {
		if err := e.WaitBeforeWrite(); err != nil {
			echoLog.Debugf("error waiting before write: %s", err)

			s.Reset()
			return
		}
	}

	s.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = s.Write(buf[:n])
	if err != nil {
		echoLog.Debugf("I/O error: %s", err)

		e.mx.Lock()
		e.status.IOErrors++
		e.mx.Unlock()

		s.Reset()
		return
	}

	e.mx.Lock()
	e.status.EchosOut++
	e.mx.Unlock()

	s.CloseWrite()
}

func (e *Echo) Echo(p peer.ID, what string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	s, err := e.Host.NewStream(ctx, p, EchoProtoID)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Scope().SetService(EchoService); err != nil {
		echoLog.Debugf("error attaching stream to echo service: %s", err)

		s.Reset()
		return err
	}

	if err := s.Scope().ReserveMemory(4096, network.ReservationPriorityAlways); err != nil {
		echoLog.Debugf("error reserving memory: %s", err)

		s.Reset()
		return err
	}

	s.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = s.Write([]byte(what))
	if err != nil {
		return err
	}
	s.CloseWrite()

	buf := make([]byte, 4096)

	s.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := s.Read(buf)
	switch {
	case err == io.EOF:
		if n == 0 {
			return err
		}

	case err != nil:
		echoLog.Debugf("I/O error : %s", err)

		s.Reset()
		return err
	}

	if what != string(buf[:n]) {
		return fmt.Errorf("echo output doesn't match input")
	}

	return nil
}
