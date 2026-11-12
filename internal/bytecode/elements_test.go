package bytecode

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// W-17 stage 2 review fixes: REQ-025, REQ-026; T-049, T-050. The use of an
// element location (verifyElementLocations). The frame: r0 Value parameter,
// r1 Value array, r2 Location of local 0 ($a, the base), r3 Value, r4
// Location (the element), r5 Location of local 1 ($b), r6 another Location
// of local 0. The body starts at pc 5.
func elementImage(body ...Instruction) Image {
	i := format12Image()
	f := &i.Functions[0]
	f.Registers = []RegisterKind{Value, Value, Location, Value, Location, Location, Location}
	f.Locals = 2
	code := []Instruction{{Op: OpConst, A: 0, B: 0}, {Op: OpNewArray, A: 1}, {Op: OpLocal, A: 2, B: 0}, {Op: OpLocal, A: 5, B: 1}, {Op: OpLocal, A: 6, B: 0}}
	code = append(code, body...)
	if last := code[len(code)-1].Op; last != OpExit && last != OpReturn {
		code = append(code, Instruction{Op: OpReturn, A: 0})
	}
	f.Code = code
	f.Positions = make([]Position, len(code))
	return i
}

const elementBody = 5

var (
	fetchA    = Instruction{Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 2, C: 0}
	fetchRW   = Instruction{Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchForAssignOp), A: 4, B: 2, C: 0}
	fetchNext = Instruction{Op: OpFetchDim, Aux: uint16(FetchWrite), A: 4, B: 4, C: 0}
	key       = Instruction{Op: OpConst, A: 3, B: 0}
	storeE    = Instruction{Op: OpStore, A: 4, B: 0}
)

// The sequences internal/vm emits, and harmless variations of them.
func TestElementLocationSequencesAreAccepted(t *testing.T) {
	for name, body := range map[string][]Instruction{
		"a fetch nothing uses":             {fetchA},
		"store":                            {fetchA, storeE},
		"a chain with literal keys":        {fetchA, key, fetchNext, key, fetchNext, {Op: OpArraySet, A: 4, B: 3, C: 0}},
		"a chain with a key in a variable": {fetchA, {Op: OpFetchDim, Aux: uint16(FetchWrite | FetchKeyLocation), A: 4, B: 4, C: 5}, {Op: OpArrayAppend, A: 4, B: 0}},
		"increment":                        {{Op: OpFetchDim, Aux: uint16(FetchReadWrite | FetchForIncDec), A: 4, B: 2, C: 0}, {Op: OpIncDec, Aux: uint16(PostInc), A: 3, B: 4}},
		"unset":                            {{Op: OpFetchDim, Aux: uint16(FetchUnset), A: 4, B: 2, C: 0}, key, {Op: OpArrayUnset, A: 4, B: 3}},
		"unset with a variable key":        {{Op: OpFetchDim, Aux: uint16(FetchUnset), A: 4, B: 2, C: 0}, {Op: OpArrayUnset, Aux: uint16(KeyLocation), A: 4, B: 2}},
		"assign-op":                        {fetchRW, {Op: OpLoad, A: 3, B: 5}, {Op: OpLoad, Aux: 1, A: 0, B: 4}, key, {Op: OpBinary, Aux: uint16(Concat), A: 0, B: 0, C: 3}, storeE},
		"a value read by the write":        {fetchA, key, {Op: OpArraySet, Aux: uint16(ValueLocation | KeyLocation), A: 4, B: 2, C: 5}},
		"a result register":                {fetchA, {Op: OpArraySet, Aux: arraySetAuxRaw(0, 3), A: 4, B: 0, C: 0}},
		"a new chain after the last":       {fetchA, storeE, fetchA, storeE},
		"a fetch at a jump target":         {{Op: OpJump, A: elementBody + 1}, fetchA, storeE},
	} {
		t.Run(name, func(t *testing.T) { requireValidBuild(t, elementImage(body...)) })
	}
}

