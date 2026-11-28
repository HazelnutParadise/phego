package runtime

import (
	"errors"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// W-17 stage 7C: REQ-001, REQ-016, REQ-025, REQ-026. try, catch and finally
// over the exception table of bytecode 1.0. PHP's frames are the VM's own
// frames, not Go frames, so the unwinding walks the table instead of
// calling execution.tryCatch; the completion rules are W-14's: exit, fatal
// errors and host stops pass every handler, a finally block's return
// replaces the pending completion, and a throw that replaces a pending
// throw keeps it as its previous.

// vmHandler is one entry of a function's exception table.
type vmHandler struct {
	start, end, target, bodyEnd int32
	reg                         uint32
	catch                       bool
	// live are the registers that are live at target, in ascending order.
	live []uint32
}

// The kinds of a pending completion: what an EndFinally goes on with.
const (
	pendThrow uint8 = iota + 1
	pendReturn
	pendJump
)

// vmPending is the completion a running finally block suspended
// (ZEND_FAST_CALL's variable): the exception to rethrow, the value to
// return, or the target of the break, continue or fall-through.
type vmPending struct {
	h      int32
	kind   uint8
	target int
	val    value
	ex     *object
	// live are the registers live at a jump's target.
	live []uint32
	// carry is the register plus one a jump writes val to at its target
	// (bytecode.LeaveCarry), or zero.
	carry uint32
}

// decodeHandlers copies the exception table and indexes it: per pc the
// innermost handler whose protected range holds it, the innermost Finally
// handler whose range holds it, and the innermost handler whose body holds
// it. The verifier proved the regions laminar.
func decodeHandlers(f *vmFunction, fv bytecode.FunctionView) {
	n := fv.HandlerCount()
	if n == 0 {
		return
	}
	f.handlers = make([]vmHandler, n)
	for i := 0; i < n; i++ {
		h, _ := fv.Handler(uint32(i))
		f.handlers[i] = vmHandler{start: int32(h.Start), end: int32(h.End), target: int32(h.Target), bodyEnd: int32(h.BodyEnd), catch: h.Kind == bytecode.Catch}
	}
	size := len(f.code)
	f.handlerAt, f.finallyAt, f.bodyAt = make([]int32, size), make([]int32, size), make([]int32, size)
	for pc := range f.handlerAt {
		f.handlerAt[pc], f.finallyAt[pc], f.bodyAt[pc] = -1, -1, -1
	}
	// A smaller region nested in a larger one wins: paint the larger
	// first. Table order is by start, then by longer range first, so for
	// laminar regions a later entry is never larger than one it overlaps.
	span := func(at []int32, lo, hi int32, i int) {
		for pc := lo; pc < hi && int(pc) < size; pc++ {
			if old := at[pc]; old < 0 || hi-lo <= f.handlers[old].end-f.handlers[old].start {
				at[pc] = int32(i)
			}
		}
	}
	for i, h := range f.handlers {
		span(f.handlerAt, h.start, h.end, i)
		if !h.catch {
			span(f.finallyAt, h.start, h.end, i)
		}
	}
	for i, h := range f.handlers {
		for pc := h.target; pc < h.bodyEnd && int(pc) < size; pc++ {
			if old := f.bodyAt[pc]; old < 0 || h.bodyEnd-h.target <= f.handlers[old].bodyEnd-f.handlers[old].target {
				f.bodyAt[pc] = int32(i)
			}
		}
	}
}

// escaped is how an exception that no frame of this run handles ends it:
// the uncaught error for {main}'s run, and for a nested run the exception
// itself, which the instruction that started the run goes on throwing.
func (s *vmState) escaped(ex *object) completion {
	if s.floor <= 1 {
		// No PHP frame is below this run: {main}, or code the engine
		// calls at request shutdown.
		return s.uncaughtObject(ex)
	}
	return completion{kind: endThrow, err: &vmThrown{object: ex}}
}

// raise handles the error of the top frame's current instruction. An
// error PHP code can catch becomes its exception object and is thrown: it
// reports true when a handler took over, and the loop then continues at
// the top frame's pc. Otherwise the run has ended.
func (s *vmState) raise(err error) bool {
	if s.ended {
		return false
	}
	var c completion
	if errors.As(err, &c) && c.kind == endExit {
		// exit, here or in a nested run (a destructor, a magic method,
		// __toString): PHP's unwind_exit goes on through the frames of
		// this run too.
		return s.exitFrames(c.code)
	}
	ex, ok, fail := s.thrownObject(err)
	if !ok {
		s.stop(fail)
		return false
	}
	return s.throwObject(ex)
}

// exitFrames is exit with status code: PHP 8 unwinds the frames of the run
// (unwind_exit) without running finally blocks. A destructor that throws
// while a frame's values are released replaces the exit: the exception is
// thrown in the caller, where a catch can take it and the script goes on
// (zend_throw_exception_internal over an unwind_exit). It reports true
// when a handler took that exception over.
func (s *vmState) exitFrames(code int) bool {
	for len(s.frames) > s.floor {
		if err := s.pop(true); err != nil {
			return s.raise(err)
		}
	}
	s.ended = true
	s.end = completion{kind: endExit, code: code}
	return false
}

// chain makes the throwable of err, raised while ex was being thrown, the
// current exception with ex as its previous (zend_objects_destroy_object,
// zend_exception_set_previous). ok is false when err is not a throwable:
// the run has then ended.
func (s *vmState) chain(ex *object, err error) (*object, bool) {
	next, ok, fail := s.thrownObject(err)
	if !ok {
		_ = s.releaseObject(ex)
		s.stop(fail)
		return nil, false
	}
	s.setPrevious(next, ex)
	return next, true
}

// chainErrors combines two errors raised one after the other while a
// frame's values are released. When both are PHP exceptions, the later one
// becomes the current exception with the earlier one as its previous, as a
// destructor that throws while an exception is pending does
// (zend_exception_set_previous). Otherwise the earlier error stands, as
// before.
func (s *vmState) chainErrors(first, next error) error {
	if first == nil {
		return next
	}
	if next == nil {
		return first
	}
	// The execution chains a throw that replaces a pending one itself
	// (replaceCompletion): then the later error already carries the
	// earlier one as its previous.
	var c1, c2 completion
	if errors.As(first, &c1) && errors.As(next, &c2) && c1.serial != 0 && containsCompletion(c2.previous, c1.serial) {
		return next
	}
	b, ok, _ := s.thrownObject(next)
	if !ok {
		return first
	}
	a, ok, _ := s.thrownObject(first)
	if !ok {
		_ = s.releaseObject(b)
		return first
	}
	s.setPrevious(b, a)
	return &vmThrown{object: b}
}

// throwObject is ZEND_HANDLE_EXCEPTION: it unwinds frames until one has a
// handler for its current instruction and enters that handler with ex
// (owned).
func (s *vmState) throwObject(ex *object) bool {
	for {
		if s.x.stop.kind != endNormal {
			_ = s.releaseObject(ex)
			return !s.stopped()
		}
		f := s.frames[len(s.frames)-1]
		fn := f.fn
		hi := int32(-1)
		if fn.handlerAt != nil && f.pc < len(fn.handlerAt) {
			hi = fn.handlerAt[f.pc]
		}
		if hi >= 0 {
			h := &fn.handlers[hi]
			s.discardPending(f, int(h.target), ex)
			saved := s.throwing
			s.throwing = ex
			err := s.enterHandler(f, h.live)
			ex, s.throwing = s.throwing, saved
			if err != nil {
				var ok bool
				if ex, ok = s.chain(ex, err); !ok {
					return false
				}
				continue
			}
			if s.x.phaseCompletion.kind == endThrow {
				// W-14's tryCatch: handling an exception ends the pending
				// event the execution recorded for it. The VM chains a
				// later throw to what is still pending itself.
				s.x.phaseCompletion = completion{}
			}
			if h.catch {
				old := f.regs[h.reg]
				f.regs[h.reg] = value{kind: valueObject, object: ex}
				if err := s.releaseValue(old); err != nil {
					f.pc = int(h.target)
					return s.raise(err)
				}
			} else {
				f.pend = append(f.pend, vmPending{h: hi, kind: pendThrow, ex: ex})
			}
			f.pc = int(h.target)
			return true
		}
		s.discardPending(f, -1, ex)
		if len(s.frames) <= s.floor {
			s.ended = true
			s.end = s.escaped(ex)
			return false
		}
		saved := s.throwing
		s.throwing = ex
		err := s.pop(true)
		ex, s.throwing = s.throwing, saved
		if err != nil {
			var ok bool
			if ex, ok = s.chain(ex, err); !ok {
				return false
			}
		}
	}
}

// keepError combines err, raised while a frame's values are released, with
// first, the error raised before it. While an exception is being thrown
// (s.throwing), an exception err becomes the current one at once, with the
// one thrown before as its previous; otherwise the two are chained by
// chainErrors.
func (s *vmState) keepError(first, err error) error {
	if err == nil {
		return first
	}
	if s.throwing != nil && first == nil {
		if next, ok, _ := s.thrownObject(err); ok {
			s.setPrevious(next, s.throwing)
			s.throwing = next
			return nil
		}
	}
	return s.chainErrors(first, err)
}

// enterHandler prepares frame f for a handler a throw enters, where the
// registers of live are live: the calls the frame was building and the
// temporaries of the code it leaves are freed in PHP's order
// (releaseUnwound), and so are the arguments no call took
// (Handler.StackDepth is zero).
func (s *vmState) enterHandler(f *vmFrame, live []uint32) error {
	var first error
	keep := func(err error) { first = s.keepError(first, err) }
	s.releaseUnwound(f, live, false, keep)
	s.releaseArgs(f, keep)
	return first
}

// releaseArgs frees, in order, the arguments above frame f's base of the
// argument stack that no unfinished call owns: those a call instruction
// failed to take.
func (s *vmState) releaseArgs(f *vmFrame, keep func(error)) {
	if len(s.args) <= f.argBase {
		return
	}
	args := append([]value(nil), s.args[f.argBase:]...)
	clear(s.args[f.argBase:])
	s.args = s.args[:f.argBase]
	for _, v := range args {
		keep(s.releaseValue(v))
	}
}

// releaseDead releases the registers of f that are not in live. A function
// without liveness facts keeps its registers until they are overwritten.
func (s *vmState) releaseDead(f *vmFrame, live []uint32) error {
	if !f.fn.liveKnown {
		return nil
	}
	var first error
	k := 0
	for r := range f.regs {
		for k < len(live) && int(live[k]) < r {
			k++
		}
		if k < len(live) && int(live[k]) == r {
			continue
		}
		v := f.regs[r]
		if v.kind < valueArray {
			continue
		}
		f.regs[r] = value{}
		if err := s.releaseValue(v); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// releaseRegisters releases every register of f: the frame is returning
// after a finally block, and PHP freed its temporaries before the
// variables.
func (s *vmState) releaseRegisters(f *vmFrame) error {
	var first error
	for r := range f.regs {
		v := f.regs[r]
		if v.kind < valueArray {
			continue
		}
		f.regs[r] = value{}
		if err := s.releaseValue(v); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// discardPending gives up the completions of the finally blocks control
// leaves on its way to dest (-1: out of the frame). A pending exception
// becomes the previous of ex, the exception being thrown; without one it
// is released, as a return in a finally block discards it
// (ZEND_DISCARD_EXCEPTION). A pending return value is released.
func (s *vmState) discardPending(f *vmFrame, dest int, ex *object) {
	for n := len(f.pend); n > 0; n-- {
		p := &f.pend[n-1]
		h := &f.fn.handlers[p.h]
		if dest >= int(h.target) && dest < int(h.bodyEnd) {
			break
		}
		switch {
		case p.kind == pendThrow && ex != nil:
			s.setPrevious(ex, p.ex)
		case p.kind == pendThrow:
			_ = s.releaseObject(p.ex)
		case p.kind == pendReturn, p.kind == pendJump:
			// A return value, or one a jump carries (bytecode.LeaveCarry):
			// PHP frees an incomplete return's value when the finally
			// block ends otherwise.
			_ = s.releaseValue(p.val)
		}
		*p = vmPending{}
		f.pend = f.pend[:n-1]
	}
}

// dropPending releases what a frame that ends without unwinding still
// holds (exit, a fatal error).
func (s *vmState) dropPending(f *vmFrame, release bool) {
	for i := range f.pend {
		if release {
			if f.pend[i].ex != nil {
				_ = s.releaseObject(f.pend[i].ex)
			}
			_ = s.releaseValue(f.pend[i].val)
		}
		f.pend[i] = vmPending{}
	}
	f.pend = f.pend[:0]
}

// through finds the finally block a transfer from pc to target runs first:
// the innermost Finally handler whose range holds pc and not target (-1
// for a return). It is -1 when there is none.
func through(fn *vmFunction, pc, target int) int32 {
	if fn.finallyAt == nil || pc >= len(fn.finallyAt) {
		return -1
	}
	h := fn.finallyAt[pc]
	if h < 0 || target >= int(fn.handlers[h].start) && target < int(fn.handlers[h].end) {
		return -1
	}
	return h
}

// leave is OpLeave at pc: a jump to target, where the registers of live
// are live, that first runs the finally blocks it leaves. It returns the
// pc to continue at. carry is the register plus one whose value the jump
// carries (bytecode.LeaveCarry), or zero; held says the value is val,
// taken out of the register when an earlier finally block began.
func (s *vmState) leave(f *vmFrame, pc, target int, live []uint32, carry uint32, val value, held bool) (int, error) {
	fn := f.fn
	if h := through(fn, pc, target); h >= 0 {
		t := int(fn.handlers[h].target)
		if len(f.pend) != 0 {
			s.discardPending(f, t, nil)
		}
		if carry != 0 && !held {
			// The finally block's code may reuse the register: the value
			// waits in the completion, as PHP keeps a return value in
			// the temporary FAST_CALL leaves alone.
			val = f.regs[carry-1]
			f.regs[carry-1] = value{}
		}
		f.pend = append(f.pend, vmPending{h: h, kind: pendJump, target: target, live: live, val: val, carry: carry})
		return t, s.releaseDead(f, fn.handlers[h].live)
	}
	if len(f.pend) != 0 {
		s.discardPending(f, target, nil)
	}
	err := s.releaseDead(f, live)
	if carry != 0 && held {
		old := f.regs[carry-1]
		f.regs[carry-1] = val
		err = s.chainErrors(err, s.releaseValue(old))
	}
	return target, err
}

// returnThrough is a Return of v (owned) at pc: when a finally block has
// to run first, the value waits in its pending completion and the pc of
// the block is returned.
func (s *vmState) returnThrough(f *vmFrame, pc int, v value) (int, bool, error) {
	h := through(f.fn, pc, -1)
	if h < 0 {
		s.discardPending(f, -1, nil)
		return 0, false, nil
	}
	t := int(f.fn.handlers[h].target)
	s.discardPending(f, t, nil)
	f.pend = append(f.pend, vmPending{h: h, kind: pendReturn, val: v})
	return t, true, s.releaseDead(f, f.fn.handlers[h].live)
}

// endFinally is OpEndFinally at pc: it takes the completion the finally
// block suspended. A jump continues at npc, through the next finally block
// it leaves; a return or a throw is handed back to the loop.
func (s *vmState) endFinally(f *vmFrame, pc int) (kind uint8, npc int, p vmPending, err error) {
	fn := f.fn
	if fn.bodyAt == nil || len(f.pend) == 0 || f.pend[len(f.pend)-1].h != fn.bodyAt[pc] {
		return 0, 0, vmPending{}, s.internal("EndFinally without the completion of its finally block")
	}
	p = f.pend[len(f.pend)-1]
	f.pend[len(f.pend)-1] = vmPending{}
	f.pend = f.pend[:len(f.pend)-1]
	if p.kind == pendJump {
		npc, err = s.leave(f, pc, p.target, p.live, p.carry, p.val, true)
		p.val = value{}
	}
	return p.kind, npc, p, err
}

// throwValue is OpThrow of register r: the error that throws its value.
// The exception moves out of the register, so the thrown object has no
// holder there whether or not the function has liveness facts.
func (s *vmState) throwValue(regs []value, r uint32) error {
	v := regs[r].deref()
	if v.kind != valueObject {
		return &Throwable{"Error", "Can only throw objects"}
	}
	if !isThrowableClass(v.object.class) {
		return &Throwable{"Error", "Cannot throw objects that do not implement Throwable"}
	}
	retain(v)
	old := regs[r]
	regs[r] = value{}
	if err := s.releaseValue(old); err != nil {
		_ = s.releaseValue(v)
		return err
	}
	return &vmThrown{object: v.object}
}
