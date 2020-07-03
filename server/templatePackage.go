﻿package server
import(
	"fmt"
    "reflect"
    "bytes"
    "net"
    "net/http"
    "net/url"
    "bufio"
    "strings"
    "strconv"
    "encoding/asn1"
    "encoding/json"
    "github.com/456vv/vmap/v2"
    "github.com/456vv/vconnpool"
    "github.com/456vv/vforward"
    "github.com/456vv/vconn"
    "github.com/456vv/vbody"
    "github.com/456vv/vcipher"
    "github.com/456vv/vweb/v2"
    "github.com/456vv/vweb/v2/builtin"
    "github.com/456vv/verifycode"
    "regexp"
    "unicode"
    "unicode/utf8"
    "os"
    "io"
    "io/ioutil"
    "context"
    "time"
    "crypto"
    "crypto/aes"
    "crypto/cipher"
    "crypto/des"
    "crypto/dsa"
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/hmac"
    "crypto/rand"
    "crypto/rsa"
    "crypto/tls"
    "crypto/x509"
    "crypto/x509/pkix"
    "crypto/sha1"
    "crypto/sha256"
    "crypto/sha512"
    "math/big"
    "unsafe"
    "path/filepath"
    "path"
   	"sync"
   	"sync/atomic"
   	"errors"
   	"log"
)