func TestElementLocationMisusesAreRejected(t *testing.T) {
	for name, body := range map[string][]Instruction{
		"a fetch into a variable":       {{Op: OpFetchDim, A: 2, B: 2, C: 0}},
		"a chain from a closed fetch":   {fetchA, storeE, fetchNext},
		"a chain into another element":  {fetchA, {Op: OpFetchDim, A: 2, B: 4, C: 0}},
		"a second use after the write":  {fetchA, storeE, storeE},
		"a load of the base":            {fetchA, {Op: OpLoad, A: 3, B: 2}, storeE},
		"a load of the base's alias":    {fetchA, {Op: OpLoad, A: 3, B: 6}, storeE},
		"a store to another variable":   {fetchA, {Op: OpStore, A: 5, B: 0}, storeE},
		"a binary before the load":      {fetchA, {Op: OpBinary, Aux: uint16(Add), A: 0, B: 0, C: 0}, storeE},
		"two loads":                     {fetchRW, {Op: OpLoad, A: 3, B: 4}, {Op: OpLoad, A: 3, B: 4}, storeE},
		"a write after the load":        {fetchRW, {Op: OpLoad, A: 3, B: 4}, {Op: OpArraySet, A: 4, B: 3, C: 3}},
		"isset":                         {fetchA, {Op: OpIsset, A: 3, B: 4}},
		"unset":                         {fetchA, {Op: OpUnset, A: 4}},
		"move":                          {fetchA, {Op: OpMove, A: 2, B: 4}},
		"drop":                          {fetchA, {Op: OpDrop, A: 4}},
		"push":                          {fetchA, {Op: OpPush, A: 4}, {Op: OpCallValue, A: 3, B: 0, C: 1}},
		"an iterator target":            {fetchA, {Op: OpIterNext, Aux: uint16(IterLocation), A: 1, B: 4, C: elementBody + 2}},
		"an iterator key target":        {fetchA, {Op: OpIterKey, Aux: uint16(IterLocation), A: 4, B: 1}},
		"the element as a key":          {fetchA, {Op: OpArraySet, Aux: uint16(KeyLocation), A: 2, B: 4, C: 0}},
		"the element as its own key":    {fetchA, {Op: OpFetchDim, Aux: uint16(FetchKeyLocation), A: 4, B: 4, C: 4}},
		"the base as the value":         {fetchA, key, {Op: OpArraySet, Aux: uint16(ValueLocation), A: 4, B: 3, C: 2}},
		"the base as an appended value": {fetchA, {Op: OpArrayAppend, Aux: uint16(ValueLocation), A: 4, B: 6}},
		"a use reached by a jump":       {fetchA, {Op: OpJump, A: elementBody + 2}, storeE},
		"a use at a branch target":      {fetchA, {Op: OpBranch, A: 0, B: elementBody + 2, C: elementBody + 2}, storeE},
		"a use after an iterator":       {fetchA, {Op: OpIterInit, A: 3, B: 1, C: elementBody + 2}, storeE},
		"a use after a call":            {fetchA, {Op: OpCallValue, A: 3, B: 0, C: 0}, storeE},
		"a use after output":            {fetchA, {Op: OpOutput, A: 0}, storeE},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Build(context.Background(), elementImage(body...), testOptions())
			var e *Error
			if !errors.As(err, &e) || !errors.Is(err, ErrInvalid) || !strings.Contains(e.Detail, "element location") || e.PC < 0 || e.Function != 0 {
				t.Fatalf("Build = %v, want ErrInvalid for the element location at its instruction", err)
			}
		})
	}
	// A Location two Locals bind is not known to differ from the base.
	i := elementImage(fetchRW, Instruction{Op: OpLoad, A: 3, B: 5}, Instruction{Op: OpLoad, A: 0, B: 4}, Instruction{Op: OpBinary, Aux: uint16(Add), A: 0, B: 0, C: 3}, storeE)
	i.Functions[0].Code = append([]Instruction{{Op: OpLocal, A: 5, B: 0}}, i.Functions[0].Code...)
	i.Functions[0].Positions = append(i.Functions[0].Positions, Position{})
	requireBuildError(t, i, ErrInvalid)
}
