; The native function passes only fixed-width integers and returns a status.
declare i32 @phego_go_callback(i64, i64)
define i32 @phego_native_invoke(i64 %owner, i64 %token) {
entry:
  %status = call i32 @phego_go_callback(i64 %owner, i64 %token)
  ret i32 %status
}
