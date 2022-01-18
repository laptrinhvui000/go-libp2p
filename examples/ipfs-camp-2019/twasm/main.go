package main

import (
	"syscall/js"
)

var state interface{}

func testPrint() js.Func {
	jsonFunc := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) != 1 {
			return "Invalid no of arguments passed"
		}
		state = args[0].String()
		println("input %s", state)
		js.Global().Set("goState", state)

		return true
	})
	return jsonFunc
}

func main() {
	println("Go Web Assembly")
	js.Global().Set("testPrint", testPrint())	

	<-make(chan bool)
}