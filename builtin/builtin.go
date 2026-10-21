package builtin

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

var (
	ErrNilValue        = errors.New("value is nil")
	ErrUnsupported     = errors.New("unsupported type")
	ErrKeyType         = errors.New("invalid key type")
	ErrIndexOutOfRange = errors.New("index out of range")
	ErrFieldNotFound   = errors.New("field not found")
	ErrMapKey          = errors.New("map key type mismatch")
	ErrInvalidArgCount = errors.New("invalid argument count: Set(obj, key1, val1, key2, val2, ...)")
	ErrUnaddressable   = errors.New("value is unaddressable or field is unexported; pass a pointer to exported field")
	ErrNilMap          = errors.New("cannot set on nil map")
	ErrImmutable       = errors.New("immutable type (string/number) cannot be modified by index")

	errNotInt   = errors.New("not integer")
	errIntRange = errors.New("out of range")
)

const (
	syntheticPkgPath = "builtin"
)

var (
	zeroVal    reflect.Value
	builtinMu  sync.RWMutex
	implFuncTy = reflect.TypeFor[func([]reflect.Value) []reflect.Value]()
)

// maxFieldCache 限制字段索引缓存条目，避免动态 StructOf 把 reflect.Type 永久钉住。
const maxFieldCache int64 = 4096

type indexCache struct {
	m sync.Map
	n atomic.Int64
}

var fieldCachePtr atomic.Pointer[indexCache]

func fieldCacheBox() *indexCache {
	if c := fieldCachePtr.Load(); c != nil {
		return c
	}
	c := &indexCache{}
	if fieldCachePtr.CompareAndSwap(nil, c) {
		return c
	}
	return fieldCachePtr.Load()
}

// ---------------------------------------------------------------------------
// unsafe 导出支持（运行时探测，兼容 Wasm / AppEngine / TinyGo 等）
// ---------------------------------------------------------------------------
// flagRO = flagStickyRO | flagEmbedRO = 1<<5 | 1<<6，自 Go 1.5 起稳定。
const flagRO uintptr = 96

type valueHeader struct {
	typ  unsafe.Pointer
	ptr  unsafe.Pointer
	flag uintptr
}

var useUnsafeExport atomic.Bool

func unsafeExportEnabled() bool { return useUnsafeExport.Load() }

func makeExported(v reflect.Value) (out reflect.Value) {
	out = v
	if !v.IsValid() {
		return v
	}
	vh := (*valueHeader)(unsafe.Pointer(&v))
	old := vh.flag
	defer func() {
		if recover() != nil {
			// panic 时恢复 flag，避免把半修改的 Value 交回调用方。
			vh.flag = old
			out = v
		}
	}()
	vh.flag &^= flagRO
	return v
}

func init() {
	// Go 若给 reflect.Value 加字段，按三字头清 flag 会写坏内存。尺寸不符直接禁用。
	if unsafe.Sizeof(reflect.Value{}) != unsafe.Sizeof(valueHeader{}) {
		useUnsafeExport.Store(false)
		return
	}
	defer func() {
		if recover() != nil {
			useUnsafeExport.Store(false)
		}
	}()
	type dummy struct{ x int }
	v := reflect.ValueOf(dummy{x: 42}).FieldByName("x")
	if v.IsValid() && !v.CanInterface() {
		ev := makeExported(v)
		if ev.CanInterface() && ev.Interface() == 42 {
			useUnsafeExport.Store(true)
		}
	}
}

// Register adds or replaces a named type in the registry.
// Safe for concurrent use.
func Register(name string, t reflect.Type) {
	if name == "" || t == nil {
		return
	}
	builtinMu.Lock()
	builtinTypes[name] = t
	builtinMu.Unlock()
}

// Value returns a zero value of the type described by v.
//
// Accepted forms for v:
//   - string type name ("int", "string", "[]int", "map[string]int", "*float64", …)
//   - reflect.Type
//   - reflect.Value (uses its Type)
//   - any concrete value (uses TypeOf)
//
// The returned Value is never a pointer unless the requested type itself is a
// pointer type.
func Value(v any) reflect.Value {
	t, err := Type(v)
	if err != nil {
		if v == nil {
			return reflect.Value{}
		}
		if _, isStr := v.(string); isStr {
			return reflect.Value{}
		}
		rt := reflect.TypeOf(v)
		if rt == nil {
			return reflect.Value{}
		}
		return reflect.New(rt).Elem()
	}
	return reflect.Zero(t)
}

// New 返回 *T 的可设置指针（类似内置 new）。
func New(typ any) any {
	t, err := Type(typ)
	if err != nil {
		return nil
	}
	return reflect.New(t).Interface()
}

// Type returns the reflect.Type described by v.
func Type(v any) (reflect.Type, error) {
	switch x := v.(type) {
	case string:
		return parseType(x)
	case reflect.Type:
		return x, nil
	case reflect.Value:
		if !x.IsValid() {
			return nil, fmt.Errorf("invalid reflect.Value")
		}
		return x.Type(), nil
	default:
		if v == nil {
			return reflect.TypeFor[any](), nil
		}
		return reflect.TypeOf(v), nil
	}
}

// ---------------------------------------------------------------------------
// Type parser
// ---------------------------------------------------------------------------

// parseType understands a small Go-like type syntax:
//
//	T
//	*T
//	[]T
//	[N]T          (N must be a non-negative integer literal)
//	map[K]V
//	chan T / <-chan T / chan<- T
//	func(T) R
//
// Nested combinations are supported (e.g. "map[string][]*int").
func parseType(s string) (reflect.Type, error) {
	return parseTypeDepth(s, 0)
}

func parseTypeDepth(s string, depth int) (reflect.Type, error) {
	if depth > maxConvertDepth {
		return nil, fmt.Errorf("type nesting limit exceeded")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty type string")
	}
	if isWrappedByParens(s) {
		return parseTypeDepth(strings.TrimSpace(s[1:len(s)-1]), depth+1)
	}
	if rest, ok := strings.CutPrefix(s, "*"); ok {
		elem, err := parseTypeDepth(strings.TrimSpace(rest), depth+1)
		if err != nil {
			return nil, err
		}
		return safeReflectType(func() reflect.Type { return reflect.PointerTo(elem) })
	}
	if rest, ok := strings.CutPrefix(s, "[]"); ok {
		elem, err := parseTypeDepth(strings.TrimSpace(rest), depth+1)
		if err != nil {
			return nil, err
		}
		return safeReflectType(func() reflect.Type { return reflect.SliceOf(elem) })
	}
	if strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return nil, fmt.Errorf("malformed array type: %q", s)
		}
		nStr := strings.TrimSpace(s[1:end])
		n, err := parseNonNegInt(nStr)
		if err != nil {
			return nil, fmt.Errorf("invalid array length %q: %w", nStr, err)
		}
		elem, err := parseTypeDepth(strings.TrimSpace(s[end+1:]), depth+1)
		if err != nil {
			return nil, err
		}
		return safeReflectType(func() reflect.Type { return reflect.ArrayOf(n, elem) })
	}
	if strings.HasPrefix(s, "map[") {
		// s[3:] 从 '[' 起，与 parseBraceGroup 一样走 parseBalancedGroup。
		inner, rest, err := parseBalancedGroup(s[3:], '[', ']')
		if err != nil {
			return nil, fmt.Errorf("malformed map type: %q", s)
		}
		key, err := parseTypeDepth(strings.TrimSpace(inner), depth+1)
		if err != nil {
			return nil, err
		}
		val, err := parseTypeDepth(strings.TrimSpace(rest), depth+1)
		if err != nil {
			return nil, err
		}
		return safeReflectType(func() reflect.Type { return reflect.MapOf(key, val) })
	}
	if st, handled, err := parseStructType(s, depth); handled {
		return st, err
	}
	if it, handled, err := parseInterfaceType(s, depth); handled {
		return it, err
	}
	if ct, handled, err := parseChanType(s, depth); handled {
		return ct, err
	}
	if ft, handled, err := parseFuncType(s, depth); handled {
		return ft, err
	}
	builtinMu.RLock()
	t, ok := builtinTypes[s]
	builtinMu.RUnlock()
	if ok {
		return t, nil
	}
	return nil, fmt.Errorf("unknown type %q", s)
}

func parseInterfaceType(s string, depth int) (reflect.Type, bool, error) {
	if !strings.HasPrefix(s, "interface") {
		return nil, false, nil
	}
	rest := strings.TrimSpace(s[len("interface"):])
	if rest == "" || rest[0] != '{' {
		return nil, false, nil
	}
	body, after, err := parseBraceGroup(rest)
	if err != nil {
		return nil, true, err
	}
	if strings.TrimSpace(after) != "" {
		return nil, true, fmt.Errorf("malformed interface type: trailing %q", after)
	}
	if strings.TrimSpace(body) != "" {
		return nil, true, fmt.Errorf("non-empty interface types are not supported")
	}
	_ = depth
	return reflect.TypeFor[any](), true, nil
}

func isWrappedByParens(s string) bool {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return false
	}
	depth := 0
	for i := range len(s) {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
			if depth == 0 && i != len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

// keywordBoundary 报告 s[:n] 是否为完整标识符，而不是更长名字的前缀。
// 修改原因：chan/func/struct/interface 若只用 HasPrefix，channel、change、function 会被当成类型关键字。
func keywordBoundary(s string, n int) bool {
	if len(s) < n {
		return false
	}
	if len(s) == n {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[n:])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
}

func parseChanType(s string, depth int) (reflect.Type, bool, error) {
	if rest, ok := strings.CutPrefix(s, "<-"); ok {
		rest = strings.TrimSpace(rest)
		if !strings.HasPrefix(rest, "chan") {
			return nil, false, nil
		}
		// <-chanint 的 rest[4:] 为 "int"，会被解析成 <-chan int。
		if !keywordBoundary(rest, len("chan")) {
			return nil, false, nil
		}
		elem, err := parseTypeDepth(strings.TrimSpace(rest[4:]), depth+1)
		if err != nil {
			return nil, true, err
		}
		t, err := safeReflectType(func() reflect.Type { return reflect.ChanOf(reflect.RecvDir, elem) })
		return t, true, err
	}
	if !strings.HasPrefix(s, "chan") {
		return nil, false, nil
	}
	rest := s[4:]
	if rest == "" {
		return nil, false, nil
	}
	restTrim := strings.TrimSpace(rest)
	if elemSrc, ok := strings.CutPrefix(restTrim, "<-"); ok {
		elem, err := parseTypeDepth(strings.TrimSpace(elemSrc), depth+1)
		if err != nil {
			return nil, true, err
		}
		t, err := safeReflectType(func() reflect.Type { return reflect.ChanOf(reflect.SendDir, elem) })
		return t, true, err
	}
	switch rest[0] {
	case ' ', '\t', '\n', '\r', '*', '[', '(', '<':
	default:
		return nil, false, nil
	}
	elem, err := parseTypeDepth(strings.TrimSpace(rest), depth+1)
	if err != nil {
		return nil, true, err
	}
	t, err := safeReflectType(func() reflect.Type { return reflect.ChanOf(reflect.BothDir, elem) })
	return t, true, err
}

func parseFuncType(s string, depth int) (reflect.Type, bool, error) {
	if !strings.HasPrefix(s, "func") {
		return nil, false, nil
	}
	rest := strings.TrimSpace(s[4:])
	if rest == "" || rest[0] != '(' {
		return nil, false, nil
	}
	args, after, err := parseParenGroup(rest)
	if err != nil {
		return nil, true, err
	}
	in, variadic, err := parseParamList(args, true, depth+1)
	if err != nil {
		return nil, true, err
	}
	after = strings.TrimSpace(after)
	var out []reflect.Type
	if after == "" {
		t, err := safeReflectType(func() reflect.Type { return reflect.FuncOf(in, out, variadic) })
		return t, true, err
	}
	if strings.HasPrefix(after, "(") {
		group, tail, err := parseParenGroup(after)
		if err != nil {
			return nil, true, err
		}
		if strings.TrimSpace(tail) != "" {
			return nil, true, fmt.Errorf("malformed func type: %q", s)
		}
		out, _, err = parseParamList(group, false, depth+1)
		if err != nil {
			return nil, true, err
		}
		t, err := safeReflectType(func() reflect.Type { return reflect.FuncOf(in, out, variadic) })
		return t, true, err
	}
	rt, err := parseTypeDepth(after, depth+1)
	if err != nil {
		return nil, true, err
	}
	t, err := safeReflectType(func() reflect.Type { return reflect.FuncOf(in, []reflect.Type{rt}, variadic) })
	return t, true, err
}

func parseParenGroup(s string) (inner, rest string, err error) {
	if s == "" || s[0] != '(' {
		return "", s, fmt.Errorf("expected '('")
	}
	return parseBalancedGroup(s, '(', ')')
}

func parseParamList(s string, allowVariadic bool, depth int) ([]reflect.Type, bool, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false, nil
	}
	parts, err := splitTopLevel(s, ',')
	if err != nil {
		return nil, false, err
	}
	out := make([]reflect.Type, 0, len(parts))
	variadic := false
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, false, fmt.Errorf("empty parameter type")
		}
		p = stripOptionalParamName(p)
		if strings.HasPrefix(p, "...") {
			if !allowVariadic || i != len(parts)-1 {
				return nil, false, fmt.Errorf("misplaced ... in %q", s)
			}
			variadic = true
			p = strings.TrimSpace(p[3:])
			if p == "" {
				return nil, false, fmt.Errorf("missing variadic element type")
			}
		}
		t, err := parseTypeDepth(p, depth)
		if err != nil {
			return nil, false, err
		}
		if variadic && i == len(parts)-1 {
			t = reflect.SliceOf(t) // func(...T) 的最后一个 In 必须是 []T
		}
		out = append(out, t)
	}
	return out, variadic, nil
}

