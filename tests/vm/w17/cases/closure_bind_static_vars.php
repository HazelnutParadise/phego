<?php
class A {
    public $n = 0;
}

$counter = function () {
    static $calls = 0;
    $calls++;
    return $calls;
};

echo "first_call_1: ", $counter(), "\n";
echo "first_call_2: ", $counter(), "\n";

$o = new A;
$copy = $counter->bindTo($o);
echo "copy_continues: ", $copy(), "\n";
echo "original_after_copy: ", $counter(), "\n";
echo "copy_again: ", $copy(), "\n";
echo "copy_again_2: ", $copy(), "\n";

$third = Closure::bind($counter, $o);
echo "third_starts_from_original: ", $third(), "\n";
echo "original_still_counts: ", $counter(), "\n";
echo "third_again: ", $third(), "\n";
echo "copy_unaffected_by_third: ", $copy(), "\n";
