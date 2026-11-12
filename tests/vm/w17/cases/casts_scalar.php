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

show((int)"12abc");
show((int)"  12");
show((int)"12  ");
show((int)"abc");
show((int)"1e3");
show((int)"0x1A");
show((int)" 0b11");
show((int)-1.9);
show((int)1.9);
show((int)true);
show((int)null);
show((float)"1.5e3abc");
show((float)".5");
show((float)"abc");
show((float)7);
show((string)1.0);
show((string)0.1);
show((string)true);
show((string)false);
show((string)null);
show((string)-0.0);
show((bool)"0");
show((bool)"0.0");
show((bool)"");
show((bool)" ");
show((bool)0.0);
show((bool)-0.0);
show((bool)[]);
show((bool)[0]);
show((bool)null);
$a = (array)5;
echo $a[0], "\n";
$b = (array)null;
echo isset($b[0]) ? "set" : "empty", "\n";
$c = (array)"str";
echo $c[0], "\n";
$d = (array)[1, 2];
echo $d[0], $d[1], "\n";
$e = (array)false;
echo $e[0] === false ? "false inside" : "other", "\n";
$f = (array)1.5;
echo $f[0], "\n";
echo (array)1 === [1] ? "same" : "diff", "\n";
echo (array)null === [] ? "empty" : "diff", "\n";
