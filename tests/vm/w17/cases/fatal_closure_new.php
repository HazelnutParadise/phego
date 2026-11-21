<?php
// Runtime rule: the Closure class cannot be instantiated with new.
// Closures are only made by function, fn and the callable creation syntax.

$f = function ($a) {
    return $a + 1;
};

echo "closure works: ", $f(1), "\n";
echo "instanceof check: ", $f instanceof Closure ? "yes" : "no", "\n";

class Plain
{
    public $v = 3;
}

$p = new Plain();
echo "plain object ok: ", $p->v, "\n";

$message = "before the bad instantiation\n";
echo $message;

$made = new Closure();

echo "not reached\n";
