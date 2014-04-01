// Package inject provides utilities for mapping and injecting dependencies in various ways.
// inject 包主要是提供了多种方式的公用映射和依赖注入
package inject

import (
	"fmt"
	"reflect"
)

// Injector represents an interface for mapping and injecting dependencies into structs
// and function arguments.
type Injector interface {
	Applicator
	Invoker
	TypeMapper
	// SetParent sets the parent of the injector. If the injector cannot find a
	// dependency in its Type map it will check its parent before returning an
	// error.
	// SetParent 主要是设置 injector的父类，如果injector在它得 Type map中找不到依赖对象，
	// 那么就去它得父类中查找，否则返回一个 error
	SetParent(Injector)
}

// Applicator represents an interface for mapping dependencies to a struct.
type Applicator interface {
	// Maps dependencies in the Type map to each field in the struct
	// that is tagged with 'inject'. Returns an error if the injection
	// fails.
	Apply(interface{}) error
}

// Invoker represents an interface for calling functions via reflection.
type Invoker interface {
	// Invoke attempts to call the interface{} provided as a function,
	// providing dependencies for function arguments based on Type. Returns
	// a slice of reflect.Value representing the returned values of the function.
	// Returns an error if the injection fails.
	Invoke(interface{}) ([]reflect.Value, error)
}

// TypeMapper represents an interface for mapping interface{} values based on type.
type TypeMapper interface {
	// Maps the interface{} value based on its immediate type from reflect.TypeOf.
	Map(interface{}) TypeMapper
	// Maps the interface{} value based on the pointer of an Interface provided.
	// This is really only useful for mapping a value as an interface, as interfaces
	// cannot at this time be referenced directly without a pointer.
	MapTo(interface{}, interface{}) TypeMapper
	// Provides a possibility to directly insert a mapping based on type and value.
	// This makes it possible to directly map type arguments not possible to instantiate
	// with reflect like unidirectional channels.
	Set(reflect.Type, reflect.Value) TypeMapper
	// Returns the Value that is mapped to the current type. Returns a zeroed Value if
	// the Type has not been mapped.
	Get(reflect.Type) reflect.Value
}

type injector struct {
	values map[reflect.Type]reflect.Value
	parent Injector
}

// InterfaceOf dereferences a pointer to an Interface type.
// It panics if value is not an pointer to an interface.
// InterfaceOf 解引用一个指针，使其指向一个 Interface 类型
// 如果 value 不是一个指向 interface 得指针，将 panic
func InterfaceOf(value interface{}) reflect.Type {
	// 返回 value 得 reflection type
	t := reflect.TypeOf(value)

	// 返回一个代表类型得常量
	// Kind 返回得是最根本得类型
	// 	type MyInt int
	//  var x MyInt = 7
	//  Kind 返回得就是int,而如果用 Type 返回得则是 MyInt
	for t.Kind() == reflect.Ptr {
		// 因为 t 为一个指针类型，为了得到 t 真正得指向得值
		t = t.Elem()
	}

	if t.Kind() != reflect.Interface {
		panic("Called inject.InterfaceOf with a value that is not a pointer to an interface. (*MyInterface)(nil)")
	}

	return t
}

// New returns a new Injector.
func New() Injector {
	return &injector{
		values: make(map[reflect.Type]reflect.Value),
	}
}

// Invoke attempts to call the interface{} provided as a function,
// providing dependencies for function arguments based on Type.
// Returns a slice of reflect.Value representing the returned values of the function.
// Returns an error if the injection fails.
// It panics if f is not a function
func (inj *injector) Invoke(f interface{}) ([]reflect.Value, error) {
	t := reflect.TypeOf(f)

	var in = make([]reflect.Value, t.NumIn()) //Panic if t is not kind of Func
	for i := 0; i < t.NumIn(); i++ {
		argType := t.In(i)
		val := inj.Get(argType)
		if !val.IsValid() {
			return nil, fmt.Errorf("Value not found for type %v", argType)
		}

		in[i] = val
	}

	return reflect.ValueOf(f).Call(in), nil
}

// Maps dependencies in the Type map to each field in the struct
// that is tagged with 'inject'.
// Returns an error if the injection fails.
func (inj *injector) Apply(val interface{}) error {
	v := reflect.ValueOf(val)

	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil // Should not panic here ?
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		// 返回结构体中字段得 Value 类型
		f := v.Field(i)
		// 返回结构体内字段得 StructField 描述
		structField := t.Field(i)
		// 如果结构体中 field 值可以设置，如果该字段得 structField 描述的 tag 是 inject
		//
		if f.CanSet() && structField.Tag == "inject" {
			ft := f.Type()
			v := inj.Get(ft)
			if !v.IsValid() {
				return fmt.Errorf("Value not found for type %v", ft)
			}

			f.Set(v)
		}

	}

	return nil
}

// Maps the concrete value of val to its dynamic type using reflect.TypeOf,
// It returns the TypeMapper registered in.
func (i *injector) Map(val interface{}) TypeMapper {
	i.values[reflect.TypeOf(val)] = reflect.ValueOf(val)
	return i
}

func (i *injector) MapTo(val interface{}, ifacePtr interface{}) TypeMapper {
	i.values[InterfaceOf(ifacePtr)] = reflect.ValueOf(val)
	return i
}

// Maps the given reflect.Type to the given reflect.Value and returns
// the Typemapper the mapping has been registered in.
func (i *injector) Set(typ reflect.Type, val reflect.Value) TypeMapper {
	i.values[typ] = val
	return i
}

func (i *injector) Get(t reflect.Type) reflect.Value {
	val := i.values[t]
	if !val.IsValid() && i.parent != nil {
		val = i.parent.Get(t)
	}
	return val
}

func (i *injector) SetParent(parent Injector) {
	i.parent = parent
}
