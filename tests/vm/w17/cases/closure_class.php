<?php
// closure_class: type identity, comparison, cloning, casts, passing and returning
$f = function () { return 1; };
echo "instanceof: ", $f instanceof Closure ? "yes" : "no", "\n";
echo "class_name: ", $f::class, "\n";
$g = $f;
echo "same_object_identity: ", $f === $g ? "same" : "different", "\n";
echo "same_object_equal: ", $f == $g ? "equal" : "unequal", "\n";

function make() {
    return function () { return 1; };
}
$h1 = make();
$h2 = make();
echo "two_from_one_declaration_identity: ", $h1 === $h2 ? "same" : "different", "\n";
echo "two_from_one_declaration_equal: ", $h1 == $h2 ? "equal" : "unequal", "\n";
echo "two_from_one_declaration_class: ", $h2 instanceof Closure ? "Closure" : "other", "\n";

$c = clone $f;
echo "clone_identity: ", $c === $f ? "same" : "different", "\n";
echo "clone_instanceof: ", $c instanceof Closure ? "yes" : "no", "\n";
echo "clone_call: ", $c(), "\n";

echo "bool_cast: ", (bool)$f ? "true" : "false", "\n";

$arr = (array)$f;
$count = 0;
foreach ($arr as $k => $v) {
    $count = $count + 1;
    echo "elem_key: ", $k, "\n";
    echo "elem_is_closure: ", $v === $f ? "same" : "different", "\n";
}
echo "array_cast_elements: ", $count, "\n";

function run($cb, $x) {
    return $cb($x);
}
$triple = function ($x) { return $x * 3; };
echo "passed: ", run($triple, 4), "\n";

function wrap($v) {
    return function () use ($v) { return "wrapped:" . $v; };
}
echo "returned_direct: ", wrap("w")(), "\n";
$w = wrap("q");
echo "returned_stored: ", $w(), "\n";
echo "returned_is_closure: ", $w instanceof Closure ? "yes" : "no", "\n";
