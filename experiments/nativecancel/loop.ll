; Native code calls Go at every iteration; it does not perform external effects.
declare i32 @phego_cancel_validate(i64)
declare i32 @phego_cancel_checkpoint(i64)

define i32 @phego_cancel_run(i64 %token, i64 %limit) {
entry:
  %valid = call i32 @phego_cancel_validate(i64 %token)
  %invalid = icmp ne i32 %valid, 0
  br i1 %invalid, label %return_invalid, label %loop
return_invalid:
  ret i32 %valid
loop:
  %index = phi i64 [ 0, %entry ], [ %next, %continue ]
  %finished = icmp uge i64 %index, %limit
  br i1 %finished, label %complete, label %check
check:
  %status = call i32 @phego_cancel_checkpoint(i64 %token)
  %stop = icmp ne i32 %status, 0
  br i1 %stop, label %return_status, label %continue
return_status:
  ret i32 %status
continue:
  %next = add i64 %index, 1
  br label %loop
complete:
  ret i32 0
}
