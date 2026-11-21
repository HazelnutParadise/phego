<?php
function hello($who) {
    return "hello " . $who;
}

class S {
    public static function make($n) {
        return "S::make " . $n;
    }
}

$f1 = 'hello'(...);
echo "literal_name: ", $f1("a"), "\n";

$name = "hello";
$f2 = $name(...);
echo "variable_name: ", $f2("b"), "\n";

$f3 = 'S::make'(...);
echo "static_string_literal: ", $f3(1), "\n";

$sm = "S::make";
$f4 = $sm(...);
echo "static_string_variable: ", $f4(2), "\n";

echo "plain_literal_call: ", 'hello'("c"), "\n";
echo "plain_variable_call: ", $name("d"), "\n";
echo "plain_static_literal: ", 'S::make'(3), "\n";
echo "plain_static_variable: ", $sm(4), "\n";

$up = "HELLO";
$f5 = $up(...);
echo "case_insensitive_name: ", $f5("e"), "\n";
echo "closure_type: ", $f1 instanceof Closure ? "Closure" : "other", "\n";
echo "same_name_two_closures: ", $f1 === $f2 ? "same" : "different", "\n";
