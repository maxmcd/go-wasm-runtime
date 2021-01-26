package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	runtimedebug "runtime/debug"
	"syscall"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/davecgh/go-spew/spew"
)

type Instance struct {
	instance *wasmtime.Instance
	mem      []byte
	this     *Object
	global   *Object
}

func NewGoJSInstance(store *wasmtime.Store, module *wasmtime.Module, debug bool) (*Instance, error) {
	i := &Instance{}
	funcMap := map[string]func(int32){
		"runtime.wasmExit":              i.runtimeWasmExit,
		"runtime.wasmWrite":             i.runtimeWasmWrite,
		"runtime.resetMemoryDataView":   i.runtimeResetMemoryDataView,
		"runtime.nanotime1":             i.runtimeNanotime1,
		"runtime.walltime1":             i.runtimeWalltime1,
		"runtime.scheduleTimeoutEvent":  i.runtimeScheduleTimeoutEvent,
		"runtime.clearTimeoutEvent":     i.runtimeClearTimeoutEvent,
		"runtime.getRandomData":         i.runtimeGetRandomData,
		"syscall/js.finalizeRef":        i.syscallJSFinalizeRef,
		"syscall/js.stringVal":          i.syscallJSStringVal,
		"syscall/js.valueGet":           i.syscallJSValueGet,
		"syscall/js.valueSet":           i.syscallJSValueSet,
		"syscall/js.valueDelete":        i.syscallJSValueDelete,
		"syscall/js.valueIndex":         i.syscallJSValueIndex,
		"syscall/js.valueSetIndex":      i.syscallJSValueSetIndex,
		"syscall/js.valueCall":          i.syscallJSValueCall,
		"syscall/js.valueInvoke":        i.syscallJSValueInvoke,
		"syscall/js.valueNew":           i.syscallJSValueNew,
		"syscall/js.valueLength":        i.syscallJSValueLength,
		"syscall/js.valuePrepareString": i.syscallJSValuePrepareString,
		"syscall/js.valueLoadString":    i.syscallJSValueLoadString,
		"syscall/js.valueInstanceOf":    i.syscallJSValueInstanceOf,
		"syscall/js.copyBytesToGo":      i.syscallJSCopyBytesToGo,
		"syscall/js.copyBytesToJS":      i.syscallJSCopyBytesToJS,
		"debug":                         i.debug,
	}
	importList := []*wasmtime.Extern{}
	for _, imprt := range module.Imports() {
		name := *imprt.Name()
		importFunc := funcMap[name]
		importList = append(importList,
			wasmtime.WrapFunc(store, func(sp int32) {
				if debug && name != "runtime.wasmWrite" {
					fmt.Printf("[called %s]\n", name)
				}
				defer func() {
					if r := recover(); r != nil {
						fmt.Println("Panic recovered in", name, r, string(runtimedebug.Stack()))
						panic(r)
					}
				}()
				importFunc(sp)
			}).AsExtern(),
		)
	}
	i.global = &Object{
		Attributes: map[interface{}]interface{}{
			"Object": &Object{},
			"Array": &Object{
				Constructor: NewFunction(func(values ...interface{}) interface{} {
					return NewArray(values...)
				}),
			},
			"fs": &Object{
				Attributes: map[interface{}]interface{}{
					"writeSync": NewFunction(func(v ...interface{}) interface{} {
						fmt.Println("writeSync")
						return Undefined
					}),
					"open": NewFunction(func(args ...interface{}) interface{} {
						spew.Dump(args)
						return Undefined
					}),
					"write": NewFunction(func(v ...interface{}) interface{} {
						fmt.Println("---------write----------")
						spew.Dump(v)
						fd := v[0].(float64)
						buf := *v[1].(*[]byte)
						_ = fd
						(*v[5].(*Function))(nil, len(buf))
						return Undefined
					}),
					"constants": &Object{
						Attributes: map[interface{}]interface{}{
							"O_WRONLY": syscall.O_WRONLY,
							"O_RDWR":   syscall.O_RDWR,
							"O_CREAT":  syscall.O_CREAT,
							"O_TRUNC":  syscall.O_TRUNC,
							"O_APPEND": syscall.O_APPEND,
							"O_EXCL":   syscall.O_EXCL,
						},
					},
				},
			},
			"Uint8Array": &Object{
				Constructor: NewFunction(func(v ...interface{}) interface{} {
					fmt.Println("Uint8Array()", spew.Sdump(v))
					if len(v) == 0 {
						return []byte{}
					}
					if vi, ok := v[0].(float64); ok {
						o := make([]byte, int(vi))
						return &o
					}
					if vi, ok := v[0].(int); ok {
						o := make([]byte, vi)
						return &o
					}
					panic(fmt.Sprint("UINT8 unsupported", v))
				}),
			},
		}}
	i.this = &Object{
		Attributes: map[interface{}]interface{}{
			"_idPool":       NewArray(),
			"exited":        false,
			"_pendingEvent": nil,
			"_values":       NewArray(math.NaN(), float64(0), nil, true, false, i.global),
			"_ids": &Object{
				Attributes: map[interface{}]interface{}{
					float64(0): 1,
					nil:        2,
					true:       3,
					false:      4,
					i.global:   5,
				},
			},
			"_goRefCounts": NewArray(),
			"_resume": NewFunction(func(args ...interface{}) interface{} {
				// instance.exports.resume()
				return nil
			}),
			"_makeFuncWrapper": NewFunction(func(args ...interface{}) interface{} {
				return NewFunction(func(a ...interface{}) interface{} {
					id := args[0]
					i.this.Attributes["_pendingEvent"] = &Object{
						Attributes: map[interface{}]interface{}{
							"id":   id,
							"this": nil, // TODO
							"args": NewArray(a...),
						},
					}
					i.instance.GetExport("resume").Func().Call()
					return nil
				})
			}),
		}}

	// TODO: does this need to be here?
	i.this.Attributes["_values"].(*Array).Append(i.this)
	i.this.Attributes["_ids"].(*Object).Attributes[i.this] = 6

	var err error
	i.instance, err = wasmtime.NewInstance(store, module, importList)
	if err != nil {
		return nil, err
	}
	i.mem = i.instance.GetExport("mem").Memory().UnsafeData()
	return i, nil
}

