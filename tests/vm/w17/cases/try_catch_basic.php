<?php
// try_catch_basic: try/catch around a throw, a non-throwing try, statements before and after
function risky($n)
{
    echo "risky start: ", $n, "\n";
    if ($n > 1) {
        throw new RuntimeException("too big: " . $n);
    }
    return $n * 10;
}

echo "before: 1\n";
try {
    $r = risky(1);
    echo "ok result: ", $r, "\n";
    $r = risky(2);
    echo "not reached: ", $r, "\n";
} catch (RuntimeException $e) {
    echo "caught class: ", $e::class, "\n";
    echo "caught message: ", $e->getMessage(), "\n";
}
echo "after catch: 1\n";

try {
    echo "body without throw\n";
} catch (RuntimeException $e) {
    echo "never\n";
}
echo "after empty catch: 1\n";

$flag = 0;
try {
    $flag = 1;
    throw new LogicException("L");
    $flag = 2;
} catch (LogicException $e) {
    echo "flag after throw: ", $flag, "\n";
}
echo "end: 1\n";
