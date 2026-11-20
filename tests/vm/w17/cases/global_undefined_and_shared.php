<?php
function producer() {
    global $never_in_main;
    $never_in_main[] = "produced";
}
function consumer() {
    global $never_in_main;
    foreach ($never_in_main as $v) {
        echo $v, "\n";
    }
    echo isset($never_in_main[1]) ? "two" : "one", "\n";
}
producer();
consumer();
producer();
consumer();

function reads_unset_global() {
    global $nothing;
    echo "value: [", $nothing, "]\n";
    echo isset($nothing) ? "isset" : "not set", "\n";
    echo empty($nothing) ? "empty" : "not empty", "\n";
}
reads_unset_global();

function warns() {
    echo $undeclared;
    global $undeclared;
    echo "[", $undeclared, "]\n";
}
$undeclared = "main value";
warns();

function conditional($use) {
    if ($use) {
        global $cond;
    }
    $cond = "from conditional " . ($use ? "global" : "local");
}
$cond = "start";
conditional(false);
echo $cond, "\n";
conditional(true);
echo $cond, "\n";

function compound() {
    global $num, $str, $arr;
    $num += 5;
    $num *= 2;
    $str .= "b";
    $arr["x"] ??= "default";
    $arr["x"] .= "!";
    $num--;
}
$str = "a";
compound();
compound();
echo $num, " ", $str, " ", $arr["x"], "\n";
