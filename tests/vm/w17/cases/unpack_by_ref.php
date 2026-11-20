<?php
function show($label, $a) {
    echo $label, ":";
    foreach ($a as $k => $v) {
        echo " ", $k, "=", $v;
    }
    echo "\n";
}

function set_both(&$a, &$b) {
    $a = "A";
    $b = "B";
}
$arr = [1, 2];
set_both(...$arr);
show("variable", $arr);

$shared = [1, 2];
$other = $shared;
set_both(...$shared);
show("shared", $shared);
show("other", $other);

set_both(...[1, 2]);
echo "temporary array passed\n";

function make() {
    return [1, 2];
}
set_both(...make());
echo "call result passed\n";

function second_only($a, &$b) {
    $a = "ignored";
    $b = "written";
}
$mixed = ["keep", "replace"];
second_only(...$mixed);
show("mixed", $mixed);

$after = $mixed;
$after[1] = "copy changed";
show("mixed again", $mixed);
show("after", $after);

function all_refs(&...$r) {
    foreach ($r as $k => &$v) {
        $v = $k . "!";
    }
}
$many = [5 => 3, "a" => 1, "b" => 2];
all_refs(...$many);
show("many", $many);

$nested = ["in" => [1, 2]];
set_both(...$nested["in"]);
show("nested element", $nested["in"]);

function named_ref($x = 0, &$y = 0) {
    $y = "y set";
}
$named = ["y" => "old"];
named_ref(...$named);
show("named", $named);

$partial = [1];
function three(&$a, &$b = "db", &$c = "dc") {
    $a = "a!";
    $b = "b!";
    $c = "c!";
}
three(...$partial);
show("partial", $partial);

$alias_target = [1, 2];
$alias = &$alias_target;
set_both(...$alias);
show("through alias", $alias_target);

function no_refs($a, $b) {
    $a = 0;
}
$untouched = [1, 2];
$share = $untouched;
no_refs(...$untouched);
$share[0] = "share";
show("untouched", $untouched);
