; Fixed-width numeric operation only; no PHP semantics are asserted.
define i64 @phego_probe_xor(i64 %left, i64 %right) {
entry:
  %result = xor i64 %left, %right
  ret i64 %result
}
