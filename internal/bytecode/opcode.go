package bytecode

func opcodeSafety(op Opcode) (Safety, bool) {
	s := Safety{Steps: 1, CheckCancel: true}
	switch op {
	case OpCheckpoint, OpJump, OpBranch:
	case OpPush:
		s.MayAllocate = true
	case OpConst, OpMove, OpLocal, OpLoad, OpStore, OpBinary, OpDrop, OpLeave, OpEndFinally, OpReturn, OpReturnRef, OpThrow, OpOutput, OpNewArray, OpArrayGet, OpCall, OpCallValue, OpHostCall, OpInclude, OpEval, OpShell:
		// Replacing or releasing a runtime value can run user destructors.
		s.MayAllocate, s.MayThrow, s.MayCallUser = true, true, true
	case OpUnary, OpCast, OpIncDec, OpArraySet, OpArrayAppend, OpArrayUnset, OpIsset, OpArrayIsset, OpUnset, OpExit, OpArrayAdd:
		// Format 1.1. Conversions can call __toString, and writes, unsets
		// and exit can release values and run destructors.
		s.MayAllocate, s.MayThrow, s.MayCallUser = true, true, true
	case OpFetchDim, OpIterInit, OpIterNext, OpIterKey:
		// Format 1.2. Element fetches separate and grow arrays and convert
		// keys; the iterator instructions retain, release and assign values.
		s.MayAllocate, s.MayThrow, s.MayCallUser = true, true, true
	case OpParamDefault:
		// A branch on the frame's argument count.
	case OpBind, OpMakeRef:
		// Format 1.3. Binding releases the destination's old value, and
		// making a reference can create an element or a null variable.
		s.MayAllocate, s.MayThrow, s.MayCallUser = true, true, true
	case Opcode(0):
		return Safety{}, false
	default:
		return Safety{}, false
	}
	switch op {
	case OpHostCall:
		s.Capabilities |= HostCallbacks
	case OpInclude:
		s.Capabilities |= FileRead
	case OpEval:
		s.Capabilities |= DynamicCode
	case OpShell:
		s.Capabilities |= Shell
	}
	return s, true
}
