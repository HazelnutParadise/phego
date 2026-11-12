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

$i = 5;
show($i++);
show($i);
show(++$i);
show($i--);
show(--$i);
$f = 1.5;
$f++;
show($f);
$f--;
$f--;
show($f);
$n = null;
$n++;
show($n);
$m = null;
$m--;
show($m);
$k = null;
show($k++);
show($k);
$neg = -1;
$neg++;
show($neg);
$big = 9007199254740992.0;
$big++;
show($big);
$t = true;
$t++;
show($t);
$f = false;
$f++;
show($f);
$t2 = true;
$t2--;
show($t2);
$f2 = false;
show($f2--);
show($f2);
