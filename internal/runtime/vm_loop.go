package runtime

import (
	"strconv"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// loop is the production dispatch: one switch over the decoded instructions
// of the top frame. Every instruction first charges one step; the slow check
// at s.next enforces the step budget before any effect and polls
// cancellation at most vmPollInterval steps apart. Back-edges, calls,
// returns and Checkpoint poll as well. Errors leave through s.stop, so the
// large completion struct never crosses the per-instruction path.
func (s *vmState) loop() {
	h := s.h
	f := &s.frames[len(s.frames)-1]
	code, regs := f.code, f.regs
	pc := 0
	for {
		f.pc = pc
		s.steps++
		if s.steps >= s.next && s.slow() {
			return
		}
		if uint(pc) >= uint(len(code)) {
			f.pc = len(code) - 1
			s.stop(s.internal("control reached the end of the function"))
			return
		}
		in := &code[pc]
		switch in.op {
		case bytecode.OpCheckpoint:
			if s.poll() {
				return
			}
			goto advance
		case bytecode.OpConst:
			old := regs[in.a]
			regs[in.a] = s.prog.consts[in.b]
			if old.kind >= valueArray {
				if err := s.releaseValue(old); err != nil {
					s.stop(err)
					return
				}
			}
			goto advance
		case bytecode.OpMove:
			if in.aux == 1 {
				f.locs[in.a] = f.locs[in.b]
			} else if err := s.setReg(regs, in.a, regs[in.b]); err != nil {
				s.stop(err)
				return
			}
			goto advance
		case bytecode.OpLocal:
			f.locs[in.a] = int32(in.b) + 1
			goto advance
		case bytecode.OpLoad:
			l := f.locs[in.b]
			if l <= 0 || int(l) > len(f.locals) {
				if err := s.loadElement(f, in); err != nil {
					s.stop(err)
					return
				}
				goto advance
			}
			v := f.locals[l-1]
			if v.kind == valueReference {
				v = v.ref.target
			}
			if v.kind == valueUndefined {
				if in.aux == 0 {
					h.warn(levelWarning, "Undefined variable $"+f.fn.localNames[l-1])
					if s.stopped() {
						return
					}
				}
				v = nullValue()
			}
			if old := regs[in.a]; v.kind < valueArray && old.kind < valueArray {
				regs[in.a] = v
			} else if err := s.setReg(regs, in.a, v); err != nil {
				s.stop(err)
				return
			}
			goto advance
		case bytecode.OpStore:
			if in.c != 0 {
				// The Store that receives a parameter with a default.
				s.cacheDefault(f, in)
			}
			l := f.locs[in.a]
			if l <= 0 || int(l) > len(f.locals) {
				if err := s.storeElement(f, in); err != nil {
					s.stop(err)
					return
				}
				goto advance
			}
			slot := &f.locals[l-1]
			v := regs[in.b]
			if v.kind == valueUndefined {
				v = nullValue()
			}
			if slot.kind < valueArray && v.kind < valueArray {
				*slot = v
			} else if err := h.assign(slot, v); err != nil {
				s.stop(err)
				return
			}
			goto advance
		case bytecode.OpBinary:
			op := bytecode.BinaryOp(in.aux)
			a, b := regs[in.b], regs[in.c]
			r, ok := s.fastBinary(op, a, b)
			if !ok {
				var err error
				if r, err = s.binary(op, a, b); err != nil {
					s.stop(err)
					return
				}
				if s.stopped() {
					return
				}
			}
			if old := regs[in.a]; r.kind < valueArray && old.kind < valueArray {
				regs[in.a] = r
			} else if err := s.setReg(regs, in.a, r); err != nil {
				s.stop(err)
				return
			}
			goto advance
		case opFusedCompareBranch:
			op := bytecode.BinaryOp(in.aux)
			a, b := regs[in.b], regs[in.c]
			r, ok := s.fastBinary(op, a, b)
			if !ok {
				var err error
				if r, err = s.binary(op, a, b); err != nil {
					s.stop(err)
					return
				}
				if s.stopped() {
					return
				}
			}
			if err := s.setReg(regs, in.a, r); err != nil {
				s.stop(err)
				return
			}
			// The Branch at pc+1, with its own step and location.
			pc++
			f.pc = pc
			s.steps++
			if s.steps >= s.next && s.slow() {
				return
			}
			br := &code[pc]
			target := int(br.c)
			if r.boolean {
				target = int(br.b)
			}
			if target <= pc && s.poll() {
				return
			}
			pc = target
			continue
		case bytecode.OpJump:
			target := int(in.a)
			if target <= pc && s.poll() {
				return
			}
			pc = target
			continue
		case bytecode.OpBranch:
			v := regs[in.a]
			var cond bool
			if v.kind == valueBool {
				cond = v.boolean
			} else {
				cond = h.toBool(v)
				if s.stopped() {
					return
				}
			}
			if in.nkill != 0 {
				if err := s.kill(f, in); err != nil {
					s.stop(err)
					return
				}
			}
			target := int(in.c)
			if cond {
				target = int(in.b)
			}
			if target <= pc && s.poll() {
				return
			}
			pc = target
			continue
		case bytecode.OpPush:
			if in.b != 0 {
				s.stop(s.unsupported("named arguments"))
				return
			}
			if in.aux == 1 {
				if err := s.pushRef(f, in); err != nil {
					s.stop(err)
					return
				}
				goto advance
			}
			v := regs[in.a]
			switch {
			case v.kind == valueUndefined:
				v = nullValue()
			case v.kind == valueReference:
				// A by-reference argument (SEND_REF): the reference MakeRef
				// put in the register moves to the argument stack, so no
				// extra owner is left behind whether or not liveness runs.
				regs[in.a] = value{}
			case v.kind >= valueArray:
				retain(v)
			}
			s.args = append(s.args, v)
		case bytecode.OpCall:
			if in.aux != 0 {
				s.stop(s.unsupported("calls returning a location"))
				return
			}
			if err := s.push(&s.prog.funcs[in.b], int(in.c), in.a); err != nil {
				s.stop(err)
				return
			}
			if s.poll() {
				return
			}
			f = &s.frames[len(s.frames)-1]
			code, regs = f.code, f.regs
			pc = 0
			continue
		case bytecode.OpCallValue:
			if err := s.callValue(regs[in.b], int(in.c), in.a); err != nil {
				s.stop(err)
				return
			}
			if s.poll() {
				return
			}
			f = &s.frames[len(s.frames)-1]
			code, regs = f.code, f.regs
			pc = 0
			continue
		case bytecode.OpReturn:
			v := take(regs, in.a)
			if len(s.frames) == 1 {
				s.ended = true
				s.end = completion{kind: endReturn, value: v, returnOwned: true}
				return
			}
			if err := s.ret(v); err != nil {
				s.stop(err)
				return
			}
			if s.poll() {
				return
			}
			f = &s.frames[len(s.frames)-1]
			code, regs = f.code, f.regs
			pc = f.pc + 1
			continue
		case bytecode.OpOutput:
			if c := s.output(regs[in.a]); c.kind != endNormal {
				s.stop(c)
				return
			}
			if s.ended {
				return
			}
		case bytecode.OpNewArray:
			if err := s.setReg(regs, in.a, h.newArrayValue()); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpArrayGet:
			if err := s.arrayGet(regs, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpDrop:
			if in.aux == 1 {
				f.locs[in.a] = 0
			} else {
				v := regs[in.a]
				regs[in.a] = value{}
				if err := s.releaseValue(v); err != nil {
					s.stop(err)
					return
				}
			}
		case bytecode.OpUnary:
			r, err := s.unary(bytecode.UnaryOp(in.aux), regs[in.b])
			if err == nil {
				err = s.setReg(regs, in.a, r)
			}
			if err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpCast:
			r, err := s.cast(bytecode.CastKind(in.aux), regs[in.b])
			if err == nil {
				err = s.setReg(regs, in.a, r)
			}
			if err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpIncDec:
			if err := s.incDec(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpArraySet:
			if err := s.arraySet(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpArrayAppend:
			if err := s.arrayAppend(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpArrayUnset:
			if err := s.arrayUnset(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpIsset:
			if err := s.isset(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpArrayAdd:
			if err := s.arrayAdd(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpArrayIsset:
			if err := s.arrayIsset(regs, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpUnset:
			if err := s.unset(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpFetchDim:
			if err := s.fetchDim(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpIterInit, bytecode.OpIterNext:
			var more bool
			var err error
			switch {
			case in.op == bytecode.OpIterInit && in.aux&uint16(bytecode.IterByRef) != 0:
				more, err = s.iterInitRef(f, in)
			case in.op == bytecode.OpIterInit:
				more, err = s.iterInit(f, in)
			case regs[in.a].kind == valueReference:
				if ri := s.refIter(regs[in.a]); ri != nil {
					more, err = s.iterNextRef(f, in, ri)
				} else {
					err = s.unsupported("advancing an iterator that IterInit did not create")
				}
			default:
				more, err = s.iterNext(f, in)
			}
			if err != nil {
				s.stop(err)
				return
			}
			if s.stopped() {
				return
			}
			if more {
				goto advance
			}
			if in.nkill != 0 {
				if err := s.kill(f, in); err != nil {
					s.stop(err)
					return
				}
			}
			target := int(in.c)
			if target <= pc && s.poll() {
				return
			}
			pc = target
			continue
		case bytecode.OpBind:
			if err := s.bind(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpMakeRef:
			if err := s.makeRef(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpIterKey:
			if ri := s.refIter(regs[in.b]); ri != nil {
				if err := s.iterKeyRef(f, in, ri); err != nil {
					s.stop(err)
					return
				}
				goto advance
			}
			if err := s.iterKey(f, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpFetchConst:
			if err := s.fetchConst(f, in, uint64(f.fn.index)<<32|uint64(pc)); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpDeclareConst:
			if err := s.declareConst(regs, in); err != nil {
				s.stop(err)
				return
			}
		case bytecode.OpBeginSilence:
			// ZEND_BEGIN_SILENCE: the old error_reporting is saved in the
			// register, then only the fatal levels stay on.
			if err := s.setReg(regs, in.a, value{kind: valueInt, integer: int64(s.reporting)}); err != nil {
				s.stop(err)
				return
			}
			s.silence()
			goto advance
		case bytecode.OpEndSilence:
			if err := s.unsilence(regs[in.a]); err != nil {
				s.stop(err)
				return
			}
			goto advance
		case bytecode.OpMatchError:
			s.stop(s.matchError(regs[in.a]))
			return
		case bytecode.OpParamDefault:
			// RECV_INIT: the default's code is skipped for a passed argument,
			// and for a default an earlier call evaluated and cached.
			passed := int(in.a) < f.nargs
			if !passed {
				hit, err := s.cachedDefault(f, in.a)
				if err != nil {
					s.stop(err)
					return
				}
				passed = hit
			}
			if passed {
				target := int(in.b)
				if target <= pc && s.poll() {
					return
				}
				pc = target
				continue
			}
			goto advance
		case bytecode.OpExit:
			code := 0
			if in.aux == 1 {
				n, c, err := s.exitStatus(regs[in.a])
				if err != nil {
					s.stop(err)
					return
				}
				if c.kind != endNormal {
					s.stop(c)
					return
				}
				if s.stopped() {
					return
				}
				code = n
			}
			s.ended = true
			s.end = completion{kind: endExit, code: code}
			return
		default:
			s.stop(s.unsupported("the " + opcodeName(in.op) + " instruction"))
			return
		}
		if s.stopped() {
			return
		}
	advance:
		if in.nkill != 0 {
			if err := s.kill(f, in); err != nil {
				s.stop(err)
				return
			}
		}
		pc++
	}
}

func opcodeName(op bytecode.Opcode) string {
	switch op {
	case bytecode.OpHostCall:
		return "HostCall"
	case bytecode.OpReturnRef:
		return "ReturnRef"
	case bytecode.OpThrow:
		return "Throw"
	case bytecode.OpInclude:
		return "Include"
	case bytecode.OpEval:
		return "Eval"
	case bytecode.OpShell:
		return "Shell"
	case bytecode.OpLeave:
		return "Leave"
	case bytecode.OpEndFinally:
		return "EndFinally"
	}
	return "opcode " + strconv.Itoa(int(op))
}
