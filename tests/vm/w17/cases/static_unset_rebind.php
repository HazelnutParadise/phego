<?php
function unset_it() {
    static $n = 10;
    $n++;
    echo "n=", $n, " ";
    unset($n);
    echo isset($n) ? "set" : "unset", "\n";
}
unset_it();
unset_it();
unset_it();

function unset_then_assign() {
    static $v = "static";
    unset($v);
    $v = "local";
    return $v;
}
function reads_same_static_again() {
    static $v = "other static";
    return $v;
}
echo unset_then_assign(), " ", unset_then_assign(), " ", reads_same_static_again(), "\n";

function rebind_to_local() {
    static $s = 1;
    $s++;
    $seen = $s;
    $local = "local";
    $s = &$local;
    $s = "written to local";
    return $seen . " " . $local;
}
echo rebind_to_local(), "\n";
echo rebind_to_local(), "\n";
echo rebind_to_local(), "\n";

$target = "global target";
function rebind_to_global() {
    static $s = 0;
    $s += 1;
    $before = $s;
    global $target;
    $s = &$target;
    $s = $s . " +" . $before;
}
rebind_to_global();
rebind_to_global();
echo $target, "\n";

function alias_static() {
    static $data = [];
    $alias = &$data;
    $alias[] = "item";
    $n = 0;
    foreach ($data as $v) {
        $n++;
    }
    return $n;
}
echo alias_static(), alias_static(), alias_static(), "\n";

function by_ref(&$x) {
    $x .= "+";
}
function pass_static() {
    static $s = "s";
    by_ref($s);
    return $s;
}
echo pass_static(), " ", pass_static(), "\n";

function twice_in_body($again) {
    static $a = 1;
    $a++;
    if ($again) {
        unset($a);
        $a = "shadow";
    }
    return $a;
}
echo twice_in_body(false), " ", twice_in_body(true), " ", twice_in_body(false), "\n";
