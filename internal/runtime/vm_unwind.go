package runtime

import (
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
	ex, ok, fail := s.thrownObject(err)
	if !ok {
		s.stop(fail)
		return false
	}
	return s.throwObject(ex)
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
			if err := s.enterHandler(f, int(h.target)); err != nil {
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
		if err := s.pop(true); err != nil {
			var ok bool
			if ex, ok = s.chain(ex, err); !ok {
				return false
			}
		}
	}
}

// enterHandler prepares frame f for the code at target, which a throw, a
// return or a Leave enters: the arguments and calls the frame was building
// are dropped (Handler.StackDepth is zero) and every register that is not
// live there is released, in register order, as PHP frees the live
// temporaries of the code it leaves (cleanup_live_vars).
func (s *vmState) enterHandler(f *vmFrame, target int) error {
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	for len(s.args) > f.argBase {
		v := s.args[len(s.args)-1]
		s.args[len(s.args)-1] = value{}
		s.args = s.args[:len(s.args)-1]
		keep(s.releaseValue(v))
	}
	if len(s.calls) > 0 {
		keep(s.dropCalls(len(s.frames)))
	}
	keep(s.releaseDead(f, target))
	return first
}

// releaseDead releases the registers of f that are not live at pc. A
// function without liveness facts keeps its registers until they are
// overwritten.
func (s *vmState) releaseDead(f *vmFrame, pc int) error {
	fn := f.fn
	if !fn.liveKnown {
		return nil
	}
	live := fn.liveAt[int32(pc)]
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
		case p.kind == pendReturn:
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

// leave is OpLeave at pc: a jump to target that first runs the finally
// blocks it leaves. It returns the pc to continue at.
func (s *vmState) leave(f *vmFrame, pc, target int) (int, error) {
	fn := f.fn
	if h := through(fn, pc, target); h >= 0 {
		t := int(fn.handlers[h].target)
		s.discardPending(f, t, nil)
		f.pend = append(f.pend, vmPending{h: h, kind: pendJump, target: target})
		return t, s.releaseDead(f, t)
	}
	s.discardPending(f, target, nil)
	return target, s.releaseDead(f, target)
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
	return t, true, s.releaseDead(f, t)
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
		npc, err = s.leave(f, pc, p.target)
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
