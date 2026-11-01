<?php
// Recursive calls. All values stay below 2^31 on every platform.
function fib($n) { return $n < 2 ? $n : fib($n - 1) + fib($n - 2); }
echo fib(27), "\n";
