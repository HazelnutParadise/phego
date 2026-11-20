<?php
function bump(&...$refs) {
    foreach ($refs as $i => &$r) {
        $r = $r + $i + 1;
    }
}
$a = 10;
$b = 20;
$c = 30;
bump($a, $b, $c);
echo $a, " ", $b, " ", $c, "\n";
bump();
bump($a);
echo $a, "\n";

function first_by_value($x, &...$others) {
    $x = "changed";
    foreach ($others as $k => $v) {
        $others[$k] = $v . "!";
    }
}
$p = "p";
$q = "q";
$r = "r";
first_by_value($p, $q, $r);
echo $p, " ", $q, " ", $r, "\n";

function first_by_ref(&$x, ...$others) {
    $x = "x changed";
    foreach ($others as $k => $v) {
        $others[$k] = "ignored";
    }
}
first_by_ref($p, $q, $r);
echo $p, " ", $q, " ", $r, "\n";

function creates(&...$made) {
    $made[0] = "created zero";
    $made[1][] = "pushed";
}
creates($new_var, $new_arr["deep"]);
echo $new_var, " ", $new_arr["deep"][0], "\n";

$list = [1, 2, 3];
function elements(&...$e) {
    $e[0] *= 10;
    $e[1] *= 10;
}
elements($list[0], $list[2]);
echo $list[0], " ", $list[1], " ", $list[2], "\n";
$copy = $list;
$copy[0] = "copy";
echo $list[0], " ", $copy[0], "\n";

function same_twice(&...$r) {
    $r[0] = "first write";
    echo $r[1], "\n";
    $r[1] = "second write";
}
$same = "start";
same_twice($same, $same);
echo $same, "\n";

function keep(&...$r) {
    static $kept = [];
    foreach ($r as &$one) {
        $kept[] = &$one;
    }
    foreach ($kept as $i => &$k) {
        $k = "kept " . $i;
    }
}
$k1 = 1;
$k2 = 2;
keep($k1);
keep($k2);
echo $k1, " ", $k2, "\n";
