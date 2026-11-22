<?php
// use_arrays: array captures are copies by value and shared by reference
function dumpList($label, $arr) {
    echo $label, ":";
    foreach ($arr as $v) {
        echo " ", $v;
    }
    echo "\n";
}

$list = [1, 2];
$addByVal = function () use ($list) {
    $list[] = 99;
    dumpList("inside_by_value", $list);
};
$addByVal();
dumpList("outer_after_by_value_call", $list);
$list[] = 5;
$addByVal();
dumpList("outer_end", $list);

$base = [1];
$seeBase = function () use ($base) {
    dumpList("seebase", $base);
};
$base[] = 2;
$seeBase();
dumpList("base_outer", $base);

$log = [];
$record = function ($msg) use (&$log) {
    $log[] = $msg;
};
$record("first");
$record("second");
dumpList("log_outside", $log);

$cfg = ["db" => ["host" => "h1", "port" => "p1"]];
$setHost = function ($h) use (&$cfg) {
    $cfg["db"]["host"] = $h;
};
$setHost("h2");
echo "nested_host: ", $cfg["db"]["host"], "\n";

$cfgCopy = function () use ($cfg) {
    $cfg["db"]["port"] = "changed";
    return $cfg["db"]["port"];
};
echo "copy_inside: ", $cfgCopy(), "\n";
echo "copy_outside_port: ", $cfg["db"]["port"], "\n";

$nestedRef = function () use (&$cfg) {
    $cfg["db"]["port"] = "p9";
};
$nestedRef();
echo "nested_ref_port: ", $cfg["db"]["port"], "\n";
echo "nested_ref_host: ", $cfg["db"]["host"], "\n";
