<?php
$x = 1;
$read = fn () => $x;
$x = 2;
echo "captured_at_creation: ", $read(), "\n";
echo "outer_x_after: ", $x, "\n";

$y = 10;
$assign = fn () => $y = 5;
echo "assign_result: ", $assign(), "\n";
echo "outer_y_after_assign: ", $y, "\n";

$z = 3;
$inc = fn () => $z++;
echo "post_inc_result: ", $inc(), "\n";
echo "outer_z_after_inc: ", $z, "\n";
echo "second_call: ", $inc(), "\n";
echo "outer_z_still: ", $z, "\n";

$w = "a";
$concat = fn ($s) => $w . $s;
$w = "b";
echo "concat_sees_a: ", $concat("!"), "\n";
echo "outer_w: ", $w, "\n";

$arr = [1, 2];
$pushCopy = fn () => $arr[] = 3;
$pushCopy();
echo "outer_arr_after_push:";
foreach ($arr as $v) {
    echo " ", $v;
}
echo "\n";

$arrVal = fn () => $arr;
$arr[] = 9;
echo "closure_arr_snapshot:";
foreach ($arrVal() as $v) {
    echo " ", $v;
}
echo "\n";
echo "outer_arr_now:";
foreach ($arr as $v) {
    echo " ", $v;
}
echo "\n";