func stripOptionalParamName(p string) string {
	if looksLikeTypeStart(p) {
		return p
	}
	_, rest, ok := cutGoIdent(p)
	if !ok {
		return p
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return p
	}
	return rest
}

func looksLikeTypeStart(s string) bool {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, "*"),
		strings.HasPrefix(s, "["),
		strings.HasPrefix(s, "<-"),
		strings.HasPrefix(s, "("),
		strings.HasPrefix(s, "..."),
		strings.HasPrefix(s, "map["):
		return true
	case strings.HasPrefix(s, "chan") && keywordBoundary(s, len("chan")),
		strings.HasPrefix(s, "func") && keywordBoundary(s, len("func")),
		strings.HasPrefix(s, "struct") && keywordBoundary(s, len("struct")),
		strings.HasPrefix(s, "interface") && keywordBoundary(s, len("interface")):
		return true
	default:
		return false
	}
}

func splitTopLevel(s string, sep byte) ([]string, error) {
	var parts []string
	depth := 0
	start := 0
	inBacktick := false
	inQuote := byte(0)
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inBacktick {
			if c == '`' {
				inBacktick = false
			}
			continue
		}
		if inQuote != 0 {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '`':
			inBacktick = true
		case '"', '\'':
			inQuote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced brackets in %q", s)
			}
		default:
			if c == sep && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced brackets in %q", s)
	}
	if inBacktick || inQuote != 0 {
		return nil, fmt.Errorf("unclosed quote in %q", s)
	}
	parts = append(parts, s[start:])
	return parts, nil
}

// parseStructType 解析 struct{...} 语法。
// 支持：
//
//	struct{}
//	struct{ I int }
//	struct{ A int; B string }
//	struct{ int }                 // 嵌入字段
//	struct{ Name string `json:"name"` }
//
// 字段之间用分号分隔，允许尾随分号，允许空白。
func parseStructType(s string, depth int) (reflect.Type, bool, error) {
	if !strings.HasPrefix(s, "struct") {
		return nil, false, nil
	}
	rest := strings.TrimSpace(s[6:])
	if rest == "" || rest[0] != '{' {
		return nil, false, nil
	}
	body, after, err := parseBraceGroup(rest)
	if err != nil {
		return nil, true, err
	}
	if strings.TrimSpace(after) != "" {
		return nil, true, fmt.Errorf("malformed struct type: trailing %q", after)
	}
	fields, err := parseStructFields(body, depth+1)
	if err != nil {
		return nil, true, err
	}
	t, err := safeReflectType(func() reflect.Type { return reflect.StructOf(fields) })
	return t, true, err
}

func parseBraceGroup(s string) (inner, rest string, err error) {
	if s == "" || s[0] != '{' {
		return "", s, fmt.Errorf("expected '{'")
	}
	return parseBalancedGroup(s, '{', '}')
}

func parseBalancedGroup(s string, open, close byte) (inner, rest string, err error) {
	depth := 0
	inBacktick := false
	inQuote := byte(0)
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inBacktick {
			if c == '`' {
				inBacktick = false
			}
			continue
		}
		if inQuote != 0 {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '`':
			inBacktick = true
		case '"', '\'':
			inQuote = c
		default:
			if c == open {
				depth++
			} else if c == close {
				depth--
				if depth == 0 {
					return s[1:i], s[i+1:], nil
				}
			}
		}
	}
	return "", s, fmt.Errorf("unbalanced %q in %q", open, s)
}

// parseStructFields 解析结构体字段列表。
func parseStructFields(s string, depth int) ([]reflect.StructField, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts, err := splitTopLevel(s, ';')
	if err != nil {
		return nil, err
	}
	var fields []reflect.StructField
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		sf, err := parseOneStructField(p, depth)
		if err != nil {
			return nil, err
		}
		fields = append(fields, sf)
	}
	return fields, nil
}

func splitFieldNameType(s string, depth int) (name, typeStr string, anonymous bool, err error) {
	if _, e := parseTypeDepth(s, depth); e == nil {
		return "", s, true, nil
	}
	ident, rest, ok := cutGoIdent(s)
	if !ok {
		return "", "", false, fmt.Errorf("malformed struct field %q", s)
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", "", false, fmt.Errorf("unknown field type %q", s)
	}
	return ident, rest, false, nil
}

func splitStructTag(s string) (body, tag string, err error) {
	s = strings.TrimSpace(s)
	if s == "" || s[len(s)-1] != '`' {
		return s, "", nil
	}
	depth := 0
	inQuote := byte(0)
	escape := false
	open := -1
	for i := 0; i < len(s)-1; i++ {
		c := s[i]
		if inQuote != 0 {
			if escape {
				escape = false
				continue
			}
			if c == '\\' && inQuote == '"' {
				escape = true
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inQuote = c
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		case '`':
			if depth == 0 {
				open = i
			}
		}
	}
	if open < 0 {
		return "", "", fmt.Errorf("malformed struct tag in %q", s)
	}
	return strings.TrimSpace(s[:open]), s[open+1 : len(s)-1], nil
}

func embeddedFieldName(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

func isExportedIdent(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

func isGoIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

func cutGoIdent(s string) (ident, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", s, false
	}
	n := 0
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return "", s, false
			}
			n = i + utf8.RuneLen(r)
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			n = i + utf8.RuneLen(r)
			continue
		}
		break
	}
	if n == 0 {
		return "", s, false
	}
	return s[:n], s[n:], true
}

// parseOneStructField 解析单个字段：
//
//	Name Type
//	Name Type `tag`
//	Type          // 嵌入字段
//	Type `tag`
func parseOneStructField(s string, depth int) (reflect.StructField, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return reflect.StructField{}, fmt.Errorf("empty struct field")
	}
	body, tag, err := splitStructTag(s)
	if err != nil {
		return reflect.StructField{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return reflect.StructField{}, fmt.Errorf("empty struct field after tag strip")
	}

	name, typeStr, anonymous, err := splitFieldNameType(body, depth)
	if err != nil {
		return reflect.StructField{}, err
	}
	t, err := parseTypeDepth(typeStr, depth)
	if err != nil {
		return reflect.StructField{}, fmt.Errorf("invalid field type %q: %w", typeStr, err)
	}
	if anonymous {
		name = embeddedFieldName(t)
		if name == "" {
			return reflect.StructField{}, fmt.Errorf("cannot embed unnamed type %s", t)
		}
	}
	if !isGoIdent(name) {
		return reflect.StructField{}, fmt.Errorf("invalid field name %q", name)
	}

	sf := reflect.StructField{
		Name:      name,
		Type:      t,
		Tag:       reflect.StructTag(tag),
		Anonymous: anonymous,
	}
	if !isExportedIdent(name) {
		sf.PkgPath = syntheticPkgPath
	}
	return sf, nil
}

func parseNonNegInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a non-negative integer")
		}
		d := int(c - '0')
		if n > (math.MaxInt-d)/10 {
			return 0, fmt.Errorf("integer overflow")
		}
		n = n*10 + d
	}
	return n, nil
}

// MustType is like Type but panics on error.
func MustType(v any) reflect.Type {
	t, err := Type(v)
	if err != nil {
		panic(err)
	}
	return t
}

// Panic(v)
func Panic(v any) {
	panic(v)
}

// Make 类似 Go 内置 make，支持：
//
//	Make([]T, length)           / Make([]T, length, capacity)
//	Make(map[K]V)               / Make(map[K]V, size)
//	Make(chan T)                / Make(chan T, buffer)
//	Make(func(...), impl)       // 扩展：MakeFunc 风格（可选）
//
// typ 可为：
//   - string 类型表达式（"[]int", "map[string]int", "chan bool" 等）
//   - reflect.Type
//   - 具体值（取其类型）
//
// 负长度/容量、非法参数个数、不支持的类型会 panic（与 Go make 更接近）。
func Make(typ any, args ...any) any {
	t, err := Type(typ)
	if err != nil {
		panic(fmt.Sprintf("make: %v", err))
	}
	defer func() {
		if r := recover(); r != nil {
			s := fmt.Sprint(r)
			if strings.HasPrefix(s, "make:") {
				panic(r)
			}
			panic("make: " + s)
		}
	}()
	switch t.Kind() {
	case reflect.Slice:
		return makeSlice(t, args...)
	case reflect.Map:
		return makeMap(t, args...)
	case reflect.Chan:
		return makeChan(t, args...)
	case reflect.Func:
		return makeFunc(t, args...)
	default:
		panic(fmt.Sprintf("make: cannot make %s", t))
	}
}

func checkArgCount(args []any, min, max int, missing string) {
	if len(args) < min {
		panic("make: missing " + missing)
	}
	if len(args) > max {
		panic("make: too many arguments")
	}
}

func makeSlice(t reflect.Type, args ...any) any {
	checkArgCount(args, 1, 2, "slice length")
	length := nonNegSize(args, 0, "slice length")
	capacity := length
	if len(args) > 1 {
		capacity = nonNegSize(args, 1, "slice capacity")
		if capacity < length {
			panic(fmt.Sprintf("make: cap out of range (cap=%d < len=%d)", capacity, length))
		}
	}
	return reflect.MakeSlice(t, length, capacity).Interface()
}

