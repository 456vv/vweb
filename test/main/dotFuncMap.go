﻿package main
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
    "math/big"
    "unsafe"
    "path/filepath"
)

var dotFuncMap = map[string]map[string]interface{}{
	"errors":{
		"New": errors.New,
	},
	"fmt":{
		"Errorf": fmt.Errorf,
		"Fprint": fmt.Fprint,
		"Fprintf": fmt.Fprintf,
		"Fprintln": fmt.Fprintln,
		"Sprint": fmt.Sprint,
		"Sprintf": fmt.Sprintf,
		"Sprintln":fmt.Sprintln,
	},
	"vmap":{
		"NewMap":vmap.NewMap,
	},
    "reflect":{
        "Copy": reflect.Copy,
        "DeepEqual": reflect.DeepEqual,
        "Select": reflect.Select,
        "Swapper": reflect.Swapper,
		"ChanDir": func(ChanDir int) reflect.ChanDir {return reflect.ChanDir(ChanDir)},
        "RecvDir": reflect.RecvDir,
        "SendDir": reflect.SendDir,
        "BothDir": reflect.BothDir,
		"Kind": func(Kind uint) reflect.Kind {return reflect.Kind(Kind)},
        "Invalid": reflect.Invalid,
        "Bool": reflect.Bool,
        "Int": reflect.Int,
        "Int8": reflect.Int8,
        "Int16": reflect.Int16,
        "Int32": reflect.Int32,
        "Int64": reflect.Int64,
        "Uint": reflect.Uint,
        "Uint8": reflect.Uint8,
        "Uint16": reflect.Uint16,
        "Uint32": reflect.Uint32,
        "Uint64": reflect.Uint64,
        "Uintptr": reflect.Uintptr,
        "Float32": reflect.Float32,
        "Float64": reflect.Float64,
        "Complex64": reflect.Complex64,
        "Complex128": reflect.Complex128,
        "Array": reflect.Array,
        "Chan": reflect.Chan,
        "Func": reflect.Func,
        "Interface": reflect.Interface,
        "Map": reflect.Map,
        "Ptr": reflect.Ptr,
        "Slice": reflect.Slice,
        "String": reflect.String,
        "Struct": reflect.Struct,
        "UnsafePointer": reflect.UnsafePointer,
		"Method": func() (reflect.Method, *reflect.Method) {r := reflect.Method{};return r, &r},
		"SelectCase": func() (reflect.SelectCase, *reflect.SelectCase) {r := reflect.SelectCase{};return r, &r},
		"SelectDir": func(SelectDir int) reflect.SelectDir {return reflect.SelectDir(SelectDir)},
		"SelectSend": reflect.SelectSend,
		"SelectRecv": reflect.SelectRecv,
		"SelectDefault": reflect.SelectDefault,
		"SliceHeader": func() (reflect.SliceHeader, *reflect.SliceHeader) {r := reflect.SliceHeader{};return r, &r},
		"StringHeader": func() (reflect.StringHeader, *reflect.StringHeader) {r := reflect.StringHeader{};return r, &r},
		"StructField": func() (reflect.StructField, *reflect.StructField) {r := reflect.StructField{};return r, &r},
		"StructTag": func(StructTag string) reflect.StructTag {return reflect.StructTag(StructTag)},
		"ArrayOf": reflect.ArrayOf,
        "ChanOf": reflect.ChanOf,
        "FuncOf": reflect.FuncOf,
        "MapOf": reflect.MapOf,
        "PtrTo": reflect.PtrTo,
        "SliceOf": reflect.SliceOf,
        "StructOf": reflect.StructOf,
        "TypeOf": reflect.TypeOf,
		"Value": func() (reflect.Value, *reflect.Value) {r := reflect.Value{};return r, &r},
        "Append": reflect.Append,
        "AppendSlice": reflect.AppendSlice,
        "Indirect": reflect.Indirect,
        "MakeChan": reflect.MakeChan,
        "MakeFunc": reflect.MakeFunc,
        "MakeMap": reflect.MakeMap,
        "MakeMapWithSize":reflect.MakeMapWithSize,
        "MakeSlice": reflect.MakeSlice,
        "New": reflect.New,
        "NewAt": reflect.NewAt,
        "ValueOf": reflect.ValueOf,
        "Zero": reflect.Zero,
    },
    "unsafe":{
		"Pointer": func(Pointer uintptr) unsafe.Pointer {return unsafe.Pointer(Pointer)},
    },
    "context":{
		"CancelFunc": func(CancelFunc func()) context.CancelFunc {return context.CancelFunc(CancelFunc)},
    	"Background": context.Background,
    	"TODO": context.TODO,
    	"WithCancel": context.WithCancel,
    	"WithDeadline": context.WithDeadline,
    	"WithTimeout": context.WithTimeout,
    	"WithValue": context.WithValue,
    },
    "time":{
		"ANSIC": time.ANSIC,
		"UnixDate": time.UnixDate,
		"RubyDate": time.RubyDate,
		"RFC822": time.RFC822,
		"RFC822Z": time.RFC822Z,
		"RFC850": time.RFC850,
		"RFC1123": time.RFC1123,
		"RFC1123Z": time.RFC1123Z,
		"RFC3339": time.RFC3339,
		"RFC3339Nano": time.RFC3339Nano,
		"Kitchen": time.Kitchen,
		"Stamp": time.Stamp,
		"StampMilli": time.StampMilli,
		"StampMicro": time.StampMicro,
		"StampNano": time.StampNano,
		"Duration": func(Duration int64) time.Duration {return time.Duration(Duration)},
		"Nanosecond": time.Nanosecond,
		"Microsecond": time.Microsecond,
		"Millisecond": time.Millisecond,
		"Second": time.Second,
		"Minute": time.Minute,
		"Hour": time.Hour,
		"After": time.After,
		"Sleep": time.Sleep,
		"Tick": time.Tick,
		"ParseDuration": time.ParseDuration,
		"Since": time.Since,
		"Until": time.Until,
		"Location": func() (time.Location, *time.Location) {r := time.Location{};return r, &r},
		"Local": time.Local,
		"UTC": time.UTC,
		"FixedZone": time.FixedZone,
		"LoadLocation": time.LoadLocation,
		"LoadLocationFromTZData": time.LoadLocationFromTZData,
		"Month": func(Month int) time.Month {return time.Month(Month)},
		"Ticker": func() (time.Ticker, *time.Ticker) {r := time.Ticker{};return r, &r},
		"NewTicker": time.NewTicker,
		"Time": func() (time.Time, *time.Time) {r := time.Time{};return r, &r},
		"Date": time.Date,
		"Now": time.Now,
		"Parse": time.Parse,
		"ParseInLocation": time.ParseInLocation,
		"Unix": time.Unix,
		"Timer": func() (time.Timer, *time.Timer) {r := time.Timer{};return r, &r},
		"AfterFunc": time.AfterFunc,
		"NewTimer": time.NewTimer,
		"Weekday": func(Weekday int) time.Weekday {return time.Weekday(Weekday)},
    },
    "net": {
		"DefaultResolver": net.DefaultResolver,
		"InterfaceAddrs": net.InterfaceAddrs,
		"Interfaces": net.Interfaces,
		"JoinHostPort": net.JoinHostPort,
		"LookupAddr": net.LookupAddr,
		"LookupCNAME": net.LookupCNAME,
		"LookupHost": net.LookupHost,
		"LookupIP": net.LookupIP,
		"LookupMX": net.LookupMX,
		"LookupNS": net.LookupNS,
		"LookupPort": net.LookupPort,
		"LookupSRV": net.LookupSRV,
		"LookupTXT": net.LookupTXT,
		"SplitHostPort": net.SplitHostPort,
		"Buffers": func(Buffers [][]byte) net.Buffers {return Buffers},
		"Dial": net.Dial,
		"DialTimeout": net.DialTimeout,
		"Dialer": func() (net.Dialer, *net.Dialer) {r := net.Dialer{};return r, &r},
		"Flags": func(Flags uint) net.Flags {return net.Flags(Flags)},
		"HardwareAddr": func(HardwareAddr []byte) net.HardwareAddr {return HardwareAddr},
		"ParseMAC": net.ParseMAC,
		"IP": func(IP []byte) net.IP {return IP},
		"IPv4": net.IPv4,
		"ParseCIDR": net.ParseCIDR,
		"ParseIP": net.ParseIP,
		"IPAddr": func() (net.IPAddr, *net.IPAddr) {r := net.IPAddr{};return r, &r},
		"ResolveIPAddr": net.ResolveIPAddr,
		"IPConn": func() (net.IPConn, *net.IPConn) {r := net.IPConn{};return r, &r},
		"DialIP": net.DialIP,
		"IPMask": func(IPMask []byte) net.IPMask {return IPMask},
		"CIDRMask": net.CIDRMask,
		"IPv4Mask": net.IPv4Mask,
		"IPNet": func() (net.IPNet, *net.IPNet) {r := net.IPNet{};return r, &r},
		"Interface": func() (net.Interface, *net.Interface) {r := net.Interface{};return r, &r},
		"InterfaceByIndex": net.InterfaceByIndex,
		"InterfaceByName": net.InterfaceByName,
		"MX": func() (net.MX, *net.MX) {r := net.MX{};return r, &r},
		"NS": func() (net.NS, *net.NS) {r := net.NS{};return r, &r},
		"Resolver": func() (net.Resolver, *net.Resolver) {r := net.Resolver{};return r, &r},
		"SRV": func() (net.SRV, *net.SRV) {r := net.SRV{};return r, &r},
		"TCPAddr": func() (net.TCPAddr, *net.TCPAddr) {r := net.TCPAddr{};return r, &r},
		"ResolveTCPAddr": net.ResolveTCPAddr,
		"TCPConn": func() (net.TCPConn, *net.TCPConn) {r := net.TCPConn{};return r, &r},
		"DialTCP": net.DialTCP,
		"UDPAddr": func() (net.UDPAddr, *net.UDPAddr) {r := net.UDPAddr{};return r, &r},
		"ResolveUDPAddr": net.ResolveUDPAddr,
		"UDPConn": func() (net.UDPConn, *net.UDPConn) {r := net.UDPConn{};return r, &r},
		"DialUDP": net.DialUDP,
    },
	"net/http":{
		"LocalAddrContextKey": http.LocalAddrContextKey,
		"ServerContextKey": http.ServerContextKey,
		"NoBody": http.NoBody,
		"DefaultClient": http.DefaultClient,
		"DefaultTransport": http.DefaultTransport,
		"Client": func() (http.Client, *http.Client) {r := http.Client{};return r, &r},
		"ConnState": func(ConnState int) http.ConnState {return http.ConnState(ConnState)},
		"Cookie": func() (http.Cookie, *http.Cookie) {r := http.Cookie{};return r, &r},
		"SameSite": func(SameSite int) http.SameSite {return http.SameSite(SameSite)},
		"Header": func() (http.Header, *http.Header) {r := http.Header{};return r, &r},
		"PushOptions": func() (http.PushOptions, *http.PushOptions) {r := http.PushOptions{};return r, &r},
		"Request": func() (http.Request, *http.Request) {r := http.Request{};return r, &r},
		"NewRequest": http.NewRequest,
		"ReadRequest": http.ReadRequest,
		"Response": func() (http.Response, *http.Response) {r := http.Response{};return r, &r},
		"Get": http.Get,
		"Head": http.Head,
		"Post": http.Post,
		"PostForm": http.PostForm,
		"ReadResponse": http.ReadResponse,
		"Transport": func() (http.Transport, *http.Transport) {r := http.Transport{};return r, &r},
	},
	"crypto":{
		"RegisterHash": crypto.RegisterHash,
		"Hash": func(Hash int) crypto.Hash {return crypto.Hash(Hash)},
	},
	"crypto/aes":{
		"NewCipher": aes.NewCipher,
	},
	"crypto/des":{
		"NewCipher": des.NewCipher,
		"NewTripleDESCipher": des.NewTripleDESCipher,
	},
	"crypto/dsa":{
		"GenerateKey": dsa.GenerateKey,
		"GenerateParameters": dsa.GenerateParameters,
		"Sign": dsa.Sign,
		"Verify": dsa.Verify,
		"ParameterSizes": func(ParameterSizes int) dsa.ParameterSizes {return dsa.ParameterSizes(ParameterSizes)},
		"Parameters": func() (dsa.Parameters, *dsa.Parameters) {r := dsa.Parameters{};return r, &r},
		"PrivateKey": func() (dsa.PrivateKey, *dsa.PrivateKey) {r := dsa.PrivateKey{};return r, &r},
		"PublicKey": func() (dsa.PublicKey, *dsa.PublicKey) {r := dsa.PublicKey{};return r, &r},
	},
	"crypto/rsa":{
		"DecryptOAEP": rsa.DecryptOAEP,
		"DecryptPKCS1v15": rsa.DecryptPKCS1v15,
		"DecryptPKCS1v15SessionKey": rsa.DecryptPKCS1v15SessionKey,
		"EncryptOAEP": rsa.EncryptOAEP,
		"EncryptPKCS1v15": rsa.EncryptPKCS1v15,
		"SignPKCS1v15": rsa.SignPKCS1v15,
		"SignPSS": rsa.SignPSS,
		"VerifyPKCS1v15": rsa.VerifyPKCS1v15,
		"VerifyPSS": rsa.VerifyPSS,
		"CRTValue": func() (rsa.CRTValue, *rsa.CRTValue) {r := rsa.CRTValue{};return r, &r},
		"OAEPOptions": func() (rsa.OAEPOptions, *rsa.OAEPOptions) {r := rsa.OAEPOptions{};return r, &r},
		"PKCS1v15DecryptOptions": func() (rsa.PKCS1v15DecryptOptions, *rsa.PKCS1v15DecryptOptions) {r := rsa.PKCS1v15DecryptOptions{};return r, &r},
		"PSSOptions": func() (rsa.PSSOptions, *rsa.PSSOptions) {r := rsa.PSSOptions{};return r, &r},
		"PrecomputedValues": func() (rsa.PrecomputedValues, *rsa.PrecomputedValues) {r := rsa.PrecomputedValues{};return r, &r},
		"PrivateKey": func() (rsa.PrivateKey, *rsa.PrivateKey) {r := rsa.PrivateKey{};return r, &r},
		"GenerateKey": rsa.GenerateKey,
		"GenerateMultiPrimeKey": rsa.GenerateMultiPrimeKey,
		"PublicKey": func() (rsa.PublicKey, *rsa.PublicKey) {r := rsa.PublicKey{};return r, &r},
	},
	"crypto/ecdsa":{
		"Sign": ecdsa.Sign,
		"Verify": ecdsa.Verify,
		"GenerateKey": ecdsa.GenerateKey,
		"PrivateKey": func() (ecdsa.PrivateKey, *ecdsa.PrivateKey) {r := ecdsa.PrivateKey{};return r, &r},
		"PublicKey": func() (ecdsa.PublicKey, *ecdsa.PublicKey) {r := ecdsa.PublicKey{};return r, &r},
	},
	"crypto/elliptic":{
		"GenerateKey": elliptic.GenerateKey,
		"Marshal": elliptic.Marshal,
		"Unmarshal": elliptic.Unmarshal,
		"P224": elliptic.P224,
		"P256": elliptic.P256,
		"P384": elliptic.P384,
		"P521": elliptic.P521,
		"CurveParams": func() (elliptic.CurveParams, *elliptic.CurveParams) {r := elliptic.CurveParams{};return r, &r},
	},
	"crypto/hmac":{
		"Equal": hmac.Equal,
		"New": hmac.New,
	},
	"crypto/rand":{
		"Reader": rand.Reader,
		"Int": rand.Int,
		"Prime": rand.Prime,
		"Read": rand.Read,
	},
	"crypto/cipher":{
		"NewGCMWithTagSize": cipher.NewGCMWithTagSize,
		"NewGCM": cipher.NewGCM,
		"NewGCMWithNonceSize": cipher.NewGCMWithNonceSize,
		"NewCBCDecrypter": cipher.NewCBCDecrypter,
		"NewCBCEncrypter": cipher.NewCBCEncrypter,
		"NewCFBDecrypter": cipher.NewCFBDecrypter,
		"NewCFBEncrypter": cipher.NewCFBEncrypter,
		"NewCTR": cipher.NewCTR,
		"NewOFB": cipher.NewOFB,
		"StreamReader": func() (cipher.StreamReader, *cipher.StreamReader) {r := cipher.StreamReader{};return r, &r},
		"StreamWriter": func() (cipher.StreamWriter, *cipher.StreamWriter) {r := cipher.StreamWriter{};return r, &r},
	},
	"crypto/tls":{
		"Certificate": func() (tls.Certificate, *tls.Certificate) {r := tls.Certificate{};return r, &r},
		"LoadX509KeyPair": tls.LoadX509KeyPair,
		"X509KeyPair": tls.X509KeyPair,
		"CertificateRequestInfo": func() (tls.CertificateRequestInfo, *tls.CertificateRequestInfo) {r := tls.CertificateRequestInfo{};return r, &r},
		"ClientAuthType": func(ClientAuthType int) tls.ClientAuthType {return tls.ClientAuthType(ClientAuthType)},
		"ClientHelloInfo": func() (tls.ClientHelloInfo, *tls.ClientHelloInfo) {r := tls.ClientHelloInfo{};return r, &r},
		"NewLRUClientSessionCache": tls.NewLRUClientSessionCache,
		"ClientSessionState": func() (tls.ClientSessionState, *tls.ClientSessionState) {r := tls.ClientSessionState{};return r, &r},
		"Config": func() (tls.Config, *tls.Config) {r := tls.Config{};return r, &r},
		"Conn": func() (tls.Conn, *tls.Conn) {r := tls.Conn{};return r, &r},
		"Client": tls.Client,
		"Dial": tls.Dial,
		"DialWithDialer": tls.DialWithDialer,
		"ConnectionState": func() (tls.ConnectionState, *tls.ConnectionState) {r := tls.ConnectionState{};return r, &r},
		"CurveID": func(CurveID uint16) tls.CurveID {return tls.CurveID(CurveID)},
		"RenegotiationSupport": func(RenegotiationSupport int) tls.RenegotiationSupport {return tls.RenegotiationSupport(RenegotiationSupport)},
		"SignatureScheme": func(SignatureScheme uint16) tls.SignatureScheme {return tls.SignatureScheme(SignatureScheme)},
   	},
   	"crypto/x509":{
		"CreateCertificate": x509.CreateCertificate,
		"CreateCertificateRequest": x509.CreateCertificateRequest,
		"DecryptPEMBlock": x509.DecryptPEMBlock,
		"EncryptPEMBlock": x509.EncryptPEMBlock,
		"IsEncryptedPEMBlock": x509.IsEncryptedPEMBlock,
		"MarshalECPrivateKey": x509.MarshalECPrivateKey,
		"MarshalPKCS1PublicKey": x509.MarshalPKCS1PublicKey,
		"MarshalPKCS1PrivateKey": x509.MarshalPKCS1PrivateKey,
		"MarshalPKIXPublicKey": x509.MarshalPKIXPublicKey,
		"MarshalPKCS8PrivateKey": x509.MarshalPKCS8PrivateKey,
		"ParseCRL": x509.ParseCRL,
		"ParseCertificates": x509.ParseCertificates,
		"ParseDERCRL": x509.ParseDERCRL,
		"ParseECPrivateKey": x509.ParseECPrivateKey,
		"ParsePKCS1PublicKey": x509.ParsePKCS1PublicKey,
		"ParsePKCS1PrivateKey": x509.ParsePKCS1PrivateKey,
		"ParsePKCS8PrivateKey": x509.ParsePKCS8PrivateKey,
		"ParsePKIXPublicKey": x509.ParsePKIXPublicKey,
		"CertPool": func() (x509.CertPool, *x509.CertPool) {r := x509.CertPool{};return r, &r},
		"NewCertPool": x509.NewCertPool,
		"SystemCertPool": x509.SystemCertPool,
		"Certificate": func() (x509.Certificate, *x509.Certificate) {r := x509.Certificate{};return r, &r},
		"ParseCertificate": x509.ParseCertificate,
		"CertificateRequest": func() (x509.CertificateRequest, *x509.CertificateRequest) {r := x509.CertificateRequest{};return r, &r},
		"ParseCertificateRequest": x509.ParseCertificateRequest,
		"KeyUsage": func(KeyUsage int) x509.KeyUsage {return x509.KeyUsage(KeyUsage)},
		"PEMCipher": func(PEMCipher int) x509.PEMCipher {return x509.PEMCipher(PEMCipher)},
		"PublicKeyAlgorithm": func(PublicKeyAlgorithm int) x509.PublicKeyAlgorithm {return x509.PublicKeyAlgorithm(PublicKeyAlgorithm)},
		"SignatureAlgorithm": func(SignatureAlgorithm int) x509.SignatureAlgorithm {return x509.SignatureAlgorithm(SignatureAlgorithm)},
		"VerifyOptions": func() (x509.VerifyOptions, *x509.VerifyOptions) {r := x509.VerifyOptions{};return r, &r},
	},
	"crypto/x509/pkix":{
		"AlgorithmIdentifier": func() (pkix.AlgorithmIdentifier, *pkix.AlgorithmIdentifier) {r := pkix.AlgorithmIdentifier{};return r, &r},
		"AttributeTypeAndValue": func() (pkix.AttributeTypeAndValue, *pkix.AttributeTypeAndValue) {r := pkix.AttributeTypeAndValue{};return r, &r},
		"AttributeTypeAndValueSET": func() (pkix.AttributeTypeAndValueSET, *pkix.AttributeTypeAndValueSET) {r := pkix.AttributeTypeAndValueSET{};return r, &r},
		"CertificateList": func() (pkix.CertificateList, *pkix.CertificateList) {r := pkix.CertificateList{};return r, &r},
		"Extension": func() (pkix.Extension, *pkix.Extension) {r := pkix.Extension{};return r, &r},
		"Name": func() (pkix.Name, *pkix.Name) {r := pkix.Name{};return r, &r},
		"RDNSequence": func(RDNSequence []pkix.RelativeDistinguishedNameSET) pkix.RDNSequence {return RDNSequence},
		"RelativeDistinguishedNameSET": func(RelativeDistinguishedNameSET []pkix.AttributeTypeAndValue) pkix.RelativeDistinguishedNameSET {return RelativeDistinguishedNameSET},
		"RevokedCertificate": func() (pkix.RevokedCertificate, *pkix.RevokedCertificate) {r := pkix.RevokedCertificate{};return r, &r},
		"TBSCertificateList": func() (pkix.TBSCertificateList, *pkix.TBSCertificateList) {r := pkix.TBSCertificateList{};return r, &r},
	},
	"encoding/asn1":{
		"MarshalWithParams": asn1.MarshalWithParams,
		"Marshal": asn1.Marshal,
		"Unmarshal": asn1.Unmarshal,
		"UnmarshalWithParams": asn1.UnmarshalWithParams,
		"BitString": func() (asn1.BitString, *asn1.BitString) {r := asn1.BitString{};return r, &r},
		"Flag": func(Flag bool) asn1.Flag {return asn1.Flag(Flag)},
		"ObjectIdentifier": func(ObjectIdentifier []int) asn1.ObjectIdentifier {return ObjectIdentifier},
		"RawContent": func(RawContent []byte) asn1.RawContent {return RawContent},
		"RawValue": func() (asn1.RawValue, *asn1.RawValue) {r := asn1.RawValue{};return r, &r},
	},
	"math/big":{
		"Jacobi": big.Jacobi,
		"Accuracy": func(Accuracy int8) big.Accuracy {return big.Accuracy(Accuracy)},
		"Float": func() (big.Float, *big.Float) {r := big.Float{};return r, &r},
		"NewFloat": big.NewFloat,
		"ParseFloat": big.ParseFloat,
		"Int": func() (big.Int, *big.Int) {r := big.Int{};return r, &r},
		"NewInt": big.NewInt,
		"Rat": func() (big.Rat, *big.Rat) {r := big.Rat{};return r, &r},
		"NewRat": big.NewRat,
		"RoundingMode": func(RoundingMode byte) big.RoundingMode {return big.RoundingMode(RoundingMode)},
		"Word": func(Word uint) big.Word {return big.Word(Word)},
	},
	"bufio":{
		"ScanBytes": bufio.ScanBytes,
		"ScanLines": bufio.ScanLines,
		"ScanRunes": bufio.ScanRunes,
		"ScanWords": bufio.ScanWords,
		"ReadWriter": func() (bufio.ReadWriter, *bufio.ReadWriter) {r := bufio.ReadWriter{};return r, &r},
		"NewReadWriter": bufio.NewReadWriter,
		"Reader": func() (bufio.Reader, *bufio.Reader) {r := bufio.Reader{};return r, &r},
		"NewReader": bufio.NewReader,
		"NewReaderSize": bufio.NewReaderSize,
		"Writer": func() (bufio.Writer, *bufio.Writer) {r := bufio.Writer{};return r, &r},
		"NewWriter": bufio.NewWriter,
		"NewWriterSize": bufio.NewWriterSize,
		"Scanner": func() (bufio.Scanner, *bufio.Scanner) {r := bufio.Scanner{};return r, &r},
		"SplitFunc": func(SplitFunc func(data []byte, atEOF bool) (advance int, token []byte, err error)) bufio.SplitFunc {return bufio.SplitFunc(SplitFunc)},
		"NewScanner": bufio.NewScanner,
	},
	"url":{
		"PathEscape": url.PathEscape,
		"PathUnescape": url.PathUnescape,
		"QueryEscape": url.QueryEscape,
		"QueryUnescape": url.QueryUnescape,
		"URL": func() (url.URL, *url.URL) {r := url.URL{};return r, &r},
		"Parse": url.Parse,
		"ParseRequestURI": url.ParseRequestURI,
		"Userinfo": func() (url.Userinfo, *url.Userinfo) {r := url.Userinfo{};return r, &r},
		"User": url.User,
		"UserPassword": url.UserPassword,
		"Values": func(Values map[string][]string) url.Values {return Values},
		"ParseQuery": url.ParseQuery,
	},
    "strings": {
    	"Compare": strings.Compare,
        "Contains": strings.Contains,
        "ContainsAny": strings.ContainsAny,
        "ContainsRune": strings.ContainsRune,
        "Count": strings.Count,
        "EqualFold": strings.EqualFold,
        "Fields": strings.Fields,
        "FieldsFunc": strings.FieldsFunc,
        "HasPrefix": strings.HasPrefix,
        "HasSuffix": strings.HasSuffix,
        "Index": strings.Index,
        "IndexAny": strings.IndexAny,
        "IndexByte": strings.IndexByte,
        "IndexFunc": strings.IndexFunc,
        "IndexRune": strings.IndexRune,
        "LastIndex": strings.LastIndex,
        "LastIndexAny": strings.LastIndexAny,
        "LastIndexByte": strings.LastIndexByte,
        "LastIndexFunc": strings.LastIndexFunc,
        "Map": strings.Map,
        "Repeat": strings.Repeat,
        "Replace": strings.Replace,
        "Join": strings.Join,
        "Split": strings.Split,
        "SplitN": strings.SplitN,
        "SplitAfter": strings.SplitAfter,
        "SplitAfterN": strings.SplitAfterN,
        "Title": strings.Title,
        "ToLower": strings.ToLower,
        "ToLowerSpecial": strings.ToLowerSpecial,
        "ToTitle": strings.ToTitle,
        "ToTitleSpecial": strings.ToTitleSpecial,
        "ToUpper": strings.ToUpper,
        "ToUpperSpecial": strings.ToUpperSpecial,
        "Trim": strings.Trim,
        "TrimFunc": strings.TrimFunc,
        "TrimLeft": strings.TrimLeft,
        "TrimPrefix": strings.TrimPrefix,
        "TrimLeftFunc": strings.TrimLeftFunc,
        "TrimRight": strings.TrimRight,
        "TrimSuffix": strings.TrimSuffix,
        "TrimRightFunc": strings.TrimRightFunc,
        "TrimSpace": strings.TrimSpace,
		"Reader": func() (strings.Reader, *strings.Reader) {r := strings.Reader{};return r, &r},
        "NewReader": strings.NewReader,
		"Replacer": func() (strings.Replacer, *strings.Replacer) {r := strings.Replacer{};return r, &r},
        "NewReplacer": strings.NewReplacer,
        "Builder": func() (strings.Builder, *strings.Builder) {r := strings.Builder{};return r, &r},
    },
    "bytes": {
        "Compare": bytes.Compare,
        "Contains": bytes.Contains,
        "ContainsAny": bytes.ContainsAny,
        "ContainsRune": bytes.ContainsRune,
        "Count": bytes.Count,
        "Equal": bytes.Equal,
        "EqualFold": bytes.EqualFold,
        "Fields": bytes.Fields,
        "FieldsFunc": bytes.FieldsFunc,
        "HasPrefix": bytes.HasPrefix,
        "HasSuffix": bytes.HasSuffix,
        "Index": bytes.Index,
        "IndexAny": bytes.IndexAny,
        "IndexByte": bytes.IndexByte,
        "IndexFunc": bytes.IndexFunc,
        "IndexRune": bytes.IndexRune,
        "LastIndex": bytes.LastIndex,
        "LastIndexAny": bytes.LastIndexAny,
        "LastIndexByte":bytes.LastIndexByte,
        "LastIndexFunc": bytes.LastIndexFunc,
        "Map": bytes.Map,
        "Repeat": bytes.Repeat,
        "Replace": bytes.Replace,
        "Runes": bytes.Runes,
        "Join": bytes.Join,
        "Split": bytes.Split,
        "SplitN": bytes.SplitN,
        "SplitAfter": bytes.SplitAfter,
        "SplitAfterN": bytes.SplitAfterN,
        "Title": bytes.Title,
        "ToLower": bytes.ToLower,
        "ToLowerSpecial": bytes.ToLowerSpecial,
        "ToTitle": bytes.ToTitle,
        "ToTitleSpecial": bytes.ToTitleSpecial,
        "ToUpper": bytes.ToUpper,
        "ToUpperSpecial": bytes.ToUpperSpecial,
        "Trim": bytes.Trim,
        "TrimFunc": bytes.TrimFunc,
        "TrimPrefix": bytes.TrimPrefix,
        "TrimLeft": bytes.TrimLeft,
        "TrimLeftFunc": bytes.TrimLeftFunc,
        "TrimSuffix": bytes.TrimSuffix,
        "TrimRight": bytes.TrimRight,
        "TrimRightFunc": bytes.TrimRightFunc,
        "TrimSpace": bytes.TrimSpace,
		"Buffer": func() (bytes.Buffer, *bytes.Buffer) {r := bytes.Buffer{};return r, &r},
        "NewBuffer": bytes.NewBuffer,
        "NewBufferString": bytes.NewBufferString,
		"Reader": func() (bytes.Reader, *bytes.Reader) {r := bytes.Reader{};return r, &r},
        "NewReader": bytes.NewReader,
    },
    "strconv": {
        "AppendBool": strconv.AppendBool,
        "AppendFloat": strconv.AppendFloat,
        "AppendInt": strconv.AppendInt,
        "AppendUint": strconv.AppendUint,
        "AppendQuote": strconv.AppendQuote,
        "AppendQuoteToASCII": strconv.AppendQuoteToASCII,
        "AppendQuoteRune": strconv.AppendQuoteRune,
        "AppendQuoteRuneToASCII": strconv.AppendQuoteRuneToASCII,
        "AppendQuoteRuneToGraphic": strconv.AppendQuoteRuneToGraphic,
        "AppendQuoteToGraphic": strconv.AppendQuoteToGraphic,
        "Atoi": strconv.Atoi,
        "Itoa": strconv.Itoa,
        "CanBackquote": strconv.CanBackquote,
        "FormatBool": strconv.FormatBool,
        "FormatFloat": strconv.FormatFloat,
        "FormatInt": strconv.FormatInt,
        "FormatUint": strconv.FormatUint,
        "IsGraphic": strconv.IsGraphic,
        "IsPrint": strconv.IsPrint,
        "ParseBool": strconv.ParseBool,
        "ParseFloat": strconv.ParseFloat,
        "ParseInt": strconv.ParseInt,
        "ParseUint": strconv.ParseUint,
        "Quote": strconv.Quote,
        "QuoteToASCII": strconv.QuoteToASCII,
        "QuoteToGraphic":strconv.QuoteToGraphic,
        "QuoteRune": strconv.QuoteRune,
        "QuoteRuneToASCII": strconv.QuoteRuneToASCII,
        "QuoteRuneToGraphic":strconv.QuoteRuneToGraphic,
        "Unquote": strconv.Unquote,
        "UnquoteChar": strconv.UnquoteChar,
    },
    "encoding/json": {
		"Compact": json.Compact,
		"Indent": json.Indent,
		"HTMLEscape": json.HTMLEscape,
		"Marshal": json.Marshal,
		"MarshalIndent": json.MarshalIndent,
		"Unmarshal": json.Unmarshal,
		"NewEncoder": json.NewEncoder,
		"NewDecoder": json.NewDecoder,
		"Valid": json.Valid,
	},
    "regexp": {
        "Match": regexp.Match,
        "MatchReader": regexp.MatchReader,
        "MatchString": regexp.MatchString,
        "QuoteMeta": regexp.QuoteMeta,
        "Compile": regexp.Compile,
        "CompilePOSIX": regexp.CompilePOSIX,
    },
    "unicode": {
    	"In": unicode.In,
        "Is": unicode.Is,
        "IsControl": unicode.IsControl,
        "IsDigit": unicode.IsDigit,
        "IsGraphic": unicode.IsGraphic,
        "IsPrint": unicode.IsPrint,
        "IsLetter": unicode.IsLetter,
        "IsLower": unicode.IsLower,
        "IsTitle": unicode.IsTitle,
        "IsUpper": unicode.IsUpper,
        "IsMark": unicode.IsMark,
        "IsNumber": unicode.IsNumber,
        "IsOneOf": unicode.IsOneOf,
        "IsPunct": unicode.IsPunct,
        "IsSpace": unicode.IsSpace,
        "IsSymbol": unicode.IsSymbol,
        "SimpleFold": unicode.SimpleFold,
        "To": unicode.To,
        "ToLower": unicode.ToLower,
        "ToTitle": unicode.ToTitle,
        "ToUpper": unicode.ToUpper,
    },
    "unicode/utf8": {
        "DecodeLastRune": utf8.DecodeLastRune,
        "DecodeLastRuneInString": utf8.DecodeLastRuneInString,
        "DecodeRune": utf8.DecodeRune,
        "DecodeRuneInString": utf8.DecodeRuneInString,
        "EncodeRune": utf8.EncodeRune,
        "FullRune": utf8.FullRune,
        "FullRuneInString": utf8.FullRuneInString,
        "RuneCount": utf8.RuneCount,
        "RuneCountInString": utf8.RuneCountInString,
        "RuneLen": utf8.RuneLen,
        "RuneStart": utf8.RuneStart,
        "Valid": utf8.Valid,
        "ValidRune": utf8.ValidRune,
        "ValidString": utf8.ValidString,
    },
    "io": {
		"EOF": io.EOF,
		"Copy": io.Copy,
		"CopyBuffer": io.CopyBuffer,
		"CopyN": io.CopyN,
		"ReadAtLeast": io.ReadAtLeast,
		"ReadFull": io.ReadFull,
		"WriteString": io.WriteString,
		"Pipe": io.Pipe,
		"LimitReader": io.LimitReader,
		"MultiReader": io.MultiReader,
		"TeeReader": io.TeeReader,
		"NewSectionReader": io.NewSectionReader,
		"MultiWriter": io.MultiWriter,
    },
    "io/ioutil": {
    	"Discard": ioutil.Discard,
    	"NopCloser": ioutil.NopCloser,
        "ReadAll": ioutil.ReadAll,
        "ReadFile":func(filename string) ([]byte, error){
    		filename = filepath.Join(*fRootPath, filepath.Clean(filename))
			return ioutil.ReadFile(filename)
        },
        "WriteFile":func(filename string, data []byte, perm os.FileMode) error {
    		filename = filepath.Join(*fRootPath, filepath.Clean(filename))
			return ioutil.WriteFile(filename, data, perm)
        },
    },
    "os":{
    	"IsTimeout": os.IsTimeout,
    	"IsExist": os.IsExist,
    	"IsNotExist": os.IsNotExist,
    	"IsPermission": os.IsPermission,
    	"Mkdir": func(name string, perm os.FileMode) error {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.Mkdir(name, perm)
    	},
    	"MkdirAll": func(name string, perm os.FileMode) error {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.MkdirAll(name, perm)
    	},
    	"Remove": func(name string) error{
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.Remove(name)
    	},
    	"RemoveAll": func(name string) error {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.RemoveAll(name)
    	},
    	"Rename": func(oldname, newname string) error{
    		oldname = filepath.Join(*fRootPath, filepath.Clean(oldname))
    		newname = filepath.Join(*fRootPath, filepath.Clean(newname))
			return os.Rename(oldname, newname)
    	},
    	"Create": func(name string) (*os.File, error) {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.Create(name)
    	},
    	"NewFile": os.NewFile,
    	"Open": func(name string) (*os.File, error) {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.Open(name)
    	},
    	"OpenFile": func(name string, flag int, perm os.FileMode) (*os.File, error) {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.OpenFile(name, flag, perm)
    	},
    	"Lstat": func(name string) (os.FileInfo, error) {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.Lstat(name)
    	},
    	"Stat": func(name string) (os.FileInfo, error) {
    		name = filepath.Join(*fRootPath, filepath.Clean(name))
			return os.Stat(name)
    	},
    	"FileMode": func(FileMode uint32) os.FileMode {return os.FileMode(FileMode)},
    },
}
