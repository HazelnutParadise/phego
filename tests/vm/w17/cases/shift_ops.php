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

show(1 << 3);
show(-8 >> 1);
show(-1 >> 1);
show(1 << 30);
show(1 << 31);
show(1 << 32);
show(1 << 62);
show(1 << 63);
show(1 << 64);
show(1 << 100);
show(-1 >> 63);
show(-1 >> 64);
show(5 >> 64);
show(-5 >> 200);
show("3" << "2");
show(1.9 << 1);
show(true << 4);
show(null >> 1);
$s = 1;
$s <<= 5;
show($s);
$s >>= 2;
show($s);
