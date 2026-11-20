<?php
$a = 1;
$b = "two";

function read() {
    global $a, $b;
    echo "read: ", $a, " ", $b, "\n";
}

function write() {
    global $a, $b;
    $a = $a + 10;
    $b .= "!";
}

function create() {
    global $made, $other;
    echo "made is null: ", $made === null ? "yes" : "no", "\n";
    $made = "created";
    $other[] = 5;
    $other["k"] = 6;
}

read();
write();
read();
echo $a, " ", $b, "\n";
create();
echo $made, "\n";
foreach ($other as $k => $v) {
    echo $k, "=", $v, "\n";
}

function local_only() {
    $a = "local";
    echo $a, "\n";
}
local_only();
echo $a, "\n";

function no_global() {
    echo "no global: ";
    echo $a;
    echo "\n";
}
no_global();
