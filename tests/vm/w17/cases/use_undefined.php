<?php
// use_undefined: by-value capture of an undefined variable warns when the closure is created
$f = function () use ($undefined) {
    return $undefined === null ? "null" : "set";
};
echo "between_creation_and_call\n";
echo "called_first: ", $f(), "\n";
echo "called_second: ", $f(), "\n";

$g = function ()
    use ($missing) {
    return $missing === null ? "null" : "set";
};
echo "second_called: ", $g(), "\n";

$h = function () use ($late) {
    return $late === null ? "null" : "set:" . $late;
};
$late = "defined later";
echo "late_not_seen: ", $h(), "\n";

$defined = 1;
$quiet = function () use ($defined) {
    return "defined before creation: " . $defined;
};
echo "no_warning_case: ", $quiet(), "\n";