var templatePackage = map[string]map[string]interface{}{
	"vweb":{
		"AddSalt":vweb.AddSalt,
		"CopyStruct":vweb.CopyStruct,
		"CopyStructDeep":vweb.CopyStructDeep,
		"DepthField":vweb.DepthField,
		"ForMethod":vweb.ForMethod,
		"ForType":vweb.ForType,
		"GenerateRandom":vweb.GenerateRandom,
		"GenerateRandomId":vweb.GenerateRandomId,
		"GenerateRandomString":vweb.GenerateRandomString,
		"InDirect":vweb.InDirect,
		"PagePath":vweb.PagePath,
		"TypeSelect":vweb.TypeSelect,
		"Cookie":func(a ...interface{}) (retn *vweb.Cookie) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Forward":func(a ...interface{}) (retn *vweb.Forward) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PluginHTTPClient":func(a ...interface{}) (retn *vweb.PluginHTTPClient) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PluginRPCClient":func(a ...interface{}) (retn *vweb.PluginRPCClient) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Route":func(a ...interface{}) (retn *vweb.Route) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ServerHandlerDynamic":func(a ...interface{}) (retn *vweb.ServerHandlerDynamic) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ServerHandlerStatic":func(a ...interface{}) (retn *vweb.ServerHandlerStatic) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Session":func(a ...interface{}) (retn *vweb.Session) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewSession":vweb.NewSession,
		"Sessions":func(a ...interface{}) (retn *vweb.Sessions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Site":func(a ...interface{}) (retn *vweb.Site) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SiteMan":func(a ...interface{}) (retn *vweb.SiteMan) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SitePool":func(a ...interface{}) (retn *vweb.SitePool) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewSitePool":vweb.NewSitePool,
		"TemplateDot":func(a ...interface{}) (retn *vweb.TemplateDot) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"vconnpool":{
		"ConnPool":func(a ...interface{}) (retn *vconnpool.ConnPool) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"vconn":{
		"NewConn":vconn.NewConn,
		"CloseNotifier":func(a ...interface{}) (retn vconn.CloseNotifier) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"vbody":{
		"NewReader":vbody.NewReader,
		"NewWriter":vbody.NewWriter,
	},
	"vcipher":{
		"AES":vcipher.AES,
		"NewCipher":vcipher.NewCipher,
	},
	"vmap":{
		"NewMap":vmap.NewMap,
	},
	"vforward":{
		"Addr":func(a ...interface{}) (retn *vforward.Addr) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"D2D":func(a ...interface{}) (retn *vforward.D2D) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"L2D":func(a ...interface{}) (retn *vforward.L2D) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"L2L":func(a ...interface{}) (retn *vforward.L2L) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"verifycode":{
		"Rand":verifycode.Rand,
		"RandRange":verifycode.RandRange,
		"RandomText":verifycode.RandomText,
		"Color":func(a ...interface{}) (retn *verifycode.Color) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Font":func(a ...interface{}) (retn *verifycode.Font) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Style":func(a ...interface{}) (retn *verifycode.Style) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Glyph":func(a ...interface{}) (retn *verifycode.Glyph) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"VerifyCode":func(a ...interface{}) (retn *verifycode.VerifyCode) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"errors":{
		"As":errors.As,
		"Is":errors.Is,
		"New":errors.New,
		"Unwrap":errors.Unwrap,
	},
	"sync":{
		"Map":func(a ...interface{}) (retn *sync.Map) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"sync/atomic":{
		"AddInt32":atomic.AddInt32,
		"AddInt64":atomic.AddInt64,
		"AddUint32":atomic.AddUint32,
		"AddUint64":atomic.AddUint64,
		"AddUintptr":atomic.AddUintptr,
		"CompareAndSwapInt32":atomic.CompareAndSwapInt32,
		"CompareAndSwapInt64":atomic.CompareAndSwapInt64,
		"CompareAndSwapPointer":atomic.CompareAndSwapPointer,
		"CompareAndSwapUint32":atomic.CompareAndSwapUint32,
		"CompareAndSwapUint64":atomic.CompareAndSwapUint64,
		"CompareAndSwapUintptr":atomic.CompareAndSwapUintptr,
		"LoadInt32":atomic.LoadInt32,
		"LoadInt64":atomic.LoadInt64,
		"LoadPointer":atomic.LoadPointer,
		"LoadUint32":atomic.LoadUint32,
		"LoadUint64":atomic.LoadUint64,
		"LoadUintptr":atomic.LoadUintptr,
		"StoreInt32":atomic.StoreInt32,
		"StoreInt64":atomic.StoreInt64,
		"StorePointer":atomic.StorePointer,
		"StoreUint32":atomic.StoreUint32,
		"StoreUint64":atomic.StoreUint64,
		"StoreUintptr":atomic.StoreUintptr,
		"SwapInt32":atomic.SwapInt32,
		"SwapInt64":atomic.SwapInt64,
		"SwapPointer":atomic.SwapPointer,
		"SwapUint32":atomic.SwapUint32,
		"SwapUint64":atomic.SwapUint64,
		"SwapUintptr":atomic.SwapUintptr,
		"Value":func(a ...interface{}) (retn *atomic.Value) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"path":{
		"Base":path.Base,
		"Clean":path.Clean,
		"Dir":path.Dir,
		"Ext":path.Ext,
		"IsAbs":path.IsAbs,
		"Join":path.Join,
		"Split":path.Split,
		"Match":path.Match,
	},
	"path/filepath":{
		"Separator":filepath.Separator,
		"ListSeparator":filepath.ListSeparator,
		"Abs":filepath.Abs,
		"Rel":filepath.Rel,
		"Base":filepath.Base,
		"Clean":filepath.Clean,
		"Dir":filepath.Dir,
		"EvalSymlinks":filepath.EvalSymlinks,
		"Ext":filepath.Ext,
		"FromSlash":filepath.FromSlash,
		"ToSlash":filepath.ToSlash,
		"Glob":filepath.Glob,
		"HasPrefix":filepath.HasPrefix,
		"IsAbs":filepath.IsAbs,
		"Join":filepath.Join,
		"Match":filepath.Match,
		"Split":filepath.Split,
		"SplitList":filepath.SplitList,
		"VolumeName":filepath.VolumeName,
	},
	"fmt":{
		"Errorf":fmt.Errorf,
		"Fprint":fmt.Fprint,
		"Fprintf":fmt.Fprintf,
		"Fprintln":fmt.Fprintln,
		"Sprint":fmt.Sprint,
		"Sprintf":fmt.Sprintf,
		"Sprintln":fmt.Sprintln,
	},
    "reflect":{
        "Copy":reflect.Copy,
        "DeepEqual":reflect.DeepEqual,
        "Select":reflect.Select,
        "Swapper":reflect.Swapper,
		"ChanDir":func(ChanDir int) reflect.ChanDir {return reflect.ChanDir(ChanDir)},
        "RecvDir":reflect.RecvDir,
        "SendDir":reflect.SendDir,
        "BothDir":reflect.BothDir,
		"Kind":func(Kind uint) reflect.Kind {return reflect.Kind(Kind)},
        "Invalid":reflect.Invalid,
        "Bool":reflect.Bool,
        "Int":reflect.Int,
        "Int8":reflect.Int8,
        "Int16":reflect.Int16,
        "Int32":reflect.Int32,
        "Int64":reflect.Int64,
        "Uint":reflect.Uint,
        "Uint8":reflect.Uint8,
        "Uint16":reflect.Uint16,
        "Uint32":reflect.Uint32,
        "Uint64":reflect.Uint64,
        "Uintptr":reflect.Uintptr,
        "Float32":reflect.Float32,
        "Float64":reflect.Float64,
        "Complex64":reflect.Complex64,
        "Complex128":reflect.Complex128,
        "Array":reflect.Array,
        "Chan":reflect.Chan,
        "Func":reflect.Func,
        "Interface":reflect.Interface,
        "Map":reflect.Map,
        "Ptr":reflect.Ptr,
        "Slice":reflect.Slice,
        "String":reflect.String,
        "Struct":reflect.Struct,
        "UnsafePointer":reflect.UnsafePointer,
		"Method":func(a ...interface{}) (retn *reflect.Method) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SelectCase":func(a ...interface{}) (retn *reflect.SelectCase) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SelectDir":func(SelectDir int) reflect.SelectDir {return reflect.SelectDir(SelectDir)},
		"SelectSend":reflect.SelectSend,
		"SelectRecv":reflect.SelectRecv,
		"SelectDefault":reflect.SelectDefault,
		"SliceHeader":func(a ...interface{}) (retn *reflect.SliceHeader) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"StringHeader":func(a ...interface{}) (retn *reflect.StringHeader) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"StructField":func(a ...interface{}) (retn *reflect.StructField) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"StructTag":func(StructTag string) reflect.StructTag {return reflect.StructTag(StructTag)},
		"ArrayOf":reflect.ArrayOf,
        "ChanOf":reflect.ChanOf,
        "FuncOf":reflect.FuncOf,
        "MapOf":reflect.MapOf,
        "PtrTo":reflect.PtrTo,
        "SliceOf":reflect.SliceOf,
        "StructOf":reflect.StructOf,
        "TypeOf":reflect.TypeOf,
		"Value":func(a ...interface{}) (retn *reflect.Value) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
        "Append":reflect.Append,
        "AppendSlice":reflect.AppendSlice,
        "Indirect":reflect.Indirect,
        "MakeChan":reflect.MakeChan,
        "MakeFunc":reflect.MakeFunc,
        "MakeMap":reflect.MakeMap,
        "MakeMapWithSize":reflect.MakeMapWithSize,
        "MakeSlice":reflect.MakeSlice,
        "New":reflect.New,
        "NewAt":reflect.NewAt,
        "ValueOf":reflect.ValueOf,
        "Zero":reflect.Zero,
    },
    "unsafe":{
    	"Uintptr":func(Pointer unsafe.Pointer) uintptr {return uintptr(Pointer)},
		"Pointer":func(Pointer uintptr) unsafe.Pointer {return unsafe.Pointer(Pointer)},
		"Alignof":func(Pointer uintptr) uintptr {return unsafe.Alignof(Pointer)},
		"Sizeof":func(Pointer uintptr) uintptr {return unsafe.Sizeof(Pointer)},
    },
    "context":{
		"CancelFunc":func(CancelFunc func()) context.CancelFunc {return context.CancelFunc(CancelFunc)},
    	"Background":context.Background,
    	"TODO":context.TODO,
    	"WithCancel":context.WithCancel,
    	"WithDeadline":context.WithDeadline,
    	"WithTimeout":context.WithTimeout,
    	"WithValue":context.WithValue,
    },
    "time":{
		"ANSIC":time.ANSIC,
		"UnixDate":time.UnixDate,
		"RubyDate":time.RubyDate,
		"RFC822":time.RFC822,
		"RFC822Z":time.RFC822Z,
		"RFC850":time.RFC850,
		"RFC1123":time.RFC1123,
		"RFC1123Z":time.RFC1123Z,
		"RFC3339":time.RFC3339,
		"RFC3339Nano":time.RFC3339Nano,
		"Kitchen":time.Kitchen,
		"Stamp":time.Stamp,
		"StampMilli":time.StampMilli,
		"StampMicro":time.StampMicro,
		"StampNano":time.StampNano,
		"Duration":func(Duration int64) time.Duration {return time.Duration(Duration)},
		"Nanosecond":time.Nanosecond,
		"Microsecond":time.Microsecond,
		"Millisecond":time.Millisecond,
		"Second":time.Second,
		"Minute":time.Minute,
		"Hour":time.Hour,
		"After":time.After,
		"Sleep":time.Sleep,
		"Tick":time.Tick,
		"ParseDuration":time.ParseDuration,
		"Since":time.Since,
		"Until":time.Until,
		"Location":func(a ...interface{}) (retn *time.Location) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Local":time.Local,
		"UTC":time.UTC,
		"FixedZone":time.FixedZone,
		"LoadLocation":time.LoadLocation,
		"LoadLocationFromTZData":time.LoadLocationFromTZData,
		"Month":func(Month int) time.Month {return time.Month(Month)},
		"Ticker":func(a ...interface{}) (retn *time.Ticker) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewTicker":time.NewTicker,
		"Time":func(a ...interface{}) (retn *time.Time) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Date":time.Date,
		"Now":time.Now,
		"Parse":time.Parse,
		"ParseInLocation":time.ParseInLocation,
		"Unix":time.Unix,
		"Timer":func(a ...interface{}) (retn *time.Timer) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"AfterFunc":time.AfterFunc,
		"NewTimer":time.NewTimer,
		"Weekday":func(Weekday int) time.Weekday {return time.Weekday(Weekday)},
    },
    "net":{
		"DefaultResolver":net.DefaultResolver,
		"InterfaceAddrs":net.InterfaceAddrs,
		"Interfaces":net.Interfaces,
		"JoinHostPort":net.JoinHostPort,
		"LookupAddr":net.LookupAddr,
		"LookupCNAME":net.LookupCNAME,
		"LookupHost":net.LookupHost,
		"LookupIP":net.LookupIP,
		"LookupMX":net.LookupMX,
		"LookupNS":net.LookupNS,
		"LookupPort":net.LookupPort,
		"LookupSRV":net.LookupSRV,
		"LookupTXT":net.LookupTXT,
		"SplitHostPort":net.SplitHostPort,
		"Buffers":func(Buffers [][]byte) net.Buffers {return Buffers},
		"Dial":net.Dial,
		"DialTimeout":net.DialTimeout,
		"Dialer":func(a ...interface{}) (retn *net.Dialer) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Flags":func(Flags uint) net.Flags {return net.Flags(Flags)},
		"HardwareAddr":func(HardwareAddr []byte) net.HardwareAddr {return HardwareAddr},
		"ParseMAC":net.ParseMAC,
		"IP":func(IP []byte) net.IP {return IP},
		"IPv4":net.IPv4,
		"ParseCIDR":net.ParseCIDR,
		"ParseIP":net.ParseIP,
		"IPAddr":func(a ...interface{}) (retn *net.IPAddr) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ResolveIPAddr":net.ResolveIPAddr,
		"IPConn":func(a ...interface{}) (retn *net.IPConn) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"DialIP":net.DialIP,
		"IPMask":func(IPMask []byte) net.IPMask {return IPMask},
		"CIDRMask":net.CIDRMask,
		"IPv4Mask":net.IPv4Mask,
		"IPNet":func(a ...interface{}) (retn *net.IPNet) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Interface":func(a ...interface{}) (retn *net.Interface) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"InterfaceByIndex":net.InterfaceByIndex,
		"InterfaceByName":net.InterfaceByName,
		"MX":func(a ...interface{}) (retn *net.MX) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NS":func(a ...interface{}) (retn *net.NS) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Resolver":func(a ...interface{}) (retn *net.Resolver) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SRV":func(a ...interface{}) (retn *net.SRV) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"TCPAddr":func(a ...interface{}) (retn *net.TCPAddr) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ResolveTCPAddr":net.ResolveTCPAddr,
		"TCPConn":func(a ...interface{}) (retn *net.TCPConn) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"DialTCP":net.DialTCP,
		"UDPAddr":func(a ...interface{}) (retn *net.UDPAddr) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ResolveUDPAddr":net.ResolveUDPAddr,
		"UDPConn":func(a ...interface{}) (retn *net.UDPConn) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"DialUDP":net.DialUDP,
		"ListenMulticastUDP":net.ListenMulticastUDP,
 		"ListenUDP":net.ListenUDP,
   },
	"net/http":{
		"LocalAddrContextKey":http.LocalAddrContextKey,
		"ServerContextKey":http.ServerContextKey,
		"NoBody":http.NoBody,
		"DefaultClient":http.DefaultClient,
		"DefaultTransport":http.DefaultTransport,
		"Client":func(a ...interface{}) (retn *http.Client) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ConnState":func(ConnState int) http.ConnState {return http.ConnState(ConnState)},
		"Cookie":func(a ...interface{}) (retn *http.Cookie) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SameSite":func(SameSite int) http.SameSite {return http.SameSite(SameSite)},
		"Header":func(a ...interface{}) (retn *http.Header) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PushOptions":func(a ...interface{}) (retn *http.PushOptions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Request":func(a ...interface{}) (retn *http.Request) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewRequest":http.NewRequest,
		"ReadRequest":http.ReadRequest,
		"Response":func(a ...interface{}) (retn *http.Response) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Get":http.Get,
		"Head":http.Head,
		"Post":http.Post,
		"PostForm":http.PostForm,
		"ReadResponse":http.ReadResponse,
		"Transport":func(a ...interface{}) (retn *http.Transport) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto":{
		"RegisterHash":crypto.RegisterHash,
		"Hash":func(Hash int) crypto.Hash {return crypto.Hash(Hash)},
	},
	"crypto/aes":{
		"NewCipher":aes.NewCipher,
	},
	"crypto/des":{
		"NewCipher":des.NewCipher,
		"NewTripleDESCipher":des.NewTripleDESCipher,
	},
	"crypto/dsa":{
		"GenerateKey":dsa.GenerateKey,
		"GenerateParameters":dsa.GenerateParameters,
		"Sign":dsa.Sign,
		"Verify":dsa.Verify,
		"ParameterSizes":func(ParameterSizes int) dsa.ParameterSizes {return dsa.ParameterSizes(ParameterSizes)},
		"Parameters":func(a ...interface{}) (retn *dsa.Parameters) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PrivateKey":func(a ...interface{}) (retn *dsa.PrivateKey) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PublicKey":func(a ...interface{}) (retn *dsa.PublicKey) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/rsa":{
		"DecryptOAEP":rsa.DecryptOAEP,
		"DecryptPKCS1v15":rsa.DecryptPKCS1v15,
		"DecryptPKCS1v15SessionKey":rsa.DecryptPKCS1v15SessionKey,
		"EncryptOAEP":rsa.EncryptOAEP,
		"EncryptPKCS1v15":rsa.EncryptPKCS1v15,
		"SignPKCS1v15":rsa.SignPKCS1v15,
		"SignPSS":rsa.SignPSS,
		"VerifyPKCS1v15":rsa.VerifyPKCS1v15,
		"VerifyPSS":rsa.VerifyPSS,
		"CRTValue":func(a ...interface{}) (retn *rsa.CRTValue) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"OAEPOptions":func(a ...interface{}) (retn *rsa.OAEPOptions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PKCS1v15DecryptOptions":func(a ...interface{}) (retn *rsa.PKCS1v15DecryptOptions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PSSOptions":func(a ...interface{}) (retn *rsa.PSSOptions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PrecomputedValues":func(a ...interface{}) (retn *rsa.PrecomputedValues) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PrivateKey":func(a ...interface{}) (retn *rsa.PrivateKey) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"GenerateKey":rsa.GenerateKey,
		"GenerateMultiPrimeKey":rsa.GenerateMultiPrimeKey,
		"PublicKey":func(a ...interface{}) (retn *rsa.PublicKey) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/ecdsa":{
		"Sign":ecdsa.Sign,
		"Verify":ecdsa.Verify,
		"GenerateKey":ecdsa.GenerateKey,
		"PrivateKey":func(a ...interface{}) (retn *ecdsa.PrivateKey) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"PublicKey":func(a ...interface{}) (retn *ecdsa.PublicKey) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/elliptic":{
		"GenerateKey":elliptic.GenerateKey,
		"Marshal":elliptic.Marshal,
		"Unmarshal":elliptic.Unmarshal,
		"P224":elliptic.P224,
		"P256":elliptic.P256,
		"P384":elliptic.P384,
		"P521":elliptic.P521,
		"CurveParams":func(a ...interface{}) (retn *elliptic.CurveParams) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/hmac":{
		"Equal":hmac.Equal,
		"New":hmac.New,
	},
	"crypto/rand":{
		"Reader":rand.Reader,
		"Int":rand.Int,
		"Prime":rand.Prime,
		"Read":rand.Read,
	},
	"crypto/cipher":{
		"NewGCMWithTagSize":cipher.NewGCMWithTagSize,
		"NewGCM":cipher.NewGCM,
		"NewGCMWithNonceSize":cipher.NewGCMWithNonceSize,
		"NewCBCDecrypter":cipher.NewCBCDecrypter,
		"NewCBCEncrypter":cipher.NewCBCEncrypter,
		"NewCFBDecrypter":cipher.NewCFBDecrypter,
		"NewCFBEncrypter":cipher.NewCFBEncrypter,
		"NewCTR":cipher.NewCTR,
		"NewOFB":cipher.NewOFB,
		"StreamReader":func(a ...interface{}) (retn *cipher.StreamReader) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"StreamWriter":func(a ...interface{}) (retn *cipher.StreamWriter) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/tls":{
		"Certificate":func(a ...interface{}) (retn *tls.Certificate) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"LoadX509KeyPair":tls.LoadX509KeyPair,
		"X509KeyPair":tls.X509KeyPair,
		"CertificateRequestInfo":func(a ...interface{}) (retn *tls.CertificateRequestInfo) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ClientAuthType":func(ClientAuthType int) tls.ClientAuthType {return tls.ClientAuthType(ClientAuthType)},
		"ClientHelloInfo":func(a ...interface{}) (retn *tls.ClientHelloInfo) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewLRUClientSessionCache":tls.NewLRUClientSessionCache,
		"ClientSessionState":func(a ...interface{}) (retn *tls.ClientSessionState) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Config":func(a ...interface{}) (retn *tls.Config) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Conn":func(a ...interface{}) (retn *tls.Conn) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Client":tls.Client,
		"Dial":tls.Dial,
		"DialWithDialer":tls.DialWithDialer,
		"ConnectionState":func(a ...interface{}) (retn *tls.ConnectionState) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"CurveID":func(CurveID uint16) tls.CurveID {return tls.CurveID(CurveID)},
		"RenegotiationSupport":func(RenegotiationSupport int) tls.RenegotiationSupport {return tls.RenegotiationSupport(RenegotiationSupport)},
		"SignatureScheme":func(SignatureScheme uint16) tls.SignatureScheme {return tls.SignatureScheme(SignatureScheme)},
		"CipherSuiteName":tls.CipherSuiteName,
		"CipherSuites":tls.CipherSuites,
		"InsecureCipherSuites":tls.InsecureCipherSuites,
   	},
   	"crypto/x509":{
		"CreateCertificate":x509.CreateCertificate,
		"CreateCertificateRequest":x509.CreateCertificateRequest,
		"DecryptPEMBlock":x509.DecryptPEMBlock,
		"EncryptPEMBlock":x509.EncryptPEMBlock,
		"IsEncryptedPEMBlock":x509.IsEncryptedPEMBlock,
		"MarshalECPrivateKey":x509.MarshalECPrivateKey,
		"MarshalPKCS1PublicKey":x509.MarshalPKCS1PublicKey,
		"MarshalPKCS1PrivateKey":x509.MarshalPKCS1PrivateKey,
		"MarshalPKIXPublicKey":x509.MarshalPKIXPublicKey,
		"MarshalPKCS8PrivateKey":x509.MarshalPKCS8PrivateKey,
		"ParseCRL":x509.ParseCRL,
		"ParseCertificates":x509.ParseCertificates,
		"ParseDERCRL":x509.ParseDERCRL,
		"ParseECPrivateKey":x509.ParseECPrivateKey,
		"ParsePKCS1PublicKey":x509.ParsePKCS1PublicKey,
		"ParsePKCS1PrivateKey":x509.ParsePKCS1PrivateKey,
		"ParsePKCS8PrivateKey":x509.ParsePKCS8PrivateKey,
		"ParsePKIXPublicKey":x509.ParsePKIXPublicKey,
		"CertPool":func(a ...interface{}) (retn *x509.CertPool) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewCertPool":x509.NewCertPool,
		"SystemCertPool":x509.SystemCertPool,
		"Certificate":func(a ...interface{}) (retn *x509.Certificate) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ParseCertificate":x509.ParseCertificate,
		"CertificateRequest":func(a ...interface{}) (retn *x509.CertificateRequest) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"ParseCertificateRequest":x509.ParseCertificateRequest,
		"KeyUsage":func(KeyUsage int) x509.KeyUsage {return x509.KeyUsage(KeyUsage)},
		"PEMCipher":func(PEMCipher int) x509.PEMCipher {return x509.PEMCipher(PEMCipher)},
		"PublicKeyAlgorithm":func(PublicKeyAlgorithm int) x509.PublicKeyAlgorithm {return x509.PublicKeyAlgorithm(PublicKeyAlgorithm)},
		"SignatureAlgorithm":func(SignatureAlgorithm int) x509.SignatureAlgorithm {return x509.SignatureAlgorithm(SignatureAlgorithm)},
		"VerifyOptions":func(a ...interface{}) (retn *x509.VerifyOptions) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/x509/pkix":{
		"AlgorithmIdentifier":func(a ...interface{}) (retn *pkix.AlgorithmIdentifier) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"AttributeTypeAndValue":func(a ...interface{}) (retn *pkix.AttributeTypeAndValue) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"AttributeTypeAndValueSET":func(a ...interface{}) (retn *pkix.AttributeTypeAndValueSET) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"CertificateList":func(a ...interface{}) (retn *pkix.CertificateList) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Extension":func(a ...interface{}) (retn *pkix.Extension) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Name":func(a ...interface{}) (retn *pkix.Name) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"RDNSequence":func(RDNSequence []pkix.RelativeDistinguishedNameSET) pkix.RDNSequence {return RDNSequence},
		"RelativeDistinguishedNameSET":func(RelativeDistinguishedNameSET []pkix.AttributeTypeAndValue) pkix.RelativeDistinguishedNameSET {return RelativeDistinguishedNameSET},
		"RevokedCertificate":func(a ...interface{}) (retn *pkix.RevokedCertificate) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"TBSCertificateList":func(a ...interface{}) (retn *pkix.TBSCertificateList) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"crypto/sha1":{
		"New":sha1.New,
		"Sum":sha1.Sum,
	},
	"crypto/sha256":{
		"New":sha256.New,
		"New224":sha256.New224,
		"Sum224":sha256.Sum224,
		"Sum256":sha256.Sum256,
	},
	"crypto/sha512":{
		"New":sha512.New,
		"New384":sha512.New384,
		"New512_224":sha512.New512_224,
		"New512_256":sha512.New512_256,
		"Sum384":sha512.Sum384,
		"Sum512":sha512.Sum512,
		"Sum512_224":sha512.Sum512_224,
		"Sum512_256":sha512.Sum512_256,
	},
	"encoding/asn1":{
		"MarshalWithParams":asn1.MarshalWithParams,
		"Marshal":asn1.Marshal,
		"Unmarshal":asn1.Unmarshal,
		"UnmarshalWithParams":asn1.UnmarshalWithParams,
		"BitString":func(a ...interface{}) (retn *asn1.BitString) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Flag":func(Flag bool) asn1.Flag {return asn1.Flag(Flag)},
		"ObjectIdentifier":func(ObjectIdentifier []int) asn1.ObjectIdentifier {return ObjectIdentifier},
		"RawContent":func(RawContent []byte) asn1.RawContent {return RawContent},
		"RawValue":func(a ...interface{}) (retn *asn1.RawValue) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
	},
	"math/big":{
		"Jacobi":big.Jacobi,
		"Accuracy":func(Accuracy int8) big.Accuracy {return big.Accuracy(Accuracy)},
		"Float":func(a ...interface{}) (retn *big.Float) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewFloat":big.NewFloat,
		"ParseFloat":big.ParseFloat,
		"Int":func(a ...interface{}) (retn *big.Int) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewInt":big.NewInt,
		"Rat":func(a ...interface{}) (retn *big.Rat) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewRat":big.NewRat,
		"RoundingMode":func(RoundingMode byte) big.RoundingMode {return big.RoundingMode(RoundingMode)},
		"Word":func(Word uint) big.Word {return big.Word(Word)},
	},
	"bufio":{
		"ScanBytes":bufio.ScanBytes,
		"ScanLines":bufio.ScanLines,
		"ScanRunes":bufio.ScanRunes,
		"ScanWords":bufio.ScanWords,
		"ReadWriter":func(a ...interface{}) (retn *bufio.ReadWriter) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewReadWriter":bufio.NewReadWriter,
		"Reader":func(a ...interface{}) (retn *bufio.Reader) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewReader":bufio.NewReader,
		"NewReaderSize":bufio.NewReaderSize,
		"Writer":func(a ...interface{}) (retn *bufio.Writer) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"NewWriter":bufio.NewWriter,
		"NewWriterSize":bufio.NewWriterSize,
		"Scanner":func(a ...interface{}) (retn *bufio.Scanner) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"SplitFunc":func(SplitFunc func(data []byte, atEOF bool) (advance int, token []byte, err error)) bufio.SplitFunc {return bufio.SplitFunc(SplitFunc)},
		"NewScanner":bufio.NewScanner,
	},
	"url":{
		"PathEscape":url.PathEscape,
		"PathUnescape":url.PathUnescape,
		"QueryEscape":url.QueryEscape,
		"QueryUnescape":url.QueryUnescape,
		"URL":func(a ...interface{}) (retn *url.URL) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"Parse":url.Parse,
		"ParseRequestURI":url.ParseRequestURI,
		"Userinfo":func(a ...interface{}) (retn *url.Userinfo) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"User":url.User,
		"UserPassword":url.UserPassword,
		"Values":func(Values map[string][]string) url.Values {return Values},
		"ParseQuery":url.ParseQuery,
	},
    "strings":{
    	"ReplaceAll":strings.ReplaceAll,
    	"Compare":strings.Compare,
        "Contains":strings.Contains,
        "ContainsAny":strings.ContainsAny,
        "ContainsRune":strings.ContainsRune,
        "Count":strings.Count,
        "EqualFold":strings.EqualFold,
        "Fields":strings.Fields,
        "FieldsFunc":strings.FieldsFunc,
        "HasPrefix":strings.HasPrefix,
        "HasSuffix":strings.HasSuffix,
        "Index":strings.Index,
        "IndexAny":strings.IndexAny,
        "IndexByte":strings.IndexByte,
        "IndexFunc":strings.IndexFunc,
        "IndexRune":strings.IndexRune,
        "LastIndex":strings.LastIndex,
        "LastIndexAny":strings.LastIndexAny,
        "LastIndexByte":strings.LastIndexByte,
        "LastIndexFunc":strings.LastIndexFunc,
        "Map":strings.Map,
        "Repeat":strings.Repeat,
        "Replace":strings.Replace,
        "Join":strings.Join,
        "Split":strings.Split,
        "SplitN":strings.SplitN,
        "SplitAfter":strings.SplitAfter,
        "SplitAfterN":strings.SplitAfterN,
        "Title":strings.Title,
        "ToLower":strings.ToLower,
        "ToLowerSpecial":strings.ToLowerSpecial,
        "ToTitle":strings.ToTitle,
        "ToTitleSpecial":strings.ToTitleSpecial,
        "ToUpper":strings.ToUpper,
        "ToUpperSpecial":strings.ToUpperSpecial,
        "ToValidUTF8":strings.ToValidUTF8,
        "Trim":strings.Trim,
        "TrimFunc":strings.TrimFunc,
        "TrimLeft":strings.TrimLeft,
        "TrimPrefix":strings.TrimPrefix,
        "TrimLeftFunc":strings.TrimLeftFunc,
        "TrimRight":strings.TrimRight,
        "TrimSuffix":strings.TrimSuffix,
        "TrimRightFunc":strings.TrimRightFunc,
        "TrimSpace":strings.TrimSpace,
		"Reader":func(a ...interface{}) (retn *strings.Reader) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
        "NewReader":strings.NewReader,
		"Replacer":func(a ...interface{}) (retn *strings.Replacer) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
        "NewReplacer":strings.NewReplacer,
        "Builder":func(a ...interface{}) (retn *strings.Builder) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
    },
    "bytes":{
    	"ReplaceAll":bytes.ReplaceAll,
        "Compare":bytes.Compare,
        "Contains":bytes.Contains,
        "ContainsAny":bytes.ContainsAny,
        "ContainsRune":bytes.ContainsRune,
        "Count":bytes.Count,
        "Equal":bytes.Equal,
        "EqualFold":bytes.EqualFold,
        "Fields":bytes.Fields,
        "FieldsFunc":bytes.FieldsFunc,
        "HasPrefix":bytes.HasPrefix,
        "HasSuffix":bytes.HasSuffix,
        "Index":bytes.Index,
        "IndexAny":bytes.IndexAny,
        "IndexByte":bytes.IndexByte,
        "IndexFunc":bytes.IndexFunc,
        "IndexRune":bytes.IndexRune,
        "LastIndex":bytes.LastIndex,
        "LastIndexAny":bytes.LastIndexAny,
        "LastIndexByte":bytes.LastIndexByte,
        "LastIndexFunc":bytes.LastIndexFunc,
        "Map":bytes.Map,
        "Repeat":bytes.Repeat,
        "Replace":bytes.Replace,
        "Runes":bytes.Runes,
        "Join":bytes.Join,
        "Split":bytes.Split,
        "SplitN":bytes.SplitN,
        "SplitAfter":bytes.SplitAfter,
        "SplitAfterN":bytes.SplitAfterN,
        "Title":bytes.Title,
        "ToLower":bytes.ToLower,
        "ToLowerSpecial":bytes.ToLowerSpecial,
        "ToTitle":bytes.ToTitle,
        "ToTitleSpecial":bytes.ToTitleSpecial,
        "ToUpper":bytes.ToUpper,
        "ToUpperSpecial":bytes.ToUpperSpecial,
        "ToValidUTF8":bytes.ToValidUTF8,
        "Trim":bytes.Trim,
        "TrimFunc":bytes.TrimFunc,
        "TrimPrefix":bytes.TrimPrefix,
        "TrimLeft":bytes.TrimLeft,
        "TrimLeftFunc":bytes.TrimLeftFunc,
        "TrimSuffix":bytes.TrimSuffix,
        "TrimRight":bytes.TrimRight,
        "TrimRightFunc":bytes.TrimRightFunc,
        "TrimSpace":bytes.TrimSpace,
		"Buffer":func(a ...interface{}) (retn *bytes.Buffer) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
        "NewBuffer":bytes.NewBuffer,
        "NewBufferString":bytes.NewBufferString,
		"Reader":func(a ...interface{}) (retn *bytes.Reader) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
        "NewReader":bytes.NewReader,
    },
    "strconv":{
        "AppendBool":strconv.AppendBool,
        "AppendFloat":strconv.AppendFloat,
        "AppendInt":strconv.AppendInt,
        "AppendUint":strconv.AppendUint,
        "AppendQuote":strconv.AppendQuote,
        "AppendQuoteToASCII":strconv.AppendQuoteToASCII,
        "AppendQuoteRune":strconv.AppendQuoteRune,
        "AppendQuoteRuneToASCII":strconv.AppendQuoteRuneToASCII,
        "AppendQuoteRuneToGraphic":strconv.AppendQuoteRuneToGraphic,
        "AppendQuoteToGraphic":strconv.AppendQuoteToGraphic,
        "Atoi":strconv.Atoi,
        "Itoa":strconv.Itoa,
        "CanBackquote":strconv.CanBackquote,
        "FormatBool":strconv.FormatBool,
        "FormatFloat":strconv.FormatFloat,
        "FormatInt":strconv.FormatInt,
        "FormatUint":strconv.FormatUint,
        "IsGraphic":strconv.IsGraphic,
        "IsPrint":strconv.IsPrint,
        "ParseBool":strconv.ParseBool,
        "ParseFloat":strconv.ParseFloat,
        "ParseInt":strconv.ParseInt,
        "ParseUint":strconv.ParseUint,
        "Quote":strconv.Quote,
        "QuoteToASCII":strconv.QuoteToASCII,
        "QuoteToGraphic":strconv.QuoteToGraphic,
        "QuoteRune":strconv.QuoteRune,
        "QuoteRuneToASCII":strconv.QuoteRuneToASCII,
        "QuoteRuneToGraphic":strconv.QuoteRuneToGraphic,
        "Unquote":strconv.Unquote,
        "UnquoteChar":strconv.UnquoteChar,
    },
    "encoding/json":{
		"Compact":json.Compact,
		"Indent":json.Indent,
		"HTMLEscape":json.HTMLEscape,
		"Marshal":json.Marshal,
		"MarshalIndent":json.MarshalIndent,
		"Unmarshal":json.Unmarshal,
		"NewEncoder":json.NewEncoder,
		"NewDecoder":json.NewDecoder,
		"Valid":json.Valid,
	},
    "regexp":{
        "Match":regexp.Match,
        "MatchReader":regexp.MatchReader,
        "MatchString":regexp.MatchString,
        "QuoteMeta":regexp.QuoteMeta,
        "Compile":regexp.Compile,
        "CompilePOSIX":regexp.CompilePOSIX,
    },
    "unicode":{
    	"In":unicode.In,
        "Is":unicode.Is,
        "IsControl":unicode.IsControl,
        "IsDigit":unicode.IsDigit,
        "IsGraphic":unicode.IsGraphic,
        "IsPrint":unicode.IsPrint,
        "IsLetter":unicode.IsLetter,
        "IsLower":unicode.IsLower,
        "IsTitle":unicode.IsTitle,
        "IsUpper":unicode.IsUpper,
        "IsMark":unicode.IsMark,
        "IsNumber":unicode.IsNumber,
        "IsOneOf":unicode.IsOneOf,
        "IsPunct":unicode.IsPunct,
        "IsSpace":unicode.IsSpace,
        "IsSymbol":unicode.IsSymbol,
        "SimpleFold":unicode.SimpleFold,
        "To":unicode.To,
        "ToLower":unicode.ToLower,
        "ToTitle":unicode.ToTitle,
        "ToUpper":unicode.ToUpper,
    },
    "unicode/utf8":{
        "DecodeLastRune":utf8.DecodeLastRune,
        "DecodeLastRuneInString":utf8.DecodeLastRuneInString,
        "DecodeRune":utf8.DecodeRune,
        "DecodeRuneInString":utf8.DecodeRuneInString,
        "EncodeRune":utf8.EncodeRune,
        "FullRune":utf8.FullRune,
        "FullRuneInString":utf8.FullRuneInString,
        "RuneCount":utf8.RuneCount,
        "RuneCountInString":utf8.RuneCountInString,
        "RuneLen":utf8.RuneLen,
        "RuneStart":utf8.RuneStart,
        "Valid":utf8.Valid,
        "ValidRune":utf8.ValidRune,
        "ValidString":utf8.ValidString,
    },
    "io":{
		"EOF":io.EOF,
		
		"Copy":io.Copy,
		"CopyBuffer":io.CopyBuffer,
		"CopyN":io.CopyN,
		"ReadAtLeast":io.ReadAtLeast,
		"ReadFull":io.ReadFull,
		"WriteString":io.WriteString,
		"Pipe":io.Pipe,
		"LimitReader":io.LimitReader,
		"MultiReader":io.MultiReader,
		"TeeReader":io.TeeReader,
		"NewSectionReader":io.NewSectionReader,
		"MultiWriter":io.MultiWriter,
    },
    "io/ioutil":{
    	"Discard":ioutil.Discard,
    	"NopCloser":ioutil.NopCloser,
        "ReadAll":ioutil.ReadAll,
        "ReadFile":ioutil.ReadFile,
        "WriteFile":ioutil.WriteFile,
    },
    "os":{
    	"Stdin":os.Stdin,
    	"Stdout":os.Stdout,
    	"Stderr":os.Stderr,
    	"Args":os.Args,
    	"Chmod":os.Chmod,
    	"Chown":os.Chown,
    	"Chtimes":os.Chtimes,
    	"Environ":os.Environ,
    	"Setenv":os.Setenv,
    	"Getenv":os.Getenv,
    	"LookupEnv":os.LookupEnv,
    	"Getgroups":os.Getgroups,
    	"Getegid":os.Getegid,
    	"Getgid":os.Getgid,
    	"Geteuid":os.Geteuid,
    	"Getuid":os.Getuid,
    	"IsTimeout":os.IsTimeout,
    	"IsExist":os.IsExist,
    	"IsNotExist":os.IsNotExist,
    	"IsPermission":os.IsPermission,
    	"Link":os.Link,
    	"Readlink":os.Readlink,
    	"Symlink":os.Symlink,
    	"Mkdir":os.Mkdir,
    	"MkdirAll":os.MkdirAll,
    	"TempDir":os.TempDir,
    	"Remove":os.Remove,
    	"RemoveAll":os.RemoveAll,
    	"Rename":os.Rename,
    	"Truncate":os.Truncate,
    	"Create":os.Create,
    	"NewFile":os.NewFile,
    	"Open":os.Open,
    	"OpenFile":os.OpenFile,
    	"Lstat":os.Lstat,
    	"Stat":os.Stat,
    	"Pipe":os.Pipe,
    	"FileMode":func(FileMode uint32) os.FileMode {return os.FileMode(FileMode)},
    },
    "log":{
		"Print":log.Print,
		"Printf":log.Printf,
		"Println":log.Println,
		"SetFlags":log.SetFlags,
		"Flags":log.Flags,
		"SetPrefix":log.SetPrefix,
		"Prefix":log.Prefix,
		"SetOutput":log.SetOutput,
		"Output":log.Output,
		"Writer":log.Writer,
		"Logger":func(a ...interface{}) (retn *log.Logger) {builtin.GoTypeTo(reflect.ValueOf(&retn))(a...);return retn},
		"New":log.New,
    },
}

