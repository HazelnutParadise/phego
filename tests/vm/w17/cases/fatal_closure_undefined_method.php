<?php
// A closure is an object of class Closure. Calling a method name that Closure
// does not have is a fatal error, even though the value is callable.

$f = function ($a) {
    return $a * 3;
};

echo "closure call: ", $f(2), "\n";
echo "is closure: ", $f instanceof Closure ? "yes" : "no", "\n";

$g = $f;
echo "same object: ", $g === $f ? "same" : "different", "\n";

class Plain
{
    public function ping()
    {
        return "pong";
    }
}

$p = new Plain();
echo "plain method: ", $p->ping(), "\n";

echo "about to call a missing method on a closure\n";

echo "result: ", $f->nope(), "\n";

echo "not reached\n";
