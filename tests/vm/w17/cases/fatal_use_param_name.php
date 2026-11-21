<?php
// Compile-time rule: a lexical variable cannot have the same name as a parameter.
// The whole file fails to compile, so none of the code below runs.

class Holder
{
    public $value = 10;

    public function make()
    {
        return function () {
            return $this->value;
        };
    }
}

$h = new Holder();
$f = $h->make();
echo "bound closure reads: ", $f(), "\n";

$x = 1;
$ok = function ($y) use ($x) {
    return $y + $x;
};
echo "valid closure: ", $ok(2), "\n";

// Invalid: $x is both a parameter and a lexical variable.
$bad = function ($x) use ($x) {
    return $x;
};

echo "not reached\n";
