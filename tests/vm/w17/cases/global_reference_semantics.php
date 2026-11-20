<?php
$list = [1, 2, 3];
$name = "n";

function double_all() {
    global $list;
    foreach ($list as &$v) {
        $v *= 2;
    }
}
double_all();
foreach ($list as $k => $v) {
    echo $k, "=", $v, " ";
}
echo "\n";

function alias() {
    global $list;
    $mine = &$list;
    $mine[] = 99;
    $copy = $list;
    $copy[0] = "copy";
    echo "copy[0]=", $copy[0], " list[0]=", $list[0], "\n";
}
alias();
echo "last=", $list[3], "\n";

function by_ref(&$x) {
    $x = "set by reference";
}
function pass_global() {
    global $name;
    by_ref($name);
}
pass_global();
echo $name, "\n";

function nested_write() {
    global $tree;
    $tree["a"]["b"][] = 1;
    $tree["a"]["b"][] = 2;
    $tree["a"]["c"] = "leaf";
}
nested_write();
echo $tree["a"]["b"][1], " ", $tree["a"]["c"], "\n";

$main_copy = $list;
function change_after_copy() {
    global $list;
    $list[0] = "changed";
}
change_after_copy();
echo $main_copy[0], " ", $list[0], "\n";

function swap() {
    global $p, $q;
    [$p, $q] = [$q, $p];
}
$p = "first";
$q = "second";
swap();
echo $p, " ", $q, "\n";

function incr() {
    global $n;
    return ++$n;
}
echo incr(), incr(), incr(), " ", $n, "\n";
