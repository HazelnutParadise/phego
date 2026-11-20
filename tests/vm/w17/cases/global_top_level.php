<?php
global $top;
echo "top is null: ", $top === null ? "yes" : "no", "\n";
$top = 3;

function bump() {
    global $top, $late;
    $top++;
    $late = "set before main mentions it";
}
bump();
echo $top, "\n";

global $late, $top;
echo $late, "\n";
$late = "main";
bump();
echo $late, " ", $top, "\n";

if ($top > 0) {
    global $in_branch;
    $in_branch = "branch";
}
function see_branch() {
    global $in_branch;
    echo $in_branch, "\n";
}
see_branch();

for ($i = 0; $i < 3; $i++) {
    global $loop;
    $loop .= $i;
}
echo $loop, "\n";

$copy = $top;
global $copy;
$copy = "changed";
echo $top, " ", $copy, "\n";
