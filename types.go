package main

import "fmt"

type Value interface{}

// null , undefined , strings, numbers, boolean, and symbols
type Null struct{}
type Undefined struct{}

type String string
type Number int64
type Float float64
type Bool bool
type Symbol string
type Array []Value

type Object struct {
	attributes  map[Value]Value
	name        string
	constructor Function
}

type NaN struct{}

// bytes.math.NaN()

var True = Bool(true)
var False = Bool(false)

type Function func(...Value) Value

var Global = &Object{
	name: "global",
	attributes: map[Value]Value{
		"Object": &Object{},
		"Array": &Object{
			constructor: func(values ...Value) Value {
				if len(values) == 0 {
					return Array{}
				}
				if num, ok := values[0].(Number); ok && len(values) == 1 {
					return make(Array, num)
				}
				return Array(values)
			},
		},
		"fs": &Object{
			attributes: map[Value]Value{
				"writeSync": Function(func(v ...Value) Value {
					fmt.Println("writeSync")
					return Undefined{}
				}),
				"constants": &Object{
					attributes: map[Value]Value{
						"O_WRONLY": -1, "O_RDWR": -1, "O_CREAT": -1, "O_TRUNC": -1, "O_APPEND": -1, "O_EXCL": -1,
					},
				},
			},
		},
		"Uint8Array": &Object{},
	},
}

var This = &Object{
	name: "this",
	attributes: map[Value]Value{
		"_idPool": Array{},
		"exited":  False,
		"_values": Array{
			NaN{}, Number(0), Null{}, True, False, Global,
		},
		"_ids": &Object{
			attributes: map[Value]Value{
				Number(0): Number(1),
				Null{}:    Number(2),
				True:      Number(3),
				False:     Number(4),
				Global:    Number(5),
			},
		},
		"_goRefCounts": Array{},
	},
}

func init() {
	// can't recursively initialize
	This.attributes["_values"] = append(This.attributes["_values"].(Array), This)
	This.attributes["_ids"].(*Object).attributes[This] = 6
}

// 	"_idPool": Array{},
// 	"exited":  False,
// }
