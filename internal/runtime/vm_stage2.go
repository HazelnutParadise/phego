package runtime

import (
	"math"
	"strconv"
	"strings"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 2: REQ-001, REQ-016, REQ-017, REQ-025, REQ-026; T-009, T-010,
// T-031..T-034. The format 1.2 instructions: element locations (FetchDim),
// string offsets, the foreach iterator and parameter defaults. Each follows
// the Zend function named in its comment; the expectations are the PHP
// 8.5.12 outputs recorded in the root package's w17_stage2_probes_test.go.

// vmHoleCheck is how many deleted elements one IterNext skips between two
// checks for a host stop.
const vmHoleCheck = 1 << 16

// vmStringOffsetLimit bounds the string a string offset write may create.
// It is PHP's default memory_limit (128M, as php -n runs), which no such
// write can pass there; Phego has no memory metering yet, so the run ends
// as a memory limit stop instead of PHP's fatal error.
const vmStringOffsetLimit = 128 << 20

// --- element locations ---

// elementSlot is the element a Location register names after FetchDim.
func (s *vmState) elementSlot(f *vmFrame, r uint32) (*value, error) {
	if l := f.locs[r]; l < 0 && int(-l) <= len(f.elems) {
		if p := f.elems[-l-1]; p != nil {
			return p, nil
		}
	}
	return nil, s.internal("location register %d read before Local or FetchDim", r)
}

// setElementLocation makes the Location register r name p.
func (s *vmState) setElementLocation(f *vmFrame, r uint32, p *value) error {
	e := f.fn.elemOf[r]
	if e < 0 || int(e) >= len(f.elems) {
		return s.internal("location register %d has no element slot", r)
	}
	f.elems[e] = p
	f.locs[r] = -(e + 1)
	return nil
}

// loadElement is Load of an element location: its value, null when the
// element holds none.
func (s *vmState) loadElement(f *vmFrame, in *vmInstr) error {
	slot, err := s.elementSlot(f, in.b)
	if err != nil {
		return err
	}
	v := slot.deref()
	if v.kind == valueUndefined {
		v = nullValue()
	}
	return s.setReg(f.regs, in.a, v)
}

// storeElement is Store to an element location.
func (s *vmState) storeElement(f *vmFrame, in *vmInstr) error {
	slot, err := s.elementSlot(f, in.a)
	if err != nil {
		return err
	}
	return s.h.assign(slot, operandValue(f.regs, in.b))
}

// fetchDim is ZEND_FETCH_DIM_W, ZEND_FETCH_DIM_RW and ZEND_FETCH_DIM_UNSET
// (zend_fetch_dimension_address), and the container and key part of
// ZEND_ASSIGN_DIM_OP: A names the element of the container B. The container
// is checked first (an undefined variable warns except in write mode, null
// and undefined become an array, false does so with PHP 8.1's deprecation,
// other scalars and strings throw), then the key is read and converted,
// then the element is found or created.
func (s *vmState) fetchDim(f *vmFrame, in *vmInstr) error {
	flags := bytecode.FetchFlags(in.aux)
	mode := flags.Mode()
	var slot *value
	if flags&bytecode.FetchContainerValue != 0 {
		// ZEND_FETCH_DIM_W on a VAR: the container is the reference the
		// right-hand side of a destructuring assignment became (MakeRef,
		// or a by-reference IterNext). The compiler never fetches from a
		// register holding anything else; FETCH_LIST_W's notice for that
		// case is not modeled, the run fails closed.
		slot = &f.regs[in.b]
		if slot.kind != valueReference {
			return s.unsupported("fetching an element of a temporary that is not a reference")
		}
	} else {
		var err error
		if slot, err = s.slot(f, in.b); err != nil {
			return err
		}
	}
	if err := s.h.runtimeCheck(); err != nil {
		return err
	}
	c := slot.deref()
	switch c.kind {
	case valueArray:
	case valueString:
		return s.fetchDimString(f, in, flags)
	case valueObject, valueResource:
		return s.unsupported("an element of an object or resource")
	case valueUndefined, valueNull, valueBool:
		if c.kind == valueBool && c.boolean {
			return s.fetchDimScalar(f, in, flags)
		}
		if c.kind == valueUndefined && mode != bytecode.FetchWrite && flags&bytecode.FetchContainerValue == 0 {
			if name, ok := s.variableName(f, in.b); ok {
				s.h.warn(levelWarning, "Undefined variable $"+name)
				if s.stopped() {
					return nil
				}
			}
		}
		if mode == bytecode.FetchUnset {
			// Nothing is created: the element is a null that belongs to
			// no array, which the following unset leaves alone.
			if c.kind == valueBool {
				s.h.warn(levelDeprecated, "Automatic conversion of false to array is deprecated")
			}
			return s.setElementLocation(f, in.a, &value{kind: valueNull})
		}
		if err := s.writable(slot, flags&bytecode.FetchAppend != 0); err != nil || s.stopped() {
			return err
		}
	default:
		return s.fetchDimScalar(f, in, flags)
	}
	if flags&bytecode.FetchAppend != 0 {
		e, err := s.h.appendElement(slot)
		if err != nil {
			return nextElementError(err)
		}
		return s.setElementLocation(f, in.a, e)
	}
	var k value
	if flags&bytecode.FetchKeyLocation == 0 {
		k = operandValue(f.regs, in.c)
	} else if kv, _, err := s.readVar(f, in.c); err != nil || s.stopped() {
		return err
	} else {
		k = kv
	}
	if err := s.h.claimValues(*slot, k); err != nil {
		return err
	}
	a, err := s.h.separate(slot)
	if err != nil {
		return err
	}
	key, err := s.h.arrayKey(k, false)
	if err != nil || s.stopped() {
		return err
	}
	if b, ok := a.lookup(key); ok {
		return s.setElementLocation(f, in.a, &b.val)
	}
	switch mode {
	case bytecode.FetchUnset:
		return s.setElementLocation(f, in.a, &value{kind: valueNull})
	case bytecode.FetchReadWrite:
		// zend_undefined_offset_write / zend_undefined_index_write.
		s.h.warn(levelWarning, undefinedKeyMessage(key))
		if s.stopped() {
			return nil
		}
	}
	return s.setElementLocation(f, in.a, &a.insert(key, nullValue(), s.h.width).val)
}

// fetchDimScalar is the true, int and float container: PHP throws without
// reading the key, except that ZEND_ASSIGN_DIM_OP reads a variable key, with
// its warning, first.
func (s *vmState) fetchDimScalar(f *vmFrame, in *vmInstr, flags bytecode.FetchFlags) error {
	if flags.Mode() == bytecode.FetchUnset {
		return &Throwable{"Error", "Cannot unset offset in a non-array variable"}
	}
	if flags.Use() == bytecode.FetchForAssignOp && flags&bytecode.FetchKeyLocation != 0 {
		if _, _, err := s.readVar(f, in.c); err != nil || s.stopped() {
			return err
		}
	}
	return &Throwable{"Error", "Cannot use a scalar value as an array"}
}

// fetchDimString is the string container: zend_check_string_offset reports
// the offset, then zend_wrong_string_offset_error says what cannot be done
// with a string offset.
func (s *vmState) fetchDimString(f *vmFrame, in *vmInstr, flags bytecode.FetchFlags) error {
	if flags&bytecode.FetchAppend != 0 {
		return &Throwable{"Error", "[] operator not supported for strings"}
	}
	var k value
	var err error
	if flags&bytecode.FetchKeyLocation == 0 {
		k = operandValue(f.regs, in.c)
	} else if k, _, err = s.readVar(f, in.c); err != nil || s.stopped() {
		return err
	}
	use := offsetWrite
	if flags.Mode() == bytecode.FetchUnset {
		use = offsetUnset
	}
	if _, _, err := s.stringOffset(k, use); err != nil || s.stopped() {
		return err
	}
	switch flags.Use() {
	case bytecode.FetchForIncDec:
		return &Throwable{"Error", "Cannot increment/decrement string offsets"}
	case bytecode.FetchForAssignOp:
		return &Throwable{"Error", "Cannot use assign-op operators with string offsets"}
	case bytecode.FetchForRef:
		return &Throwable{"Error", "Cannot create references to/from string offsets"}
	}
	return &Throwable{"Error", "Cannot use string offset as an array"}
}

// variableName is the PHP name of the variable a Location register names;
// ok is false for an element location.
func (s *vmState) variableName(f *vmFrame, r uint32) (string, bool) {
	if l := f.locs[r]; l > 0 && int(l) <= len(f.fn.localNames) {
		return f.fn.localNames[l-1], true
	}
	return "", false
}

// --- string offsets ---

type offsetUse uint8

const (
	offsetRead  offsetUse = iota // BP_VAR_R: $s[k]
	offsetIsset                  // BP_VAR_IS: $s[k] ?? d
	offsetWrite                  // BP_VAR_W and BP_VAR_RW: $s[k] = v and what leads to it
	offsetUnset                  // BP_VAR_UNSET
)

// floatOffset is zend_dval_to_lval as string offsets use it: no diagnostic,
// 0 for a non-finite float and modular arithmetic out of range.
func floatOffset(d float64, width int) int64 {
	switch {
	case math.IsInf(d, 0) || math.IsNaN(d):
		return 0
	case !floatFitsInt(d, width):
		return floatToIntModular(d, width)
	}
	return int64(d)
}

// stringOffset converts a string offset as zend_fetch_dimension_str and
// zend_check_string_offset do. Integers and integer numeric strings are
// offsets; a leading-numeric string is one after a warning; null, bool and
// float are cast, with a warning except when read by ??; other strings and
// arrays throw, except that ?? reads a string that is not an offset as
// null (ok is false).
func (s *vmState) stringOffset(k value, use offsetUse) (off int64, ok bool, err error) {
	switch k = k.deref(); k.kind {
	case valueInt:
		return k.integer, true, nil
	case valueString:
		n := php8512NumericString(k.text, s.h.width, true)
		if n.kind == numericInt {
			if n.trailing && use != offsetUnset {
				s.h.warn(levelWarning, "Illegal string offset \""+cString(k.text)+"\"")
			}
			return n.integer, true, nil
		}
		if use == offsetIsset {
			return 0, false, nil
		}
	case valueUndefined, valueNull, valueBool, valueFloat:
		if use != offsetIsset {
			s.h.warn(levelWarning, "String offset cast occurred")
		}
		switch k.kind {
		case valueBool:
			if k.boolean {
				off = 1
			}
		case valueFloat:
			off = floatOffset(k.floating, s.h.width)
		}
		return off, true, nil
	}
	// zend_illegal_container_offset.
	if use == offsetUnset {
		return 0, false, &Throwable{"Error", "Cannot unset string offsets"}
	}
	return 0, false, &Throwable{"TypeError", "Cannot access offset of type " + typeName(k) + " on string"}
}

// stringOffsetGet is FETCH_DIM_R and FETCH_DIM_IS on a string: the byte at
// the offset as a one-byte string. Outside the string, a read warns and
// yields "", and ?? yields null.
func (s *vmState) stringOffsetGet(regs []value, in *vmInstr, str string, k value) error {
	quiet := in.aux == 1
	use := offsetRead
	if quiet {
		use = offsetIsset
	}
	off, ok, err := s.stringOffset(k, use)
	if err != nil || s.stopped() {
		return err
	}
	if !ok {
		return s.setReg(regs, in.a, nullValue())
	}
	n := int64(len(str))
	pos := off
	if pos < 0 {
		pos += n
	}
	if off < -n || pos >= n {
		if quiet {
			return s.setReg(regs, in.a, nullValue())
		}
		s.h.warn(levelWarning, "Uninitialized string offset "+strconv.FormatInt(off, 10))
		return s.setReg(regs, in.a, stringValue(""))
	}
	return s.setReg(regs, in.a, stringValue(str[pos:pos+1]))
}

// stringOffsetSet is ZEND_ASSIGN_DIM on a string (zend_assign_to_string_offset):
// the key, then the offset's diagnostics, then the value variable, then the
// write of the value's first byte. An offset before the string warns and
// writes nothing; one past its end pads with spaces. result, when the
// instruction has one, is the byte written, or null when nothing was.
func (s *vmState) stringOffsetSet(f *vmFrame, in *vmInstr, slot *value) error {
	flags := bytecode.ArrayWriteFlags(in.aux) & (bytecode.KeyLocation | bytecode.ValueLocation)
	var k value
	var err error
	if flags&bytecode.KeyLocation == 0 {
		k = operandValue(f.regs, in.b)
	} else if k, _, err = s.readVar(f, in.b); err != nil || s.stopped() {
		return err
	}
	off, _, err := s.stringOffset(k, offsetWrite)
	if err != nil || s.stopped() {
		return err
	}
	str := slot.deref().text
	n := int64(len(str))
	if off < -n {
		s.h.warn(levelWarning, "Illegal string offset "+strconv.FormatInt(off, 10))
		if s.stopped() {
			return nil
		}
		return s.arraySetResult(f, in, nullValue())
	}
	if off < 0 {
		off += n
	}
	var v value
	if flags&bytecode.ValueLocation == 0 {
		v = operandValue(f.regs, in.c)
	} else if v, _, err = s.readVar(f, in.c); err != nil || s.stopped() {
		return err
	}
	text := v.text
	if v.kind != valueString {
		if text, err = s.h.toString(v); err != nil || s.stopped() {
			return err
		}
	}
	if len(text) != 1 {
		if text == "" {
			return &Throwable{"Error", "Cannot assign an empty string to a string offset"}
		}
		s.h.warn(levelWarning, "Only the first byte will be assigned to the string offset")
		if s.stopped() {
			return nil
		}
	}
	if off >= vmStringOffsetLimit {
		return s.x.fail(endLimit, &LimitError{Resource: "memory", Limit: vmStringOffsetLimit, Used: n, Requested: off + 1 - n})
	}
	out, ok := s.offsetString(str, off, text[0], s.poll)
	if !ok {
		return nil
	}
	if err := s.h.assign(slot, stringValue(out)); err != nil {
		return err
	}
	return s.arraySetResult(f, in, stringValue(text[:1]))
}

// vmSpaces is a block of the padding a string offset write appends.
var vmSpaces = strings.Repeat(" ", 4096)

// offsetString is str with the byte b at offset off (0 <= off <
// vmStringOffsetLimit), padded with spaces when off is past its end. ok is
// false when poll reports a host stop, which it is asked before a large
// result is allocated and after every vmCopyChunk bytes copied or padded.
//
// PHP writes in place when the string has one owner; Go strings are
// immutable and a value carries no owner count, so a write builds a new
// string. A write that extends the string the previous one produced costs
// only the bytes it adds: that string's bytes are the start of
// offsetBuf, and strings.Builder only ever appends past the bytes of the
// strings it has returned, so every holder of an earlier result keeps its
// bytes. A write inside the string still copies it.
func (s *vmState) offsetString(str string, off int64, b byte, poll func() bool) (out string, ok bool) {
	n := int64(len(str))
	need := max(n, off+1)
	ob := &s.offsetBuf
	// str == s.offsetLast compares the data pointers first, so the usual
	// case, the string the last write produced, costs nothing; a different
	// string of the same length costs at most the copy it saves.
	extend := off >= n && s.offsetValid && int64(ob.Cap()) >= need && str == s.offsetLast
	s.offsetValid, s.offsetLast = false, ""
	if !extend {
		if need > vmCopyChunk && poll() {
			return "", false
		}
		// Reset leaves the bytes of the strings already returned alone.
		ob.Reset()
		size := need
		if off >= n {
			// Room to append as much again, so a run of appends copies
			// the string a logarithmic number of times.
			size = min(max(need, 2*n), vmStringOffsetLimit)
		}
		ob.Grow(int(size))
	}
	w := chunkWriter{b: ob, poll: poll}
	if !extend {
		if !w.write(str[:min(off, n)]) {
			return "", false
		}
	}
	if off > n && !w.pad(off-n) {
		return "", false
	}
	ob.WriteByte(b)
	if off < n-1 && !w.write(str[off+1:]) {
		return "", false
	}
	s.offsetValid, s.offsetLast = true, ob.String()
	return s.offsetLast, true
}

// chunkWriter appends to a strings.Builder, calling poll after every
// vmCopyChunk bytes and stopping when it reports a host stop.
type chunkWriter struct {
	b     *strings.Builder
	poll  func() bool
	since int
}

func (w *chunkWriter) room() bool {
	if w.since < vmCopyChunk {
		return true
	}
	w.since = 0
	return !w.poll()
}

func (w *chunkWriter) write(p string) bool {
	for len(p) > 0 {
		if !w.room() {
			return false
		}
		k := min(len(p), vmCopyChunk-w.since)
		w.b.WriteString(p[:k])
		p, w.since = p[k:], w.since+k
	}
	return true
}

func (w *chunkWriter) pad(n int64) bool {
	for n > 0 {
		if !w.room() {
			return false
		}
		k := int(min(n, int64(len(vmSpaces)), int64(vmCopyChunk-w.since)))
		w.b.WriteString(vmSpaces[:k])
		n, w.since = n-int64(k), w.since+k
	}
	return true
}

// arraySetResult stores the value of the assignment expression in the
// result register of an ArraySet that names one.
func (s *vmState) arraySetResult(f *vmFrame, in *vmInstr, v value) error {
	if r, ok := bytecode.ArraySetResult(in.aux); ok {
		return s.setReg(f.regs, r, v)
	}
	return nil
}

// --- foreach ---

// nextLive finds the first live bucket of a at or after pos and returns the
// position after it. poll is called every vmHoleCheck deleted elements and
// stops the search when it reports a host stop.
func nextLive(a *phpArray, pos int, poll func() bool) (next int, b *bucket, stopped bool) {
	if pos < 0 {
		return pos, nil, false
	}
	skipped := 0
	for pos < len(a.buckets) {
		b := a.buckets[pos]
		pos++
		if b != nil {
			return pos, b, false
		}
		if skipped++; skipped%vmHoleCheck == 0 && poll() {
			return pos, nil, true
		}
	}
	return pos, nil, false
}

// iterInit is ZEND_FE_RESET_R. The iterator register holds the array, which
// it retains, and in its integer field the position of the next element, as
// Zend keeps fe_pos beside the array. Retaining is all a by-value foreach
// needs: a write to the variable separates it from the iterator's array, so
// the loop sees the array as it was and nothing is copied until then. A
// subject that is not an array warns, clears the register and is not
// iterated.
func (s *vmState) iterInit(f *vmFrame, in *vmInstr) (iterable bool, err error) {
	v := f.regs[in.b].deref()
	switch v.kind {
	case valueArray:
		return true, s.setReg(f.regs, in.a, value{kind: valueArray, array: v.array})
	case valueObject:
		return false, s.unsupported("iterating an object")
	}
	old := f.regs[in.a]
	f.regs[in.a] = value{}
	if err := s.releaseValue(old); err != nil {
		return false, err
	}
	// zend_zval_value_name: false and true are named, where the heap's
	// warnForeach says "bool".
	s.h.warn(levelWarning, "foreach() argument must be of type array|object, "+valueName(v)+" given")
	return false, nil
}

// iterNext is ZEND_FE_FETCH_R: the next element is copied to the destination
// register, or assigned to the destination variable as FE_FETCH_R assigns a
// CV. It reports false when the iteration is over or the run stopped.
func (s *vmState) iterNext(f *vmFrame, in *vmInstr) (more bool, err error) {
	it := &f.regs[in.a]
	if it.kind != valueArray {
		return false, s.unsupported("advancing an iterator that IterInit did not create")
	}
	pos := -1
	if it.integer >= 0 && it.integer <= int64(len(it.array.buckets)) {
		pos = int(it.integer)
	}
	pos, b, stopped := nextLive(it.array, pos, s.poll)
	if stopped || b == nil {
		if pos >= 0 {
			it.integer = int64(pos)
		}
		return false, nil
	}
	it.integer = int64(pos)
	return true, s.iterStore(f, in.aux, in.b, b.val.deref())
}

// iterKey is the key of the element IterNext fetched last.
func (s *vmState) iterKey(f *vmFrame, in *vmInstr) error {
	it := f.regs[in.b]
	if it.kind != valueArray || it.integer <= 0 || it.integer > int64(len(it.array.buckets)) || it.array.buckets[it.integer-1] == nil {
		return s.unsupported("reading the key of an iterator that has no current element")
	}
	k := it.array.buckets[it.integer-1].key
	if k.isString {
		return s.iterStore(f, in.aux, in.a, stringValue(k.text))
	}
	return s.iterStore(f, in.aux, in.a, s.h.number(k.integer))
}

// iterStore writes v to a Value register, or assigns it to a Location with
// bytecode.IterLocation.
func (s *vmState) iterStore(f *vmFrame, aux uint16, r uint32, v value) error {
	if bytecode.IterFlags(aux) != bytecode.IterLocation {
		return s.setReg(f.regs, r, v)
	}
	slot, err := s.slot(f, r)
	if err != nil {
		return err
	}
	if slot.kind < valueArray && v.kind < valueArray {
		*slot = v
		return nil
	}
	return s.h.assign(slot, v)
}

// --- parameter defaults ---

// vmCachedDefault is one entry of RECV_INIT's run-time cache. emptyArray
// stands for PHP's shared empty array, which each call receives as a new
// empty array.
type vmCachedDefault struct {
	set        bool
	emptyArray bool
	v          value
}

// vmDefaultCacheable reports whether ZEND_RECV_INIT keeps v, the default it
// just evaluated, for later calls: it keeps a value that is not refcounted.
// That is null, bool, int, float, the empty array and an interned string,
// which for the defaults the compiler lets through is one of at most one
// byte (internal/vm refuses a default whose string result PHP may or may
// not intern; see defaultMayBeInterned in internal/vm/function.go). A
// default PHP folded when it compiled the function has no diagnostics, so
// whether it is kept is invisible.
func vmDefaultCacheable(v value) (vmCachedDefault, bool) {
	switch v.kind {
	case valueNull, valueBool, valueInt, valueFloat:
		return vmCachedDefault{set: true, v: v}, true
	case valueString:
		if len(v.text) <= 1 {
			return vmCachedDefault{set: true, v: v}, true
		}
	case valueArray:
		if v.array != nil && v.array.live == 0 && v.array.nextFree == math.MinInt64 {
			return vmCachedDefault{set: true, emptyArray: true}, true
		}
	}
	return vmCachedDefault{}, false
}

// cachedDefault puts the cached default of parameter p in its register and
// reports whether there was one.
func (s *vmState) cachedDefault(f *vmFrame, p uint32) (bool, error) {
	fi := f.fn.index
	if fi >= len(s.defaults) || int(p) >= len(s.defaults[fi]) || !s.defaults[fi][p].set {
		return false, nil
	}
	d := s.defaults[fi][p]
	if d.emptyArray {
		return true, s.setReg(f.regs, p, s.h.newArrayValue())
	}
	return true, s.setReg(f.regs, p, d.v)
}

// cacheDefault runs at the prologue Store that receives a parameter with a
// default (decoded c is the parameter plus one): when the call omitted the
// argument, the register holds the default just evaluated, which is cached
// if PHP would keep it.
func (s *vmState) cacheDefault(f *vmFrame, in *vmInstr) {
	p := int(in.c) - 1
	if p < f.nargs || p >= f.fn.params {
		return
	}
	fi := f.fn.index
	if fi < len(s.defaults) && s.defaults[fi] != nil && s.defaults[fi][p].set {
		return
	}
	d, ok := vmDefaultCacheable(f.regs[p])
	if !ok {
		return
	}
	if s.defaults == nil {
		s.defaults = make([][]vmCachedDefault, len(s.prog.funcs))
	}
	if s.defaults[fi] == nil {
		s.defaults[fi] = make([]vmCachedDefault, f.fn.params)
	}
	s.defaults[fi][p] = d
}
