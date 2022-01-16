package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"	
)

func main() {

	n := NewP2P()
	fmt.Println(n.Host.ID())

	topic, err := n.PubSub.Join(pubsubTopic)
	if err != nil {
		panic(err)
	}
	defer topic.Close()
	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	go pubsubHandler(n.Ctx, sub)

	donec := make(chan struct{}, 1)
	go chatInputLoop(n.Ctx, n.Host, topic, donec)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		n.Host.Close()
		os.Exit(0)
	case <-donec:
		n.Host.Close()
	}
}