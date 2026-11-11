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
