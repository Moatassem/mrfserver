/*
# Software Name : Session Router (SR)
# SPDX-FileCopyrightText: Copyright (c) Orange Business - OINIS/Services/NSF
# SPDX-License-Identifier: Apache-2.0
#
# This software is distributed under the Apache-2.0
# See the "LICENSES" directory for more details.
#
# Authors:
# - Moatassem Talaat <moatassem.talaat@orange.com>

---
*/

package global

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

// ============================================================
func LogCallStack(r any) {
	fmt.Printf("Panic Recovered! Encountered Error:\n%v\n", r)
	buf := make([]byte, 1024)
	n := runtime.Stack(buf, false)
	fmt.Printf("Stack trace:\n%s\n", buf[:n])
}

func GetLocalIPs() ([]net.IP, error) {
	var IPs []net.IP
	var ip net.IP
	ifaces, _ := net.Interfaces()
outer:
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagRunning == 0 { //|| i.Flags&net.FlagBroadcast == 0
			continue
		}
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			if v, ok := addr.(*net.IPNet); ok {
				ip = v.IP
				if ip.To4() != nil && ip.IsPrivate() {
					IPs = append(IPs, ip)
					continue outer
				}
			}
		}
	}
	if len(IPs) == 0 {
		return nil, errors.New("no valid IPv4 found")
	}
	return IPs, nil
}

func StartListening(ip net.IP, prt int) (*net.UDPConn, error) {
	var socket net.UDPAddr
	socket.IP = ip
	socket.Port = prt
	return net.ListenUDP("udp", &socket)
}

func GenerateUDPSocket(conn *net.UDPConn) *net.UDPAddr {
	return conn.LocalAddr().(*net.UDPAddr)
}

func GetUDPSocket(ipt string) (*net.UDPAddr, error) {
	return net.ResolveUDPAddr("udp", ipt)
}

// ============================================================

func GenerateViaWithoutBranch(conn *net.UDPConn) string {
	udpsocket := GenerateUDPSocket(conn)
	return fmt.Sprintf("SIP/2.0/UDP %s", udpsocket)
}

func GenerateContact(skt *net.UDPAddr) string {
	return fmt.Sprintf("<sip:%s;transport=udp>", skt)
}

func GetURIUsername(uri string) string {
	var mtch []string
	if RMatch(uri, NumberOnly, &mtch) {
		return mtch[1]
	}
	return ""
}

// =============================================================

func TrimWithSuffix(s string, sfx string) string {
	s = strings.Trim(s, " ")
	if s == "" {
		return s
	}
	return fmt.Sprintf("%s%s", s, sfx)
}

func GetNextIndex(pdu []byte, markstrng string) int {
	markBytes := []byte(markstrng)
	for i := 0; i <= len(pdu)-len(markBytes); i++ {
		k := 0
		for k < len(markBytes) {
			if pdu[i+k] != markBytes[k] {
				goto nextloop
			}
			k++
		}
		return i
	nextloop:
	}
	return -1
}

func GetUsedSize(pdu []byte) int {
	sz := len(pdu)
	for i := 0; i < sz; i++ {
		if pdu[i] == 0 {
			return i
		}
	}
	return sz
}

func DropVisualSeparators(strng string) string {
	strng = strings.ReplaceAll(strng, ".", "")
	strng = strings.ReplaceAll(strng, "-", "")
	strng = strings.ReplaceAll(strng, "(", "")
	strng = strings.ReplaceAll(strng, ")", "")
	return strng
}

func CleanAndSplitHeader(HeaderValue string, DropParameterValueDQ ...bool) map[string]string {
	if HeaderValue == "" {
		return nil
	}

	NVC := make(map[string]string)
	splitChar := ';'

	splitCharFirstIndex := strings.IndexRune(HeaderValue, splitChar)
	if splitCharFirstIndex == -1 {
		NVC["!headerValue"] = HeaderValue
		return NVC
	} else {
		NVC["!headerValue"] = HeaderValue[:splitCharFirstIndex]
	}

	chrlst := []rune(HeaderValue[splitCharFirstIndex:])
	var sb strings.Builder

	var fn, fv string
	DQO := false
	dropDQ := len(DropParameterValueDQ) > 0 && DropParameterValueDQ[0]

	for i := 0; i < len(chrlst); {
		switch chrlst[i] {
		case ' ':
			if DQO {
				sb.WriteRune(chrlst[i])
			}
		case '=':
			if DQO {
				sb.WriteRune(chrlst[i])
			} else {
				fn = sb.String()
				sb.Reset()
			}
		case splitChar:
			if DQO {
				sb.WriteRune(chrlst[i])
			} else {
				if sb.Len() == 0 {
					break
				}
				fv = sb.String()
				NVC[fn] = DropConcatenationChars(fv, dropDQ)
				fn, fv = "", ""
				sb.Reset()
			}
		case '"':
			if DQO {
				fv = sb.String()
				NVC[fn] = DropConcatenationChars(fv, dropDQ)
				fn, fv = "", ""
				sb.Reset()
				DQO = false
			} else {
				DQO = true
			}
		default:
			sb.WriteRune(chrlst[i])
		}
		chrlst = append(chrlst[:i], chrlst[i+1:]...)
	}

	if fn != "" && sb.Len() > 0 {
		fv = sb.String()
		NVC[fn] = DropConcatenationChars(fv, dropDQ)
	}

	return NVC
}