func (i *Instance) getSP() int32 {
	spi, err := i.instance.GetExport("getsp").Func().Call()
	if err != nil {
		panic(err)
	}
	return spi.(int32)
}

func (i *Instance) getUint32(sp int32) uint32 {
	return binary.LittleEndian.Uint32(i.mem[int(sp) : int(sp)+4])
}
func (i *Instance) getInt64(sp int32) int64 {
	low := i.getUint32(sp)
	high := i.getUint32(sp + 4)
	return int64(low) + int64(high)*4294967296
}
func (i *Instance) getFloat64(sp int32) float64 {
	bits := binary.LittleEndian.Uint64(i.mem[int(sp) : int(sp)+8])
	return math.Float64frombits(bits)
}
func (i *Instance) setUint32(sp int32, v uint32) {
	binary.LittleEndian.PutUint32(i.mem[sp:sp+4], v)
}
func (i *Instance) setInt64(addr int32, v int64) {
	i.setUint32(addr, uint32(v))
	i.setUint32(addr+4, uint32(math.Floor(float64(v)/float64(4294967296))))
}
func (i *Instance) setFloat64(sp int32, v float64) {
	binary.LittleEndian.PutUint64(i.mem[sp:sp+8], math.Float64bits(v))
}
func (i *Instance) loadValue(addr int32) interface{} {
	f := i.getFloat64(addr)
	if f == 0 {
		return nil
	}
	if math.Trunc(f) == f {
		return f
	}
	id := i.getUint32(addr)
	return i.this.Attributes["_values"].(*Array).Get(int(id))
}
func (i *Instance) loadSlice(addr int32) []byte {
	array := i.getInt64(addr)
	len := i.getInt64(addr + 8)
	return i.mem[array : array+len]
}
func (i *Instance) loadSliceOfValues(addr int32) *Array {
	array := i.getInt64(addr + 0)
	len := i.getInt64(addr + 8)
	defer fmt.Println("loadSliceOfValues", array, len)
	a := NewArray(int(len))
	for j := int64(0); j < len; j++ {
		a.Set(int(j), i.loadValue(int32(array+j*8)))
	}
	return a
}
func (i *Instance) storeValue(addr int32, val interface{}) {
	var nanHead uint32 = 0x7FF80000

	if num, ok := val.(int); ok && num != 0 {
		val = float64(num)
	}
	if num, ok := val.(uint); ok && num != 0 {
		val = float64(num)
	}

	if float, ok := val.(float64); ok && float != 0 {
		if math.IsNaN(float) {
			i.setUint32(addr+4, nanHead)
			i.setUint32(addr, 0)
			return
		}
		i.setFloat64(addr, float64(float))
		return
	}
	if val == Undefined {
		i.setFloat64(addr, 0)
		return
	}

	var id int
	idValue, found := i.this.Attributes["_ids"].(*Object).Attributes[val]
	refCounts := i.this.Attributes["_goRefCounts"].(*Array)
	if !found {
		idPool := i.this.Attributes["_idPool"].(*Array)
		values := i.this.Attributes["_values"].(*Array)
		if idPool.Len() == 0 {
			id = values.Len()
		} else {
			id = idPool.Pop().(int)
		}
		values.Set(id, val)
		refCounts.Set(id, 0)
		i.this.Attributes["_ids"].(*Object).Attributes[val] = id
	} else {
		id = idValue.(int)
	}
	refCounts.Increment(id)

	var typeFlag uint32
	switch val.(type) {
	case *Object, *Array, []byte:
		typeFlag = 1
	case string:
		typeFlag = 2
	case Symbol:
		typeFlag = 3
	case func(...interface{}) interface{}:
		typeFlag = 4
	}
	i.setUint32(addr+4, nanHead|typeFlag)
	i.setUint32(addr, uint32(id))
}
func (i *Instance) loadString(sp int32) string {
	saddr := i.getInt64(sp + 0)
	len := i.getInt64(sp + 8)
	return string(i.mem[saddr : saddr+len])
}