func makeMap(t reflect.Type, args ...any) any {
	checkArgCount(args, 0, 1, "map size")
	size := 0
	if len(args) > 0 {
		size = nonNegSize(args, 0, "map size")
	}
	return reflect.MakeMapWithSize(t, size).Interface()
}

func makeChan(t reflect.Type, args ...any) any {
	// 单向 channel 类型会让 reflect.MakeChan panic，错误信息不指向 Make。
	if t.ChanDir() != reflect.BothDir {
		panic("make: unidirectional channel type")
	}
	checkArgCount(args, 0, 1, "chan buffer")
	buffer := 0
	if len(args) > 0 {
		buffer = nonNegSize(args, 0, "chan buffer")
	}
	return reflect.MakeChan(t, buffer).Interface()
}

func makeFunc(t reflect.Type, args ...any) any {
	checkArgCount(args, 0, 1, "func implementation")
	if len(args) == 0 {
		return reflect.Zero(t).Interface()
	}
	fn, err := asMakeFuncImpl(args[0])
	if err != nil {
		panic(fmt.Sprintf("make: %v", err))
	}
	if fn == nil {
		return reflect.Zero(t).Interface()
	}
	return reflect.MakeFunc(t, wrapMakeFunc(t, fn)).Interface()
}

func asMakeFuncImpl(impl any) (func([]reflect.Value) []reflect.Value, error) {
	if impl == nil {
		return nil, nil
	}
	if fn, ok := impl.(func([]reflect.Value) []reflect.Value); ok {
		return fn, nil
	}
	rv := asValue(impl)
	if !rv.IsValid() {
		return nil, nil
	}
	if rv.Kind() != reflect.Func {
		return nil, fmt.Errorf("func implementation must be func([]reflect.Value) []reflect.Value, got %T", impl)
	}
	if rv.IsNil() {
		return nil, nil
	}
	if rv.Type() == implFuncTy {
		fn, _ := rv.Interface().(func([]reflect.Value) []reflect.Value)
		return fn, nil
	}
	if !rv.Type().ConvertibleTo(implFuncTy) {
		return nil, fmt.Errorf("func implementation must be func([]reflect.Value) []reflect.Value, got %s", rv.Type())
	}
	fn, _ := rv.Convert(implFuncTy).Interface().(func([]reflect.Value) []reflect.Value)
	return fn, nil
}

func wrapMakeFunc(t reflect.Type, fn func([]reflect.Value) []reflect.Value) func([]reflect.Value) []reflect.Value {
	nOut := t.NumOut()
	return func(in []reflect.Value) []reflect.Value {
		out := fn(in)
		if len(out) != nOut {
			panic(fmt.Sprintf("make: func implementation returned %d values, want %d", len(out), nOut))
		}
		if nOut == 0 {
			return out
		}
		fixed := make([]reflect.Value, nOut)
		changed := false
		for i, v := range out {
			want := t.Out(i)
			if !v.IsValid() {
				fixed[i] = reflect.Zero(want)
				changed = true
				continue
			}
			vt := v.Type()
			if vt == want || vt.AssignableTo(want) {
				fixed[i] = v
				continue
			}
			if v.CanConvert(want) {
				fixed[i] = v.Convert(want)
				changed = true
				continue
			}
			panic(fmt.Sprintf("make: cannot convert result %d from %s to %s", i, vt, want))
		}
		if !changed {
			return out
		}
		return fixed
	}
}

func safeReflectType(fn func() reflect.Type) (t reflect.Type, err error) {
	defer func() {
		if r := recover(); r != nil {
			t = nil
			err = fmt.Errorf("%v", r)
		}
	}()
	t = fn()
	if t == nil {
		return nil, fmt.Errorf("invalid type")
	}
	return t, nil
}

// nonNegSize 提取非负整数；负数或非法类型 panic（对齐 Go make 行为）
func nonNegSize(args []any, idx int, name string) int {
	if len(args) <= idx {
		return 0
	}
	n, err := convertToInt(args[idx])
	if err != nil {
		if err == errIntRange {
			panic(fmt.Sprintf("make: %s out of range", name))
		}
		panic(fmt.Sprintf("make: %s must be integer, got %T", name, args[idx]))
	}
	if n < 0 {
		panic(fmt.Sprintf("make: %s out of range", name))
	}
	return n
}

func toValue(v any) reflect.Value {
	rv := asValue(v)
	if !rv.IsValid() {
		return rv
	}
	if rv.Kind() == reflect.Pointer && !rv.IsNil() {
		return rv.Elem()
	}
	return rv
}

// Bool returns bool(a)
func Bool(a any) bool {
	switch a1 := a.(type) {
	case bool:
		return a1
	case nil:
		return false
	case string:
		return a1 != ""
	case int:
		return a1 != 0
	case int64:
		return a1 != 0
	case float64:
		return a1 != 0
	}
	return isTrue(inDirect(asValue(a)))
}

func Len(a any) int {
	if a == nil {
		return 0
	}
	if ch, ok := getChanValue(a); ok && ch.IsValid() && ch.Kind() == reflect.Chan {
		if ch.IsNil() {
			return 0
		}
		return ch.Len()
	}
	switch x := a.(type) {
	case string:
		return len(x)
	case []byte:
		return len(x)
	}
	v := inDirect(asValue(a))
	if !v.IsValid() {
		return 0
	}
	switch v.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len()
	}
	return 0
}

// Cap returns the capacity of a, or 0 if a is not an array, slice, or channel.
func Cap(a any) int {
	if a == nil {
		return 0
	}
	if ch, ok := getChanValue(a); ok && ch.IsValid() && ch.Kind() == reflect.Chan {
		if ch.IsNil() {
			return 0
		}
		return ch.Cap()
	}
	v := inDirect(asValue(a))
	if !v.IsValid() {
		return 0
	}
	// 处理 nil 指针指向数组的情况：容量在类型上，不依赖值
	if v.Kind() == reflect.Pointer && v.IsNil() {
		elem := v.Type().Elem()
		if elem.Kind() == reflect.Array {
			return elem.Len() // 数组 Cap == Len
		}
	}
	switch v.Kind() {
	case reflect.Array, reflect.Slice, reflect.Chan:
		return v.Cap()
	}
	return 0
}

// CopyStruct 浅复制：src 字段为空（nil 指针/slice/map 等）时不覆盖 dst。
func CopyStruct(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	return copyStruct(asValue(dsc), asValue(src), "", exclude, false, nil)
}

func destNeedsInit(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	}
	return false
}

func srcHasValue(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return !v.IsNil()
	}
	return true
}

// copyVisit 的 addr 用结构体自身地址。
// 结构体与其偏移 0 的第一个字段地址相同，但类型不同；只按地址去重会跳过整个首字段。
type copyVisit struct {
	addr uintptr
	typ  reflect.Type
}

// CopyStructDeep 深复制：src 字段为空也写入 dst（结果可为 nil）。
func CopyStructDeep(dsc, src any, exclude func(name string, dsc, src reflect.Value) bool) error {
	// 值改为目标 reflect.Value：碰到环时把指针指回已在拷贝的对象，而不是留零值。
	return copyStruct(asValue(dsc), asValue(src), "", exclude, true, make(map[copyVisit]reflect.Value))
}

// setVisitedPtr 在源间接后的结构体仍是当前祖先时，把可设置的指针字段指回已有目标。
// 返回 true 表示已接好环，调用方不要再 typeInit（否则会留下一个无关的空对象）。
func setVisitedPtr(field, src reflect.Value, visited map[copyVisit]reflect.Value) bool {
	if visited == nil || field.Kind() != reflect.Pointer || !field.CanSet() {
		return false
	}
	vb := inDirect(src)
	if !vb.IsValid() || !vb.CanAddr() || field.Type().Elem() != vb.Type() {
		return false
	}
	prev, ok := visited[copyVisit{addr: vb.UnsafeAddr(), typ: vb.Type()}]
	if !ok || !prev.IsValid() || !prev.CanAddr() || prev.Type() != vb.Type() {
		return false
	}
	field.Set(prev.Addr())
	return true
}

func trySet(dst, src reflect.Value) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reflect set: %v", r)
		}
	}()
	// Set 要求两侧都是 exported。目标可能已通过 makeSettable/NewAt 可写，
	// 源仍常是未导出字段 Value（flagRO）。对源清 RO，避免 mustBeExported panic。
	if src.IsValid() && !src.CanInterface() && unsafeExportEnabled() {
		src = makeExported(src)
	}
	dst.Set(src)
	return nil
}

func trySetMapIndex(mv, key, elem reflect.Value) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("reflect set map: %v", r)
		}
	}()

	mv.SetMapIndex(key, elem)
	return nil
}

func assignConverted(dst, src reflect.Value) error {
	if !dst.IsValid() || !dst.CanSet() {
		return nil
	}
	if !src.IsValid() {
		return trySet(dst, reflect.Zero(dst.Type()))
	}
	if src.Type().AssignableTo(dst.Type()) {
		return trySet(dst, src)
	}
	if cv, err := autoConvertReflect(dst.Type(), src); err == nil && cv.IsValid() && cv.Type().AssignableTo(dst.Type()) {
		return trySet(dst, cv)
	}
	if canConvertSafely(src, dst.Type()) {
		return trySet(dst, src.Convert(dst.Type()))
	}
	return fmt.Errorf("copy: cannot convert %s to %s", src.Type(), dst.Type())
}

