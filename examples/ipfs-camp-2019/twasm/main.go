package main

import (
	"syscall/js"
)

// function definition
func add(this js.Value, i []js.Value) interface{} {
	return js.ValueOf(i[0].Int()+i[1].Int())
	}  

func testPrint(this js.Value, i []js.Value) interface{}{
	println(js.ValueOf(i[0]).String())
	return js.ValueOf(i[0])
}

func registerCallbacks() {
	js.Global().Set("add", js.FuncOf(add))
	js.Global().Set("testPrint", js.FuncOf(testPrint))
}

func main() {
    c := make(chan struct{}, 0)

    println("WASM Go Initialized")
    // register functions
    registerCallbacks()
    <-c
}