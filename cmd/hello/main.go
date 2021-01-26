package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
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
	f, err := os.Open("go.mod")
	if err != nil {
		panic(err)
	}
	b, err := ioutil.ReadAll(f)
	fmt.Print(string(b), err)
}

type Value interface{}

type Foo struct{}

type Bar struct{}

type String string
type Symbol string
