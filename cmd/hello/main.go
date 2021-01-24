package main

import (
	"fmt"
	"math/rand"
)

func main() {
	fmt.Println("Hello wasm")

	fmt.Println(rand.Intn(100))

	var foo Value = Bar{}
	var bar Value = Foo{}
	var bar2 Value = Bar{}
	fmt.Println(foo == bar, foo == bar2)

	var first interface{} = String("hi")
	var second interface{} = Symbol("hi")
	switch v := first.(type) {
	case Symbol:
		fmt.Println("symbol", v)
	case String:
		fmt.Println("string", v)
	}
	switch v := second.(type) {
	case Symbol:
		fmt.Println("symbol", v)
	case String:
		fmt.Println("string", v)
	}
}

type Value interface{}

type Foo struct{}

type Bar struct{}

type String string
type Symbol string
