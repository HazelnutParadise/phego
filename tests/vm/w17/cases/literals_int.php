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

show(0);
show(-0);
show(42);
show(0x1F);
show(0XFF);
show(0777);
show(0b1011);
show(0B11);
show(2147483647);
show(2147483648);
show(-2147483648);
show(4294967295);
show(9223372036854775807);
show(9223372036854775808);
show(-9223372036854775807 - 1);
show(-9223372036854775808);
show(0x7FFFFFFF);
show(0xFFFFFFFF);
show(0x7FFFFFFFFFFFFFFF);
show(0xFFFFFFFFFFFFFFFF);
show(0b11111111111111111111111111111111);
