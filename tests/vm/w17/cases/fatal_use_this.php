<?php
// Compile-time rule: $this cannot be captured with a use list.
// The whole file fails to compile, so none of the code below runs.

class Counter
{
    private $n = 0;

    public function bump()
    {
        $this->n++;
        return $this->n;
    }

    public function callback()
    {
        return function () {
            return $this->bump();
        };
    }
}

$c = new Counter();
$f = $c->callback();
echo "bump through closure: ", $f(), "\n";

$plain = function ($a) use ($c) {
    return $a;
};
echo "valid use list: ", $plain(3), "\n";

// Invalid: $this is never a lexical variable.
$bad = function () use ($this) {
    return 1;
};

echo "not reached\n";
