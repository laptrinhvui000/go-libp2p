package main

import (
	"syscall/js"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	
)

var (
	node *P2P
    topic *pubsub.Topic
	err error
)

func init(){
	println("INIT")
	node = NewP2P()
	println(node.Host.ID())
}

func joinTopic() {
	topic, err = node.PubSub.Join(pubsubTopic)
	if err != nil {
		panic(err)
	}
	defer topic.Close()
	sub, err := topic.Subscribe()
	if err != nil {
		panic(err)
	}
	go pubsubHandler(node.Ctx, sub)

	

	// stop := make(chan os.Signal, 1)
	// signal.Notify(stop, syscall.SIGINT)

	// select {
	// case <-stop:
	// 	n.Host.Close()
	// 	os.Exit(0)
	// case <-donec:
	// 	n.Host.Close()
	// }
}


func testSub() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			return "Invalid no of arguments passed"
		}

		joinTopic()
		return true
	})
	return jsonFunc
}

func testSend() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			return "Invalid no of arguments passed"
		}
		state := args[0].String()
		println("input %s \n", state)
		// js.Global().Set("goState", state)
		donec := make(chan struct{}, 1)
		go chatInputLoop(node.Ctx, node.Host, topic, donec)

		return true
	})
	return jsonFunc
}

func main() {
	js.Global().Set("testSub", testSub())	

	js.Global().Set("testSend", testSend())	

	<-make(chan bool)
}