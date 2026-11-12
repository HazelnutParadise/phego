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
				if err := h.release(old); err != nil {
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
				s.stop(s.internal("location register %d read before Local", in.b))
				return
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
			l := f.locs[in.a]
			if l <= 0 || int(l) > len(f.locals) {
				s.stop(s.internal("location register %d read before Local", in.a))
				return
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
				s.stop(s.unsupported("passing a location as an argument"))
				return
			}
			v := regs[in.a]
			if v.kind == valueUndefined {
				v = nullValue()
			}
			if v.kind >= valueArray {
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
