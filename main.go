package main

import "syscall/js"

func main() {
	document := js.Global().Get("document")
	body := document.Get("body")

	h1 := document.Call("createElement", "h1")
	h1.Set("textContent", "Hello World")

	body.Call("appendChild", h1)

	// Keep the program running
	select {}
}
