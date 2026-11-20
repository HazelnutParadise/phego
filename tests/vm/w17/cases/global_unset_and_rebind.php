<?php
$g = "global value";
$h = 1;

function unset_local() {
    global $g;
    unset($g);
    echo "after unset: ";
    echo isset($g) ? "set" : "unset", "\n";
    $g = "local value";
    echo $g, "\n";
}
unset_local();
echo $g, "\n";

function unset_then_global() {
    global $g;
    unset($g);
    global $g;
    $g = "written through the second binding";
}
unset_then_global();
echo $g, "\n";

function rebind() {
    global $g, $h;
    $local = "mine";
    $g = &$local;
    $g = "changed local only";
    echo $local, "\n";
    $h = &$g;
    $h = "still local";
    echo $local, "\n";
}
rebind();
echo $g, " ", $h, "\n";

function twice() {
    global $h;
    global $h;
    $h++;
}
twice();
echo $h, "\n";

function param_then_global($h) {
    echo "param: ", $h, "\n";
    global $h;
    echo "global: ", $h, "\n";
    $h = 50;
}
param_then_global("arg");
echo $h, "\n";

unset($g);
function after_main_unset() {
    global $g;
    echo "null after main unset: ", $g === null ? "yes" : "no", "\n";
    $g = "back";
}
after_main_unset();
echo $g, "\n";