// cloneElemInto 把 src 树拷贝进已可设置的 dst。
// channel/func 无法克隆，仍共享。visited 只保留祖先链，兄弟共享会各拷一份。
func cloneElemInto(dst, src reflect.Value, name string, exclude func(string, reflect.Value, reflect.Value) bool, visited map[copyVisit]reflect.Value, depth int) error {
	if depth > maxConvertDepth {
		return fmt.Errorf("copy: recursion limit exceeded")
	}
	if !dst.IsValid() {
		return fmt.Errorf("copy: invalid destination")
	}
	if !dst.CanSet() {
		if dst.CanAddr() {
			dst = makeSettable(dst, true)
		}
		if !dst.CanSet() {
			return nil
		}
	}
	if !src.IsValid() {
		return trySet(dst, reflect.Zero(dst.Type()))
	}
	switch dst.Kind() {
	case reflect.Pointer:
		return clonePtrInto(dst, src, name, exclude, visited, depth)
	case reflect.Interface:
		return cloneIfaceInto(dst, src, name, exclude, visited, depth)
	case reflect.Struct:
		sv := src
		if sv.Kind() == reflect.Pointer || sv.Kind() == reflect.Interface {
			if sv.IsNil() {
				return fmt.Errorf("copy: nil source for struct %s", name)
			}
			sv = sv.Elem()
		}
		if sv.Kind() == reflect.Struct {
			return copyStruct(dst, sv, name, exclude, true, visited)
		}
		return assignConverted(dst, src)
	case reflect.Slice:
		sv := src
		if sv.Kind() == reflect.Interface && !sv.IsNil() {
			sv = sv.Elem()
		}
		if sv.Kind() != reflect.Slice {
			return assignConverted(dst, src)
		}
		if sv.IsNil() {
			return trySet(dst, reflect.Zero(dst.Type()))
		}
		n := sv.Len()
		nv := reflect.MakeSlice(dst.Type(), n, n)
		for j := range n {
			if err := cloneElemInto(nv.Index(j), sv.Index(j), name, exclude, visited, depth+1); err != nil {
				return err
			}
		}
		return trySet(dst, nv)
	case reflect.Array:
		sv := src
		if sv.Kind() == reflect.Interface && !sv.IsNil() {
			sv = sv.Elem()
		}
		if sv.Kind() != reflect.Array || sv.Len() != dst.Len() {
			return assignConverted(dst, src)
		}
		for j := range dst.Len() {
			if err := cloneElemInto(dst.Index(j), sv.Index(j), name, exclude, visited, depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		sv := src
		if sv.Kind() == reflect.Interface && !sv.IsNil() {
			sv = sv.Elem()
		}
		if sv.Kind() != reflect.Map {
			return assignConverted(dst, src)
		}
		if sv.IsNil() {
			return trySet(dst, reflect.Zero(dst.Type()))
		}
		mv := reflect.MakeMapWithSize(dst.Type(), sv.Len())
		iter := sv.MapRange()
		for iter.Next() {
			k, err := autoConvertReflect(dst.Type().Key(), iter.Key())
			if err != nil {
				return err
			}
			slot := reflect.New(dst.Type().Elem()).Elem()
			if err := cloneElemInto(slot, iter.Value(), name, exclude, visited, depth+1); err != nil {
				return err
			}
			if err := trySetMapIndex(mv, k, slot); err != nil {
				return err
			}
		}
		return trySet(dst, mv)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return assignConverted(dst, src)
	default:
		return assignConverted(dst, src)
	}
}

func clonePtrInto(dst, src reflect.Value, name string, exclude func(string, reflect.Value, reflect.Value) bool, visited map[copyVisit]reflect.Value, depth int) error {
	if src.Kind() == reflect.Interface {
		if src.IsNil() {
			return trySet(dst, reflect.Zero(dst.Type()))
		}
		src = src.Elem()
	}
	if src.Kind() == reflect.Pointer {
		if src.IsNil() {
			return trySet(dst, reflect.Zero(dst.Type()))
		}
		if setVisitedPtr(dst, src, visited) {
			return nil
		}
		elem := src.Elem()
		if visited != nil && elem.CanAddr() {
			key := copyVisit{addr: elem.UnsafeAddr(), typ: elem.Type()}
			if prev, ok := visited[key]; ok && prev.IsValid() && prev.CanAddr() && prev.Type() == dst.Type().Elem() {
				return trySet(dst, prev.Addr())
			}
		}
		// 不在此登记 visited：copyStruct 进入时会按源地址登记。
		// 若在此先登记，随后 copyStruct 会命中同一 key 并直接 return，字段不会被拷贝。
		if dst.IsNil() {
			if err := trySet(dst, reflect.New(dst.Type().Elem())); err != nil {
				return err
			}
		}
		return cloneElemInto(dst.Elem(), elem, name, exclude, visited, depth+1)
	}
	if dst.IsNil() {
		if err := trySet(dst, reflect.New(dst.Type().Elem())); err != nil {
			return err
		}
	}
	return cloneElemInto(dst.Elem(), src, name, exclude, visited, depth+1)
}

func cloneIfaceInto(dst, src reflect.Value, name string, exclude func(string, reflect.Value, reflect.Value) bool, visited map[copyVisit]reflect.Value, depth int) error {
	if src.Kind() == reflect.Interface {
		if src.IsNil() {
			return trySet(dst, reflect.Zero(dst.Type()))
		}
		src = src.Elem()
	}
	if !src.IsValid() {
		return trySet(dst, reflect.Zero(dst.Type()))
	}
	box := reflect.New(src.Type()).Elem()
	if src.Type().AssignableTo(box.Type()) {
		if err := trySet(box, src); err != nil {
			return err
		}
	}
	if err := cloneElemInto(box, src, name, exclude, visited, depth+1); err != nil {
		return err
	}
	if !box.Type().AssignableTo(dst.Type()) {
		return fmt.Errorf("copy: %s not assignable to %s", box.Type(), dst.Type())
	}
	return trySet(dst, box)
}

func copyStruct(dsc, src reflect.Value, name string, exclude func(name string, dsc, src reflect.Value) bool, deep bool, visited map[copyVisit]reflect.Value) error {
	va := inDirect(dsc)
	vb := inDirect(src)
	if !va.IsValid() || !vb.IsValid() {
		return fmt.Errorf("无效的值，dsc(%v)，src(%v)", va.Kind(), vb.Kind())
	}
	if va.Kind() != vb.Kind() || va.Kind() != reflect.Struct {
		return fmt.Errorf("仅支持struct类型，dsc(%s)，src(%s)", va.Kind(), vb.Kind())
	}

	if deep && visited == nil {
		visited = make(map[copyVisit]reflect.Value)
	}

	// 必须先处理“接口内具体值不可设置”，再登记 visited。
	// 若先登记源地址，下面这次递归会命中同一地址并直接 return nil，深拷贝丢失。
	if !va.CanSet() && dsc.CanSet() {
		tmp := reflect.New(va.Type()).Elem()
		tmp.Set(va)
		if err := copyStruct(tmp, vb, name, exclude, deep, visited); err != nil {
			return err
		}
		if tmp.Type().AssignableTo(dsc.Type()) || dsc.Kind() == reflect.Interface {
			dsc.Set(tmp)
		}
		return nil
	}

	// 传入结构体值时字段都不可写，旧实现返回 nil，调用方以为已经拷贝成功。
	if !va.CanAddr() && !va.CanSet() {
		return fmt.Errorf("目标不可寻址，请传入指针。dsc(%s)", va.Kind())
	}

	// 只记录仍在栈上的源。返回时删除，因此兄弟字段指向同一对象会再拷一份（树拷贝）。
	// 键含类型：父结构体与偏移 0 的首字段地址相同，不能互相算作已访问。
	if visited != nil && vb.CanAddr() {
		key := copyVisit{addr: vb.UnsafeAddr(), typ: vb.Type()}
		if prev, ok := visited[key]; ok {
			// 源就是正在拷贝的祖先。指针字段改指向已有目标；值类型无法共享，停止以免死循环。
			if dsc.Kind() == reflect.Pointer && dsc.CanSet() && prev.IsValid() && prev.CanAddr() && dsc.Type().Elem() == prev.Type() {
				dsc.Set(prev.Addr())
			}
			return nil
		}
		visited[key] = va
		defer delete(visited, key)
	}

	bt := vb.Type()
	for i := range bt.NumField() {
		bvField := vb.Field(i)
		if !bvField.IsValid() {
			continue
		}
		info := bt.Field(i)
		fieldName := name + info.Name
		avField := va.FieldByName(info.Name)
		if exclude != nil && exclude(fieldName, avField, bvField) {
			continue
		}
		if !avField.IsValid() {
			continue
		}
		if !avField.CanSet() && avField.CanAddr() {
			// 跨包未导出但可寻址。NewAt 不依赖 flag 布局。
			avField = makeSettable(avField, true)
		}
		// 环：源指针指向祖先时直接接回目标，避免先分配一个空对象再跳过拷贝。
		if deep && setVisitedPtr(avField, bvField, visited) {
			continue
		}
		// 深拷贝若落到后面的 Set(bvField)，指针和接口会与源共享。
		if deep && (avField.Kind() == reflect.Pointer || avField.Kind() == reflect.Interface) {
			if err := cloneElemInto(avField, bvField, fieldName+".", exclude, visited, 0); err != nil {
				return err
			}
			continue
		}

		avfi := inDirect(avField)
		bvfi := inDirect(bvField)

		// nil 指针仍 IsValid，原先不会 typeInit，深层复制会跳过指针字段。
		if destNeedsInit(avfi) && srcHasValue(bvfi) {
			typeInit(avField, false)
			avfi = inDirect(avField)
		}
		afKind, bfKind := avfi.Kind(), bvfi.Kind()
		// 深度复制嵌套结构体（值类型）
		if deep && afKind == reflect.Struct && bfKind == reflect.Struct {
			// 嵌套路径应使用完整 fieldName，否则 exclude 前缀丢失。
			if err := copyStruct(avField, bvField, fieldName+".", exclude, true, visited); err != nil {
				return err
			}
			continue
		}

		// Map 复制（支持键值类型转换，深度时对结构体值递归）
		if afKind == reflect.Map && bfKind == reflect.Map {
			// 统一保证 avfi 可写（未导出字段 / 间接后的值）
			ensureSettable := func(v reflect.Value) reflect.Value {
				if !v.CanSet() && v.CanAddr() {
					return makeSettable(v, true)
				}
				return v
			}
			avfi = ensureSettable(avfi)

			if bvfi.IsNil() {
				if deep && avfi.CanSet() {
					// 深拷贝遇到 nil map：清空目标
					if err := trySet(avfi, reflect.Zero(avfi.Type())); err != nil {
						return err
					}
				}
				continue
			}

			afType := avfi.Type()
			if avfi.IsNil() {
				if !avfi.CanSet() {
					continue
				}
				if err := trySet(avfi, reflect.MakeMapWithSize(afType, bvfi.Len())); err != nil {
					return err
				}
			}

			// 源和目标是同一张 map 时，遍历中插入新键的行为未定义。
			writeTo := avfi
			var tmp reflect.Value
			if avfi.Pointer() == bvfi.Pointer() {
				tmp = reflect.MakeMapWithSize(afType, bvfi.Len())
				writeTo = tmp
			}

			bfmr := bvfi.MapRange()
			for bfmr.Next() {
				key, err := autoConvertReflect(afType.Key(), bfmr.Key())
				if err != nil {
					return fmt.Errorf("copy: map key %s: %w", fieldName, err)
				}

				val := bfmr.Value()
				if deep {
					slot := reflect.New(afType.Elem()).Elem()
					if err := cloneElemInto(slot, val, fieldName+".", exclude, visited, 0); err != nil {
						return err
					}
					if err := trySetMapIndex(writeTo, key, slot); err != nil {
						return err
					}
					continue
				}
				cv, err := autoConvertReflect(afType.Elem(), val)
				if err != nil {
					return fmt.Errorf("copy: map value %s: %w", fieldName, err)
				}
				if err := trySetMapIndex(writeTo, key, cv); err != nil {
					return err
				}
			}

			if tmp.IsValid() {
				avfi = ensureSettable(avfi)
				if avfi.CanSet() {
					if err := trySet(avfi, tmp); err != nil {
						return err
					}
				} else {
					// 不可直接 Set 整个 map 时，逐键清空再写入（原逻辑）
					for _, k := range avfi.MapKeys() {
						avfi.SetMapIndex(k, reflect.Value{})
					}
					iter := tmp.MapRange()
					for iter.Next() {
						if err := trySetMapIndex(avfi, iter.Key(), iter.Value()); err != nil {
							return err
						}
					}
				}
			}
			continue
		}

		// Slice 复制
		if afKind == reflect.Slice && bfKind == reflect.Slice {
			if bvfi.IsNil() {
				if deep {
					target := avfi
					if !target.CanSet() && target.CanAddr() {
						target = makeSettable(target, true)
					}
					if target.CanSet() {
						if err := trySet(target, reflect.Zero(target.Type())); err != nil {
							return err
						}
					}
				}
				continue
			}
			srcSlice := bvfi
			dstType := avfi.Type()
			if dstType.Kind() != reflect.Slice {
				dstType = srcSlice.Type()
			}
			n := srcSlice.Len()
			nv := reflect.MakeSlice(dstType, n, max(srcSlice.Cap(), n))
			elemDst := dstType.Elem()
			if deep {
				for j := range n {
					if err := cloneElemInto(nv.Index(j), srcSlice.Index(j), fieldName+".", exclude, visited, 0); err != nil {
						return err
					}
				}
			} else {
				for j := range n {
					ev, err := autoConvertReflect(elemDst, srcSlice.Index(j))
					if err != nil {
						return fmt.Errorf("copy: slice element %s[%d]: %w", fieldName, j, err)
					}
					if err := trySet(nv.Index(j), ev); err != nil {
						return err
					}
				}
			}

			// 字段是 *[]T 时 avField 可设置但是指针，Set(切片) 会 panic。
			// 优先写间接后、类型匹配的切片头；接口字段仍写 avField。
			switch {
			case avfi.CanSet() && avfi.Type() == nv.Type():
				if err := trySet(avfi, nv); err != nil {
					return err
				}
			case avField.CanSet() && (avField.Type() == nv.Type() || avField.Kind() == reflect.Interface):
				if err := trySet(avField, nv); err != nil {
					return err
				}
			default:
				target := avfi
				if !target.CanSet() && target.CanAddr() {
					target = makeSettable(target, true)
				}
				if target.CanSet() && target.Type() == nv.Type() {
					if err := trySet(target, nv); err != nil {
						return err
					}
				} else {
					target = avField
					if !target.CanSet() && target.CanAddr() {
						target = makeSettable(target, true)
					}
					if target.CanSet() && (target.Type() == nv.Type() || target.Kind() == reflect.Interface) {
						if err := trySet(target, nv); err != nil {
							return err
						}
					}
				}
			}
			// 深拷贝已写入独立切片头，勿再浅赋值覆盖。
			continue
		}

		// Array（定长数组）复制
		if afKind == reflect.Array && bfKind == reflect.Array {
			if avfi.Type() != bvfi.Type() || !avfi.CanSet() {
				continue
			}
			if deep {
				for j := range avfi.Len() {
					if err := cloneElemInto(avfi.Index(j), bvfi.Index(j), fieldName+".", exclude, visited, 0); err != nil {
						return err
					}
				}
			} else {
				reflect.Copy(avfi, bvfi)
			}
			continue
		}

		// 普通字段赋值 / 类型转换
		// 目标可能经 makeSettable 已可写，源仍可能是未导出字段（flagRO），
		// 必须走 trySet（内部 makeExported），禁止直接 Set。
		setField := func(dst, src reflect.Value) error {
			if !dst.CanSet() && dst.CanAddr() {
				dst = makeSettable(dst, true)
			}
			if !dst.CanSet() {
				return nil
			}
			return trySet(dst, src)
		}

		// tryAssignField 尝试赋值，任何转换路径都失败时返回错误，
		tryAssignField := func(dst, src reflect.Value) error {
			if src.Type().AssignableTo(dst.Type()) {
				return setField(dst, src)
			}
			if cv, err := autoConvertReflect(dst.Type(), src); err == nil {
				return setField(dst, cv)
			} else if src.Type().ConvertibleTo(dst.Type()) && canConvertSafely(src, dst.Type()) {
				return setField(dst, src.Convert(dst.Type()))
			} else {
				return fmt.Errorf("copy: field %s cannot convert %s to %s: %w", fieldName, src.Type(), dst.Type(), err)
			}
		}
		switch {
		case avField.CanSet() || avField.CanAddr():
			if err := tryAssignField(avField, bvField); err != nil {
				return err
			}
		case avfi.CanSet() || avfi.CanAddr():
			if err := tryAssignField(avfi, bvfi); err != nil {
				return err
			}
		}

	}
	return nil
}

func DepthField(s any, index ...any) (field any, err error) {
	field = s
	for _, i := range index {
		field, err = depthField(field, i)
		if err != nil {
			return nil, err
		}
	}
	return field, nil
}

func depthField(s any, index any) (any, error) {
	sv := inDirect(asValue(s))
	var v reflect.Value
	switch sv.Kind() {
	case reflect.Struct:
		switch idx := index.(type) {
		case string:
			v = sv.FieldByName(idx)
		default:
			i, ok := toInt(index)
			if !ok {
				return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
			}
			n := sv.NumField()
			if i < 0 {
				i += n
			}
			if i < 0 || i >= n {
				return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
			}
			v = sv.Field(i)
		}
	case reflect.Map:
		if sv.IsNil() {
			return nil, fmt.Errorf("该字段是 nil。错误的字段名为(%#v)", index)
		}
		k, ok := convertKey(index, sv.Type().Key())
		if !ok {
			return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
		}
		v = sv.MapIndex(k)
	case reflect.Slice, reflect.Array:
		i, ok := toInt(index)
		if !ok {
			return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
		}
		n := sv.Len()
		if i < 0 {
			i += n
		}
		if i < 0 || i >= n {
			return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
		}
		v = sv.Index(i)
	default:
		return nil, fmt.Errorf("非结构类型，无法正确读取。错误的类型为（%s）", sv.Kind())
	}

	if v.Kind() != reflect.Invalid {
		if !v.CanInterface() { // 不可导出的私有小写字段读取 panic 阻断
			return nil, fmt.Errorf("该字段未导出，不可读。字段名（%#v）", index)
		}
		return v.Interface(), nil
	}
	return nil, fmt.Errorf("该字段不是有效。错误的字段名为（%#v）", index)
}

// Init 对已有值做指针链分配，可选把最终值置零/做成容器，并在 isZero 时补结构体里的 nil 指针。
// 与 Make 职责分离，解决原 initChain 与 Make 混用问题。
func Init(v any, isZero bool, args ...any) bool {
	target := toValue(v)
	if !typeInit(target, isZero, args...) {
		return false
	}
	// isZero=true：typeInit 已把容器/零值就位，再补齐结构体里的 nil 指针。
	// isZero=false：只沿指针链分配，不遍历结构体字段，避免改写已有数据。
	if isZero {
		initNestedNilPtrs(target, 0, make(map[copyVisit]struct{}))
	}
	return true
}

func Convert(a, b any) bool {
	return typeConvert(toValue(a), asValue(b))
}

func To(typ, v any) (any, error) {
	telem, err := Type(typ)
	if err != nil {
		return nil, err
	}
	rv, err := autoConvert(telem, v)
	if err != nil {
		return nil, err
	}
	return rv.Interface(), nil
}

// Get 通用取值。
//
// 支持：
//   - map[K]V          → key 可赋值/可转换/常见数字↔字符串互转
//   - slice / array    → 整数索引（支持负索引）
//   - string           → 整数索引（按字节，支持负索引）
//   - struct / *struct → 字段名（精确 / 首字母大小写 / json·mapstructure 标签）或整数索引（支持负索引）
//   - 数值类型         → 转为十进制字符串后按索引取字符
//   - 指针 / interface → 自动多层解包
//   - reflect.Value    → 直接接受
//
// 行为约定：
//   - map 中不存在的 key          → (nil, nil)
//   - 结构体找不到字段            → (nil, ErrFieldNotFound)
//   - 索引越界                    → (nil, ErrIndexOutOfRange)
//   - 类型不支持 / key 类型错误   → 对应错误
func Get(m any, key any) (any, error) {
	return getInternal(m, false, key)
}

func GetUnexported(m any, key any) (any, error) {
	return getInternal(m, true, key)
}

func getInternal(m any, allowUnexported bool, key any) (any, error) {
	if m == nil {
		return nil, ErrNilValue
	}
	v := inDirect(asValue(m))
	if !v.IsValid() {
		return nil, ErrNilValue
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, ErrNilValue
		}
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
	switch v.Kind() {
	case reflect.Map:
		return getMap(v, key)
	case reflect.Slice, reflect.Array:
		return getIndexable(v, key)
	case reflect.String:
		return getString(v, key)
	case reflect.Struct:
		return getStruct(v, key, allowUnexported)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return getNumber(v, key)
	default:
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
}

// ---------------------------------------------------------------------------
// map
// ---------------------------------------------------------------------------
func getMap(v reflect.Value, key any) (any, error) {
	kt := v.Type().Key()
	k, ok := convertKey(key, kt)
	if !ok {
		return nil, fmt.Errorf("%w: need %v, got %T", ErrMapKey, kt, key)
	}
	res := v.MapIndex(k)
	if !res.IsValid() {
		return nil, nil // 与 map 语义一致：key 不存在返回 (nil, nil)
	}
	return valueToInterface(res), nil
}

// ---------------------------------------------------------------------------
// 核心：reflect.Value → any
// 路径优先级：CanInterface → unsafe 清 flag → NewAt → 按 Kind 降级
// ---------------------------------------------------------------------------

func valueToInterface(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	// Fast path：导出字段
	if v.CanInterface() {
		return v.Interface()
	}

	// 未导出字段：优先使用已验证的 unsafe 方案
	if unsafeExportEnabled() {
		ev := makeExported(v)
		if ev.CanInterface() {
			return ev.Interface()
		}
	}

	// 可寻址时用 NewAt（官方推荐安全写法）
	if v.CanAddr() {
		exported := reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
		if exported.CanInterface() {
			return exported.Interface()
		}
	}

	// 最终降级：按 Kind 手动提取
	return typeSelect(v)
}

// ---------------------------------------------------------------------------
// slice / array
// ---------------------------------------------------------------------------

func getIndexable(v reflect.Value, key any) (any, error) {
	idx, ok := toInt(key)
	if !ok {
		return nil, fmt.Errorf("%w: slice/array key must be integer, got %T", ErrKeyType, key)
	}
	n := v.Len()
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, n)
	}
	return valueToInterface(v.Index(idx)), nil
}

// ---------------------------------------------------------------------------
// string（按字节索引）
// ---------------------------------------------------------------------------

func getString(v reflect.Value, key any) (any, error) {
	idx, ok := toInt(key)
	if !ok {
		return nil, fmt.Errorf("%w: string key must be integer, got %T", ErrKeyType, key)
	}
	s := v.String()
	n := len(s)
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, n)
	}
	return s[idx], nil
}

