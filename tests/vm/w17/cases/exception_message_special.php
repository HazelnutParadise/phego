<?php
// exception_message_special: messages with newlines, tabs, empty, long, binary-like bytes, numeric strings and non-string input
$nl = new Exception("line one\nline two");
echo "newline_message: ", $nl->getMessage(), "\n";
echo "newline_string: ", (string)$nl, "\n";

$tab = new Exception("a\tb\r");
echo "tab_message: [", $tab->getMessage(), "]\n";

$long = "";
for ($i = 0; $i < 40; $i++) {
    $long = $long . "abcdefghij";
}
$le = new Exception($long);
echo "long_length_matches: ", ($le->getMessage() === $long ? 1 : 0), "\n";
echo "long_message: ", $le->getMessage(), "\n";

$bin = new Exception("\x01\x7fA\xfe");
echo "binary_message: ", $bin->getMessage(), "\n";
echo "binary_is_bytes: ", ($bin->getMessage() === "\x01\x7fA\xfe" ? 1 : 0), "\n";

$empty = new Exception("");
echo "empty_message: [", $empty->getMessage(), "]\n";
echo "empty_string_form: ", (string)$empty, "\n";

$num = new Exception("12345");
echo "numeric_message: ", $num->getMessage(), "\n";
echo "numeric_is_string: ", ($num->getMessage() === "12345" ? 1 : 0), "\n";

$percent = new Exception('literal %s and $x and {y}');
echo "percent_message: ", $percent->getMessage(), "\n";

$int = new Exception(12);
echo "int_message: ", $int->getMessage(), "\n";
echo "int_message_is_string: ", ($int->getMessage() === "12" ? 1 : 0), "\n";
echo "end\n";
