package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"time"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/davecgh/go-spew/spew"
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
	var memoryBuf []byte

	getUint32 := func(sp int32) uint32 {
		return binary.LittleEndian.Uint32(memoryBuf[int(sp) : int(sp)+4])
	}
	getInt64 := func(sp int32) int64 {
		low := getUint32(sp)
		high := getUint32(sp + 4)
		return int64(low) + int64(high)*4294967296
	}
	getFloat64 := func(sp int32) float64 {
		bits := binary.LittleEndian.Uint64(memoryBuf[int(sp) : int(sp)+8])
		return math.Float64frombits(bits)
	}
	setUint32 := func(sp int32, v uint32) {
		binary.LittleEndian.PutUint32(memoryBuf[sp:sp+4], v)
	}
	setFloat64 := func(sp int32, v float64) {
		binary.LittleEndian.PutUint64(memoryBuf[sp:sp+8], math.Float64bits(v))
	}
	loadValue := func(addr int32) interface{} {
		f := getFloat64(addr)
		if f == 0 {
			return nil
		}
		if math.Trunc(f) == f {
			return f
		}
		id := getUint32(addr)
		return This.attributes["_values"].(Array)[id]
	}
	storeValue := func(addr int32, val Value) {
		var nanHead uint32 = 0x7FF80000

		if _, isNaN := val.(NaN); isNaN {
			setUint32(addr+4, nanHead)
			setUint32(addr, 0)
			return
		}

		if num, ok := val.(Number); ok && num != 0 {
			setFloat64(addr, float64(num))
			return
		}
		if num, ok := val.(int); ok && num != 0 {
			setFloat64(addr, float64(num))
			return
		}
		if float, ok := val.(Float); ok && float != 0 {
			setFloat64(addr, float64(float))
			return
		}
		if _, ok := val.(Undefined); ok {
			setFloat64(addr, 0)
			return
		}

		var id Number
		idValue, found := This.attributes["_ids"].(*Object).attributes[val]
		if !found {
			p := This.attributes["_idPool"].(Array)
			if len(p) == 0 {
				id = Number(len(This.attributes["_values"].(Array)))
			} else {
				id, This.attributes["_idPool"] = Number(p[len(p)-1].(Number)), p[:len(p)-1]
			}
			fmt.Println("-----------", id)
			for Number(len(This.attributes["_values"].(Array))) <= id {
				This.attributes["_values"] = append(This.attributes["_values"].(Array), Undefined{})
			}
			This.attributes["_values"].(Array)[id] = val
			for Number(len(This.attributes["_goRefCounts"].(Array))) <= id {
				This.attributes["_goRefCounts"] = append(This.attributes["_goRefCounts"].(Array), Undefined{})
			}
			This.attributes["_goRefCounts"].(Array)[id] = Number(0)
			fmt.Println("----------------", id)
			This.attributes["_ids"].(*Object).attributes[val] = id
		} else {
			id := int(idValue.(Number))
			fmt.Println("\\//\\//\\//\\//\\//\\///\\//\\//")
			spew.Dump(val, id)
		}
		if _, isNumber := This.attributes["_goRefCounts"].(Array)[id].(Number); isNumber {
			This.attributes["_goRefCounts"].(Array)[id] = This.attributes["_goRefCounts"].(Array)[id].(Number) + 1
		}
		spew.Dump(This.attributes["_goRefCounts"].(Array))

		var typeFlag uint32
		switch val.(type) {
		case *Object, Array:
			fmt.Println("it's an object!", id)
			typeFlag = 1
		case String, string:
			typeFlag = 2
		case Symbol:
			typeFlag = 3
		case Function:
			typeFlag = 4
		}
		setUint32(addr+4, nanHead|typeFlag)
		setUint32(addr, uint32(id))
	}
	loadString := func(sp int32) string {
		saddr := getInt64(sp + 0)
		len := getInt64(sp + 8)
		return string(memoryBuf[saddr : saddr+len])
	}
	imports := map[string]func(int32){
		"runtime.wasmExit": func(int32) {
			fmt.Println("runtime.wasmExit")
		},
		"runtime.wasmWrite": func(sp int32) {
			fd := getInt64(sp + 8)
			p := getInt64(sp + 16)
			n := int32(getUint32(sp + 24))
			var writer io.Writer
			switch fd {
			case 1:
				writer = os.Stdout
			case 2:
				writer = os.Stderr
			default:
				panic(fd)
			}
			fmt.Fprint(writer, string(memoryBuf[int(p):int(p)+int(n)]))
			// const fd = getInt64(sp + 8)
			// const p = getInt64(sp + 16)
			// const n = this.mem.getInt32(sp+24, true)
		},
		"runtime.resetMemoryDataView": func(sp int32) {
			fmt.Println("runtime.resetMemoryDataView")
		},
		"runtime.nanotime1": func(sp int32) {
			fmt.Println("runtime.nanotime1")
		},
		"runtime.walltime1": func(sp int32) {
			fmt.Println("runtime.walltime1")
		},
		"runtime.scheduleTimeoutEvent": func(sp int32) {
			fmt.Println("runtime.scheduleTimeoutEvent")
		},
		"runtime.clearTimeoutEvent": func(sp int32) {
			fmt.Println("runtime.clearTimeoutEvent")
		},
		"runtime.getRandomData": func(sp int32) {
			fmt.Println("runtime.getRandomData")
		},
		"syscall/js.finalizeRef": func(sp int32) {
			fmt.Println("syscall/js.finalizeRef")
		},
		"syscall/js.stringVal": func(sp int32) {
			fmt.Println("syscall/js.stringVal")
		},
		"syscall/js.valueGet": func(sp int32) {
			obj := loadValue(sp + 8)
			attr := loadString(sp + 16)
			val := obj.(*Object).attributes[attr]
			storeValue(sp+32, val)
			fmt.Println("syscall/js.valueGet", obj, attr)
		},
		"syscall/js.valueSet": func(sp int32) {
			fmt.Println("syscall/js.valueSet")
		},
		"syscall/js.valueDelete": func(sp int32) {
			fmt.Println("syscall/js.valueDelete")
		},
		"syscall/js.valueIndex": func(sp int32) {
			fmt.Println("syscall/js.valueIndex")
		},
		"syscall/js.valueSetIndex": func(sp int32) {
			fmt.Println("syscall/js.valueSetIndex")
		},
		"syscall/js.valueCall": func(int32) {
			fmt.Println("syscall/js.valueCall")
		},
		"syscall/js.valueInvoke": func(int32) {
			fmt.Println("syscall/js.valueInvoke")
		},
		"syscall/js.valueNew": func(int32) {
			fmt.Println("syscall/js.valueNew")
		},
		"syscall/js.valueLength": func(int32) {
			fmt.Println("syscall/js.valueLength")
		},
		"syscall/js.valuePrepareString": func(int32) {
			fmt.Println("syscall/js.valuePrepareString")
		},
		"syscall/js.valueLoadString": func(int32) {
			fmt.Println("syscall/js.valueLoadString")
		},
		"syscall/js.valueInstanceOf": func(int32) {
			fmt.Println("syscall/js.valueInstanceOf")
		},
		"syscall/js.copyBytesToGo": func(int32) {
			fmt.Println("syscall/js.copyBytesToGo")
		},
		"syscall/js.copyBytesToJS": func(int32) {
			fmt.Println("syscall/js.copyBytesToJS")
		},
		"debug": func(i int32) {
			fmt.Println("debug", i)
		},
	}
	importList := []*wasmtime.Extern{}
	for _, imprt := range module.Imports() {
		importList = append(importList,
			wasmtime.WrapFunc(store, imports[*imprt.Name()]).AsExtern(),
		)
	}

	// Next up we instantiate a module which is where we link in all our
	// imports. We've got one import so we pass that in here.
	instance, err := wasmtime.NewInstance(store, module, importList)
	memory := instance.GetExport("mem").Memory()
	memoryBuf = memory.UnsafeData()
	check(err)
	_ = imports
	// for name := range imports {
	// 	wasmtime.NewFuncType([]*wasmtime.ValType{wasmtime.KindI32}, results []*wasmtime.ValType)
	// }
	// After we've instantiated we can lookup our `run` function and call
	// it.
	run := instance.GetExport("run").Func()
	_, err = run.Call(0, 0)
	check(err)
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}