// ---------------------------------------------------------------------------
// 数值类型 → 十进制字符串后按索引取字符
// ---------------------------------------------------------------------------

func getNumber(v reflect.Value, key any) (any, error) {
	var s string
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s = strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s = strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		s = strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.Complex64, reflect.Complex128:
		s = fmt.Sprintf("%g", v.Complex())
	}
	idx, ok := toInt(key)
	if !ok {
		return nil, fmt.Errorf("%w: number key must be integer, got %T", ErrKeyType, key)
	}
	n := len(s)
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, n)
	}
	return s[idx], nil
}

// ---------------------------------------------------------------------------
// struct
// ---------------------------------------------------------------------------

func getStruct(v reflect.Value, key any, allowUnexported bool) (any, error) {
	if k, ok := key.(string); ok {
		return getStructByName(v, k, allowUnexported)
	}
	if idx, ok := toInt(key); ok {
		return getStructByIndex(v, idx, allowUnexported)
	}
	return nil, fmt.Errorf("%w: struct key must be string or int, got %T", ErrKeyType, key)
}

// fieldByIndexSafe 安全地按索引路径取值。
// 若路径上任意中间指针为 nil，返回 ErrNilValue，避免 FieldByIndex 的 panic。
func fieldByIndexSafe(v reflect.Value, index []int) (reflect.Value, error) {
	if len(index) == 1 {
		return v.Field(index[0]), nil
	}
	for i, x := range index {
		if i > 0 {
			if v.Kind() == reflect.Pointer {
				if v.IsNil() {
					return reflect.Value{}, ErrNilValue
				}
				v = v.Elem()
			}
		}
		if v.Kind() != reflect.Struct {
			return reflect.Value{}, ErrNilValue
		}
		v = v.Field(x)
	}
	return v, nil
}