func DropConcatenationChars(s string, dropDQ bool) string {
	if dropDQ {
		s = strings.ReplaceAll(s, "'", "")
		return strings.ReplaceAll(s, `"`, "")
	}
	return s
}

func ParseParameters(parsline string) *map[string]string {
	parsline = strings.Trim(parsline, ";")
	parsline = strings.Trim(parsline, ",")
	parsMap := make(map[string]string)
	if parsline == "" {
		return &parsMap
	}
	tpls := strings.Split(parsline, ";")
	for _, tpl := range tpls {
		tmp := strings.Split(tpl, "=")
		switch len(tmp) {
		case 2:
			if _, ok := parsMap[tmp[0]]; !ok {
				parsMap[tmp[0]] = tmp[1]
			} else {
				LogError(LTSIPStack, fmt.Sprintf("duplicate parameter: [%s] - in line: [%s]", tmp[0], parsline))
			}
		case 1:
			if _, ok := parsMap[tmp[0]]; !ok {
				parsMap[tmp[0]] = ""
			} else {
				LogError(LTSIPStack, fmt.Sprintf("duplicate parameter: [%s] - in line: [%s]", tmp[0], parsline))
			}
		default:
			LogError(LTSIPStack, fmt.Sprintf("badly formatted parameter line: [%s]", parsline))
		}
	}
	return &parsMap
}

func GenerateParameters(pars *map[string]string) string {
	if pars == nil {
		return ""
	}
	var sb strings.Builder
	for k, v := range *pars {
		if v == "" {
			sb.WriteString(fmt.Sprintf(";%v", k))
		} else {
			sb.WriteString(fmt.Sprintf(";%v=%v", k, v))
		}
	}
	return sb.String()
}

func RandomNum(min, max uint32) uint32 {
	// #nosec G404: Ignoring gosec error - crypto is not required
	return rand.Uint32N(max-min+1) + min
}

func GetBodyType(contentType string) BodyType {
	contentType = ASCIIToLower(contentType)
	for k, v := range DicBodyContentType {
		if v == contentType {
			return k
		}
	}
	if strings.Contains(contentType, "xml") {
		return AnyXML
	}
	return Unknown
}

// Convert string to int with default value with not included minimum and maximum
func Str2IntDefaultMinMax[T int | int8 | int16 | int32 | int64](s string, d, min, max T) (T, bool) {
	var out T
	if len(s) == 0 {
		return d, false
	}
	idx := 0
	isN := s[idx] == '-'
	if isN {
		idx++
	}
	for i := idx; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return d, false
		}
		out = out*10 + T(s[i]-'0')
	}
	if isN {
		out = -out
	}
	if out <= min || out >= max {
		return d, false
	}
	return out, true
}

func Str2IntDefaultMinimum[T int | int8 | int16 | int32 | int64](s string, d T, min T) (T, bool) {
	var out T
	if len(s) == 0 {
		return d, false
	}
	idx := 0
	isN := s[idx] == '-'
	if isN {
		idx++
	}
	for i := idx; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return d, false
		}
		out = out*10 + T(s[i]-'0')
	}
	if isN {
		out = -out
	}
	if out <= min {
		return d, false
	}
	return out, true
}

func Str2IntDefault[T int | int8 | int16 | int32 | int64](s string, d T) (T, bool) {
	var out T
	if len(s) == 0 {
		return d, false
	}
	idx := 0
	isN := s[idx] == '-'
	if isN {
		idx++
	}
	for i := idx; i < len(s); i++ {
		out = out*10 + T(s[i]-'0')
	}
	if isN {
		return -out, true
	}
	return out, true
}

func Str2Int[T int | int8 | int16 | int32 | int64](s string) T {
	var out T
	if len(s) == 0 {
		return out
	}
	idx := 0
	isN := s[idx] == '-'
	if isN {
		idx++
	}
	for i := idx; i < len(s); i++ {
		out = out*10 + T(s[i]-'0')
	}
	if isN {
		return -out
	}
	return out
}

