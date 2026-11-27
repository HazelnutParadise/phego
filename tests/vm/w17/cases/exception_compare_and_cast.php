<?php
// exception_compare_and_cast: comparison, boolean cast, array key use and interpolation of exception objects
$a = new Exception("same text");
$b = new Exception("same text");
$c = $a;
echo "identical_self: ", ($a === $c ? 1 : 0), "\n";
echo "identical_other: ", ($a === $b ? 1 : 0), "\n";
echo "equal_other_different_line: ", ($a == $b ? 1 : 0), "\n";
echo "not_equal_other: ", ($a != $b ? 1 : 0), "\n";
echo "not_equal_self: ", ($a != $c ? 1 : 0), "\n";
echo "bool_cast: ", ((bool)$a ? 1 : 0), "\n";
echo "not_operator: ", (!$a ? 1 : 0), "\n";
echo "interpolated: {$a->getMessage()}!\n";
echo "message_equal: ", ($a->getMessage() === $b->getMessage() ? 1 : 0), "\n";

$list = [];
try {
    $list[$a] = "value";
    echo "not reached\n";
} catch (TypeError $t) {
    echo "key_error_class: ", $t::class, "\n";
    echo "key_error_message_nonempty: ", ($t->getMessage() === "" ? 0 : 1), "\n";
}
echo "list_untouched: ", (isset($list[0]) ? 1 : 0), "\n";

$x = new Exception("twin", 1);
$y = new Exception("twin", 1);
$yy = $y;
echo "same_object_eq_via_alias: ", ($y == $yy ? 1 : 0), "\n";
echo "twin_messages_eq: ", ($x->getMessage() === $y->getMessage() ? 1 : 0), "\n";
echo "twin_codes_eq: ", ($x->getCode() === $y->getCode() ? 1 : 0), "\n";
echo "end\n";
