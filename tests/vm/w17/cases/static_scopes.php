<?php
for ($i = 0; $i < 3; $i++) {
    static $top = 100;
    $top += $i;
    echo $top, " ";
}
echo "\n";

static $main_expr = "main" . " " . "static";
echo $main_expr, "\n";

function branch($which) {
    if ($which) {
        static $a = "A";
        $a .= "a";
        return $a;
    }
    static $b = "B";
    $b .= "b";
    return $b;
}
echo branch(true), " ", branch(false), " ", branch(true), " ", branch(false), "\n";

function before_declaration() {
    echo isset($late) ? "set" : "unset", " ";
    $late = "assigned first";
    static $late = "static value";
    echo $late, " ";
    $late .= "+";
}
before_declaration();
before_declaration();
echo "\n";

function in_loop() {
    $out = "";
    for ($i = 0; $i < 3; $i++) {
        static $acc = "";
        $acc .= $i;
        $out = $acc;
    }
    return $out;
}
echo in_loop(), " ", in_loop(), "\n";

function param_shadow($p) {
    static $p = "static p";
    $p .= "!";
    return $p;
}
echo param_shadow("arg"), " ", param_shadow("arg2"), "\n";

$g = "global g";
function static_and_global() {
    static $x = 0;
    global $g;
    $x++;
    $g .= $x;
}
static_and_global();
static_and_global();
echo $g, "\n";

function copies() {
    static $arr = [1, 2];
    $copy = $arr;
    $copy[] = "only in copy";
    $arr[] = 3;
    $n = 0;
    foreach ($arr as $v) {
        $n++;
    }
    $m = 0;
    foreach ($copy as $v) {
        $m++;
    }
    return $n . "/" . $m;
}
echo copies(), " ", copies(), "\n";