func Str2Uint[T uint | uint8 | uint16 | uint32 | uint64](s string) T {
	var out T
	if len(s) == 0 {
		return out
	}
	for i := 0; i < len(s); i++ {
		out = out*10 + T(s[i]-'0')
	}
	return out
}

//====================================================

func GetEnumString[T comparable](m map[T]string, s string, keepCase bool) T {
	if !keepCase {
		s = ASCIIToLower(s)
	}
	var rslt T
	for k, v := range m {
		if v == s {
			return k
		}
	}
	return rslt
}

//==================================================

func ASCIIToLower(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += byte(DeltaRune)
		}
		b.WriteByte(c)
	}
	return b.String()
}

func ASCIIToUpper(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' {
			c -= byte(DeltaRune)
		}
		b.WriteByte(c)
	}
	return b.String()
}

func LowerDash(s string) string {
	return strings.ReplaceAll(ASCIIToLower(s), " ", "-")
}

func ASCIIPascal(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'a' <= c && c <= 'z' && (i == 0 || s[i-1] == '-') {
			c -= byte(DeltaRune)
		}
		b.WriteByte(c)
	}
	return b.String()
}

func HeaderCase(h string) string {
	h = ASCIIToLower(h)
	for k := range HeaderStringtoEnum {
		if ASCIIToLower(k) == h {
			return k
		}
	}
	return ASCIIPascal(h)
}

func ASCIIToLowerInPlace(s []byte) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		s[i] = c
	}
}

//==================================================

func Any[T any](items []*T, predict func(*T) bool) bool {
	for _, item := range items {
		if predict(item) {
			return true
		}
	}
	return false
}

func Find[T any](items []*T, predict func(*T) bool) *T {
	for _, item := range items {
		if predict(item) {
			return item
		}
	}
	return nil
}

func Filter[T any](items []*T, predict func(*T) bool) []*T {
	var result []*T
	for _, item := range items {
		if predict(item) {
			result = append(result, item)
		}
	}
	return result
}

func FirstKeyValue[T1 comparable, T2 any](m map[T1]T2) (T1, T2) {
	var key T1
	var value T2
	for k, v := range m {
		return k, v
	}
	return key, value
}

func Keys[T1 comparable, T2 any](m map[T1]T2) []T1 {
	var rslt []T1
	for k := range m {
		rslt = append(rslt, k)
	}
	return rslt
}

func FirstKey[T1 comparable, T2 any](m map[T1]T2) T1 {
	k, _ := FirstKeyValue(m)
	return k
}

func FirstValue[T1 comparable, T2 any](m map[T1]T2) T2 {
	_, v := FirstKeyValue(m)
	return v
}

func GetEnum[T1 comparable, T2 comparable](m map[T1]T2, i T2) T1 {
	var rslt T1
	for k, v := range m {
		if v == i {
			return k
		}
	}
	return rslt
}

// ===================================================================

func StringToHexString(input string) string {
	return BytesToHexString(StringToBytes(input))
}

func StringToBytes(input string) []byte {
	return []byte(input)
}

func BytesToHexString(data []byte) string {
	return hex.EncodeToString(data)
}