func fieldByIndexNoPanic(v reflect.Value, index []int) (fv reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrUnsupported, r)
		}
	}()
	fv = v.FieldByIndex(index)
	return fv, nil
}

func getStructByName(v reflect.Value, name string, allowUnexported bool) (any, error) {
	idxMap := buildFieldIndex(v.Type(), allowUnexported)
	if idx, exists := idxMap[name]; exists {
		fv, err := fieldByIndexSafe(v, idx)
		if err != nil {
			return nil, err
		}
		return valueToInterface(fv), nil
	}
	// 缓存未命中时再尝试一次首字母大写（防止极端自定义类型名）
	if capName := capitalize(name); capName != name {
		if idx, exists := idxMap[capName]; exists {
			fv, err := fieldByIndexSafe(v, idx)
			if err != nil {
				return nil, err
			}
			return valueToInterface(fv), nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrFieldNotFound, name)
}

func getStructByIndex(v reflect.Value, idx int, allowUnexported bool) (any, error) {
	n := v.NumField()
	if idx < 0 {
		idx += n
	}
	if idx < 0 || idx >= n {
		return nil, fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, idx, n)
	}
	sf := v.Type().Field(idx)
	// Get 约定不读未导出字段，按下标原先会泄漏。
	if sf.PkgPath != "" && !allowUnexported {
		return nil, fmt.Errorf("%w: %q", ErrFieldNotFound, sf.Name)
	}
	return valueToInterface(v.Field(idx)), nil
}

// Set 通用赋值，与 Get 功能、错误、转换规则精确对称。
//
// 支持：
//   - map[K]V          → key 可赋值/可转换/常见数字↔字符串互转；值为 nil 时删除键
//   - slice / array    → 整数索引（支持负索引）；slice 可自动扩容
//   - struct / *struct → 字段名（精确 / 首字母大小写 / json·mapstructure 标签）或整数索引（支持负索引）
//   - 指针 / interface → 自动多层解包
//   - reflect.Value    → 直接接受（需最终可设置）
//
// 行为约定（与 Get 对应）：
//   - 结构体找不到字段            → ErrFieldNotFound
//   - 索引越界                    → ErrIndexOutOfRange
//   - 类型不支持 / key 类型错误   → ErrUnsupported / ErrKeyType / ErrMapKey
//   - 不可寻址或未导出字段        → ErrUnaddressable
//   - 字符串 / 数值按索引修改     → ErrImmutable
//   - map 为 nil                  → ErrNilMap
//
// 扩展行为：
//   - 写嵌入（Anonymous）指针路径时，路径上的 nil 嵌入指针会自动分配；普通指针字段不会
//   - 整次 Set 失败时会回滚：恢复已写叶子字段，并清除本次自动分配的中间嵌入指针，
//     保证失败后结构体与调用前完全一致（事务语义）
//
// 并发安全：函数本身无共享可变状态；字段索引使用 sync.Map 缓存（只读共享）。
// 对同一个 map / slice 的并发写仍需调用方加锁。
// 本函数不允许写入未导出字段（安全默认）。
func Set(m any, args ...any) error {
	return setInternal(m, false, args...)
}

// SetUnexported 与 Set 行为完全一致，但允许通过 unsafe 写入未导出但可寻址的字段。
// 仅应在可信代码中使用。字段索引缓存按 (type, allow) 分别维护，与 Set 互不干扰。
func SetUnexported(m any, args ...any) error {
	return setInternal(m, true, args...)
}

