<?php
function b($r)
{
    return $r ? "T" : "F";
}
echo b(true && false), b(true || false), b(true xor true), b(true xor false), "\n";
echo b(!true), b(!0), b(!""), b(!"a"), b(!0.0), b(!-0.0), b(!"0.0"), "\n";
echo b(1 && "0"), b(0 || "0"), b([] || null), b([0] && 1), "\n";
$x = true xor true;
echo b($x), "\n";
$y = (true xor true);
echo b($y), "\n";
echo b(!1 == 0), "\n";
$r = 5 && 0;
echo $r === false ? "bool false" : "other", "\n";
