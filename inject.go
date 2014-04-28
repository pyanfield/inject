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
	// 返回当前 refelct.Type 所映射的 reflect.Value
	Get(reflect.Type) reflect.Value
}

type injector struct {
	// 保存注入的参数
	values map[reflect.Type]reflect.Value
	parent Injector
}

// InterfaceOf dereferences a pointer to an Interface type.
// It panics if value is not an pointer to an interface.
// 主要是用于获取参数类型
// value 必须是接口类型的指针，如果不是将引发 panic
func InterfaceOf(value interface{}) reflect.Type {
	// 返回 value 得 reflection type
	t := reflect.TypeOf(value)

	// 返回一个代表类型得常量
	// Kind 返回得是最底层类型
	// 	type MyInt int
	//  var x MyInt = 7
	//  Kind 返回得就是int,而如果用 Type 返回得则是 MyInt
	for t.Kind() == reflect.Ptr {
		// 因为 t 为一个指针类型，为了得到 t 真正得指向的底层类型
		t = t.Elem()
	}

	if t.Kind() != reflect.Interface {
		panic("Called inject.InterfaceOf with a value that is not a pointer to an interface. (*MyInterface)(nil)")
	}

	return t
}

// New returns a new Injector.
// 初始化 injector 结构体，返回一个指向 injector 结构体的指针，这个指针被 Injector 接口包装了。
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
// 主要是用于执行函数 f,f 的底层类型必须为 Func
func (inj *injector) Invoke(f interface{}) ([]reflect.Value, error) {
	t := reflect.TypeOf(f)
	// 创建一个参数数组
	// NumIn() 返回函数的参数个数，如果 t 不是 Func 类型的话，将 Panic
	var in = make([]reflect.Value, t.NumIn()) //Panic if t is not kind of Func
	for i := 0; i < t.NumIn(); i++ {
		// 返回第 i 个参数的类型
		argType := t.In(i)
		// 根据参数的Type ，去获得Value值
		val := inj.Get(argType)
		// 判断 Value 是否为有效值
		if !val.IsValid() {
			return nil, fmt.Errorf("Value not found for type %v", argType)
		}

		in[i] = val
	}
	// 调用函数，返回函数返回值
	return reflect.ValueOf(f).Call(in), nil
}

// Maps dependencies in the Type map to each field in the struct
// that is tagged with 'inject'.
// Returns an error if the injection fails.
// 将结构体中的标记为 'inject' 的字段值更新成新的结构体中的值
// 主要作用是注入 struct
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
		// 该结构体字段是可导出字段，且该字段的 tag 是 `inject` 或者不为空
		// 则检查当前的结构体中的字段的 reflect.Type 和 reflect.Value 映射表
		// 为对应的类型注入新的值
		if f.CanSet() && (structField.Tag == "inject" || structField.Tag.Get("inject") != "") {
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
// 将当前 val 的类型和值映射表注册到当前的 TypeMapper 中
// Map 和 MapTo 主要是用于注入参数
func (i *injector) Map(val interface{}) TypeMapper {
	i.values[reflect.TypeOf(val)] = reflect.ValueOf(val)
	return i
}

// ifacePtr 必须是一个接口指针类型，否则 InterfaceOf 的时候会 panic
func (i *injector) MapTo(val interface{}, ifacePtr interface{}) TypeMapper {
	i.values[InterfaceOf(ifacePtr)] = reflect.ValueOf(val)
	return i
}

// Maps the given reflect.Type to the given reflect.Value and returns
// the Typemapper the mapping has been registered in.
// 给当前的 reflect.Type 赋新的 reflect.Value 值
// 将 val 值重新映射到 injector 的Type,Value对应关系中
func (i *injector) Set(typ reflect.Type, val reflect.Value) TypeMapper {
	i.values[typ] = val
	return i
}

// 获取注入的参数
func (i *injector) Get(t reflect.Type) reflect.Value {
	val := i.values[t]
	// 判断 Value是否是零值，如果是零值则返回false.
	// 如果其有父类，则去检测父类的 reflect.Value
	if val.IsValid() {
		return val
	}

	// no concrete types found, try to find implementors
	// if t is an interface
	// 如果不是具体的值，则判断是否是 Interface 类型，是否与 t 有相同的接口
	if t.Kind() == reflect.Interface {
		for k, v := range i.values {
			if k.Implements(t) {
				val = v
				break
			}
		}
	}

	// Still no type found, try to look it up on the parent
	if !val.IsValid() && i.parent != nil {
		val = i.parent.Get(t)
	}

	return val

}

// 设置父 injector， 查找继承
func (i *injector) SetParent(parent Injector) {
	i.parent = parent
}