func (i *Instance) runtimeWasmExit(sp int32) {

}
func (i *Instance) runtimeWasmWrite(sp int32) {
	fd := i.getInt64(sp + 8)
	p := i.getInt64(sp + 16)
	n := int32(i.getUint32(sp + 24))
	var writer io.Writer
	switch fd {
	case 1:
		writer = os.Stdout
	case 2:
		writer = os.Stderr
	default:
		panic(fd)
	}
	fmt.Fprint(writer, string(i.mem[int(p):int(p)+int(n)]))
}
func (i *Instance) runtimeResetMemoryDataView(sp int32) {

}
func (i *Instance) runtimeNanotime1(sp int32) {

}
func (i *Instance) runtimeWalltime1(sp int32) {

}
func (i *Instance) runtimeScheduleTimeoutEvent(sp int32) {

}
func (i *Instance) runtimeClearTimeoutEvent(sp int32) {

}
func (i *Instance) runtimeGetRandomData(sp int32) {

}
func (i *Instance) syscallJSFinalizeRef(sp int32) {

}
func (i *Instance) syscallJSStringVal(sp int32) {
	i.storeValue(sp+24, i.loadString(sp+8))
}
func (i *Instance) syscallJSValueGet(sp int32) {
	obj := i.loadValue(sp + 8)
	attr := i.loadString(sp + 16)
	val := obj.(*Object).Attributes[attr]
	i.storeValue(sp+32, val)
	fmt.Println("syscall/js.valueGet", obj, attr)
}
func (i *Instance) syscallJSValueSet(sp int32) {

}
func (i *Instance) syscallJSValueDelete(sp int32) {

}
func (i *Instance) syscallJSValueIndex(sp int32) {
	indexable := i.loadValue(sp + 8).(*Array)
	index := int(i.getInt64(sp + 16))
	fmt.Println(len(indexable.values), index)
	if index >= len(indexable.values) {
		i.storeValue(sp+24, Undefined)
	} else {
		i.storeValue(sp+24, indexable.Get(index))
	}
}
func (i *Instance) syscallJSValueSetIndex(sp int32) {

}
func (i *Instance) syscallJSValueCall(sp int32) {
	v := i.loadValue(sp + 8)
	attr := i.loadString(sp + 16)
	fmt.Println("syscall/js.valueCall", v, attr)
	m := v.(*Object).Attributes[attr]
	args := i.loadSliceOfValues(sp + 32)
	fn, ok := m.(*Function)
	if !ok {
		fmt.Println("NOT A FUNCTION")
		return
	}
	result := fn.Call(args.values...)
	sp = i.getSP()
	i.storeValue(sp+56, result)
	i.mem[sp+64] = 1
}
func (i *Instance) syscallJSValueNew(sp int32) {
	v := i.loadValue(sp + 8)
	args := i.loadSliceOfValues(sp + 16)
	fmt.Println("valueNew", v, spew.Sdump(args))
	result := v.(*Object).Constructor.Call(args.values...)
	sp = i.getSP()
	i.storeValue(sp+40, result)
	i.mem[sp+48] = 1
}