func BytesToBase64String(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func HashSDPBytes(bytes []byte) string {
	// hash := sha256.New()
	// return bytesToHexString(hash.Sum(bytes))
	hash := sha256.Sum256(bytes)
	return BytesToHexString(hash[:])
}

//===================================================================

func LogInfo(lt LogTitle, msg string) {
	LogHandler(LLInformation, lt, msg)
}

func LogWarning(lt LogTitle, msg string) {
	LogHandler(LLWarning, lt, msg)
}

func LogError(lt LogTitle, msg string) {
	LogHandler(LLError, lt, msg)
}

func LogHandler(ll LogLevel, lt LogTitle, msg string) {
	log.Printf("\t%v\t%v\t%s\n", ll.String(), lt.String(), msg)
}

//===================================================================

func RMatch(s string, rgxfp FieldPattern, mtch *[]string) bool {
	if s == "" {
		return false
	}
	*mtch = DicFieldRegEx[rgxfp].FindStringSubmatch(s)
	return *mtch != nil
}

func RReplace1(input string, rgxfp FieldPattern, replacement string) string {
	return DicFieldRegEx[rgxfp].ReplaceAllString(input, replacement)
}

func RReplaceNumberOnly(input string, replacement string) string {
	return DicFieldRegEx[ReplaceNumberOnly].ReplaceAllString(input, replacement)
}

func TranslateInternal(input string, matches []string) (string, error) {
	if input == "" {
		return "", nil
	}
	if matches == nil {
		return "", fmt.Errorf("empty matches slice")
	}
	sbToInt := func(sb strings.Builder) int {
		return Str2Int[int](sb.String())
	}

	item := func(idx int, dblbrkt bool) string {
		if idx >= len(matches) {
			if dblbrkt {
				return fmt.Sprintf("${%v}", idx)
			}
			return fmt.Sprintf("$%v", idx)
		}
		return matches[idx]
	}

	var b strings.Builder
outerloop:
	for i := 0; i < len(input); i++ {
		c := input[i]
		if c == '$' {
			i++
			if i == len(input) {
				b.WriteByte(c)
				return b.String(), nil
			}
			c = input[i]
			if c == '$' {
				b.WriteByte(c)
				continue outerloop
			}
			var grpsb strings.Builder
			for {
				if '0' <= c && c <= '9' {
					grpsb.WriteByte(c)
					i++
					if i == len(input) {
						v := item(sbToInt(grpsb), false)
						b.WriteString(v)
						return b.String(), nil
					}
					c = input[i]
				} else if c == '{' {
					if grpsb.Len() == 0 {
						break
					} else {
						b.WriteByte(c)
						v := item(sbToInt(grpsb), false)
						b.WriteString(v)
						continue outerloop
					}
				} else {
					if grpsb.Len() == 0 {
						b.WriteByte('$')
						b.WriteByte(c)
					} else {
						v := item(sbToInt(grpsb), false)
						b.WriteString(v)
					}
					continue outerloop
				}
			}
			for {
				i++
				if i == len(input) {
					return "", fmt.Errorf("bracket unclosed")
				}
				c = input[i]
				if '0' <= c && c <= '9' {
					grpsb.WriteByte(c)
				} else if c == '}' {
					if grpsb.Len() == 0 {
						return "", fmt.Errorf("bracket closed without group index")
					}
					v := item(sbToInt(grpsb), true)
					b.WriteString(v)
					continue outerloop
				} else if c == '{' {
					b.WriteByte(c)
					continue outerloop
				} else {
					return "", fmt.Errorf("invalid character within bracket")
				}
			}
		}
		b.WriteByte(c)
	}
	return b.String(), nil
}

func TranslateExternal(input string, rgxstring string, trans string) string {
	rgx, err := regexp.Compile(rgxstring)
	if err != nil {
		return ""
	}
	var result []byte
	result = rgx.ExpandString(result, trans, input, rgx.FindStringSubmatchIndex(input))
	return string(result)
}

// Use rgx.FindStringSubmatchIndex(input) to get matches
func TranslateResult(rgx *regexp.Regexp, input string, trans string, matches []int) string {
	var result []byte
	result = rgx.ExpandString(result, trans, input, matches)
	return string(result)
}

//===================================================================

func Stringlen(s string) int {
	return utf8.RuneCountInString(s)
}

func (m Method) IsDialogueCreating() bool {
	switch m {
	case OPTIONS, INVITE: // MESSAGE, NEGOTIATE
		return true
	}
	return false
}

func (m Method) RequiresACK() bool {
	switch m {
	case INVITE, ReINVITE:
		return true
	}
	return false
}

// =====================================================

func (he HeaderEnum) LowerCaseString() string {
	h := HeaderEnumToString[he]
	return ASCIIToLower(h)
}

func (he HeaderEnum) String() string {
	return HeaderEnumToString[he]
}

// case insensitive equality with string header name
func (he HeaderEnum) Equals(h string) bool {
	return he.LowerCaseString() == ASCIIToLower(h)
}

// ====================================================

func IsProvisional(sc int) bool {
	return 100 <= sc && sc <= 199
}

func IsProvisional18x(sc int) bool {
	return 180 <= sc && sc <= 189
}

func Is18xOrPositive(sc int) bool {
	return (180 <= sc && sc <= 189) || (200 <= sc && sc <= 299)
}

func IsFinal(sc int) bool {
	return 200 <= sc && sc <= 699
}

func IsPositive(sc int) bool {
	return 200 <= sc && sc <= 299
}

func IsNegative(sc int) bool {
	return 300 <= sc && sc <= 699
}

func IsRedirection(sc int) bool {
	return 300 <= sc && sc <= 399
}

func IsNegativeClient(sc int) bool {
	return 400 <= sc && sc <= 499
}

func IsNegativeServer(sc int) bool {
	return 500 <= sc && sc <= 599
}

func IsNegativeGlobal(sc int) bool {
	return 600 <= sc && sc <= 699
}

//===================================================================
