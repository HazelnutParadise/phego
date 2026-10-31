; Native side of the W-07 handle probe. It sees only fixed-width integers and
; its own stack memory; every Go value is reached through the Go accessors.
declare i32 @phegoHandleLen(i64, i64, ptr)
declare i32 @phegoHandleAt(i64, i64, i64, ptr)

; Returns 0 and stores sum(item[i] * (i + 1)), or returns the first accessor
; status unchanged without writing the result. The weight makes element order
; and argument order observable.
define i32 @phego_native_weighted_sum(i64 %owner, i64 %handle, ptr %result) {
entry:
  %len.addr = alloca i64
  %item.addr = alloca i64
  %len.status = call i32 @phegoHandleLen(i64 %owner, i64 %handle, ptr %len.addr)
  %len.ok = icmp eq i32 %len.status, 0
  br i1 %len.ok, label %start, label %len.fail
start:
  %len = load i64, ptr %len.addr
  br label %loop
loop:
  %i = phi i64 [ 0, %start ], [ %next, %body ]
  %acc = phi i64 [ 0, %start ], [ %acc.next, %body ]
  %done = icmp uge i64 %i, %len
  br i1 %done, label %finish, label %fetch
fetch:
  %item.status = call i32 @phegoHandleAt(i64 %owner, i64 %handle, i64 %i, ptr %item.addr)
  %item.ok = icmp eq i32 %item.status, 0
  br i1 %item.ok, label %body, label %item.fail
body:
  %item = load i64, ptr %item.addr
  %next = add i64 %i, 1
  %weighted = mul i64 %item, %next
  %acc.next = add i64 %acc, %weighted
  br label %loop
finish:
  store i64 %acc, ptr %result
  ret i32 0
len.fail:
  ret i32 %len.status
item.fail:
  ret i32 %item.status
}
