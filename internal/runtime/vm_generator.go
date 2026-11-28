package runtime

import (
	"errors"
	"strconv"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 8: REQ-001, REQ-016, REQ-025, REQ-026; T-029..T-032.
// Generators (Zend/zend_generators.c).
//
// The Generator class and the iterator interfaces are immutable and shared
// between requests. A Generator object, its suspended frame, the values it
// yielded, the value sent in and the generator it delegates to belong to
// the run that created it.
//
// A call of a generator function receives its parameters like any call and
// stops at GeneratorCreate (ZEND_GENERATOR_CREATE): the frame moves out of
// the value, location and element stacks into storage of its own and the
// Generator object is the call's result. A resume pushes that frame as the
// bottom frame of a nested run; Yield ends the run with the frame taken off
// the stack again, at the instruction it suspended at.
//
// Delegation (yield from a generator) follows PHP's tree without its
// caches: each generator names the generator it delegates to (from), and
// the one that runs is the innermost of that chain that has not finished
// (current).

// vmGenerator is what a Generator object holds (zend_generator).
type vmGenerator struct {
	// obj is the Generator object that holds this (not an owner).
	obj *object
	fn  *vmFunction
	// frame is the suspended frame (its pc names the GeneratorCreate,
	// Yield or YieldFrom it stopped at), or nil once the generator has
	// finished or was closed.
	frame *vmFrame
	// value and key are what it yielded last (undefined before the first
	// yield); largest is largest_used_integer_key.
	value, key value
	largest    int64
	// send is the register plus one that receives the value sent in
	// (send_target), zero when the generator has none.
	send uint32
	// retval is the value it returned; hasRet says it returned.
	retval value
	hasRet bool
	// values is the array or the iterator a yield from walks
	// (generator->values), with the array position or the iterator's
	// index in pos.
	values value
	pos    int64
	// from is the generator this one delegates to (an owner), or nil.
	from *object
	// The flags of zend_generator.
	running, forced, doInit, atFirst bool
	// throwIn is an exception to throw into the generator at its
	// suspension point when it is resumed (Generator::throw), owned.
	throwIn *object
	// via is the method of Generator that resumes it (for backtraces) and
	// viaArgs its arguments, borrowed while it runs.
	via     uint8
	viaArgs []value
}

// The methods of Generator the VM implements (methodSpec.native), after
// the methods of the exception classes.
const (
	genRewind uint8 = excGetSeverity + 1 + iota
	genValid
	genCurrent
	genKey
	genNext
	genSend
	genThrow
	genGetReturn
	genDebugInfo
)

var genNames = [...]string{genRewind - genRewind: "rewind", genValid - genRewind: "valid", genCurrent - genRewind: "current",
	genKey - genRewind: "key", genNext - genRewind: "next", genSend - genRewind: "send", genThrow - genRewind: "throw",
	genGetReturn - genRewind: "getReturn", genDebugInfo - genRewind: "__debugInfo"}

// isGeneratorNative reports a native kind of a method of Generator.
func isGeneratorNative(kind uint8) bool { return kind >= genRewind && kind <= genDebugInfo }

// traversableBuiltin is PHP's Traversable interface.
var traversableBuiltin = &class{name: "Traversable", schema: &classSchema{classSpec: classSpec{name: "Traversable", isInterface: true}}}

// iteratorBuiltin and iteratorAggregateBuiltin are PHP's Iterator and
// IteratorAggregate interfaces, whose methods carry tentative return
// types.
var iteratorBuiltin, iteratorAggregateBuiltin = func() (*class, *class) {
	abstract := func(name, ret string) methodSpec {
		return methodSpec{name: name, visibility: publicAccess, abstract: true, returnType: ret, tentative: true}
	}
	it, err := buildClass(classSpec{name: "Iterator", isInterface: true, interfaces: []*class{traversableBuiltin}, methods: []methodSpec{
		abstract("current", "mixed"), abstract("next", "void"), abstract("key", "mixed"), abstract("valid", "bool"), abstract("rewind", "void"),
	}})
	if err != nil {
		panic(err)
	}
	agg, err := buildClass(classSpec{name: "IteratorAggregate", isInterface: true, interfaces: []*class{traversableBuiltin}, methods: []methodSpec{
		abstract("getIterator", "Traversable"),
	}})
	if err != nil {
		panic(err)
	}
	return it, agg
}()

// generatorBuiltin is PHP's Generator class: final, never instantiated by
// new and never cloned. Its destructor runs the finally blocks of a
// suspended generator (zend_generator_dtor_storage).
var generatorBuiltin = func() *class {
	m := func(kind uint8, ret string) methodSpec {
		call := func(*heap, value, []value) (value, error) {
			return value{}, errors.New("a method of Generator was called outside the VM")
		}
		return methodSpec{name: genNames[kind-genRewind], visibility: publicAccess, native: kind, returnType: ret, call: call}
	}
	c, err := buildClass(classSpec{name: "Generator", final: true, noDynamic: true, interfaces: []*class{iteratorBuiltin},
		methods: []methodSpec{m(genRewind, "void"), m(genValid, "bool"), m(genCurrent, "mixed"), m(genKey, "mixed"), m(genNext, "void"),
			m(genSend, "mixed"), m(genThrow, "mixed"), m(genGetReturn, "mixed"), m(genDebugInfo, "array")},
		destructor: func(h *heap, this value) error {
			if h.vm == nil || this.kind != valueObject || this.object.gen == nil {
				return nil
			}
			return generatorDestroy(h.vm, this.object)
		}})
	if err != nil {
		panic(err)
	}
	return c
}()

// generatorDestroy is (*vmState).destroyGenerator, set at init: the
// class's destructor cannot name it directly without an initialization
// cycle through the throwable classes.
var generatorDestroy func(*vmState, *object) error

func init() { generatorDestroy = (*vmState).destroyGenerator }

// builtinMethodOrder is the declaration order of the methods of the
// internal classes whose order PHP's messages show.
var builtinMethodOrder = map[*class][]string{
	iteratorBuiltin:          {"current", "next", "key", "valid", "rewind"},
	iteratorAggregateBuiltin: {"getiterator"},
	generatorBuiltin:         {"rewind", "valid", "current", "key", "next", "send", "throw", "getreturn", "__debuginfo"},
}

// iterationClass returns the built-in class of the iteration interfaces
// and Generator by lower-case name, or nil.
func iterationClass(lower string) *class {
	switch lower {
	case "traversable":
		return traversableBuiltin
	case "iterator":
		return iteratorBuiltin
	case "iteratoraggregate":
		return iteratorAggregateBuiltin
	case "generator":
		return generatorBuiltin
	}
	return nil
}

// registerIteration registers Generator and the iterator interfaces with
// the run, once.
func (s *vmState) registerIteration() error {
	if s.generatorClass {
		return nil
	}
	for _, c := range [...]*class{traversableBuiltin, iteratorBuiltin, iteratorAggregateBuiltin, generatorBuiltin} {
		if s.h.classes[phpNameLower(c.name)] == nil {
			if err := s.h.registerClass(c); err != nil {
				return err
			}
		}
	}
	s.generatorClass = true
	return nil
}

// vmGenSplitStorage gives a generator's frame separate blocks for its
// registers and its locals. The layout benchmark measures it against the
// one block a generator gets (engineering/decisions/W-17-vm.md, stage 8).
var vmGenSplitStorage = false

// createGenerator is ZEND_GENERATOR_CREATE at pc of the top frame f: the
// frame moves into a new Generator object, which becomes the result of
// the call that entered it.
func (s *vmState) createGenerator(f *vmFrame, pc int) error {
	if f.generator != nil || !f.fn.generator {
		return s.internal("GeneratorCreate in a frame that is not a generator function's call")
	}
	if err := s.registerIteration(); err != nil {
		return err
	}
	v, err := s.h.newObjectShell(generatorBuiltin)
	if err != nil {
		return err
	}
	o := v.object
	gf := new(vmFrame)
	*gf = *f
	if vmGenSplitStorage {
		gf.regs = append([]value(nil), f.regs...)
		gf.locals = append([]value(nil), f.locals...)
	} else {
		// One block for the registers and the locals, as the value
		// stack's window had them.
		vals := make([]value, len(f.regs)+len(f.locals))
		copy(vals, f.regs)
		copy(vals[len(f.regs):], f.locals)
		gf.regs, gf.locals = vals[:len(f.regs):len(f.regs)], vals[len(f.regs):]
	}
	gf.locs = append([]int32(nil), f.locs...)
	gf.elems = make([]*value, len(f.elems))
	gf.pc, gf.dest, gf.boundary, gf.generator = pc, 0, false, o
	gf.freshV, gf.freshL, gf.freshE = false, false, false
	gf.argBase, gf.pend, gf.leaving = 0, nil, false
	o.gen = &vmGenerator{obj: o, fn: f.fn, frame: gf, largest: -1}
	// The frame's values now belong to the generator: the window is given
	// back without releasing them.
	clear(f.regs)
	s.releaseWindow(f)
	f.this, f.closure, f.extra, f.call, f.via = nil, nil, nil, nil, 0
	f.freshV, f.freshL, f.freshE = false, false, false
	dest := f.dest
	boundary := len(s.frames) == s.floor
	s.frames = s.frames[:len(s.frames)-1]
	var outer *class
	if n := len(s.frames); n > 0 {
		outer = s.frames[n-1].called
	}
	s.h.currentCalledScope = outer
	v.object.refcount++
	if boundary {
		// The call was a nested run's (getIterator, a callback): it ends
		// with the Generator as its result.
		s.ended = true
		s.end = completion{kind: endReturn, value: v, returnOwned: true}
		return nil
	}
	caller := s.frames[len(s.frames)-1]
	old := caller.regs[dest]
	caller.regs[dest] = v
	return s.releaseValue(old)
}

// genCurrent is zend_generator_get_current: the generator that runs when
// g is resumed, the innermost one of its delegation chain that has not
// finished. A delegate that finished meanwhile hands its return value to
// the generator that yielded from it (zend_generator_update_current), or,
// when it has none, that generator gets a ClosedGeneratorException thrown
// in.
func (s *vmState) genCurrent(g *vmGenerator) *vmGenerator {
	return s.genCurrentAfter(g, false)
}

// genCurrentAfter is genCurrent; with failing an exception is already on
// its way into the generator that yielded from a delegate that died, and
// no ClosedGeneratorException is made for it.
func (s *vmState) genCurrentAfter(g *vmGenerator, failing bool) *vmGenerator {
	cur := g
	for cur.from != nil {
		inner := cur.from.gen
		if inner.frame != nil {
			cur = inner
			continue
		}
		// The delegate is done: cur goes on after its yield from.
		from := cur.from
		cur.from = nil
		if inner.hasRet {
			if cur.frame != nil {
				retain(inner.value)
				_ = s.releaseValue(cur.value)
				cur.value = inner.value
				if r := cur.frame.code[cur.frame.pc]; r.a < uint32(len(cur.frame.regs)) {
					retain(inner.retval)
					_ = s.setReg(cur.frame.regs, r.a, inner.retval)
					_ = s.releaseValue(inner.retval)
				}
			}
		} else if cur.frame != nil && cur.throwIn == nil && !failing {
			ex, ok, _ := s.thrownObject(&Throwable{"ClosedGeneratorException", "Generator yielded from aborted, no return value available"})
			if ok {
				cur.throwIn = ex
			}
		}
		_ = s.releaseObject(from)
		if cur.throwIn != nil && !g.running && !cur.running {
			// The exception is thrown in at once, and g goes on
			// (zend_generator_update_current resumes it).
			if err := s.genResume(g.obj); err != nil && s.genErr == nil {
				s.genErr = err
			}
			return s.genCurrent(g)
		}
		break
	}
	return cur
}

// genCur is genCurrent with the error of a resume it made.
func (s *vmState) genCur(g *vmGenerator) (*vmGenerator, error) {
	c := s.genCurrent(g)
	err := s.genErr
	s.genErr = nil
	return c, err
}

// genEnsureInit is zend_generator_ensure_initialized: a generator that
// has not yielded yet runs to its first yield.
func (s *vmState) genEnsureInit(o *object) error {
	g := o.gen
	if g.value.kind == valueUndefined && g.frame != nil && g.from == nil {
		if err := s.genResume(o); err != nil {
			return err
		}
		g.atFirst = true
	}
	return nil
}

// genResume is zend_generator_resume.
func (s *vmState) genResume(orig *object) error {
	og := orig.gen
	g, err := s.genCur(og)
	if err != nil {
		return err
	}
	if g.frame == nil {
		return nil
	}
	delegator := og
	for {
		if g.running {
			return &Throwable{"Error", "Cannot resume an already running generator"}
		}
		if delegator.doInit {
			delegator.doInit = false
			if g.value.kind != valueUndefined {
				// An already initialized delegate is not advanced on the
				// first resumption.
				return nil
			}
		}
		og.atFirst = false
		if g.values.kind != valueUndefined && g.throwIn == nil {
			more, err := s.genNextDelegated(g)
			if err != nil {
				return err
			}
			if more {
				return nil
			}
		}
		out := s.runGenerator(g, og)
		switch {
		case out.kind == endThrow || out.kind == endExit || out.kind == endFatal || out.kind == endInternal || out.kind == endLimit || out.kind == endCancel:
			if g == og || out.kind != endThrow {
				return out
			}
			// The delegate failed: the exception is thrown into the
			// generator that yielded from it.
			var t *vmThrown
			if !errors.As(out.err, &t) || t.object == nil {
				return out
			}
			g = s.genCurrentAfter(og, true)
			if g.frame == nil {
				return out
			}
			if g.throwIn != nil {
				_ = s.releaseObject(g.throwIn)
			}
			g.throwIn, t.object = t.object, nil
			delegator = og
			continue
		case out.kind != endNormal && out.kind != endReturn:
			return out
		}
		if g.frame != nil && g.frame.code[g.frame.pc].op == bytecode.OpYieldFrom {
			// yield from was used: resume what it delegates to.
			delegator = g
			if g, err = s.genCur(og); err != nil {
				return err
			}
			continue
		}
		if g != og && g.hasRet {
			delegator = og
			if g, err = s.genCur(og); err != nil {
				return err
			}
			continue
		}
		return nil
	}
}

// runGenerator runs g's frame from where it is suspended until it yields,
// returns or an exception or exit leaves it, as a nested run. orig is the
// generator the resume was made for, whose method (orig.via) a backtrace
// shows below the frame.
func (s *vmState) runGenerator(g, orig *vmGenerator) completion {
	x := s.x
	if depth := len(x.frames) - 1 + len(s.frames); depth+1 > x.limits.depth {
		return x.fail(endLimit, limitHit("frames", x.limits.depth, depth, 1))
	}
	if s.nesting >= vmMaxNesting {
		return completion{kind: endThrow, err: &Throwable{"Error", "Maximum call stack size of 8339456 bytes (zend.max_allowed_stack_size - zend.reserved_stack_size) reached. Infinite recursion?"}}
	}
	s.nesting++
	throwing := s.throwing
	s.throwing = nil
	ended, end, floor := s.ended, s.end, s.floor
	s.ended, s.end = false, completion{}
	gf := g.frame
	gf.boundary = true
	gf.via = 0
	// A delegate runs above the frames of the generators that yield from
	// it, suspended at their yield from, as PHP links them for the
	// backtrace (zend_generator_check_placeholder_frame): the outermost
	// one shows the method of Generator that resumed it.
	base := len(s.frames)
	for x := orig; x != g && x != nil && x.frame != nil; {
		x.frame.via = 0
		if x == orig {
			x.frame.via = orig.via
		}
		x.frame.leaving = false
		s.frames = append(s.frames, x.frame)
		if x.from == nil {
			break
		}
		x = x.from.gen
	}
	if g == orig && orig.via != 0 {
		gf.via = orig.via
	}
	gf.argBase = len(s.args)
	s.frames = append(s.frames, gf)
	s.floor = len(s.frames)
	if s.h.currentCalledScope != gf.called {
		s.h.currentCalledScope = gf.called
	}
	g.running = true
	if ex := g.throwIn; ex != nil {
		// Thrown in at the instruction the generator is suspended at, as
		// if that yield had thrown it.
		g.throwIn = nil
		if g.values.kind != valueUndefined {
			_ = s.releaseValue(g.values)
			g.values = value{}
		}
		if s.throwObject(ex) {
			s.loop()
		}
	} else {
		gf.pc++
		s.loop()
	}
	g.running = false
	out := s.end
	if s.suspended {
		s.suspended = false
		out = completion{}
	} else {
		// The generator finished: what it returned is kept, and its frame
		// goes (zend_generator_close).
		if out.kind == endReturn {
			g.retval, g.hasRet = out.value, true
			if !out.returnOwned {
				retain(g.retval)
			}
			out = completion{kind: endReturn}
		}
		release := out.kind == endNormal || out.kind == endReturn || out.kind == endThrow || out.kind == endExit
		for len(s.frames) >= s.floor && len(s.frames) > 0 && s.frames[len(s.frames)-1] != gf {
			if err := s.pop(release); err != nil && release {
				out = s.genFold(out, err)
			}
		}
		if len(s.frames) > 0 && s.frames[len(s.frames)-1] == gf {
			s.throwing = nil
			if t := s.thrownOf(out); t != nil {
				s.throwing = t
			}
			err := s.pop(release)
			if t := s.thrownOf(out); t != nil && s.throwing != t {
				// A destructor's exception became the current one.
				out = completion{kind: endThrow, err: &vmThrown{object: s.throwing}}
			}
			s.throwing = nil
			if err != nil && release {
				out = s.genFold(out, err)
			}
		}
		g.frame = nil
	}
	// The delegators' frames leave the stack as they were.
	if len(s.frames) > base {
		clear(s.frames[base:])
		s.frames = s.frames[:base]
	}
	s.ended, s.end, s.floor = ended, end, floor
	s.throwing = throwing
	s.nesting--
	return out
}

// thrownOf is the exception completion c ends with, or nil.
func (s *vmState) thrownOf(c completion) *object {
	var t *vmThrown
	if c.kind == endThrow && errors.As(c.err, &t) {
		return t.object
	}
	return nil
}

// genFold combines err, raised while a finished generator's frame was
// released, with out, how it finished.
func (s *vmState) genFold(out completion, err error) completion {
	next, ok, fail := s.thrownObject(err)
	if !ok {
		var c completion
		if errors.As(fail, &c) {
			return c
		}
		return s.x.fail(endInternal, fail)
	}
	if prev := s.thrownOf(out); prev != nil {
		s.setPrevious(next, prev)
	}
	return completion{kind: endThrow, err: &vmThrown{object: next}}
}

// yield is ZEND_YIELD in generator frame f: the value and key replace the
// ones the generator yielded before, and the frame suspends.
func (s *vmState) yield(f *vmFrame, in *vmInstr) error {
	o := f.generator
	if o == nil || o.gen == nil {
		return s.internal("yield outside a generator's frame")
	}
	g := o.gen
	if g.forced {
		return &Throwable{"Error", "Cannot yield from finally in a force-closed generator"}
	}
	flags := in.aux
	var v value
	switch {
	case flags&uint16(bytecode.YieldValue) == 0:
		v = nullValue()
	case flags&uint16(bytecode.YieldNotice) != 0:
		s.h.warn(levelNotice, "Only variable references should be yielded by reference")
		if s.stopped() {
			return nil
		}
		v = f.regs[in.b].deref()
		retain(v)
	case flags&uint16(bytecode.YieldRef) != 0:
		v = f.regs[in.b]
		retain(v)
	default:
		v = f.regs[in.b].deref()
		retain(v)
	}
	var k value
	if flags&uint16(bytecode.YieldKey) != 0 {
		k = f.regs[in.c].deref()
		retain(k)
		if k.kind == valueInt && k.integer > g.largest {
			g.largest = k.integer
		}
	} else {
		g.largest++
		k, _ = intValue(g.largest, s.prog.width)
	}
	oldV, oldK := g.value, g.key
	g.value, g.key = v, k
	err := s.releaseValue(oldV)
	err = s.chainErrors(err, s.releaseValue(oldK))
	g.send = in.a + 1
	err = s.chainErrors(err, s.setReg(f.regs, in.a, nullValue()))
	return err
}

// yieldFrom is ZEND_YIELD_FROM in generator frame f. It reports whether
// the frame suspends: an array or an iterator is walked by the resumes
// from now on, a generator becomes the delegate; a delegate that already
// returned gives its return value at once.
func (s *vmState) yieldFrom(f *vmFrame, in *vmInstr) (bool, error) {
	o := f.generator
	if o == nil || o.gen == nil {
		return false, s.internal("yield from outside a generator's frame")
	}
	g := o.gen
	if g.forced {
		return false, &Throwable{"Error", "Cannot use \"yield from\" in a force-closed generator"}
	}
	v := f.regs[in.b].deref()
	switch {
	case v.kind == valueArray:
		retain(v)
		g.values, g.pos = v, 0
	case v.kind == valueObject && v.object.gen != nil:
		inner := v.object.gen
		if inner.frame == nil {
			return false, &Throwable{"Error", "Generator passed to yield from was aborted without proper return and is unable to continue"}
		}
		if inner.hasRet {
			retain(inner.retval)
			err := s.setReg(f.regs, in.a, inner.retval)
			return false, s.chainErrors(err, s.releaseValue(inner.retval))
		}
		if s.genCurrent(inner) == g {
			return false, &Throwable{"Error", "Impossible to yield from the Generator being currently run"}
		}
		v.object.refcount++
		g.from = v.object
		g.doInit = true
	case v.kind == valueObject && s.isTraversable(v.object):
		it, err := s.objectIterator(v.object, false)
		if err != nil {
			return false, err
		}
		if err := s.iterRewind(it.object); err != nil {
			_ = s.releaseValue(it)
			return false, err
		}
		g.values, g.pos = it, 0
	default:
		return false, &Throwable{"Error", "Can use \"yield from\" only with arrays and Traversables"}
	}
	g.send = 0
	return true, s.setReg(f.regs, in.a, nullValue())
}

// genNextDelegated is zend_generator_get_next_delegated_value: the next
// element of the array or the iterator g yields from becomes its value
// and key. It reports false when the walk is over, and g goes on after
// its yield from.
func (s *vmState) genNextDelegated(g *vmGenerator) (bool, error) {
	if g.values.kind == valueArray {
		a := g.values.array
		pos, b, stopped := nextLive(a, int(g.pos), s.poll)
		if stopped {
			return false, nil
		}
		if b == nil {
			_ = s.releaseValue(g.values)
			g.values = value{}
			return false, nil
		}
		g.pos = int64(pos)
		v := b.val.deref()
		retain(v)
		var k value
		if b.key.isString {
			k = stringValue(b.key.text)
		} else {
			k = s.h.number(b.key.integer)
		}
		oldV, oldK := g.value, g.key
		g.value, g.key = v, k
		return true, s.chainErrors(s.releaseValue(oldV), s.releaseValue(oldK))
	}
	more, v, k, err := s.objectIterStep(&g.values, &g.pos, true, true)
	if err != nil || !more {
		_ = s.releaseValue(g.values)
		g.values = value{}
		return false, err
	}
	oldV, oldK := g.value, g.key
	g.value, g.key = v, k
	return true, s.chainErrors(s.releaseValue(oldV), s.releaseValue(oldK))
}

// destroyGenerator is zend_generator_dtor_storage: a generator that is
// suspended inside a try with a finally block runs that finally block,
// and the ones around it, before its frame is freed; a yield there throws.
func (s *vmState) destroyGenerator(o *object) error {
	g := o.gen
	if g.values.kind != valueUndefined {
		v := g.values
		g.values = value{}
		if err := s.releaseValue(v); err != nil {
			return err
		}
	}
	if from := g.from; from != nil {
		g.from = nil
		if err := s.releaseObject(from); err != nil {
			return err
		}
	}
	gf := g.frame
	if gf == nil {
		return nil
	}
	if gf.fn.finallyAt == nil || gf.pc >= len(gf.fn.finallyAt) || gf.fn.finallyAt[gf.pc] < 0 || g.running {
		return s.closeGenerator(g)
	}
	h := gf.fn.handlers[gf.fn.finallyAt[gf.pc]]
	g.forced = true
	saved := s.throwing
	s.throwing = nil
	out := s.runGeneratorFinally(g, int(h.target))
	s.throwing = saved
	if g.frame != nil {
		if err := s.closeGenerator(g); err != nil && out.kind == endNormal {
			return err
		}
	}
	if out.kind != endNormal && out.kind != endReturn {
		return out
	}
	return nil
}

// runGeneratorFinally resumes g at target, the finally block around its
// suspension point, with nothing pending: the block and those around it
// run, and the generator ends at the last one.
func (s *vmState) runGeneratorFinally(g *vmGenerator, target int) completion {
	gf := g.frame
	// The temporaries the finally block does not use go first
	// (zend_generator_cleanup_unfinished_execution up to the finally op).
	var live []uint32
	if hi := gf.fn.finallyAt[gf.pc]; hi >= 0 {
		live = gf.fn.handlers[hi].live
	}
	var first error
	s.frames = append(s.frames, gf)
	s.releaseUnwound(gf, live, false, func(err error) { first = s.keepError(first, err) })
	s.frames = s.frames[:len(s.frames)-1]
	if first != nil {
		return completion{kind: endThrow, err: first}
	}
	gf.pend = append(gf.pend[:0], vmPending{h: gf.fn.finallyAt[gf.pc], kind: pendClose})
	gf.pc = target - 1
	return s.runGenerator(g, g)
}

// closeGenerator is zend_generator_close for a generator that did not
// finish: its variables, its object, its extra arguments and then the
// temporaries of the code it is suspended in are freed.
func (s *vmState) closeGenerator(g *vmGenerator) error {
	gf := g.frame
	if gf == nil {
		return nil
	}
	g.frame = nil
	var first error
	keep := func(err error) { first = s.keepError(first, err) }
	for i := range gf.locals {
		v := gf.locals[i]
		gf.locals[i] = value{}
		keep(s.releaseValue(v))
	}
	if gf.call != nil {
		keep(s.releaseValue(gf.call.named))
		gf.call = nil
	}
	if this := gf.this; this != nil {
		gf.this = nil
		keep(s.releaseObject(this))
	}
	for i, v := range gf.extra {
		gf.extra[i] = value{}
		keep(s.releaseValue(v))
	}
	gf.extra = nil
	for i := range gf.regs {
		v := gf.regs[i]
		gf.regs[i] = value{}
		keep(s.releaseValue(v))
	}
	s.dropPending(gf, true)
	if c := gf.closure; c != nil {
		gf.closure = nil
		keep(s.releaseObject(c))
	}
	return first
}

// freeGeneratorStorage releases what a Generator object holds when it is
// freed (zend_generator_free_storage): the frame, if it is still there,
// then the value, the key and the return value.
func (h *heap) freeGeneratorStorage(o *object, dead map[gcNode]bool) error {
	g := o.gen
	if g == nil {
		return nil
	}
	o.gen = nil
	var first error
	rel := func(v value) {
		first = h.mergeObjectErrors(first, h.releaseOutside(v, dead))
	}
	if gf := g.frame; gf != nil {
		g.frame = nil
		for _, vs := range [...][]value{gf.locals, gf.extra, gf.regs} {
			for i, v := range vs {
				vs[i] = value{}
				rel(v)
			}
		}
		for _, p := range gf.pend {
			rel(p.val)
			if p.ex != nil {
				rel(value{kind: valueObject, object: p.ex})
			}
		}
		if gf.this != nil {
			rel(value{kind: valueObject, object: gf.this})
		}
		if gf.closure != nil {
			rel(value{kind: valueObject, object: gf.closure})
		}
	}
	if g.from != nil {
		rel(value{kind: valueObject, object: g.from})
	}
	if g.throwIn != nil {
		rel(value{kind: valueObject, object: g.throwIn})
	}
	rel(g.values)
	rel(g.value)
	rel(g.key)
	if g.hasRet {
		rel(g.retval)
	}
	return first
}

// generatorEdges visits what a Generator object keeps alive
// (zend_generator_get_gc).
func generatorEdges(g *vmGenerator, cell func(value)) {
	if gf := g.frame; gf != nil {
		for _, vs := range [...][]value{gf.regs, gf.locals, gf.extra} {
			for _, v := range vs {
				cell(v)
			}
		}
		for _, p := range gf.pend {
			cell(p.val)
			if p.ex != nil {
				cell(value{kind: valueObject, object: p.ex})
			}
		}
		if gf.this != nil {
			cell(value{kind: valueObject, object: gf.this})
		}
		if gf.closure != nil {
			cell(value{kind: valueObject, object: gf.closure})
		}
	}
	if g.from != nil {
		cell(value{kind: valueObject, object: g.from})
	}
	cell(g.values)
	cell(g.value)
	cell(g.key)
	if g.hasRet {
		cell(g.retval)
	}
}

// callGenerator runs the method of Generator the pending call names.
func (s *vmState) callGenerator(rec *vmPendingCall, in *vmInstr) error {
	f := s.frames[len(s.frames)-1]
	o := rec.this
	if o == nil || o.gen == nil {
		return s.internal("a method of Generator called on another object")
	}
	name := genNames[rec.native-genRewind]
	args := s.args[rec.base:]
	for i := range args {
		if args[i].kind == valueReference {
			t := args[i].ref.target
			retain(t)
			old := args[i]
			args[i] = t
			if err := s.releaseValue(old); err != nil {
				return err
			}
		}
	}
	want := 0
	if rec.native == genSend || rec.native == genThrow {
		want = 1
	}
	if rec.named.kind == valueArray {
		pos := 0
		for b, ok := rec.named.array.next(&pos); ok; b, ok = rec.named.array.next(&pos) {
			pname := "value"
			if rec.native == genThrow {
				pname = "exception"
			}
			if want == 0 || b.key.text != pname {
				return &Throwable{"Error", "Unknown named parameter $" + b.key.text}
			}
			if len(args) > 0 {
				return &Throwable{"Error", "Named parameter $" + b.key.text + " overwrites previous argument"}
			}
			v := b.val.deref()
			retain(v)
			s.args = append(s.args, v)
		}
		named := rec.named
		rec.named = value{}
		if err := s.releaseValue(named); err != nil {
			return err
		}
		args = s.args[rec.base:]
	}
	if len(args) != want {
		plural := "s"
		if want == 1 {
			plural = ""
		}
		return &vmInternalFrameThrow{function: "Generator->" + name, args: append([]value(nil), args...),
			thrown: &Throwable{"ArgumentCountError", "Generator::" + name + "() expects exactly " + strconv.Itoa(want) + " argument" + plural + ", " + strconv.Itoa(len(args)) + " given"}}
	}
	if rec.native == genThrow && (args[0].kind != valueObject || !isThrowableClass(args[0].object.class)) {
		return &vmInternalFrameThrow{function: "Generator->throw", args: append([]value(nil), args...),
			thrown: &Throwable{"TypeError", "Generator::throw(): Argument #1 ($exception) must be of type Throwable, " + valueName(args[0]) + " given"}}
	}
	// The call stays on the argument stack while the generator runs: the
	// arguments show in a backtrace as those of the method's frame.
	o.refcount++
	defer func() { _ = s.releaseObject(o) }()
	g := o.gen
	savedVia, savedArgs := g.via, g.viaArgs
	g.via, g.viaArgs = rec.native, args
	out, err := s.genMethod(o, rec.native, args)
	g.via, g.viaArgs = savedVia, savedArgs
	if err != nil {
		if out.kind >= valueArray {
			_ = s.releaseValue(out)
		}
		return err
	}
	if s.stopped() {
		return nil
	}
	err = s.dropArgs(len(s.args) - rec.base)
	if perr := s.popCall(); err == nil {
		err = perr
	}
	if serr := s.setReg(f.regs, in.a, out); err == nil {
		err = serr
	}
	if out.kind >= valueArray {
		if rerr := s.releaseValue(out); err == nil {
			err = rerr
		}
	}
	return err
}

// genMethod is the method of Generator native names, on generator o with
// the arguments args. The result is owned.
func (s *vmState) genMethod(o *object, native uint8, args []value) (value, error) {
	g := o.gen
	copyOut := func(v value) value {
		v = v.deref()
		if v.kind == valueUndefined {
			return nullValue()
		}
		retain(v)
		return v
	}
	switch native {
	case genRewind:
		return nullValue(), s.genRewind(o)
	case genValid:
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		if _, err := s.genCur(g); err != nil {
			return value{}, err
		}
		return boolValue(g.frame != nil), nil
	case genCurrent, genKey:
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		root, err := s.genCur(g)
		if err != nil {
			return value{}, err
		}
		if g.frame == nil {
			return nullValue(), nil
		}
		if native == genCurrent {
			return copyOut(root.value), nil
		}
		return copyOut(root.key), nil
	case genNext:
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		return nullValue(), s.genResume(o)
	case genSend:
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		if g.frame == nil {
			return nullValue(), nil
		}
		root, err := s.genCur(g)
		if err != nil {
			return value{}, err
		}
		if root.send != 0 && !root.running && root.frame != nil {
			v := args[0].deref()
			retain(v)
			err := s.setReg(root.frame.regs, root.send-1, v)
			_ = s.releaseValue(v)
			if err != nil {
				return value{}, err
			}
		}
		if err := s.genResume(o); err != nil {
			return value{}, err
		}
		if root, err = s.genCur(g); err != nil {
			return value{}, err
		}
		if g.frame == nil {
			return nullValue(), nil
		}
		return copyOut(root.value), nil
	case genThrow:
		ex := args[0].object
		ex.refcount++
		if err := s.genEnsureInit(o); err != nil {
			_ = s.releaseObject(ex)
			return value{}, err
		}
		if g.frame == nil {
			// A closed generator throws the exception in the caller.
			return value{}, &vmThrown{object: ex}
		}
		root, err := s.genCur(g)
		if err != nil {
			_ = s.releaseObject(ex)
			return value{}, err
		}
		if root.running {
			_ = s.releaseObject(ex)
			return value{}, &Throwable{"Error", "Cannot resume an already running generator"}
		}
		if root.throwIn != nil {
			_ = s.releaseObject(root.throwIn)
		}
		root.throwIn = ex
		if err := s.genResume(o); err != nil {
			return value{}, err
		}
		if root, err = s.genCur(g); err != nil {
			return value{}, err
		}
		if g.frame == nil {
			return nullValue(), nil
		}
		return copyOut(root.value), nil
	case genGetReturn:
		if err := s.genEnsureInit(o); err != nil {
			return value{}, err
		}
		if !g.hasRet {
			return value{}, &Throwable{"Exception", "Cannot get return value of a generator that hasn't returned"}
		}
		return copyOut(g.retval), nil
	case genDebugInfo:
		arr := s.h.newArrayValue()
		name := g.fn.name
		if gf := g.frame; gf != nil && gf.scope != nil {
			name = gf.scope.name + "::" + name
		}
		if err := s.h.setElement(&arr, stringValue("function"), stringValue(name)); err != nil {
			return value{}, err
		}
		return arr, nil
	}
	return value{}, s.internal("unknown method %d of Generator", native)
}

// genRewind is zend_generator_rewind.
func (s *vmState) genRewind(o *object) error {
	if err := s.genEnsureInit(o); err != nil {
		return err
	}
	if !o.gen.atFirst {
		return &Throwable{"Exception", "Cannot rewind a generator that was already run"}
	}
	return nil
}
