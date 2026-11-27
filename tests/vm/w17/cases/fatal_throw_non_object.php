<?php
// fatal_throw_non_object: uncaught throw of an int at top level
$value = 5;
$text = "7";
echo "before\n";
echo "value plus one: ", $value + 1, "\n";
echo "value is int: ", $value === 5 ? "yes" : "no", "\n";
echo "numeric string plus one: ", $text + 1, "\n";
echo "is_object: ", $value instanceof stdClass ? "yes" : "no", "\n";
$list = [$value, $text];
echo "list item: ", $list[0], "\n";
echo "list item type check: ", $list[1] === "7" ? "string" : "other", "\n";
echo "throwing value: ", $value, "\n";
throw $value;
echo "never\n";
