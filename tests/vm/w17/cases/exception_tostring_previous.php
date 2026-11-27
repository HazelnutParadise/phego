<?php
// exception_tostring_previous: string conversion with two levels of previous, each created in a different function
function low() {
    return new DomainException("low level");
}
function mid() {
    return new UnexpectedValueException("mid level", 10, low());
}
function high() {
    return new OverflowException("high level", 20, mid());
}

$e = high();
$first_prev = $e->getPrevious();
$second_prev = $first_prev->getPrevious();
echo "class: ", $e::class, "\n";
echo "previous_class: ", $first_prev::class, "\n";
echo "previous_previous_class: ", $second_prev::class, "\n";
echo "--- string ---\n";
echo (string)$e;
echo "\n--- end ---\n";

$solo = new Exception("solo");
echo "solo_string: ", (string)$solo, "\n";
echo "previous_depth_ok: ", ($second_prev->getPrevious() === null ? 1 : 0), "\n";
