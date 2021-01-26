package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/bytecodealliance/wasmtime-go"
)

func readWasm(engine *wasmtime.Engine, file string) (module *wasmtime.Module, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	hasher := sha256.New()
	b, err := ioutil.ReadAll(io.TeeReader(f, hasher))
	if err != nil {
		return nil, err
	}
	hashString := fmt.Sprintf("%x", hasher.Sum(nil))
	if b, err := ioutil.ReadFile("./.cache/" + hashString); err == nil {
		then := time.Now()
		defer func() { fmt.Println("deserialize time", time.Since(then)) }()
		return wasmtime.NewModuleDeserialize(engine, b)
	}
	module, err = wasmtime.NewModule(engine, b)
	if err != nil {
		return nil, err
	}
	b, err = module.Serialize()
	if err != nil {
		return nil, err
	}
	return module, ioutil.WriteFile("./.cache/"+hashString, b, 0655)
}

func main() {
	// Almost all operations in wasmtime require a contextual `store`
	// argument to share, so create that first
	store := wasmtime.NewStore(wasmtime.NewEngine())

	then := time.Now()
	module, err := readWasm(store.Engine, "hello.wasm")
	check(err)
	fmt.Println("comp time", time.Since(then))
	debug := true
	instance, err := NewGoJSInstance(store, module, debug)
	check(err)
	// for name := range imports {
	// 	wasmtime.NewFuncType([]*wasmtime.ValType{wasmtime.KindI32}, results []*wasmtime.ValType)
	// }
	// After we've instantiated we can lookup our `run` function and call
	// it.
	run := instance.instance.GetExport("run").Func()
	_, err = run.Call(0, 0)
	check(err)
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
