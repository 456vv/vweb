package vweb

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ForMethod 遍历并返回方法信息
func ForMethod(x any) string {
	if x == nil {
		return "<nil>"
	}
	t := reflect.TypeOf(x)
	var sb strings.Builder

	// 如果是指针，获取其指向的类型的方法
	numMethod := t.NumMethod()
	for i := 0; i < numMethod; i++ {
		m := t.Method(i)
		// 索引 | 包路径 | 方法名 | 类型
		fmt.Fprintf(&sb, "[%d] %s.%s \tType: %v\n", m.Index, m.PkgPath, m.Name, m.Type)
	}
	return sb.String()
}

// ForType 遍历字段
// x: 目标对象
// showUnexported: 是否展示小写（未导出）字段
// maxDepth: 最大递归深度
func ForType(x any, showUnexported bool, maxDepth int) string {
	if x == nil {
		return "<nil>"
	}
	var sb strings.Builder
	visited := make(map[uintptr]bool) // 防止循环引用
	val := reflect.ValueOf(x)
	inspectType(val, 0, showUnexported, maxDepth, &sb, visited)
	return sb.String()
}

func inspectType(v reflect.Value, floor int, lower bool, depth int, sb *strings.Builder, visited map[uintptr]bool) {
	if floor >= depth && depth != -1 {
		return
	}

	// 解引用指针和接口
	rv := v
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			fmt.Fprintf(sb, "%s <nil>\n", strings.Repeat("\t", floor))
			return
		}

		// 记录指针地址，防止循环引用
		if rv.Kind() == reflect.Pointer {
			addr := rv.Pointer()
			if visited[addr] {
				fmt.Fprintf(sb, "%s <Circular Reference>\n", strings.Repeat("\t", floor))
				return
			}
			visited[addr] = true
		}
		rv = rv.Elem()
	}

	rt := rv.Type()
	indent := strings.Repeat("\t", floor)

	switch rv.Kind() {
	case reflect.Map:
		if rv.CanInterface() && (rv.Type().Elem().Kind() == reflect.Struct || rv.Type().Elem().Kind() == reflect.Pointer) {
			fmt.Fprintf(sb, "%s L:%d Type:%v\n", indent, rv.Len(), rt)
			for _, key := range rv.MapKeys() {
				if key.CanInterface() {
					fmt.Fprintf(sb, "%s [%#v]:", indent, key.Interface())
				} else {
					fmt.Fprintf(sb, "%s [%#v]:", indent, key.String())
				}
				inspectType(rv.MapIndex(key), floor+1, lower, depth, sb, visited)
			}
		}
	case reflect.Slice, reflect.Array:
		if rv.CanInterface() && (rv.Type().Elem().Kind() == reflect.Struct || rv.Type().Elem().Kind() == reflect.Pointer) {
			fmt.Fprintf(sb, "%s L:%d Type:%v\n", indent, rv.Len(), rt)
			for i := 0; i < rv.Len(); i++ {
				fmt.Fprintf(sb, "%s [%d]:", indent, i)
				inspectType(rv.Index(i), floor+1, lower, depth, sb, visited)
			}
		}
	case reflect.Struct:
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)

			// 导出判断优化：PkgPath 不为空表示是私有（未导出）字段
			isExported := field.PkgPath == ""
			if !isExported && !lower {
				continue
			}

			fv := rv.Field(i)
			var valStr string
			var comment string

			if fv.CanInterface() {
				if c := byteSliceComment(fv); c != "" {
					comment = c
				}
				valStr = fmt.Sprintf("%#v", fv.Interface())
			} else {
				valStr = fmt.Sprintf("%#v", fv.String())
			}

			fmt.Fprintf(sb, "%s %d: %-10s %-15v Tag:`%s` = %s %s\n", indent, i, field.Name, field.Type, field.Tag, valStr, comment)

			// 递归处理子结构
			if (fv.Kind() == reflect.Struct || fv.Kind() == reflect.Pointer || fv.Kind() == reflect.Slice || fv.Kind() == reflect.Map) && (floor+1 < depth || depth == -1) {
				inspectType(fv, floor+1, lower, depth, sb, visited)
			}
		}

	default:
		// 基本类型处理
		if v.CanInterface() {
			valStr := fmt.Sprintf("%#v", v.Interface())
			if c := byteSliceComment(v); c != "" {
				valStr += " " + c
			}
			fmt.Fprintf(sb, "%s %v\n", indent, valStr)
		} else {
			fmt.Fprintf(sb, "%s %v\n", indent, v.String())
		}
	}
}

// isPrintableUTF8 判断字节串是否为可打印 UTF-8。
// 允许 \t \n \r 等常见空白；其余控制字符返回 false。
func isPrintableUTF8(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, r := range string(b) {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// byteSliceComment 为 []byte 生成可读注释：
//   - 空：// String:
//   - 全部可打印：// String: xxx
//   - 含不可打印/非法字符：// Hex: 01000000
func byteSliceComment(v reflect.Value) string {
	if v.Kind() != reflect.Slice || v.Type().Elem().Kind() != reflect.Uint8 {
		return ""
	}
	if !v.CanInterface() { // 未导出字段的 Bytes() 会 panic
		return ""
	}
	bz := v.Bytes()
	if len(bz) == 0 {
		return "// String: "
	}
	if isPrintableUTF8(bz) {
		return "// String: " + string(bz)
	}
	return "// Hex: " + hex.EncodeToString(bz)
}
