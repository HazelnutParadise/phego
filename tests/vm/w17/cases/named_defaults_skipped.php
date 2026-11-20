<?php
const TEN = 10;

function defaults($a = 1, $b = TEN, $c = TEN * 2, $d = [1, 2], $e = null) {
    echo $a, " ", $b, " ", $c, " ", $d[1], " ", $e === null ? "null" : $e, "\n";
}
defaults(e: "e");
defaults(c: "c");
defaults(d: ["x", "y"], a: "a");
defaults(b: "b", e: "e");

function warns($a = "5 apples" + TEN, $b = [1][3] ?? "fallback", $c = "c") {
    echo $a, " ", $b, " ", $c, "\n";
}
warns(c: "first");
warns(c: "second");
warns(b: "b given", c: "third");
warns();

function array_default($list = [TEN, TEN + 1], $flag = false) {
    $list[] = "local";
    $n = 0;
    foreach ($list as $v) {
        $n++;
    }
    echo $n, $flag ? " flagged" : " plain", "\n";
}
array_default(flag: true);
array_default(flag: false);

function optional_then_required($opt = "opt", $req = "req default", $last = "last") {
    echo $opt, " ", $req, " ", $last, "\n";
}
optional_then_required(last: "L");
optional_then_required(req: "R");

function order_of_defaults($a = TEN + 0.5, $b = "b" . TEN, $c = -TEN, $z = "given") {
    echo $a, " ", $b, " ", $c, " ", $z, "\n";
}
order_of_defaults(z: "z1");
order_of_defaults(z: "z2", b: "B");

function by_ref_default(&$r = "ref default", $other = 0) {
    $r .= "!";
    return $r;
}
echo by_ref_default(other: 1), " ", by_ref_default(other: 2), "\n";

function null_passed($a = "default a", $b = "default b") {
    echo $a === null ? "null" : $a, " ", $b, "\n";
}
null_passed(a: null);
null_passed(b: null, a: null);
