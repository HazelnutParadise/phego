<?php
// use_by_ref: by-reference captures share one variable with the outer scope
$x = 1;
$seeX = function () use (&$x) {
    return "x=" . $x;
};
echo "before_change: ", $seeX(), "\n";
$x = 2;
echo "outer_change_seen: ", $seeX(), "\n";

$setX = function ($v) use (&$x) {
    $x = $v;
};
$setX(40);
echo "inside_write_seen_outside: ", $x, "\n";

$a = 0;
$inc = function () use (&$a) {
    $a = $a + 1;
    return $a;
};
$get = function () use (&$a) {
    return $a;
};
echo "inc_1: ", $inc(), "\n";
echo "inc_2: ", $inc(), "\n";
echo "get: ", $get(), "\n";
$a = 10;
echo "get_after_outer: ", $get(), "\n";

$made = function () use (&$brandNew) {
    return $brandNew === null ? "null" : "set";
};
echo "created_var_is_null: ", $made(), "\n";
echo "brandNew_outside_null: ", $brandNew === null ? "null" : "set", "\n";

$y = "yes";
$readY = function () use (&$y) {
    return $y;
};
unset($y);
echo "after_unset_outer: ", $readY(), "\n";

$z = "z1";
$writeZ = function ($v) use (&$z) {
    $z = $v;
};
$writeZ("z2");
echo "z_after_write: ", $z, "\n";
$writeZ("z3");
echo "z_after_second_write: ", $z, "\n";
