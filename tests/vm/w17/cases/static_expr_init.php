<?php
const BASE = 40;

function make($label) {
    echo "[make ", $label, "]";
    return $label . "!";
}

function once() {
    static $v = make("once");
    $v .= ".";
    return $v;
}
echo once(), "\n";
echo once(), "\n";
echo once(), "\n";

function from_const() {
    static $a = BASE + 2, $b = PHP_INT_SIZE > 0 ? "sized" : "none", $c = [BASE, BASE * 2];
    $a++;
    return $a . " " . $b . " " . $c[1];
}
echo from_const(), "\n";
echo from_const(), "\n";

function from_param($x) {
    static $first = $x * 2;
    static $second = $first + 1;
    return $first . "/" . $second;
}
echo from_param(5), " ", from_param(100), "\n";

function order() {
    echo "(before)";
    static $a = make("a");
    echo "(between)";
    static $b = make("b");
    echo "(after)";
    return $a . $b;
}
echo order(), "\n";
echo order(), "\n";

function null_init() {
    static $n = make_null();
    echo $n === null ? "null" : "set", "\n";
}
function make_null() {
    echo "[make_null]";
    return null;
}
null_init();
null_init();

function array_expr() {
    static $list = [make("x"), "k" => make("y")];
    $list[] = "more";
    $n = 0;
    foreach ($list as $v) {
        $n++;
    }
    return $n;
}
echo array_expr(), "\n";
echo array_expr(), "\n";
