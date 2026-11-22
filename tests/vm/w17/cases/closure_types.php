<?php
// closure_types: weak-mode coercion and declared parameter and return types
$toInt = function (int $n): int {
    return $n;
};
$r = $toInt("5");
echo "string_to_int: ", $r, " is_int=", $r === 5 ? "yes" : "no", "\n";

$toStr = function (string $s): string {
    return $s . "!";
};
echo "int_to_string: ", $toStr(42), "\n";
echo "float_to_string: ", $toStr(1.5), "\n";

$toFloat = function (float $x): float {
    return $x;
};
echo "int_to_float: ", $toFloat(3) === 3.0 ? "float3" : "other", "\n";
echo "numeric_string_to_float: ", $toFloat("2.5") === 2.5 ? "ok" : "bad", "\n";

$toBool = function (bool $b): string {
    return $b ? "T" : "F";
};
echo "bool_from_1: ", $toBool(1), "\n";
echo "bool_from_0: ", $toBool(0), "\n";

$maybe = function (?int $n): string {
    return $n === null ? "null" : "int:" . $n;
};
echo "nullable_null: ", $maybe(null), "\n";
echo "nullable_value: ", $maybe("8"), "\n";

$union = function (int|string $v): string {
    return "v=" . $v;
};
echo "union_string_kept: ", $union("7"), "\n";
echo "union_int: ", $union(7), "\n";

$retInt = function ($s): int {
    return $s;
};
echo "return_coerced: ", $retInt("12") + 1, "\n";

$v = function (): void {
    echo "void body runs\n";
};
echo "void_result_null: ", $v() === null ? "yes" : "no", "\n";

class Box {
    public $label = "box";
}
$boxLabel = function (Box $b): Box {
    return $b;
};
$bx = new Box();
echo "class_param: ", $boxLabel($bx)->label, "\n";
echo "class_same_object: ", $boxLabel($bx) === $bx ? "same" : "different", "\n";

$apply = function (Closure $c, $x): Closure {
    echo "apply_called\n";
    return $c;
};
$back = $apply(function ($y) { return "got:" . $y; }, 1);
echo "closure_param_and_return: ", $back("ok"), "\n";

$maybeFn = function (bool $on): ?Closure {
    return $on ? function () { return "on"; } : null;
};
echo "nullable_closure_null: ", $maybeFn(false) === null ? "null" : "set", "\n";
echo "nullable_closure_set: ", $maybeFn(true)(), "\n";
