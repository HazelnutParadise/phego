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

$max32 = 2147483647;
show($max32 + 1);
show(-$max32 - 2);
show($max32 * 2);
show(65536 * 65536);
$max64 = 9223372036854775807;
show($max64);
show($max64 + 1);
show($max64 - 1);
show(-$max64 - 1);
show(-$max64 - 2);
show($max64 * 2);
show($max64 * -1);
$min = -$max64 - 1;
show(-$min);
show($min / -1);
show($min % -1);
show(3037000500 * 3037000500);
$x = $max64;
$x++;
show($x);
$y = $min;
$y--;
show($y);
$z = $max32;
$z++;
show($z);
$min32 = -2147483647 - 1;
show($min32);
show(-$min32);
show($min32 / -1);
show($min32 % -1);
show($min % 3);
show(-9 % 4);
