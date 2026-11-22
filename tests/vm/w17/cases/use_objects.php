<?php
// use_objects: objects are handles, reassigning the variable is separate
class Box {
    public $v = 0;
}

$box = new Box();
$byVal = function () use ($box) {
    $box->v = $box->v + 1;
    return $box->v;
};
echo "by_value_call_1: ", $byVal(), "\n";
echo "outer_prop_seen: ", $box->v, "\n";
echo "by_value_call_2: ", $byVal(), "\n";

$reassign = function () use ($box) {
    $box = new Box();
    $box->v = 500;
    return $box->v;
};
echo "reassign_inside: ", $reassign(), "\n";
echo "outer_still_same: ", $box->v, "\n";

$holder = function () use ($box) {
    return $box;
};
echo "returned_same_object: ", $holder() === $box ? "same" : "different", "\n";

$byRef = function () use (&$box) {
    $box = new Box();
    $box->v = 900;
    return $box->v;
};
echo "reassign_by_ref_inside: ", $byRef(), "\n";
echo "outer_new_object: ", $box->v, "\n";
echo "old_object_kept_by_holder: ", $holder()->v, "\n";

$tmp = new Box();
$tmp->v = 7;
$keep = function () use ($tmp) {
    return $tmp->v;
};
$tmp = null;
echo "kept_after_null: ", $keep(), "\n";