func setInternal(m any, allowUnexported bool, args ...any) error {
	n := len(args)
	if n == 0 || n&1 != 0 {
		return ErrInvalidArgCount
	}

	v, err := resolveSettable(m)
	if err != nil {
		return err
	}

	switch v.Kind() {
	case reflect.Map:
		return setMap(v, args)
	case reflect.Slice:
		return setIndexable(v, args, true, allowUnexported)
	case reflect.Array:
		return setIndexable(v, args, false, allowUnexported)
	case reflect.Struct:
		return setStruct(v, args, allowUnexported)
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return ErrImmutable
	default:
		return fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
}

// resolveSettable 多层解包指针/interface/reflect.Value，得到最终操作目标。
func resolveSettable(m any) (reflect.Value, error) {
	if m == nil {
		return reflect.Value{}, ErrNilValue
	}
	v := inDirect(asValue(m))
	if !v.IsValid() {
		return reflect.Value{}, ErrNilValue
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return reflect.Value{}, ErrNilValue
		}
		return reflect.Value{}, fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
	switch v.Kind() {
	case reflect.Map, reflect.Slice:
		// 引用类型，即使不可寻址也可操作底层数据（元素赋值）；
		// 但 slice 扩容需要可设置的 header，由 setIndexable 检查。
		return v, nil
	case reflect.Struct, reflect.Array:
		if !v.CanSet() && !v.CanAddr() {
			return reflect.Value{}, ErrUnaddressable
		}
		return v, nil
	case reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return v, nil
	default:
		return reflect.Value{}, fmt.Errorf("%w: %v", ErrUnsupported, v.Kind())
	}
}

// ---------------------------------------------------------------------------
// map
// ---------------------------------------------------------------------------

type undoFunc func()

func setMap(mv reflect.Value, args []any) error {
	if mv.IsNil() {
		return ErrNilMap
	}
	kt, et := mv.Type().Key(), mv.Type().Elem()
	var undos []undoFunc
	rollback := func() {
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}
	for i := 0; i < len(args); i += 2 {
		k, ok := convertKey(args[i], kt)
		if !ok {
			rollback()
			return fmt.Errorf("%w: need %v, got %T", ErrMapKey, kt, args[i])
		}

		old := mv.MapIndex(k)
		existed := old.IsValid()

		var elem reflect.Value

		if args[i+1] == nil {
			elem = reflect.Value{} // 删除
		} else {
			converted, err := autoConvert(et, args[i+1])
			if err != nil {
				rollback()
				return err
			}
			elem = converted
		}

		// 记录 undo
		if existed {
			oldCopy := reflect.New(et).Elem()
			oldCopy.Set(old)
			keyCopy := k
			undos = append(undos, func() { mv.SetMapIndex(keyCopy, oldCopy) })
		} else {
			keyCopy := k
			undos = append(undos, func() { mv.SetMapIndex(keyCopy, reflect.Value{}) })
		}

		if err := trySetMapIndex(mv, k, elem); err != nil {
			rollback()
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// slice / array
// ---------------------------------------------------------------------------

func setIndexable(sv reflect.Value, args []any, growable bool, allowUnexported bool) error {
	et := sv.Type().Elem()
	// 不依赖运行时切片头布局，32/64 位和 wasm 都能 Set 回去。
	var oldView reflect.Value
	hasOldView := false
	if growable {
		if sv.IsNil() {
			oldView = reflect.Zero(sv.Type())
		} else {
			oldView = sv.Slice(0, sv.Len())
		}
		hasOldView = true
	}

	var undos []undoFunc
	rolledBack := false

	rollback := func() {
		if rolledBack {
			return
		}
		rolledBack = true
		if growable && hasOldView {
			if !sv.CanSet() {
				sv = makeSettable(sv, allowUnexported)
			}
			if sv.CanSet() {
				sv.Set(oldView)
			}
		}
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
	}

	for i := 0; i < len(args); i += 2 {
		idx, ok := toInt(args[i])
		if !ok {
			rollback()
			return fmt.Errorf("%w: slice/array key must be integer, got %T", ErrKeyType, args[i])
		}

		if idx < 0 {
			// 扩容后再出现的负索引按当前 len 解释（TestSet: [1,2] 设 3 再 -1 → [1,2,0,8]）
			idx += sv.Len()
		}
		if idx < 0 {
			rollback()
			return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, sv.Len())
		}

		// 自动扩容（仅 slice）
		if growable && idx >= sv.Len() {
			if idx == math.MaxInt {
				rollback()
				return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, sv.Len())
			}
			if !sv.CanSet() {
				sv = makeSettable(sv, allowUnexported)
			}
			if !sv.CanSet() {
				rollback()
				return ErrUnaddressable
			}
			newLen := idx + 1
			newCap := sv.Cap()
			if newLen > newCap {
				newCap = newLen
				if newLen <= math.MaxInt/2 {
					newCap = max(newCap, newLen*2)
				}
				newCap = max(newCap, 4)
			}
			ns := reflect.MakeSlice(sv.Type(), newLen, newCap)
			reflect.Copy(ns, sv)
			sv.Set(ns)
		}

		if idx >= sv.Len() {
			rollback()
			return fmt.Errorf("%w: [%d] with length %d", ErrIndexOutOfRange, idx, sv.Len())
		}

		elem := sv.Index(idx)
		if !elem.CanSet() {
			elem = makeSettable(elem, allowUnexported)
		}
		if !elem.CanSet() {
			rollback()
			return ErrUnaddressable
		}

		// 保存旧值。扩容新增的下标靠恢复切片头丢弃，不能再 Index。
		var old reflect.Value
		if idx < sv.Len() {
			old = reflect.New(et).Elem()
			old.Set(elem)
		}

		converted, err := autoConvert(et, args[i+1])
		if err != nil {
			rollback()
			return err
		}

		l := sv.Len()
		if idx < l {
			curIdx := idx
			undos = append(undos, func() {
				if curIdx >= sv.Len() {
					return
				}
				el := sv.Index(curIdx)
				if !el.CanSet() {
					el = makeSettable(el, allowUnexported)
				}
				if el.CanSet() {
					el.Set(old)
				}
			})
		}

		if err := trySet(elem, converted); err != nil {
			rollback()
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// struct
// ---------------------------------------------------------------------------

func setStruct(sv reflect.Value, args []any, allowUnexported bool) error {
	t := sv.Type()
	idxMap := buildFieldIndex(t, allowUnexported)
	n := t.NumField()
	var undos, allocUndos []undoFunc
	rollback := func() {
		// 1. 先恢复叶子字段（此时中间指针仍有效）
		for i := len(undos) - 1; i >= 0; i-- {
			undos[i]()
		}
		// 2. 再按分配逆序清除中间指针，保证失败后与调用前完全一致
		for i := len(allocUndos) - 1; i >= 0; i-- {
			allocUndos[i]()
		}
	}

	for i := 0; i < len(args); i += 2 {
		key, val := args[i], args[i+1]
		var field reflect.Value

		switch k := key.(type) {
		case string:
			idx, exists := idxMap[k]
			if !exists {
				if capName := capitalize(k); capName != k {
					idx, exists = idxMap[capName]
				}
			}
			if !exists {
				rollback()
				return fmt.Errorf("%w: %q", ErrFieldNotFound, k)
			}

			// 按需初始化路径中的 nil 嵌入指针，安全返回可设置字段
			f, intermediates, err := safeFieldByIndex(sv, idx, allowUnexported)
			// 即使失败也先收录已分配的中间指针，再统一回滚清理
			allocUndos = append(allocUndos, intermediates...)
			if err != nil {
				rollback()
				return err
			}
			field = f

		default:
			idx, ok := toInt(key)
			if !ok {
				rollback()
				return fmt.Errorf("%w: struct key must be string or int, got %T", ErrKeyType, key)
			}
			if idx < 0 {
				idx += n
			}
			if idx < 0 || idx >= n {
				rollback()
				return fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, idx, n)
			}
			sf := t.Field(idx)
			if sf.PkgPath != "" && !allowUnexported {
				rollback()
				return fmt.Errorf("%w: %q", ErrFieldNotFound, sf.Name)
			}
			// 与字符串路径统一：走 safeFieldByIndex，保证行为一致
			f, intermediates, err := safeFieldByIndex(sv, []int{idx}, allowUnexported)
			allocUndos = append(allocUndos, intermediates...)
			if err != nil {
				rollback()
				return err
			}
			field = f
		}

		if !field.IsValid() {
			rollback()
			return fmt.Errorf("%w: %v", ErrFieldNotFound, key)
		}

		// 若为未导出字段且 AllowUnexported 开启，通过 makeSettable 使其可写
		if !field.CanSet() {
			field = makeSettable(field, allowUnexported)
		}
		if !field.CanSet() {
			rollback()
			return fmt.Errorf("%w: field %v", ErrUnaddressable, key)
		}

		// 保存旧值
		old := reflect.New(field.Type()).Elem()
		old.Set(field)

		converted, err := autoConvert(field.Type(), val)
		if err != nil {
			rollback()
			return err
		}

		// 记录叶子 undo：直接捕获可写 Value，避免回滚时重新走路径导致误分配
		settableField := field
		undos = append(undos, func() {
			if settableField.CanSet() {
				settableField.Set(old)
			}
		})

		if err := trySet(field, converted); err != nil {
			rollback()
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

func convertToInt(v any) (int, error) {
	if v == nil {
		return 0, errNotInt
	}
	switch x := v.(type) {
	case int:
		return x, nil
	case int8:
		return int(x), nil
	case int16:
		return int(x), nil
	case int32:
		return int(x), nil
	case int64:
		return intFromInt64(x)
	case uint:
		if x <= uint(math.MaxInt) {
			return int(x), nil
		}
		return 0, errIntRange
	case uint8:
		return int(x), nil
	case uint16:
		return int(x), nil
	case uint32:
		if uint64(x) <= uint64(math.MaxInt) {
			return int(x), nil
		}
		return 0, errIntRange
	case uint64:
		if x <= uint64(math.MaxInt) {
			return int(x), nil
		}
		return 0, errIntRange
	case uintptr:
		if uint64(x) <= uint64(math.MaxInt) {
			return int(x), nil
		}
		return 0, errIntRange
	case float32:
		return floatToInt(float64(x))
	case float64:
		return floatToInt(x)
	case string:
		return parseIntString(x)
	}

	rv := asValue(v)
	if !rv.IsValid() {
		return 0, errNotInt
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return intFromInt64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u <= uint64(math.MaxInt) {
			return int(u), nil
		}
		return 0, errIntRange
	case reflect.Float32, reflect.Float64:
		return floatToInt(rv.Float())
	case reflect.String:
		return parseIntString(rv.String())
	}
	return 0, errNotInt
}

// [CHANGED]
func toInt(v any) (int, bool) {
	n, err := convertToInt(v)
	return n, err == nil
}

func intFromInt64(i int64) (int, error) {
	n, ok := safeInt64(i)
	if !ok {
		return 0, errIntRange
	}
	return n, nil
}

func parseIntString(s string) (int, error) {
	i, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		var ne *strconv.NumError
		if errors.As(err, &ne) && errors.Is(ne.Err, strconv.ErrRange) {
			return 0, errIntRange
		}
		return 0, errNotInt
	}
	return intFromInt64(i)
}

// floatToInt 拒绝 NaN、非整数和不能放进 int64 的值。
func floatToInt(f float64) (int, error) {
	if f != f || f != math.Trunc(f) {
		return 0, errNotInt
	}
	if f >= 9223372036854775808.0 || f < float64(math.MinInt64) {
		return 0, errIntRange
	}
	return intFromInt64(int64(f))
}

// capitalize 将首个字符转为大写（替代已废弃的 strings.Title）。
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(s)
	if unicode.IsUpper(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// convertKey 将任意 key 转为 map 的 key 类型；失败返回 false。
// 支持常见数字↔字符串、bool 等互转，并做目标类型位宽范围检查（禁止截断）。
func convertKey(key any, target reflect.Type) (reflect.Value, bool) {
	if key == nil {
		switch target.Kind() {
		case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
			return reflect.Zero(target), true
		}
		return reflect.Value{}, false
	}
	kv := asValue(key)
	if !kv.IsValid() {
		return reflect.Value{}, false
	}
	if kv.Type().AssignableTo(target) {
		return kv, true
	}

	switch target.Kind() {
	case reflect.Bool: // 支持 Map 的 Bool 键的转换与解析
		switch kv.Kind() {
		case reflect.Bool:
			return kv, true
		case reflect.String:
			b, err := strconv.ParseBool(kv.String())
			if err == nil {
				return reflect.ValueOf(b), true
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(kv.Int() != 0), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(kv.Uint() != 0), true
		case reflect.Float32, reflect.Float64:
			f := kv.Float()
			// NaN != 0 为真，不能当成 true 键。
			if f != f {
				return reflect.Value{}, false
			}
			return reflect.ValueOf(f != 0), true
		}
	case reflect.String:
		// 数字→字符串：用十进制表示，而不是 rune 转换（Convert 会把 1 变成 "\x01"）
		switch kv.Kind() {
		case reflect.String:
			return kv, true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return reflect.ValueOf(strconv.FormatInt(kv.Int(), 10)), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return reflect.ValueOf(strconv.FormatUint(kv.Uint(), 10)), true
		case reflect.Float32, reflect.Float64:
			return reflect.ValueOf(strconv.FormatFloat(kv.Float(), 'f', -1, 64)), true
		case reflect.Bool:
			return reflect.ValueOf(strconv.FormatBool(kv.Bool())), true
		default:
			return reflect.Value{}, false
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, ok := toInt64ForKey(kv, key)
		if !ok {
			return reflect.Value{}, false
		}
		// 范围检查：必须能完整放入目标类型，禁止截断
		bits := target.Bits()
		minv := -(int64(1) << (bits - 1))
		maxv := (int64(1) << (bits - 1)) - 1
		if i < minv || i > maxv {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(i).Convert(target), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, ok := toUint64ForKey(kv, key)
		if !ok {
			return reflect.Value{}, false
		}
		// 范围检查：必须能完整放入目标类型，禁止截断/wrap
		if target.Bits() < 64 && u > (uint64(1)<<target.Bits())-1 {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(u).Convert(target), true
	case reflect.Float32, reflect.Float64:
		f, ok := toFloat64ForKey(kv, key)
		if !ok {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(f).Convert(target), true
	}

	// 其它类型：仅在可安全 Convert 时放行（非数字/字符串的命名类型等）
	if kv.Type().ConvertibleTo(target) {
		return kv.Convert(target), true
	}
	return reflect.Value{}, false
}

// toInt64ForKey 将 key 转为 int64，失败返回 false。
// 拒绝：无法解析的字符串、超出 int64 的 uint、NaN/Inf、非整数浮点。
func toInt64ForKey(kv reflect.Value, key any) (int64, bool) {
	if k, ok := key.(string); ok {
		i, err := strconv.ParseInt(k, 10, 64)
		return i, err == nil
	}
	switch kv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return kv.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := kv.Uint()
		if u > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(u), true
	case reflect.Float32, reflect.Float64:
		f := kv.Float()
		// 2^63 不是合法 int64；小数键不应被截断。
		if f != f || f != math.Trunc(f) || f >= 9223372036854775808.0 || f < float64(math.MinInt64) {
			return 0, false
		}
		return int64(f), true
	}
	return 0, false
}

// toUint64ForKey 将 key 转为 uint64，失败返回 false。
// 拒绝：负数、无法解析的字符串、NaN/Inf、非整数。
func toUint64ForKey(kv reflect.Value, key any) (uint64, bool) {
	if k, ok := key.(string); ok {
		u, err := strconv.ParseUint(k, 10, 64)
		return u, err == nil
	}
	switch kv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := kv.Int()
		if i < 0 {
			return 0, false
		}
		return uint64(i), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return kv.Uint(), true
	case reflect.Float32, reflect.Float64:
		f := kv.Float()
		if f != f || f != math.Trunc(f) || f < 0 || f >= 18446744073709551616.0 {
			return 0, false
		}
		return uint64(f), true
	}
	return 0, false
}

// toFloat64ForKey 将 key 转为 float64。
// 拒绝无法解析的字符串；NaN/Inf 对 float 目标通常可接受，故不额外过滤。
func toFloat64ForKey(kv reflect.Value, key any) (float64, bool) {
	if k, ok := key.(string); ok {
		f, err := strconv.ParseFloat(k, 64)
		return f, err == nil
	}
	switch kv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(kv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(kv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return kv.Float(), true
	}
	return 0, false
}

type fieldIndex map[string][]int

type fieldIndexKey struct {
	t     reflect.Type
	allow bool
}

// buildFieldIndex 构建并缓存某个 struct 类型的字段索引表（含嵌入字段提升）。
// 使用 FieldByIndex 路径，支持 json / mapstructure 标签及首字母大小写别名。
// 同名冲突时：本层字段优先于嵌入字段（符合 Go 遮蔽规则）；同层冲突时先出现者优先。
// 并发安全：sync.Map + LoadOrStore 保证每个 (type, allow) 只完整构建一次。
func buildFieldIndex(t reflect.Type, allow bool) fieldIndex {
	key := fieldIndexKey{t: t, allow: allow}
	box := fieldCacheBox()
	if v, ok := box.m.Load(key); ok {
		return v.(fieldIndex)
	}

	m := make(fieldIndex, t.NumField()*2)
	// 同一遍里先登记的 tag 会占掉后面字段的精确名。
	walkFields(t, nil, nil, allow, m, false)
	walkFields(t, nil, nil, allow, m, true)

	if box.n.Add(1) > maxFieldCache {
		fresh := &indexCache{}
		fieldCachePtr.CompareAndSwap(box, fresh)
	}
	actual, _ := box.m.LoadOrStore(key, m)
	return actual.(fieldIndex)
}

func walkFields(tt reflect.Type, path []int, stack []reflect.Type, allow bool, m fieldIndex, tagsOnly bool) {
	if tt == nil || tt.Kind() != reflect.Struct {
		return
	}
	for _, seen := range stack {
		if seen == tt {
			// type A struct { *A } 一类递归嵌入会无限递归。
			return
		}
	}
	// append 可能复用父栈底层数组，兄弟嵌入会互相覆盖环检测记录。
	nextStack := make([]reflect.Type, len(stack)+1)
	copy(nextStack, stack)
	nextStack[len(stack)] = tt

	n := tt.NumField()
	type embedInfo struct {
		ft  reflect.Type
		idx []int
	}
	var embeds []embedInfo
	for i := range n {
		sf := tt.Field(i)
		unexported := sf.PkgPath != ""
		// 与 Go 选择规则对齐：
		// - 未导出且非匿名：不登记、不提升
		// - 未导出但匿名：不登记该字段名本身，仍递归以提升其中的导出字段
		// - allow=true：未导出字段也可登记
		if unexported && !allow && !sf.Anonymous {
			continue
		}
		idx := make([]int, len(path)+1)
		copy(idx, path)
		idx[len(path)] = i
		if !unexported || allow {
			if !tagsOnly {
				if _, exists := m[sf.Name]; !exists {
					m[sf.Name] = idx
				}
			} else {
				for _, tagName := range []string{"json", "mapstructure", "yaml", "toml", "bson"} {
					tag := sf.Tag.Get(tagName)
					if tag == "" {
						continue
					}
					name, _, _ := strings.Cut(tag, ",")
					name = strings.TrimSpace(name)
					if name == "" || name == "-" {
						continue
					}
					if _, exists := m[name]; !exists {
						m[name] = idx
					}
				}
			}
		}
		if sf.Anonymous {
			ft := sf.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			embeds = append(embeds, embedInfo{ft: ft, idx: idx})
			if ft.Kind() == reflect.Struct {
			}
		}
	}
	for _, e := range embeds {
		walkFields(e.ft, e.idx, nextStack, allow, m, tagsOnly)
	}
}

// safeInt64 converts int64 to int with platform overflow check（兼容 32/64 位）。
func safeInt64(i int64) (int, bool) {
	if i < int64(math.MinInt) || i > int64(math.MaxInt) {
		return 0, false
	}
	return int(i), true
}

// makeSettable 利用 unsafe 强制使未导出但可寻址的字段具备可写属性。
// 仅当 allowUnexported == true 时生效。依赖 unsafe，仅应在可信代码中使用。
func makeSettable(v reflect.Value, allowUnexported bool) reflect.Value {
	if !v.IsValid() || v.CanSet() {
		return v
	}
	if !allowUnexported || !v.CanAddr() {
		return v
	}
	// 利用底层地址重构可写 reflect.Value。NewAt 不依赖 reflect.Value 的 flag 布局。
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}

// safeFieldByIndex 安全地根据索引路径访问结构体字段。
// 仅当路径中的 nil 指针来自【匿名字段（嵌入）】且可寻址时，才自动分配实例。
// 返回的 allocUndos 记录本次自动分配的中间指针，失败时按逆序回滚。
func safeFieldByIndex(v reflect.Value, index []int, allowUnexported bool) (reflect.Value, []undoFunc, error) {
	curr := v
	var lastSF reflect.StructField
	hasLast := false
	var allocUndos []undoFunc

	for _, i := range index {
		if curr.Kind() == reflect.Pointer {
			if curr.IsNil() {
				// 只对嵌入（Anonymous）指针字段自动初始化；普通指针字段保持 nil 并报错
				if !hasLast || !lastSF.Anonymous {
					// 仍返回已分配的 undos，供调用方清理
					return reflect.Value{}, allocUndos, fmt.Errorf("%w: nil non-embedded pointer in field path", ErrUnaddressable)
				}
				if !curr.CanSet() {
					curr = makeSettable(curr, allowUnexported)
				}
				if !curr.CanSet() {
					return reflect.Value{}, allocUndos, ErrUnaddressable
				}
				curr.Set(reflect.New(curr.Type().Elem()))
				// 记录回滚：将本次分配的指针重新置为 nil
				ptrField := curr
				allocUndos = append(allocUndos, func() {
					p := ptrField
					if !p.CanSet() {
						p = makeSettable(p, allowUnexported)
					}
					if p.CanSet() {
						p.Set(reflect.Zero(p.Type()))
					}
				})
			}
			curr = curr.Elem()
		}
		if curr.Kind() != reflect.Struct {
			return reflect.Value{}, allocUndos, ErrUnsupported
		}
		if i < 0 || i >= curr.NumField() {
			return reflect.Value{}, allocUndos, fmt.Errorf("%w: field index [%d] with %d fields", ErrIndexOutOfRange, i, curr.NumField())
		}
		lastSF = curr.Type().Field(i)
		hasLast = true
		curr = curr.Field(i)
	}
	// 手工 Field 链穿过未导出嵌入后，提升字段的 CanSet 会变成 false。
	// 分配完 nil 嵌入指针后，用 FieldByIndex 取 Go 认可的可设置视图。
	if fv, err := fieldByIndexNoPanic(v, index); err == nil && fv.IsValid() {
		return fv, allocUndos, nil
	}
	return curr, allocUndos, nil
}

// Delete 删除 map 中的键。
func Delete(m any, key any) {
	// 对齐 Go：nil map 为空操作，非 map 忽略以免脚本层崩溃。
	mv := inDirect(asValue(m))
	if !mv.IsValid() || mv.Kind() != reflect.Map || mv.IsNil() {
		return
	}
	k, ok := convertKey(key, mv.Type().Key())
	if !ok {
		return
	}
	mv.SetMapIndex(k, zeroVal)
}

// GetSlice 返回 a[start:end] 的反射兼容版本。
//
// 语义严格对齐 Go 原生切片表达式：
//   - 要求 0 ≤ start ≤ end ≤ len(a)
//   - 对 slice / 可寻址 array / string：返回值与源共享底层数组（或字符串数据）
//   - 对不可寻址 array：返回独立拷贝，避免 panic
//
// 返回值约定：
//   - 成功时返回与源类型一致的切片（或子串）
//   - 任何越界、nil、不支持类型均返回 nil（interface{} 的 nil）
//
// 并发安全：
//
//	 本函数本身无共享状态，可被多 goroutine 同时调用。
//	 但当返回值与源共享底层数组时，调用方必须自行保证对源的并发写安全
//	（通常用 sync.Mutex / RWMutex 保护源 slice/array）。
//
// 支持类型：
//   - []T、*[N]T、[N]T、string
//   - 以及它们的任意层指针 / interface 包装（会自动解引用）
func GetSlice(a any, start, end int) (out any) {
	// 不可寻址数组拷贝未导出元素时 Set 可能 panic，应返回 nil 而不是崩。
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()
	if start < 0 || end < start {
		return nil
	}
	v := inDirect(asValue(a))
	if !v.IsValid() || v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		return nil
	}
	switch v.Kind() {
	case reflect.Slice:
		if end > v.Len() {
			return nil
		}
		if v.IsNil() {
			return v.Interface()
		}
		return v.Slice(start, end).Interface()
	case reflect.Array:
		if end > v.Len() {
			return nil
		}
		if v.CanAddr() {
			return v.Slice(start, end).Interface()
		}
		n := end - start
		copied := reflect.MakeSlice(reflect.SliceOf(v.Type().Elem()), n, n)
		for i := range n {
			copied.Index(i).Set(v.Index(start + i))
		}
		return copied.Interface()
	case reflect.String:
		if end > v.Len() {
			return nil
		}
		return v.Slice(start, end).Interface()
	default:
		return nil
	}
}

// SetSlice 将 src 的全部内容严格写入 dst[start:end]。
//
// 严格语义：
//   - 0 ≤ start ≤ end ≤ len(dst)
//   - len(src) 必须精确等于 end-start，否则失败
//   - 元素类型必须完全可赋值（reflect.Copy 规则）
//
// 失败回滚保证：
//
//	任意校验失败都不会修改 dst。
//	即使在拷贝过程中出现异常（当前实现下几乎不会），也会自动还原原区间。
//
// 支持目标：[]T、[N]T 及其任意层 * / interface 包装
// 支持源：  []T、[N]T、string（仅当目标元素为 byte）及其 * / interface 包装
//
// 返回 true 表示完整成功；false 表示失败且 dst 保持原样。
//
// 并发安全：函数本身无状态；共享底层数组时由调用方加锁。
func SetSlice(dst any, start, end int, src any) (ok bool) {
	if start < 0 || end < start {
		return false
	}
	dv := inDirect(asValue(dst))
	if !dv.IsValid() || dv.Kind() == reflect.Pointer || dv.Kind() == reflect.Interface {
		return false
	}
	var dSub reflect.Value
	switch dv.Kind() {
	case reflect.Slice:
		if dv.IsNil() || end > dv.Len() {
			return false
		}
		dSub = dv.Slice(start, end)
		// slice 的元素总是可写的（只要 header 有效）
	case reflect.Array:
		if end > dv.Len() || !dv.CanSet() {
			return false
		}
		dSub = dv.Slice(start, end)
	default:
		return false // string 等不可变类型
	}
	need := end - start
	if dSub.Len() != need {
		return false
	}
	sv := inDirect(asValue(src))
	if !sv.IsValid() || sv.Kind() == reflect.Pointer || sv.Kind() == reflect.Interface {
		return false
	}
	switch sv.Kind() {
	case reflect.Slice, reflect.Array, reflect.String:
		// ok
	default:
		return false
	}
	if sv.Len() != need {
		return false // 严格长度匹配
	}

	compatible := false
	if sv.Kind() == reflect.String {
		compatible = dSub.Type().Elem().Kind() == reflect.Uint8
	} else {
		compatible = sv.Type().Elem() == dSub.Type().Elem()
	}
	if !compatible {
		return false
	}

	// ---------- 3. 备份原区间（实现真正回滚） ----------
	backup := reflect.MakeSlice(dSub.Type(), need, need)
	reflect.Copy(backup, dSub)

	// ---------- 4. 执行写入 ----------
	// recover 必须写在独立函数里，写在 defer 内部的 defer 管不到这次 Copy。
	defer func() {
		if recover() != nil {
			ok = false
		}
		if !ok {
			restoreSlice(dSub, backup)
		}
	}()
	if reflect.Copy(dSub, sv) != need {
		return false
	}
	ok = true
	return true
}

func restoreSlice(dst, backup reflect.Value) {
	defer func() { recover() }()
	reflect.Copy(dst, backup)
}

// Copy 模拟内置 copy 函数，支持任意切片类型及 string -> []byte。
// 参数 a 为目标（必须是切片），b 为源（切片或字符串）。
// 返回复制成功的元素个数（int），若类型不兼容则返回 0。

func Copy(a any, b any) any {
	va, vb := inDirect(asValue(a)), inDirect(asValue(b))
	if va.Kind() != reflect.Slice {
		return 0
	}
	switch vb.Kind() {
	case reflect.String:
		// copy(dst []byte, src string)
		// type Bytes []byte 可通过（Elem 仍是 byte）；[]myByte 不可。
		if va.Type().Elem() != reflect.TypeOf(byte(0)) {
			return 0
		}
	case reflect.Slice:
		// copy(dst, src []T)：只比元素类型，不比切片命名类型。
		if va.Type().Elem() != vb.Type().Elem() {
			return 0
		}
	default:
		return 0
	}
	return reflect.Copy(va, vb)
}

// Append 模拟内置 append 函数，支持任意切片类型和追加元素。
// 参数 a 必须是一个切片（可以是 nil 切片），vals 为待追加的值。
// 返回的新切片类型与原切片相同，若类型不匹配或 a 不是切片则会 panic。
func Append(a any, vals ...any) any {
	v := inDirect(asValue(a))
	if v.Kind() != reflect.Slice {
		panic("Append: first argument must be a slice")
	}
	elemType := v.Type().Elem()
	elems := make([]reflect.Value, len(vals))
	for i, val := range vals {
		cv, err := autoConvert(elemType, val)
		if err != nil {
			panic("Append: " + err.Error())
		}
		elems[i] = cv
	}
	return reflect.Append(v, elems...).Interface()
}
