<?php
function counter() {
    static $n = 0;
    $n++;
    return $n;
}
echo counter(), counter(), counter(), "\n";

function pair() {
    static $a = 1, $b;
    echo "a=", $a, " b=", $b === null ? "null" : $b, "\n";
    $a *= 2;
    $b = $b . "x";
}
pair();
pair();
pair();

function no_init() {
    static $seen;
    if ($seen) {
        return "again";
    }
    $seen = true;
    return "first";
}
echo no_init(), " ", no_init(), " ", no_init(), "\n";

function kinds() {
    static $i = 5, $f = 1.5, $s = "str", $t = true, $z = null, $arr = [1, "k" => 2];
    $i--;
    $f += 0.25;
    $s .= "+";
    $t = !$t;
    $arr[] = $i;
    echo $i, " ", $f, " ", $s, " ", $t ? "T" : "F", " ", $z === null ? "null" : "set";
    foreach ($arr as $k => $v) {
        echo " ", $k, "=", $v;
    }
    echo "\n";
    $z = 1;
}
kinds();
kinds();

function same_name_a() {
    static $v = "a";
    $v .= "a";
    return $v;
}
function same_name_b() {
    static $v = "b";
    $v .= "b";
    return $v;
}
echo same_name_a(), " ", same_name_b(), " ", same_name_a(), " ", same_name_b(), "\n";
