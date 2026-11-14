<?php
$x = 9;
echo match (
    $x
) {
    PHP_EOL . "x"
        => "a",
    [1]
        => "b",
};
