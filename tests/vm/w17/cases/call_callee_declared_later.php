<?php
function caller() {
    later(
        b: $u1,
        a: $u2
    );
    later(
        $u3,
        $u4
    );
    later(...[
        1], b: $u5);
}
caller();
function later($a = 1, $b = 2) { echo "ok\n"; }
function rec($n, $a = 1, $b = 2) {
    if ($n > 0) {
        return rec(
            $n - 1,
            b: $undef_b,
            a: $undef_a
        );
    }
    return "done\n";
}
echo rec(1);
function known($a = 1, $b = 2) { echo "known\n"; }
function after_known() {
    known(
        b: $k1,
        a: $k2
    );
    known(
        $k3,
        $k4
    );
}
after_known();
known(
    $m1,
    b: $m2
);