func (i *Instance) syscallJSCopyBytesToJS(sp int32) {
	dst := i.loadValue(sp + 8)
	src := i.loadSlice(sp + 16)
	byteVal, ok := dst.(*[]byte)
	if !ok {
		i.mem[sp+48] = 0
		return
	}
	toCopy := src[:len(src)+len(*byteVal)]
	copy(*byteVal, toCopy)
	i.setInt64(sp+40, int64(len(toCopy)))
	i.mem[sp+48] = 1
}
func (i *Instance) syscallJSValueInvoke(sp int32) {
}
func (i *Instance) syscallJSValueLength(sp int32) {
	len := i.loadValue(sp + 8).(*Array).Len()
	i.setInt64(sp+16, int64(len))
}
func (i *Instance) syscallJSValuePrepareString(sp int32) {

}
func (i *Instance) syscallJSValueLoadString(sp int32) {

}
func (i *Instance) syscallJSValueInstanceOf(sp int32) {

}
func (i *Instance) syscallJSCopyBytesToGo(sp int32) {

}
func (i *Instance) debug(sp int32) {
	fmt.Println(sp)
}

// null , undefined , strings, numbers, boolean, and symbols
var Undefined = &struct{}{}

type Object struct {
	Attributes  map[interface{}]interface{}
	Constructor *Function
}
type Symbol string
type Array struct {
	values []interface{}
}

func (a *Array) Pop() (out interface{}) {
	out, a.values = a.values[len(a.values)-1], a.values[:len(a.values)-1]
	return out
}
func (a *Array) Len() int {
	return len(a.values)
}
func (a *Array) Set(idx int, v interface{}) {
	for a.Len() <= idx {
		a.values = append(a.values, Undefined)
	}
	a.values[idx] = v
}
func (a *Array) Get(idx int) interface{} {
	return a.values[idx]
}
func (a *Array) Increment(idx int) {
	if v, ok := a.values[idx].(int); ok {
		a.values[idx] = v + 1
	}
}
func (a *Array) Append(val interface{}) {
	a.values = append(a.values, val)
}

func NewArray(values ...interface{}) *Array {
	if len(values) == 0 {
		return &Array{}
	}
	if num, ok := values[0].(int); ok && len(values) == 1 {
		v := make([]interface{}, num)
		return &Array{values: v}
	}
	return &Array{values: values}
}

type Function func(...interface{}) interface{}

func (f *Function) Call(vs ...interface{}) interface{} {
	return (*f)(vs...)
}

func NewFunction(fn func(...interface{}) interface{}) *Function {
	v := Function(fn)
	return &v
}
