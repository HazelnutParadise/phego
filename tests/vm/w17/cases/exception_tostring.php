<?php
// exception_tostring: string conversion of an exception created in nested calls with arguments
function level_three($label, $n) {
    return new RuntimeException("deep $label");
}
function level_two($x) {
    return level_three("two", $x + 1);
}
function level_one($flag) {
    return level_two(41);
}

$e = level_one(true);
$text = (string)$e;
echo "cast_class: ", $e::class, "\n";
echo $text, "\n";
echo "--- echo ---\n";
echo $e;
echo "\n";
echo "message_only: ", $e->getMessage(), "\n";
echo "line_of_creation: ", $e->getLine(), "\n";
echo "file: ", $e->getFile(), "\n";

$plain = new LogicException("plain");
echo "plain_string: ", (string)$plain, "\n";

function no_args_thrower() {
    return new Exception("called with no args");
}
echo "no_args_string: ", no_args_thrower(), "\n";
echo "concat_length_check: ", (("x" . $e) === ("x" . (string)$e) ? 1 : 0), "\n";
