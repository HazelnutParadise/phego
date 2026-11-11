<?php
function show($v)
{
    if ($v === null) {
        echo "NULL\n";
    } elseif ($v === true) {
        echo "bool(true)\n";
    } elseif ($v === false) {
        echo "bool(false)\n";
    } elseif ($v !== $v) {
        echo "float(NAN)\n";
    } elseif ((array)$v === $v) {
        echo "array\n";
    } elseif ((float)$v === $v) {
        echo "float(", $v, ")\n";
    } elseif ((string)$v === $v) {
        echo "string(\"", $v, "\")\n";
    } else {
        echo "int(", $v, ")\n";
    }
}

$a = 10;
$a += 5;
show($a);
$a -= 20;
show($a);
$a *= -2;
show($a);
$a /= 4;
show($a);
$a %= 2;
show($a);
$b = 2;
$b **= 10;
show($b);
$b .= "!";
show($b);
$c = 12;
$c &= 10;
show($c);
$c |= 1;
show($c);
$c ^= 3;
show($c);
$c <<= 2;
show($c);
$c >>= 1;
show($c);
$d = "3";
$d += 1;
show($d);
$e = 1;
$e += $e += 1;
show($e);
$x = $y = 4;
$x += 1;
show($x);
show($y);
$arr = [1, 2];
$arr[0] += 10;
$arr[1] .= "x";
show($arr[0]);
show($arr[1]);
