<?php
function say($what, $value) {
    echo "<", $what, ">";
    return $value;
}
function target($a = "-", $b = "-", $c = "-", ...$rest) {
    echo " => ", $a, $b, $c;
    foreach ($rest as $k => $v) {
        echo " ", $k, ":", $v;
    }
    echo "\n";
}
target(say("first", 1), ...say("unpack", [2, 3]), ...say("second unpack", [4]));
target(...say("u1", [1]), c: say("named c", 3));
target(say("p", 1), ...say("u", ["b" => 2]), c: say("n", 3));
target(c: say("c", 3), a: say("a", 1), b: say("b", 2));
target(say("one", 1), b: say("two", say("inner", 2)));

$i = 0;
target($i++, $i++, ...[$i++, $i++]);
echo $i, "\n";

$v = "old";
target($v, ...[$v = "new"], c: $v);

$arr = [1, 2];
target(...$arr, ...[$arr[] = 3], ...$arr);

function nested($x) {
    echo "(nested ", $x, ")";
    return [$x, $x + 1];
}
target(...nested(1), ...nested(10));
target(say("outer", 0), ...nested(say("inner arg", 5)));

$undef_order = [say("a", 1) => say("b", 2)];
target(...$undef_order);
